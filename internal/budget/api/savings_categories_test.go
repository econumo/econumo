package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/fixture"
)

// Income and expenses booked on a savings account belong to its Savings row
// only: the category rows, their drill-downs and the plan's income/expense
// cells count the everyday accounts.

const savingsSalaryCatID = "cccc1111-0000-7000-8000-0000000000a1"

func newSavingsCategoriesBudget(t *testing.T) (*harness, string) {
	t.Helper()
	h, tok, _ := newSavingsBudget(t)
	h.f.Category(fixture.Category{ID: savingsSalaryCatID, UserID: seedUserID, Name: "Salary", Type: 1, Icon: "payments"})
	at := func(day int) time.Time { return time.Date(2026, 8, day, 12, 0, 0, 0, time.UTC) }
	for _, tx := range []fixture.Transaction{
		{AccountID: accountID, CategoryID: catID, Type: 0, Amount: "40", SpentAt: at(10)},
		{AccountID: accountID, CategoryID: savingsSalaryCatID, Type: 1, Amount: "1000", SpentAt: at(10)},
		{AccountID: savingsUSDID, CategoryID: catID, Type: 0, Amount: "5", SpentAt: at(11)},
		{AccountID: savingsUSDID, CategoryID: savingsSalaryCatID, Type: 1, Amount: "12", SpentAt: at(11)},
		{AccountID: savingsUSDID, Type: 1, Amount: "3", SpentAt: at(12)},
		{AccountID: savingsUSDID, Type: 0, Amount: "1", SpentAt: at(12)},
	} {
		tx.UserID = seedUserID
		h.f.Transaction(tx)
	}
	return h, tok
}

func TestSavingsIncomeExpense_MonthlyCategoriesCountEverydayOnly(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-15", tok, nil)
	res := mustUnmarshal[model.GetBudgetResult](t, env.Data).Item

	spent := map[string]string{}
	for _, e := range res.Structure.Elements {
		spent[e.Id] = e.Spent
	}
	if !decEq(spent[catID], "40") {
		t.Errorf("category spent = %s, want 40 (the savings account's 5 left out)", spent[catID])
	}
	if s, ok := spent[model.UncategorizedID]; ok && !decEq(s, "0") {
		t.Errorf("uncategorized spent = %s, want 0 (the savings account's 1 left out)", s)
	}
	for _, s := range res.Structure.Savings {
		if s.Id == savingsUSDID && !decEq(s.Spent, "309") {
			t.Errorf("S1 saved = %s, want 309 (300 moved in + 12 + 3 - 5 - 1)", s.Spent)
		}
	}
}

func TestSavingsIncomeExpense_DrillDownsCountEverydayOnly(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	base := "/api/v1/budget/get-transaction-list?budgetId=" + budgetID1 + "&periodStart=2026-08-01"
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"&categoryId=" + catID, 1},
		{"&uncategorized=true", 0},
	} {
		env := h.mustDo(t, http.MethodGet, base+tc.query, tok, nil)
		if got := mustUnmarshal[txListView](t, env.Data); len(got.Items) != tc.want {
			t.Errorf("%s: %d items, want %d: %s", tc.query, len(got.Items), tc.want, env.Data)
		}
	}
}

func TestSavingsIncomeExpense_PlanCellsAndAddBack(t *testing.T) {
	h, tok := newSavingsCategoriesBudget(t)
	_, env := getPlan(t, h, tok, "id="+budgetID1+savingsPlanWindow)
	plan := planItem(t, env)

	actual := map[string]string{}
	for _, e := range plan.Structure.Elements {
		actual[e.Id] = e.Cells[1].Actual
	}
	if !decEq(actual[catID], "40") || !decEq(actual[savingsSalaryCatID], "1000") {
		t.Errorf("August actuals: category %s, salary %s; want 40, 1000", actual[catID], actual[savingsSalaryCatID])
	}
	for id, a := range actual {
		if id != catID && id != savingsSalaryCatID && !decEq(a, "0") && a != "" {
			t.Errorf("element %s August actual = %s, want nothing (the savings account's rows left out)", id, a)
		}
	}
	if s1 := planSavingsByID(toPlanSavingsRows(plan))[savingsUSDID]; !decEq(s1.Cells[1].Actual, "309") {
		t.Errorf("S1 August saved = %s, want 309", s1.Cells[1].Actual)
	}
	got := plan.SavingsIncomeExpense
	if len(got) != 1 || got[0].Month != "2026-08-01" || got[0].CurrencyId != usdID || !decEq(got[0].Amount, "9") {
		t.Errorf("savingsIncomeExpense = %+v, want one August USD entry of 9 (12 + 3 - 5 - 1)", got)
	}
}

func toPlanSavingsRows(plan model.BudgetPlanResult) []planSavingsRowView {
	out := make([]planSavingsRowView, 0, len(plan.Structure.Savings))
	for _, r := range plan.Structure.Savings {
		cells := make([]planSavingsCellView, 0, len(r.Cells))
		for _, c := range r.Cells {
			cells = append(cells, planSavingsCellView{Actual: c.Actual, Planned: c.Planned})
		}
		out = append(out, planSavingsRowView{Id: r.Id, Cells: cells})
	}
	return out
}
