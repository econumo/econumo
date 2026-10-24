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

// seedDay puts a (key a3), b (a2) and c (a1) on accountID on 2024-03-01, and d
// alone on 2024-03-05.
func seedDay(t *testing.T, h *harness) *fixture.Builder {
	t.Helper()
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvA, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2024-03-01 09:00:00", SortKey: "a3"})
	f.Transaction(fixture.Transaction{ID: mvB, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "2", SpentAt: "2024-03-01 12:00:00", SortKey: "a2"})
	f.Transaction(fixture.Transaction{ID: mvC, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "3", SpentAt: "2024-03-01 18:30:00", SortKey: "a1"})
	f.Transaction(fixture.Transaction{ID: mvD, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "4", SpentAt: "2024-03-05 07:00:00", SortKey: "a0"})
	return f
}

// move posts a drop: above is the row now directly above the moved one, "" for
// the top of the day. The row is sent as shown on the day it is stored on.
func (h *harness) move(t *testing.T, id, account, date, above string) (int, envelope) {
	t.Helper()
	_, _, at := h.keys(t, id)
	return h.moveFrom(t, id, account, at[:10], date, above)
}

func (h *harness) moveFrom(t *testing.T, id, account, fromDate, date, above string) (int, envelope) {
	t.Helper()
	body := map[string]any{"id": id, "accountId": account, "fromDate": fromDate, "date": date, "aboveId": nil}
	if above != "" {
		body["aboveId"] = above
	}
	return h.do(t, http.MethodPost, "/api/v1/transaction/move-transaction", h.token(t), body)
}

func (h *harness) keys(t *testing.T, id string) (sortKey string, recipientKey *string, spentAt string) {
	t.Helper()
	var at time.Time
	if err := h.db.QueryRow(`SELECT sort_key, recipient_sort_key, spent_at FROM transactions WHERE id = ?`, id).Scan(&sortKey, &recipientKey, &at); err != nil {
		t.Fatal(err)
	}
	return sortKey, recipientKey, at.Format(datetime.Layout)
}

func (h *harness) key(t *testing.T, id string) string {
	t.Helper()
	k, _, _ := h.keys(t, id)
	return k
}

