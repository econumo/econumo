// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// often directly, but sometimes (ExportLookup, Importer) via an adapter in
// this feature's own repo package composing smaller cross-feature sub-ports.
// Features never import each other (enforced by internal/test/archtest).
package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserLookup resolves the author (id, name, avatar).
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (model.OwnerView, error)
}

// AccountResolver answers ownership/existence questions about an account and
// supplies the account-list embed. The account module's service satisfies the
// list method; ownership is answered by a small repo lookup (AccountOwner).
type AccountResolver interface {
	// AccountOwner returns the owner user id of an account (for the access
	// check). Missing -> *errs.NotFoundError.
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	// AccountListForUser returns the user's available accounts in the wire shape
	// (reverse order), for the create/update/delete result embed.
	AccountListForUser(ctx context.Context, userID vo.Id) ([]model.AccountResult, error)
}

// VisibleAccounts supplies the set of account ids whose transactions a user may
// list (own + shared, minus deleted + hidden-folder). The account module
// provides this.
type VisibleAccounts interface {
	VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

// AccountGrants reports whether a connected (non-owner) user holds an
// admin-or-user shared-access grant on an account, for the write-access
// check. Backed by the connection module's AccountAccess repository.
type AccountGrants interface {
	HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// ExportLookup supplies the read-side data the export needs without coupling the
// transaction service to the account/metadata repo packages: the user's
// accessible accounts (own + shared, not deleted) and name resolution for the
// optional category/tag/payee of each transaction. Name lookups return "" when
// the entity is missing.
type ExportLookup interface {
	ExportAccounts(ctx context.Context, userID vo.Id) ([]model.ExportAccount, error)
	CategoryName(ctx context.Context, id vo.Id) (string, error)
	TagName(ctx context.Context, id vo.Id) (string, error)
	PayeeName(ctx context.Context, id vo.Id) (string, error)
}

// Importer is the read/write port the import orchestration drives. It abstracts
// the account/metadata repos + create services so app/transaction stays
// decoupled from those packages. All methods run within the service's
// import-wide transaction.
//
// Accepted: its 11-method breadth fronts the import pipeline's find-or-create
// surface (accounts, categories, payees, tags, plus the access check and the
// final save) by design — narrower ports would just push the same surface
// back into the service as several fields (decision recorded at Phase 6
// planning).
type Importer interface {
	// AvailableAccounts returns the user's available (own, not deleted) accounts.
	AvailableAccounts(ctx context.Context, userID vo.Id) ([]model.ImportAccount, error)
	// AccountByID returns an available account by id (nil if not found).
	AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*model.ImportAccount, error)
	// CanAddTransaction reports whether the user may add a transaction to the
	// account: they own it, or hold an admin/user grant on it.
	CanAddTransaction(ctx context.Context, userID vo.Id, accountID vo.Id) (bool, error)
	// CreateAccount creates a new account (base currency, first/new folder, icon
	// 'wallet', balance 0) and returns its view.
	CreateAccount(ctx context.Context, userID vo.Id, name string) (model.ImportAccount, error)

	// CategoriesByOwner / PayeesByOwner / TagsByOwner return the owner's entities.
	CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)
	PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)
	TagsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error)
	// CreateCategory creates a category (income type when income==true, else
	// expense; icon 'category'). CreatePayee/CreateTag create by name.
	CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (model.ImportNamed, error)
	CreatePayee(ctx context.Context, ownerID vo.Id, name string) (model.ImportNamed, error)
	CreateTag(ctx context.Context, ownerID vo.Id, name string) (model.ImportNamed, error)

	// SaveTransaction persists a built transaction (no idempotency id).
	SaveTransaction(ctx context.Context, t *model.Transaction) error
}
