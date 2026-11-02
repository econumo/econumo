// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package payee

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccess resolves shared-account ownership/admin-grant for the
// create-for-account path. A missing grant is reported as false (nil error).
type AccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// DebtUsage is backed by the debt feature: a debt keyed on this payee must
// block its deletion, and follows a merge to the target. Reassign returns the
// coded debt.key_conflict refusal when the merge would leave two debts that
// can match the same payment.
type DebtUsage interface {
	InUse(ctx context.Context, id vo.Id) (bool, error)
	Reassign(ctx context.Context, oldID, newID vo.Id) error
	// PrepareMerge locks the owners of the debts a merge touches. The merge
	// calls it before any write: a debt write takes the owner row before the
	// budget rows, and a merge must take them in the same order.
	PrepareMerge(ctx context.Context, oldID, newID vo.Id) error
}
