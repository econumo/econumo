package connection

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// AccountAccessRepository persists the accounts_access grants and reads the
// symmetric users_connections links. A missing grant returns an
// *errs.NotFoundError.
type AccountAccessRepository interface {
	// Get loads the grant for (accountID, userID); NotFound if absent.
	Get(ctx context.Context, accountID, userID vo.Id) (*AccountAccess, error)

	// Save upserts a grant.
	Save(ctx context.Context, a *AccountAccess) error

	// Delete removes the grant for (accountID, userID). No-op if absent.
	Delete(ctx context.Context, accountID, userID vo.Id) error

	// ListReceived returns grants made TO userID (accounts shared with them).
	ListReceived(ctx context.Context, userID vo.Id) ([]*AccountAccess, error)

	// ListIssued returns grants on accounts OWNED by userID (issued to others).
	ListIssued(ctx context.Context, userID vo.Id) ([]*AccountAccess, error)

	// AccountOwner returns the owner user id of an account.
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)

	// ConnectedUserIDs returns the users linked to userID via users_connections.
	ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)

	// DeleteConnection removes the symmetric users_connections link between the
	// two users (both directions).
	DeleteConnection(ctx context.Context, a, b vo.Id) error

	// DeleteOption removes a user's accounts_options row for an account (used when
	// revoking a shared account: the guest's per-user ordering row is dropped).
	DeleteOption(ctx context.Context, accountID, userID vo.Id) error
}
