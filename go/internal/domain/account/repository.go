package account

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Repository is the account aggregate's persistence port (accounts +
// accounts_options + the balance read). The app service depends only on this
// interface. A missing account returns an *errs.NotFoundError.
type Repository interface {
	// NextIdentity allocates a fresh account id.
	NextIdentity() vo.Id

	// GetByID loads an account by id. Missing -> *errs.NotFoundError.
	GetByID(ctx context.Context, id vo.Id) (*Account, error)

	// ListAvailable returns the user's non-deleted accounts.
	ListAvailable(ctx context.Context, userID vo.Id) ([]*Account, error)

	// CountAvailable returns how many non-deleted accounts the user has (seeds a
	// new account's position when no options rows exist).
	CountAvailable(ctx context.Context, userID vo.Id) (int, error)

	// Save upserts an account row. Runs inside WithTx.
	Save(ctx context.Context, a *Account) error

	// --- per-user position (accounts_options) ---

	// GetPosition returns the account's position for the given user (the
	// accounts_options row). missing -> ok=false.
	GetPosition(ctx context.Context, accountID, userID vo.Id) (position int16, ok bool, err error)

	// MaxPosition returns the highest accounts_options.position for the user (0
	// if none).
	MaxPosition(ctx context.Context, userID vo.Id) (int16, error)

	// SavePosition upserts an accounts_options row.
	SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error

	// --- balance (computed from transactions) ---

	// Balance returns one account's balance as of `before` (exclusive on
	// spent_at), normalized to the wire decimal form.
	Balance(ctx context.Context, accountID vo.Id, before time.Time) (string, error)

	// Balances returns balances for all the user's non-deleted accounts as of
	// `before`, keyed by account id (normalized decimal strings).
	Balances(ctx context.Context, userID vo.Id, before time.Time) (map[string]string, error)

	// --- balance-correction transaction ---

	// SaveCorrection inserts a balance-correction transaction (type income/expense
	// by sign; amount is the absolute value). Runs inside WithTx.
	SaveCorrection(ctx context.Context, c Correction) error
}

// Correction is a balance-correction transaction to insert (account create with
// non-zero balance, or update changing the balance).
type Correction struct {
	ID          vo.Id
	UserID      vo.Id
	AccountID   vo.Id
	Description string
	Type        int16 // 0 expense, 1 income
	Amount      string
	SpentAt     time.Time
	CreatedAt   time.Time
}

// FolderRepository is the folder aggregate's persistence port (folders +
// accounts_folders membership).
type FolderRepository interface {
	// NextIdentity allocates a fresh folder id.
	NextIdentity() vo.Id

	// GetByID loads a folder by id. Missing -> *errs.NotFoundError.
	GetByID(ctx context.Context, id vo.Id) (*Folder, error)

	// ListByUser returns the user's folders (unordered; the caller sorts by
	// position).
	ListByUser(ctx context.Context, userID vo.Id) ([]*Folder, error)

	// CountByUser returns how many folders the user has.
	CountByUser(ctx context.Context, userID vo.Id) (int, error)

	// Save upserts a folder row.
	Save(ctx context.Context, f *Folder) error

	// Delete removes a folder row (its accounts_folders rows cascade).
	Delete(ctx context.Context, id vo.Id) error

	// MembershipsByUser returns folderID -> []accountID for the user's folders.
	MembershipsByUser(ctx context.Context, userID vo.Id) (map[string][]string, error)

	// FolderAccountIDs returns the account ids in a folder.
	FolderAccountIDs(ctx context.Context, folderID vo.Id) ([]string, error)

	// AddAccount adds an account to a folder (idempotent).
	AddAccount(ctx context.Context, folderID, accountID vo.Id) error

	// RemoveAccount removes an account from a folder.
	RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error
}
