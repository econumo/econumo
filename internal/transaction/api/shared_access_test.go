package api_test

// Coverage for adding/updating/deleting transactions on accounts SHARED with the
// caller by another user, across every access level. Write access (add/update/
// delete transaction) is allowed for the owner and for connected users holding an
// admin or user grant; a guest grant (or no grant) is denied. Regression guard
// against reducing the check to owner-only, which would lock shared users out of
// creating transactions.

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	ownerTwoID    = "22222222-2222-2222-2222-222222222222"
	ownerTwoEmail = "owner2@example.test"
	ownerTwoSalt  = "0000000000000000000000000000000000000002"
	sharedAcctID  = "aaaa2222-0000-0000-0000-0000000000a2"
	// A category/payee owned by the shared account's OWNER (ownerTwo), not by the
	// caller — this is what the SPA presents on a shared account (its picker
	// filters to the account owner's entities).
	ownerCatID   = "cccc2222-0000-0000-0000-0000000000c2"
	ownerPayeeID = "dddd2222-0000-0000-0000-0000000000d2"

	// roles (admin=0, user=1, guest=2) — matches connection.Role.
	roleAdmin = 0
	roleUser  = 1
	roleGuest = 2
)

// shareAccount seeds a second user who owns sharedAcctID and grants the seed user
// the given role on it. With grant==false no accounts_access row is created (the
// account is simply owned by another user, invisible to the seed user).
func (h *harness) shareAccount(t *testing.T, role int, grant bool) {
	t.Helper()
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: ownerTwoID, Email: ownerTwoEmail, Name: "Owner Two", Avatar: "https://avatar.test/o2", Password: "pw", Salt: ownerTwoSalt})
	f.Account(fixture.Account{ID: sharedAcctID, UserID: ownerTwoID, CurrencyID: usdID, Name: "Shared"})
	// The owner's own category/payee — the SPA presents these (not the caller's)
	// when categorizing a transaction on the shared account.
	f.Category(fixture.Category{ID: ownerCatID, UserID: ownerTwoID, Name: "Owner Food", Type: 0, Icon: "local_offer"})
	f.Payee(fixture.Payee{ID: ownerPayeeID, UserID: ownerTwoID, Name: "Owner Market"})
	if grant {
		f.AccountAccess(sharedAcctID, seedUserID, role)
	}
}

// sharedCreateReq builds a create-transaction request targeting the shared account.
func sharedCreateReq(id, amount string) map[string]any {
	return map[string]any{"id": id, "type": "expense", "amount": amount, "accountId": sharedAcctID, "categoryId": catID, "date": "2024-03-01 10:00:00", "description": "shared"}
}

// shareAccountPending seeds a second user who owns sharedAcctID and grants the
// seed user a PENDING (not yet accepted) role on it — an unaccepted invite
// confers no access, so it must behave like no grant at all.
func (h *harness) shareAccountPending(t *testing.T, role int) {
	t.Helper()
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: ownerTwoID, Email: ownerTwoEmail, Name: "Owner Two", Avatar: "https://avatar.test/o2", Password: "pw", Salt: ownerTwoSalt})
	f.Account(fixture.Account{ID: sharedAcctID, UserID: ownerTwoID, CurrencyID: usdID, Name: "Shared"})
	f.AccountAccessPending(sharedAcctID, seedUserID, role)
}

func TestCreateTransaction_SharedAccount_AdminRole_Succeeds(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleAdmin, true)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, sharedCreateReq(txID1, "10"))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.AccountID != sharedAcctID {
		t.Fatalf("accountId = %q, want shared account %q", res.Item.AccountID, sharedAcctID)
	}
}

func TestCreateTransaction_SharedAccount_UserRole_Succeeds(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleUser, true)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, sharedCreateReq(txID1, "10"))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.AccountID != sharedAcctID {
		t.Fatalf("accountId = %q, want shared account %q", res.Item.AccountID, sharedAcctID)
	}
}

// TestCreateTransaction_SharedAccount_UsesOwnerCategory_Succeeds reproduces the
// real shared-account flow: the caller (user-role grantee) categorizes a
// transaction on the shared account with the ACCOUNT OWNER's category/payee —
// exactly what the SPA presents there. The reference check must accept entities
// owned by the account owner, not only the caller's own. Regression guard
// against the owner-only requireOwnedEntity check that rejected these with
// transaction.item_not_available.
func TestCreateTransaction_SharedAccount_UsesOwnerCategory_Succeeds(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleUser, true)
	tok := h.token(t)
	req := map[string]any{
		"id": txID1, "type": "expense", "amount": "10", "accountId": sharedAcctID,
		"categoryId": ownerCatID, "payeeId": ownerPayeeID,
		"date": "2024-03-01 10:00:00", "description": "shared",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, req)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[writeResult](t, env.Data)
	if res.Item.AccountID != sharedAcctID {
		t.Fatalf("accountId = %q, want shared account %q", res.Item.AccountID, sharedAcctID)
	}
}

func TestCreateTransaction_SharedAccount_GuestRole_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, sharedCreateReq(txID1, "10"))
	assertValidationDenied(t, status, env, "This account is not available for this operation.")
}

