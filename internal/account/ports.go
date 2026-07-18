// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package account

import (
	"context"

	"github.com/econumo/econumo/internal/model"
)

// CurrencyLookup resolves a currency by id for the account-result embed.
type CurrencyLookup interface {
	GetByID(ctx context.Context, id string) (model.CurrencyView, error)
}

// UserLookup resolves the owner (id, name, avatar) for the account-result embed.
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (model.OwnerView, error)
}
