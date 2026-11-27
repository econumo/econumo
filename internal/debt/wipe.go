package debt

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// BudgetIDsOfCategories names, without locking them, the budgets of the debts
// on any of the categories, in id order: the budgets a user wipe locks with
// LockBudget before it releases those categories.
func (s *Service) BudgetIDsOfCategories(ctx context.Context, ids ...vo.Id) ([]vo.Id, error) {
	budgets, err := s.budgetsOn(ctx, ids...)
	if err != nil {
		return nil, err
	}
	if budgets == nil {
		budgets = []vo.Id{}
	}
	return budgets, nil
}

// HoldsBudgetLock reports whether the caller's transaction took LockBudget
// on the budget, so a wipe can tell that the set of budgets did not grow
// after it locked them.
func (s *Service) HoldsBudgetLock(ctx context.Context, budgetID vo.Id) bool {
	return s.repo.HoldsBudgetLock(ctx, budgetID)
}
