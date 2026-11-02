// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server.
package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Classifications is backed by the category, payee and tag repos.
type Classifications interface {
	// CategoryOf returns errs.NotFound when the owner does not have the category.
	CategoryOf(ctx context.Context, ownerID, id vo.Id) (name, icon string, isExpense bool, err error)
	PayeeOwned(ctx context.Context, ownerID, id vo.Id) (bool, error)
	TagOwned(ctx context.Context, ownerID, id vo.Id) (bool, error)
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

// BudgetElements is backed by the budget service. It is installed late and
// the service tolerates its absence.
type BudgetElements interface {
	// TransferData reports leftBehind when some budget held source data that
	// was not moved (another currency, or a month the target already plans).
	TransferData(ctx context.Context, t model.ElementDataTransfer) (leftBehind bool, err error)
	HasData(ctx context.Context, externalID vo.Id, typ int16) (bool, error)
	// LockRows write-locks the rows of the external id in every budget, so a
	// guard that then reads their data cannot miss a concurrent plan.
	LockRows(ctx context.Context, externalID vo.Id, typ int16) error
}
