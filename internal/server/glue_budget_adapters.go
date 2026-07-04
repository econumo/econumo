// BudgetAccountLookup adapts the account repository to budget.AccountLookup,
// BudgetCategoryMetadataLookup adapts the category repository,
// BudgetTagMetadataLookup adapts the tag repository, and
// BudgetPayeeMetadataLookup adapts the payee repository. They live here, not
// in internal/budget/repo, because they need the
// account/category/tag/payee features' types and an infra package must not
// import a feature (see archtest).
package server

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type budgetAccountRepo interface {
	ListAvailable(ctx context.Context, userID vo.Id) ([]*model.Account, error)
	GetByID(ctx context.Context, id vo.Id) (*model.Account, error)
}

// BudgetAccountLookup adapts the account repository to budget.AccountLookup.
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
func (l *BudgetAccountLookup) AccountsForOwners(ctx context.Context, userIDs []vo.Id) ([]model.AccountView, error) {
	owners := make(map[string]bool, len(userIDs))
	for _, uid := range userIDs {
		owners[uid.String()] = true
	}
	var out []model.AccountView
	seen := map[string]bool{}
	for _, uid := range userIDs {
		accts, err := l.accounts.ListAvailable(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, a := range accts {
			if !owners[a.UserID.String()] {
				continue // shared with a participant but not owned by one
			}
			if seen[a.ID.String()] {
				continue
			}
			seen[a.ID.String()] = true
			out = append(out, model.AccountView{
				ID: a.ID.String(), CurrencyID: a.CurrencyID.String(), OwnerID: a.UserID.String(),
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
	return a.UserID, nil
}

type budgetCategoryRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Category, error)
}

// BudgetCategoryMetadataLookup adapts the category repository to the category
// slice of budget.MetadataLookup (wired into budgetrepo.MetadataLookup's
// categories field). It lives here, not in internal/budget/repo,
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
func (l *BudgetCategoryMetadataLookup) CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]model.CategoryMeta, error) {
	var out []model.CategoryMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		cats, err := l.categories.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, c := range cats {
			if seen[c.ID.String()] {
				continue
			}
			seen[c.ID.String()] = true
			out = append(out, model.CategoryMeta{
				ID: c.ID.String(), OwnerID: c.UserID.String(), Name: c.Name, Icon: c.Icon,
				IsIncome: c.Type == model.TypeIncome, IsArchived: c.IsArchived,
			})
		}
	}
	return out, nil
}

type budgetTagRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Tag, error)
}

// BudgetTagMetadataLookup adapts the tag repository to the tag slice of
// budget.MetadataLookup (wired into budgetrepo.MetadataLookup's tags
// field). It lives here, not in internal/budget/repo, because it needs
// the tag feature's Tag type and an infra package must not import a feature
// (see archtest).
type BudgetTagMetadataLookup struct {
	tags budgetTagRepo
}

// NewBudgetTagMetadataLookup wraps a tag repository.
func NewBudgetTagMetadataLookup(tags budgetTagRepo) *BudgetTagMetadataLookup {
	return &BudgetTagMetadataLookup{tags: tags}
}

// TagsByOwners returns all tags owned by the given users.
func (l *BudgetTagMetadataLookup) TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]model.TagMeta, error) {
	var out []model.TagMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		tags, err := l.tags.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, t := range tags {
			if seen[t.ID.String()] {
				continue
			}
			seen[t.ID.String()] = true
			out = append(out, model.TagMeta{
				ID: t.ID.String(), OwnerID: t.UserID.String(), Name: t.Name, IsArchived: t.IsArchived,
			})
		}
	}
	return out, nil
}

type budgetPayeeRepo interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Payee, error)
}

// BudgetPayeeMetadataLookup adapts the payee repository to the payee slice of
// budget.MetadataLookup (wired into budgetrepo.MetadataLookup's payees
// field). It lives here, not in internal/budget/repo, because it needs
// the payee feature's Payee type and an infra package must not import a
// feature (see archtest).
type BudgetPayeeMetadataLookup struct {
	payees budgetPayeeRepo
}

// NewBudgetPayeeMetadataLookup wraps a payee repository.
func NewBudgetPayeeMetadataLookup(payees budgetPayeeRepo) *BudgetPayeeMetadataLookup {
	return &BudgetPayeeMetadataLookup{payees: payees}
}

// PayeesByOwners returns all payees owned by the given users.
func (l *BudgetPayeeMetadataLookup) PayeesByOwners(ctx context.Context, userIDs []vo.Id) ([]model.PayeeMeta, error) {
	var out []model.PayeeMeta
	seen := map[string]bool{}
	for _, uid := range userIDs {
		payees, err := l.payees.ListByOwner(ctx, uid)
		if err != nil {
			return nil, err
		}
		for _, p := range payees {
			if seen[p.ID.String()] {
				continue
			}
			seen[p.ID.String()] = true
			out = append(out, model.PayeeMeta{ID: p.ID.String(), Name: p.Name})
		}
	}
	return out, nil
}
