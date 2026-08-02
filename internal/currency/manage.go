// User-facing currency management: per-user custom currencies. Global
// currencies (user_id NULL) stay admin/CLI territory; every mutation here
// requires the caller to own the target currency.
package currency

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ManageModel is the write-side persistence port for per-user custom
// currencies. The infra currency ManageRepo implements it.
type ManageModel interface {
	GetCurrencyRecord(ctx context.Context, id string) (model.CurrencyRecord, error)
	GlobalCodeExists(ctx context.Context, code string) (bool, error)
	OwnerCodeExists(ctx context.Context, userID, code string) (bool, error)
	InsertUserCurrency(ctx context.Context, c model.CurrencyRecord) error
	UpdateCurrencyDetails(ctx context.Context, id, name, symbol string, fractionDigits int, rate *string) error
	SoftDeleteCurrency(ctx context.Context, id string) error
	CountCurrencyUsage(ctx context.Context, id string) (int64, error)
	HideCurrency(ctx context.Context, userID, currencyID string, now time.Time) error
	ShowCurrency(ctx context.Context, userID, currencyID string) error
	// HideGlobalCurrencies hides every global currency for userID except
	// excludeIDs, idempotently; ShowGlobalCurrencies clears the hidden flag
	// on every global (customs keep their per-row preference).
	HideGlobalCurrencies(ctx context.Context, userID string, excludeIDs []string, now time.Time) error
	ShowGlobalCurrencies(ctx context.Context, userID string) error
}

// ProfileCurrency is the consumer-side port onto the user feature's profile
// currency lookup, satisfied by a glue adapter at composition time. The
// stored value is a currency id; an empty string means "no default set".
type ProfileCurrency interface {
	CurrencyID(ctx context.Context, userID string) (string, error)
}

// ManageService is the currency lifecycle use-case orchestrator (create,
// update, delete, hide, show).
type ManageService struct {
	repo    ManageModel
	tx      port.TxRunner
	ops     port.OperationGuard
	clock   port.Clock
	profile ProfileCurrency
	base    model.BaseCurrency
	nextID  func() vo.Id
}

// NewManageService wires the currency manage service. base is the
// boot-resolved instance base currency; rate rows are denominated against it.
func NewManageService(repo ManageModel, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, profile ProfileCurrency, base model.BaseCurrency) *ManageService {
	return &ManageService{repo: repo, tx: tx, ops: ops, clock: clock, profile: profile, base: base, nextID: vo.NewId}
}

// rateShape: scale-8 positive decimal string, no sign, no exponent.
var rateShape = regexp.MustCompile(`^[0-9]{1,11}(\.[0-9]{1,8})?$`)

func validateRate(rate string) error {
	bad := errs.NewValidation("Validation failed",
		errs.FieldError{Key: "rate", Message: "Rate must be a positive number", Code: errs.CodeCurrencyRateInvalid})
	if !rateShape.MatchString(rate) {
		return bad
	}
	if strings.Trim(strings.ReplaceAll(rate, ".", ""), "0") == "" {
		return bad // all zeros
	}
	return nil
}

func validateName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if l := len([]rune(n)); l < 1 || l > 64 {
		return "", errs.NewValidation("Validation failed",
			errs.FieldError{Key: "name", Message: "Currency name must be 1-64 characters", Code: errs.CodeCurrencyNameLength})
	}
	return n, nil
}

func validateSymbol(symbol string) (string, error) {
	sym := strings.TrimSpace(symbol)
	if l := len([]rune(sym)); l < 1 || l > 12 {
		return "", errs.NewValidation("Validation failed",
			errs.FieldError{Key: "symbol", Message: "Currency symbol must be 1-12 characters", Code: errs.CodeCurrencySymbolLength})
	}
	return sym, nil
}

func validateFractionDigits(d int) error {
	if d < 0 || d > 8 {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "fractionDigits", Message: "Fraction digits must be between 0 and 8", Code: errs.CodeCurrencyFractionDigitsRange})
	}
	return nil
}

// ownedRecord loads a currency and enforces that the caller owns it. A global
// or foreign currency answers with the same AccessDenied as other features'
// ownership failures (no existence leak beyond what the list already shows).
func (s *ManageService) ownedRecord(ctx context.Context, id string, userID vo.Id) (model.CurrencyRecord, error) {
	rec, err := s.repo.GetCurrencyRecord(ctx, id)
	if err != nil {
		return model.CurrencyRecord{}, err
	}
	if rec.UserID == nil || *rec.UserID != userID.String() {
		return model.CurrencyRecord{}, errs.NewAccessDenied("")
	}
	return rec, nil
}

func toCurrencyResult(rec model.CurrencyRecord, scope string) model.CurrencyListItem {
	name := rec.Code
	if rec.Name != nil && *rec.Name != "" {
		name = *rec.Name
	}
	isDeleted := 0
	if rec.IsDeleted {
		isDeleted = 1
	}
	return model.CurrencyListItem{
		CurrencyResult: model.CurrencyResult{
			Id:             rec.ID,
			Code:           rec.Code,
			Name:           name,
			Symbol:         rec.Symbol,
			FractionDigits: rec.FractionDigits,
		},
		Scope:     scope,
		IsHidden:  0,
		IsDeleted: isDeleted,
	}
}

func (s *ManageService) CreateCurrency(ctx context.Context, userID vo.Id, req model.CreateCurrencyRequest) (*model.CreateCurrencyResult, error) {
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	code, err := validateCodeField(req.Code, "code")
	if err != nil {
		return nil, err
	}
	name, err := validateName(req.Name)
	if err != nil {
		return nil, err
	}
	symbol := code
	if req.Symbol != nil && *req.Symbol != "" {
		if symbol, err = validateSymbol(*req.Symbol); err != nil {
			return nil, err
		}
	}
	digits := 2
	if req.FractionDigits != nil {
		digits = *req.FractionDigits
	}
	if err := validateFractionDigits(digits); err != nil {
		return nil, err
	}
	if req.Rate == nil || strings.TrimSpace(*req.Rate) == "" {
		return nil, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "rate", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if err := validateRate(*req.Rate); err != nil {
		return nil, err
	}
	uid := userID.String()
	rec := model.CurrencyRecord{
		ID: s.nextID().String(), Code: code, Symbol: symbol, Name: &name,
		FractionDigits: digits, UserID: &uid, Rate: req.Rate,
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		already, cerr := s.ops.Claim(ctx, opID, now)
		if cerr != nil {
			return cerr
		}
		if already {
			return &errs.ValidationError{Msg: "Operation is locked", MsgCode: errs.CodeOperationLocked}
		}
		dupOwn, cerr := s.repo.OwnerCodeExists(ctx, uid, code)
		if cerr != nil {
			return cerr
		}
		dupGlobal, cerr := s.repo.GlobalCodeExists(ctx, code)
		if cerr != nil {
			return cerr
		}
		if dupOwn || dupGlobal {
			return errs.NewValidation("Validation failed",
				errs.FieldError{Key: "code", Message: "Currency already exists", Code: errs.CodeCurrencyAlreadyExists})
		}
		rec.CreatedAt = now
		if serr := s.repo.InsertUserCurrency(ctx, rec); serr != nil {
			return serr
		}
		return s.ops.MarkHandled(ctx, opID, now)
	}); err != nil {
		return nil, err
	}
	return &model.CreateCurrencyResult{Item: toCurrencyResult(rec, ScopeOwn)}, nil
}
