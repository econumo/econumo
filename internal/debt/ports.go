// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server.
package debt

import (
	"context"
	"time"

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

// BudgetElements is backed by the budget service. It is installed late and
// the service tolerates its absence.
type BudgetElements interface {
	TransferData(ctx context.Context, fromID vo.Id, fromType int16, toID vo.Id, toType int16, currencyID vo.Id) error
	HasData(ctx context.Context, externalID vo.Id, typ int16) (bool, error)
}
