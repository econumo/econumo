// Adapters bridging the transaction service's ports to existing collaborators:
// the account service (VisibleAccounts) and the metadata repos (export). Kept
// in infra so app/transaction depends only on its own small interfaces. The
// UserLookup, AccountResolver, category-name-lookup, tag-name-lookup, and
// payee-name-lookup counterparts live in internal/server: they need the
// account/category/tag/payee features' types, which an infra package must not
// import — CategoryName/TagName/PayeeName delegate to
// categoryNameLookup/tagNameLookup/payeeNameLookup, ports expressed purely in
// primitives, so this file itself never imports category, tag, or payee.
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

// visibleAccountsPort is the subset of the account service VisibleAccounts uses.
type visibleAccountsPort interface {
	VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

// accountWriteGranter reports whether a user holds an admin-or-user grant on
// an account; backed by connectionrepo.AccountAccessResolver.HasWriteGrant.
type accountWriteGranter interface {
	HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// AccountGrants adapts the connection AccountAccess resolver to
// app/transaction.AccountGrants.
type AccountGrants struct{ access accountWriteGranter }

var _ apptransaction.AccountGrants = (*AccountGrants)(nil)

// NewAccountGrants wraps the connection AccountAccessResolver.
func NewAccountGrants(access accountWriteGranter) *AccountGrants {
	return &AccountGrants{access: access}
}

// HasWriteGrant reports whether the user holds an admin-or-user grant on the
// account (the write-access check; a missing or guest grant is false).
func (g *AccountGrants) HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error) {
	return g.access.HasWriteGrant(ctx, accountID, userID)
}

// VisibleAccounts adapts the account service to app/transaction.VisibleAccounts.
type VisibleAccounts struct{ svc visibleAccountsPort }

var _ apptransaction.VisibleAccounts = (*VisibleAccounts)(nil)

// NewVisibleAccounts wraps the account service.
func NewVisibleAccounts(svc visibleAccountsPort) *VisibleAccounts { return &VisibleAccounts{svc: svc} }

func (v *VisibleAccounts) VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	return v.svc.VisibleAccountIDs(ctx, userID)
}

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
func (l *ExportLookup) ExportAccounts(ctx context.Context, userID vo.Id) ([]apptransaction.ExportAccount, error) {
	rows, err := l.accounts.ListExportAccountsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ExportAccount, len(rows))
	for i, r := range rows {
		out[i] = apptransaction.ExportAccount{ID: r.ID, Name: r.Name, CurrencyCode: r.CurrencyCode}
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
