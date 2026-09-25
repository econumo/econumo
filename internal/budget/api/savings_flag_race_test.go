package api_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/test/fixture"
)

// interleavingTx runs a one-shot hook just before the next transaction opens,
// which is exactly the gap between a use case's pre-transaction reads and its
// write.
type interleavingTx struct {
	inner port.TxRunner
	hook  atomic.Pointer[func()]
}

func (x *interleavingTx) WithTx(ctx context.Context, fn func(context.Context) error) error {
	if h := x.hook.Swap(nil); h != nil {
		(*h)()
	}
	return x.inner.WithTx(ctx, fn)
}

// A member flagged and planned by another request after the removal loaded the
// budget must still meet the guard: judged from the stale membership it was an
// everyday account, and removing it would have left its savings row and plan
// behind with no member to hang on.
func TestSavingsFlag_GuardSeesMembershipChangedAfterLoad(t *testing.T) {
	for _, c := range []struct {
		label, path string
		body        map[string]any
	}{
		{"remove-account", "/api/v1/budget/remove-account", map[string]any{"id": budgetID1, "accountId": accountID2}},
		{"update-budget", "/api/v1/budget/update-budget", map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "accountIds": []string{accountID}}},
	} {
		t.Run(c.label, func(t *testing.T) {
			var tx *interleavingTx
			h := newHarnessWithTx(t, fixedAugust(), func(inner port.TxRunner) port.TxRunner {
				tx = &interleavingTx{inner: inner}
				return tx
			})
			tok := h.token(t)
			h.f.Account(fixture.Account{ID: accountID2, UserID: seedUserID, CurrencyID: usdID, Name: "Spare"})
			h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
				"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01",
				"accountIds": []string{accountID, accountID2},
			})

			var flagSt, planSt int
			hook := func() {
				flagSt, _ = h.do(t, http.MethodPost, "/api/v1/budget/add-account", tok,
					map[string]any{"id": budgetID1, "accountId": accountID2, "isSavings": true})
				planSt, _ = h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok,
					map[string]any{"budgetId": budgetID1, "elementId": accountID2, "period": "2026-08-01", "amount": "75"})
			}
			tx.hook.Store(&hook)

			st, env := h.do(t, http.MethodPost, c.path, tok, c.body)
			if flagSt != http.StatusOK || planSt != http.StatusOK {
				t.Fatalf("interleaved flag/plan = %d/%d, want 200/200", flagSt, planSt)
			}
			wantFieldError(t, c.label, st, env, "confirmSavingsRemoval")
			sameFlags(t, c.label, memberFlags(t, h, budgetID1), map[string]bool{accountID: false, accountID2: true})
			if el, lim, _ := savingsData(t, h, budgetID1, accountID2); el != 1 || lim != 1 {
				t.Fatalf("savings element/limits = %d/%d after the refusal, want 1/1", el, lim)
			}
		})
	}
}
