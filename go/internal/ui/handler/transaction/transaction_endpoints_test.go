package transaction_test

import (
	"net/http"
	"testing"
)

const txID1 = "bbbb1111-0000-7000-8000-000000000001"

type txItem struct {
	ID                 string                            `json:"id"`
	Author             struct{ ID, Avatar, Name string } `json:"author"`
	Type               string                            `json:"type"`
	AccountID          string                            `json:"accountId"`
	AccountRecipientID *string                           `json:"accountRecipientId"`
	Amount             string                            `json:"amount"`
	AmountRecipient    *string                           `json:"amountRecipient"`
	CategoryID         *string                           `json:"categoryId"`
	Description        string                            `json:"description"`
	PayeeID            *string                           `json:"payeeId"`
	TagID              *string                           `json:"tagId"`
	Date               string                            `json:"date"`
}

type writeResult struct {
	Item     txItem `json:"item"`
	Accounts []struct {
		ID      string `json:"id"`
		Balance string `json:"balance"`
	} `json:"accounts"`
}
type listResult struct {
	Items []txItem `json:"items"`
}

func createReq(id, typ, amount string) map[string]any {
	return map[string]any{"id": id, "type": typ, "amount": amount, "accountId": accountID, "categoryId": catID, "date": "2024-03-01 10:00:00", "description": "groceries"}
}

func TestCreateTransaction_Success_EmbedsAuthorAndAccounts(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "42.50"))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	it := res.Item
	// The entity id is server-minted (NOT the request/operation id txID1); PHP
	// mints a fresh id via getNextIdentity(). Just assert it is present and
	// differs from the operation id.
	if it.ID == "" || it.ID == txID1 || it.Type != "expense" || it.Amount != "42.5" {
		t.Fatalf("item = %+v, want fresh id != %s, expense/42.5", it, txID1)
	}
	if it.Author.ID != seedUserID || it.Author.Name != seedName {
		t.Fatalf("author = %+v, want seed user", it.Author)
	}
	if it.CategoryID == nil || *it.CategoryID != catID {
		t.Fatalf("categoryId = %v, want %s", it.CategoryID, catID)
	}
	// amountRecipient falls back to amount.
	if it.AmountRecipient == nil || *it.AmountRecipient != "42.5" {
		t.Fatalf("amountRecipient = %v, want 42.5 (fallback)", it.AmountRecipient)
	}
	// embedded account list reflects the new balance (expense -> -42.5).
	if len(res.Accounts) != 1 || res.Accounts[0].Balance != "-42.5" {
		t.Fatalf("accounts = %+v, want one balance -42.5", res.Accounts)
	}
}

func TestCreateTransaction_DuplicateId_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, e := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "income", "10")); st != 200 {
		t.Fatalf("first=%d body=%s", st, e.raw)
	}
	st, _ := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "income", "99"))
	if st != http.StatusBadRequest {
		t.Fatalf("dup=%d want 400", st)
	}
}

func TestGetTransactionList_ByAccount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "42.50"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	_, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+accountID, tok, nil)
	list := mustUnmarshal[listResult](t, env.Data)
	// The listed tx carries the server-minted id returned by create, not txID1.
	if len(list.Items) != 1 || list.Items[0].ID != created.Item.ID {
		t.Fatalf("list = %+v, want one tx with id %s", list.Items, created.Item.ID)
	}
}

func TestUpdateTransaction_ChangesAmount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "42.50"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	// update targets the server-minted entity id, not the create operation id.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok, map[string]any{
		"id": created.Item.ID, "type": "income", "amount": "100", "accountId": accountID, "categoryId": catID,
		"date": "2024-03-02 10:00:00", "description": "refund",
	})
	if status != http.StatusOK {
		t.Fatalf("update=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.Type != "income" || res.Item.Amount != "100" || res.Item.Description != "refund" {
		t.Fatalf("item = %+v, want income/100/refund", res.Item)
	}
	// income 100 -> balance +100.
	if res.Accounts[0].Balance != "100" {
		t.Fatalf("balance = %q, want 100", res.Accounts[0].Balance)
	}
}

func TestDeleteTransaction_RemovesAndRefreshes(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "42.50"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/delete-transaction", tok, map[string]any{"id": created.Item.ID})
	if status != http.StatusOK {
		t.Fatalf("delete=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	// deleted item returned (the server-minted id); balance back to 0.
	if res.Item.ID != created.Item.ID || res.Accounts[0].Balance != "0" {
		t.Fatalf("res = %+v, want deleted item %s + balance 0", res, created.Item.ID)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+accountID, tok, nil)
	if l := mustUnmarshal[listResult](t, listEnv.Data); len(l.Items) != 0 {
		t.Fatalf("list after delete = %+v, want empty", l.Items)
	}
}

func TestCreateTransaction_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", "", createReq(txID1, "expense", "10"))
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", status)
	}
}
