package currency

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// HideCurrency removes a currency (a global, or the caller's own custom)
// from the caller's dropdowns. The base currency and the caller's profile
// currency must stay visible.
func (s *ManageService) HideCurrency(ctx context.Context, userID vo.Id, req model.HideCurrencyRequest) (*model.HideCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.repo.GetCurrencyRecord(ctx, req.Id)
		if err != nil {
			return err
		}
		// Globals and the caller's OWN customs are hideable; foreign customs
		// are not (hide is each user's own picker preference).
		if rec.UserID != nil && *rec.UserID != userID.String() {
			return &errs.ValidationError{Msg: "This currency cannot be hidden", MsgCode: errs.CodeCurrencyCannotBeHidden}
		}
		if rec.ID == s.base.ID {
			return &errs.ValidationError{Msg: "The base currency cannot be modified", MsgCode: errs.CodeCurrencyBaseImmutable}
		}
		profileID, err := s.profile.CurrencyID(ctx, userID.String())
		if err != nil {
			return err
		}
		if profileID != "" && rec.ID == profileID {
			return &errs.ValidationError{Msg: "This currency cannot be hidden", MsgCode: errs.CodeCurrencyCannotBeHidden}
		}
		return s.repo.HideCurrency(ctx, userID.String(), rec.ID, s.clock.Now())
	}); err != nil {
		return nil, err
	}
	return &model.HideCurrencyResult{}, nil
}

func (s *ManageService) ShowCurrency(ctx context.Context, userID vo.Id, req model.ShowCurrencyRequest) (*model.ShowCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.repo.GetCurrencyRecord(ctx, req.Id)
		if err != nil {
			return err
		}
		return s.repo.ShowCurrency(ctx, userID.String(), rec.ID)
	}); err != nil {
		return nil, err
	}
	return &model.ShowCurrencyResult{}, nil
}

// HideAllCurrencies hides every GLOBAL currency for the caller in one step,
// keeping the base currency and the caller's profile default visible. Customs
// (own and foreign) are out of scope: the bulk action lives on the globals
// section.
func (s *ManageService) HideAllCurrencies(ctx context.Context, userID vo.Id) (*model.HideAllCurrenciesResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		profileID, err := s.profile.CurrencyID(ctx, userID.String())
		if err != nil {
			return err
		}
		exclude := []string{s.base.ID}
		if profileID != "" && profileID != s.base.ID {
			exclude = append(exclude, profileID)
		}
		return s.repo.HideGlobalCurrencies(ctx, userID.String(), exclude, s.clock.Now())
	}); err != nil {
		return nil, err
	}
	return &model.HideAllCurrenciesResult{}, nil
}

// ShowAllCurrencies clears the caller's hidden flag on every GLOBAL currency.
// A hidden own custom keeps its per-row preference.
func (s *ManageService) ShowAllCurrencies(ctx context.Context, userID vo.Id) (*model.ShowAllCurrenciesResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		return s.repo.ShowGlobalCurrencies(ctx, userID.String())
	}); err != nil {
		return nil, err
	}
	return &model.ShowAllCurrenciesResult{}, nil
}
