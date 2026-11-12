package category

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteCategory deletes the category. The user must own it; an ownership
// failure surfaces as a ValidationError (HTTP 400, "Category not found"), NOT an
// AccessDenied. See CLAUDE.md.
//
// Transactions referencing it have category_id set to NULL via the ON DELETE
// SET NULL FK. To keep them, use merge-category instead. Debt rules naming it
// are removed (a debt left with none matches nothing).
//
// Returns an empty result ({}).
func (s *Service) DeleteCategory(ctx context.Context, userID vo.Id, req model.DeleteCategoryRequest) (*model.DeleteCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		c, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !c.UserID.Equal(userID) {
			return &errs.ValidationError{Msg: "Category not found", MsgCode: errs.CodeCategoryNotFound}
		}

		if err := s.releaseDebtRules(ctx, id); err != nil {
			return err
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeleteCategoryResult{}, nil
}

// releaseDebtRules runs before the delete's first write, in the lock order
// DebtUsage describes: the debts' budget rows, then the category row (waiting
// out a debt write adding a rule on it), then the rules are released while
// the row still exists, since debts_rules refers to it.
func (s *Service) releaseDebtRules(ctx context.Context, id vo.Id) error {
	if s.debts == nil {
		return nil
	}
	if err := s.debts.Prepare(ctx, id); err != nil {
		return err
	}
	if err := s.repo.LockRow(ctx, id); err != nil {
		return err
	}
	return s.debts.Release(ctx, id)
}
