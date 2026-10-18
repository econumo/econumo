package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// Deleting a template sets recurring_id to NULL on the transactions it posted,
// which changes the rows the transaction list returns for their accounts.
func TestRevision_DeleteRecurring_BumpsAccountsOfPostedTransactions(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok)

	f := fixture.New(t, h.tdb)
	a := f.Account(fixture.Account{UserID: seedUserID, Name: "A"})
	b := f.Account(fixture.Account{UserID: seedUserID, Name: "B"})
	c := f.Account(fixture.Account{UserID: seedUserID, Name: "C"})
	posted := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: a, Amount: "10"})
	postedTransfer := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: a, AccountRecipientID: b, Type: 2, Amount: "5", AmountRecipient: "5"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: c, Amount: "7"})
	for _, id := range []string{posted, postedTransfer} {
		if _, err := h.db.Exec(`UPDATE transactions SET recurring_id = ? WHERE id = ?`, item.ID, id); err != nil {
			t.Fatalf("link transaction to template: %v", err)
		}
	}
	before := f.Revisions(a, b, c)

	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/delete-recurring-transaction", tok, map[string]any{"id": item.ID})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}

	f.RequireRevisionsMoved(before, a, b)
}
