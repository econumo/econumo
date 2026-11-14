package budget_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// secondBudget is another budget of the harness's owner over the same two
// accounts.
func (h *debtHarness) secondBudget(t *testing.T) string {
	t.Helper()
	id := h.f.Budget(fixture.Budget{UserID: h.owner.String(), Name: "Second", StartedAt: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)})
	h.f.BudgetAccount(id, h.usd)
	h.f.BudgetAccount(id, h.eur)
	return id
}

func (h *debtHarness) budgetIn(t *testing.T, budgetID, date string) *model.GetBudgetResult {
	t.Helper()
	res, err := h.svc.GetBudget(h.ctx, h.owner, model.GetBudgetRequest{Id: budgetID, Date: date})
	if err != nil {
		t.Fatalf("GetBudget %s: %v", budgetID, err)
	}
	return res
}

func (h *debtHarness) debtIn(t *testing.T, budgetID, elementID string) string {
	t.Helper()
	res, err := h.debts.CreateDebt(h.ctx, h.owner, model.CreateDebtRequest{
		BudgetId: budgetID, ElementId: elementID, Payment: vo.NewFlexString("1000"), Schedule: "monthly",
	})
	if err != nil {
		t.Fatalf("CreateDebt in %s: %v", budgetID, err)
	}
	return res.Id
}

// An account in two budgets, each with a debt on the same category: each
// budget shows its own debt on the category, which keeps the payment.
func TestGetBudget_EachBudgetShowsItsOwnDebt(t *testing.T) {
	h := newDebtHarness(t)
	second := h.secondBudget(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	own := h.debtIn(t, h.budgetID, h.loan)

	if el := findElement(h.budgetIn(t, second, "2026-06-01"), h.loan, model.ElementCategory); el == nil || el.Debt != nil || el.Spent != "1000" {
		t.Fatalf("the other budget's loan row = %+v, want the payment and no debt", el)
	}
	theirs := h.debtIn(t, second, h.loan)
	for budgetID, want := range map[string]string{h.budgetID: own, second: theirs} {
		el := findElement(h.budgetIn(t, budgetID, "2026-06-01"), h.loan, model.ElementCategory)
		if el == nil || debtIDOf(el.Debt) != want || el.Spent != "1000" {
			t.Errorf("budget %s loan row = %+v, want its own debt %s and the payment", budgetID, el, want)
		}
	}
}

// A participant who leaves takes their category element; the debt attached
// to it stays with the budget.
func TestRevokeAccess_KeepsTheBudgetsDebts(t *testing.T) {
	h := newDebtHarness(t)
	partner := h.f.User(fixture.User{Name: "Partner", Email: "partner@revoke.test"})
	h.f.BudgetAccess(h.budgetID, partner, int(model.BudgetRoleUser), true)
	partnerCash := h.f.Account(fixture.Account{UserID: partner, Name: "Her cash"})
	h.f.BudgetAccount(h.budgetID, partnerCash)
	herLoan := h.f.Category(fixture.Category{UserID: partner, Name: "Her loan"})
	h.f.Transaction(fixture.Transaction{UserID: partner, AccountID: partnerCash, CategoryID: herLoan,
		Type: 0, Amount: "500", SpentAt: time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)})
	debtID := h.debtIn(t, h.budgetID, herLoan)
	if el := findElement(h.budgetAt(t, "2026-06-01"), herLoan, model.ElementCategory); el == nil || debtIDOf(el.Debt) != debtID {
		t.Fatalf("her loan row before = %+v, want the debt on it", el)
	}

	if _, err := h.svc.RevokeAccess(h.ctx, h.owner, model.RevokeAccessRequest{BudgetId: h.budgetID, UserId: partner}); err != nil {
		t.Fatalf("RevokeAccess: %v", err)
	}
	if el := findElement(h.budgetAt(t, "2026-06-01"), herLoan, model.ElementCategory); el != nil {
		t.Errorf("her loan row after = %+v, want it gone with her", el)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE id = ?", debtID); n != 1 {
		t.Error("the debt went with the partner")
	}
}

func TestDeleteBudget_DeletesItsDebts(t *testing.T) {
	h := newDebtHarness(t)
	second := h.secondBudget(t)
	h.debtIn(t, h.budgetID, h.loan)
	kept := h.debtIn(t, second, h.food)

	if _, err := h.svc.DeleteBudget(h.ctx, h.owner, model.DeleteBudgetRequest{Id: h.budgetID}); err != nil {
		t.Fatalf("DeleteBudget: %v", err)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE budget_id = ?", h.budgetID); n != 0 {
		t.Errorf("%d debts of the deleted budget left", n)
	}
	if n := h.count(t, "SELECT COUNT(*) FROM debts WHERE id = ?", kept); n != 1 {
		t.Error("the other budget's debt went too")
	}
}

// Onboarding's category history reads the spending once per payment, however
// many budgets' debts split it.
func TestCategoryHistory_PaymentSplitInTwoBudgetsCountsOnce(t *testing.T) {
	h := newDebtHarness(t)
	second := h.secondBudget(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	h.debtIn(t, h.budgetID, h.loan)
	h.debtIn(t, second, h.loan)
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
