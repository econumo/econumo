// Package debt holds the budget-debt use cases: loans and credit lines a
// budget pays down through one of its elements, a top-level expense category
// or an envelope.
package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CandidateTx is an untagged expense that may pay a debt, in its account's
// currency.
type CandidateTx struct {
	ID         vo.Id
	SpentAt    time.Time
	Amount     vo.DecimalNumber
	CurrencyID vo.Id
	CategoryID vo.Id
}

type Repository interface {
	NextIdentity() vo.Id
	// LockBudget takes the budget row lock for the rest of the transaction
	// (PostgreSQL, FOR NO KEY UPDATE; a missing budget succeeds); a no-op on
	// SQLite.
	LockBudget(ctx context.Context, budgetID vo.Id) error
	// HoldsBudgetLock reports whether this transaction took LockBudget on the
	// budget (also on SQLite, where the lock itself is a no-op).
	HoldsBudgetLock(ctx context.Context, budgetID vo.Id) bool
	GetByID(ctx context.Context, id vo.Id) (*model.Debt, error)
	// ListByBudget is ordered by created_at, id; ListByBudgets and
	// ListByElement too.
	ListByBudget(ctx context.Context, budgetID vo.Id) ([]*model.Debt, error)
	ListByBudgets(ctx context.Context, budgetIDs []vo.Id) ([]*model.Debt, error)
	// ListByElement lists the debts attached to the element in every budget.
	ListByElement(ctx context.Context, elementID vo.Id) ([]*model.Debt, error)
	Save(ctx context.Context, d *model.Debt) error
	// MoveToElement re-attaches the budget's debts of one element to another.
	MoveToElement(ctx context.Context, budgetID, fromID, toID vo.Id, typ model.DebtElementType, now time.Time) error
	Delete(ctx context.Context, id vo.Id) error
	// AddSnapshot is always paired with a Save of the same terms in one transaction.
	AddSnapshot(ctx context.Context, s *model.DebtSnapshot) error
	// UpdateSnapshot rewrites a snapshot's as-of, balance and terms: only the
	// origin's facts and, with them, a later snapshot's terms are corrected.
	UpdateSnapshot(ctx context.Context, s *model.DebtSnapshot) error
	// DeleteSnapshot removes a statement balance (never the origin).
	DeleteSnapshot(ctx context.Context, id vo.Id) error
	// Snapshots are ordered by debt, as_of, created_at, id. A debt's first is
	// its origin: the service tells it by position, with no flag stored. No
	// statement may precede the start (validateStatement, laterConflict), and
	// on a shared date the origin, written first, sorts first by created_at
	// (then by its UUIDv7 id, minted earlier).
	Snapshots(ctx context.Context, debtIDs []vo.Id) ([]*model.DebtSnapshot, error)
	// RateChanges are ordered by debt, effective_date.
	RateChanges(ctx context.Context, debtIDs []vo.Id) ([]*model.DebtRateChange, error)
	// SaveRateChange replaces the debt's change on the same effective date.
	SaveRateChange(ctx context.Context, c *model.DebtRateChange) error
	// DeleteRateChange reports whether the debt had such a change.
	DeleteRateChange(ctx context.Context, debtID, id vo.Id) (bool, error)
	Splits(ctx context.Context, debtIDs []vo.Id) ([]model.DebtSplit, error)
	// InsertSplit upserts on (debt, transaction).
	InsertSplit(ctx context.Context, s model.DebtSplit) error
	DeleteSplits(ctx context.Context, debtID vo.Id, transactionIDs []vo.Id) error
	// CandidateTransactions returns the untagged expenses (type 0) in the
	// categories on the accounts.
	CandidateTransactions(ctx context.Context, categoryIDs, accountIDs []vo.Id) ([]CandidateTx, error)
	// LockCategories key-share-locks the categories in id order (PostgreSQL;
	// SQLite only reads them): a concurrent category delete waits for the
	// debt write and then finds its debt. A category already deleted returns
	// errs.NotFound "Category not found".
	LockCategories(ctx context.Context, categoryIDs []vo.Id) error
}
