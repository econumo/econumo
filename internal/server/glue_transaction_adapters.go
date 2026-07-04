// TransactionCategoryNameLookup adapts the category repository to the
// transaction export adapter's categoryNameLookup port, TransactionTagNameLookup
// adapts the tag repository to its tagNameLookup port, and
// TransactionPayeeNameLookup adapts the payee repository to its
// payeeNameLookup port. All three live here, not in internal/transaction/repo,
// because they need the category/tag/payee features' types and an infra
// package must not import a feature (see archtest).
//
// The account counterpart (transaction.AccountResolver) needs no adapter here:
// now that transaction's own AccountResult/AccountOwnerResult/
// AccountCurrencyResult/AccountSharedAccess twins are retired in favor of
// model.AccountResult (etc.), the account service's AccountOwner/
// AccountListForUser methods already match the port's signatures exactly, so
// server.go wires the account service directly (the former
// TransactionAccountResolver wrapper + its toTransactionAccountResults
// conversion were a pure pass-through once both sides spoke model.AccountResult
// — deleted per the Phase 4 "delete the adapter once it's an identity
// conversion" rule).
package server

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

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
