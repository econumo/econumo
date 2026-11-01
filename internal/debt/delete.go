package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteDebt hands a whole-category debt's plans and comments back to the
// category; any other debt's budget data is dropped, after the caller
// confirms.
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
			if err := s.transferData(ctx, d.ID, elementDebt, d.CategoryID, elementCategory, d.CurrencyID); err != nil {
				return err
			}
		} else if !req.ConfirmDataRemoval {
			has, err := s.hasData(ctx, d.ID)
			if err != nil {
				return err
			}
			if has {
				return &errs.ValidationError{Msg: "This deletes the planned amounts and comments of this debt in your budgets; confirm to continue.",
					MsgCode: errs.CodeDebtRemovalUnconfirmed}
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
