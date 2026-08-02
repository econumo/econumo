package api_test

// create-recurring-transaction can name an existing transaction as its source
// (the "make recurring from this transaction" flow). The source is linked to
// the new template in the same write, so it immediately reads as the series'
// first instance. Access on the source row is checked like any transaction
// write, and a denied link rolls the template back with it.

import (
	"net/http"
	"testing"
)

type txListItem struct {
	ID          string  `json:"id"`
	RecurringId *string `json:"recurringId"`
}

// createSourceTransaction creates a transaction (opID is the idempotency id)
// and returns the freshly minted entity id.
func createSourceTransaction(t *testing.T, h *harness, tok, opID string, body map[string]any) string {
	t.Helper()
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("create source transaction: status=%d body=%s", status, env.raw)
	}
	item := mustUnmarshal[struct {
		Item txListItem `json:"item"`
	}](t, env.Data).Item
	if item.ID == "" || item.ID == opID {
		t.Fatalf("entity id must be fresh, got %q", item.ID)
	}
	return item.ID
}

func (h *harness) transactionByID(t *testing.T, tok, txID string) txListItem {
	t.Helper()
	_, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	list := mustUnmarshal[struct {
		Items []txListItem `json:"items"`
	}](t, env.Data)
	for _, item := range list.Items {
		if item.ID == txID {
			return item
		}
	}
	t.Fatalf("transaction %s not in list", txID)
	return txListItem{}
}

func TestCreateRecurringTransaction_FromTransaction_LinksSource(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const opTx = "0197c400-0000-7000-8000-000000000001"
	srcTxID := createSourceTransaction(t, h, tok, opTx, map[string]any{
		"id": opTx, "type": "expense", "amount": "42.50",
		"accountId": accountID, "categoryId": catID,
		"date": "2026-07-31 10:00:00", "description": "rent",
	})

	req := createRecurringReq("0197c400-0000-7000-8000-000000000002", "expense", "42.50")
	req["sourceTransactionId"] = srcTxID
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, req)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	item := mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data).Item

	src := h.transactionByID(t, tok, srcTxID)
	if src.RecurringId == nil || *src.RecurringId != item.ID {
		t.Fatalf("source transaction recurringId = %v, want %q", src.RecurringId, item.ID)
	}
}

func TestCreateRecurringTransaction_FromForeignTransaction_DeniedAndRolledBack(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, 0, false) // recSharedAcctID owned by another user, invisible to the seed user
	ownerTok := recOwnerTwoID   // authstub: the bearer token IS the user id string.

	// The foreign user's transaction, on their own account.
	const opTx = "0197c400-0000-7000-8000-000000000003"
	foreignTxID := createSourceTransaction(t, h, ownerTok, opTx, map[string]any{
		"id": opTx, "type": "expense", "amount": "10.00",
		"accountId": recSharedAcctID, "date": "2026-07-31 10:00:00", "description": "theirs",
	})

	// The template names the CALLER's own (writable) account; only the check on
	// the source row stands between a valid foreign UUID and tagging a
	// stranger's transaction.
	tok := h.token(t)
	req := createRecurringReq("0197c400-0000-7000-8000-000000000004", "expense", "10")
	req["sourceTransactionId"] = foreignTxID
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, req)
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")

	// The denied link must roll the template back with it.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	if list := mustUnmarshal[recurringList](t, listEnv.Data); len(list.Items) != 0 {
		t.Fatalf("denied link must not leave a template behind; items=%+v", list.Items)
	}

	// And the foreign transaction stays unlinked.
	if src := h.transactionByID(t, ownerTok, foreignTxID); src.RecurringId != nil {
		t.Fatalf("foreign transaction was linked: recurringId=%q", *src.RecurringId)
	}
}

func TestCreateRecurringTransaction_FromMissingTransaction_Fails(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	req := createRecurringReq("0197c400-0000-7000-8000-000000000005", "expense", "10")
	req["sourceTransactionId"] = "0197c400-0000-7000-8000-000000000006"
	status, _ := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, req)
	if status == http.StatusOK {
		t.Fatalf("create with a missing source transaction must fail")
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	if list := mustUnmarshal[recurringList](t, listEnv.Data); len(list.Items) != 0 {
		t.Fatalf("failed create must not leave a template behind; items=%+v", list.Items)
	}
}
