package api_test

// Write-access coverage for recurring templates on accounts SHARED with the
// caller by another user. Mirrors internal/transaction/api/shared_access_test.go:
// write access (create/update/skip/post/delete a recurring template) requires
// owner or an admin/user grant; no grant or a guest grant is denied. Regression
// guard against the recurring service silently trusting the account id on the
// request instead of checking access on it.

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	recOwnerTwoID    = "22222222-3333-2222-3333-222222222222"
	recOwnerTwoEmail = "recowner2@example.test"
	recOwnerTwoSalt  = "0000000000000000000000000000000000000003"
	recSharedAcctID  = "aaaa3333-0000-0000-0000-0000000000a3"

	// roles (admin=0, user=1, guest=2) — matches connection.Role.
	recRoleGuest = 2
)

// shareAccount seeds a second user who owns recSharedAcctID and grants the
// seed user the given role on it. With grant==false no accounts_access row is
// created (the account is simply owned by another user, invisible to the
// seed user).
func (h *harness) shareAccount(t *testing.T, role int, grant bool) {
	t.Helper()
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: recOwnerTwoID, Email: recOwnerTwoEmail, Name: "Rec Owner Two", Avatar: "https://avatar.test/ro2", Password: "pw", Salt: recOwnerTwoSalt})
	f.Account(fixture.Account{ID: recSharedAcctID, UserID: recOwnerTwoID, CurrencyID: usdID, Name: "Shared"})
	if grant {
		f.AccountAccess(recSharedAcctID, seedUserID, role)
	}
}

func sharedCreateRecurringReq(opID, amount string) map[string]any {
	return map[string]any{
		"id": opID, "type": "expense", "amount": amount,
		"accountId": recSharedAcctID, "schedule": "monthly",
		"nextPaymentAt": "2026-08-31 00:00:00", "description": "rent",
	}
}

func TestCreateRecurringTransaction_SharedAccount_NoGrant_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, 0, false) // account owned by another user, no grant to seed user
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, sharedCreateRecurringReq("0197c300-0000-7000-8000-000000000001", "10"))
	assertValidationDenied(t, status, env, "account.account.not_available")
}

func TestSkipRecurringTransaction_SharedAccount_GuestRole_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, recRoleGuest, true)
	ownerTok := recOwnerTwoID // authstub: the bearer token IS the user id string.

	// The template is created by the account's OWNER (recOwnerTwoID), not the
	// guest-role seed user.
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", ownerTok, sharedCreateRecurringReq("0197c300-0000-7000-8000-000000000002", "10"))
	if status != http.StatusOK {
		t.Fatalf("owner create failed: status=%d body=%s", status, env.raw)
	}
	item := mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data).Item

	tok := h.token(t) // seed user, guest role on recSharedAcctID
	status, env = h.do(t, http.MethodPost, "/api/v1/recurring/skip-recurring-transaction", tok, map[string]any{"id": item.ID})
	assertValidationDenied(t, status, env, "account.account.not_available")
}

func TestUpdateRecurringTransaction_MoveToUnwritableAccount_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, recRoleGuest, true) // seed user has only a guest (read-only) grant on recSharedAcctID
	tok := h.token(t)

	// Template starts on the seed user's own (writable) account.
	item := createTemplate(t, h, tok)

	body := map[string]any{
		"id": item.ID, "type": "expense", "amount": "99",
		"accountId": recSharedAcctID, "schedule": "weekly",
		"nextPaymentAt": "2026-09-05 00:00:00", "description": "moved",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/update-recurring-transaction", tok, body)
	assertValidationDenied(t, status, env, "account.account.not_available")

	// The template must be unchanged (still on the original account).
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	list := mustUnmarshal[recurringList](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].AccountID != accountID {
		t.Fatalf("template should remain on the original account; items=%+v", list.Items)
	}
}

// assertValidationDenied checks the denial envelope: HTTP 400, success false,
// and the fieldless *ValidationError's own message on the wire (an i18n key
// the frontend localizes).
func assertValidationDenied(t *testing.T, status int, env envelope, wantMsg string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (access denied); body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("success=true, want false; body: %s", env.raw)
	}
	if env.Message != wantMsg {
		t.Fatalf("message = %q, want %q", env.Message, wantMsg)
	}
}
