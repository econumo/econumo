package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MergePayee absorbs one payee into another: every transaction and recurring
// template on the source is re-pointed at the target, then the source is
// deleted. Irreversible, and one transaction end to end so a failure anywhere
// leaves the data untouched.
//
// Ownership is checked on BOTH ids before any write, and the two failures are
// reported differently on purpose: a foreign-owned source is not-found, so ids
// cannot be probed (matching delete), while a foreign-owned target — which the
// caller can see in their own list, since the list endpoints include shared
// entries — is a plain refusal.
//
// The reassignment is keyed on the payee id alone, so a shared account's
// transactions written by a connected user follow the merge too. They carry the
// account owner's payee, and leaving them behind would strand them on a deleted
// row.
func (s *Service) MergePayee(ctx context.Context, userID vo.Id, req model.MergePayeeRequest) (*model.MergePayeeResult, error) {
	sourceID, err := vo.ParseId(req.SourceId)
	if err != nil {
		return nil, err
	}
	targetID, err := vo.ParseId(req.TargetId)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		source, gerr := s.repo.GetByID(txCtx, sourceID)
		if gerr != nil {
			return gerr
		}
		if !source.UserID.Equal(userID) {
			return errs.NewNotFound("Payee not found")
		}
		if sourceID.Equal(targetID) {
			return &errs.ValidationError{Msg: "Payees cannot be merged", MsgCode: errs.CodePayeeCannotBeMerged}
		}
		target, terr := s.repo.GetByID(txCtx, targetID)
		if terr != nil {
			return terr
		}
		if !target.UserID.Equal(userID) {
			return &errs.ValidationError{Msg: "Payees cannot be merged", MsgCode: errs.CodePayeeCannotBeMerged}
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

	return &model.MergePayeeResult{}, nil
}
