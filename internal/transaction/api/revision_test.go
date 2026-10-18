package api_test

// accounts.transactions_revision is what clients compare to decide which
// accounts' transactions to download again, so a request must move each
// account it touched by exactly one, whatever number of rows it wrote.

import (
	"net/http"
	"testing"
)

type revisionAccounts struct {
	Accounts []struct {
		ID                   string `json:"id"`
		TransactionsRevision *int64 `json:"transactionsRevision"`
	} `json:"accounts"`
}

func (h *harness) revision(t *testing.T, accountID string) int64 {
	t.Helper()
	var rev int64
	if err := h.db.QueryRow(`SELECT transactions_revision FROM accounts WHERE id = ?`, accountID).Scan(&rev); err != nil {
		t.Fatalf("read transactions_revision of %s: %v", accountID, err)
	}
	return rev
}

func (h *harness) revisions(t *testing.T, ids ...string) map[string]int64 {
	t.Helper()
	out := make(map[string]int64, len(ids))
	for _, id := range ids {
		out[id] = h.revision(t, id)
	}
	return out
}

func requireMoved(t *testing.T, h *harness, before map[string]int64, moved ...string) {
	t.Helper()
	isMoved := map[string]bool{}
	for _, id := range moved {
		isMoved[id] = true
	}
	for id, was := range before {
		want := was
		if isMoved[id] {
			want++
		}
		if got := h.revision(t, id); got != want {
			t.Errorf("account %s: transactions_revision = %d, want %d (before %d, moved=%v)", id, got, want, was, isMoved[id])
		}
	}
}

func requireResponseRevisions(t *testing.T, h *harness, env envelope) {
	t.Helper()
	res := mustUnmarshal[revisionAccounts](t, env.Data)
	if len(res.Accounts) == 0 {
		t.Fatalf("response carries no accounts: %s", env.raw)
	}
	for _, a := range res.Accounts {
		if a.TransactionsRevision == nil {
			t.Fatalf("account %s: transactionsRevision missing from the response", a.ID)
		}
		if want := h.revision(t, a.ID); *a.TransactionsRevision != want {
			t.Errorf("account %s: response transactionsRevision = %d, DB = %d", a.ID, *a.TransactionsRevision, want)
		}
	}
}

func (h *harness) post(t *testing.T, path string, body map[string]any) envelope {
	t.Helper()
	status, env := h.do(t, http.MethodPost, path, h.token(t), body)
	if status != http.StatusOK {
		t.Fatalf("%s: status=%d body=%s", path, status, env.raw)
	}
	return env
}

func TestRevision_CreateWithLabels_BumpsOncePerAccount(t *testing.T) {
	h := newHarness(t)
	before := h.revisions(t, accountID)

	env := h.post(t, "/api/v1/transaction/create-transaction",
		createReqWithLabels(txID1, "expense", "10", []string{label1ID, label2ID}))

	requireMoved(t, h, before, accountID)
	requireResponseRevisions(t, h, env)
}

func TestRevision_CreateTransfer_BumpsBothSides(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, delAcctID, "Vault", 1)
	before := h.revisions(t, accountID, delAcctID)

	env := h.post(t, "/api/v1/transaction/create-transaction", transferBody(txID1, accountID, delAcctID, "5", transferDate))

	requireMoved(t, h, before, accountID, delAcctID)
	requireResponseRevisions(t, h, env)
}

func TestRevision_UpdateExpenseAcrossAccounts_BumpsOldAndNew(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, delAcctID, "B", 1)
	h.seedAccount(t, otherAcctID, "C", 2)
	id := mustUnmarshal[writeResult](t, h.post(t, "/api/v1/transaction/create-transaction", createReq(txID1, "expense", "10")).Data).Item.ID
	before := h.revisions(t, accountID, delAcctID, otherAcctID)

	env := h.post(t, "/api/v1/transaction/update-transaction", map[string]any{
		"id": id, "type": "expense", "amount": "10", "accountId": otherAcctID,
		"categoryId": catID, "date": "2024-03-01 10:00:00",
	})

	requireMoved(t, h, before, accountID, otherAcctID)
	requireResponseRevisions(t, h, env)
}

func TestRevision_UpdateTransferRecipient_BumpsOldAndNewRecipient(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, delAcctID, "B", 1)
	h.seedAccount(t, otherAcctID, "C", 2)
	id := h.createTransfer(t, txID1, accountID, delAcctID, "5", transferDate)
	before := h.revisions(t, accountID, delAcctID, otherAcctID)

	env := h.post(t, "/api/v1/transaction/update-transaction", transferBody(id, accountID, otherAcctID, "5", transferDate))

	requireMoved(t, h, before, accountID, delAcctID, otherAcctID)
	requireResponseRevisions(t, h, env)
}

func TestRevision_Delete_BumpsAccount(t *testing.T) {
	h := newHarness(t)
	id := mustUnmarshal[writeResult](t, h.post(t, "/api/v1/transaction/create-transaction", createReq(txID1, "expense", "10")).Data).Item.ID
	before := h.revisions(t, accountID)

	status, env := h.deleteTransaction(t, id)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}

	requireMoved(t, h, before, accountID)
	requireResponseRevisions(t, h, env)
}

func TestRevision_DeleteTransferToDeletedAccount_BumpsLiveSideOnce(t *testing.T) {
	h := newHarness(t)
	txn := h.seedTransferToDeleted(t)
	before := h.revisions(t, accountID)

	status, env := h.deleteTransaction(t, txn)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}

	requireMoved(t, h, before, accountID)
	requireResponseRevisions(t, h, env)
}

func TestRevision_CSVImport_BumpsOncePerRequest(t *testing.T) {
	h := newHarness(t)
	before := h.revisions(t, accountID)

	csv := "Account,Date,Amount,Category,Note,Payee\n" +
		"Cash,2024-03-01,-1,Food,a,\n" +
		"Cash,2024-03-02,-2,Food,b,\n" +
		"Cash,2024-03-03,-3,Food,c,\n"
	status, env := h.doImport(t, h.token(t), csv, importMapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if res := mustUnmarshal[importResult](t, env.Data); res.Imported != 3 {
		t.Fatalf("imported = %d, want 3; errors=%v", res.Imported, res.Errors)
	}

	requireMoved(t, h, before, accountID)
}
