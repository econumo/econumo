package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// TestSetLimit_TagCreatedAfterBudget is the regression for a reported production
// bug: budget_elements rows are seeded at create-budget and backfilled by
// restoreElementsOrder (move / envelope mutations) only, so a tag created AFTER
// the budget had no element row and set-limit failed with "BudgetElement not
// found" — even though get-budget happily shows such a tag (its visibility is
// computed from spending/limits, not from the element row). set-limit must
// self-heal the missing row instead.
func TestSetLimit_TagCreatedAfterBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Later Tag Budget"))

	// A tag created after the budget: no budget_elements row exists for it.
	const laterTagID = "dddd2222-0000-7000-8000-000000000002"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Tag(fixture.Tag{ID: laterTagID, UserID: seedUserID, Name: "Later Tag"})

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": laterTagID, "period": "2099-01-01", "amount": "184.18",
	})
	if st != http.StatusOK {
		t.Fatalf("set-limit on a tag created after the budget = %d, want 200; body=%s", st, env.raw)
	}

	// The limit must round-trip into get-budget.
	status, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2099-01-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, b.raw)
	}
	budgeted, _, ok := mustUnmarshal[tagBudgetView](t, b.Data).findElement(laterTagID)
	if !ok {
		t.Fatalf("tag with a fresh limit must be visible in get-budget; body: %s", b.Data)
	}
	if budgeted != "184.18" {
		t.Errorf("tag budgeted=%q want 184.18", budgeted)
	}
}

// TestSetLimit_CategoryCreatedAfterBudget: same self-heal for a category created
// after the budget.
func TestSetLimit_CategoryCreatedAfterBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Later Cat Budget"))

	const laterCatID = "cccc2222-0000-7000-8000-000000000002"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: laterCatID, UserID: seedUserID, Name: "Later Cat", Type: 0, Icon: "local_offer"})

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": laterCatID, "period": "2099-01-01", "amount": "50",
	})
	if st != http.StatusOK {
		t.Fatalf("set-limit on a category created after the budget = %d, want 200; body=%s", st, env.raw)
	}
}

// TestChangeElementCurrency_TagCreatedAfterBudget: change-element-currency
// resolves elements the same way and must self-heal too.
func TestChangeElementCurrency_TagCreatedAfterBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Later Tag Currency Budget"))

	const laterTagID = "dddd2222-0000-7000-8000-000000000003"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Tag(fixture.Tag{ID: laterTagID, UserID: seedUserID, Name: "Later Tag"})

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/change-element-currency", tok, map[string]any{
		"budgetId": budgetID1, "elementId": laterTagID, "currencyId": usdID,
	})
	if st != http.StatusOK {
		t.Fatalf("change-element-currency on a tag created after the budget = %d, want 200; body=%s", st, env.raw)
	}
}

// TestSetLimit_UnknownElement_StillNotFound guards the frozen error contract:
// the self-heal must not turn a genuinely unknown element id into anything other
// than the "BudgetElement not found" 400.
func TestSetLimit_UnknownElement_StillNotFound(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Unknown Element Budget"))

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": "9999aaaa-0000-7000-8000-00000000dead", "period": "2099-01-01", "amount": "10",
	})
	if st != http.StatusBadRequest {
		t.Fatalf("set-limit on an unknown element = %d, want 400; body=%s", st, env.raw)
	}
	if env.Message != "BudgetElement not found" {
		t.Errorf("message=%q want %q", env.Message, "BudgetElement not found")
	}
}
