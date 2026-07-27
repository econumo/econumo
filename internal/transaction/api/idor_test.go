package api_test

// IDOR regressions for the transaction write endpoints. A valid UUID of another
// user's record must never be sufficient to mutate it:
//   - update-transaction must check write access on the transaction's EXISTING
//     account, not only on the request's target account (else a stranger's
//     transaction can be overwritten and relocated onto an account the caller
//     owns);
//   - a transfer's recipient account must be write-accessible to the caller
//     (else a phantom leg can be injected into a stranger's account balance);
//   - an optional category/payee/tag must belong to the caller (else another
//     user's entity can be attached to a transaction).

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	foreignCatID   = "cccc9999-0000-7000-8000-0000000000f1"
	foreignTagID   = "dddd9999-0000-7000-8000-0000000000f1"
	foreignPayeeID = "eeee9999-0000-7000-8000-0000000000f1"
)

// seedOwnerTwoEntities creates ownerTwo plus a category/tag/payee they own, so
// the caller (seed user) can attempt to reference another user's entities.
func (h *harness) seedOwnerTwoEntities(t *testing.T) {
	t.Helper()
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: ownerTwoID, Email: ownerTwoEmail, Name: "Owner Two", Avatar: "https://avatar.test/o2", Password: "pw", Salt: ownerTwoSalt})
	f.Category(fixture.Category{ID: foreignCatID, UserID: ownerTwoID, Name: "Theirs", Type: 0, Icon: "x"})
	f.Tag(fixture.Tag{ID: foreignTagID, UserID: ownerTwoID, Name: "TheirTag"})
	f.Payee(fixture.Payee{ID: foreignPayeeID, UserID: ownerTwoID, Name: "TheirPayee"})
}

// --- G1: update-transaction cross-user relocation ---

func TestUpdateTransaction_ForeignTransaction_DeniedAndNotRelocated(t *testing.T) {
	h := newHarness(t)
	// ownerTwo owns sharedAcctID; the caller (seed user) has NO grant on it.
	h.shareAccount(t, 0, false)
	// A transaction authored by ownerTwo on their own account.
	txm := backend.NewTxManager(h.db)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	victimCat := f.Category(fixture.Category{ID: foreignCatID, UserID: ownerTwoID, Name: "VFood", Type: 0, Icon: "x"})
	victimTx := f.Transaction(fixture.Transaction{UserID: ownerTwoID, AccountID: sharedAcctID, CategoryID: victimCat, Type: 0, Amount: "9.00000000", Description: "victim"})

	attacker := h.token(t)
	// Move the victim's transaction onto the attacker's own account.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", attacker, map[string]any{
		"id": victimTx, "type": "expense", "amount": "1.00", "accountId": accountID, "categoryId": catID,
		"date": "2024-03-02 10:00:00", "description": "stolen",
	})
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")

	var acct, desc string
	h.db.QueryRow(`SELECT account_id, description FROM transactions WHERE id = ?`, victimTx).Scan(&acct, &desc)
	if acct != sharedAcctID || desc != "victim" {
		t.Fatalf("victim txn account_id=%q desc=%q want %q/\"victim\" (mutated/relocated cross-user)", acct, desc, sharedAcctID)
	}
}

// --- G2: transfer recipient injection ---

func TestCreateTransfer_ForeignRecipient_DeniedAndNoLeg(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, 0, false) // ownerTwo owns sharedAcctID, no grant to the caller
	attacker := h.token(t)
	// Source is the attacker's own account; recipient is the victim's.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", attacker,
		transferReq(txID1, accountID, sharedAcctID, "10", "10"))
	assertValidationDenied(t, status, env, "This account is not available for this operation.")

	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE account_recipient_id = ?`, sharedAcctID).Scan(&n)
	if n != 0 {
		t.Fatalf("phantom transfer leg injected into victim account: %d rows", n)
	}
}

func TestUpdateTransaction_IntoForeignRecipient_Denied(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, 0, false)
	tok := h.token(t)
	// A legitimate own expense, then an attempt to turn it into a transfer whose
	// recipient is the victim's account.
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "5"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok,
		transferReq(created.Item.ID, accountID, sharedAcctID, "5", "5"))
	assertValidationDenied(t, status, env, "This account is not available for this operation.")

	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE account_recipient_id = ?`, sharedAcctID).Scan(&n)
	if n != 0 {
		t.Fatalf("phantom transfer leg injected into victim account: %d rows", n)
	}
}

// --- G3: foreign category/payee/tag ---

func TestCreateTransaction_ForeignCategory_Denied(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerTwoEntities(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, map[string]any{
		"id": txID1, "type": "expense", "amount": "5", "accountId": accountID, "categoryId": foreignCatID,
		"date": "2024-03-01 10:00:00", "description": "x",
	})
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}

func TestCreateTransaction_ForeignTag_Denied(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerTwoEntities(t)
	tok := h.token(t)
	// Own category, but a tag owned by another user.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, map[string]any{
		"id": txID1, "type": "expense", "amount": "5", "accountId": accountID, "categoryId": catID, "tagId": foreignTagID,
		"date": "2024-03-01 10:00:00", "description": "x",
	})
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}

func TestUpdateTransaction_ForeignPayee_Denied(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerTwoEntities(t)
	tok := h.token(t)
	_, cEnv := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok, createReq(txID1, "expense", "5"))
	created := mustUnmarshal[writeResult](t, cEnv.Data)
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", tok, map[string]any{
		"id": created.Item.ID, "type": "expense", "amount": "5", "accountId": accountID, "categoryId": catID, "payeeId": foreignPayeeID,
		"date": "2024-03-02 10:00:00", "description": "x",
	})
	assertValidationDenied(t, status, env, "This transaction is not available for this operation.")
}
