package currency

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SetCurrencyRate upserts one dated rate for an owned custom currency against
// the instance base currency. Date defaults to today in the caller's timezone.
func (s *ManageService) SetCurrencyRate(ctx context.Context, userID vo.Id, req model.SetCurrencyRateRequest) (*model.SetCurrencyRateResult, error) {
	if err := validateRate(req.Rate); err != nil {
		return nil, err
	}
	date := todayIn(ctx, s.clock.Now())
	if req.Date != nil && *req.Date != "" {
		parsed, perr := time.ParseInLocation(datetime.DateLayout, *req.Date, time.UTC)
		if perr != nil {
			return nil, errs.NewValidation("Validation failed",
				errs.FieldError{Key: "date", Message: "Date is not valid", Code: errs.CodeCurrencyDateInvalid})
		}
		date = parsed
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.ownedRecord(ctx, req.CurrencyId, userID)
		if err != nil {
			return err
		}
		baseID, err := s.repo.GetGlobalIDByCode(ctx, s.baseCode)
		if err != nil {
			return err
		}
		return s.repo.UpsertRate(ctx, model.RateRow{
			ID: s.nextID().String(), CurrencyID: rec.ID, BaseCurrencyID: baseID,
			Date: date, Rate: req.Rate,
		})
	}); err != nil {
		return nil, err
	}
	return &model.SetCurrencyRateResult{}, nil
}
