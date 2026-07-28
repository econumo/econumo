package api_test

import (
	"math"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// uncategorizedElement is the full shape of a get-budget parent element that
// this file's tests assert on (matches model.ParentElementResult).
type uncategorizedElement struct {
	Id          string  `json:"id"`
	Type        int     `json:"type"`
	Name        string  `json:"name"`
	Icon        string  `json:"icon"`
	CurrencyId  string  `json:"currencyId"`
	IsArchived  int     `json:"isArchived"`
	FolderId    *string `json:"folderId"`
	Position    int     `json:"position"`
	Budgeted    string  `json:"budgeted"`
	Available   string  `json:"available"`
	Spent       string  `json:"spent"`
	BudgetSpent string  `json:"budgetSpent"`
	OwnerUserId *string `json:"ownerUserId"`
	Children    []struct {
		Id          string `json:"id"`
		Type        int    `json:"type"`
		Name        string `json:"name"`
		Icon        string `json:"icon"`
		IsArchived  int    `json:"isArchived"`
		Spent       string `json:"spent"`
		BudgetSpent string `json:"budgetSpent"`
		OwnerUserId string `json:"ownerUserId"`
	} `json:"children"`
}

// uncategorizedBudgetView is the slice of get-budget tests in this file care
// about: elements plus their children.
type uncategorizedBudgetView struct {
	Item struct {
		Structure struct {
			Elements []uncategorizedElement `json:"elements"`
		} `json:"structure"`
	} `json:"item"`
}

func (v uncategorizedBudgetView) find(id string) (uncategorizedElement, bool) {
	for _, e := range v.Item.Structure.Elements {
		if e.Id == id {
			return e, true
		}
	}
	return uncategorizedElement{}, false
}

// TestUncategorized_TopLevelRowAppears: an expense with no category and no tag
// surfaces as a top-level "uncategorized" element carrying that spending.
func TestUncategorized_TopLevelRowAppears(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Uncat Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{
		ID: "eeee1111-0000-7000-8000-0000000000a1", UserID: seedUserID, AccountID: accountID,
		Type: 0, Amount: "42.00", SpentAt: "2024-04-10 00:00:00",
	})

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[uncategorizedBudgetView](t, b.Data)
	el, ok := res.find("uncategorized")
	if !ok {
		t.Fatalf("uncategorized element must be present; body: %s", b.Data)
	}
	if el.Type != 1 {
		t.Errorf("type=%d want 1 (ElementCategory)", el.Type)
	}
	if el.Name != "Uncategorized" {
		t.Errorf("name=%q want Uncategorized", el.Name)
	}
	if el.Icon != "question_mark" {
		t.Errorf("icon=%q want question_mark", el.Icon)
	}
	if el.CurrencyId != usdID {
		t.Errorf("currencyId=%q want %q", el.CurrencyId, usdID)
	}
	if el.IsArchived != 0 {
		t.Errorf("isArchived=%d want 0", el.IsArchived)
	}
	if el.Position != math.MaxInt16 {
		t.Errorf("position=%d want %d (sentinel sorting it last)", el.Position, math.MaxInt16)
	}
	if el.OwnerUserId != nil {
		t.Errorf("ownerUserId=%v want nil", el.OwnerUserId)
	}
	if el.Budgeted != "0" {
		t.Errorf("budgeted=%q want 0", el.Budgeted)
	}
	if el.Spent != "42" {
		t.Errorf("spent=%q want 42", el.Spent)
	}
	if el.BudgetSpent != "42" {
		t.Errorf("budgetSpent=%q want 42", el.BudgetSpent)
	}
	// available = budgetedBefore - spentBefore - spent, which with no limits
	// (this element can never have one) reduces to -spent. Asserting the real
	// value guards against a future change hardcoding it to "0".
	if el.Available != "-42" {
		t.Errorf("available=%q want -42", el.Available)
	}
	if el.FolderId != nil {
		t.Errorf("folderId=%v want nil", el.FolderId)
	}
	if len(el.Children) != 0 {
		t.Errorf("children=%v want empty", el.Children)
	}
}

// TestUncategorized_HiddenWhenNoSuchSpending: with only categorized spending,
// no "uncategorized" element appears.
func TestUncategorized_HiddenWhenNoSuchSpending(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Uncat Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{
		ID: "eeee1111-0000-7000-8000-0000000000a2", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "10.00", SpentAt: "2024-04-10 00:00:00",
	})

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[uncategorizedBudgetView](t, b.Data)
	if _, ok := res.find("uncategorized"); ok {
		t.Fatalf("uncategorized element must not appear with no uncategorized spending; body: %s", b.Data)
	}
}

