// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server.
package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Classifications is backed by the category, payee, tag and label repos.
type Classifications interface {
	// Classification returns errs.NotFound when there is no such id; kind is
	// KindCategory, KindPayee, KindTag or KindLabel.
	Classification(ctx context.Context, kind string, id vo.Id) (model.DebtClassification, error)
}

type Rates interface {
	ConvertOnDay(ctx context.Context, from, to vo.Id, amount vo.DecimalNumber, day time.Time) (vo.DecimalNumber, error)
	FractionDigits(ctx context.Context, currencyID vo.Id) (int, error)
}

// Currencies is backed by the currency lookup.
type Currencies interface {
	// EnsureUsable refuses a currency the user may not denominate a new
	// entity in: deleted, or another user's custom one.
	EnsureUsable(ctx context.Context, userID, currencyID string) error
}

// Users is backed by the user repo.
type Users interface {
	// GetTimezone is the user's stored IANA timezone, "" when none is stored.
	GetTimezone(ctx context.Context, id vo.Id) (string, error)
	GetName(ctx context.Context, id vo.Id) (string, error)
}

// Budgets is backed by the budget service. It is installed late; every use
// case needs it.
type Budgets interface {
	// Access returns errs.NotFound unless userID has accepted access to the
	// budget.
	Access(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error)
	// Readable lists the budgets userID has accepted access to, in the order
	// of the budget list.
	Readable(ctx context.Context, userID vo.Id) ([]model.DebtBudget, error)
	// Scope reads the budget's member accounts, with no access check.
	Scope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error)
	RuleOptions(ctx context.Context, viewerID, budgetID vo.Id) (*model.DebtRuleOptionsResult, error)
	// DebtPlans returns the budget's plans of the debt's row; ok is false
	// when the budget holds no row for the debt.
	DebtPlans(ctx context.Context, budgetID, debtID vo.Id) (plans model.DebtBudgetPlans, ok bool, err error)
}

// BudgetElements is backed by the budget service. It is installed late and
// the service tolerates its absence. A debt's rows live in its own budget
// only.
type BudgetElements interface {
	// TransferData reports leftBehind when source data was not moved
	// (another currency, or a month the target already plans).
	TransferData(ctx context.Context, t model.ElementDataTransfer) (leftBehind bool, err error)
	HasData(ctx context.Context, externalID vo.Id, typ model.ElementType) (bool, error)
	// LockRows write-locks the rows of the external id, so a guard that then
	// reads their data cannot miss a concurrent plan.
	LockRows(ctx context.Context, externalID vo.Id, typ model.ElementType) error
	// RemoveRows deletes the rows of the external id, with their plans and
	// comments.
	RemoveRows(ctx context.Context, externalID vo.Id, typ model.ElementType) error
	// SetRowsCurrency moves the rows of the external id to currencyID,
	// keeping their plans' numbers.
	SetRowsCurrency(ctx context.Context, externalID vo.Id, typ model.ElementType, currencyID vo.Id) error
	// EnsureRows brings the budget's rows up to date with what it shows, a
	// debt saved in the caller's transaction included.
	EnsureRows(ctx context.Context, budgetID vo.Id) error
}
