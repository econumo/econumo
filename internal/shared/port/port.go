// Package port holds the cross-cutting seam interfaces every feature consumes:
// the clock, the transaction runner, and the create-idempotency guard. One
// canonical declaration each — features used to re-declare these locally, and
// Go's structural typing means consolidating them changes nothing at runtime.
package port

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Clock supplies the current instant; the infra implementation wraps time.Now,
// tests pin a fixed instant.
type Clock interface{ Now() time.Time }

// TxRunner runs fn inside one database transaction.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// OperationGuard is the row-based idempotency seam for create endpoints that
// take a client-supplied operation id (backed by operation_requests_ids).
type OperationGuard interface {
	// Claim inserts the id. already=true means a row existed (duplicate).
	// Runs inside the caller's tx.
	Claim(ctx context.Context, id vo.Id, now time.Time) (already bool, err error)
	// MarkHandled flips is_handled after the operation succeeds.
	MarkHandled(ctx context.Context, id vo.Id, now time.Time) error
}
