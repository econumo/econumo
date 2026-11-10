package api_test

import (
	"net/http"
	"testing"
)

// Settings is the only place a template can be edited or deleted, so one on an
// account in a hidden folder must still be listed.
func TestGetRecurringTransactionList_IncludesHiddenFolderAccounts(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const opID = "0197c400-0000-7000-8000-0000000000f1"
	if status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok,
		createRecurringReqWithLabels(opID, "expense", "10", []string{})); status != http.StatusOK {
		t.Fatalf("create status = %d, env = %+v", status, env)
	}
	if _, err := h.db.Exec(`UPDATE folders SET is_visible = 0 WHERE id = ?`, folderID); err != nil {
		t.Fatalf("hide folder: %v", err)
	}

	status, env := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("list status = %d, env = %+v", status, env)
	}
	list := mustUnmarshal[recurringList](t, env.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list = %+v, want the template on the hidden account", list.Items)
	}
}
