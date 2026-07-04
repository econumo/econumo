// TransactionAccountResolver adapts the account service to
// transaction.AccountResolver, TransactionCategoryNameLookup adapts the
// category repository to the transaction export adapter's categoryNameLookup
// port, TransactionTagNameLookup adapts the tag repository to its
// tagNameLookup port, and TransactionPayeeNameLookup adapts the payee
// repository to its payeeNameLookup port. All four live here, not in
// internal/transaction/repo, because they need the
// account/category/tag/payee features' types and an infra package must not
// import a feature (see archtest).
package server

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

// transactionAccountResolverPort is the subset of the account service this
// adapter uses.
type transactionAccountResolverPort interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	AccountListForUser(ctx context.Context, userID vo.Id) ([]model.AccountResult, error)
}

// TransactionAccountResolver adapts the account service to
// transaction.AccountResolver.
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

// AccountListForUser now returns the account feature's own model.AccountResult
// directly: transaction's AccountResult/AccountOwnerResult/AccountCurrencyResult/
// AccountSharedAccess twins retired in favor of the shared model.* survivors
// (see the collision map), so no field-for-field conversion remains — this
// method (and the resolver as a whole) is now a pure pass-through, retired in
// the next commit per the Phase 4 "delete the adapter once it's an identity
// conversion" rule.
func (a *TransactionAccountResolver) AccountListForUser(ctx context.Context, userID vo.Id) ([]model.AccountResult, error) {
	return a.svc.AccountListForUser(ctx, userID)
}

// transactionCategoryByID is the minimal category-repo surface the export
// adapter's name lookup uses.
type transactionCategoryByID interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Category, error)
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
	return c.Name, nil
}

// transactionTagByID is the minimal tag-repo surface the export adapter's
// name lookup uses.
type transactionTagByID interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Tag, error)
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
	return t.Name, nil
}

// transactionPayeeByID is the minimal payee-repo surface the export adapter's
// name lookup uses.
type transactionPayeeByID interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Payee, error)
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
	return p.Name, nil
}
