package category

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MergeCategory absorbs one category into another: every transaction and
// recurring template on the source is re-pointed at the target, the source's
// budget elements and per-period limits are folded into the target's, and then
// the source is deleted. Irreversible, and one transaction end to end.
//
// Ownership is checked on BOTH ids before any write. The two failures are
// reported differently on purpose: a foreign-owned source is masked as
// not-found so ids cannot be probed (matching delete), while a foreign-owned
// target — which the caller can see, since the list endpoints include shared
// entries — is a plain refusal.
//
// Envelope membership is deliberately untouched: it is a structural choice with
// no meaningful sum, so the target keeps its own and the source's rows cascade
// away with it.
func (s *Service) MergeCategory(ctx context.Context, userID vo.Id, req model.MergeCategoryRequest) (*model.MergeCategoryResult, error) {
	sourceID, err := vo.ParseId(req.SourceId)
	if err != nil {
		return nil, err
	}
	targetID, err := vo.ParseId(req.TargetId)
	if err != nil {
		return nil, err
	}

	cannotMerge := &errs.ValidationError{Msg: "Categories cannot be merged", MsgCode: errs.CodeCategoryCannotBeMerged}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		source, gerr := s.repo.GetByID(txCtx, sourceID)
		if gerr != nil {
			return gerr
		}
		if !source.UserID.Equal(userID) {
			return &errs.ValidationError{Msg: "Category not found", MsgCode: errs.CodeCategoryNotFound}
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
		// Income and expense categories live in different halves of the budget;
		// merging across would move spending into the wrong side of every report.
		if target.Type != source.Type {
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

	return &model.MergeCategoryResult{}, nil
}
