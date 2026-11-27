package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the transaction aggregate's persistence port. The app service
// depends only on this interface. A missing transaction returns
// *errs.NotFoundError.
type Repository interface {
	// NextIdentity allocates a fresh transaction id.
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error)

	Save(ctx context.Context, t *model.Transaction) error

	// LinkRecurring stamps recurring_id on an existing transaction. A dedicated
	// write because Save deliberately never updates the column (provenance must
	// survive ordinary edits).
	LinkRecurring(ctx context.Context, id, recurringID vo.Id, now time.Time) error

	Delete(ctx context.Context, id vo.Id) error

	// ListByAccount returns transactions where the account is source or recipient,
	// newest first (spent_at DESC).
	ListByAccount(ctx context.Context, accountID vo.Id) ([]*model.Transaction, error)

	// ListByAccountIDs returns transactions whose source OR recipient account is in
	// accountIDs, narrowed by filter — filter.PeriodStart/PeriodEnd bound the
	// window (both zero = no period) and the classification fields AND-compose.
	// The zero model.TransactionFilter applies no predicate beyond the accounts.
	// Used for the user-wide visible-accounts list and any filtered single-account
	// query.
	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, filter model.TransactionFilter) ([]*model.Transaction, error)

	// TransactionsRevisions reads accounts.transactions_revision for the given
	// accounts, keyed by account id. Accounts that do not exist are absent.
	TransactionsRevisions(ctx context.Context, accountIDs []vo.Id) (map[string]int64, error)

	// DayKeys returns, for every transaction on accountID during spentAt's day,
	// its key on that account's side, keyed by transaction id.
	DayKeys(ctx context.Context, accountID vo.Id, spentAt time.Time) (map[string]string, error)

	// SetSideKey stores key as id's position on accountID's side.
	SetSideKey(ctx context.Context, accountID, id vo.Id, key string) error

	// ReplaceLabels rewrites a transaction's label links; the caller runs this
	// inside the same tx as Save so the classification and the row commit or
	// roll back together.
	ReplaceLabels(ctx context.Context, transactionID vo.Id, labelIDs []vo.Id) error

	// LabelsByTransactionIDs batch-loads label ids for many transactions, keyed
	// by transaction id (string), for attaching to already-hydrated results.
	LabelsByTransactionIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error)

	// ImportedTransactionIDs reports which of the given transactions carry at
	// least one live import link (the list endpoint's isImported flag).
	ImportedTransactionIDs(ctx context.Context, ids []vo.Id) (map[string]bool, error)

	// BumpAccountsLosingReferences queues a revision bump for every account
	// whose rows the deletion cascade of userID will change (the user's
	// categories, payees, tags, labels, import sources and recurring templates).
	BumpAccountsLosingReferences(ctx context.Context, userID vo.Id) error

	// ReassignAuthorInForeignAccounts makes each row userID wrote into an
	// account it does not own authored by that account's owner.
	ReassignAuthorInForeignAccounts(ctx context.Context, userID vo.Id) error

	// ListCrossOwnerTransfers lists the transfers with exactly one side on
	// userID's accounts.
	ListCrossOwnerTransfers(ctx context.Context, userID vo.Id) ([]CrossOwnerTransfer, error)

	// ReplaceTransferWithCorrection deletes t and inserts c on the surviving
	// side; a nil c only deletes. The surviving account's revision moves.
	ReplaceTransferWithCorrection(ctx context.Context, t CrossOwnerTransfer, c *WipeCorrection) error
}

// CrossOwnerTransfer is a transfer between a deleted user's account and
// someone else's.
type CrossOwnerTransfer struct {
	ID, SourceAccountID, RecipientAccountID vo.Id
	SourceOwnerID, RecipientOwnerID         vo.Id
	Amount                                  string
	AmountRecipient                         *string
	Description                             string
	SpentAt                                 time.Time
	SortKey                                 string
	RecipientSortKey                        *string
}

// WipeCorrection replaces a CrossOwnerTransfer on its surviving side.
type WipeCorrection struct {
	ID, AuthorID, AccountID vo.Id
	Type                    model.TransactionType
	Amount                  string
	Description             string
	SpentAt                 time.Time
	SortKey                 string
	CreatedAt               time.Time
}
