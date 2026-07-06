package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// foodElementID returns the budgets_elements row id created for the seeded Food
// category (external_id = the category id, type = 1 for a category element).
func foodElementID(t *testing.T, h *harness) string {
	t.Helper()
	var id string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT id FROM budgets_elements WHERE external_id = ? AND type = 1`, catID).Scan(&id); err != nil {
		t.Fatalf("find budget element for category: %v", err)
	}
	return id
}

// foodElement reads the Food category element out of a get-budget response.
type elementView struct {
	Item struct {
		Structure struct {
			Elements []struct {
				Id        string `json:"id"`
				Budgeted  string `json:"budgeted"`
				Spent     string `json:"spent"`
				Available string `json:"available"`
			} `json:"elements"`
		} `json:"structure"`
	} `json:"item"`
}

// TestGetBudget_MultiPeriodCarryover exercises the carryover math end to end:
// `available` accumulates unspent budget from PRIOR periods, which no other
// budget test covers (they all use a single period, so their `available` is 0).
//
// The formula (see builder_structure_build.go) is
//
//	available = budgetedBefore - spentBefore - spent
//
// where the "before" terms sum every period from the budget's start month up to
// (not including) the viewed period, and the current-month allocation is
// reported separately in `budgeted` (NOT folded into available).
//
// Fixture — a single USD budget starting 2025-03, so conversion is identity:
//
//	          limit    spending
//	March     1000       700       (the prior period -> "before")
//	April     1200       500       (the viewed period)
//
// Viewing April: budgeted=1200, spent=500, budgetedBefore=1000, spentBefore=700,
// so available = 1000 - 700 - 500 = -200 (March's 300 surplus, minus April's 500
// overspend). Numbers are hand-derived from the inputs above, not read back from
// the code.
func TestGetBudget_MultiPeriodCarryover(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// startDate pins the budget's first month to March 2025 so the before-walk
	// runs March -> April; without it the budget starts in the current month and
	// no prior period exists.
	createReq := map[string]any{"id": budgetID1, "name": "Carryover Budget", "currencyId": usdID, "startDate": "2025-03-01"}
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createReq); st != 200 {
		t.Fatalf("create=%d body=%s", st, e.raw)
	}

	elementID := foodElementID(t, h)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.BudgetLimit(fixture.BudgetLimit{
		ID: "eeee1111-0000-7000-8000-000000000001", ElementID: elementID,
		Period: "2025-03-01 00:00:00", Amount: "1000.00",
	})
	f.BudgetLimit(fixture.BudgetLimit{
		ID: "eeee1111-0000-7000-8000-000000000002", ElementID: elementID,
		Period: "2025-04-01 00:00:00", Amount: "1200.00",
	})
	f.Transaction(fixture.Transaction{
		ID: "eeee2222-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "700.00", SpentAt: "2025-03-15 00:00:00",
	})
	f.Transaction(fixture.Transaction{
		ID: "eeee2222-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "500.00", SpentAt: "2025-04-10 00:00:00",
	})

	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2025-04-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[elementView](t, env.Data)

	var found bool
	for _, e := range res.Item.Structure.Elements {
		if e.Id != catID {
			continue
		}
		found = true
		if e.Budgeted != "1200" {
			t.Errorf("Food budgeted=%q want 1200 (April limit)", e.Budgeted)
		}
		if e.Spent != "500" {
			t.Errorf("Food spent=%q want 500 (April spending)", e.Spent)
		}
		if e.Available != "-200" {
			t.Errorf("Food available=%q want -200 (budgetedBefore 1000 - spentBefore 700 - spent 500)", e.Available)
		}
	}
	if !found {
		t.Fatalf("Food element not in structure: %+v", res.Item.Structure.Elements)
	}
}

// TestGetBudget_IncomeTransactionNotCountedAsSpending guards the CountSpending
// type filter: an INCOME transaction (type=1) that references an expense category
// must NOT inflate that category's budget `spent`. Only expenses (type=0) count.
// Mixing an expense and an income on the same category isolates the filter — the
// element's spent must equal the expense alone.
func TestGetBudget_IncomeTransactionNotCountedAsSpending(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Income Filter Budget")); st != 200 {
		t.Fatalf("create=%d body=%s", st, e.raw)
	}

	// Both categorized to the expense Food category, inside the viewed period:
	// a 120 expense (counts) and a 300 income (must not count).
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{
		ID: "eeee3333-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "120.00", SpentAt: "2025-04-10 00:00:00",
	})
	f.Transaction(fixture.Transaction{
		ID: "eeee3333-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 1, Amount: "300.00", SpentAt: "2025-04-11 00:00:00",
	})

	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2025-04-15", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[elementView](t, env.Data)

	var foodSpent string
	var found bool
	for _, e := range res.Item.Structure.Elements {
		if e.Id == catID {
			foodSpent, found = e.Spent, true
		}
	}
	if !found {
		t.Fatalf("Food element not in structure: %+v", res.Item.Structure.Elements)
	}
	if foodSpent != "120" {
		t.Fatalf("Food spent=%q want 120 (expense only; the 300 income must not count)", foodSpent)
	}
}
