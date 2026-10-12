package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// accounts.transactions_revision must move for every account whose listed
// transactions a tag merge or delete changes, a transfer recipient included.
type tagRevisionSeed struct {
	f       *fixture.Builder
	a, b, c string
	src     string
	dst     string
}

func seedTagRevision(t *testing.T, h *harness) tagRevisionSeed {
	t.Helper()
	f := fixture.New(t, h.tdb)
	s := tagRevisionSeed{f: f}
	s.a = f.Account(fixture.Account{UserID: seedUserID, Name: "A"})
	s.b = f.Account(fixture.Account{UserID: seedUserID, Name: "B"})
	s.c = f.Account(fixture.Account{UserID: seedUserID, Name: "C"})
	s.src = f.Tag(fixture.Tag{UserID: seedUserID, Name: "Src"})
	s.dst = f.Tag(fixture.Tag{UserID: seedUserID, Name: "Dst", Position: 1})
	other := f.Tag(fixture.Tag{UserID: seedUserID, Name: "Other", Position: 2})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, Amount: "10", TagID: s.src})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, AccountRecipientID: s.b, Type: 2, Amount: "5", AmountRecipient: "5", TagID: s.src})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.c, Amount: "7", TagID: other})
	return s
}

func TestRevision_MergeTag_BumpsEveryAccountWithTheSource(t *testing.T) {
	h := newHarness(t)
	s := seedTagRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/merge-tag", h.issueToken(t),
		map[string]any{"sourceId": s.src, "targetId": s.dst})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}

func TestRevision_DeleteTag_BumpsEveryAccountWithTheDeleted(t *testing.T) {
	h := newHarness(t)
	s := seedTagRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/delete-tag", h.issueToken(t), map[string]any{"id": s.src})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}
