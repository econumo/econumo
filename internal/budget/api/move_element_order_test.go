package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// elementOrderView is the slice of get-budget this file needs.
type elementOrderView struct {
	Item struct {
		Structure struct {
			Elements []struct {
				Id       string  `json:"id"`
				FolderId *string `json:"folderId"`
				Position int     `json:"position"`
			} `json:"elements"`
		} `json:"structure"`
	} `json:"item"`
}

// elementOrderIn returns the ids of the budget's ungrouped elements, in the
// order the structure reports them.
func elementOrderIn(t *testing.T, h *harness, tok string) []string {
	t.Helper()
	_, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2024-05-15", tok, nil)
	view := mustUnmarshal[elementOrderView](t, env.Data)
	out := []string{}
	for _, e := range view.Item.Structure.Elements {
		if e.FolderId == nil {
			out = append(out, e.Id)
		}
	}
	return out
}

// TestMoveElement_LandsAfterTheAnchor is the behaviour a drag depends on: an
// element dropped between two others must land THERE, not at the end.
//
// The element ids on the wire are EXTERNAL ids (the category/tag/envelope id),
// not budgets_elements row ids, so the sibling list the anchor is resolved
// against has to be keyed the same way. Keyed by row id, the anchor never
// matches and every move silently appends -- which looks like rows jumping to
// random places.
func TestMoveElement_LandsAfterTheAnchor(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// Two extra expense categories, so the budget seeds enough ungrouped
	// elements to have a middle to drop into.
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: "cccc1111-0000-7000-8000-000000000002", UserID: seedUserID, Name: "Rent", Type: 0, Icon: "home", Position: 1})
	f.Category(fixture.Category{ID: "cccc1111-0000-7000-8000-000000000003", UserID: seedUserID, Name: "Fuel", Type: 0, Icon: "car", Position: 2})
	seedBudget(t, h, tok)

	before := elementOrderIn(t, h, tok)
	if len(before) < 3 {
		t.Fatalf("need at least 3 ungrouped elements, got %v", before)
	}
	first, second, last := before[0], before[1], before[len(before)-1]

	// Drop the LAST element directly after the FIRST.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": last, "folderId": nil, "afterId": first,
	})
	if status != http.StatusOK {
		t.Fatalf("move-element=%d body=%s", status, env.raw)
	}

	after := elementOrderIn(t, h, tok)
	if len(after) < 3 {
		t.Fatalf("elements vanished: %v", after)
	}
	if after[0] != first || after[1] != last {
		t.Fatalf("order = %v, want %q then %q (dropped after the first, not appended); was %v",
			after, first, last, before)
	}
	if after[2] != second {
		t.Errorf("third element = %q, want %q", after[2], second)
	}
}
