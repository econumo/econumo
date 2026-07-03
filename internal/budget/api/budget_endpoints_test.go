package api_test

import (
	"net/http"
	"testing"
)

const budgetID1 = "bbbb2222-0000-7000-8000-000000000001"

// budgetResult mirrors the slice of the BudgetResult these tests assert on.
type budgetResult struct {
	Item struct {
		Meta struct {
			Id          string `json:"id"`
			OwnerUserId string `json:"ownerUserId"`
			Name        string `json:"name"`
			CurrencyId  string `json:"currencyId"`
			Access      []struct {
				Role       string `json:"role"`
				IsAccepted int    `json:"isAccepted"`
			} `json:"access"`
		} `json:"meta"`
		Structure struct {
			Elements []struct {
				Id   string `json:"id"`
				Type int    `json:"type"`
				Name string `json:"name"`
			} `json:"elements"`
		} `json:"structure"`
	} `json:"item"`
}

func createBudgetReq(id, name string) map[string]any {
	return map[string]any{"id": id, "name": name, "currencyId": usdID}
}

func TestCreateBudget_SeedsElements_OwnerAccess(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Test Budget"))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[budgetResult](t, env.Data)
	if res.Item.Meta.Id != budgetID1 || res.Item.Meta.Name != "Test Budget" {
		t.Fatalf("meta=%+v want id/name", res.Item.Meta)
	}
	if res.Item.Meta.OwnerUserId != seedUserID {
		t.Fatalf("ownerUserId=%q want %q", res.Item.Meta.OwnerUserId, seedUserID)
	}
	if res.Item.Meta.CurrencyId != usdID {
		t.Fatalf("currencyId=%q want %q", res.Item.Meta.CurrencyId, usdID)
	}
	// The owner gets a synthetic "owner" access entry (accepted).
	var sawOwner bool
	for _, a := range res.Item.Meta.Access {
		if a.Role == "owner" && a.IsAccepted == 1 {
			sawOwner = true
		}
	}
	if !sawOwner {
		t.Fatalf("access=%+v want an accepted owner entry", res.Item.Meta.Access)
	}
	// The non-income category appears in the structure (a tag with zero spending
	// is pruned from the structure, so assert the tag ELEMENT row exists in the DB
	// rather than in the wire structure).
	var sawCat bool
	for _, e := range res.Item.Structure.Elements {
		if e.Id == catID {
			sawCat = true
		}
	}
	if !sawCat {
		t.Fatalf("structure elements=%+v want seeded category %s", res.Item.Structure.Elements, catID)
	}
	// create-budget seeds an element row for both the category and the tag.
	for _, ext := range []string{catID, tagID} {
		var n int
		h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, ext).Scan(&n)
		if n != 1 {
			t.Fatalf("element rows for %s = %d, want 1 (seeded)", ext, n)
		}
	}
}

func TestCreateBudget_ShortName_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "B"))
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (name 3-64)", status)
	}
}

func TestCreateBudget_SecondBudget_BothListed(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const budgetID2 = "bbbb2222-0000-7000-8000-000000000002"
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "First Budget")); st != 200 {
		t.Fatalf("first=%d body=%s", st, e.raw)
	}
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID2, "Second Budget")); st != 200 {
		t.Fatalf("second=%d body=%s", st, e.raw)
	}
	_, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget-list", tok, nil)
	type listWrap struct {
		Items []struct {
			Id string `json:"id"`
		} `json:"items"`
	}
	wrap := mustUnmarshal[listWrap](t, env.Data)
	ids := map[string]bool{}
	for _, it := range wrap.Items {
		ids[it.Id] = true
	}
	if !ids[budgetID1] || !ids[budgetID2] {
		t.Fatalf("list ids=%v want both budgets", ids)
	}
}

func TestGetBudget_ReturnsCreatedBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Test Budget")); st != 200 {
		t.Fatalf("create=%d body=%s", st, e.raw)
	}
	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[budgetResult](t, env.Data)
	if res.Item.Meta.Id != budgetID1 {
		t.Fatalf("get-budget meta id=%q want %q", res.Item.Meta.Id, budgetID1)
	}
}

func TestGetBudgetList_ListsCreated(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Test Budget"))
	_, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget-list", tok, nil)
	type listWrap struct {
		Items []struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	wrap := mustUnmarshal[listWrap](t, env.Data)
	var found bool
	for _, it := range wrap.Items {
		if it.Id == budgetID1 && it.Name == "Test Budget" {
			found = true
		}
	}
	if !found {
		t.Fatalf("budget-list=%+v want the created budget", wrap.Items)
	}
}

func TestSetLimit_OnSeededCategoryElement(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Test Budget"))
	// elementId = the category's EXTERNAL id (the service resolves it via
	// GetElementByExternal). period = first-of-month >= budget.startedAt.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01", "amount": "200",
	})
	if status != http.StatusOK {
		t.Fatalf("set-limit=%d want 200; body=%s", status, env.raw)
	}
	// The limit row now exists for the element.
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits l
		JOIN budgets_elements e ON e.id = l.element_id
		WHERE e.budget_id = ? AND e.external_id = ?`, budgetID1, catID).Scan(&n)
	if n != 1 {
		t.Fatalf("limit rows=%d want 1", n)
	}
}

func TestCreateBudget_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", "", createBudgetReq(budgetID1, "Test Budget"))
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", status)
	}
}
