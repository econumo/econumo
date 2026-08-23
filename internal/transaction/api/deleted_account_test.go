package api_test

// Spec §5a — self-healing zero: a deleted account is a permanent budget member
// pinned at a zero balance, and that invariant must survive later transaction
// writes. A transfer between a deleted account and a live one is a single row
// that stays editable from the live side, so every delete/edit that moves the
// deleted side's balance re-zeroes it, and no write may put a NEW flow onto a
// deleted account.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

const (
	delAcctID   = "aaaa5555-0000-0000-0000-0000000000a5" // A: deleted before the write under test
	del2AcctID  = "aaaa5555-0000-0000-0000-0000000000a6" // D: a second deleted account
	otherAcctID = "aaaa5555-0000-0000-0000-0000000000a7" // C: a second LIVE account

	// Operation (idempotency) ids for the writes these tests issue.
	opID2 = "bbbb5555-0000-7000-8000-000000000002"
	opID3 = "bbbb5555-0000-7000-8000-000000000003"

	transferDate = "2026-02-10 00:00:00"
	movedDate    = "2026-03-15 00:00:00"

	deletedTxCorrection = "Balance adjustment (deleted transaction)"
	editedTxCorrection  = "Balance adjustment (edited transaction)"
	deletedAccountError = "This account has been deleted"
)

type correction struct {
	typ         int
	amount      string
	spentAt     string
	description string
}

func (h *harness) fixtures(t *testing.T) *fixture.Builder {
	t.Helper()
	txm := backend.NewTxManager(h.db)
	return fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
}

// seedAccount adds another own account of the seed user, in the main folder.
func (h *harness) seedAccount(t *testing.T, id, name string, position int) {
	t.Helper()
	f := h.fixtures(t)
	f.Account(fixture.Account{ID: id, UserID: seedUserID, CurrencyID: usdID, Name: name})
	f.AccountInFolder(folderID, id)
	f.AccountOption(id, seedUserID, position)
}

// deleteAccount runs the real delete-account use case (which auto-zeroes the
// balance first). The account routes are not mounted on this harness, so the
// service is driven directly.
func (h *harness) deleteAccount(t *testing.T, id string) {
	t.Helper()
	if _, err := h.acc.DeleteAccount(context.Background(), mustParseID(t, seedUserID), model.DeleteAccountRequest{Id: id}); err != nil {
		t.Fatalf("delete-account %s: %v", id, err)
	}
}

func mustParseID(t *testing.T, raw string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(raw)
	if err != nil {
		t.Fatalf("parse id %q: %v", raw, err)
	}
	return id
}

