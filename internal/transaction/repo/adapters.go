// ExportLookup bridges the transaction service's ExportLookup port to the
// transaction repo + metadata repos (export). Kept in infra so app/transaction
// depends only on its own small interfaces. The UserLookup, AccountResolver,
// AccountGrants, VisibleAccounts, category-name-lookup, tag-name-lookup, and
// payee-name-lookup counterparts live in internal/server or are wired directly
// from concretes that already satisfy the transaction ports structurally: they
// need the account/category/tag/payee features' types, which an infra package
// must not import — CategoryName/TagName/PayeeName delegate to
// categoryNameLookup/tagNameLookup/payeeNameLookup, ports expressed purely in
// primitives, so this file itself never imports category, tag, or payee.
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

// exportAccountLister is the subset of the transaction repo the export adapter
// uses to read the accessible-account set.
type exportAccountLister interface {
	ListExportAccountsForUser(ctx context.Context, userID vo.Id) ([]ExportAccountRow, error)
}

// categoryNameLookup resolves a category's name ("" if not found); backed by
// server.TransactionCategoryNameLookup so this leaf never imports category.
// tagNameLookup is the tag equivalent, backed by
// server.TransactionTagNameLookup. payeeNameLookup is the payee equivalent,
// backed by server.TransactionPayeeNameLookup.
type categoryNameLookup interface {
	CategoryName(ctx context.Context, id vo.Id) (string, error)
}
type tagNameLookup interface {
	TagName(ctx context.Context, id vo.Id) (string, error)
}
type payeeNameLookup interface {
	PayeeName(ctx context.Context, id vo.Id) (string, error)
}

// ExportLookup adapts the transaction + metadata repos to
// app/transaction.ExportLookup. Name lookups return "" when the entity cannot be
// resolved (e.g. a related entity owned by another user on a shared account).
type ExportLookup struct {
	accounts   exportAccountLister
	categories categoryNameLookup
	tags       tagNameLookup
	payees     payeeNameLookup
}

var _ apptransaction.ExportLookup = (*ExportLookup)(nil)

// NewExportLookup wires the export lookup over the transaction repo, the
// category name lookup (server.TransactionCategoryNameLookup), the tag name
// lookup (server.TransactionTagNameLookup), and the payee name lookup
// (server.TransactionPayeeNameLookup).
func NewExportLookup(accounts exportAccountLister, categories categoryNameLookup, tags tagNameLookup, payees payeeNameLookup) *ExportLookup {
	return &ExportLookup{accounts: accounts, categories: categories, tags: tags, payees: payees}
}

// ExportAccounts returns the user's accessible accounts (own + shared, not
// deleted) with their currency code.
func (l *ExportLookup) ExportAccounts(ctx context.Context, userID vo.Id) ([]model.ExportAccount, error) {
	rows, err := l.accounts.ListExportAccountsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ExportAccount, len(rows))
	for i, r := range rows {
		out[i] = model.ExportAccount{ID: r.ID, Name: r.Name, CurrencyCode: r.CurrencyCode}
	}
	return out, nil
}

// CategoryName resolves a category's name ("" if not found).
func (l *ExportLookup) CategoryName(ctx context.Context, id vo.Id) (string, error) {
	return l.categories.CategoryName(ctx, id)
}

// TagName resolves a tag's name ("" if not found).
func (l *ExportLookup) TagName(ctx context.Context, id vo.Id) (string, error) {
	return l.tags.TagName(ctx, id)
}

// PayeeName resolves a payee's name ("" if not found).
func (l *ExportLookup) PayeeName(ctx context.Context, id vo.Id) (string, error) {
	return l.payees.PayeeName(ctx, id)
}
