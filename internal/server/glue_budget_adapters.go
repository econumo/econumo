// BudgetAccountLookup adapts the account repository to app/budget.AccountLookup.
// It lives here, not in internal/infra/repo/budget, because it needs the
// account feature's Account type and an infra package must not import a
// feature (see archtest).
package server

import (
	"context"

	account "github.com/econumo/econumo/internal/account"
	appbudget "github.com/econumo/econumo/internal/app/budget"
	category "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/shared/vo"
)

type budgetAccountRepo interface {
	ListAvailable(ctx context.Context, userID vo.Id) ([]*account.Account, error)
	GetByID(ctx context.Context, id vo.Id) (*account.Account, error)
}

// BudgetAccountLookup adapts the account repository to app/budget.AccountLookup.
type BudgetAccountLookup struct {
	accounts budgetAccountRepo
}

var _ appbudget.AccountLookup = (*BudgetAccountLookup)(nil)

// NewBudgetAccountLookup wraps an account repository.
func NewBudgetAccountLookup(accounts budgetAccountRepo) *BudgetAccountLookup {
	return &BudgetAccountLookup{accounts: accounts}
}

// AccountsForOwners returns the accounts OWNED by the given users. Budget
// membership is owner-only (a.user IN :users), NOT the own+shared "available"
// set. ListAvailable returns own + shared accounts, so we filter to accounts
// actually owned by one of the participants — otherwise shared accounts inflate
// the budget's start balance.
func (l *BudgetAccountLookup) AccountsForOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.AccountView, error) {
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
func (l *BudgetAccountLookup) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	a, err := l.accounts.GetByID(ctx, accountID)
	if err != nil {
		return vo.Id{}, err
	}
	return a.UserId(), nil
}

type budgetCategoryRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*category.Category, error)
}

// BudgetCategoryMetadataLookup adapts the category repository to the category
// slice of app/budget.MetadataLookup (wired into budgetrepo.MetadataLookup's
// categories field). It lives here, not in internal/infra/repo/budget,
// because it needs the category feature's Category type and an infra package
// must not import a feature (see archtest).
type BudgetCategoryMetadataLookup struct {
	categories budgetCategoryRepo
}

// NewBudgetCategoryMetadataLookup wraps a category repository.
func NewBudgetCategoryMetadataLookup(categories budgetCategoryRepo) *BudgetCategoryMetadataLookup {
	return &BudgetCategoryMetadataLookup{categories: categories}
}

// CategoriesByOwners returns all categories owned by the given users.
func (l *BudgetCategoryMetadataLookup) CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]appbudget.CategoryMeta, error) {
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
				IsIncome: c.Type() == category.TypeIncome, IsArchived: c.IsArchived(),
			})
		}
	}
	return out, nil
}
