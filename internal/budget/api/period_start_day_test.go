package api_test

import (
	"net/http"
	"testing"
)

type periodDayOut struct {
	Item struct {
		PeriodStartDay int `json:"periodStartDay"`
	} `json:"item"`
}

type periodDayBudgetOut struct {
	Item struct {
		Meta struct {
			PeriodStartDay int `json:"periodStartDay"`
		} `json:"meta"`
	} `json:"item"`
}

const (
	budgetID2 = "bbbb2222-0000-7000-8000-000000000002"
	budgetID3 = "bbbb2222-0000-7000-8000-000000000003"
)

func countBudgets(t *testing.T, h *harness) int {
	t.Helper()
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateBudgetPeriodStartDay(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	env := h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Default"))
	if got := mustUnmarshal[periodDayBudgetOut](t, env.Data).Item.Meta.PeriodStartDay; got != 1 {
		t.Fatalf("absent periodStartDay -> %d, want 1", got)
	}

	body := createBudgetReq(budgetID2, "Shifted")
	body["periodStartDay"] = 28
	env = h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, body)
	if got := mustUnmarshal[periodDayBudgetOut](t, env.Data).Item.Meta.PeriodStartDay; got != 28 {
		t.Fatalf("periodStartDay 28 -> %d", got)
	}

	for _, bad := range []int{29, -1, 100} {
		before := countBudgets(t, h)
		body = createBudgetReq(budgetID3, "Bad")
		body["periodStartDay"] = bad
		st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, body)
		if st != http.StatusBadRequest || !containsField(env, "periodStartDay", "Choose a day from 1 to 28") {
			t.Fatalf("periodStartDay %d: status=%d body=%s", bad, st, env.raw)
		}
		if after := countBudgets(t, h); after != before {
			t.Fatalf("periodStartDay %d wrote a budget row (%d -> %d)", bad, before, after)
		}
	}
}

func TestUpdateBudgetPeriodStartDay(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	body := createBudgetReq(budgetID1, "Budget")
	body["periodStartDay"] = 28
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, body)

	update := func(extra map[string]any) (int, envelope) {
		b := map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID}
		for k, v := range extra {
			b[k] = v
		}
		return h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, b)
	}

	st, env := update(nil)
	if st != http.StatusOK || mustUnmarshal[periodDayOut](t, env.Data).Item.PeriodStartDay != 28 {
		t.Fatalf("absent field must leave 28: status=%d body=%s", st, env.raw)
	}
	st, env = update(map[string]any{"periodStartDay": 5})
	if st != http.StatusOK || mustUnmarshal[periodDayOut](t, env.Data).Item.PeriodStartDay != 5 {
		t.Fatalf("set 5: status=%d body=%s", st, env.raw)
	}
	for _, bad := range []int{0, 29} {
		st, env = update(map[string]any{"periodStartDay": bad})
		if st != http.StatusBadRequest || !containsField(env, "periodStartDay", "Choose a day from 1 to 28") {
			t.Fatalf("periodStartDay %d: status=%d body=%s", bad, st, env.raw)
		}
	}
	st, env = update(nil)
	if st != http.StatusOK || mustUnmarshal[periodDayOut](t, env.Data).Item.PeriodStartDay != 5 {
		t.Fatalf("a refused update must not change the day: status=%d body=%s", st, env.raw)
	}

	h.mustDo(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": budgetID1})
	st, env = update(map[string]any{"periodStartDay": 9})
	if st != http.StatusForbidden {
		t.Fatalf("archived budget: status=%d body=%s want 403 budget.archived", st, env.raw)
	}
}

// Plans are keyed by first-of-month periods that never move, so changing the
// start day must not orphan or re-key them.
func TestUpdateBudgetPeriodStartDayKeepsPlans(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2026-10-01", "amount": "100",
	})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok,
		map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "periodStartDay": 28})

	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-10-01", tok, nil)
	type view struct {
		Item struct {
			Structure struct {
				Elements []struct {
					Id       string `json:"id"`
					Budgeted string `json:"budgeted"`
				} `json:"elements"`
			} `json:"structure"`
		} `json:"item"`
	}
	var got string
	for _, e := range mustUnmarshal[view](t, env.Data).Item.Structure.Elements {
		if e.Id == catID {
			got = e.Budgeted
		}
	}
	if got != "100" {
		t.Fatalf("budgeted=%q want 100 after the start day changed", got)
	}
}

func TestCloneBudgetCopiesPeriodStartDay(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	body := createBudgetReq(cloneSrcID, "Src")
	body["periodStartDay"] = 28
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, body)

	env := h.mustDo(t, http.MethodPost, "/api/v1/budget/clone-budget", tok,
		map[string]any{"id": cloneSrcID, "newId": cloneDstID, "name": "Copy", "withLimits": false})
	if got := mustUnmarshal[periodDayBudgetOut](t, env.Data).Item.Meta.PeriodStartDay; got != 28 {
		t.Fatalf("copy periodStartDay=%d want 28", got)
	}
}
