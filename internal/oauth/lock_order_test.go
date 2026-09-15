package oauth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
)

// record arms the lock recorder on the two seams a credential-granting flow
// touches and returns the log, cleared of whatever the setup already did.
func (h *harness) record() *lockLog {
	log := &lockLog{}
	h.users.lock = log
	h.ids.lock = log
	return log
}

// assertMintUnderLock checks the flow took the account's row lock before it
// wrote an identity, and that the write ran inside a unit of work — the lock is
// only held to commit, so an insert outside the transaction is unfenced.
func assertMintUnderLock(t *testing.T, calls []string) {
	t.Helper()
	locked := false
	mints := 0
	for _, c := range calls {
		if c == "LockRow" {
			locked = true
			continue
		}
		if !strings.HasPrefix(c, "InsertIfCurrent") {
			continue
		}
		mints++
		if !locked {
			t.Fatalf("identity insert before the user row lock: calls = %v", calls)
		}
		if !strings.HasSuffix(c, "(tx)") {
			t.Fatalf("identity insert outside a transaction: calls = %v", calls)
		}
	}
	if mints != 1 {
		t.Fatalf("want exactly one identity insert, got %d: calls = %v", mints, calls)
	}
}

// Every identity INSERT is a credential mint: it hands the account another way
// to sign in. The fence alone cannot carry it — under PostgreSQL's READ
// COMMITTED an insert running inside an open reclaim's transaction reads the
// pre-bump generation, passes, and lands behind the reclaim's identity sweep —
// so each of the three insert sites must hold the user's row lock, in the same
// transaction as the insert.
func TestEveryIdentityInsertRunsUnderTheUserRowLock(t *testing.T) {
	t.Run("complete-link", func(t *testing.T) {
		h := newHarness(t, false, true)
		u := h.users.seed(t, "me@example.test", model.AlgorithmArgon2id)
		r := h.startLink(u.ID, "google", "web")
		log := h.record()
		if _, err := h.completeLink(u.ID, r); err != nil {
			t.Fatalf("CompleteLink: %v", err)
		}
		assertMintUnderLock(t, log.calls)
	})

	t.Run("auto-link", func(t *testing.T) {
		h := newHarness(t, false, true)
		u := h.users.seed(t, "external@example.test", model.AlgorithmNone)
		h.fake.Email, h.fake.EmailVerified = "external@example.test", true
		log := h.record()
		if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
			t.Fatalf("auto-link did not sign the user in: %s", r)
		}
		if _, err := h.ids.GetByUserProvider(context.Background(), u.ID, "oidc"); err != nil {
			t.Fatalf("identity not linked: %v", err)
		}
		assertMintUnderLock(t, log.calls)
	})

	t.Run("provision", func(t *testing.T) {
		h := newHarness(t, false, true)
		log := h.record()
		if r := h.login("oidc", "web"); handoffOf(t, r) == "" {
			t.Fatalf("provisioning did not sign the user in: %s", r)
		}
		assertMintUnderLock(t, log.calls)
	})
}
