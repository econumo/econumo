// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server.
package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Categories is backed by the category repo: it names a category element no
// participant of the budget shows any more (its owner left the budget).
type Categories interface {
	// Category returns errs.NotFound when there is no such category.
	Category(ctx context.Context, id vo.Id) (model.DebtClassification, error)
}

type Rates interface {
	ConvertOnDay(ctx context.Context, from, to vo.Id, amount vo.DecimalNumber, day time.Time) (vo.DecimalNumber, error)
	FractionDigits(ctx context.Context, currencyID vo.Id) (int, error)
}

// Users is backed by the user repo.
type Users interface {
	// GetTimezone is the user's stored IANA timezone, "" when none is stored.
	GetTimezone(ctx context.Context, id vo.Id) (string, error)
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
	// Scope reads the budget's everyday member accounts, with no access check.
	Scope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error)
}

// Elements is backed by the budget service: the elements debts are attached
// to. It reads only, so a budget read can call the debt service inside its
// own transaction. It is installed late; every use case needs it.
type Elements interface {
	// Element describes the budget's element of that external id; an id the
	// budget shows no category or envelope for has a zero Type and is not
	// eligible.
	Element(ctx context.Context, budgetID, elementID vo.Id) (model.DebtElementInfo, error)
	// Plans are the element row's plans, period key "YYYY-MM-01" -> amount in
	// the row's currency; none when the budget has no row for it.
	Plans(ctx context.Context, budgetID, elementID vo.Id) (map[string]float64, error)
}
