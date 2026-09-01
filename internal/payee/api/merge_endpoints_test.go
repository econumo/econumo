package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	mergeSrcPayee = "aaaaaaaa-0000-0000-0000-0000000000b1"
	mergeDstPayee = "aaaaaaaa-0000-0000-0000-0000000000b2"
	mergeAccount  = "aaaaaaaa-0000-0000-0000-0000000000c1"
	mergeUSD      = "dffc2a06-6f29-4704-8575-31709adee926"
)

// seedMergeAccount gives transactions a valid account FK to hang off.
func (h *harness) seedMergeAccount(t *testing.T, ownerID string) {
	t.Helper()
	fixture.New(t, h.tdb).Account(fixture.Account{
		ID: mergeAccount, CurrencyID: mergeUSD, UserID: ownerID, Name: "Wallet", Icon: "wallet",
	})
}

func (h *harness) seedMergeTransaction(t *testing.T, id, ownerID, payeeID string) {
	t.Helper()
	fixture.New(t, h.tdb).Transaction(fixture.Transaction{
		ID: id, UserID: ownerID, AccountID: mergeAccount, PayeeID: payeeID, Amount: "10.00000000",
	})
}

// recurring templates have no fixture builder, so insert directly.
func (h *harness) seedMergeRecurring(t *testing.T, id, ownerID, payeeID string) {
	t.Helper()
	if _, err := h.db.Exec(`INSERT INTO recurring_transactions
		(id, user_id, account_id, payee_id, type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, '10.00000000', '', 'monthly', '2026-09-01 00:00:00', 1, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		id, ownerID, mergeAccount, payeeID); err != nil {
		t.Fatalf("seed recurring %s: %v", id, err)
	}
}

func (h *harness) payeeOf(t *testing.T, table, id string) string {
	t.Helper()
	var got *string
	if err := h.db.QueryRow(`SELECT payee_id FROM `+table+` WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatalf("read %s.%s payee: %v", table, id, err)
	}
	if got == nil {
		return ""
	}
	return *got
}

func (h *harness) payeeExists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM payees WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count payee %s: %v", id, err)
	}
	return n > 0
}

func TestMergePayee_ReassignsTransactionsAndRecurringThenDeletes(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedMergeAccount(t, seedUserID)
	h.seedPayee(t, mergeSrcPayee, seedUserID, "Grocer", 0, false)
	h.seedPayee(t, mergeDstPayee, seedUserID, "Grocery Store", 1, false)
	h.seedMergeTransaction(t, "aaaaaaaa-0000-0000-0000-0000000000d1", seedUserID, mergeSrcPayee)
	h.seedMergeRecurring(t, "aaaaaaaa-0000-0000-0000-0000000000e1", seedUserID, mergeSrcPayee)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/merge-payee", token,
		map[string]any{"sourceId": mergeSrcPayee, "targetId": mergeDstPayee})

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	if got := h.payeeOf(t, "transactions", "aaaaaaaa-0000-0000-0000-0000000000d1"); got != mergeDstPayee {
		t.Errorf("transaction payee = %q, want %q", got, mergeDstPayee)
	}
	// The gap that made delete-category's replace mode lossy: templates left
	// behind get silently nulled by the ON DELETE SET NULL FK.
	if got := h.payeeOf(t, "recurring_transactions", "aaaaaaaa-0000-0000-0000-0000000000e1"); got != mergeDstPayee {
		t.Errorf("recurring payee = %q, want %q", got, mergeDstPayee)
	}
	if h.payeeExists(t, mergeSrcPayee) {
		t.Error("source payee still exists after the merge")
	}
	if !h.payeeExists(t, mergeDstPayee) {
		t.Error("target payee was deleted")
	}
}

// TestMergePayee_ForeignSource_IsNotFoundAndTouchesNothing is the ownership
// guard. The list endpoints return connected users' payees, so a client can see
// — and therefore name — an id it does not own.
func TestMergePayee_ForeignSource_IsNotFoundAndTouchesNothing(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedMergeAccount(t, otherUserID)
	h.seedPayee(t, mergeSrcPayee, otherUserID, "Theirs", 0, false)
	h.seedPayee(t, mergeDstPayee, seedUserID, "Mine", 0, false)
	h.seedMergeTransaction(t, "aaaaaaaa-0000-0000-0000-0000000000d2", otherUserID, mergeSrcPayee)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/merge-payee", token,
		map[string]any{"sourceId": mergeSrcPayee, "targetId": mergeDstPayee})

	// 400 "Payee not found" — the same masking update/delete use, so the
	// response cannot be used to probe which payee ids exist.
	if status != http.StatusBadRequest || env.Message != "Payee not found" {
		t.Fatalf("status = %d, message = %q; want 400 / \"Payee not found\"; body: %s", status, env.Message, env.raw)
	}
	if !h.payeeExists(t, mergeSrcPayee) {
		t.Error("foreign payee was deleted")
	}
	if got := h.payeeOf(t, "transactions", "aaaaaaaa-0000-0000-0000-0000000000d2"); got != mergeSrcPayee {
		t.Errorf("foreign user's transaction was re-pointed to %q", got)
	}
}

func TestMergePayee_ForeignTarget_IsRefusedAndTouchesNothing(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedMergeAccount(t, seedUserID)
	h.seedPayee(t, mergeSrcPayee, seedUserID, "Mine", 0, false)
	h.seedPayee(t, mergeDstPayee, otherUserID, "Theirs", 0, false)
	h.seedMergeTransaction(t, "aaaaaaaa-0000-0000-0000-0000000000d3", seedUserID, mergeSrcPayee)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/merge-payee", token,
		map[string]any{"sourceId": mergeSrcPayee, "targetId": mergeDstPayee})

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if !h.payeeExists(t, mergeSrcPayee) {
		t.Error("source was deleted despite the refusal")
	}
	if got := h.payeeOf(t, "transactions", "aaaaaaaa-0000-0000-0000-0000000000d3"); got != mergeSrcPayee {
		t.Errorf("transaction was re-pointed to %q despite the refusal", got)
	}
}

func TestMergePayee_IntoItself_IsRefused(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedPayee(t, mergeSrcPayee, seedUserID, "Mine", 0, false)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/merge-payee", token,
		map[string]any{"sourceId": mergeSrcPayee, "targetId": mergeSrcPayee})

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if !h.payeeExists(t, mergeSrcPayee) {
		t.Error("payee merged into itself and vanished")
	}
}

func TestMergePayee_BlankIds_AreValidationErrors(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"blank source", map[string]any{"sourceId": "", "targetId": mergeDstPayee}},
		{"blank target", map[string]any{"sourceId": mergeSrcPayee, "targetId": ""}},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, env := h.do(t, http.MethodPost, "/api/v1/payee/merge-payee", token, c.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
			}
		})
	}
}
