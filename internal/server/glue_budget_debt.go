package server

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type debtBudgetElements interface {
	DebtElement(ctx context.Context, budgetID, elementID vo.Id) (model.DebtElementInfo, error)
	DebtElementPlans(ctx context.Context, budgetID, elementID vo.Id) (map[string]float64, error)
}

// DebtElements adapts the budget service's method names to debt.Elements.
type DebtElements struct{ budget debtBudgetElements }

var (
	_ debt.Elements        = (*DebtElements)(nil)
	_ debt.Budgets         = (*DebtBudgets)(nil)
	_ appbudget.DebtSource = (*debt.Service)(nil)
)

func NewDebtElements(budget debtBudgetElements) *DebtElements { return &DebtElements{budget: budget} }

func (a *DebtElements) Element(ctx context.Context, budgetID, elementID vo.Id) (model.DebtElementInfo, error) {
	return a.budget.DebtElement(ctx, budgetID, elementID)
}

func (a *DebtElements) Plans(ctx context.Context, budgetID, elementID vo.Id) (map[string]float64, error) {
	return a.budget.DebtElementPlans(ctx, budgetID, elementID)
}

type debtBudgetAccess interface {
	DebtBudget(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error)
	DebtBudgets(ctx context.Context, userID vo.Id) ([]model.DebtBudget, error)
	DebtScope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error)
}

// DebtBudgets adapts the budget service's method names to debt.Budgets.
type DebtBudgets struct{ budget debtBudgetAccess }

func NewDebtBudgets(budget debtBudgetAccess) *DebtBudgets { return &DebtBudgets{budget: budget} }

func (a *DebtBudgets) Access(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error) {
	return a.budget.DebtBudget(ctx, userID, budgetID)
}

func (a *DebtBudgets) Readable(ctx context.Context, userID vo.Id) ([]model.DebtBudget, error) {
	return a.budget.DebtBudgets(ctx, userID)
}

func (a *DebtBudgets) Scope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error) {
	return a.budget.DebtScope(ctx, budgetID)
}
