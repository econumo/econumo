// TransactionAccountResolver adapts the account service to
// app/transaction.AccountResolver, and TransactionCategoryNameLookup adapts
// the category repository to the transaction export adapter's
// categoryNameLookup port. Both live here, not in
// internal/infra/repo/transaction, because they need the account/category
// features' types and an infra package must not import a feature (see
// archtest).
package server

import (
	"context"

	account "github.com/econumo/econumo/internal/account"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	category "github.com/econumo/econumo/internal/category"
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

// transactionCategoryByID is the minimal category-repo surface the export
// adapter's name lookup uses.
type transactionCategoryByID interface {
	GetByID(ctx context.Context, id vo.Id) (*category.Category, error)
}

// TransactionCategoryNameLookup adapts the category repository to the
// transaction export adapter's categoryNameLookup port.
type TransactionCategoryNameLookup struct {
	categories transactionCategoryByID
}

// NewTransactionCategoryNameLookup wraps a category repository.
func NewTransactionCategoryNameLookup(categories transactionCategoryByID) *TransactionCategoryNameLookup {
	return &TransactionCategoryNameLookup{categories: categories}
}

// CategoryName resolves a category's name ("" if not found).
func (l *TransactionCategoryNameLookup) CategoryName(ctx context.Context, id vo.Id) (string, error) {
	c, err := l.categories.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return c.Name(), nil
}
