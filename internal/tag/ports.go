// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package tag

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccess resolves shared-account ownership/admin-grant for the
// create-for-account path: which user owns an account, and whether a
// connected user holds an admin grant on it. Backed by the connection
// module's AccountAccess repo (the connection/domconnection.Role comparison
// lives on that side, so this port stays free of connection's types).
type AccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// LabelNames exposes the reporting-tag names of one owner. Both classification
// kinds share a single user-facing namespace ("tag"), so a name taken by either
// blocks the other; this feature cannot import the label feature to check.
type LabelNames interface {
	NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error)
}

// BudgetElementMerger folds the source classification's budget presence (its
// element, and that element's per-period limits) into the target's. Implemented
// by internal/budget and wired in internal/server, so this package never imports
// it.
type BudgetElementMerger interface {
	MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error
}

// DebtUsage is backed by the debt feature. Deleting or merging a tag is never
// refused because of a debt: the debt rules naming it are released (delete)
// or moved to the target (merge), whatever access the caller has to those
// debts.
//
// Lock order: a debt write locks its budget row before it touches a tag row or
// a budget element row, so a delete or merge calls Prepare first. Then the
// delete locks its tag row, a merge both rows in id order (Repository.LockRow):
// a debt write adding a rule on one of them commits first, and Release or
// Reassign sees its rule. One window is left: a rule committed between
// Prepare's read and those row locks names a budget Prepare did not lock.
// Release or Reassign then fails with debt.ErrRulesChangedConcurrently (a 5xx,
// safe to retry) rather than lock that budget after the row, which could
// deadlock with a debt write waiting for the row.
type DebtUsage interface {
	// Prepare locks the budgets of the debts whose rules name any of ids;
	// first in the transaction.
	Prepare(ctx context.Context, ids ...vo.Id) error
	// Release takes id out of every debt rule; called before the row is
	// deleted.
	Release(ctx context.Context, id vo.Id) error
	Reassign(ctx context.Context, oldID, newID vo.Id) error
}
