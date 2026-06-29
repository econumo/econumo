package connection

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// AccountAccessRepository persists the per-account grants and reads the
// symmetric user-connection links. A missing grant returns an
// *errs.NotFoundError.
type AccountAccessRepository interface {
	Get(ctx context.Context, accountID, userID vo.Id) (*AccountAccess, error)

	Save(ctx context.Context, a *AccountAccess) error

	// Delete removes the grant for (accountID, userID). No-op if absent.
	Delete(ctx context.Context, accountID, userID vo.Id) error

	// ListReceived returns grants made TO userID (accounts shared with them).
	ListReceived(ctx context.Context, userID vo.Id) ([]*AccountAccess, error)

	// ListIssued returns grants on accounts OWNED by userID (issued to others).
	ListIssued(ctx context.Context, userID vo.Id) ([]*AccountAccess, error)

	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)

	// ConnectedUserIDs returns the users linked to userID.
	ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)

	// DeleteConnection removes the symmetric link between the two users (both
	// directions).
	DeleteConnection(ctx context.Context, a, b vo.Id) error

	// ConnectUsers creates the symmetric link between the two users (both
	// directions), idempotent if it already exists.
	ConnectUsers(ctx context.Context, a, b vo.Id) error

	// DeleteOption removes a user's per-account ordering row (used when revoking
	// a shared account: the guest's ordering row is dropped).
	DeleteOption(ctx context.Context, accountID, userID vo.Id) error
}

// InviteRepository persists the one-per-user connection invite row. A missing
// row for a user is represented by a nil invite (not an error); a
// missing/expired code on lookup-by-code is a NotFoundError.
type InviteRepository interface {
	// GetByUser returns the user's invite row, or nil if the user has none.
	GetByUser(ctx context.Context, userID vo.Id) (*ConnectionInvite, error)

	// GetByCode returns the (non-expired) invite bearing the code; NotFound if no
	// row has that code or it is expired.
	GetByCode(ctx context.Context, code ConnectionCode, now time.Time) (*ConnectionInvite, error)

	// Save upserts the user's invite row (code + expiry, both nullable).
	Save(ctx context.Context, inv *ConnectionInvite) error
}
