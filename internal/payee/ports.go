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

// DebtUsage is backed by the debt feature. Deleting or merging a payee is
// never refused because of a debt: the debt rules naming it are released
// (delete) or moved to the target (merge), whatever access the caller has to
// those debts.
//
// Lock order: a debt write locks its budget row before it touches a payee
// row, so a delete or merge calls Prepare first. Then the delete locks its
// payee row, a merge both rows in id order (Repository.LockRow): a debt write
// adding a rule on one of them commits first, and Release or Reassign sees
// its rule. One window is left: a rule committed between Prepare's read and
// those row locks names a budget Prepare did not lock, which Release or
// Reassign then locks; if another debt write on that budget is waiting for
// the row, PostgreSQL aborts one side with a deadlock (40P01), which is safe
// to retry.
type DebtUsage interface {
	// Prepare locks the budgets of the debts whose rules name any of ids;
	// first in the transaction.
	Prepare(ctx context.Context, ids ...vo.Id) error
	// Release takes id out of every debt rule; called before the row is
	// deleted.
	Release(ctx context.Context, id vo.Id) error
	Reassign(ctx context.Context, oldID, newID vo.Id) error
}
