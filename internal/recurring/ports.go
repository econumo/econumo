package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Consumer-side ports; internal/server wires the account, connection and
// transaction services onto these at composition time.
type AccountResolver interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
}

type AccountGrants interface {
	HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

type VisibleAccounts interface {
	VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

type TransactionCreator interface {
	CreateTransactionFromRecurring(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest, recurringID vo.Id) (*model.CreateTransactionResult, error)

	// LinkTransactionToRecurring stamps recurringID onto an existing transaction
	// (create-from-transaction attaches the source as the series' first instance).
	LinkTransactionToRecurring(ctx context.Context, userID, transactionID, recurringID vo.Id) error
}

// LabelOwnership resolves the owning user for a set of label ids, for
// validating a create/update template's labelIds. Mirrors
// transaction.LabelOwnership exactly: a missing id is simply absent from the
// returned map, so the caller's membership check (id present AND owner ==
// the template's account owner) rejects both an unknown id and one owned by
// someone else.
type LabelOwnership interface {
	LabelOwners(ctx context.Context, ids []vo.Id) (map[string]vo.Id, error)
}
