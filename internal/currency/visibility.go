package currency

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// HideCurrency removes a GLOBAL currency from the caller's dropdowns. Custom
// currencies archive instead; the base currency and the caller's profile
// currency must stay visible.
func (s *ManageService) HideCurrency(ctx context.Context, userID vo.Id, req model.HideCurrencyRequest) (*model.HideCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.repo.GetCurrencyRecord(ctx, req.Id)
		if err != nil {
			return err
		}
		if rec.UserID != nil {
			return errs.NewValidation("This currency cannot be hidden")
		}
		if rec.Code == s.baseCode {
			return errs.NewValidation("The base currency cannot be modified")
		}
		profileCode, err := s.profile.CurrencyCode(ctx, userID.String())
		if err != nil {
			return err
		}
		if rec.Code == profileCode {
			return errs.NewValidation("This currency cannot be hidden")
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
