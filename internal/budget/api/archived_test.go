package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	archFolderID1   = "bfff1111-0000-7000-8000-0000000000a1"
	archFolderID2   = "bfff1111-0000-7000-8000-0000000000a2"
	archEnvelopeID1 = "beee1111-0000-7000-8000-0000000000a1"
)

// TestArchivedBudget_WriteMatrix pins spec §2: an archived budget refuses every
// write with the coded 403, except the allowlist (unarchive, delete, revoke,
// decline, accept — and archive itself, idempotent).
func TestArchivedBudget_WriteMatrix(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok,
		map[string]any{"budgetId": budgetID1, "id": archFolderID1, "name": "Folder"})
	h.f.Account(fixture.Account{ID: accountID2, UserID: seedUserID, CurrencyID: usdID})
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Connect(seedUserID, otherUserID)

	if _, err := h.db.Exec(`UPDATE budgets SET is_archived = 1 WHERE id = ?`, budgetID1); err != nil {
		t.Fatal(err)
	}

	blocked := []struct {
		label, path string
		body        map[string]any
	}{
		{"update-budget", "/api/v1/budget/update-budget", map[string]any{"id": budgetID1, "name": "Renamed", "currencyId": usdID}},
		{"reset-budget", "/api/v1/budget/reset-budget", map[string]any{"id": budgetID1, "startedAt": "2026-07-01 00:00:00"}},
		{"create-folder", "/api/v1/budget/create-folder", map[string]any{"budgetId": budgetID1, "id": archFolderID2, "name": "Folder2"}},
		{"update-folder", "/api/v1/budget/update-folder", map[string]any{"budgetId": budgetID1, "id": archFolderID1, "name": "Folder3"}},
		{"delete-folder", "/api/v1/budget/delete-folder", map[string]any{"budgetId": budgetID1, "id": archFolderID1}},
		{"move-folder", "/api/v1/budget/move-folder", map[string]any{"budgetId": budgetID1, "id": archFolderID1, "after": ""}},
		{"create-envelope", "/api/v1/budget/create-envelope", map[string]any{"budgetId": budgetID1, "id": archEnvelopeID1, "name": "Envelope", "icon": "i", "currencyId": usdID, "categories": []string{}}},
		{"grant-access", "/api/v1/budget/grant-access", map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"}},
		{"add-account", "/api/v1/budget/add-account", map[string]any{"id": budgetID1, "accountId": accountID2}},
		{"remove-account", "/api/v1/budget/remove-account", map[string]any{"id": budgetID1, "accountId": accountID}},
		{"set-limit", "/api/v1/budget/set-limit", map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-08-01", "amount": "10"}},
		{"move-element", "/api/v1/budget/move-element", map[string]any{"budgetId": budgetID1, "id": catID, "folderId": nil, "after": ""}},
		{"change-element-currency", "/api/v1/budget/change-element-currency", map[string]any{"budgetId": budgetID1, "elementId": catID, "currencyId": usdID}},
	}
	for _, c := range blocked {
		st, env := h.do(t, http.MethodPost, c.path, tok, c.body)
		if st != http.StatusForbidden || !strings.Contains(string(env.raw), "archived") {
			t.Fatalf("%s: status=%d body=%s (want 403 budget.archived)", c.label, st, env.raw)
		}
	}

	// allowlist: unarchive works on an archived budget, archive is idempotent,
	// and delete succeeds while archived.
	h.mustDo(t, http.MethodPost, "/api/v1/budget/unarchive-budget", tok, map[string]any{"id": budgetID1})
	env := h.mustDo(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": budgetID1})
	res := mustUnmarshal[metaItemOut](t, env.Data)
	if res.Item.IsArchived != 1 {
		t.Fatalf("archive: meta=%+v want isArchived=1", res.Item)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": budgetID1})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/delete-budget", tok, map[string]any{"id": budgetID1})
}

// Access answers stay open on an archived budget: a pending invite can still be
// accepted, declined or revoked (spec §2 allowlist).
func TestArchivedBudget_AccessAnswersStillWork(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Budget"))
	// grant BEFORE archiving; the invitee answers AFTER
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"})
	if _, err := h.db.Exec(`UPDATE budgets SET is_archived = 1 WHERE id = ?`, budgetID1); err != nil {
		t.Fatal(err)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID})

	// decline path: re-grant, then the invitee declines while archived
	if _, err := h.db.Exec(`UPDATE budgets SET is_archived = 0 WHERE id = ?`, budgetID1); err != nil {
		t.Fatal(err)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"})
	if _, err := h.db.Exec(`UPDATE budgets SET is_archived = 1 WHERE id = ?`, budgetID1); err != nil {
		t.Fatal(err)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/decline-access", otherUserID, map[string]any{"budgetId": budgetID1})
}

// Archive/unarchive are owner|admin only (canDelete), like delete-budget.
func TestArchiveBudget_RequiresOwnerOrAdmin(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Budget"))
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1})

	for _, path := range []string{"/api/v1/budget/archive-budget", "/api/v1/budget/unarchive-budget"} {
		st, env := h.do(t, http.MethodPost, path, otherUserID, map[string]any{"id": budgetID1})
		if st != http.StatusForbidden {
			t.Fatalf("%s as role=user: status=%d body=%s want 403", path, st, env.raw)
		}
	}
}
