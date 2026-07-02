package budget_test

// Reproduction: creating a budget without an explicit start date must start it in
// the CALLER's current month (X-Timezone), not the server's UTC month. For a
// behind-UTC caller in the window where UTC has rolled into the next month but
// the caller's local date has not (e.g. June 30 evening in Vancouver == July 1
// early morning UTC), a UTC-derived default makes the budget start a month late.

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

type tzClock struct{ t time.Time }

func (c tzClock) Now() time.Time { return c.t }

type budgetTZResult struct {
	Item struct {
		Meta struct {
			Id        string `json:"id"`
			StartedAt string `json:"startedAt"`
		} `json:"meta"`
		Filters struct {
			PeriodStart string `json:"periodStart"`
		} `json:"filters"`
	} `json:"item"`
}

func TestCreateBudget_DefaultStartMonth_HonorsRequestTimezone(t *testing.T) {
	// 03:00 UTC on July 1st == 20:00 on June 30th in Vancouver: still June for
	// the caller, so the budget must start June 1st.
	clk := tzClock{t: time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC)}
	h := newHarnessWithClock(t, clk)
	tok := h.token(t)

	headers := map[string]string{"X-Timezone": "America/Vancouver"}
	status, env := h.doH(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		createBudgetReq(budgetID1, "June Budget"), headers)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[budgetTZResult](t, env.Data)
	if res.Item.Meta.StartedAt != "2026-06-01 00:00:00" {
		t.Fatalf("startedAt=%q want 2026-06-01 00:00:00 (caller's month, not UTC's)", res.Item.Meta.StartedAt)
	}
	// The initial build period must be the caller's current month too.
	if res.Item.Filters.PeriodStart != "2026-06-01 00:00:00" {
		t.Fatalf("periodStart=%q want 2026-06-01 00:00:00", res.Item.Filters.PeriodStart)
	}

	// The stored start date is the caller-local month as well (the driver's
	// serialization varies, so assert the date only).
	var stored string
	if err := h.db.QueryRow(`SELECT started_at FROM budgets WHERE id = ?`, budgetID1).Scan(&stored); err != nil {
		t.Fatalf("query started_at: %v", err)
	}
	if !strings.HasPrefix(stored, "2026-06-01") {
		t.Fatalf("stored started_at=%q want date 2026-06-01", stored)
	}
}

func TestGetBudget_NoDate_DefaultPeriodHonorsRequestTimezone(t *testing.T) {
	// Same boundary: July 1st 03:00 UTC is June 30th 20:00 in Vancouver, so a
	// get-budget without a date must default to the caller's June, not UTC's July.
	clk := tzClock{t: time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC)}
	h := newHarnessWithClock(t, clk)
	tok := h.token(t)

	headers := map[string]string{"X-Timezone": "America/Vancouver"}
	req := createBudgetReq(budgetID1, "June Budget")
	req["startDate"] = "2026-06-01"
	if st, e := h.doH(t, http.MethodPost, "/api/v1/budget/create-budget", tok, req, headers); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	status, env := h.doH(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[budgetTZResult](t, env.Data)
	if res.Item.Filters.PeriodStart != "2026-06-01 00:00:00" {
		t.Fatalf("periodStart=%q want 2026-06-01 00:00:00 (caller's month, not UTC's)", res.Item.Filters.PeriodStart)
	}
}
