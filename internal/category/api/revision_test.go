package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// accounts.transactions_revision must move for every account whose listed
// transactions a category merge or delete changes, a transfer recipient included.
type categoryRevisionSeed struct {
	f       *fixture.Builder
	a, b, c string
	src     string
	dst     string
}

func seedCategoryRevision(t *testing.T, h *harness) categoryRevisionSeed {
	t.Helper()
	f := fixture.New(t, h.tdb)
	s := categoryRevisionSeed{f: f}
	s.a = f.Account(fixture.Account{UserID: seedUserID, Name: "A"})
	s.b = f.Account(fixture.Account{UserID: seedUserID, Name: "B"})
	s.c = f.Account(fixture.Account{UserID: seedUserID, Name: "C"})
	s.src = f.Category(fixture.Category{UserID: seedUserID, Name: "Src", Type: 0})
	s.dst = f.Category(fixture.Category{UserID: seedUserID, Name: "Dst", Type: 0, Position: 1})
	other := f.Category(fixture.Category{UserID: seedUserID, Name: "Other", Type: 0, Position: 2})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, Amount: "10", CategoryID: s.src})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.a, AccountRecipientID: s.b, Type: 2, Amount: "5", AmountRecipient: "5", CategoryID: s.src})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.c, Amount: "7", CategoryID: other})
	return s
}

func TestRevision_MergeCategory_BumpsEveryAccountWithTheSource(t *testing.T) {
	h := newHarness(t)
	s := seedCategoryRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/merge-category", h.issueToken(t),
		map[string]any{"sourceId": s.src, "targetId": s.dst})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}

func TestRevision_DeleteCategory_BumpsEveryAccountWithTheDeleted(t *testing.T) {
	h := newHarness(t)
	s := seedCategoryRevision(t, h)
	before := s.f.Revisions(s.a, s.b, s.c)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/delete-category", h.issueToken(t), map[string]any{"id": s.src, "mode": "delete"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.a, s.b)
}
