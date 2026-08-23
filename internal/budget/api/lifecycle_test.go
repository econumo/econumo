package api_test

import (
	"net/http"
	"testing"
)

// metaItemOut matches {item: MetaResult} responses (update-budget,
// archive-budget, unarchive-budget). Clone returns {item: BudgetResult},
// whose meta nests one level deeper — see metaOut in clone_test.go.
type metaItemOut struct {
	Item struct {
		EndedAt    string `json:"endedAt"`
		IsArchived int    `json:"isArchived"`
	} `json:"item"`
}

func TestUpdateBudget_EndDate(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Budget"))

	// absent endDate → untouched (still "")
	env := h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID})
	res := mustUnmarshal[metaItemOut](t, env.Data)
	if res.Item.EndedAt != "" || res.Item.IsArchived != 0 {
		t.Fatalf("meta=%+v want zero-value lifecycle fields", res.Item)
	}

	// set: a mid-month input snaps to first-of-month, full datetime on the wire
	env = h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "2030-06-15"})
	res = mustUnmarshal[metaItemOut](t, env.Data)
	if res.Item.EndedAt != "2030-06-01 00:00:00" {
		t.Fatalf("set: endedAt=%q want 2030-06-01 00:00:00", res.Item.EndedAt)
	}

	// before the budget start → coded 400 on endDate
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "1999-01-01"})
	if st != http.StatusBadRequest || !containsField(env, "endDate", "end month") {
		t.Fatalf("before-start: status=%d body=%s", st, env.raw)
	}

	// garbage → blank-validation on endDate
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "not-a-date"})
	if st != http.StatusBadRequest {
		t.Fatalf("garbage accepted: %s", env.raw)
	}

	// "" clears
	env = h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": ""})
	res = mustUnmarshal[metaItemOut](t, env.Data)
	if res.Item.EndedAt != "" {
		t.Fatalf("clear: endedAt=%q want empty", res.Item.EndedAt)
	}
}
