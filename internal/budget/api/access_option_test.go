package api_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// A user's active-budget option must not keep pointing at a budget they can no
// longer read: the SPA loads the option's budget on boot and a stale id turns
// into a permanent 403 (get-budget) with the budget absent from get-budget-list.

func budgetOption(t *testing.T, h *harness, userID string) sql.NullString {
	t.Helper()
	var v sql.NullString
	if err := h.db.QueryRow(`SELECT value FROM users_options WHERE user_id=? AND name='budget'`, userID).Scan(&v); err != nil {
		t.Fatalf("read budget option for %s: %v", userID, err)
	}
	return v
}

func setBudgetOption(t *testing.T, h *harness, userID, budgetID string) {
	t.Helper()
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Option(userID, "budget", &budgetID)
}

func TestRevokeAccess_ClearsRevokedUsersActiveBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	h.seedSecondUser(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	}); st != http.StatusOK {
		t.Fatalf("grant precondition=%d body=%s", st, e.raw)
	}
	setBudgetOption(t, h, secondUserID, budgetID1)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID,
	}); st != http.StatusOK {
		t.Fatalf("revoke-access=%d body=%s", st, e.raw)
	}

	if got := budgetOption(t, h, secondUserID); got.Valid {
		t.Fatalf("revoked user's budget option=%q want cleared", got.String)
	}
	// the owner's own option is untouched
	if got := budgetOption(t, h, seedUserID); !got.Valid || got.String != budgetID1 {
		t.Fatalf("owner's budget option=%v want %s", got, budgetID1)
	}
}

func TestRevokeAccess_LeavesOtherActiveBudgetAlone(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	h.seedSecondUser(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	}); st != http.StatusOK {
		t.Fatalf("grant precondition=%d body=%s", st, e.raw)
	}
	// the revoked user's active budget is some OTHER budget — must survive
	setBudgetOption(t, h, secondUserID, "bbbb2222-0000-7000-8000-00000000beef")

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID,
	}); st != http.StatusOK {
		t.Fatalf("revoke-access=%d body=%s", st, e.raw)
	}
	if got := budgetOption(t, h, secondUserID); !got.Valid || got.String != "bbbb2222-0000-7000-8000-00000000beef" {
		t.Fatalf("unrelated budget option=%v want untouched", got)
	}
}

func TestDeclineAccess_ClearsOwnActiveBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	}); st != http.StatusOK {
		t.Fatalf("grant precondition=%d body=%s", st, e.raw)
	}
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/accept-access", other, map[string]any{
		"budgetId": budgetID1,
	}); st != http.StatusOK {
		t.Fatalf("accept precondition=%d body=%s", st, e.raw)
	}
	setBudgetOption(t, h, secondUserID, budgetID1)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/decline-access", other, map[string]any{
		"budgetId": budgetID1,
	}); st != http.StatusOK {
		t.Fatalf("decline-access=%d body=%s", st, e.raw)
	}
	if got := budgetOption(t, h, secondUserID); got.Valid {
		t.Fatalf("declining user's budget option=%q want cleared", got.String)
	}
}

func TestDeleteBudget_ClearsParticipantsActiveBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	}); st != http.StatusOK {
		t.Fatalf("grant precondition=%d body=%s", st, e.raw)
	}
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/accept-access", other, map[string]any{
		"budgetId": budgetID1,
	}); st != http.StatusOK {
		t.Fatalf("accept precondition=%d body=%s", st, e.raw)
	}
	setBudgetOption(t, h, secondUserID, budgetID1)
	// create-budget set the owner's option to budgetID1 already
	if got := budgetOption(t, h, seedUserID); !got.Valid || got.String != budgetID1 {
		t.Fatalf("owner option precondition=%v want %s", got, budgetID1)
	}

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/delete-budget", tok, map[string]any{"id": budgetID1}); st != http.StatusOK {
		t.Fatalf("delete-budget=%d body=%s", st, e.raw)
	}

	if got := budgetOption(t, h, seedUserID); got.Valid {
		t.Fatalf("owner's budget option=%q want cleared after delete", got.String)
	}
	if got := budgetOption(t, h, secondUserID); got.Valid {
		t.Fatalf("participant's budget option=%q want cleared after delete", got.String)
	}
}
