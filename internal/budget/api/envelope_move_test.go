package api_test

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	moveCatA      = "cccc5555-0000-7000-8000-000000000001"
	moveCatB      = "cccc5555-0000-7000-8000-000000000002"
	moveIncomeCat = "cccc5555-0000-7000-8000-000000000003"
	moveEnvOne    = "beee5555-0000-7000-8000-000000000001"
	moveEnvTwo    = "beee5555-0000-7000-8000-000000000002"
	moveEnvIncome = "beee5555-0000-7000-8000-000000000003"
	moveFolder    = "bfff5555-0000-7000-8000-000000000001"
)

// envelopesOf lists the envelopes holding the category.
func envelopesOf(t *testing.T, h *harness, categoryID string) []string {
	t.Helper()
	rows, err := h.db.Query(`SELECT budget_envelope_id FROM budgets_envelopes_categories WHERE category_id = ? ORDER BY budget_envelope_id`, categoryID)
	if err != nil {
		t.Fatalf("read memberships: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan membership: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func elementPlace(t *testing.T, h *harness, externalID string) (folderID, sortKey string) {
	t.Helper()
	var folder, key sql.NullString
	if err := h.db.QueryRow(`SELECT folder_id, sort_key FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, externalID).Scan(&folder, &key); err != nil {
		t.Fatalf("read element place: %v", err)
	}
	return folder.String, key.String
}

func newEnvelopeMoveBudget(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness(t)
	tok := h.token(t)
	h.f.Category(fixture.Category{ID: moveCatA, UserID: seedUserID, Name: "Groceries Move", Type: 0, Icon: "cart"})
	h.f.Category(fixture.Category{ID: moveCatB, UserID: seedUserID, Name: "Cafes Move", Type: 0, Icon: "cafe"})
	h.f.Category(fixture.Category{ID: moveIncomeCat, UserID: seedUserID, Name: "Wages Move", Type: 1, Icon: "payments"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Envelope Move Budget"))
	for _, env := range []struct {
		id, side string
		cats     []string
	}{{moveEnvOne, "expense", []string{moveCatA}}, {moveEnvTwo, "expense", []string{}}, {moveEnvIncome, "income", []string{}}} {
		h.mustDo(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
			"budgetId": budgetID1, "id": env.id, "name": "Env " + env.id[len(env.id)-2:], "icon": "i",
			"currencyId": usdID, "folderId": nil, "side": env.side, "categories": env.cats,
		})
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": budgetID1, "id": moveFolder, "name": "Living", "side": "expense"})
	return h, tok
}

func moveTo(t *testing.T, h *harness, tok, id string, folderID, envelopeID any) (int, envelope) {
	t.Helper()
	body := map[string]any{"budgetId": budgetID1, "id": id, "folderId": folderID, "afterId": nil}
	if envelopeID != nil {
		body["envelopeId"] = envelopeID
	}
	return h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, body)
}

func TestMoveElement_CategoryLeavesItsEnvelopeForTheTargetPlace(t *testing.T) {
	h, tok := newEnvelopeMoveBudget(t)
	if got := envelopesOf(t, h, moveCatA); len(got) != 1 || got[0] != moveEnvOne {
		t.Fatalf("setup: memberships = %v", got)
	}
	if st, env := moveTo(t, h, tok, moveCatA, moveFolder, nil); st != http.StatusOK {
		t.Fatalf("move out = %d; body=%s", st, env.raw)
	}
	if got := envelopesOf(t, h, moveCatA); len(got) != 0 {
		t.Fatalf("the category must leave its envelope, still in %v", got)
	}
	folder, key := elementPlace(t, h, moveCatA)
	if folder != moveFolder || key == "" {
		t.Fatalf("placed at folder=%q key=%q, want the folder with a key", folder, key)
	}
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if !strings.Contains(string(env.Data), `"id":"`+moveCatA+`","type":1`) {
		t.Errorf("the category must list as its own element; body=%s", env.Data)
	}
}

func TestMoveElement_IntoAnEnvelopeAndBetweenEnvelopes(t *testing.T) {
	h, tok := newEnvelopeMoveBudget(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/move-element", tok,
		map[string]any{"budgetId": budgetID1, "id": moveCatB, "folderId": moveFolder, "afterId": nil})

	// a top-level category joins an envelope and drops out of the listing
	if st, env := moveTo(t, h, tok, moveCatB, nil, moveEnvTwo); st != http.StatusOK {
		t.Fatalf("into envelope = %d; body=%s", st, env.raw)
	}
	if got := envelopesOf(t, h, moveCatB); len(got) != 1 || got[0] != moveEnvTwo {
		t.Fatalf("memberships = %v, want envelope two", got)
	}
	if folder, key := elementPlace(t, h, moveCatB); folder != "" || key != "" {
		t.Fatalf("an envelope child has no place of its own: folder=%q key=%q", folder, key)
	}

	// from one envelope to another: it leaves the first
	if st, env := moveTo(t, h, tok, moveCatA, nil, moveEnvTwo); st != http.StatusOK {
		t.Fatalf("between envelopes = %d; body=%s", st, env.raw)
	}
	if got := envelopesOf(t, h, moveCatA); len(got) != 1 || got[0] != moveEnvTwo {
		t.Fatalf("memberships = %v, want envelope two only", got)
	}

	// dropping it on its own envelope again changes nothing
	if st, env := moveTo(t, h, tok, moveCatA, nil, moveEnvTwo); st != http.StatusOK {
		t.Fatalf("into its own envelope = %d; body=%s", st, env.raw)
	}
	if got := envelopesOf(t, h, moveCatA); len(got) != 1 {
		t.Fatalf("memberships = %v", got)
	}
}

func TestMoveElement_IntoEnvelopeRefusals(t *testing.T) {
	h, tok := newEnvelopeMoveBudget(t)

	st, env := moveTo(t, h, tok, moveIncomeCat, nil, moveEnvTwo)
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot contain both income and expense categories") {
		t.Fatalf("income category into an expense envelope: st=%d body=%s", st, env.raw)
	}
	st, env = moveTo(t, h, tok, moveCatB, nil, moveEnvIncome)
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot contain both income and expense categories") {
		t.Fatalf("expense category into an income envelope: st=%d body=%s", st, env.raw)
	}
	st, env = moveTo(t, h, tok, moveEnvOne, nil, moveEnvTwo)
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "Only a category can go into an envelope") {
		t.Fatalf("envelope into an envelope: st=%d body=%s", st, env.raw)
	}
	st, env = moveTo(t, h, tok, moveCatB, nil, "beee5555-0000-7000-8000-0000000000ff")
	if st != http.StatusForbidden {
		t.Fatalf("unknown envelope = %d, want 403; body=%s", st, env.raw)
	}
	if got := envelopesOf(t, h, moveCatB); len(got) != 0 {
		t.Fatalf("a refused move must not write: memberships = %v", got)
	}
}

func TestMoveElement_IncomeCategoryIntoIncomeEnvelope(t *testing.T) {
	h, tok := newEnvelopeMoveBudget(t)
	if st, env := moveTo(t, h, tok, moveIncomeCat, nil, moveEnvIncome); st != http.StatusOK {
		t.Fatalf("income into income envelope = %d; body=%s", st, env.raw)
	}
	if got := envelopesOf(t, h, moveIncomeCat); len(got) != 1 || got[0] != moveEnvIncome {
		t.Fatalf("memberships = %v", got)
	}
}

// A category created after the budget's last structure write has no element row
// yet; placing it must create the row and land it, not silently do nothing.
func TestMoveElement_PlacesACategoryWithNoElementRowYet(t *testing.T) {
	h, tok := newEnvelopeMoveBudget(t)
	const fresh = "cccc5555-0000-7000-8000-0000000000f1"
	h.f.Category(fixture.Category{ID: fresh, UserID: seedUserID, Name: "Fresh Move", Type: 0, Icon: "i"})
	var rows int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, fresh).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("setup: element rows = %d (err=%v), want none yet", rows, err)
	}
	if st, env := moveTo(t, h, tok, fresh, moveFolder, nil); st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, env.raw)
	}
	if folder, key := elementPlace(t, h, fresh); folder != moveFolder || key == "" {
		t.Fatalf("placed at folder=%q key=%q, want the folder", folder, key)
	}
}
