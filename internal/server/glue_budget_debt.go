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
}

// DebtBudgetElements adapts the budget service to debt.BudgetElements; the
// debt feature names element types by their stored value.
type DebtBudgetElements struct{ budget debtBudgetElements }

var (
	_ debt.BudgetElements  = (*DebtBudgetElements)(nil)
	_ appbudget.DebtSource = (*debt.Service)(nil)
)

func NewDebtBudgetElements(budget debtBudgetElements) *DebtBudgetElements {
	return &DebtBudgetElements{budget: budget}
}

func (a *DebtBudgetElements) TransferData(ctx context.Context, t model.ElementDataTransfer) (bool, error) {
	return a.budget.TransferElementData(ctx, t)
}

func (a *DebtBudgetElements) LockRows(ctx context.Context, externalID vo.Id, typ int16) error {
	return a.budget.LockElements(ctx, externalID, model.ElementType(typ))
}

func (a *DebtBudgetElements) HasData(ctx context.Context, externalID vo.Id, typ int16) (bool, error) {
	return a.budget.ElementHasData(ctx, externalID, model.ElementType(typ))
}
