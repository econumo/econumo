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
}
