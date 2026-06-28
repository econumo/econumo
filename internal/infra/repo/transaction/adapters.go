// Adapters bridging the transaction service's ports to existing collaborators:
// the account service (AccountResolver + VisibleAccounts) and a user lookup for
// the author embed. Kept in infra so app/transaction depends only on its own
// small interfaces.
package transactionrepo

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/app/account"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	domcategory "github.com/econumo/econumo/internal/domain/category"
	dompayee "github.com/econumo/econumo/internal/domain/payee"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtag "github.com/econumo/econumo/internal/domain/tag"
	domuser "github.com/econumo/econumo/internal/domain/user"
)

// accountServicePort is the subset of the account service the adapters use.
type accountServicePort interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	AccountListForUser(ctx context.Context, userID vo.Id) ([]appaccount.AccountResult, error)
	VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

// AccountResolver adapts the account service to app/transaction.AccountResolver.
type AccountResolver struct{ svc accountServicePort }

var _ apptransaction.AccountResolver = (*AccountResolver)(nil)

// NewAccountResolver wraps the account service.
func NewAccountResolver(svc accountServicePort) *AccountResolver { return &AccountResolver{svc: svc} }

func (a *AccountResolver) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return a.svc.AccountOwner(ctx, accountID)
}
func (a *AccountResolver) AccountListForUser(ctx context.Context, userID vo.Id) ([]appaccount.AccountResult, error) {
	return a.svc.AccountListForUser(ctx, userID)
}

// VisibleAccounts adapts the account service to app/transaction.VisibleAccounts.
type VisibleAccounts struct{ svc accountServicePort }

var _ apptransaction.VisibleAccounts = (*VisibleAccounts)(nil)

// NewVisibleAccounts wraps the account service.
func NewVisibleAccounts(svc accountServicePort) *VisibleAccounts { return &VisibleAccounts{svc: svc} }

func (v *VisibleAccounts) VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	return v.svc.VisibleAccountIDs(ctx, userID)
}

// userByID is the minimal user-repo surface for the author embed.
type userByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (domuser.Header, error)
}

// UserLookup adapts the user repository to app/transaction.UserLookup.
type UserLookup struct{ users userByID }

var _ apptransaction.UserLookup = (*UserLookup)(nil)

// NewUserLookup wraps a user repository.
func NewUserLookup(users userByID) *UserLookup { return &UserLookup{users: users} }

func (l *UserLookup) GetOwner(ctx context.Context, userID string) (apptransaction.AuthorView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return apptransaction.AuthorView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return apptransaction.AuthorView{}, err
	}
	return apptransaction.AuthorView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}

// --- export lookup ---

// exportAccountLister is the subset of the transaction repo the export adapter
// uses to read the accessible-account set.
type exportAccountLister interface {
	ListExportAccountsForUser(ctx context.Context, userID vo.Id) ([]ExportAccountRow, error)
}

// categoryByID/tagByID/payeeByID are the minimal metadata-repo surfaces for name
// resolution (mirroring the eager-loaded Category/Tag/Payee getters in PHP).
type categoryByID interface {
	GetByID(ctx context.Context, id vo.Id) (*domcategory.Category, error)
}
type tagByID interface {
	GetByID(ctx context.Context, id vo.Id) (*domtag.Tag, error)
}
type payeeByID interface {
	GetByID(ctx context.Context, id vo.Id) (*dompayee.Payee, error)
}

// ExportLookup adapts the transaction + metadata repos to
// app/transaction.ExportLookup. Name lookups return "" when the entity cannot be
// resolved (e.g. a related entity owned by another user on a shared account) —
// matching PHP, which simply reads the eager-loaded relation's name or null.
type ExportLookup struct {
	accounts   exportAccountLister
	categories categoryByID
	tags       tagByID
	payees     payeeByID
}

var _ apptransaction.ExportLookup = (*ExportLookup)(nil)

// NewExportLookup wires the export lookup over the transaction repo and the
// category/tag/payee repos.
func NewExportLookup(accounts exportAccountLister, categories categoryByID, tags tagByID, payees payeeByID) *ExportLookup {
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
	c, err := l.categories.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return c.Name(), nil
}

// TagName resolves a tag's name ("" if not found).
func (l *ExportLookup) TagName(ctx context.Context, id vo.Id) (string, error) {
	t, err := l.tags.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return t.Name(), nil
}

// PayeeName resolves a payee's name ("" if not found).
func (l *ExportLookup) PayeeName(ctx context.Context, id vo.Id) (string, error) {
	p, err := l.payees.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return p.Name(), nil
}
