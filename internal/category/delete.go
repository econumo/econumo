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
// SET NULL FK. To keep them, use merge-category instead.
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

		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeleteCategoryResult{}, nil
}