// corrections reads the balance-correction rows on an account, oldest first.
// Filtered by description because a seeded plain transaction also has a NULL
// category and would otherwise be counted as a correction.
func corrections(t *testing.T, h *harness, accountID string) []correction {
	t.Helper()
	rows, err := h.db.Query(`SELECT type, amount, spent_at, description FROM transactions
		WHERE account_id = ? AND description LIKE 'Balance adjustment%' ORDER BY created_at, description`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []correction
	for rows.Next() {
		var c correction
		if err := rows.Scan(&c.typ, &c.amount, &c.spentAt, &c.description); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

// allTimeBalance is the account's balance over EVERY transaction (the bound a
// deleted account must reconcile to zero against).
func allTimeBalance(t *testing.T, h *harness, accountID string) float64 {
	t.Helper()
	var v float64
	err := h.db.QueryRow(`SELECT
		  COALESCE((SELECT SUM(amount) FROM transactions WHERE account_id = ? AND type = 1), 0)
		+ COALESCE((SELECT SUM(amount_recipient) FROM transactions WHERE account_recipient_id = ? AND type = 2), 0)
		- COALESCE((SELECT SUM(amount) FROM transactions WHERE account_id = ? AND type = 0), 0)
		- COALESCE((SELECT SUM(amount) FROM transactions WHERE account_id = ? AND type = 2), 0)`,
		accountID, accountID, accountID, accountID).Scan(&v)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// wallClock renders a stored spent_at as the wire layout. Rows written through
// the app bind spent_at as a time.Time, which the sqlite driver serializes as
// RFC3339Nano; fixture-seeded rows hold the bare "Y-m-d H:i:s" string.
func wallClock(t *testing.T, stored string) string {
	t.Helper()
	for _, layout := range []string{time.RFC3339Nano, datetime.Layout} {
		if ts, err := time.Parse(layout, stored); err == nil {
			return ts.UTC().Format(datetime.Layout)
		}
	}
	t.Fatalf("unparseable spent_at %q", stored)
	return ""
}

func transferBody(opID, from, to, amount, date string) map[string]any {
	return map[string]any{
		"id": opID, "type": "transfer", "amount": amount,
		"accountId": from, "accountRecipientId": to, "date": date, "description": "move",
	}
}

func (h *harness) createTransfer(t *testing.T, opID, from, to, amount, date string) string {
	t.Helper()
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", h.token(t),
		transferBody(opID, from, to, amount, date))
	if status != http.StatusOK {
		t.Fatalf("create transfer: status=%d body=%s", status, env.raw)
	}
	return mustUnmarshal[writeResult](t, env.Data).Item.ID
}

// seedTransferToDeleted is the shared arrangement: a live account (the seeded
// "Cash") transfers 50 into account A, then A is deleted — so A's balance is
// pinned at zero by a single delete-time correction, and the transfer row is
// still there, editable from the live side. Returns the transfer's id.
func (h *harness) seedTransferToDeleted(t *testing.T) string {
	t.Helper()
	h.seedAccount(t, delAcctID, "Vault", 1)
	id := h.createTransfer(t, txID1, accountID, delAcctID, "50", transferDate)
	h.deleteAccount(t, delAcctID)
	if c := corrections(t, h, delAcctID); len(c) != 1 || c[0].typ != 0 || c[0].amount != "50" {
		t.Fatalf("delete-time corrections on A = %+v, want one expense of 50", c)
	}
	return id
}

func (h *harness) deleteTransaction(t *testing.T, id string) (int, envelope) {
	t.Helper()
	return h.do(t, http.MethodPost, "/api/v1/transaction/delete-transaction", h.token(t), map[string]any{"id": id})
}

func TestDeleteTransaction_ReZeroesDeletedSide(t *testing.T) {
	h := newHarness(t)
	txn := h.seedTransferToDeleted(t)

	status, env := h.deleteTransaction(t, txn)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}

	c := corrections(t, h, delAcctID)
	if len(c) != 2 {
		t.Fatalf("corrections on A = %+v, want 2 (delete-time + re-zero)", c)
	}
	got := c[1]
	if got.description != deletedTxCorrection || got.typ != 1 || got.amount != "50" {
		t.Fatalf("re-zero correction = %+v, want income 50 %q", got, deletedTxCorrection)
	}
	if ts := wallClock(t, got.spentAt); ts != transferDate {
		t.Fatalf("correction spent_at = %s, want the deleted transaction's %s", ts, transferDate)
	}
	if n := len(corrections(t, h, accountID)); n != 0 {
		t.Fatalf("live side gained %d corrections, want 0", n)
	}
	if b := allTimeBalance(t, h, delAcctID); b != 0 {
		t.Fatalf("A all-time balance = %v, want 0", b)
	}
}

func TestUpdateTransaction_AmountEdit_ReZeroesDeletedSide(t *testing.T) {
	h := newHarness(t)
	txn := h.seedTransferToDeleted(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", h.token(t),
		transferBody(txn, accountID, delAcctID, "30", transferDate))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}

	c := corrections(t, h, delAcctID)
	if len(c) != 2 {
		t.Fatalf("corrections on A = %+v, want 2", c)
	}
	got := c[1]
	if got.description != editedTxCorrection || got.typ != 1 || got.amount != "20" {
		t.Fatalf("re-zero correction = %+v, want income 20 %q", got, editedTxCorrection)
	}
	if ts := wallClock(t, got.spentAt); ts != transferDate {
		t.Fatalf("correction spent_at = %s, want %s", ts, transferDate)
	}
	if b := allTimeBalance(t, h, delAcctID); b != 0 {
		t.Fatalf("A all-time balance = %v, want 0", b)
	}
}

func TestUpdateTransaction_SwapOutDeletedSide(t *testing.T) {
	h := newHarness(t)
	txn := h.seedTransferToDeleted(t)
	h.seedAccount(t, otherAcctID, "Bank", 2)

	// The recipient moves to a live account AND the date moves: the correction
	// must land on the OLD date, where the removed flow used to sit.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", h.token(t),
		transferBody(txn, accountID, otherAcctID, "50", movedDate))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}

	c := corrections(t, h, delAcctID)
	if len(c) != 2 {
		t.Fatalf("corrections on A = %+v, want 2", c)
	}
	got := c[1]
	if got.description != editedTxCorrection || got.typ != 1 || got.amount != "50" {
		t.Fatalf("re-zero correction = %+v, want income 50 %q", got, editedTxCorrection)
	}
	if ts := wallClock(t, got.spentAt); ts != transferDate {
		t.Fatalf("correction spent_at = %s, want the OLD %s", ts, transferDate)
	}
	if b := allTimeBalance(t, h, delAcctID); b != 0 {
		t.Fatalf("A all-time balance = %v, want 0", b)
	}
}

