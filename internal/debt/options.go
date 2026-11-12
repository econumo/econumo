package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetDebtRuleOptions lists what a rule of a debt in the budget can name: each
// participant's active expense categories, payees, tags and labels. It is the
// only way a participant sees a partner's classifications, so it needs the
// right to plan in the budget.
func (s *Service) GetDebtRuleOptions(ctx context.Context, userID vo.Id, req model.GetDebtRuleOptionsRequest) (*model.DebtRuleOptionsResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, err
	}
	b, err := s.access(ctx, userID, budgetID)
	if err != nil {
		return nil, err
	}
	if !b.CanPlan {
		return nil, checkPlan(b)
	}
	return s.budgets.RuleOptions(ctx, userID, budgetID)
}
