package onboarding_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/onboarding"
	"github.com/econumo/econumo/internal/shared/vo"
)

const otherBudget = "019a0000-0000-7000-8000-0000000000b1"

type fakeDebtBudgets struct{ onboarding.BudgetSteps }

func (fakeDebtBudgets) CategoryHistory(context.Context, vo.Id, vo.Id, int) ([]model.BudgetCategoryHistory, error) {
	return []model.BudgetCategoryHistory{{CategoryID: cash, CurrencyID: "019a0000-0000-7000-8000-0000000000c1"}}, nil
}

type fakeDebtCurrencies struct{ onboarding.CurrencySteps }

func (fakeDebtCurrencies) FractionDigits(context.Context, string) (int, error) { return 2, nil }

// newDebtSvc answers the debt detection with a debt for the loan category.
func newDebtSvc(debts *fakeDebts) *onboarding.Service {
	svc, _ := newSvc(replying(`{"debts":[{"categoryId":"`+cash+`","name":"Loan"}]}`), nil)
	svc.SetSteps(onboarding.Steps{Categories: fakeCategorySteps{}, Budgets: fakeDebtBudgets{}, Currencies: fakeDebtCurrencies{}})
	svc.SetDebts(debts)
	return svc
}

func TestDetectDebts_CreatesInTheGivenBudget(t *testing.T) {
	debts := &fakeDebts{}
	newDebtSvc(debts).StartPlanDebts(context.Background(), vo.MustParseId(me), budgetID, map[string]vo.DecimalNumber{cash: vo.NewDecimal("450")})
	if len(debts.created) != 1 || debts.created[0].String() != budgetID {
		t.Fatalf("created in %v", debts.created)
	}
}

func TestStartPlanDebts_TakesPlansInThePlannedBudget(t *testing.T) {
	debts := &fakeDebts{covered: ids(cash)}
	newDebtSvc(debts).StartPlanDebts(context.Background(), vo.MustParseId(me), budgetID, map[string]vo.DecimalNumber{cash: vo.NewDecimal("450")})
	if len(debts.takenIn) != 1 || debts.takenIn[0].String() != budgetID || len(debts.created) != 0 {
		t.Fatalf("taken in %v, created %v", debts.takenIn, debts.created)
	}
}

// A debt of another budget covering the category does not keep it from being
// asked about here.
func TestDetectDebts_CoveredIsPerBudget(t *testing.T) {
	debts := &fakeDebts{covered: ids(cash), coveredIn: otherBudget}
	newDebtSvc(debts).StartPlanDebts(context.Background(), vo.MustParseId(me), budgetID, map[string]vo.DecimalNumber{cash: vo.NewDecimal("450")})
	if len(debts.created) != 1 || len(debts.taken) != 0 {
		t.Fatalf("created %v, taken %v", debts.created, debts.taken)
	}
}
