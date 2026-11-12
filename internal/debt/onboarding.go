package debt

import (
	"context"
	"slices"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CoveredCategories are the categories the budget's debts count payments in.
func (s *Service) CoveredCategories(ctx context.Context, budgetID vo.Id) ([]vo.Id, error) {
	debts, err := s.repo.ListByBudget(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	out := []vo.Id{}
	for _, d := range debts {
		for _, c := range ruleCategories(d.Rules) {
			if !slices.ContainsFunc(out, c.Equal) {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// TakeCategoryPlans hands the category's plans and comments in the budget to
// the budget's debt not set up yet with a whole-category rule on it, as
// creating that debt did: a plan made on the category after the debt appeared
// would otherwise sit on a second row. moved is false when there is no such
// debt.
func (s *Service) TakeCategoryPlans(ctx context.Context, budgetID, categoryID vo.Id) (moved bool, err error) {
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		moved = false
		if err := s.repo.LockBudget(ctx, budgetID); err != nil {
			return err
		}
		debts, err := s.repo.ListByBudget(ctx, budgetID)
		if err != nil {
			return err
		}
		for _, d := range debts {
			if d.IsSetUp() {
				continue
			}
			for _, r := range d.Rules {
				if r.CategoryID.Equal(categoryID) && model.IsWholeCategoryRule(d, r, debts) {
					moved = true
					return s.moveToDebt(ctx, d, categoryID)
				}
			}
		}
		return nil
	})
	return moved, err
}
