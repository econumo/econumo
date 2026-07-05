package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// TestGetBudget_SpendingIncludesFirstOfMonth is the regression for the
// api-compare finding that the budget spending query dropped the
// first-of-month transaction: it bound the spent_at range bounds as time.Time,
// which the SQLite driver serialized in a form that does not compare equal to
// the stored datetime TEXT at the month boundary, excluding the row. The query
// now binds 'Y-m-d H:i:s' strings. This seeds a transaction dated exactly on the
// period's first day and asserts it is counted in the element's spent.
func TestGetBudget_SpendingIncludesFirstOfMonth(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "FoM Budget")); st != 200 {
		t.Fatalf("create=%d body=%s", st, e.raw)
	}
	// An expense (type=0) dated EXACTLY on 2025-04-01 00:00:00 (the period start),
	// categorized to the seeded Food category on the seeded account.
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{
		ID: "dddd1111-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "42.00", SpentAt: "2025-04-01 00:00:00",
	})

	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2025-04-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}
	type budgetView struct {
		Item struct {
			Structure struct {
				Elements []struct {
					Id    string `json:"id"`
					Spent string `json:"spent"`
				} `json:"elements"`
			} `json:"structure"`
		} `json:"item"`
	}
	res := mustUnmarshal[budgetView](t, env.Data)
	var foodSpent string
	for _, e := range res.Item.Structure.Elements {
		if e.Id == catID {
			foodSpent = e.Spent
		}
	}
	if foodSpent != "42" {
		t.Fatalf("Food spent=%q want 42 (first-of-month transaction must be counted)", foodSpent)
	}
}

// TestGetBudget_ElementBudgetedFromLimit is the regression for the api-compare
// finding that every element's `budgeted` was "0": the SQLite limits query
// compared a bound time.Time against the stored datetime TEXT with raw "=",
// which never matched (the period is stored as datetime text, not the driver's
// time serialization), so ListLimitsForPeriod returned zero rows. The query now
// uses datetime(period)=datetime(?) with a 'Y-m-d H:i:s' bound. This test seeds a
// limit and asserts the element's `budgeted` reflects it.
func TestGetBudget_ElementBudgetedFromLimit(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ctx := context.Background()

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Limit Budget")); st != 200 {
		t.Fatalf("create=%d body=%s", st, e.raw)
	}

	// Find the budget element created for the seeded Food category (external_id =
	// the category id), then seed a 1700 limit for the April-2025 period. The
	// period is stored as "Y-m-d H:i:s" text.
	var elementID string
	// category element type = 1 (envelope=0, category=1, tag=2).
	if err := h.db.QueryRowContext(ctx,
		`SELECT id FROM budgets_elements WHERE external_id = ? AND type = 1`, catID).Scan(&elementID); err != nil {
		t.Fatalf("find budget element for category: %v", err)
	}
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.BudgetLimit(fixture.BudgetLimit{
		ID: "cccc1111-0000-7000-8000-000000000001", ElementID: elementID,
		Period: "2025-04-01 00:00:00", Amount: "1700.00",
	})

	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2025-04-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
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
	res := mustUnmarshal[budgetView](t, env.Data)

	var foodBudgeted string
	var found bool
	for _, e := range res.Item.Structure.Elements {
		if e.Id == catID {
			foodBudgeted = e.Budgeted
			found = true
		}
	}
	if !found {
		t.Fatalf("Food element not in structure: %+v", res.Item.Structure.Elements)
	}
	if foodBudgeted != "1700" {
		t.Fatalf("Food budgeted=%q want 1700 (limit must be loaded; period-format match)", foodBudgeted)
	}
}
