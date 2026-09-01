// Budget glue: every adapter satisfying a port that the budget feature
// declares (see internal/budget/ports.go). Features must not import each
// other (archtest); the composition root bridges them here.
package server

import (
	"context"
	"fmt"
	appbudget "github.com/econumo/econumo/internal/budget"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BudgetCurrencyLookup adapts currencyrepo.Lookup to budget.CurrencyLookup
// (the usability check on explicit currency ids).
type BudgetCurrencyLookup struct {
	inner *currencyrepo.Lookup
}

var _ appbudget.CurrencyLookup = (*BudgetCurrencyLookup)(nil)

// NewBudgetCurrencyLookup wraps a currencyrepo.Lookup.
func NewBudgetCurrencyLookup(inner *currencyrepo.Lookup) *BudgetCurrencyLookup {
	return &BudgetCurrencyLookup{inner: inner}
}

// EnsureUsable confirms the currency is usable by the user (global, or their
// own custom).
func (l *BudgetCurrencyLookup) EnsureUsable(ctx context.Context, userID, currencyID string) error {
	return l.inner.EnsureUsable(ctx, userID, currencyID)
}

type budgetAccountRepo interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Account, error)
	ListAvailable(ctx context.Context, userID vo.Id) ([]*model.Account, error)
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

// AccountsByIDs resolves the budget's explicit member accounts, in input
// order. Deleted accounts are returned too (never filtered here) — a deleted
// member keeps counting in the budget. An unknown id propagates the account
// repo's *errs.NotFoundError: a hard-deleted account cascades its membership
// row, so it can never legitimately be asked for.
func (l *BudgetAccountLookup) AccountsByIDs(ctx context.Context, ids []vo.Id) ([]model.AccountView, error) {
	out := make([]model.AccountView, 0, len(ids))
	for _, id := range ids {
		a, err := l.accounts.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, model.AccountView{
			ID: a.ID.String(), CurrencyID: a.CurrencyID.String(), OwnerID: a.UserID.String(), IsDeleted: a.IsDeleted,
		})
	}
	return out, nil
}

// OwnedLiveAccountIDs returns the user's own non-deleted accounts.
// ListAvailable also yields accounts shared WITH the user (accepted
// accounts_access grants); those belong to their owner's budgets, not the
// acceptor's, so the ownership filter here is load-bearing.
func (l *BudgetAccountLookup) OwnedLiveAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	accounts, err := l.accounts.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]vo.Id, 0, len(accounts))
	for _, a := range accounts {
		if a.IsDeleted || !a.UserID.Equal(userID) {
			continue
		}
		out = append(out, a.ID)
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

// budgetUserRepo is the minimal user-repo surface this adapter needs.
type budgetUserRepo interface {
	GetByID(ctx context.Context, id vo.Id) (*model.User, error)
	GetHeaderByID(ctx context.Context, id vo.Id) (model.Header, error)
	Save(ctx context.Context, u *model.User) error
}

// BudgetUserLookup adapts the user repository to budget.UserLookup.
type BudgetUserLookup struct {
	users budgetUserRepo
	clock port.Clock
}

var _ appbudget.UserLookup = (*BudgetUserLookup)(nil)

// NewBudgetUserLookup wraps a user repository + clock.
func NewBudgetUserLookup(users budgetUserRepo, clock port.Clock) *BudgetUserLookup {
	return &BudgetUserLookup{users: users, clock: clock}
}

// GetOwner resolves the embed (id, name, avatar).
func (l *BudgetUserLookup) GetOwner(ctx context.Context, userID string) (model.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return model.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return model.OwnerView{}, err
	}
	return model.OwnerView{ID: h.ID, Name: h.Name, Avatar: h.Avatar}, nil
}

// DefaultCurrencyID returns the user's stored default currency id. The
// migration guarantees every user holds one, so an absent option is an error.
func (l *BudgetUserLookup) DefaultCurrencyID(ctx context.Context, userID string) (string, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return "", err
	}
	u, err := l.users.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if o := u.Option(model.OptionCurrency); o != nil && o.Value != nil && *o.Value != "" {
		return *o.Value, nil
	}
	return "", fmt.Errorf("user %s: currency option missing", userID)
}

// SetActiveBudget writes the user's active-budget option.
func (l *BudgetUserLookup) SetActiveBudget(ctx context.Context, userID, budgetID vo.Id) error {
	u, err := l.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	u.UpdateBudget(budgetID.String(), l.clock.Now())
	return l.users.Save(ctx, u)
}

// ClearActiveBudget clears the user's active-budget option when it points at
// the given budget.
func (l *BudgetUserLookup) ClearActiveBudget(ctx context.Context, userID, budgetID vo.Id) error {
	u, err := l.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !u.ClearBudget(budgetID.String(), l.clock.Now()) {
		return nil
	}
	return l.users.Save(ctx, u)
}
