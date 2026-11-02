package budget

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) SyncElementsForTest(ctx context.Context, budgetID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		return s.syncElements(ctx, budgetID, s.clock.Now())
	})
}

type DebtPaidForTest struct{ Paid, BudgetPaid, PaidBefore string }

func (s *Service) GetBudgetWithDebtsForTest(ctx context.Context, userID vo.Id, req model.GetBudgetRequest) (*model.GetBudgetResult, map[string]DebtPaidForTest, error) {
	res, debts, err := s.getBudget(ctx, userID, req)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]DebtPaidForTest{}
	for id, d := range debts {
		out[id] = DebtPaidForTest{Paid: d.paid.String(), BudgetPaid: d.budgetPaid.String(), PaidBefore: d.paidBefore.String()}
	}
	return res, out, nil
}

func (s *Service) GetBudgetPlanWithDebtsForTest(ctx context.Context, userID vo.Id, req model.GetBudgetPlanRequest) (*model.GetBudgetPlanResult, map[string][]string, error) {
	res, debts, err := s.getBudgetPlan(ctx, userID, req)
	if err != nil {
		return nil, nil, err
	}
	out := map[string][]string{}
	for id, months := range debts {
		for _, m := range months {
			out[id] = append(out[id], m.String())
		}
	}
	return res, out, nil
}
