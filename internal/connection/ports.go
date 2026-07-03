// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package connection

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// BudgetAccessRevoker is the slice of budget-sharing behavior delete-connection
// needs: drop any budget access SHARED BETWEEN the two users (both directions).
// Backed by the budget module via an adapter in main. A nil revoker is tolerated
// (delete-connection then only unwinds account access + the connection link) so
// test harnesses without the budget module still work.
type BudgetAccessRevoker interface {
	// RevokeBetween removes any budget-access grants where one user owns a budget
	// the other can access, in BOTH directions, for the given user pair.
	RevokeBetween(ctx context.Context, a, b vo.Id) error
}

// UserLookup resolves the connected-user embed (id, name, avatar).
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (OwnerView, error)
}

// FolderPort is the slice of folder behavior the connection side effects need:
// finding the guest's last folder (by position) and mutating membership. Backed
// by the account module's FolderRepository via an adapter.
type FolderPort interface {
	// LastFolderID returns the user's last folder (highest position). ok=false if
	// the user has no folders.
	LastFolderID(ctx context.Context, userID vo.Id) (folderID vo.Id, ok bool, err error)
	// FoldersContaining returns the user's folder ids that contain the account.
	FoldersContaining(ctx context.Context, userID, accountID vo.Id) ([]vo.Id, error)
	// AddAccount adds the account to the folder (idempotent).
	AddAccount(ctx context.Context, folderID, accountID vo.Id) error
	// RemoveAccount removes the account from the folder.
	RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error
}

// OptionPort is the slice of accounts_options behavior the side effects need.
type OptionPort interface {
	// MaxPosition returns the user's highest accounts_options.position (0 if none).
	MaxPosition(ctx context.Context, userID vo.Id) (int16, error)
	// SavePosition upserts the user's accounts_options row.
	SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error
}
