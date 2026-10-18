package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// The transaction list carries each row's author name and avatar, so a profile
// change must move every account holding a transaction that user wrote,
// including one on a partner's shared account.
type authorRevisionSeed struct {
	f                     *fixture.Builder
	own, partnerAcct, not string
}

func seedAuthorRevision(t *testing.T, h *harness) authorRevisionSeed {
	t.Helper()
	const partnerID = "22222222-2222-2222-2222-222222222222"
	f := fixture.New(t, h.tdb).WithCrypto("")
	f.User(fixture.User{ID: partnerID, Email: "partner@example.test", Name: "Partner", Password: "pw", Salt: seedSalt})
	s := authorRevisionSeed{f: f}
	s.own = f.Account(fixture.Account{UserID: seedUserID, Name: "Own"})
	s.partnerAcct = f.Account(fixture.Account{UserID: partnerID, Name: "Shared"})
	f.AccountAccess(s.partnerAcct, seedUserID, 1)
	s.not = f.Account(fixture.Account{UserID: partnerID, Name: "Partner only"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.own, Amount: "1"})
	f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: s.partnerAcct, Amount: "2"})
	f.Transaction(fixture.Transaction{UserID: partnerID, AccountID: s.partnerAcct, Amount: "3"})
	f.Transaction(fixture.Transaction{UserID: partnerID, AccountID: s.not, Amount: "4"})
	return s
}

func TestRevision_UpdateName_BumpsEveryAccountTheUserAuthoredIn(t *testing.T) {
	h := newHarness(t)
	s := seedAuthorRevision(t, h)
	before := s.f.Revisions(s.own, s.partnerAcct, s.not)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-name", h.issueToken(t), map[string]string{"name": "Renamed"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.own, s.partnerAcct)
}

func TestRevision_UpdateAvatar_BumpsEveryAccountTheUserAuthoredIn(t *testing.T) {
	h := newHarness(t)
	s := seedAuthorRevision(t, h)
	before := s.f.Revisions(s.own, s.partnerAcct, s.not)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-avatar", h.issueToken(t), map[string]string{"icon": "face", "color": "fuchsia"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before, s.own, s.partnerAcct)
}

func TestRevision_UpdateNameToTheSameName_BumpsNothing(t *testing.T) {
	h := newHarness(t)
	s := seedAuthorRevision(t, h)
	before := s.f.Revisions(s.own, s.partnerAcct, s.not)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/update-name", h.issueToken(t), map[string]string{"name": seedName})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before)
}

func TestRevision_LoginAndOptionChange_BumpNothing(t *testing.T) {
	h := newHarness(t)
	s := seedAuthorRevision(t, h)
	before := s.f.Revisions(s.own, s.partnerAcct, s.not)

	status, env := h.do(t, http.MethodPost, "/api/v1/user/login-user", "", map[string]string{"username": seedEmail, "password": seedPassword})
	if status != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body: %s", status, env.raw)
	}
	status, env = h.do(t, http.MethodPost, "/api/v1/user/update-analytics", h.issueToken(t), map[string]any{"enabled": false})
	if status != http.StatusOK {
		t.Fatalf("update-analytics status = %d, want 200; body: %s", status, env.raw)
	}

	s.f.RequireRevisionsMoved(before)
}
