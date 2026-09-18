package api_test

// Issue #261: a transfer stored with a NULL account_recipient_id shows up as
// "Transfer to [Hidden account]" and credits no account. Both write endpoints
// must refuse a transfer that names no recipient.

import (
	"net/http"
	"testing"
)

func transferWithoutRecipient(id string) map[string]any {
	return map[string]any{
		"id": id, "type": "transfer", "amount": "300", "accountId": accountID,
		"date": "2024-03-01 10:00:00", "description": "move",
	}
}

func assertRecipientBlank(t *testing.T, status int, env envelope) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body: %s", status, env.raw)
	}
	msgs := env.errorsMap()["accountRecipientId"]
	if len(msgs) != 1 || msgs[0] != "This value should not be blank." {
		t.Fatalf("errors.accountRecipientId = %v, want the blank-value message; body: %s", msgs, env.raw)
	}
}

func TestCreateTransfer_WithoutRecipient_400(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, transferWithoutRecipient(txID1))
	assertRecipientBlank(t, status, env)
}

func TestUpdateTransfer_WithoutRecipient_400(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	status, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		transferReq(txID1, accountID, usd2AcctID, "100", nil))
	if status != http.StatusOK {
		t.Fatalf("seed create status=%d; body: %s", status, cEnv.raw)
	}
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok, transferWithoutRecipient(created.Item.ID))
	assertRecipientBlank(t, status, env)
}
