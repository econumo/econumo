package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
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

// periodsOut reads the filters block's period window.
type periodsOut struct {
	Item struct {
		Filters struct {
			PeriodStart string `json:"periodStart"`
			PeriodEnd   string `json:"periodEnd"`
		} `json:"filters"`
	} `json:"item"`
}

// TestEndedBudget_Clamps pins spec §2: readers snap a requested period later
// than the end month down to it, and set-limit refuses periods past it.
func TestEndedBudget_Clamps(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust()) // "now" = 2026-08-17
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "2026-05-01"})

	// get-budget for August clamps down to the end month
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-15", tok, nil)
	fl := mustUnmarshal[periodsOut](t, env.Data)
	if fl.Item.Filters.PeriodStart != "2026-05-01 00:00:00" {
		t.Fatalf("periodStart=%q want clamped to 2026-05-01", fl.Item.Filters.PeriodStart)
	}
	// a period inside the range is untouched
	env = h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-03-15", tok, nil)
	fl = mustUnmarshal[periodsOut](t, env.Data)
	if fl.Item.Filters.PeriodStart != "2026-03-01 00:00:00" {
		t.Fatalf("in-range periodStart=%q want 2026-03-01", fl.Item.Filters.PeriodStart)
	}

	// get-transaction-list clamps the same way
	env = h.mustDo(t, http.MethodGet,
		"/api/v1/budget/get-transaction-list?budgetId="+budgetID1+"&periodStart=2026-08-01&categoryId="+catID, tok, nil)
	_ = env // the clamp is asserted through get-budget; this pins that the call stays 200

	// set-limit AT the end month is fine; after it is a validation error
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
		map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-05-01", "amount": "10"})
	st, env2 := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
		map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-06-01", "amount": "10"})
	if st != http.StatusBadRequest {
		t.Fatalf("set-limit past the end month: status=%d body=%s want 400", st, env2.raw)
	}
}

// TestEndedBudget_RemovalBoundary pins spec §2: the membership removal window
// ends at min(current month, end month + 1), so a transaction in a month after
// the end date no longer locks the account.
func TestEndedBudget_RemovalBoundary(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	// spend in July — inside the budget, but AFTER the end month (May)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "10", SpentAt: "2026-07-10 12:00:00"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}})

	// without an end date, July is a closed month → the account is locked
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-15", tok, nil)
	acc := mustUnmarshal[filtersOut](t, env.Data)
	if len(acc.Item.Filters.Accounts) != 1 || acc.Item.Filters.Accounts[0].Removable {
		t.Fatalf("pre-end: accounts=%+v want the member locked by July spending", acc.Item.Filters.Accounts)
	}

	// ending the budget in May pulls the window back past the July transaction
	h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "2026-05-01"})
	env = h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-04-15", tok, nil)
	acc = mustUnmarshal[filtersOut](t, env.Data)
	if len(acc.Item.Filters.Accounts) != 1 || !acc.Item.Filters.Accounts[0].Removable {
		t.Fatalf("post-end: accounts=%+v want removable (July is outside the ended budget)", acc.Item.Filters.Accounts)
	}
}
