package connection

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccessRepository persists the per-account grants and reads the
// symmetric user-connection links. A missing grant returns an
// *errs.NotFoundError.
type AccountAccessRepository interface {
	Get(ctx context.Context, accountID, userID vo.Id) (*model.AccountAccess, error)

	Save(ctx context.Context, a *model.AccountAccess) error

	// Delete removes the grant for (accountID, userID). No-op if absent.
	Delete(ctx context.Context, accountID, userID vo.Id) error

	// ListReceived returns grants made TO userID (accounts shared with them).
	ListReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)

	// ListIssued returns grants on accounts OWNED by userID (issued to others).
	ListIssued(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error)

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
	GetByUser(ctx context.Context, userID vo.Id) (*model.ConnectionInvite, error)

	// GetByCode returns the (non-expired) invite bearing the code; NotFound if no
	// row has that code or it is expired.
	GetByCode(ctx context.Context, code model.ConnectionCode, now time.Time) (*model.ConnectionInvite, error)

	// Save upserts the user's invite row (code + expiry, both nullable).
	Save(ctx context.Context, inv *model.ConnectionInvite) error
}
