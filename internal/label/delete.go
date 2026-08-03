package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteLabel deletes the label. The user must own it; a foreign-owned label
// is reported as not-found (matching the repo above) so the response can't
// probe which label ids exist. transactions_labels rows referencing it go with
// it via the ON DELETE CASCADE FK. Delete is unconditional — there is no
// mode/replaceId.
func (s *Service) DeleteLabel(ctx context.Context, userID vo.Id, req model.DeleteLabelRequest) (*model.DeleteLabelResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		l, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !l.UserID.Equal(userID) {
			return errs.NewNotFound("Label not found")
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeleteLabelResult{}, nil
}
