package server

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type debtBudgetElements interface {
	TransferElementData(ctx context.Context, t model.ElementDataTransfer) (bool, error)
	ElementHasData(ctx context.Context, externalID vo.Id, typ model.ElementType) (bool, error)
	LockElements(ctx context.Context, externalID vo.Id, typ model.ElementType) error
	RemoveElements(ctx context.Context, externalID vo.Id, typ model.ElementType) error
	SetElementsCurrency(ctx context.Context, externalID vo.Id, typ model.ElementType, currencyID vo.Id) error
}

// DebtBudgetElements adapts the budget service's method names to
// debt.BudgetElements.
type DebtBudgetElements struct{ budget debtBudgetElements }

var (
	_ debt.BudgetElements  = (*DebtBudgetElements)(nil)
	_ debt.BudgetPlans     = (*DebtBudgetPlans)(nil)
	_ appbudget.DebtSource = (*debt.Service)(nil)
)

func NewDebtBudgetElements(budget debtBudgetElements) *DebtBudgetElements {
	return &DebtBudgetElements{budget: budget}
}

func (a *DebtBudgetElements) TransferData(ctx context.Context, t model.ElementDataTransfer) (bool, error) {
	return a.budget.TransferElementData(ctx, t)
}

func (a *DebtBudgetElements) LockRows(ctx context.Context, externalID vo.Id, typ model.ElementType) error {
	return a.budget.LockElements(ctx, externalID, typ)
}

func (a *DebtBudgetElements) RemoveRows(ctx context.Context, externalID vo.Id, typ model.ElementType) error {
	return a.budget.RemoveElements(ctx, externalID, typ)
}

func (a *DebtBudgetElements) HasData(ctx context.Context, externalID vo.Id, typ model.ElementType) (bool, error) {
	return a.budget.ElementHasData(ctx, externalID, typ)
}

func (a *DebtBudgetElements) SetRowsCurrency(ctx context.Context, externalID vo.Id, typ model.ElementType, currencyID vo.Id) error {
	return a.budget.SetElementsCurrency(ctx, externalID, typ, currencyID)
}

type debtRowPlans interface {
	DebtRowPlans(ctx context.Context, userID, budgetID, debtID vo.Id) (model.DebtBudgetPlans, bool, error)
}

type debtPlanUsers interface {
	GetByID(ctx context.Context, id vo.Id) (*model.User, error)
}

// DebtBudgetPlans gives the debt forecast a budget's plans of the debt's row,
// and the user's active budget from their options.
type DebtBudgetPlans struct {
	budget debtRowPlans
	users  debtPlanUsers
}

func NewDebtBudgetPlans(budget debtRowPlans, users debtPlanUsers) *DebtBudgetPlans {
	return &DebtBudgetPlans{budget: budget, users: users}
}

func (a *DebtBudgetPlans) ActiveBudget(ctx context.Context, userID vo.Id) (*vo.Id, error) {
	u, err := a.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	o := u.Option(model.OptionBudget)
	if o == nil || o.Value == nil || *o.Value == "" {
		return nil, nil
	}
	id, err := vo.ParseId(*o.Value)
	if err != nil {
		// a stale or malformed option names no budget
		return nil, nil
	}
	return &id, nil
}

func (a *DebtBudgetPlans) DebtPlans(ctx context.Context, userID, budgetID, debtID vo.Id) (model.DebtBudgetPlans, bool, error) {
	return a.budget.DebtRowPlans(ctx, userID, budgetID, debtID)
}
