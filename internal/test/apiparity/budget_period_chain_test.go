package apiparity

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// A month's startBalance is the previous month's endBalance, and a transaction
// dated exactly at the month start counts once — in that month's flows, not
// also in its startBalance. The start balance used spent_at <= start while the
// flows use spent_at >= start, so a midnight transaction on the 1st (every
// date-only CSV import row) was counted twice.
func TestGetBudget_PeriodsChainAcrossMidnightTransaction(t *testing.T) {
	h := NewHarness(t, dbtest.New(t))
	tok := h.Token(t, OwnerID, OwnerEmail)
	const budgetID = "b0000000-0000-0000-0000-0000000000c1"

	if st, body := h.Call(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID, "name": "Chain", "currencyId": USD, "startDate": "2024-04-01", "accountIds": []string{OwnerAccount}}); st != http.StatusOK {
		t.Fatalf("create-budget = %d; body: %s", st, body)
	}
	for _, tx := range []struct{ id, amount, date string }{
		{"d0000000-0000-0000-0000-0000000000c1", "100", "2024-04-10 10:00:00"},
		{"d0000000-0000-0000-0000-0000000000c2", "40", "2024-05-01 00:00:00"},
	} {
		if st, body := h.Call(t, http.MethodPost, "/api/v1/transaction/create-transaction", tok,
			map[string]any{"id": tx.id, "accountId": OwnerAccount, "type": "income", "amount": tx.amount, "date": tx.date}); st != http.StatusOK {
			t.Fatalf("create-transaction %s = %d; body: %s", tx.date, st, body)
		}
	}

	type balance struct {
		CurrencyId   string  `json:"currencyId"`
		StartBalance *string `json:"startBalance"`
		EndBalance   *string `json:"endBalance"`
		Income       *string `json:"income"`
		Expenses     *string `json:"expenses"`
		Exchanges    *string `json:"exchanges"`
		Holdings     *string `json:"holdings"`
	}
	usdBalance := func(date string) balance {
		t.Helper()
		st, body := h.Call(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID+"&date="+date, tok, nil)
		if st != http.StatusOK {
			t.Fatalf("get-budget %s = %d; body: %s", date, st, body)
		}
		var env struct {
			Data struct {
				Item struct {
					Balances []balance `json:"balances"`
				} `json:"item"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode get-budget %s: %v; body: %s", date, err, body)
		}
		for _, b := range env.Data.Item.Balances {
			if b.CurrencyId == USD {
				if b.StartBalance == nil || b.EndBalance == nil || b.Income == nil || b.Expenses == nil || b.Exchanges == nil || b.Holdings == nil {
					t.Fatalf("get-budget %s: USD balance has null fields for an ended month: %s", date, body)
				}
				return b
			}
		}
		t.Fatalf("get-budget %s: no USD balance; body: %s", date, body)
		return balance{}
	}
	dec := func(s *string) vo.DecimalNumber { return vo.NewDecimal(*s) }

	april, may := usdBalance("2024-04-01"), usdBalance("2024-05-01")
	if !dec(april.EndBalance).Equals(dec(may.StartBalance)) {
		t.Errorf("[%s] May startBalance %s != April endBalance %s", h.Engine(), *may.StartBalance, *april.EndBalance)
	}
	// Flows: income and exchanges/holdings add, expenses are reported as a
	// positive amount that subtracts.
	flows := dec(may.StartBalance).Add(dec(may.Income)).Sub(dec(may.Expenses)).Add(dec(may.Exchanges)).Add(dec(may.Holdings))
	if !flows.Equals(dec(may.EndBalance)) {
		t.Errorf("[%s] May start %s + income %s - expenses %s + exchanges %s + holdings %s = %s, want endBalance %s",
			h.Engine(), *may.StartBalance, *may.Income, *may.Expenses, *may.Exchanges, *may.Holdings, flows.String(), *may.EndBalance)
	}
}
