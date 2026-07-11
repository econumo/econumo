package api_test

// Transfer amount_recipient integrity: the recipient account's balance is
// SUM(amount_recipient), and the reporting queries classify a transfer leg as a
// currency exchange via amount != amount_recipient. The server therefore
// normalizes what clients send: a same-currency transfer always stores
// amount_recipient = amount (stale/missing client values would silently corrupt
// the recipient balance), and a cross-currency transfer must carry an explicit
// amountRecipient (defaulting would fabricate an exchange rate).

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usd2AcctID = "aaaa3333-0000-0000-0000-0000000000a3"
	eurAcctID  = "aaaa4444-0000-0000-0000-0000000000a4"
)

// seedTransferAccounts adds two more seed-user accounts: a second USD account
// and a EUR one, both in the main folder.
func (h *harness) seedTransferAccounts(t *testing.T) {
	t.Helper()
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.Account(fixture.Account{ID: usd2AcctID, UserID: seedUserID, CurrencyID: usdID, Name: "Bank"})
	f.AccountInFolder(folderID, usd2AcctID)
	f.AccountOption(usd2AcctID, seedUserID, 1)
	eurID := f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	f.Account(fixture.Account{ID: eurAcctID, UserID: seedUserID, CurrencyID: eurID, Name: "Euro Stash"})
	f.AccountInFolder(folderID, eurAcctID)
	f.AccountOption(eurAcctID, seedUserID, 2)
}

// transferReq builds a create/update-transaction transfer body; amountRecipient
// is included only when non-nil.
func transferReq(id, from, to, amount string, amountRecipient any) map[string]any {
	m := map[string]any{
		"id": id, "type": "transfer", "amount": amount, "accountId": from,
		"accountRecipientId": to, "date": "2024-03-01 10:00:00", "description": "move",
	}
	if amountRecipient != nil {
		m["amountRecipient"] = amountRecipient
	}
	return m
}

func balanceOf(t *testing.T, res writeResult, accountID string) string {
	t.Helper()
	for _, a := range res.Accounts {
		if a.ID == accountID {
			return a.Balance
		}
	}
	t.Fatalf("account %s not in embed: %+v", accountID, res.Accounts)
	return ""
}

func TestCreateTransfer_SameCurrency_MismatchedRecipientAmountNormalized(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		transferReq(txID1, accountID, usd2AcctID, "100", "50"))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.AmountRecipient == nil || *res.Item.AmountRecipient != "100" {
		t.Fatalf("amountRecipient = %v, want 100 (normalized to amount)", res.Item.AmountRecipient)
	}
	if got := balanceOf(t, res, usd2AcctID); got != "100" {
		t.Fatalf("recipient balance = %q, want 100", got)
	}
	if got := balanceOf(t, res, accountID); got != "-100" {
		t.Fatalf("source balance = %q, want -100", got)
	}
}

func TestCreateTransfer_SameCurrency_MissingRecipientAmountDefaults(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		transferReq(txID1, accountID, usd2AcctID, "100", nil))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if got := balanceOf(t, res, usd2AcctID); got != "100" {
		t.Fatalf("recipient balance = %q, want 100 (NULL amount_recipient credits nothing)", got)
	}
}

func TestCreateTransfer_CrossCurrency_MissingRecipientAmount400(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		transferReq(txID1, accountID, eurAcctID, "100", nil))
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body: %s", status, env.raw)
	}
	msgs := env.errorsMap()["amountRecipient"]
	if len(msgs) != 1 || msgs[0] != "This value should not be blank." {
		t.Fatalf("errors.amountRecipient = %v, want the blank-value message", msgs)
	}
}

func TestCreateTransfer_CrossCurrency_ClientRecipientAmountPreserved(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		transferReq(txID1, accountID, eurAcctID, "100", "90"))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.AmountRecipient == nil || *res.Item.AmountRecipient != "90" {
		t.Fatalf("amountRecipient = %v, want 90 (client value kept cross-currency)", res.Item.AmountRecipient)
	}
	if got := balanceOf(t, res, eurAcctID); got != "90" {
		t.Fatalf("recipient balance = %q, want 90", got)
	}
}

// The reported production bug: editing a same-currency transfer's amount while
// the client re-sends the stale recipient amount must not freeze the recipient
// account's balance at the old value.
func TestUpdateTransfer_SameCurrency_StaleRecipientAmountNormalized(t *testing.T) {
	h := newHarness(t)
	h.seedTransferAccounts(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		transferReq(txID1, accountID, usd2AcctID, "10", "10"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	upd := transferReq(created.Item.ID, accountID, usd2AcctID, "25", "10")
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok, upd)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.AmountRecipient == nil || *res.Item.AmountRecipient != "25" {
		t.Fatalf("amountRecipient = %v, want 25 (normalized to amount)", res.Item.AmountRecipient)
	}
	if got := balanceOf(t, res, usd2AcctID); got != "25" {
		t.Fatalf("recipient balance = %q, want 25 (stale 10 must not survive)", got)
	}
}
