package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteLabel deletes the label. The user must own it; a foreign-owned label
// is reported as not-found (matching the repo above) so the response can't
// probe which label ids exist. transactions_labels AND
// recurring_transactions_labels rows referencing it go with it via their
// ON DELETE CASCADE FKs — recurring templates silently lose the label, so
// future postings of those templates no longer carry it. A label a debt
// matches its payments by cannot be deleted; otherwise to keep them, use
// merge-label instead.
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
		if s.debts != nil {
			used, uerr := s.debts.InUse(ctx, id)
			if uerr != nil {
				return uerr
			}
			if used {
				return &errs.ValidationError{Msg: "A debt uses this reporting tag. Delete or change the debt first.", MsgCode: errs.CodeLabelUsedByDebt}
			}
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeleteLabelResult{}, nil
}