func TestCreateTransaction_SharedAccount_NoGrant_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, 0, false) // account owned by another user, no grant to seed user
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, sharedCreateReq(txID1, "10"))
	assertValidationDenied(t, status, env, "This account is not available for this operation.")
}

// TestGetTransactionList_SharedAccount_GuestCanView is the positive read-access
// counterpart to TestGetTransactionList_ForbiddenAccount: a guest grant confers
// VIEW access to the shared account's transactions (view access = any visible /
// shared account, independent of the write role), so the list returns 200.
func TestGetTransactionList_SharedAccount_GuestCanView(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+sharedAcctID, tok, nil)
	if status != http.StatusOK {
		t.Fatalf("guest get-transaction-list = %d, want 200; body: %s", status, env.raw)
	}
	if !env.Success {
		t.Fatalf("success=false, want true for a guest with view access; body: %s", env.raw)
	}
}

// TestCreateTransaction_SharedAccount_PendingUserRole_Denied: an otherwise
// write-capable role (user) confers NO access while the grant is pending — the
// invite must be accepted first. Regression guard against checking only the
// role and ignoring is_accepted.
func TestCreateTransaction_SharedAccount_PendingUserRole_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccountPending(t, roleUser)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, sharedCreateReq(txID1, "10"))
	assertValidationDenied(t, status, env, "This account is not available for this operation.")
}

// TestGetTransactionList_SharedAccount_PendingGrant_NotVisible: a pending grant
// (any role) does not add the account to the visible set, so requesting it
// explicitly is 403 — same as no grant at all.
func TestGetTransactionList_SharedAccount_PendingGrant_NotVisible(t *testing.T) {
	h := newHarness(t)
	h.shareAccountPending(t, roleUser)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+sharedAcctID, tok, nil)
	if status != http.StatusForbidden {
		t.Fatalf("status=%d want 403 (pending grant confers no view access); body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("success=true, want false for a pending grant; body: %s", env.raw)
	}
}

// TestImport_SharedAccount_UserRole_LandsInSharedAccount: a CSV import whose row
// names an account SHARED with the caller (user-role grant) must import the
// transaction INTO that shared account, not silently fork a duplicate own
// account. Regression guard against an owner-only add-transaction check.
func TestImport_SharedAccount_UserRole_LandsInSharedAccount(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleUser, true) // ownerTwo owns sharedAcctID, named "Shared"; seed user has a user grant
	tok := h.token(t)
	csv := "Account,Date,Amount,Category,Note,Payee\n" +
		"Shared,2024-03-01,-42.50,Food,groceries,Market\n"
	status, env := h.doImport(t, tok, csv, importMapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", status, env.raw)
	}
	if res := mustUnmarshal[importResult](t, env.Data); res.Imported != 1 {
		t.Fatalf("imported=%d want 1; errors=%v", res.Imported, res.Errors)
	}
	// The transaction must be on the shared account, not a duplicated own account.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+sharedAcctID, tok, nil)
	if list := mustUnmarshal[listResult](t, listEnv.Data); len(list.Items) != 1 {
		t.Fatalf("shared account has %d transactions, want 1 (import forked a duplicate own account instead of using the shared one); body=%s", len(list.Items), listEnv.raw)
	}
}

// assertValidationDenied checks the denial envelope: HTTP 400, success false,
// code 400, and the fieldless *ValidationError's own message on the wire (an
// i18n key the frontend localizes — see WriteError, which surfaces a fieldless
// validation message instead of masking it).
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

func TestUpdateTransaction_SharedAccount_UserRole_Succeeds(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleUser, true)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, sharedCreateReq(txID1, "10"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok, map[string]any{
		"id": created.Item.ID, "type": "income", "amount": "20", "accountId": sharedAcctID, "categoryId": catID,
		"date": "2024-03-02 10:00:00", "description": "edited",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	if res := mustUnmarshal[writeResult](t, env.Data); res.Item.Amount != "20" {
		t.Fatalf("amount = %q, want 20", res.Item.Amount)
	}
}

func TestUpdateTransaction_SharedAccount_GuestRole_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	tok := h.token(t)
	// Access is checked on the target account before the transaction is loaded,
	// so a guest is rejected regardless of the (nonexistent) transaction id.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok, map[string]any{
		"id": txID1, "type": "income", "amount": "20", "accountId": sharedAcctID, "categoryId": catID,
		"date": "2024-03-02 10:00:00", "description": "edited",
	})
	assertValidationDenied(t, status, env, "This account is not available for this operation.")
}

func TestDeleteTransaction_SharedAccount_UserRole_Succeeds(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleUser, true)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, sharedCreateReq(txID1, "10"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/delete-transaction", tok, map[string]any{"id": created.Item.ID})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
}

func TestDeleteTransaction_SharedAccount_GuestRole_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	// Seed a transaction on the shared account (authored by its owner) so the
	// delete path reaches the access check after loading it.
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	seededTx := f.Transaction(fixture.Transaction{UserID: ownerTwoID, AccountID: sharedAcctID, CategoryID: catID, Type: 0, Amount: "5.00000000", Description: "owner tx"})
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/delete-transaction", tok, map[string]any{"id": seededTx})
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}
