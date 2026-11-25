package server

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

type importLinkMarkers interface {
	ClearAssignedByTransaction(ctx context.Context, transactionID vo.Id) error
}

// TransactionReclassifiedImports clears who classified an imported
// transaction once the user reclassifies it by hand. It runs inside the
// edit's own DB transaction.
type TransactionReclassifiedImports struct {
	links importLinkMarkers
}

func NewTransactionReclassifiedImports(links importLinkMarkers) *TransactionReclassifiedImports {
	return &TransactionReclassifiedImports{links: links}
}

func (a *TransactionReclassifiedImports) ReclassifiedByHand(ctx context.Context, transactionID vo.Id) error {
	return a.links.ClearAssignedByTransaction(ctx, transactionID)
}

var _ apptransaction.ReclassificationListener = (*TransactionReclassifiedImports)(nil)
