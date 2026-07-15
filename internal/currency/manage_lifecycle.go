package currency

import (
	"context"

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
	var rec model.CurrencyRecord
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		r, lerr := s.ownedRecord(ctx, req.Id, userID)
		if lerr != nil {
			return lerr
		}
		if uerr := s.repo.UpdateCurrencyDetails(ctx, r.ID, name, symbol, req.FractionDigits); uerr != nil {
			return uerr
		}
		r.Name = &name
		r.Symbol = symbol
		r.FractionDigits = req.FractionDigits
		rec = r
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.UpdateCustomCurrencyResult{Item: toCurrencyResult(rec, ScopeOwn)}, nil
}

func (s *ManageService) ArchiveCurrency(ctx context.Context, userID vo.Id, req model.ArchiveCurrencyRequest) (*model.ArchiveCurrencyResult, error) {
	if err := s.setArchived(ctx, userID, req.Id, true); err != nil {
		return nil, err
	}
	return &model.ArchiveCurrencyResult{}, nil
}

func (s *ManageService) UnarchiveCurrency(ctx context.Context, userID vo.Id, req model.UnarchiveCurrencyRequest) (*model.UnarchiveCurrencyResult, error) {
	if err := s.setArchived(ctx, userID, req.Id, false); err != nil {
		return nil, err
	}
	return &model.UnarchiveCurrencyResult{}, nil
}

func (s *ManageService) setArchived(ctx context.Context, userID vo.Id, id string, archived bool) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.ownedRecord(ctx, id, userID)
		if err != nil {
			return err
		}
		if rec.IsArchived == archived {
			return nil
		}
		return s.repo.SetCurrencyArchived(ctx, rec.ID, archived)
	})
}

func (s *ManageService) DeleteCurrency(ctx context.Context, userID vo.Id, req model.DeleteCurrencyRequest) (*model.DeleteCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.ownedRecord(ctx, req.Id, userID)
		if err != nil {
			return err
		}
		used, err := s.repo.CountCurrencyUsage(ctx, rec.ID, rec.Code)
		if err != nil {
			return err
		}
		if used > 0 {
			return errs.NewValidation("Currency is in use and cannot be deleted")
		}
		return s.repo.DeleteCurrency(ctx, rec.ID)
	}); err != nil {
		return nil, err
	}
	return &model.DeleteCurrencyResult{}, nil
}
