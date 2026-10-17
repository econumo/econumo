package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// accounts.transactions_revision must move for every account whose listed
// transactions a label merge or delete changes, a transfer recipient included.
type labelRevisionSeed struct {
	f       *fixture.Builder
	a, b, c string
	src     string
	dst     string
}

func seedLabelRevision(t *testing.T, h *harness) labelRevisionSeed {
	t.Helper()
	f := fixture.New(t, h.tdb)
	s := labelRevisionSeed{f: f}
	s.a = f.Account(fixture.Account{UserID: seedUserID, Name: "A"})
	s.b = f.Account(fixture.Account{UserID: seedUserID, Name: "B"})
	s.c = f.Account(fixture.Account{UserID: seedUserID, Name: "C"})
	s.src = f.Label(fixture.Label{UserID: seedUserID, Name: "Src"})
	s.dst = f.Label(fixture.Label{UserID: seedUserID, Name: "Dst", Position: 1})
	other := f.Label(fixture.Label{UserID: seedUserID, Name: "Other", Position: 2})
	expense := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, Amount: "10"})
	transfer := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, AccountRecipientID: s.b, Type: 2, Amount: "5", AmountRecipient: "5"})
	unrelated := f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.c, Amount: "7"})
	for _, l := range [][2]string{{expense, s.src}, {transfer, s.src}, {unrelated, other}} {
		if _, err := h.db.Exec(`INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)`, l[0], l[1]); err != nil {
			t.Fatalf("seed transactions_labels: %v", err)
		}
	}
	return s
}

func TestRevision_MergeLabel_BumpsEveryAccountWithTheSource(t *testing.T) {
	h := newHarness(t)
	s := seedLabelRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/label/merge-label", h.issueToken(t),
		map[string]any{"sourceId": s.src, "targetId": s.dst})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}

func TestRevision_DeleteLabel_BumpsEveryAccountWithTheDeleted(t *testing.T) {
	h := newHarness(t)
	s := seedLabelRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/label/delete-label", h.issueToken(t), map[string]any{"id": s.src})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}
