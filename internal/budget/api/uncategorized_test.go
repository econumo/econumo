package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
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
	// The Uncategorized row still sorts LAST in its group, but position is now
	// the dense 0-based index rather than an int16 sentinel, so "last" means
	// "one past every real element" instead of 32767.
	maxReal := -1
	for _, other := range res.Item.Structure.Elements {
		if other.Id != el.Id && other.FolderId == nil && other.Position > maxReal {
			maxReal = other.Position
		}
	}
	if el.Position != maxReal+1 {
		t.Errorf("position=%d want %d (one past the last real ungrouped element)", el.Position, maxReal+1)
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

// TestUncategorized_Drilldown_TopLevelAndTagChild: clicking the top-level
// "uncategorized" row must list exactly the untagged-and-uncategorized
// transactions; clicking a tag's "uncategorized" child must list exactly that
// tag's tagged-but-uncategorized transactions. Neither leaks a categorized row.
func TestUncategorized_Drilldown_TopLevelAndTagChild(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Uncat Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	// Categorized, unaffected by either drill-down below.
	f.Transaction(fixture.Transaction{
		ID: "eeee3333-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "10.00", SpentAt: "2024-04-10 00:00:00",
	})
	uncategorizedUntaggedID := f.Transaction(fixture.Transaction{
		ID: "eeee3333-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		Type: 0, Amount: "20.00", SpentAt: "2024-04-11 00:00:00",
	})
	uncategorizedTaggedID := f.Transaction(fixture.Transaction{
		ID: "eeee3333-0000-7000-8000-000000000003", UserID: seedUserID, AccountID: accountID,
		TagID: tagID, Type: 0, Amount: "30.00", SpentAt: "2024-04-12 00:00:00",
	})

	base := "/api/v1/budget/get-transaction-list?budgetId=" + budgetID1 + "&periodStart=2024-04-01"

	// Top-level uncategorized row.
	st, b := h.do(t, http.MethodGet, base+"&uncategorized=1", tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-transaction-list uncategorized=%d body=%s", st, b.raw)
	}
	got := mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != uncategorizedUntaggedID {
		t.Fatalf("top-level uncategorized drill-down must return exactly %q; got %s", uncategorizedUntaggedID, b.Data)
	}
	if vo.NewDecimal(got.Items[0].Amount).String() != vo.NewDecimal("20.00").String() {
		t.Errorf("amount mismatch: got %q, want 20.00", got.Items[0].Amount)
	}

	// A tag's uncategorized child.
	st, b = h.do(t, http.MethodGet, base+"&tagId="+tagID+"&uncategorized=1", tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-transaction-list tag uncategorized=%d body=%s", st, b.raw)
	}
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != uncategorizedTaggedID {
		t.Fatalf("tag uncategorized-child drill-down must return exactly %q; got %s", uncategorizedTaggedID, b.Data)
	}
	if vo.NewDecimal(got.Items[0].Amount).String() != vo.NewDecimal("30.00").String() {
		t.Errorf("amount mismatch: got %q, want 30.00", got.Items[0].Amount)
	}

	// uncategorized + categoryId is a validation error, mirroring
	// TransactionListRequest.Validate().
	status, env := h.do(t, http.MethodGet, base+"&uncategorized=1&categoryId="+catID, tok, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("get-transaction-list uncategorized+categoryId=%d want 400; body=%s", status, env.raw)
	}
	msgs := env.errorsMap()["categoryId"]
	if len(msgs) == 0 {
		t.Fatalf("errors[categoryId] must be present; body=%s", env.raw)
	}
}

// TestUncategorized_EnvelopeIdCombinationIsRejected: envelopes have no
// uncategorized bucket of their own, so uncategorized+envelopeId must be
// rejected the same way tagId+envelopeId already is - falling through to the
// "at least one selector" error - rather than silently returning the
// envelope's CATEGORIZED transactions (the opposite of what was asked for).
func TestUncategorized_EnvelopeIdCombinationIsRejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Uncat Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	envelopeID := f.BudgetEnvelope(fixture.BudgetEnvelope{BudgetID: budgetID1, Name: "Envelope"})
	f.EnvelopeCategory(envelopeID, catID)
	f.Transaction(fixture.Transaction{
		ID: "eeee4444-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "50.00", SpentAt: "2024-04-10 00:00:00",
	})

	base := "/api/v1/budget/get-transaction-list?budgetId=" + budgetID1 + "&periodStart=2024-04-01"
	status, env := h.do(t, http.MethodGet, base+"&envelopeId="+envelopeID+"&uncategorized=1", tok, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("get-transaction-list uncategorized+envelopeId=%d want 400 (must not silently return categorized rows); body=%s", status, env.raw)
	}
}
