// BudgetUserLookup satisfies the budget service's UserLookup port (owner
// embed, default currency code, active-budget write) by delegating to the
// user repository. It lives here, not in internal/budget/repo, because
// it needs the model package's User/Header types and an infra package must
// not import a feature (see archtest).
package server

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

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
func (l *BudgetUserLookup) GetOwner(ctx context.Context, userID string) (appbudget.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return appbudget.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return appbudget.OwnerView{}, err
	}
	return appbudget.OwnerView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}

// CurrencyCode returns the user's default currency code (the currency option).
func (l *BudgetUserLookup) CurrencyCode(ctx context.Context, userID string) (string, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return "", err
	}
	u, err := l.users.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if o := u.Option(model.OptionCurrency); o != nil && o.Value != nil {
		return *o.Value, nil
	}
	return model.DefaultCurrency, nil
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
