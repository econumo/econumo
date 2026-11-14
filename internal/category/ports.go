// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package category

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccess resolves shared-account ownership/admin-grant for the
// create-for-account path: which user owns an account, and whether a connected
// user holds an admin grant on it. Backed by the connection module's
// AccountAccess repo (the connection/domconnection.Role comparison lives on
// that side, so this port stays free of connection's types).
type AccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// BudgetElementMerger folds the source classification's budget presence (its
// element, and that element's per-period limits) into the target's. Implemented
// by internal/budget and wired in internal/server, so this package never imports
// it.
type BudgetElementMerger interface {
	MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error
}

// DebtUsage is backed by the debt feature. Deleting or merging a category is
// never refused because of a debt: the debts attached to it are deleted
// (delete) or moved to the target when it can take them (merge), whatever
// access the caller has to those debts.
//
// Lock order: a debt write locks its budget row before it touches a category
// row or a budget element row, so a delete or merge calls Prepare first. Then
// the delete locks its category row, a merge both rows in id order
// (Repository.LockRow): a debt create on one of them commits first, and
// Release or Reassign sees its debt. One window is left: a debt committed
// between Prepare's read and those row locks names a budget Prepare did not
// lock. Release or Reassign then fails with debt.ErrDebtsChangedConcurrently
// (a 5xx, safe to retry) rather than lock that budget after the row, which
// could deadlock with a debt write waiting for the row.
type DebtUsage interface {
	// Prepare locks the budgets of the debts attached to any of ids; first
	// in the transaction.
	Prepare(ctx context.Context, ids ...vo.Id) error
	// Release deletes the debts attached to id; called before the row is
	// deleted.
	Release(ctx context.Context, id vo.Id) error
	// Reassign moves the debts of oldID to newID, or deletes them; called
	// after the budget rows are merged.
	Reassign(ctx context.Context, oldID, newID vo.Id) error
}
