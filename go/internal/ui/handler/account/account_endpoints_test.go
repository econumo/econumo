package account_test

import (
	"net/http"
	"testing"
)

const (
	acctID1 = "aaaa1111-0000-7000-8000-000000000001"
	acctID2 = "aaaa1111-0000-7000-8000-000000000002"
)

type accountItemWrapper struct {
	Item     accountItem   `json:"item"`
	Accounts []accountItem `json:"accounts"`
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

func TestCreateAccount_Success_EmbedsOwnerCurrencyFolder(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Cash", "0"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	it := res.Item
	if it.ID != acctID1 || it.Name != "Cash" {
		t.Fatalf("item id/name = %q/%q", it.ID, it.Name)
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
	if len(res.Accounts) != 1 {
		t.Fatalf("accounts list len = %d, want 1", len(res.Accounts))
	}
}

func TestCreateAccount_WithBalance_WritesCorrection(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	// Create with a non-zero balance -> a correction transaction makes the
	// computed balance match.
	_, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Savings", "150.50"))
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	if res.Item.Balance != "150.5" {
		t.Fatalf("balance = %q, want 150.5 (normalized)", res.Item.Balance)
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

	h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Cash", "100"))

	// Update balance to 250 -> correction of 150 (income), transaction returned.
	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": acctID1, "name": "Cash", "balance": "250", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00",
	})
	if status != http.StatusOK {
		t.Fatalf("update = %d, want 200; body: %s", status, env.raw)
	}
	var res struct {
		Item        accountItem `json:"item"`
		Transaction *struct {
			Type   string `json:"type"`
			Amount string `json:"amount"`
		} `json:"transaction"`
	}
	mustDecode(t, env.Data, &res)
	if res.Item.Balance != "250" {
		t.Fatalf("balance after update = %q, want 250", res.Item.Balance)
	}
	if res.Transaction == nil {
		t.Fatalf("expected a correction transaction; body: %s", env.raw)
	}
	if res.Transaction.Type != "income" || res.Transaction.Amount != "150" {
		t.Fatalf("correction = %+v, want income/150", *res.Transaction)
	}
}

func TestUpdateAccount_NoBalanceChange_NullTransaction(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Cash", "100"))

	_, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": acctID1, "name": "Renamed", "balance": "100", "icon": "wallet",
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
	h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, createAccountReq(acctID1, "Cash", "0"))

	status, _ := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": acctID1})
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
