package budget_test

import (
	"net/http"
	"testing"
)

// TestSetLimit_RoundTripsToGetBudget is the regression for a real bug the test
// suite caught: a limit SET via POST /budget/set-limit must subsequently show up
// in GET /budget/get-budget's element `budgeted`. The sqlite SaveLimit adapter
// originally bound `period` as a time.Time, which modernc serializes as RFC3339
// ("2099-01-01T00:00:00Z") — SQLite's datetime() can't parse that, so the read
// side's `datetime(period)=datetime(?)` never matched and the just-set limit
// read back as budgeted="0" (the limit was silently ignored). The adapter now
// stores period as a "Y-m-d H:i:s" string, matching the read side.
//
// The period is in the future (>= the budget's startedAt, which set-limit
// validates) so the limit is accepted; the get-budget date falls in that month.
func TestSetLimit_RoundTripsToGetBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "RT Budget"))

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01", "amount": "1700",
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
					Id       string `json:"id"`
					Budgeted string `json:"budgeted"`
				} `json:"elements"`
			} `json:"structure"`
		} `json:"item"`
	}
	res := mustUnmarshal[budgetView](t, b.Data)
	var got string
	for _, e := range res.Item.Structure.Elements {
		if e.Id == catID {
			got = e.Budgeted
		}
	}
	if got != "1700" {
		t.Fatalf("Food budgeted=%q want 1700 (set-limit must round-trip to get-budget)", got)
	}
}
