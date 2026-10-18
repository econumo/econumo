package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// accounts.transactions_revision must move for every account whose listed
// transactions a payee merge or delete changes, a transfer recipient included.
type payeeRevisionSeed struct {
	f       *fixture.Builder
	a, b, c string
	src     string
	dst     string
}

func seedPayeeRevision(t *testing.T, h *harness) payeeRevisionSeed {
	t.Helper()
	f := fixture.New(t, h.tdb)
	s := payeeRevisionSeed{f: f}
	s.a = f.Account(fixture.Account{UserID: seedUserID, Name: "A"})
	s.b = f.Account(fixture.Account{UserID: seedUserID, Name: "B"})
	s.c = f.Account(fixture.Account{UserID: seedUserID, Name: "C"})
	s.src = f.Payee(fixture.Payee{UserID: seedUserID, Name: "Src"})
	s.dst = f.Payee(fixture.Payee{UserID: seedUserID, Name: "Dst", Position: 1})
	other := f.Payee(fixture.Payee{UserID: seedUserID, Name: "Other", Position: 2})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, Amount: "10", PayeeID: s.src})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, AccountRecipientID: s.b, Type: 2, Amount: "5", AmountRecipient: "5", PayeeID: s.src})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.c, Amount: "7", PayeeID: other})
	return s
}

func TestRevision_MergePayee_BumpsEveryAccountWithTheSource(t *testing.T) {
	h := newHarness(t)
	s := seedPayeeRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/merge-payee", h.issueToken(t),
		map[string]any{"sourceId": s.src, "targetId": s.dst})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}

func TestRevision_DeletePayee_BumpsEveryAccountWithTheDeleted(t *testing.T) {
	h := newHarness(t)
	s := seedPayeeRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/delete-payee", h.issueToken(t), map[string]any{"id": s.src})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}
