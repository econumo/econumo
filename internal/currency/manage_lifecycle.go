package currency

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *ManageService) UpdateCurrency(ctx context.Context, userID vo.Id, req model.UpdateCustomCurrencyRequest) (*model.UpdateCustomCurrencyResult, error) {
	name, err := validateName(req.Name)
	if err != nil {
		return nil, err
	}
	symbol, err := validateSymbol(req.Symbol)
	if err != nil {
		return nil, err
	}
	if err := validateFractionDigits(req.FractionDigits); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Rate) == "" {
		return nil, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "rate", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if err := validateRate(req.Rate); err != nil {
		return nil, err
	}
	var rec model.CurrencyRecord
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		r, lerr := s.ownedRecord(ctx, req.Id, userID)
		if lerr != nil {
			return lerr
		}
		if uerr := s.repo.UpdateCurrencyDetails(ctx, r.ID, name, symbol, req.FractionDigits, &req.Rate); uerr != nil {
			return uerr
		}
		r.Name = &name
		r.Symbol = symbol
		r.FractionDigits = req.FractionDigits
		r.Rate = &req.Rate
		rec = r
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.UpdateCustomCurrencyResult{Item: toCurrencyResult(rec, ScopeOwn)}, nil
}

func (s *ManageService) DeleteCurrency(ctx context.Context, userID vo.Id, req model.DeleteCurrencyRequest) (*model.DeleteCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.ownedRecord(ctx, req.Id, userID)
		if err != nil {
			return err
		}
		used, err := s.repo.CountCurrencyUsage(ctx, rec.ID)
		if err != nil {
			return err
		}
		if used > 0 {
			return &errs.ValidationError{Msg: "Currency is in use and cannot be deleted", MsgCode: errs.CodeCurrencyInUse}
		}
		return s.repo.SoftDeleteCurrency(ctx, rec.ID)
	}); err != nil {
		return nil, err
	}
	return &model.DeleteCurrencyResult{}, nil
}
