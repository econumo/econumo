package onboarding_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/vo"
)

type fakeDebtBudgets struct{ onboarding.BudgetSteps }

func (fakeDebtBudgets) CategoryHistory(context.Context, vo.Id, vo.Id, int) ([]model.BudgetCategoryHistory, error) {
	return []model.BudgetCategoryHistory{{CategoryID: cash, CurrencyID: "019a0000-0000-7000-8000-0000000000c1"}}, nil
}

type fakeDebtCurrencies struct{ onboarding.CurrencySteps }

func (fakeDebtCurrencies) FractionDigits(context.Context, string) (int, error) { return 2, nil }

// newDebtSvc answers the debt detection with a debt for the loan category.
func newDebtSvc(debts *fakeDebts) *onboarding.Service {
	svc, _ := newSvc(replying(`{"debts":[{"categoryId":"`+cash+`"}]}`), nil)
	svc.SetSteps(onboarding.Steps{Categories: fakeCategorySteps{}, Budgets: fakeDebtBudgets{}, Currencies: fakeDebtCurrencies{}})
	svc.SetDebts(debts)
	return svc
}

// Each debt the detection finds is attached to its category in the budget
// the step works in.
func TestDetectDebts_AttachToTheCategory(t *testing.T) {
	debts := &fakeDebts{}
	newDebtSvc(debts).StartPlanDebts(context.Background(), vo.MustParseId(me), budgetID, map[string]vo.DecimalNumber{cash: vo.NewDecimal("450")})
	if len(debts.created) != 1 || debts.created[0].String() != budgetID || debts.categories[0].String() != cash {
		t.Fatalf("created in %v on %v", debts.created, debts.categories)
	}
}

// A category the debt feature refuses (it already carries an active debt)
// is skipped without failing the job.
func TestDetectDebts_SkipsARefusedCategory(t *testing.T) {
	debts := &fakeDebts{refused: ids(cash)}
	newDebtSvc(debts).StartPlanDebts(context.Background(), vo.MustParseId(me), budgetID, map[string]vo.DecimalNumber{cash: vo.NewDecimal("450")})
	if debts.calls != 1 || len(debts.created) != 0 {
		t.Fatalf("calls %d, created %v", debts.calls, debts.created)
	}
}

// A category whose element already carries a debt, even a paid-off one, is
// never asked about: with no other candidate the AI is not called at all.
func TestDetectDebts_SkipsACategoryWithADebt(t *testing.T) {
	debts := &fakeDebts{existing: ids(cash)}
	ai := replying(`{"debts":[{"categoryId":"` + cash + `"}]}`)
	svc, _ := newSvc(ai, nil)
	svc.SetSteps(onboarding.Steps{Categories: fakeCategorySteps{}, Budgets: fakeDebtBudgets{}, Currencies: fakeDebtCurrencies{}})
	svc.SetDebts(debts)
	svc.StartPlanDebts(context.Background(), vo.MustParseId(me), budgetID, map[string]vo.DecimalNumber{cash: vo.NewDecimal("450")})
	if ai.calls != 0 || debts.calls != 0 {
		t.Fatalf("AI calls %d, create calls %d; want neither", ai.calls, debts.calls)
	}
}
