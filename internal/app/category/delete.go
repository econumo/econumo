// Delete use case: remove a category (delete mode) or reassign its transactions
// to a replacement and then remove it (replace mode).
package category

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// DeleteCategory deletes the category. The user must own it; an ownership
// failure surfaces as a ValidationError (HTTP 400, "Category not found"), NOT an
// AccessDenied. See CLAUDE.md.
//
//   - mode=delete: just delete the category. Transactions referencing it have
//     category_id set to NULL via the ON DELETE SET NULL FK.
//   - mode=replace: reassign the category's transactions to replaceId, then
//     delete. Both categories must exist, be owned by the user, and share the
//     same type.
//
// Returns an empty result ({}).
func (s *Service) DeleteCategory(ctx context.Context, userID vo.Id, req DeleteCategoryRequest) (*DeleteCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		c, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !c.UserId().Equal(userID) {
			return errs.NewValidation("Category not found")
		}

		if req.Mode == ModeReplace {
			if req.ReplaceId == nil {
				return errs.NewValidation("Category not found")
			}
			replaceID, perr := vo.ParseId(*req.ReplaceId)
			if perr != nil {
				return perr
			}
			replacement, rerr := s.repo.GetByID(ctx, replaceID)
			if rerr != nil {
				return rerr
			}
			if !replacement.UserId().Equal(userID) {
				return errs.NewValidation("Categories cannot be replaced")
			}
			if replacement.Type() != c.Type() {
				return errs.NewValidation("Categories cannot be replaced")
			}
			if rerr := s.repo.ReassignTransactions(ctx, id, replaceID); rerr != nil {
				return rerr
			}
		}

		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &DeleteCategoryResult{}, nil
}
