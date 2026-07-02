// TransactionAccountResolver adapts the account service to
// app/transaction.AccountResolver, TransactionCategoryNameLookup adapts the
// category repository to the transaction export adapter's categoryNameLookup
// port, TransactionTagNameLookup adapts the tag repository to its
// tagNameLookup port, and TransactionPayeeNameLookup adapts the payee
// repository to its payeeNameLookup port. All four live here, not in
// internal/infra/repo/transaction, because they need the
// account/category/tag/payee features' types and an infra package must not
// import a feature (see archtest).
package server

import (
	"context"

	account "github.com/econumo/econumo/internal/account"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	category "github.com/econumo/econumo/internal/category"
	payee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/shared/vo"
	tag "github.com/econumo/econumo/internal/tag"
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

// transactionTagByID is the minimal tag-repo surface the export adapter's
// name lookup uses.
type transactionTagByID interface {
	GetByID(ctx context.Context, id vo.Id) (*tag.Tag, error)
}

// TransactionTagNameLookup adapts the tag repository to the transaction
// export adapter's tagNameLookup port.
type TransactionTagNameLookup struct {
	tags transactionTagByID
}

// NewTransactionTagNameLookup wraps a tag repository.
func NewTransactionTagNameLookup(tags transactionTagByID) *TransactionTagNameLookup {
	return &TransactionTagNameLookup{tags: tags}
}

// TagName resolves a tag's name ("" if not found).
func (l *TransactionTagNameLookup) TagName(ctx context.Context, id vo.Id) (string, error) {
	t, err := l.tags.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return t.Name(), nil
}

// transactionPayeeByID is the minimal payee-repo surface the export adapter's
// name lookup uses.
type transactionPayeeByID interface {
	GetByID(ctx context.Context, id vo.Id) (*payee.Payee, error)
}

// TransactionPayeeNameLookup adapts the payee repository to the transaction
// export adapter's payeeNameLookup port.
type TransactionPayeeNameLookup struct {
	payees transactionPayeeByID
}

// NewTransactionPayeeNameLookup wraps a payee repository.
func NewTransactionPayeeNameLookup(payees transactionPayeeByID) *TransactionPayeeNameLookup {
	return &TransactionPayeeNameLookup{payees: payees}
}

// PayeeName resolves a payee's name ("" if not found).
func (l *TransactionPayeeNameLookup) PayeeName(ctx context.Context, id vo.Id) (string, error) {
	p, err := l.payees.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return p.Name(), nil
}
