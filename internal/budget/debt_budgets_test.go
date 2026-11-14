package budget_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// secondBudget is another budget of the harness's owner over the same two
// accounts, from April 2026.
func (h *attrHarness) secondBudget(t *testing.T) string {
	t.Helper()
	started := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	id := h.f.Budget(fixture.Budget{UserID: h.owner.String(), Name: "Second", StartedAt: started})
	h.f.BudgetAccount(id, h.usd)
	h.f.BudgetAccount(id, h.eur)
	return id
}

func (h *attrHarness) budgetIn(t *testing.T, budgetID, date string) *model.GetBudgetResult {
	t.Helper()
	res, err := h.svc.GetBudget(h.ctx, h.owner, model.GetBudgetRequest{Id: budgetID, Date: date})
	if err != nil {
		t.Fatalf("GetBudget %s: %v", budgetID, err)
	}
	return res
}

func (h *attrHarness) wholeDebtIn(t *testing.T, budgetID, name string, categoryID string) string {
	t.Helper()
	res, err := createDebtAs(h, h.owner, debtSpec{
		BudgetId: budgetID, ElementId: categoryID,
		Payment: vo.NewFlexString("1000"), Schedule: "monthly",
	})
	if err != nil {
		t.Fatalf("CreateDebt in %s: %v", budgetID, err)
	}
	return res.Id
}

// rowsSpent is every expense the budget's structure shows for the month:
// element rows plus debt rows.
func rowsSpent(res *model.GetBudgetResult) vo.DecimalNumber {
	sum := vo.NewDecimal("0")
	for _, el := range res.Item.Structure.Elements {
		sum = sum.Add(vo.NewDecimal(el.BudgetSpent))
	}
	for _, d := range res.Item.Structure.Debts {
		sum = sum.Add(vo.NewDecimal(d.BudgetSpent))
	}
	return sum
}

func TestGetBudget_DebtRowOnlyInItsOwnBudget(t *testing.T) {
	h := newAttrHarness(t)
	second := h.secondBudget(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createDebt(t)

	if row := debtRow(h.budgetIn(t, h.budgetID, "2026-06-01"), debtID); row == nil || row.Spent != "1000" {
		t.Fatalf("own budget's debt row = %+v, want 1000 paid", row)
	}
	res := h.budgetIn(t, second, "2026-06-01")
	if len(res.Item.Structure.Debts) != 0 {
		t.Errorf("the other budget shows debt rows: %+v", res.Item.Structure.Debts)
	}
	if got := spentOf(t, res, h.loan, model.ElementCategory); got != "1000" {
		t.Errorf("the other budget's loan row = %s, want the payment there", got)
	}
	plan, err := h.svc.GetBudgetPlan(h.ctx, h.owner, model.GetBudgetPlanRequest{Id: second, From: "2026-06-01", Months: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Item.Structure.Debts) != 0 {
		t.Errorf("the other budget's plan shows debt rows: %+v", plan.Item.Structure.Debts)
	}
}

// Review Focus 1: an account in two budgets, each with a debt matching the
// same payment. Each budget counts it once, on its own debt row.
func TestGetBudget_PaymentMatchedInTwoBudgetsCountsOnceInEach(t *testing.T) {
	h := newAttrHarness(t)
	second := h.secondBudget(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, "", "50", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	debts := map[string]string{
		h.budgetID: h.wholeDebtIn(t, h.budgetID, "Mortgage", h.loan),
		second:     h.wholeDebtIn(t, second, "Mortgage", h.loan),
	}

	for budgetID, own := range debts {
		res := h.budgetIn(t, budgetID, "2026-06-01")
		if len(res.Item.Structure.Debts) != 1 {
			t.Fatalf("budget %s debt rows = %+v, want only its own", budgetID, res.Item.Structure.Debts)
		}
		if row := debtRow(res, own); row == nil || row.Spent != "1000" || row.BudgetSpent != "1000" {
			t.Errorf("budget %s debt row = %+v, want 1000 once", budgetID, row)
		}
		if got := spentOf(t, res, h.loan, model.ElementCategory); got != "0" {
			t.Errorf("budget %s loan row = %s, want 0", budgetID, got)
		}
		if got := rowsSpent(res); got.String() != "1050" {
			t.Errorf("budget %s rows add up to %s, want 1050", budgetID, got)
		}

		plan, err := h.svc.GetBudgetPlan(h.ctx, h.owner, model.GetBudgetPlanRequest{Id: budgetID, From: "2026-06-01", Months: "1"})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Item.Structure.Debts) != 1 {
			t.Fatalf("budget %s plan debt rows = %+v", budgetID, plan.Item.Structure.Debts)
		}
		if row := planDebtRow(plan, own); row == nil || row.Cells[0].Actual != "1000" {
			t.Errorf("budget %s plan debt row = %+v, want 1000", budgetID, row)
		}
		for _, el := range plan.Item.Structure.Elements {
			if el.Id == h.loan && el.Cells[0].Actual != "0" {
				t.Errorf("budget %s plan loan actual = %s, want 0", budgetID, el.Cells[0].Actual)
			}
		}
	}
}

func TestRevokeAccess_KeepsTheBudgetsDebts(t *testing.T) {
	h := newAttrHarness(t)
	partner := h.f.User(fixture.User{Name: "Partner", Email: "partner@revoke.test"})
	h.f.BudgetAccess(h.budgetID, partner, int(model.BudgetRoleUser), true)
	partnerCash := h.f.Account(fixture.Account{UserID: partner, Name: "Her cash"})
	h.f.BudgetAccount(h.budgetID, partnerCash)
	herLoan := h.f.Category(fixture.Category{UserID: partner, Name: "Her loan"})
	h.f.Transaction(fixture.Transaction{UserID: partner, AccountID: partnerCash, CategoryID: herLoan,
		Type: 0, Amount: "500", SpentAt: time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)})
	debtID := h.wholeDebtIn(t, h.budgetID, "Her loan", herLoan)
	h.setLimit(t, debtID, "2026-06-01", "500")
	if row := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID); row == nil || row.Spent != "500" {
		t.Fatalf("debt row before = %+v, want her 500", row)
	}

	if _, err := h.svc.RevokeAccess(h.ctx, h.owner, model.RevokeAccessRequest{BudgetId: h.budgetID, UserId: partner}); err != nil {
		t.Fatalf("RevokeAccess: %v", err)
	}

	row := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if row == nil {
		t.Fatal("the debt row went with the partner")
	}
	if row.Spent != "0" || row.Budgeted != "500" {
		t.Errorf("debt row after = %+v, want her payment gone and the plan kept", row)
	}
	list, err := h.debts.GetDebtList(h.ctx, h.owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Id != debtID {
		t.Errorf("debts after = %+v, want the budget's debt kept", list.Items)
	}
}

func TestCloneBudget_CopiesDebtsAndTheirRows(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createDebt(t)
	h.setLimit(t, debtID, "2026-05-01", "700")
	h.setLimit(t, debtID, "2026-06-01", "1200")

	newID := vo.NewId().String()
	if _, err := h.svc.CloneBudget(h.ctx, h.owner, model.CloneBudgetRequest{
		Id: h.budgetID, NewId: newID, Name: "Copy", StartDate: "2026-06-01", WithLimits: true,
	}); err != nil {
		t.Fatalf("CloneBudget: %v", err)
	}

	var copyID string
	if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind("SELECT id FROM debts WHERE budget_id = ?"), newID).Scan(&copyID); err != nil {
		t.Fatalf("the copy's debt: %v", err)
	}
	if copyID == debtID {
		t.Fatal("the copy shares the source debt's id")
	}
	var onLoan int
	if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind("SELECT COUNT(*) FROM debts WHERE id = ? AND element_external_id = ?"), copyID, h.loan).Scan(&onLoan); err != nil || onLoan != 1 {
		t.Fatalf("the copy on the loan category = %d, %v; want it attached there", onLoan, err)
	}
	june := h.budgetIn(t, newID, "2026-06-01")
	if len(june.Item.Structure.Debts) != 1 {
		t.Fatalf("the copy's debt rows = %+v", june.Item.Structure.Debts)
	}
	row := debtRow(june, copyID)
	if row == nil || row.Budgeted != "1200" || row.Spent != "1000" {
		t.Errorf("the copy's June row = %+v, want the June plan and the payment", row)
	}
	var mayPlans int
	if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind(
		"SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ? AND e.external_id = ? AND l.period < ?"),
		newID, copyID, "2026-06-01").Scan(&mayPlans); err != nil || mayPlans != 0 {
		t.Errorf("the copy holds %d plans before its start, %v", mayPlans, err)
	}
	if row := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID); row == nil || row.Budgeted != "1200" {
		t.Errorf("the source row = %+v, want it untouched", row)
	}
}