// TestUncategorized_TaggedGoesToTheTagAsAChild: a tagged-but-uncategorized
// expense belongs to its tag, as an Uncategorized child - not the top-level row.
func TestUncategorized_TaggedGoesToTheTagAsAChild(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Uncat Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{
		ID: "eeee1111-0000-7000-8000-0000000000a3", UserID: seedUserID, AccountID: accountID,
		TagID: tagID, Type: 0, Amount: "17.50", SpentAt: "2024-04-10 00:00:00",
	})

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[uncategorizedBudgetView](t, b.Data)
	if _, ok := res.find("uncategorized"); ok {
		t.Fatalf("top-level uncategorized row must be absent when the spending is tagged; body: %s", b.Data)
	}
	tagEl, ok := res.find(tagID)
	if !ok {
		t.Fatalf("tag element must be present; body: %s", b.Data)
	}
	if tagEl.Spent != "17.5" {
		t.Errorf("tag spent=%q want 17.5", tagEl.Spent)
	}
	if tagEl.BudgetSpent != "17.5" {
		t.Errorf("tag budgetSpent=%q want 17.5", tagEl.BudgetSpent)
	}
	if tagEl.Available != "-17.5" {
		t.Errorf("tag available=%q want -17.5", tagEl.Available)
	}
	if len(tagEl.Children) != 1 {
		t.Fatalf("tag children=%+v want exactly one child", tagEl.Children)
	}
	child := tagEl.Children[0]
	if child.Id != "uncategorized" {
		t.Errorf("child id=%q want uncategorized", child.Id)
	}
	if child.Type != 1 {
		t.Errorf("child type=%d want 1 (ElementCategory)", child.Type)
	}
	if child.Name != "Uncategorized" {
		t.Errorf("child name=%q want Uncategorized", child.Name)
	}
	if child.Icon != "question_mark" {
		t.Errorf("child icon=%q want question_mark", child.Icon)
	}
	if child.IsArchived != 0 {
		t.Errorf("child isArchived=%d want 0", child.IsArchived)
	}
	if child.OwnerUserId != "" {
		t.Errorf("child ownerUserId=%q want empty", child.OwnerUserId)
	}
	if child.Spent != "17.5" {
		t.Errorf("child spent=%q want 17.5", child.Spent)
	}
	if child.BudgetSpent != "17.5" {
		t.Errorf("child budgetSpent=%q want 17.5", child.BudgetSpent)
	}
}

// TestUncategorized_SetLimitIsRejected: the uncategorized id is not a UUID, so
// set-limit fails validation the same way an empty elementId would, and no
// budget_elements row is ever written for it.
func TestUncategorized_SetLimitIsRejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": "uncategorized", "period": "2099-01-01", "amount": "10",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("set-limit uncategorized=%d want 400; body=%s", status, env.raw)
	}
	msgs := env.errorsMap()["elementId"]
	if len(msgs) == 0 {
		t.Fatalf("errors[elementId] must be present; body=%s", env.raw)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE external_id = ?`, "uncategorized").Scan(&n); err != nil {
		t.Fatalf("count budgets_elements: %v", err)
	}
	if n != 0 {
		t.Errorf("the uncategorized element must never be persisted; found %d rows", n)
	}
}

// TestUncategorized_TagWithOnlyCategoryDeletedSpending is the regression guard
// for the historical shape left behind by delete-category's hard-delete mode
// (internal/category/delete.go): a transaction whose category FK was nulled but
// which still carries a tag. Before this feature the row was dropped from
// aggregation entirely, so such a tag stayed hidden; now it must show as a real
// parent element carrying the spending, with an Uncategorized child - not an
// all-zero ghost row.
func TestUncategorized_TagWithOnlyCategoryDeletedSpending(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Uncat Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	// Shape left behind by delete-category in delete mode: tag present, category
	// FK nulled. No limit on the tag anywhere.
	f.Transaction(fixture.Transaction{
		ID: "eeee1111-0000-7000-8000-0000000000a4", UserID: seedUserID, AccountID: accountID,
		TagID: tagID, Type: 0, Amount: "88.00", SpentAt: "2024-04-10 00:00:00",
	})

	_, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-04-15", tok, nil)
	res := mustUnmarshal[uncategorizedBudgetView](t, b.Data)

	if _, ok := res.find("uncategorized"); ok {
		t.Fatalf("top-level uncategorized row must be absent - the tag wins; body: %s", b.Data)
	}
	tagEl, ok := res.find(tagID)
	if !ok {
		t.Fatalf("tag must be visible carrying the category-deleted spending; body: %s", b.Data)
	}
	if tagEl.Spent != "88" {
		t.Errorf("tag spent=%q want 88 (must not render as an all-zero ghost row)", tagEl.Spent)
	}
	if tagEl.BudgetSpent != "88" {
		t.Errorf("tag budgetSpent=%q want 88", tagEl.BudgetSpent)
	}
	if tagEl.Available != "-88" {
		t.Errorf("tag available=%q want -88", tagEl.Available)
	}
	if len(tagEl.Children) != 1 {
		t.Fatalf("tag children=%+v want exactly one child", tagEl.Children)
	}
	child := tagEl.Children[0]
	if child.Id != "uncategorized" || child.Name != "Uncategorized" || child.Icon != "question_mark" {
		t.Errorf("child=%+v want id/name/icon = uncategorized/Uncategorized/question_mark", child)
	}
	if child.Spent != "88" || child.BudgetSpent != "88" {
		t.Errorf("child spent/budgetSpent=%q/%q want 88/88", child.Spent, child.BudgetSpent)
	}
}
