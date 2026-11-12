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
	_ debt.Budgets         = (*DebtBudgets)(nil)
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

type debtBudgetAccess interface {
	DebtBudget(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error)
	DebtBudgets(ctx context.Context, userID vo.Id) ([]model.DebtBudget, error)
	DebtScope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error)
	DebtRuleOptions(ctx context.Context, viewerID, budgetID vo.Id) (*model.DebtRuleOptionsResult, error)
	DebtRowPlans(ctx context.Context, budgetID, debtID vo.Id) (model.DebtBudgetPlans, bool, error)
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

func (a *DebtBudgets) RuleOptions(ctx context.Context, viewerID, budgetID vo.Id) (*model.DebtRuleOptionsResult, error) {
	return a.budget.DebtRuleOptions(ctx, viewerID, budgetID)
}

func (a *DebtBudgets) DebtPlans(ctx context.Context, budgetID, debtID vo.Id) (model.DebtBudgetPlans, bool, error) {
	return a.budget.DebtRowPlans(ctx, budgetID, debtID)
}
