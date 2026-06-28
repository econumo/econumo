package payee_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// usdID is the baseline USD currency (seeded by migration 20210812210548).
const usdID = fixture.USD

func (h *harness) seedAccount(t *testing.T, id, ownerID, name string) {
	t.Helper()
	fixture.New(t, h.tdb).Account(fixture.Account{
		ID:         id,
		UserID:     ownerID,
		CurrencyID: usdID,
		Name:       name,
	})
}

func (h *harness) seedGrant(t *testing.T, accountID, userID string, role int) {
	t.Helper()
	fixture.New(t, h.tdb).AccountAccess(accountID, userID, role)
}

// TestGetPayeeList_IncludesSharedOwners: own + payees of users who shared an
// account with this user (PHP PayeeRepository::findAvailableForUserId).
func TestGetPayeeList_IncludesSharedOwners(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedPayee(t, payeeID1, seedUserID, "Mine", 0, false)
	h.seedPayee(t, payeeID2, otherUserID, "Shared", 0, false)
	h.seedAccount(t, "33333333-3333-3333-3333-333333333333", otherUserID, "Other's account")
	h.seedGrant(t, "33333333-3333-3333-3333-333333333333", seedUserID, 1)

	status, env := h.do(t, http.MethodGet, "/api/v1/payee/get-payee-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	got := map[string]bool{}
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		got[it.ID] = true
	}
	if !got[payeeID1] {
		t.Errorf("own payee %s missing", payeeID1)
	}
	if !got[payeeID2] {
		t.Errorf("shared-owner payee %s missing (own+shared not applied)", payeeID2)
	}
}

// TestGetPayeeList_ExcludesUnsharedOwners: without a grant, another user's
// payees are hidden.
func TestGetPayeeList_ExcludesUnsharedOwners(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedPayee(t, payeeID1, seedUserID, "Mine", 0, false)
	h.seedPayee(t, payeeID2, otherUserID, "NotShared", 0, false)

	status, env := h.do(t, http.MethodGet, "/api/v1/payee/get-payee-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	got := map[string]bool{}
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		got[it.ID] = true
	}
	if got[payeeID2] {
		t.Errorf("payee %s of a non-sharing user must NOT appear", payeeID2)
	}
}
