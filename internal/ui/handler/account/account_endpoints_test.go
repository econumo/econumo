package account_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

const (
	acctID1 = "aaaa1111-0000-7000-8000-000000000001"
	acctID2 = "aaaa1111-0000-7000-8000-000000000002"
)

type accountItemWrapper struct {
	Item accountItem `json:"item"`
}
type accountItemsWrapper struct {
	Items []accountItem `json:"items"`
}

func createAccountReq(id, name, balance string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "currencyId": usdID,
		"balance": balance, "icon": "wallet", "folderId": seedFolderID,
	}
}

// createAccount POSTs create-account and returns the SERVER-MINTED entity id from
// the {item} response. The request `id` is only the operation id, so callers must
// not assume the entity id equals the request id.
func (h *harness) createAccount(t *testing.T, opID, name, balance string) (string, accountItem) {
	t.Helper()
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(opID, name, balance))
	if status != http.StatusOK {
		t.Fatalf("create-account = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	if res.Item.ID == "" {
		t.Fatalf("create-account returned no item id; body: %s", env.raw)
	}
	return res.Item.ID, res.Item
}

func TestCreateAccount_Success_EmbedsOwnerCurrencyFolder(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Cash", "0"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	it := res.Item
	// The entity id is server-minted (UUIDv7), NOT the request/operation id.
	if it.ID == "" || it.ID == acctID1 {
		t.Fatalf("entity id = %q, want a fresh server-minted id (not the request id %q)", it.ID, acctID1)
	}
	if it.Name != "Cash" {
		t.Fatalf("item name = %q, want Cash", it.Name)
	}
	if it.Owner.ID != seedUserID || it.Owner.Name != seedName || it.Owner.Avatar != seedAvatar {
		t.Fatalf("owner embed = %+v, want seed user", it.Owner)
	}
	if it.Currency.Code != "USD" || it.Currency.Name != "US Dollar" || it.Currency.Symbol != "$" {
		t.Fatalf("currency embed = %+v, want USD/US Dollar/$", it.Currency)
	}
	if it.FolderID == nil || *it.FolderID != seedFolderID {
		t.Fatalf("folderId = %v, want %s", it.FolderID, seedFolderID)
	}
	if it.Type != 2 { // always CREDIT_CARD on create
		t.Fatalf("type = %d, want 2 (credit card)", it.Type)
	}
	if it.Balance != "0" {
		t.Fatalf("balance = %q, want 0", it.Balance)
	}
	if it.SharedAccess == nil {
		t.Fatalf("sharedAccess must be [] not null; body: %s", env.raw)
	}
	// The create-account result is {item} ONLY — there is no accounts list.
	var rawObj map[string]json.RawMessage
	mustDecode(t, env.Data, &rawObj)
	if _, ok := rawObj["accounts"]; ok {
		t.Fatalf("create-account response must not carry an 'accounts' field ({item} only); body: %s", env.raw)
	}
}

