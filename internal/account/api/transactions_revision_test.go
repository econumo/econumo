package api_test

import (
	"net/http"
	"testing"
)

func (h *harness) dbRevision(t *testing.T, accountID string) int64 {
	t.Helper()
	var rev int64
	if err := h.db.QueryRow(`SELECT transactions_revision FROM accounts WHERE id = ?`, accountID).Scan(&rev); err != nil {
		t.Fatalf("read transactions_revision: %v", err)
	}
	return rev
}

func requireRevision(t *testing.T, what string, got *int64, want int64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: transactionsRevision missing from the response", what)
	}
	if *got != want {
		t.Fatalf("%s: transactionsRevision = %d, want %d", what, *got, want)
	}
}

func TestAccountDTO_TransactionsRevisionMatchesColumn(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	id, created := h.createAccount(t, acctID1, "Cash", "100")
	requireRevision(t, "create item", created.TransactionsRevision, 1)
	if got := h.dbRevision(t, id); got != 1 {
		t.Fatalf("revision after create with an opening balance = %d, want 1", got)
	}

	if _, err := h.db.Exec(`UPDATE accounts SET transactions_revision = 7 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	list := mustUnmarshal[accountItemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list = %+v, want one account", list.Items)
	}
	requireRevision(t, "list item", list.Items[0].TransactionsRevision, 7)

	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": id, "name": "Cash", "balance": "250", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00",
	})
	if status != http.StatusOK {
		t.Fatalf("update = %d, want 200; body: %s", status, env.raw)
	}
	updated := mustUnmarshal[accountItemWrapper](t, env.Data)
	requireRevision(t, "update item", updated.Item.TransactionsRevision, 8)
	if got := h.dbRevision(t, id); got != 8 {
		t.Fatalf("revision after a balance change = %d, want 8 (7 + the correction)", got)
	}
}

func TestAccountUpdate_WithoutBalanceChange_LeavesRevision(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	id, _ := h.createAccount(t, acctID1, "Cash", "100")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": id, "name": "Renamed", "balance": "100", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00",
	})
	if status != http.StatusOK {
		t.Fatalf("update = %d, want 200; body: %s", status, env.raw)
	}
	requireRevision(t, "update item", mustUnmarshal[accountItemWrapper](t, env.Data).Item.TransactionsRevision, 1)
}
