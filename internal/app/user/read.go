// CQRS read side of the user module. ReadService answers the read endpoints
// (get-user-data, get-option-list) by issuing purpose-built read queries and
// building the response DTO directly — no domain aggregate and no extra
// per-field round trips. This is the reference pattern for read endpoints across
// all modules; heavy reads (budgets, transaction lists) follow the same shape
// with a single tailored query doing the joins/aggregation.
//
// Writes still go through the aggregate-based Service (service.go); only reads
// take this shortcut.
package user

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
	"github.com/econumo/econumo/internal/infra/auth"
)

// ReadModel is the read-side data source. The infra user ReadRepo implements it.
// Returning lightweight view rows (not domain entities) keeps the read path free
// of aggregate hydration.
type ReadModel interface {
	UserView(ctx context.Context, id string) (UserViewRow, error)
	OptionViews(ctx context.Context, userID string) ([]OptionViewRow, error)
	// CurrencyIDByCode resolves a code to its id; returns sql.ErrNoRows when the
	// code is unknown so the service can apply the USD fallback.
	CurrencyIDByCode(ctx context.Context, code string) (string, error)
}

// UserViewRow / OptionViewRow are the read-side row shapes the ReadModel returns.
// They are declared here, rather than in infra, so the app layer does not import
// the infra package (dependency points inward).
type UserViewRow struct {
	ID        string
	Email     string
	Name      string
	AvatarURL string
}

type OptionViewRow struct {
	Name  string
	Value *string
}

// ReadService serves the user read endpoints.
type ReadService struct {
	read   ReadModel
	encode encoder
}

// NewReadService wires the read service.
func NewReadService(read ReadModel, encode *auth.EncodeService) *ReadService {
	return &ReadService{read: read, encode: encode}
}

// GetUserData returns the current-user view in one read path: the user row, its
// options, and the synthetic currency_id (resolved from the currency option,
// USD fallback) — assembled directly into the DTO.
func (s *ReadService) GetUserData(ctx context.Context, userID vo.Id) (*GetUserDataResult, error) {
	cur, err := s.currentUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &GetUserDataResult{User: cur}, nil
}

// GetOptionList returns the raw persisted options (no synthetic currency_id).
func (s *ReadService) GetOptionList(ctx context.Context, userID vo.Id) (*GetOptionListResult, error) {
	opts, err := s.read.OptionViews(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]OptionResult, 0, len(opts))
	for _, o := range opts {
		items = append(items, OptionResult{Name: o.Name, Value: o.Value})
	}
	return &GetOptionListResult{Items: items}, nil
}

// currentUser builds the CurrentUserResult from read queries. The email is
// decoded; the currency_id option is resolved with a USD fallback. The
// deprecated currency/reportPeriod fields are derived from the persisted options.
func (s *ReadService) currentUser(ctx context.Context, userID vo.Id) (CurrentUserResult, error) {
	u, err := s.read.UserView(ctx, userID.String())
	if err != nil {
		return CurrentUserResult{}, err
	}
	opts, err := s.read.OptionViews(ctx, userID.String())
	if err != nil {
		return CurrentUserResult{}, err
	}

	email, err := s.encode.Decode(u.Email)
	if err != nil {
		return CurrentUserResult{}, err
	}

	options := make([]OptionResult, 0, len(opts)+1)
	currencyCode := domuser.DefaultCurrency
	reportPeriod := domuser.DefaultReportPeriod
	for _, o := range opts {
		options = append(options, OptionResult{Name: o.Name, Value: o.Value})
		switch o.Name {
		case domuser.OptionCurrency:
			if o.Value != nil {
				currencyCode = *o.Value
			}
		case domuser.OptionReportPeriod:
			if o.Value != nil {
				reportPeriod = *o.Value
			}
		}
	}

	// Resolve currency_id, falling back to USD when the code is unknown.
	currencyID, err := s.read.CurrencyIDByCode(ctx, currencyCode)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return CurrentUserResult{}, err
		}
		currencyCode = domuser.DefaultCurrency
		currencyID, err = s.read.CurrencyIDByCode(ctx, currencyCode)
		if err != nil {
			return CurrentUserResult{}, err
		}
	}
	cid := currencyID
	options = append(options, OptionResult{Name: domuser.OptionCurrencyID, Value: &cid})

	return CurrentUserResult{
		Id:           u.ID,
		Name:         u.Name,
		Email:        email,
		Avatar:       u.AvatarURL,
		Options:      options,
		Currency:     currencyCode,
		ReportPeriod: reportPeriod,
	}, nil
}
