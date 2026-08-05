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
