package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	mvA   = "bbbb1111-0000-7000-8000-0000000006a1"
	mvB   = "bbbb1111-0000-7000-8000-0000000006a2"
	mvC   = "bbbb1111-0000-7000-8000-0000000006a3"
	mvD   = "bbbb1111-0000-7000-8000-0000000006a4"
	mvT   = "bbbb1111-0000-7000-8000-0000000006a5"
	mvAc2 = "aaaa1111-0000-0000-0000-0000000006b1"
)

// seedDay puts a (key 3), b (2) and c (1) on accountID on 2024-03-01, and d
// alone on 2024-03-05.
func seedDay(t *testing.T, h *harness) *fixture.Builder {
	t.Helper()
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvA, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2024-03-01 09:00:00", SortKey: 3})
	f.Transaction(fixture.Transaction{ID: mvB, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "2", SpentAt: "2024-03-01 12:00:00", SortKey: 2})
	f.Transaction(fixture.Transaction{ID: mvC, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "3", SpentAt: "2024-03-01 18:30:00", SortKey: 1})
	f.Transaction(fixture.Transaction{ID: mvD, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "4", SpentAt: "2024-03-05 07:00:00", SortKey: 1})
	return f
}

func (h *harness) move(t *testing.T, id, account, date string, ids ...string) (int, envelope) {
	t.Helper()
	return h.do(t, http.MethodPost, "/api/v1/transaction/move-transaction", h.token(t), map[string]any{
		"id": id, "accountId": account, "date": date, "transactionIds": ids,
	})
}

func (h *harness) keys(t *testing.T, id string) (sortKey int64, recipientKey *int64, spentAt string) {
	t.Helper()
	var at time.Time
	if err := h.db.QueryRow(`SELECT sort_key, recipient_sort_key, spent_at FROM transactions WHERE id = ?`, id).Scan(&sortKey, &recipientKey, &at); err != nil {
		t.Fatal(err)
	}
	return sortKey, recipientKey, at.Format(datetime.Layout)
}

func TestMoveReordersWithinDay(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	before := h.revision(t, accountID)
	st, e := h.move(t, mvC, accountID, "2024-03-01", mvC, mvA, mvB)
	if st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	for id, want := range map[string]int64{mvC: 3, mvA: 2, mvB: 1} {
		if k, _, _ := h.keys(t, id); k != want {
			t.Errorf("%s: key %d, want %d", id, k, want)
		}
	}
	if _, _, at := h.keys(t, mvC); at != "2024-03-01 18:30:00" {
		t.Errorf("moved row's time changed: %s", at)
	}
	if got := h.revision(t, accountID); got != before+1 {
		t.Errorf("revision %d -> %d, want one bump", before, got)
	}
	var res struct {
		Items []struct {
			Id      string `json:"id"`
			SortKey int64  `json:"sortKey"`
		} `json:"items"`
		Accounts []json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(e.Data, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 || len(res.Accounts) == 0 {
		t.Errorf("response items=%d accounts=%d, want the 3 rewritten rows and the account list", len(res.Items), len(res.Accounts))
	}
}

func TestMoveToAnotherDayKeepsTime(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	st, e := h.move(t, mvA, accountID, "2024-03-05", mvD, mvA)
	if st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	k, _, at := h.keys(t, mvA)
	if at != "2024-03-05 09:00:00" || k != 1 {
		t.Errorf("moved row: %s key %d, want 2024-03-05 09:00:00 key 1", at, k)
	}
	if k, _, _ := h.keys(t, mvD); k != 2 {
		t.Errorf("d: key %d, want 2", k)
	}
}

func TestMoveTransferRecipientSideOnly(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, mvAc2, "Savings", 1)
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvT, UserID: seedUserID, AccountID: mvAc2, AccountRecipientID: accountID, Type: 2, Amount: "5", AmountRecipient: "5", SpentAt: "2024-03-01 08:00:00", SortKey: 5, RecipientSortKey: 1})
	f.Transaction(fixture.Transaction{ID: mvA, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2024-03-01 09:00:00", SortKey: 2})
	st, e := h.move(t, mvT, accountID, "2024-03-01", mvT, mvA)
	if st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	k, rk, _ := h.keys(t, mvT)
	if k != 5 || rk == nil || *rk != 2 {
		t.Errorf("transfer keys %d/%v, want source untouched at 5 and recipient 2", k, rk)
	}
}

func TestMoveRejectsStaleDay(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	st, e := h.move(t, mvC, accountID, "2024-03-01", mvC, mvA)
	if st != http.StatusBadRequest || e.Message != "This day changed meanwhile. Refresh and try again." {
		t.Fatalf("stale move = %d %q; body=%s", st, e.Message, e.raw)
	}
	if k, _, _ := h.keys(t, mvC); k != 1 {
		t.Errorf("stale move wrote: c key %d", k)
	}
	if st, e := h.move(t, mvC, accountID, "2024-03-01", mvC, mvA, mvB, mvB); st != http.StatusBadRequest {
		t.Errorf("duplicate ids = %d; body=%s", st, e.raw)
	}
}

func TestMoveRequiresWriteAccess(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvA, UserID: ownerTwoID, AccountID: sharedAcctID, Type: 0, Amount: "1", SpentAt: "2024-03-01 09:00:00"})
	if st, e := h.move(t, mvA, sharedAcctID, "2024-03-01", mvA); st != http.StatusBadRequest {
		t.Fatalf("guest move = %d; body=%s", st, e.raw)
	}
}

func TestMoveCrossDayNeedsWriteAccessToBothSides(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvT, UserID: ownerTwoID, AccountID: sharedAcctID, AccountRecipientID: accountID, Type: 2, Amount: "5", AmountRecipient: "5", SpentAt: "2024-03-01 08:00:00"})
	if st, e := h.move(t, mvT, accountID, "2024-03-01", mvT); st != http.StatusOK {
		t.Fatalf("reorder on own side = %d; body=%s", st, e.raw)
	}
	st, e := h.move(t, mvT, accountID, "2024-03-02", mvT)
	if st != http.StatusBadRequest || e.Message != "This transaction can't be moved to another day." {
		t.Fatalf("cross-day move = %d %q; body=%s", st, e.Message, e.raw)
	}
	if _, _, at := h.keys(t, mvT); at != "2024-03-01 08:00:00" {
		t.Errorf("refused move changed the date: %s", at)
	}
}

func TestMoveRejectsRowNotOnAccount(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, mvAc2, "Savings", 1)
	seedDay(t, h)
	if st, e := h.move(t, mvA, mvAc2, "2024-03-01", mvA); st != http.StatusBadRequest {
		t.Fatalf("move on a foreign account = %d; body=%s", st, e.raw)
	}
}
