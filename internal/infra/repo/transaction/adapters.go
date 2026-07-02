// Adapters bridging the transaction service's ports to existing collaborators:
// the account service (VisibleAccounts) and the metadata repos (export). Kept
// in infra so app/transaction depends only on its own small interfaces. The
// UserLookup, AccountResolver, category-name-lookup, tag-name-lookup, and
// payee-name-lookup counterparts live in internal/server: they need the
// account/category/tag/payee features' types, which an infra package must not
// import — CategoryName/TagName/PayeeName delegate to
// categoryNameLookup/tagNameLookup/payeeNameLookup, ports expressed purely in
// primitives, so this file itself never imports category, tag, or payee.
package transactionrepo

import (
	"context"
	"errors"

	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// visibleAccountsPort is the subset of the account service VisibleAccounts uses.
type visibleAccountsPort interface {
	VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

// accountGrantReader is the subset of the connection AccountAccess repo used to
// read a single (account, user) grant for the transaction write-access check.
type accountGrantReader interface {
	Get(ctx context.Context, accountID, userID vo.Id) (*domconnection.AccountAccess, error)
}

// AccountGrants adapts the connection AccountAccess repo to
// app/transaction.AccountGrants.
type AccountGrants struct{ access accountGrantReader }

var _ apptransaction.AccountGrants = (*AccountGrants)(nil)

// NewAccountGrants wraps the connection AccountAccess repo.
func NewAccountGrants(access accountGrantReader) *AccountGrants {
	return &AccountGrants{access: access}
}

// GrantRole returns the user's granted role on the account. A missing grant
// (the repo's *errs.NotFoundError) is reported as ok=false, nil error — the
// "no access" case the write-access check treats as denied; other errors
// propagate.
func (g *AccountGrants) GrantRole(ctx context.Context, accountID, userID vo.Id) (domconnection.Role, bool, error) {
	grant, err := g.access.Get(ctx, accountID, userID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return grant.Role(), true, nil
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
