package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MergeLabel absorbs one label into another: every transaction and recurring
// template carrying the source also gets the target, then the source is
// deleted. Irreversible, and one transaction end to end.
//
// Labels differ from the other classifications in two ways. They are
// many-to-many, so re-pointing has to dedupe — a transaction already carrying
// both ends up with one row, not a duplicate — and the source's own join rows
// cascade away with the label rather than needing a delete. And they are
// budget-neutral, so there are no elements or limits to fold in.
//
// Ownership is checked on BOTH ids before any write: a foreign-owned source is
// masked as not-found, a foreign-owned target is refused.
func (s *Service) MergeLabel(ctx context.Context, userID vo.Id, req model.MergeLabelRequest) (*model.MergeLabelResult, error) {
	sourceID, err := vo.ParseId(req.SourceId)
	if err != nil {
		return nil, err
	}
	targetID, err := vo.ParseId(req.TargetId)
	if err != nil {
		return nil, err
	}

	cannotMerge := &errs.ValidationError{Msg: "The selected tags cannot be merged", MsgCode: errs.CodeLabelCannotBeMerged}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		source, gerr := s.repo.GetByID(txCtx, sourceID)
		if gerr != nil {
			return gerr
		}
		if !source.UserID.Equal(userID) {
			return errs.NewNotFound("Label not found")
		}
		if sourceID.Equal(targetID) {
			return cannotMerge
		}
		target, terr := s.repo.GetByID(txCtx, targetID)
		if terr != nil {
			return terr
		}
		if !target.UserID.Equal(userID) {
			return cannotMerge
		}

		if rerr := s.repo.ReassignTransactions(txCtx, sourceID, targetID); rerr != nil {
			return rerr
		}
		if rerr := s.repo.ReassignRecurring(txCtx, sourceID, targetID); rerr != nil {
			return rerr
		}
		return s.repo.Delete(txCtx, sourceID)
	}); err != nil {
		return nil, err
	}

	return &model.MergeLabelResult{}, nil
}