func TestDeleteBudget_DeletesItsDebts(t *testing.T) {
	h := newAttrHarness(t)
	second := h.secondBudget(t)
	h.createDebt(t)
	kept := h.wholeDebtIn(t, second, "Other", h.food)

	if _, err := h.svc.DeleteBudget(h.ctx, h.owner, model.DeleteBudgetRequest{Id: h.budgetID}); err != nil {
		t.Fatalf("DeleteBudget: %v", err)
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := h.tdb.Raw.QueryRowContext(h.ctx, h.tdb.Rebind(query), args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count("SELECT COUNT(*) FROM debts WHERE budget_id = ?", h.budgetID); n != 0 {
		t.Errorf("%d debts of the deleted budget left", n)
	}
	if n := count("SELECT COUNT(*) FROM debts WHERE id = ?", kept); n != 1 {
		t.Error("the other budget's debt went too")
	}
}

// Onboarding's category history reads the spending once per payment, however
// many budgets' debts split it.
func TestCategoryHistory_PaymentSplitInTwoBudgetsCountsOnce(t *testing.T) {
	h := newAttrHarness(t)
	second := h.secondBudget(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	h.wholeDebtIn(t, h.budgetID, "Mortgage", h.loan)
	h.wholeDebtIn(t, second, "Mortgage", h.loan)
	h.budgetIn(t, h.budgetID, "2026-06-01")
	h.budgetIn(t, second, "2026-06-01")

	items, err := h.svc.CategoryHistory(h.ctx, h.owner, vo.MustParseId(h.budgetID), 3)
	if err != nil {
		t.Fatalf("CategoryHistory: %v", err)
	}
	for _, it := range items {
		if it.CategoryID == h.loan {
			if got := it.Amounts[2]; vo.NewDecimal(got).String() != "1000" {
				t.Errorf("loan May = %q, want 1000 once (amounts %v)", got, it.Amounts)
			}
			return
		}
	}
	t.Fatalf("no history for the loan category: %+v", items)
}
