// Delete use case: remove a tag (unconditional — no replace mode).
package tag

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// DeleteTag deletes the tag. The user must own it; an ownership failure surfaces
// as an AccessDenied (HTTP 403), mirroring the PHP TagService::deleteTag flow
// (the application service checks ownership before delegating). Transactions
// referencing the tag have tag_id set to NULL via the ON DELETE SET NULL FK.
//
// Unlike category-delete, there is no mode/replaceId — tag delete is
// unconditional. Returns an empty result ({}).
func (s *Service) DeleteTag(ctx context.Context, userID vo.Id, req DeleteTagRequest) (*DeleteTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		t, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !t.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &DeleteTagResult{}, nil
}
