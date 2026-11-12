package server_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// rowPlans are the plans of a budget's row by period ("2006-01").
func (e *onbEnv) rowPlans(budget, externalID string) map[string]string {
	e.t.Helper()
	rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT l.period, l.amount FROM budgets_elements_limits l
		JOIN budgets_elements el ON el.id = l.element_id WHERE el.budget_id = ? AND el.external_id = ?`), budget, externalID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var period time.Time
		var amount string
		if err := rows.Scan(&period, &amount); err != nil {
			e.t.Fatal(err)
		}
		out[period.Format("2006-01")] = vo.NewDecimal(amount).String()
	}
	return out
}

func (e *onbEnv) rowComments(budget, externalID string) map[string]bool {
	e.t.Helper()
	rows, err := e.db.Raw.Query(e.db.Rebind(`SELECT c.period FROM budgets_elements_comments c
		JOIN budgets_elements el ON el.id = c.element_id WHERE el.budget_id = ? AND el.external_id = ?`), budget, externalID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var period time.Time
		if err := rows.Scan(&period); err != nil {
			e.t.Fatal(err)
		}
		out[period.Format("2006-01")] = true
	}
	return out
}

// Finishing the setup of a whole-category debt in the same save that changes
// its currency and moves its start later hands both the months before the new
// start and the months after its end back to the category, in the currency the
// rows were in; the months in between stay on the debt row, which then takes
// the new currency.
func TestUpdateDebt_SetupWithCurrencyAndLaterStartMovesHeadAndTail(t *testing.T) {
	e := newOnbEnv(t, time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))
	budget := e.f.Budget(fixture.Budget{UserID: e.user, StartedAt: "2026-01-01 00:00:00"})
	category := e.f.Category(fixture.Category{UserID: e.user, Name: "Car loan"})

	status, data := e.post("/api/v1/debt/create-debt", map[string]any{
		"budgetId": budget, "name": "Car loan", "rules": []map[string]any{{"categoryId": category}},
		"startDate": "2026-03-01", "payment": "500", "schedule": "monthly",
	})
	if status != 200 {
		t.Fatalf("create-debt %d: %v", status, data)
	}
	debt, _ := data["id"].(string)
	if debt == "" {
		t.Fatalf("create-debt: %v", data)
	}
	months := []string{"2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2027-02"}
	for _, m := range months {
		if status, data := e.post("/api/v1/budget/set-limit", map[string]any{
			"budgetId": budget, "elementId": debt, "period": m + "-01", "amount": "100",
		}); status != 200 {
			t.Fatalf("set-limit %s %d: %v", m, status, data)
		}
	}
	for _, m := range []string{"2026-03", "2026-07", "2027-02"} {
		if status, data := e.post("/api/v1/budget/create-comment", map[string]any{
			"id": fixture.NewID(), "budgetId": budget, "elementId": debt, "period": m + "-01", "comment": "note " + m,
		}); status != 200 {
			t.Fatalf("create-comment %s %d: %v", m, status, data)
		}
	}

	status, data = e.post("/api/v1/debt/update-debt", map[string]any{
		"id": debt, "name": "Car loan", "currencyId": e.eur,
		"amountBorrowed": "3000", "startDate": "2026-06-10", "lastPaymentDate": "2026-12-10",
		"payment": "500", "schedule": "monthly",
	})
	if status != 200 {
		t.Fatalf("update-debt %d: %v", status, data)
	}

	hundred := vo.NewDecimal("100").String()
	wantCategory := map[string]string{"2026-03": hundred, "2026-04": hundred, "2026-05": hundred, "2027-02": hundred}
	if got := e.rowPlans(budget, category); !sameStrings(got, wantCategory) {
		t.Errorf("category plans = %v, want %v", got, wantCategory)
	}
	wantDebt := map[string]string{"2026-06": hundred, "2026-07": hundred}
	if got := e.rowPlans(budget, debt); !sameStrings(got, wantDebt) {
		t.Errorf("debt plans = %v, want %v", got, wantDebt)
	}
	if got := e.rowComments(budget, category); len(got) != 2 || !got["2026-03"] || !got["2027-02"] {
		t.Errorf("category comments = %v, want March and February", got)
	}
	if got := e.rowComments(budget, debt); len(got) != 1 || !got["2026-07"] {
		t.Errorf("debt comments = %v, want July", got)
	}
	if got := e.str(`SELECT currency_id FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budget, debt); got != e.eur {
		t.Errorf("debt row currency = %s, want EUR", got)
	}
}

func sameStrings(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
