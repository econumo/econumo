package api_test

// Reproduction: creating an account with a non-zero opening balance must show
// that balance immediately, even for a caller behind UTC during the window where
// the server's UTC clock has rolled past midnight but the caller's local day has
// not. The opening-balance correction is dated at "now"; if it is dated in the
// server's UTC wall-clock it lands AFTER the caller's day boundary and is treated
// as a future transaction, so the balance reads 0.

import (
	"net/http"
	"testing"
	"time"
)

func TestCreateAccount_WithBalance_HonorsRequestTimezone(t *testing.T) {
	// 02:00 UTC on the 22nd == 22:00 on the 21st in New York: still the 21st for
	// the caller, so their balance cutoff is the start of the 22nd.
	clk := tzClock{t: time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC)}
	h := newHarnessWithClock(t, clk)
	tok := h.token(t)

	headers := map[string]string{"X-Timezone": "America/New_York"}
	_, env := h.doH(t, http.MethodPost, "/api/v1/account/create-account", tok,
		createAccountReq(acctID1, "Savings", "150.50"), headers)
	res := mustUnmarshal[accountItemWrapper](t, env.Data)
	if res.Item.Balance != "150.5" {
		t.Fatalf("create balance = %q, want 150.5 (opening balance must show for a behind-UTC caller); body: %s", res.Item.Balance, env.raw)
	}

	// And it must still show in the list for the same caller.
	_, listEnv := h.doH(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil, headers)
	list := mustUnmarshal[accountItemsWrapper](t, listEnv.Data)
	if len(list.Items) != 1 || list.Items[0].Balance != "150.5" {
		t.Fatalf("list = %+v, want one account balance 150.5", list.Items)
	}
}
