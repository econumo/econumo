package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// LinkTransactionToRecurring stamps recurringID onto an existing transaction —
// the "make recurring from this transaction" flow, where the source transaction
// becomes the series' first instance. The row being linked must be on an
// account the caller may write to; a valid foreign UUID must not let a caller
// tag a stranger's transaction.
func (s *Service) LinkTransactionToRecurring(ctx context.Context, userID, transactionID, recurringID vo.Id) error {
	t, err := s.repo.GetByID(ctx, transactionID)
	if err != nil {
		return err
	}
	if aerr := s.checkWriteAccess(ctx, userID, t.AccountID, "transaction.transaction.not_available"); aerr != nil {
		return aerr
	}
	return s.repo.LinkRecurring(ctx, transactionID, recurringID, s.clock.Now())
}

// ReplaceLabels rewrites transactionID's label set. Satisfies
// recurring.TransactionCreator's ReplaceLabels port: PostRecurringTransaction
// calls it right after creating a transaction from a template, to copy the
// template's already-validated label ids over — no fresh ownership check here,
// since a template's labels were validated when they were attached to it.
func (s *Service) ReplaceLabels(ctx context.Context, transactionID vo.Id, labelIDs []vo.Id) error {
	return s.repo.ReplaceLabels(ctx, transactionID, labelIDs)
}
