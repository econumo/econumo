package tag

import (
	"context"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteTag deletes the tag. The user must own it; an ownership failure surfaces
// as AccessDenied (HTTP 403). Transactions referencing the tag have tag_id set
// to NULL via the ON DELETE SET NULL FK. Delete is unconditional — there is no
// mode/replaceId.
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
		if !t.UserID.Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &DeleteTagResult{}, nil
}
