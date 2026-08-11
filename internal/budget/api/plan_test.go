package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// decEq compares two decimal strings as normalized values (sqlite float
// rendering vs stored text differ in trailing digits, never in value).
func decEq(a, b string) bool {
	return vo.NewDecimal(a).Sub(vo.NewDecimal(b)).IsZero()
}

func getPlan(t *testing.T, h *harness, tok, query string) (int, envelope) {
	t.Helper()
	return h.do(t, http.MethodGet, "/api/v1/budget/get-budget-plan?"+query, tok, nil)
}

func planItem(t *testing.T, env envelope) model.BudgetPlanResult {
	t.Helper()
	res := mustUnmarshal[model.GetBudgetPlanResult](t, env.Data)
	return res.Item
}

// TestGetBudgetPlan_SkeletonShape: months list, meta, openingBalances,
// per-month currencyRates and folders — the response frame Tasks 3-4 fill.
func TestGetBudgetPlan_SkeletonShape(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Plan Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const folderID2 = "bfff2222-0000-7000-8000-0000000000b0"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID2, "name": "Bills",
	})

	st, env := getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-15&months=3")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	// from snaps to first-of-month; months are ordered date strings.
	wantMonths := []string{"2024-04-01", "2024-05-01", "2024-06-01"}
	if len(item.Months) != 3 || item.Months[0] != wantMonths[0] || item.Months[1] != wantMonths[1] || item.Months[2] != wantMonths[2] {
		t.Fatalf("months = %v, want %v", item.Months, wantMonths)
	}
	if item.Meta.Id != budgetID1 {
		t.Errorf("meta.id = %q", item.Meta.Id)
	}
	// One included USD account with no transactions before the window: one
	// opening balance row, zero-valued.
	if len(item.OpeningBalances) != 1 || item.OpeningBalances[0].CurrencyId != usdID || !decEq(item.OpeningBalances[0].Amount, "0") {
		t.Errorf("openingBalances = %+v", item.OpeningBalances)
	}
	if len(item.CurrencyRates) != 3 {
		t.Fatalf("currencyRates length = %d, want 3", len(item.CurrencyRates))
	}
	for i, cr := range item.CurrencyRates {
		if cr.Period != wantMonths[i] {
			t.Errorf("currencyRates[%d].period = %q want %q", i, cr.Period, wantMonths[i])
		}
		if cr.Rates == nil {
			t.Errorf("currencyRates[%d].rates must be [] not null", i)
		}
	}
	if len(item.Structure.Folders) != 1 || item.Structure.Folders[0].Id != folderID2 || item.Structure.Folders[0].Position != 0 {
		t.Errorf("folders = %+v", item.Structure.Folders)
	}
	if item.Structure.Elements == nil {
		t.Errorf("structure.elements must be [] not null")
	}
}

// TestGetBudgetPlan_Validation: id blank; months non-integer / out of 1..24
// rejected with the frozen invalid-choice message; empty months defaults to 12.
func TestGetBudgetPlan_Validation(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Plan Bounds Budget"))

	st, env := getPlan(t, h, tok, "from=2024-04-01&months=3")
	if st != http.StatusBadRequest {
		t.Fatalf("missing id = %d; body=%s", st, env.raw)
	}
	if msgs := env.errorsMap()["id"]; len(msgs) == 0 || msgs[0] != "This value should not be blank." {
		t.Errorf("id error = %v", env.errorsMap())
	}

	for _, months := range []string{"0", "25", "-1", "junk", "1.5"} {
		st, env = getPlan(t, h, tok, "id="+budgetID1+"&months="+months)
		if st != http.StatusBadRequest {
			t.Fatalf("months=%s -> %d, want 400; body=%s", months, st, env.raw)
		}
		if msgs := env.errorsMap()["months"]; len(msgs) == 0 || msgs[0] != "The value you selected is not a valid choice." {
			t.Errorf("months=%s error = %v", months, env.errorsMap())
		}
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1)
	if st != http.StatusOK {
		t.Fatalf("default window = %d; body=%s", st, env.raw)
	}
	if item := planItem(t, env); len(item.Months) != 12 {
		t.Errorf("default months = %d, want 12", len(item.Months))
	}
}

// TestGetBudgetPlan_RequiresMembership: a non-member gets the standard 403.
func TestGetBudgetPlan_RequiresMembership(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Private Budget"))

	const strangerID = "99992222-0000-7000-8000-0000000000aa"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: strangerID, Email: "stranger@example.test", Name: "Stranger", Password: "pw"})

	st, env := getPlan(t, h, strangerID, "id="+budgetID1+"&months=3")
	if st != http.StatusForbidden {
		t.Fatalf("stranger = %d, want 403; body=%s", st, env.raw)
	}
	_ = json.RawMessage(env.raw)
}
