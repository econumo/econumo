package account_test

// End-to-end proof that the X-Timezone request header flows through the real
// HTTP stack (Timezone middleware -> reqctx -> account service balance cutoff)
// and changes which transactions count toward an account's balance. The balance
// is "as of end of TODAY in the caller's timezone"; a transaction dated the
// caller's tomorrow is future and must be excluded — even when the server's UTC
// clock has already rolled into that day.

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/fixture"
)

type tzClock struct{ t time.Time }

func (c tzClock) Now() time.Time { return c.t }

func TestGetAccountList_BalanceHonorsRequestTimezone(t *testing.T) {
	// Fixed "now": 02:00 UTC on the 22nd == 22:00 on the 21st in New York. So for
	// a New York caller it is still the 21st; for a UTC caller it is the 22nd.
	clk := tzClock{t: time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC)}
	h := newHarnessWithClock(t, clk)
	tok := h.token(t)

	const acctID = "aaaa1111-0000-7000-8000-0000000000e1"
	h.f.Account(fixture.Account{ID: acctID, UserID: seedUserID, CurrencyID: usdID, Name: "Cash"})
	h.f.AccountInFolder(seedFolderID, acctID)
	h.f.AccountOption(acctID, seedUserID, 0)
	// The 21st: the New York caller's "today" -> always counted.
	h.f.Transaction(fixture.Transaction{
		ID: "d0000000-0000-7000-8000-0000000000e1", UserID: seedUserID, AccountID: acctID,
		Type: 1, Amount: "100.00", SpentAt: "2026-06-21 12:00:00",
	})
	// The 22nd: the New York caller's TOMORROW (future) but the UTC day's today.
	h.f.Transaction(fixture.Transaction{
		ID: "d0000000-0000-7000-8000-0000000000e2", UserID: seedUserID, AccountID: acctID,
		Type: 1, Amount: "999.00", SpentAt: "2026-06-22 12:00:00",
	})

	balanceFor := func(tz string) string {
		t.Helper()
		var headers map[string]string
		if tz != "" {
			headers = map[string]string{"X-Timezone": tz}
		}
		_, env := h.doH(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil, headers)
		for _, it := range mustUnmarshal[accountItemsWrapper](t, env.Data).Items {
			if it.ID == acctID {
				return it.Balance
			}
		}
		t.Fatalf("account %s missing from list: %s", acctID, env.Data)
		return ""
	}

	// New York: caller's today is the 21st, so the 22nd transaction is future and
	// excluded -> balance 100. (Before the fix this returned 1099.)
	if got := balanceFor("America/New_York"); got != "100" {
		t.Errorf("balance with X-Timezone=America/New_York = %q, want 100 (the 22nd is future for this caller)", got)
	}
	// No header -> UTC day is the 22nd, cutoff the 23rd, so the 22nd transaction
	// counts -> balance 1099. Confirms the boundary really is timezone-driven.
	if got := balanceFor(""); got != "1099" {
		t.Errorf("balance without timezone = %q, want 1099 (UTC day includes the 22nd)", got)
	}
}
