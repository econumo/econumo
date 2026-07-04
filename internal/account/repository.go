package account

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountStore is the account entity's own persistence surface: identity,
// lookup/listing, the write, and the balance-correction insert. Consumed by
// CreateAccount, DeleteAccount, UpdateAccount, GetAccountList/AccountOwner/
// VisibleAccountIDs (read.go), OrderAccountList, and buildAccountList
// (usecase.go). A missing account returns an *errs.NotFoundError.
type AccountStore interface {
	// NextIdentity allocates a fresh id (accounts and corrections share the pool).
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*model.Account, error)

	// ListAvailable returns the user's non-deleted accounts.
	ListAvailable(ctx context.Context, userID vo.Id) ([]*model.Account, error)

	// CountAvailable returns how many non-deleted accounts the user has (seeds a
	// new account's position when no options rows exist).
	CountAvailable(ctx context.Context, userID vo.Id) (int, error)

	Save(ctx context.Context, a *model.Account) error

	// SaveCorrection inserts a balance-correction transaction (type income/expense
	// by sign; amount is the absolute value).
	SaveCorrection(ctx context.Context, c model.AccountCorrection) error
}

// PositionStore is the per-user account ordering surface (the accounts_options
// table). Consumed by CreateAccount and OrderAccountList (writes) and
// buildAccountResult (usecase.go, the read for the embed).
type PositionStore interface {
	// GetPosition returns the account's per-user position. missing -> ok=false.
	GetPosition(ctx context.Context, accountID, userID vo.Id) (position int16, ok bool, err error)

	// MaxPosition returns the highest per-user position for the user (0 if none).
	MaxPosition(ctx context.Context, userID vo.Id) (int16, error)

	// SavePosition upserts a per-user position row.
	SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error
}

// BalanceReader computes account balances from the transactions table.
// Consumed by CreateAccount/UpdateAccount (the single-account balance) and
// buildAccountList (usecase.go, the bulk balances for a list).
type BalanceReader interface {
	// Balance returns one account's balance as of `before` (exclusive on
	// spent_at), normalized to the wire decimal form.
	Balance(ctx context.Context, accountID vo.Id, before time.Time) (string, error)

	// Balances returns balances for all the user's non-deleted accounts as of
	// `before`, keyed by account id (normalized decimal strings).
	Balances(ctx context.Context, userID vo.Id, before time.Time) (map[string]string, error)
}

// Repository is the account aggregate's full persistence port — the composite
// of AccountStore, PositionStore and BalanceReader. It exists for wiring (one
// constructor param in server.go); consumers depend on the narrowest role they
// actually use.
type Repository interface {
	AccountStore
	PositionStore
	BalanceReader
}

// FolderStore is the folder entity's own persistence surface: identity,
// lookup/listing, write and delete. Consumed by CreateAccount's
// resolveAccountFolder/defaultFolderOr, CreateFolder/createFolderTx,
// UpdateFolder, toggleVisibility (Hide/ShowFolder), ReplaceFolder,
// resetFolderPositions, OrderFolderList (folder_usecase.go), GetFolderList/
// VisibleAccountIDs (read.go), OrderAccountList (order.go), and sortedFolders/
// buildAccountList (usecase.go).
type FolderStore interface {
	// NextIdentity allocates a fresh folder id.
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*model.Folder, error)

	// ListByUser returns the user's folders (unordered; the caller sorts by
	// position).
	ListByUser(ctx context.Context, userID vo.Id) ([]*model.Folder, error)

	// CountByUser returns how many folders the user has.
	CountByUser(ctx context.Context, userID vo.Id) (int, error)

	Save(ctx context.Context, f *model.Folder) error

	// Delete removes a folder row (its membership rows cascade).
	Delete(ctx context.Context, id vo.Id) error
}

// FolderMembership is the folder/account join-table surface. Consumed by
// CreateAccount (place the new account), OrderAccountList (move an account
// between folders), ReplaceFolder (move accounts off a deleted folder),
// VisibleAccountIDs (read.go), and buildAccountList (usecase.go, the
// folderId embed).
type FolderMembership interface {
	// MembershipsByUser returns folderID -> []accountID for the user's folders.
	MembershipsByUser(ctx context.Context, userID vo.Id) (map[string][]string, error)

	// FolderAccountIDs returns the account ids in a folder.
	FolderAccountIDs(ctx context.Context, folderID vo.Id) ([]string, error)

	// AddAccount adds an account to a folder (idempotent).
	AddAccount(ctx context.Context, folderID, accountID vo.Id) error

	// RemoveAccount removes an account from a folder.
	RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error
}

// FolderRepository is the folder aggregate's full persistence port — the
// composite of FolderStore and FolderMembership. It exists for wiring (one
// constructor param in server.go); consumers depend on the narrowest role they
// actually use.
type FolderRepository interface {
	FolderStore
	FolderMembership
}
