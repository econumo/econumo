package tag

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MergeTag absorbs one tag into another: every transaction and recurring
// template on the source is re-pointed at the target, the source's budget
// elements and per-period limits are folded into the target's, and then the
// source is deleted. Irreversible, and one transaction end to end.
//
// Ownership is checked on BOTH ids before any write. A foreign-owned source is
// masked as not-found so ids cannot be probed (matching delete); a foreign-owned
// target — visible to the caller, since the list endpoints include shared
// entries — is a plain refusal.
//
// Unlike categories there is no type to reconcile: tags have none.
func (s *Service) MergeTag(ctx context.Context, userID vo.Id, req model.MergeTagRequest) (*model.MergeTagResult, error) {
	sourceID, err := vo.ParseId(req.SourceId)
	if err != nil {
		return nil, err
	}
	targetID, err := vo.ParseId(req.TargetId)
	if err != nil {
		return nil, err
	}

	cannotMerge := &errs.ValidationError{Msg: "Tags cannot be merged", MsgCode: errs.CodeTagCannotBeMerged}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		source, gerr := s.repo.GetByID(txCtx, sourceID)
		if gerr != nil {
			return gerr
		}
		if !source.UserID.Equal(userID) {
			return errs.NewNotFound("Tag not found")
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
		if berr := s.budget.MergeElements(txCtx, sourceID, targetID); berr != nil {
			return berr
		}
		return s.repo.Delete(txCtx, sourceID)
	}); err != nil {
		return nil, err
	}

	return &model.MergeTagResult{}, nil
}
