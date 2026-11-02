package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteDebt hands a whole-category debt's plans and comments back to the
// category; budget data that cannot go back (another currency, a month the
// category already plans) and any other debt's budget data are dropped, after
// the caller confirms.
func (s *Service) DeleteDebt(ctx context.Context, userID vo.Id, req model.DeleteDebtRequest) (*model.DeleteDebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	var category vo.Id
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockOwner(ctx, userID); err != nil {
			return err
		}
		d, err := s.ownedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		category = d.CategoryID
		if isWholeCategory(d) {
			left, err := s.transferData(ctx, d.ID, elementDebt, d.CategoryID, elementCategory, d.CurrencyID)
			if err != nil {
				return err
			}
			if left && !req.ConfirmDataRemoval {
				return removalUnconfirmed()
			}
		} else if !req.ConfirmDataRemoval {
			has, err := s.hasData(ctx, d.ID)
			if err != nil {
				return err
			}
			if has {
				return removalUnconfirmed()
			}
		}
		return s.repo.Delete(ctx, d.ID)
	})
	if err != nil {
		return nil, err
	}
	// The debts left in the category may now cover the removed one's range.
	if _, err := s.syncCategory(ctx, category); err != nil {
		return nil, err
	}
	return &model.DeleteDebtResult{}, nil
}

// The field key is the SPA's handle on this refusal: the envelope carries no
// catalogue code, so the client recognises the error by field.
func removalUnconfirmed() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key:     "confirmDataRemoval",
		Message: "This deletes the planned amounts and comments of this debt in your budgets; confirm to continue.",
		Code:    errs.CodeDebtRemovalUnconfirmed,
	})
}
