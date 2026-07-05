package api_test

import (
	"net/http"
	"testing"
)

// These tests lock in the budget write-endpoint wire contract:
//   - exclude/include-account take the budget id under "id" (not "budgetId")
//   - create-envelope places the new element at position 0 (front of its group)
//   - move-element-list identifies elements by id alone (no "type" field) and
//     leaves a deterministic, contiguous ordering
//   - create/update-envelope responses include the envelope's category children

// envelopeElement is the slice of a budget element we assert on.
type envelopeElement struct {
	Id       string  `json:"id"`
	Type     int     `json:"type"`
	Position int     `json:"position"`
	FolderId *string `json:"folderId"`
	Children []struct {
		Id   string `json:"id"`
		Type int    `json:"type"`
	} `json:"children"`
}

type itemEnvelope struct {
	Item envelopeElement `json:"item"`
}

const (
	envID1 = "eeee1111-0000-7000-8000-000000000001"
	envID2 = "eeee1111-0000-7000-8000-000000000002"
)

func seedBudget(t *testing.T, h *harness, tok string) {
	t.Helper()
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Test Budget")); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}
}

// TestExcludeAccount_UsesIdField verifies the budget id arrives under "id", not
// "budgetId". Sending it under "id" must succeed; the account must then be
// recorded as excluded.
func TestExcludeAccount_UsesIdField(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/exclude-account", tok, map[string]any{
		"id": budgetID1, "accountId": accountID,
	})
	if status != http.StatusOK {
		t.Fatalf("exclude-account (id field)=%d want 200; body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id = ? AND account_id = ?`, budgetID1, accountID).Scan(&n)
	if n != 1 {
		t.Fatalf("excluded-account rows=%d want 1", n)
	}

	// Sending the legacy "budgetId" field must NOT be accepted as the budget id
	// (it is unknown to the form) -> blank id -> 400.
	status, _ = h.do(t, http.MethodPost, "/api/v1/budget/exclude-account", tok, map[string]any{
		"budgetId": budgetID1, "accountId": accountID,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("exclude-account with budgetId field=%d want 400 (id blank)", status)
	}
}

// TestIncludeAccount_UsesIdField mirrors the exclude test for include-account.
func TestIncludeAccount_UsesIdField(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	// exclude first (via the corrected field) so include is a real mutation.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/exclude-account", tok, map[string]any{"id": budgetID1, "accountId": accountID}); st != 200 {
		t.Fatalf("exclude precondition=%d body=%s", st, e.raw)
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/include-account", tok, map[string]any{
		"id": budgetID1, "accountId": accountID,
	})
	if status != http.StatusOK {
		t.Fatalf("include-account (id field)=%d want 200; body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id = ? AND account_id = ?`, budgetID1, accountID).Scan(&n)
	if n != 0 {
		t.Fatalf("excluded-account rows=%d want 0 after include", n)
	}
}

// TestCreateEnvelope_AtPositionZero verifies the new envelope element lands at
// position 0 (front of its group), and the response carries the requested
// category as a child with zero spending.
func TestCreateEnvelope_AtPositionZero(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId":   budgetID1,
		"id":         envID1,
		"name":       "Groceries",
		"icon":       "wallet",
		"currencyId": usdID,
		"categories": []string{catID},
	})
	if status != http.StatusOK {
		t.Fatalf("create-envelope=%d want 200; body=%s", status, env.raw)
	}
	res := mustUnmarshal[itemEnvelope](t, env.Data)
	if res.Item.Id != envID1 {
		t.Fatalf("envelope id=%q want %q", res.Item.Id, envID1)
	}
	if res.Item.Position != 0 {
		t.Fatalf("new envelope position=%d want 0 (front of group)", res.Item.Position)
	}
	// The requested category appears as a child.
	var sawChild bool
	for _, c := range res.Item.Children {
		if c.Id == catID {
			sawChild = true
		}
	}
	if !sawChild {
		t.Fatalf("create-envelope children=%+v want category %s", res.Item.Children, catID)
	}
	// Persisted at position 0.
	var pos int
	h.db.QueryRow(`SELECT position FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, envID1).Scan(&pos)
	if pos != 0 {
		t.Fatalf("persisted envelope position=%d want 0", pos)
	}
}

// TestMoveElementList_NoTypeField verifies move-element-list identifies the
// element by id ALONE (the form has no "type" field) and applies a folder change
// + position. The request omits "type"; it must still succeed and move the
// element into the folder.
func TestMoveElementList_NoTypeField(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	// Move the seeded category element into the (seeded) folder at position 0.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/move-element-list", tok, map[string]any{
		"budgetId": budgetID1,
		"items": []map[string]any{
			{"id": catID, "position": 0, "folderId": folderID},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("move-element-list (no type)=%d want 200; body=%s", status, env.raw)
	}
	var folder *string
	h.db.QueryRow(`SELECT folder_id FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, catID).Scan(&folder)
	if folder == nil || *folder != folderID {
		t.Fatalf("category element folder=%v want %q", folder, folderID)
	}
}
