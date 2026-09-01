// Transaction glue: the §5a self-healing zero. The transaction feature must
// re-pin a deleted account's balance at zero after a write that moved it, but
// zeroing is the account feature's use case — the composition root bridges the
// two (features never import each other; see internal/test/archtest).
package server

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// accountZeroer is the account-service surface the adapter needs.
type accountZeroer interface {
	ZeroIfDeleted(ctx context.Context, accountID vo.Id, spentAt time.Time, description string) (bool, error)
}

// TransactionAccountZeroer adapts the account service to the transaction
// feature's AccountZeroer port.
type TransactionAccountZeroer struct {
	accounts accountZeroer
}

// NewTransactionAccountZeroer wraps the account service.
func NewTransactionAccountZeroer(accounts accountZeroer) *TransactionAccountZeroer {
	return &TransactionAccountZeroer{accounts: accounts}
}

// ZeroDeleted re-zeroes accountID when it is deleted, dropping the
// "was anything written" answer the caller has no use for. No transaction of
// its own: it runs inside the transaction the caller already opened.
func (z *TransactionAccountZeroer) ZeroDeleted(ctx context.Context, accountID vo.Id, spentAt time.Time, description string) error {
	_, err := z.accounts.ZeroIfDeleted(ctx, accountID, spentAt, description)
	return err
}
