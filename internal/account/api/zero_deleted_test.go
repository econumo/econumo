package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	seedAccountID = "aaaa1111-0000-7000-8000-0000000000d0"

	zeroAcctPos = "acc00000-0000-0000-0000-00000000aa01"
	zeroAcctNeg = "acc00000-0000-0000-0000-00000000aa02"
	zeroAcctNil = "acc00000-0000-0000-0000-00000000aa03"
)

type corrRow struct {
	typ         int
	amount      string
	spentAt     string
	description string
}

// seedOwnerAccount seeds an owned, non-deleted account with folder membership
// (the delete-account tests need a real account of seedUserID's to delete).
func (h *harness) seedOwnerAccount(t *testing.T, id string) {
	t.Helper()
	h.f.Account(fixture.Account{ID: id, UserID: seedUserID, CurrencyID: usdID, Name: "Cash"})
	h.f.AccountInFolder(seedFolderID, id)
}

// corrections reads the balance-correction rows written for an account.
// Filtered to description LIKE 'Balance adjustment%' (not just category_id IS
// NULL) because seeded plain transactions also have a NULL category by
// default -- without the description filter they would be miscounted as
// corrections.
func corrections(t *testing.T, h *harness, accountID string) []corrRow {
	t.Helper()
	rows, err := h.db.Query(`SELECT type, amount, spent_at, description FROM transactions WHERE account_id = ? AND category_id IS NULL AND description LIKE 'Balance adjustment%' ORDER BY created_at`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []corrRow
	for rows.Next() {
		var r corrRow
		if err := rows.Scan(&r.typ, &r.amount, &r.spentAt, &r.description); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestZeroDeletedAccounts_WritesOneCorrectionPerNonZeroDeletedAccount(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerAccount(t, seedAccountID)
	// +12.5 (income), -3 (expense), and a zero-balance deleted account.
	h.f.Account(fixture.Account{ID: zeroAcctPos, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 1, Amount: "12.5", SpentAt: "2026-01-10 00:00:00"})
	h.f.Account(fixture.Account{ID: zeroAcctNeg, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctNeg, Type: 0, Amount: "3", SpentAt: "2026-01-10 00:00:00"})
	h.f.Account(fixture.Account{ID: zeroAcctNil, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	// a live account with a balance must be untouched
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: seedAccountID, Type: 1, Amount: "100", SpentAt: "2026-01-10 00:00:00"})

	n, err := h.svc.ZeroDeletedAccounts(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("ZeroDeletedAccounts = %d, %v; want 2, nil", n, err)
	}
	var updatedAt string
	if err := h.db.QueryRow(`SELECT updated_at FROM accounts WHERE id = ?`, zeroAcctPos).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	pos := corrections(t, h, zeroAcctPos)
	if len(pos) != 1 || pos[0].typ != 0 || pos[0].amount != "12.5" || pos[0].description != "Balance adjustment (account deleted)" || pos[0].spentAt != updatedAt {
		t.Fatalf("positive: %+v (updated_at=%s)", pos, updatedAt)
	}
	neg := corrections(t, h, zeroAcctNeg)
	if len(neg) != 1 || neg[0].typ != 1 || neg[0].amount != "3" {
		t.Fatalf("negative: %+v", neg)
	}
	if len(corrections(t, h, zeroAcctNil)) != 0 {
		t.Fatal("zero-balance account got a correction")
	}
	if len(corrections(t, h, seedAccountID)) != 0 {
		t.Fatal("live account got a correction")
	}
	// idempotent
	n, err = h.svc.ZeroDeletedAccounts(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("second run = %d, %v; want 0", n, err)
	}
}

func TestZeroDeletedAccounts_IgnoresSubScaleResidue(t *testing.T) {
	h := newHarness(t)
	h.f.Account(fixture.Account{ID: zeroAcctPos, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	// 0.1 + 0.2 - 0.3 is a float residue in SQLite's SUM; rounded to 8 dp it is 0.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 1, Amount: "0.1", SpentAt: "2026-01-10 00:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 1, Amount: "0.2", SpentAt: "2026-01-10 00:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 0, Amount: "0.3", SpentAt: "2026-01-10 00:00:00"})
	n, err := h.svc.ZeroDeletedAccounts(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("residue produced a correction: n=%d err=%v", n, err)
	}
}

func TestDeleteAccount_AutoZeroesBalanceThenSoftDeletes(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerAccount(t, seedAccountID)
	tok := h.token(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: seedAccountID, Type: 1, Amount: "40", SpentAt: "2026-01-10 00:00:00"})
	status, env := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": seedAccountID})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	var deleted int
	if err := h.db.QueryRow(`SELECT is_deleted FROM accounts WHERE id = ?`, seedAccountID).Scan(&deleted); err != nil || deleted != 1 {
		t.Fatalf("is_deleted=%d err=%v", deleted, err)
	}
	c := corrections(t, h, seedAccountID)
	if len(c) != 1 || c[0].typ != 0 || c[0].amount != "40" || c[0].description != "Balance adjustment (account deleted)" {
		t.Fatalf("corrections=%+v", c)
	}
	// The sqlc-generated correction insert binds spent_at as a raw time.Time
	// (matching create.go's existing correction path), which the sqlite driver
	// serializes as RFC3339Nano on disk regardless of the "Y-m-d H:i:s" wire
	// layout -- that layout is produced separately, by explicit .Format() calls
	// on the API response DTOs. This just checks the stored value is a sane,
	// parseable timestamp rather than garbage.
	if _, err := time.Parse(time.RFC3339Nano, c[0].spentAt); err != nil {
		t.Fatalf("spent_at layout: %v", err)
	}
}

func TestDeleteAccount_ZeroBalanceWritesNoCorrection(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerAccount(t, seedAccountID)
	tok := h.token(t)
	if st, env := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": seedAccountID}); st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	if len(corrections(t, h, seedAccountID)) != 0 {
		t.Fatal("unexpected correction")
	}
}
