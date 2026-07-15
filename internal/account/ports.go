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

// SharedAccessLookup lists the accounts_access grants on an account (for the
// account result's sharedAccess[] embed). Satisfied by an adapter over the
// connection repo. A nil lookup means "no connection module" -> empty slice.
type SharedAccessLookup interface {
	ListByAccount(ctx context.Context, accountID vo.Id) ([]model.SharedAccessView, error)
}

// AccessRevoker drops the caller's own grant on a shared account (the
// delete-account non-owner branch). HasAccess reports whether the user owns or
// has any grant on the account (the gate for deleting it). Satisfied by an
// adapter over the connection service. May be nil (no connection module) ->
// non-owner delete falls back to AccessDenied.
type AccessRevoker interface {
	HasAccess(ctx context.Context, userID, accountID vo.Id) (bool, error)
	RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error
}
