package budget_test

import (
	"net/http"
	"testing"
)

// TestTagWithLimitButNoTransactions_StaysVisible is the regression for a
// reported bug: a tag given a budget limit for a period, but with no
// transactions in (or before) that period, disappeared from get-budget. The
// structure builder gated tags on having a spending entry (transactions only)
// before ever considering the limit, so removing a tag's last transaction made
// its budget vanish. A tag must show when it has a limit even with zero spending
// (the rule is transactions OR budget OR non-zero available).
func TestTagWithLimitButNoTransactions_StaysVisible(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Tag Limit Budget"))

	// Set a limit on the tag — and deliberately create NO transaction for it.
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": tagID, "period": "2099-01-01", "amount": "500",
	})
	if st != http.StatusOK {
		t.Fatalf("set-limit=%d body=%s", st, env.raw)
	}

	status, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2099-01-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, b.raw)
	}
	type budgetView struct {
		Item struct {
			Structure struct {
				Elements []struct {
					Id        string `json:"id"`
					Budgeted  string `json:"budgeted"`
					Available string `json:"available"`
				} `json:"elements"`
			} `json:"structure"`
		} `json:"item"`
	}
	res := mustUnmarshal[budgetView](t, b.Data)

	var found bool
	var budgeted, available string
	for _, e := range res.Item.Structure.Elements {
		if e.Id == tagID {
			found, budgeted, available = true, e.Budgeted, e.Available
		}
	}
	if !found {
		t.Fatalf("tag with a limit but no transactions must stay visible in get-budget; elements: %s", b.Data)
	}
	if budgeted != "500" {
		t.Errorf("tag budgeted=%q want 500", budgeted)
	}
	// available = budgetedBefore - spentBefore - spent (carryover semantics,
	// matching PHP). The budget starts this month so there is no carry-in, and
	// nothing was spent, so available is "0" — the current month's allocation is
	// reported in `budgeted`, not `available`.
	if available != "0" {
		t.Errorf("tag available=%q want 0 (no carryover, nothing spent)", available)
	}
}
