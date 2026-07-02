// TransactionAccountResolver adapts the account service to
// app/transaction.AccountResolver. It lives here, not in
// internal/infra/repo/transaction, because it needs the account feature's
// AccountResult type and an infra package must not import a feature (see
// archtest).
package server

import (
	"context"

	account "github.com/econumo/econumo/internal/account"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	"github.com/econumo/econumo/internal/shared/vo"
)

// transactionAccountResolverPort is the subset of the account service this
// adapter uses.
type transactionAccountResolverPort interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	AccountListForUser(ctx context.Context, userID vo.Id) ([]account.AccountResult, error)
}

// TransactionAccountResolver adapts the account service to
// app/transaction.AccountResolver.
type TransactionAccountResolver struct {
	svc transactionAccountResolverPort
}

var _ apptransaction.AccountResolver = (*TransactionAccountResolver)(nil)

// NewTransactionAccountResolver wraps the account service.
func NewTransactionAccountResolver(svc transactionAccountResolverPort) *TransactionAccountResolver {
	return &TransactionAccountResolver{svc: svc}
}

func (a *TransactionAccountResolver) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return a.svc.AccountOwner(ctx, accountID)
}

func (a *TransactionAccountResolver) AccountListForUser(ctx context.Context, userID vo.Id) ([]account.AccountResult, error) {
	return a.svc.AccountListForUser(ctx, userID)
}
