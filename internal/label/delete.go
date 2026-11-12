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
// future postings of those templates no longer carry it; to keep them, use
// merge-label instead. Debt rules naming the label stop narrowing by it.
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
		if err := s.releaseDebtRules(ctx, id); err != nil {
			return err
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeleteLabelResult{}, nil
}

// releaseDebtRules runs before the delete's first write, in the lock order
// DebtUsage describes: the debts' budget rows, then the label row (waiting
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