func TestUpdateTransaction_DateOnlyEdit_NoCorrection(t *testing.T) {
	h := newHarness(t)
	txn := h.seedTransferToDeleted(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", h.token(t),
		transferBody(txn, accountID, delAcctID, "50", movedDate))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if c := corrections(t, h, delAcctID); len(c) != 1 {
		t.Fatalf("corrections on A = %+v, want just the delete-time one (the total balance did not move)", c)
	}
}

func TestUpdateTransaction_CannotIntroduceDeletedAccount(t *testing.T) {
	h := newHarness(t)
	h.seedTransferToDeleted(t)
	h.seedAccount(t, otherAcctID, "Bank", 2)

	// (a) a plain expense on the live account, repointed at the deleted one.
	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", h.token(t),
		createReq(opID2, "expense", "9.00"))
	if status != http.StatusOK {
		t.Fatalf("seed expense: status=%d body=%s", status, env.raw)
	}
	expense := mustUnmarshal[writeResult](t, env.Data).Item.ID

	status, env = h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", h.token(t),
		map[string]any{"id": expense, "type": "expense", "amount": "9.00",
			"accountId": delAcctID, "categoryId": catID, "date": transferDate})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body=%s", status, env.raw)
	}
	if msgs := env.errorsMap()["accountId"]; len(msgs) != 1 || msgs[0] != deletedAccountError {
		t.Fatalf("errors.accountId = %v, want [%q]", msgs, deletedAccountError)
	}
	var storedAccount string
	if err := h.db.QueryRow(`SELECT account_id FROM transactions WHERE id = ?`, expense).Scan(&storedAccount); err != nil {
		t.Fatal(err)
	}
	if storedAccount != accountID {
		t.Fatalf("expense moved to %s despite the 400", storedAccount)
	}

	// (b) a live-to-live transfer, repointed at the deleted account.
	transfer := h.createTransfer(t, opID3, accountID, otherAcctID, "5", transferDate)
	status, env = h.do(t, http.MethodPost, "/api/v1/transaction/update-transaction", h.token(t),
		transferBody(transfer, accountID, delAcctID, "5", transferDate))
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body=%s", status, env.raw)
	}
	if msgs := env.errorsMap()["accountRecipientId"]; len(msgs) != 1 || msgs[0] != deletedAccountError {
		t.Fatalf("errors.accountRecipientId = %v, want [%q]", msgs, deletedAccountError)
	}
	if c := corrections(t, h, delAcctID); len(c) != 1 {
		t.Fatalf("a rejected edit wrote corrections: %+v", c)
	}
}

