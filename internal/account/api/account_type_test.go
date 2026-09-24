package api_test

import (
	"net/http"
	"testing"
)

func TestCreateAccount_WithType_Savings(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	req := createAccountReq(acctID1, "Savings", "0")
	req["type"] = 3
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, req)
	if status != http.StatusOK {
		t.Fatalf("create with type=3 = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	if res.Item.Type != 3 {
		t.Fatalf("item.type = %d, want 3", res.Item.Type)
	}
}

func TestCreateAccount_WithoutType_DefaultsToCreditCard(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	req := createAccountReq(acctID1, "Cash", "0")
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, req)
	if status != http.StatusOK {
		t.Fatalf("create without type = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	if res.Item.Type != 2 {
		t.Fatalf("item.type = %d, want 2 (default credit card)", res.Item.Type)
	}
}

func TestUpdateAccount_WithType_ChangesType(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": acctID, "name": "Cash", "balance": "0", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00", "type": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("update with type=1 = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	if res.Item.Type != 1 {
		t.Fatalf("item.type = %d, want 1", res.Item.Type)
	}
}

func TestUpdateAccount_WithoutType_Unchanged(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, created := h.createAccount(t, acctID1, "Cash", "0")
	if created.Type != 2 {
		t.Fatalf("precondition: created type = %d, want 2", created.Type)
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": acctID, "name": "Cash", "balance": "0", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00",
	})
	if status != http.StatusOK {
		t.Fatalf("update without type = %d, want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	if res.Item.Type != 2 {
		t.Fatalf("item.type = %d, want 2 (unchanged)", res.Item.Type)
	}
}

func TestCreateAccount_InvalidType_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	req := createAccountReq(acctID1, "Cash", "0")
	req["type"] = 7
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, req)
	if status != http.StatusBadRequest {
		t.Fatalf("create with type=7 = %d, want 400; body: %s", status, env.raw)
	}
	if msgs := env.errorsMap()["type"]; len(msgs) == 0 || msgs[0] != "Account type must be 1, 2 or 3" {
		t.Fatalf("type error = %v, want exact account-invalid-type message", env.errorsMap()["type"])
	}
}

func TestUpdateAccount_InvalidType_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": acctID, "name": "Cash", "balance": "0", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00", "type": 7,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("update with type=7 = %d, want 400; body: %s", status, env.raw)
	}
	if msgs := env.errorsMap()["type"]; len(msgs) == 0 || msgs[0] != "Account type must be 1, 2 or 3" {
		t.Fatalf("type error = %v, want exact account-invalid-type message", env.errorsMap()["type"])
	}
}
