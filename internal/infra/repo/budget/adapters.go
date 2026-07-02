// Adapters wiring the budget service's cross-module ports (AccountLookup,
// MetadataLookup) to the existing account / category / tag / payee
// repositories. They live here (infra) so app/budget depends only on its own
// small interfaces. The UserLookup counterpart lives in internal/server (it
// needs the user feature's User/Header types, which an infra package must
// not import).
//
// The multi-owner reads (categories/tags/accounts for the budget participants)
// loop the existing single-owner repo queries: a budget's participant set is
// tiny (owner + a few accepted users), so N small queries are fine and avoid a
// new dynamic-IN query surface.
package budgetrepo

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/app/budget"
	domaccount "github.com/econumo/econumo/internal/domain/account"
	domcategory "github.com/econumo/econumo/internal/domain/category"
	dompayee "github.com/econumo/econumo/internal/domain/payee"
	domtag "github.com/econumo/econumo/internal/domain/tag"
	"github.com/econumo/econumo/internal/shared/vo"
)

type accountRepo interface {
	ListAvailable(ctx context.Context, userID vo.Id) ([]*domaccount.Account, error)
	GetByID(ctx context.Context, id vo.Id) (*domaccount.Account, error)
}

// AccountLookup adapts the account repository to app/budget.AccountLookup.
type AccountLookup struct {
	accounts accountRepo
}

var _ appbudget.AccountLookup = (*AccountLookup)(nil)

// NewAccountLookup wraps an account repository.
func NewAccountLookup(accounts accountRepo) *AccountLookup { return &AccountLookup{accounts: accounts} }

// AccountsForOwners returns the accounts OWNED by the given users. Budget
// membership is owner-only (a.user IN :users), NOT the own+shared "available"
// set. ListAvailable returns own + shared accounts, so we filter to accounts
// actually owned by one of the participants — otherwise shared accounts inflate
// the budget's start balance.
func (l *AccountLookup) AccountsForOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.AccountView, error) {
	owners := make(map[string]bool, len(userIDs))
	for _, uid := range userIDs {
		owners[uid.String()] = true
	}
	var out []appbudget.AccountView
	seen := map[string]bool{}
	for _, uid := range userIDs {
		accts, err := l.accounts.ListAvailable(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, a := range accts {
			if !owners[a.UserId().String()] {
				continue // shared with a participant but not owned by one
			}
			if seen[a.Id().String()] {
				continue
			}
			seen[a.Id().String()] = true
			out = append(out, appbudget.AccountView{
				ID: a.Id().String(), CurrencyID: a.CurrencyId().String(), OwnerID: a.UserId().String(),
			})
		}
	}
	return out, nil
}

// AccountOwner returns an account's owner id.
func (l *AccountLookup) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	a, err := l.accounts.GetByID(ctx, accountID)
	if err != nil {
		return vo.Id{}, err
	}
	return a.UserId(), nil
}

type categoryRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*domcategory.Category, error)
}
type tagRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*domtag.Tag, error)
}
type payeeRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*dompayee.Payee, error)
}

// MetadataLookup adapts the category + tag + payee repositories to
// app/budget.MetadataLookup.
type MetadataLookup struct {
	categories categoryRepo
	tags       tagRepo
	payees     payeeRepo
}

var _ appbudget.MetadataLookup = (*MetadataLookup)(nil)

// NewMetadataLookup wraps the category + tag + payee repositories.
func NewMetadataLookup(categories categoryRepo, tags tagRepo, payees payeeRepo) *MetadataLookup {
	return &MetadataLookup{categories: categories, tags: tags, payees: payees}
}

// PayeesByOwners returns all payees owned by the given users.
func (l *MetadataLookup) PayeesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.PayeeMeta, error) {
	var out []appbudget.PayeeMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		payees, err := l.payees.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, p := range payees {
			if seen[p.Id().String()] {
				continue
			}
			seen[p.Id().String()] = true
			out = append(out, appbudget.PayeeMeta{ID: p.Id().String(), Name: p.Name()})
		}
	}
	return out, nil
}

// CategoriesByOwners returns all categories owned by the given users.
func (l *MetadataLookup) CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.CategoryMeta, error) {
	var out []appbudget.CategoryMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		cats, err := l.categories.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, c := range cats {
			if seen[c.Id().String()] {
				continue
			}
			seen[c.Id().String()] = true
			out = append(out, appbudget.CategoryMeta{
				ID: c.Id().String(), OwnerID: c.UserId().String(), Name: c.Name(), Icon: c.Icon(),
				IsIncome: c.Type() == domcategory.TypeIncome, IsArchived: c.IsArchived(),
			})
		}
	}
	return out, nil
}

// TagsByOwners returns all tags owned by the given users.
func (l *MetadataLookup) TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.TagMeta, error) {
	var out []appbudget.TagMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		tags, err := l.tags.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, t := range tags {
			if seen[t.Id().String()] {
				continue
			}
			seen[t.Id().String()] = true
			out = append(out, appbudget.TagMeta{
				ID: t.Id().String(), OwnerID: t.UserId().String(), Name: t.Name(), IsArchived: t.IsArchived(),
			})
		}
	}
	return out, nil
}
