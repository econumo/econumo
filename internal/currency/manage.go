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
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ManageModel is the write-side persistence port for per-user custom
// currencies. The infra currency ManageRepo implements it.
type ManageModel interface {
	GetCurrencyRecord(ctx context.Context, id string) (model.CurrencyRecord, error)
	GlobalCodeExists(ctx context.Context, code string) (bool, error)
	OwnerCodeExists(ctx context.Context, userID, code string) (bool, error)
	InsertUserCurrency(ctx context.Context, c model.CurrencyRecord) error
	UpdateCurrencyDetails(ctx context.Context, id, name, symbol string, fractionDigits int) error
	SetCurrencyArchived(ctx context.Context, id string, archived bool) error
	DeleteCurrency(ctx context.Context, id string) error
	CountCurrencyUsage(ctx context.Context, id, code string) (int64, error)
	GetGlobalIDByCode(ctx context.Context, code string) (string, error)
	UpsertRate(ctx context.Context, r model.RateRow) error
	HideCurrency(ctx context.Context, userID, currencyID string, now time.Time) error
	ShowCurrency(ctx context.Context, userID, currencyID string) error
}

// ProfileCurrency is the consumer-side port onto the user feature's profile
// currency lookup, satisfied by a glue adapter at composition time.
type ProfileCurrency interface {
	CurrencyCode(ctx context.Context, userID string) (string, error)
}

// ManageService is the currency lifecycle use-case orchestrator (create,
// update, archive, unarchive, delete, set-rate, hide, show).
type ManageService struct {
	repo     ManageModel
	tx       port.TxRunner
	ops      port.OperationGuard
	clock    port.Clock
	profile  ProfileCurrency
	baseCode string
	nextID   func() vo.Id
}

// NewManageService wires the currency manage service. baseCode is the
// application-wide base currency (ECONUMO_CURRENCY_BASE), used to resolve the
// base currency id when an initial rate is supplied at create-currency time.
func NewManageService(repo ManageModel, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, profile ProfileCurrency, baseCode string) *ManageService {
	return &ManageService{repo: repo, tx: tx, ops: ops, clock: clock, profile: profile, baseCode: baseCode, nextID: vo.NewId}
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
	archived := 0
	if rec.IsArchived {
		archived = 1
	}
	return model.CurrencyListItem{
		CurrencyResult: model.CurrencyResult{
			Id:             rec.ID,
			Code:           rec.Code,
			Name:           name,
			Symbol:         rec.Symbol,
			FractionDigits: rec.FractionDigits,
		},
		Scope:      scope,
		IsArchived: archived,
		IsHidden:   0,
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
	if req.Rate != nil {
		if err := validateRate(*req.Rate); err != nil {
			return nil, err
		}
	}
	uid := userID.String()
	rec := model.CurrencyRecord{
		ID: s.nextID().String(), Code: code, Symbol: symbol, Name: &name,
		FractionDigits: digits, UserID: &uid,
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
		if req.Rate != nil {
			baseID, berr := s.repo.GetGlobalIDByCode(ctx, s.baseCode)
			if berr != nil {
				return berr
			}
			if serr := s.repo.UpsertRate(ctx, model.RateRow{
				ID: s.nextID().String(), CurrencyID: rec.ID, BaseCurrencyID: baseID,
				Date: todayIn(ctx, now), Rate: *req.Rate,
			}); serr != nil {
				return serr
			}
		}
		return s.ops.MarkHandled(ctx, opID, now)
	}); err != nil {
		return nil, err
	}
	return &model.CreateCurrencyResult{Item: toCurrencyResult(rec, ScopeOwn)}, nil
}

// todayIn resolves "today" in the caller's timezone (X-Timezone header via
// reqctx), truncated to a date, expressed in UTC for storage.
func todayIn(ctx context.Context, now time.Time) time.Time {
	local := now.In(reqctx.Location(ctx))
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}
