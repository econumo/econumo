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

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ReadModel is the read-side data source. The infra user ReadRepo implements it.
// Returning lightweight view rows (not domain entities) keeps the read path free
// of aggregate hydration.
type ReadModel interface {
	UserView(ctx context.Context, id string) (model.UserViewRow, error)
	OptionViews(ctx context.Context, userID string) ([]model.OptionViewRow, error)
	// CurrencyIDByCode resolves a code to its id (own custom first, then
	// global); returns sql.ErrNoRows when the code is unknown so the service
	// can apply the USD fallback.
	CurrencyIDByCode(ctx context.Context, userID, code string) (string, error)
	// CurrencyCodeByID maps a stored profile-currency id back to its code
	// (sql.ErrNoRows for dangling ids; callers apply the USD fallback).
	CurrencyCodeByID(ctx context.Context, id string) (string, error)
}

// ReadService serves the user read endpoints.
type ReadService struct {
	read   ReadModel
	encode *auth.EncodeService
	clock  port.Clock
}

// NewReadService wires the read service.
func NewReadService(read ReadModel, encode *auth.EncodeService, clock port.Clock) *ReadService {
	return &ReadService{read: read, encode: encode, clock: clock}
}

// GetUserData returns the current-user view in one read path: the user row, its
// options, and the synthetic currency_id (resolved from the currency option,
// USD fallback) — assembled directly into the DTO.
func (s *ReadService) GetUserData(ctx context.Context, userID vo.Id) (*model.GetUserDataResult, error) {
	cur, err := s.currentUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &model.GetUserDataResult{User: cur}, nil
}

// GetOptionList returns the raw persisted options (no synthetic currency_id).
func (s *ReadService) GetOptionList(ctx context.Context, userID vo.Id) (*model.GetOptionListResult, error) {
	opts, err := s.read.OptionViews(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.OptionResult, 0, len(opts))
	for _, o := range opts {
		value := o.Value
		// The stored currency value is an ID; the options wire is frozen to
		// show the CODE (dangling/absent falls back to the default code).
		if o.Name == model.OptionCurrency {
			code := model.DefaultCurrency
			if value != nil {
				if c, cerr := s.read.CurrencyCodeByID(ctx, *value); cerr == nil {
					code = c
				} else if !errors.Is(cerr, sql.ErrNoRows) {
					return nil, cerr
				}
			}
			value = &code
		}
		items = append(items, model.OptionResult{Name: o.Name, Value: value})
	}
	return &model.GetOptionListResult{Items: items}, nil
}

// currentUser builds the CurrentUserResult from read queries. The email is
// decoded; the currency_id option is resolved with a USD fallback. The
// deprecated currency/reportPeriod fields are derived from the persisted options.
func (s *ReadService) currentUser(ctx context.Context, userID vo.Id) (model.CurrentUserResult, error) {
	u, err := s.read.UserView(ctx, userID.String())
	if err != nil {
		return model.CurrentUserResult{}, err
	}
	opts, err := s.read.OptionViews(ctx, userID.String())
	if err != nil {
		return model.CurrentUserResult{}, err
	}

	email, err := s.encode.Decode(u.Email)
	if err != nil {
		return model.CurrentUserResult{}, err
	}

	// The stored currency value is an ID (the wire keeps showing the code).
	// Dangling/absent -> the frozen USD fallback for both code and id.
	storedCurrencyID := ""
	reportPeriod := model.DefaultReportPeriod
	rawOptions := make([]model.OptionResult, 0, len(opts))
	for _, o := range opts {
		rawOptions = append(rawOptions, model.OptionResult{Name: o.Name, Value: o.Value})
		switch o.Name {
		case model.OptionCurrency:
			if o.Value != nil {
				storedCurrencyID = *o.Value
			}
		case model.OptionReportPeriod:
			if o.Value != nil {
				reportPeriod = *o.Value
			}
		}
	}
	currencyCode := ""
	currencyID := storedCurrencyID
	if storedCurrencyID != "" {
		code, cerr := s.read.CurrencyCodeByID(ctx, storedCurrencyID)
		if cerr != nil && !errors.Is(cerr, sql.ErrNoRows) {
			return model.CurrentUserResult{}, cerr
		}
		currencyCode = code
	}
	if currencyCode == "" {
		currencyCode = model.DefaultCurrency
		fallbackID, cerr := s.read.CurrencyIDByCode(ctx, userID.String(), currencyCode)
		if cerr != nil {
			return model.CurrentUserResult{}, cerr
		}
		currencyID = fallbackID
	}
	options := make([]model.OptionResult, 0, len(rawOptions)+1)
	for _, o := range rawOptions {
		if o.Name == model.OptionCurrency {
			code := currencyCode
			options = append(options, model.OptionResult{Name: o.Name, Value: &code})
			continue
		}
		options = append(options, o)
	}
	cid := currencyID
	options = append(options, model.OptionResult{Name: model.OptionCurrencyID, Value: &cid})

	level := model.EffectiveAccessLevel(u.AccessLevel, u.AccessUntil, s.clock.Now())

	return model.CurrentUserResult{
		Id:           u.ID,
		Name:         u.Name,
		Email:        email,
		Avatar:       u.Avatar,
		Options:      options,
		Currency:     currencyCode,
		ReportPeriod: reportPeriod,
		AccessLevel:  string(level),
		AccessUntil:  datetime.FormatOrEmpty(u.AccessUntil),
	}, nil
}
