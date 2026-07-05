// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package tag

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccess resolves shared-account ownership/admin-grant for the
// create-for-account path: which user owns an account, and whether a
// connected user holds an admin grant on it. Backed by the connection
// module's AccountAccess repo (the connection/domconnection.Role comparison
// lives on that side, so this port stays free of connection's types).
type AccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}