func TestCreateAccount_WithBalance_WritesCorrection(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// Create with a non-zero balance -> a correction transaction makes the
	// computed balance match, and that transaction is returned so the client can
	// show it immediately (same full shape as update-account's).
	_, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Savings", "150.50"))
	var res struct {
		Item        accountItem `json:"item"`
		Transaction *struct {
			ID                 string  `json:"id"`
			Type               string  `json:"type"`
			AccountID          string  `json:"accountId"`
			AccountRecipientID *string `json:"accountRecipientId"`
			Amount             string  `json:"amount"`
			AmountRecipient    string  `json:"amountRecipient"`
			CategoryID         *string `json:"categoryId"`
			Description        string  `json:"description"`
			PayeeID            *string `json:"payeeId"`
			TagID              *string `json:"tagId"`
			Date               string  `json:"date"`
			Author             struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"author"`
		} `json:"transaction"`
	}
	mustDecode(t, env.Data, &res)
	if res.Item.Balance != "150.5" {
		t.Fatalf("balance = %q, want 150.5 (normalized)", res.Item.Balance)
	}
	if res.Transaction == nil {
		t.Fatalf("expected an opening-balance correction transaction; body: %s", env.raw)
	}
	tr := res.Transaction
	if tr.Type != "income" || tr.Amount != "150.5" || tr.AmountRecipient != "150.5" {
		t.Fatalf("correction type/amount/amountRecipient = %s/%s/%s, want income/150.5/150.5", tr.Type, tr.Amount, tr.AmountRecipient)
	}
	if tr.Author.ID != seedUserID || tr.Author.Name != seedName {
		t.Fatalf("correction author = %+v, want seed user", tr.Author)
	}
	if tr.AccountID != res.Item.ID {
		t.Fatalf("correction accountId = %q, want the created account %q", tr.AccountID, res.Item.ID)
	}
	if tr.Description != "" {
		t.Fatalf("description = %q, want empty for an opening-balance correction", tr.Description)
	}
	if tr.AccountRecipientID != nil || tr.CategoryID != nil || tr.PayeeID != nil || tr.TagID != nil {
		t.Fatalf("correction recipient/category/payee/tag must all be null; got %+v", tr)
	}
	if tr.Date == "" {
		t.Fatalf("correction date must be set")
	}

	// And it shows in the list.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	list := mustUnmarshal[accountItemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].Balance != "150.5" {
		t.Fatalf("list = %+v, want one account balance 150.5", list.Items)
	}
}

func TestCreateAccount_DuplicateId_Idempotency_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Cash", "0")); st != http.StatusOK {
		t.Fatalf("first create = %d; body: %s", st, env.raw)
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Other", "0"))
	if status != http.StatusBadRequest {
		t.Fatalf("duplicate-id create = %d, want 400; body: %s", status, env.raw)
	}
}

func TestCreateAccount_ShortName_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "ab", "0"))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if msgs := env.errorsMap()["name"]; len(msgs) == 0 || msgs[0] != "Account name must be 3-64 characters" {
		t.Fatalf("name error = %v, want exact account-name message", env.errorsMap()["name"])
	}
}

func TestUpdateAccount_BalanceCorrection(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	acctID, _ := h.createAccount(t, acctID1, "Cash", "100")

	// Update balance to 250 -> correction of 150 (income), transaction returned.
	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": acctID, "name": "Cash", "balance": "250", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00",
	})
	if status != http.StatusOK {
		t.Fatalf("update = %d, want 200; body: %s", status, env.raw)
	}
	var res struct {
		Item        accountItem `json:"item"`
		Transaction *struct {
			ID                 string  `json:"id"`
			Type               string  `json:"type"`
			AccountID          string  `json:"accountId"`
			AccountRecipientID *string `json:"accountRecipientId"`
			Amount             string  `json:"amount"`
			AmountRecipient    string  `json:"amountRecipient"`
			CategoryID         *string `json:"categoryId"`
			Description        string  `json:"description"`
			PayeeID            *string `json:"payeeId"`
			TagID              *string `json:"tagId"`
			Date               string  `json:"date"`
			Author             struct {
				ID     string `json:"id"`
				Avatar string `json:"avatar"`
				Name   string `json:"name"`
			} `json:"author"`
		} `json:"transaction"`
	}
	mustDecode(t, env.Data, &res)
	if res.Item.Balance != "250" {
		t.Fatalf("balance after update = %q, want 250", res.Item.Balance)
	}
	if res.Transaction == nil {
		t.Fatalf("expected a correction transaction; body: %s", env.raw)
	}
	tr := res.Transaction
	if tr.Type != "income" || tr.Amount != "150" {
		t.Fatalf("correction type/amount = %s/%s, want income/150", tr.Type, tr.Amount)
	}
	// The correction transaction is the full transaction result shape: author
	// embedded, amountRecipient = amount, the auto comment, and null
	// account-recipient/category/payee/tag.
	if tr.Author.ID != seedUserID || tr.Author.Name != seedName {
		t.Fatalf("correction author = %+v, want seed user", tr.Author)
	}
	if tr.AccountID != acctID {
		t.Fatalf("correction accountId = %q, want %q", tr.AccountID, acctID)
	}
	if tr.AmountRecipient != "150" {
		t.Fatalf("amountRecipient = %q, want 150 (falls back to amount)", tr.AmountRecipient)
	}
	if tr.Description != "Balance adjustment" {
		t.Fatalf("description = %q, want \"Balance adjustment\"", tr.Description)
	}
	if tr.AccountRecipientID != nil || tr.CategoryID != nil || tr.PayeeID != nil || tr.TagID != nil {
		t.Fatalf("correction recipient/category/payee/tag must all be null; got %+v", tr)
	}
	if tr.Date != "2024-01-01 12:00:00" {
		t.Fatalf("date = %q, want the request updatedAt", tr.Date)
	}
}

func TestUpdateAccount_NoBalanceChange_NullTransaction(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "100")

	_, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": acctID, "name": "Renamed", "balance": "100", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00",
	})
	var res struct {
		Item        accountItem `json:"item"`
		Transaction *any        `json:"transaction"`
	}
	mustDecode(t, env.Data, &res)
	if res.Item.Name != "Renamed" {
		t.Fatalf("name = %q, want Renamed", res.Item.Name)
	}
	if res.Transaction != nil {
		t.Fatalf("transaction should be null when balance unchanged; body: %s", env.raw)
	}
}

func TestDeleteAccount_SoftDelete_RemovesFromList(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")

	status, _ := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": acctID})
	if status != http.StatusOK {
		t.Fatalf("delete = %d, want 200", status)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	list := mustUnmarshal[accountItemsWrapper](t, listEnv.Data)
	if len(list.Items) != 0 {
		t.Fatalf("list after delete = %+v, want empty (soft-deleted excluded)", list.Items)
	}
}

func TestDeleteAccount_NotOwned_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// Seed an account owned by the other user directly.
	h.seedAccount(t, acctID2, otherUserID, "Theirs")
	status, _ := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": acctID2})
	if status != http.StatusForbidden {
		t.Fatalf("delete not-owned = %d, want 403", status)
	}
}

func TestGetAccountList_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}
