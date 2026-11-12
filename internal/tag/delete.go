package tag

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteTag deletes the tag. The user must own it; an ownership failure surfaces
// as AccessDenied (HTTP 403). Transactions referencing the tag have tag_id set
// to NULL via the ON DELETE SET NULL FK. Delete is unconditional; to keep them,
// use merge-tag instead. Debt rules naming it stop narrowing by it.
func (s *Service) DeleteTag(ctx context.Context, userID vo.Id, req model.DeleteTagRequest) (*model.DeleteTagResult, error) {
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
			// Foreign-owned tag: report as not-found (matching the repo above) so
			// the response can't probe which tag ids exist.
			return errs.NewNotFound("Tag not found")
		}
		if err := s.releaseDebtRules(ctx, id); err != nil {
			return err
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeleteTagResult{}, nil
}

// releaseDebtRules runs before the delete's first write, in the lock order
// DebtUsage describes: the debts' budget rows, then the tag row (waiting
// out a debt write adding a rule on it), then the rules are released while
// the row still exists, since debts_rules refers to it.
func (s *Service) releaseDebtRules(ctx context.Context, id vo.Id) error {
	if s.debts == nil {
		return nil
	}
	if err := s.debts.Prepare(ctx, id); err != nil {
		return err
	}
	if err := s.repo.LockRow(ctx, id); err != nil {
		return err
	}
	return s.debts.Release(ctx, id)
}
