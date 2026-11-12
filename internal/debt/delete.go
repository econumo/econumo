package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteDebt hands the plans and comments of a debt whose one rule is
// whole-category back to that category; budget data that cannot go back
// (another currency, a month the category already plans) and any other
// debt's budget data are dropped, after the caller confirms: the plans of a
// debt with several rules cannot be split back by category.
func (s *Service) DeleteDebt(ctx context.Context, userID vo.Id, req model.DeleteDebtRequest) (*model.DeleteDebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	var budgetID vo.Id
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		d, _, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		budgetID = d.BudgetID
		if err := s.lockRows(ctx, d.ID); err != nil {
			return err
		}
		budgetDebts, err := s.repo.ListByBudget(ctx, d.BudgetID)
		if err != nil {
			return err
		}
		if category, ok := model.SoleWholeCategory(d, budgetDebts); ok {
			left, err := s.moveToCategory(ctx, d, category)
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
		// The rows go now, not on the budget's next structure sync: their
		// data is confirmed gone (or moved back) by this point.
		if err := s.removeRows(ctx, d.ID); err != nil {
			return err
		}
		return s.repo.Delete(ctx, d.ID)
	})
	if err != nil {
		return nil, err
	}
	// The debts left in the budget may now cover the removed one's range.
	if err := s.syncBudgetID(ctx, budgetID); err != nil {
		return nil, err
	}
	return &model.DeleteDebtResult{}, nil
}

// The field key is the SPA's handle on this refusal: the envelope carries no
// catalogue code, so the client recognises the error by field.
func removalUnconfirmed() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key:     "confirmDataRemoval",
		Message: "This deletes the planned amounts and comments of this debt in its budget; confirm to continue.",
		Code:    errs.CodeDebtRemovalUnconfirmed,
	})
}
