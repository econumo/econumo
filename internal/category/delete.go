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
// SET NULL FK. To keep them, use merge-category instead. The debts attached
// to it go with it.
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

		if err := s.releaseDebts(ctx, id); err != nil {
			return err
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeleteCategoryResult{}, nil
}

// releaseDebts runs before the delete's first write, in the lock order
// DebtUsage describes: the debts' budget rows, then the category row (waiting
// out a debt create on it), then the debts go.
func (s *Service) releaseDebts(ctx context.Context, id vo.Id) error {
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