func TestMoveToTopOfDayWritesOneRow(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	before := h.revision(t, accountID)
	st, e := h.move(t, mvC, accountID, "2024-03-01", "")
	if st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	if c := h.key(t, mvC); !(c > "a3") {
		t.Errorf("c key %q, want above a's a3", c)
	}
	if h.key(t, mvA) != "a3" || h.key(t, mvB) != "a2" {
		t.Errorf("neighbours renumbered: a %q b %q", h.key(t, mvA), h.key(t, mvB))
	}
	if _, _, at := h.keys(t, mvC); at != "2024-03-01 18:30:00" {
		t.Errorf("moved row's time changed: %s", at)
	}
	if got := h.revision(t, accountID); got != before+1 {
		t.Errorf("revision %d -> %d, want one bump", before, got)
	}
	var res struct {
		Items    []struct{ Id, SortKey string } `json:"items"`
		Accounts []json.RawMessage              `json:"accounts"`
	}
	if err := json.Unmarshal(e.Data, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Id != mvC || res.Items[0].SortKey != h.key(t, mvC) || len(res.Accounts) == 0 {
		t.Errorf("response %+v, want only the moved row and the account list", res)
	}
}

func TestMoveBetweenTwoRows(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	if st, e := h.move(t, mvC, accountID, "2024-03-01", mvA); st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	if c := h.key(t, mvC); !(c < "a3" && c > "a2") {
		t.Errorf("c key %q, want between b's a2 and a's a3", c)
	}
}

func TestMoveToAnotherDayKeepsTime(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	st, e := h.move(t, mvA, accountID, "2024-03-05", mvD)
	if st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	k, _, at := h.keys(t, mvA)
	if at != "2024-03-05 09:00:00" || !(k < "a0") {
		t.Errorf("moved row: %s key %q, want 2024-03-05 09:00:00 below d's a0", at, k)
	}
	if h.key(t, mvD) != "a0" {
		t.Errorf("d renumbered: %q", h.key(t, mvD))
	}
}

func TestMoveTransferRecipientSideOnly(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, mvAc2, "Savings", 1)
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvT, UserID: seedUserID, AccountID: mvAc2, AccountRecipientID: accountID, Type: 2, Amount: "5", AmountRecipient: "5", SpentAt: "2024-03-01 08:00:00", SortKey: "a5", RecipientSortKey: "a1"})
	f.Transaction(fixture.Transaction{ID: mvA, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2024-03-01 09:00:00", SortKey: "a2"})
	if st, e := h.move(t, mvT, accountID, "2024-03-01", ""); st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	k, rk, _ := h.keys(t, mvT)
	if k != "a5" || rk == nil || !(*rk > "a2") {
		t.Errorf("transfer keys %q/%v, want source untouched at a5 and the recipient key above a2", k, rk)
	}
}

func TestMoveRejectsStaleDay(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	st, e := h.move(t, mvC, accountID, "2024-03-01", mvD)
	if st != http.StatusBadRequest || e.Message != "This day changed meanwhile. Refresh and try again." {
		t.Fatalf("stale move = %d %q; body=%s", st, e.Message, e.raw)
	}
	if h.key(t, mvC) != "a1" {
		t.Errorf("stale move wrote: c key %q", h.key(t, mvC))
	}
	if st, e := h.move(t, mvC, accountID, "2024-03-01", mvC); st != http.StatusBadRequest {
		t.Errorf("above itself = %d; body=%s", st, e.raw)
	}
}

// Two rows can share a key when two inserts raced; a drop next to them re-keys
// just the tied rows so a key strictly between neighbours exists.
func TestMoveNextToTiedRows(t *testing.T) {
	h := newHarness(t)
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvA, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2024-03-01 09:00:00", SortKey: "a1"})
	f.Transaction(fixture.Transaction{ID: mvB, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "2", SpentAt: "2024-03-01 10:00:00", SortKey: "a1"})
	f.Transaction(fixture.Transaction{ID: mvC, UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "3", SpentAt: "2024-03-01 11:00:00", SortKey: "a5"})
	// shown a, b (tie broken by id), c is on top; drop c between a and b
	if st, e := h.move(t, mvC, accountID, "2024-03-01", mvA); st != http.StatusOK {
		t.Fatalf("move = %d; body=%s", st, e.raw)
	}
	a, b, c := h.key(t, mvA), h.key(t, mvB), h.key(t, mvC)
	if !(a > c && c > b) {
		t.Errorf("keys a %q c %q b %q, want a > c > b", a, c, b)
	}
}

func TestMoveRequiresWriteAccess(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvA, UserID: ownerTwoID, AccountID: sharedAcctID, Type: 0, Amount: "1", SpentAt: "2024-03-01 09:00:00"})
	if st, e := h.move(t, mvA, sharedAcctID, "2024-03-01", ""); st != http.StatusBadRequest {
		t.Fatalf("guest move = %d; body=%s", st, e.raw)
	}
}

func TestMoveCrossDayNeedsWriteAccessToBothSides(t *testing.T) {
	h := newHarness(t)
	h.shareAccount(t, roleGuest, true)
	f := h.fixtures(t)
	f.Transaction(fixture.Transaction{ID: mvT, UserID: ownerTwoID, AccountID: sharedAcctID, AccountRecipientID: accountID, Type: 2, Amount: "5", AmountRecipient: "5", SpentAt: "2024-03-01 08:00:00"})
	if st, e := h.move(t, mvT, accountID, "2024-03-01", ""); st != http.StatusOK {
		t.Fatalf("reorder on own side = %d; body=%s", st, e.raw)
	}
	st, e := h.move(t, mvT, accountID, "2024-03-02", "")
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
	if st, e := h.move(t, mvA, mvAc2, "2024-03-01", ""); st != http.StatusBadRequest {
		t.Fatalf("move on a foreign account = %d; body=%s", st, e.raw)
	}
}

// Another device moved the row to another day while this page still shows it
// on the old one: a drag within the old day must not quietly move it back.
func TestMoveRejectsRowMovedElsewhere(t *testing.T) {
	h := newHarness(t)
	seedDay(t, h)
	// shown on 2024-03-02 here, but stored on 2024-03-01 now
	st, e := h.moveFrom(t, mvC, accountID, "2024-03-02", "2024-03-02", "")
	if st != http.StatusBadRequest || e.Message != "This day changed meanwhile. Refresh and try again." {
		t.Fatalf("stale move = %d %q; body=%s", st, e.Message, e.raw)
	}
	if k, _, at := h.keys(t, mvC); at != "2024-03-01 18:30:00" || k != "a1" {
		t.Errorf("stale move wrote: %s key %q", at, k)
	}
	if st, e := h.moveFrom(t, mvC, accountID, "", "2024-03-01", ""); st != http.StatusBadRequest {
		t.Errorf("blank fromDate = %d; body=%s", st, e.raw)
	}
}
