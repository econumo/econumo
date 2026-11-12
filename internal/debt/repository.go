// Package debt holds the budget-debt use cases (loans and credit lines a
// budget pays down through one or more categories).
package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CandidateTx is an expense that may pay a debt, in its account's currency.
type CandidateTx struct {
	ID         vo.Id
	SpentAt    time.Time
	Amount     vo.DecimalNumber
	CurrencyID vo.Id
	CategoryID vo.Id
	PayeeID    *vo.Id
	TagID      *vo.Id
	LabelIDs   []vo.Id
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
	// GetByID and the lists load the debts' rules, ordered by created_at, id.
	GetByID(ctx context.Context, id vo.Id) (*model.Debt, error)
	// ListByBudget is ordered by created_at, id; ListByBudgets too.
	ListByBudget(ctx context.Context, budgetID vo.Id) ([]*model.Debt, error)
	ListByBudgets(ctx context.Context, budgetIDs []vo.Id) ([]*model.Debt, error)
	// ListByClassification returns the debts with a rule naming id; kind is
	// "category", "payee", "tag" or "label".
	ListByClassification(ctx context.Context, kind string, id vo.Id) ([]*model.Debt, error)
	// Save upserts the debt and replaces its rules with d.Rules.
	Save(ctx context.Context, d *model.Debt) error
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
	Splits(ctx context.Context, debtIDs []vo.Id) ([]model.DebtSplit, error)
	// InsertSplit upserts on (debt, transaction).
	InsertSplit(ctx context.Context, s model.DebtSplit) error
	DeleteSplits(ctx context.Context, debtID vo.Id, transactionIDs []vo.Id) error
	// CandidateTransactions returns the expenses (type 0) in the categories
	// on the accounts.
	CandidateTransactions(ctx context.Context, categoryIDs, accountIDs []vo.Id) ([]CandidateTx, error)
	// LockRuleCategories key-share-locks the categories in id order
	// (PostgreSQL; SQLite only reads them): a concurrent category delete
	// waits for the rule write and then releases it. A category already
	// deleted returns errs.NotFound "Category not found".
	LockRuleCategories(ctx context.Context, categoryIDs []vo.Id) error
}
