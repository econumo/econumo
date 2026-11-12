package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeletePayee deletes the payee. The user must own it; an ownership failure
// surfaces as AccessDenied (HTTP 403). Transactions referencing the payee have
// payee_id set to NULL via the ON DELETE SET NULL FK. Delete is unconditional —
// to keep them, use merge-payee instead. Debt rules naming it stop narrowing
// by it.
func (s *Service) DeletePayee(ctx context.Context, userID vo.Id, req model.DeletePayeeRequest) (*model.DeletePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		p, gerr := s.repo.GetByID(txCtx, id)
		if gerr != nil {
			return gerr
		}
		if !p.UserID.Equal(userID) {
			// Foreign-owned payee: report as not-found (matching the repo above) so
			// the response can't probe which payee ids exist.
			return errs.NewNotFound("Payee not found")
		}
		if err := s.releaseDebtRules(txCtx, id); err != nil {
			return err
		}
		return s.repo.Delete(txCtx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeletePayeeResult{}, nil
}

// releaseDebtRules runs before the delete's first write, in the lock order
// DebtUsage describes: the debts' budget rows, then the payee row (waiting
// out a debt write adding a rule on it), then the rules are released while
// the row still exists, since debts_rules refers to it.
func (s *Service) releaseDebtRules(ctx context.Context, id vo.Id) error {
	if s.debts == nil {
		return nil
	}
	if err := s.debts.Prepare(ctx, id); err != nil {
		return err
	}
	if err := s.repo.LockForDelete(ctx, id); err != nil {
		return err
	}
	return s.debts.Release(ctx, id)
}
