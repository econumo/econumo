package server

import (
	"context"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type debtBudgetElements interface {
	TransferElementData(ctx context.Context, fromID vo.Id, fromType model.ElementType, toID vo.Id, toType model.ElementType, currencyID vo.Id) (bool, error)
	ElementHasData(ctx context.Context, externalID vo.Id, typ model.ElementType) (bool, error)
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

func (a *DebtBudgetElements) TransferData(ctx context.Context, fromID vo.Id, fromType int16, toID vo.Id, toType int16, currencyID vo.Id) (bool, error) {
	return a.budget.TransferElementData(ctx, fromID, model.ElementType(fromType), toID, model.ElementType(toType), currencyID)
}

func (a *DebtBudgetElements) HasData(ctx context.Context, externalID vo.Id, typ int16) (bool, error) {
	return a.budget.ElementHasData(ctx, externalID, model.ElementType(typ))
}
