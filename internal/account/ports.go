// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package account

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CurrencyLookup resolves a currency by id for the account-result embed, and
// gates which currencies a user may denominate a new/changed account in.
type CurrencyLookup interface {
	GetByID(ctx context.Context, id string) (model.CurrencyView, error)
	// EnsureUsable confirms the currency is usable by the user: global, or
	// their own non-archived custom. Returns NotFound (missing) or a
	// field-level ValidationError (foreign/archived) otherwise.
	EnsureUsable(ctx context.Context, userID, currencyID string) error
}

// UserLookup resolves the owner (id, name, avatar) for the account-result embed.
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (model.OwnerView, error)
}

// Connections reports whether two users hold a connection link — the
// precondition for sharing an account (grant-access may only target a connected
// user). Satisfied by the connection feature at wiring time.
type Connections interface {
	AreConnected(ctx context.Context, a, b vo.Id) (bool, error)
}