func TestCreateTransaction_RejectsDeletedAccount(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, delAcctID, "Vault", 1)
	h.deleteAccount(t, delAcctID)
	tok := h.token(t)

	status, env := h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		map[string]any{"id": txID1, "type": "expense", "amount": "10.00",
			"accountId": delAcctID, "categoryId": catID, "date": transferDate})
	if status != http.StatusBadRequest {
		t.Fatalf("expense on deleted: status=%d body=%s", status, env.raw)
	}
	if msgs := env.errorsMap()["accountId"]; len(msgs) != 1 || msgs[0] != deletedAccountError {
		t.Fatalf("errors.accountId = %v, want [%q]", msgs, deletedAccountError)
	}

	status, env = h.do(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
		transferBody(opID2, accountID, delAcctID, "10", transferDate))
	if status != http.StatusBadRequest {
		t.Fatalf("transfer into deleted: status=%d body=%s", status, env.raw)
	}
	if msgs := env.errorsMap()["accountRecipientId"]; len(msgs) != 1 || msgs[0] != deletedAccountError {
		t.Fatalf("errors.accountRecipientId = %v, want [%q]", msgs, deletedAccountError)
	}

	// The recurring-posting entry point shares createTransaction, so it is
	// guarded too — it has no route of its own, so it is called directly.
	cat := catID
	_, err := h.svc.CreateTransactionFromRecurring(context.Background(), mustParseID(t, seedUserID),
		model.CreateTransactionRequest{
			Id: vo.NewId().String(), Type: "expense", Amount: vo.NewFlexString("10.00"),
			AccountId: delAcctID, CategoryId: &cat, Date: transferDate,
		}, vo.NewId())
	v, ok := errs.AsValidation(err)
	if !ok || len(v.Fields) != 1 || v.Fields[0].Key != "accountId" ||
		v.Fields[0].Message != deletedAccountError || v.Fields[0].Code != errs.CodeTransactionAccountDeleted {
		t.Fatalf("recurring post error = %#v, want the coded accountId validation error", err)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE account_id = ? OR account_recipient_id = ?`, delAcctID, delAcctID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d transactions written to the deleted account, want 0", n)
	}
}

func TestDeleteTransaction_DeletingCorrectionRestoresIt(t *testing.T) {
	h := newHarness(t)
	h.seedTransferToDeleted(t)

	var correctionID string
	if err := h.db.QueryRow(`SELECT id FROM transactions WHERE account_id = ? AND description LIKE 'Balance adjustment%'`, delAcctID).Scan(&correctionID); err != nil {
		t.Fatal(err)
	}
	status, env := h.deleteTransaction(t, correctionID)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}

	c := corrections(t, h, delAcctID)
	if len(c) != 1 || c[0].description != deletedTxCorrection || c[0].typ != 0 || c[0].amount != "50" {
		t.Fatalf("corrections after deleting the correction = %+v, want a fresh expense of 50", c)
	}
	if b := allTimeBalance(t, h, delAcctID); b != 0 {
		t.Fatalf("A all-time balance = %v, want 0", b)
	}
}

func TestDeleteTransaction_BothSidesDeleted(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, delAcctID, "Vault", 1)
	h.seedAccount(t, del2AcctID, "Safe", 2)
	txn := h.createTransfer(t, txID1, delAcctID, del2AcctID, "50", transferDate)
	h.deleteAccount(t, delAcctID)
	h.deleteAccount(t, del2AcctID)

	status, env := h.deleteTransaction(t, txn)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	for _, id := range []string{delAcctID, del2AcctID} {
		c := corrections(t, h, id)
		if len(c) != 2 || c[1].description != deletedTxCorrection || c[1].amount != "50" {
			t.Fatalf("corrections on %s = %+v, want a re-zero of 50", id, c)
		}
		if b := allTimeBalance(t, h, id); b != 0 {
			t.Fatalf("%s all-time balance = %v, want 0", id, b)
		}
	}
}

// The import path bypasses createTransaction, so §5a leaves it unguarded and
// relies on its account resolution returning AVAILABLE accounts only: a row
// naming a deleted account's name creates a new live account instead of
// writing to the deleted one.
func TestImport_CannotTargetDeletedAccount(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, delAcctID, "Vault", 1)
	h.deleteAccount(t, delAcctID)

	csv := "Account,Date,Amount,Category,Note,Payee\nVault,2026-02-10,-42.50,Food,groceries,Market\n"
	status, env := h.doImport(t, h.token(t), csv, importMapping, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if res := mustUnmarshal[importResult](t, env.Data); res.Imported != 1 {
		t.Fatalf("import result = %+v, want 1 imported", res)
	}
	var onDeleted int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE account_id = ? OR account_recipient_id = ?`, delAcctID, delAcctID).Scan(&onDeleted); err != nil {
		t.Fatal(err)
	}
	if onDeleted != 0 {
		t.Fatalf("import wrote %d transactions to the deleted account", onDeleted)
	}
	var live int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE name = 'Vault' AND is_deleted = 0`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 1 {
		t.Fatalf("live accounts named Vault = %d, want 1 (a fresh one)", live)
	}
}

// failingZeroer lets the FIRST party's re-zero through and fails the second,
// standing in for any error raised after a correction was written.
type failingZeroer struct {
	inner  apptransaction.AccountZeroer
	failOn string
}

func (z failingZeroer) ZeroDeleted(ctx context.Context, accountID vo.Id, spentAt time.Time, description string) error {
	if accountID.String() == z.failOn {
		return errZeroFailed
	}
	return z.inner.ZeroDeleted(ctx, accountID, spentAt, description)
}

var errZeroFailed = errors.New("zeroing blew up")

// A correction joins the caller's transaction rather than committing on its
// own: when a later step of the same delete fails, the correction written
// moments earlier must vanish along with the delete itself.
func TestDeleteTransaction_ReZeroRollsBackWithTheWrite(t *testing.T) {
	h := newHarness(t, withZeroer(func(inner apptransaction.AccountZeroer) apptransaction.AccountZeroer {
		return failingZeroer{inner: inner, failOn: del2AcctID}
	}))
	h.seedAccount(t, delAcctID, "Vault", 1)
	h.seedAccount(t, del2AcctID, "Safe", 2)
	txn := h.createTransfer(t, txID1, delAcctID, del2AcctID, "50", transferDate)
	h.deleteAccount(t, delAcctID)
	h.deleteAccount(t, del2AcctID)

	// The source's re-zero runs first and succeeds; the recipient's fails.
	status, env := h.deleteTransaction(t, txn)
	if status != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500; body=%s", status, env.raw)
	}
	if c := corrections(t, h, delAcctID); len(c) != 1 || c[0].description != "Balance adjustment (account deleted)" {
		t.Fatalf("corrections on A = %+v, want only the delete-time one (the re-zero must have rolled back)", c)
	}
	var alive int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE id = ?`, txn).Scan(&alive); err != nil {
		t.Fatal(err)
	}
	if alive != 1 {
		t.Fatal("the transaction was deleted even though the write failed")
	}
}
