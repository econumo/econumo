package debt_test

import (
	"context"
	"slices"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
)

// The budgets of the debts on a set of categories, paid off ones included,
// come back once each and in id order, whatever order the categories are in.
func TestBudgetIDsOfCategories(t *testing.T) {
	e := newEnv(t)
	paid := e.create(t, nil)
	e.clock.now = ymd(2026, 4, 1)
	e.markPaidOff(t, paid.Id, "2026-03-20")

	second := e.newBudget(e.user, "Second", e.account)
	onSecond := e.newCategory(e.user, true)
	e.elements.set(second, e.elements.items[e.budget][onSecond])
	e.create(t, func(r *createSpec) { r.BudgetId, r.ElementId, r.StartDate = second, onSecond, ptr("2026-04-01") })
	e.elements.set(second, e.elements.items[e.budget][e.category])
	e.create(t, func(r *createSpec) { r.BudgetId, r.ElementId, r.StartDate = second, e.category, ptr("2026-04-01") })
	withoutDebts := e.newCategory(e.user, true)

	want := []string{e.budget, second}
	slices.Sort(want)

	got, err := e.svc.BudgetIDsOfCategories(context.Background(),
		vo.MustParseId(onSecond), vo.MustParseId(e.category), vo.MustParseId(withoutDebts))
	if err != nil {
		t.Fatal(err)
	}
	if s := idStrings(got); !slices.Equal(s, want) {
		t.Fatalf("budgets = %v, want %v", s, want)
	}

	none, err := e.svc.BudgetIDsOfCategories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if none == nil || len(none) != 0 {
		t.Fatalf("no categories: budgets = %#v, want an empty slice", none)
	}
}

// The wipe locks each budget BudgetIDsOfCategories names, then releases the
// category in the same transaction: the debts go.
func TestReleaseCategory_AfterExplicitLocks(t *testing.T) {
	e := newEnv(t)
	paid := e.create(t, nil)
	e.clock.now = ymd(2026, 4, 1)
	e.markPaidOff(t, paid.Id, "2026-03-20")
	active := e.create(t, func(r *createSpec) { r.StartDate = ptr("2026-03-25") })
	cat := vo.MustParseId(e.category)

	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		budgets, err := e.svc.BudgetIDsOfCategories(ctx, cat)
		if err != nil {
			return err
		}
		for _, b := range budgets {
			if err := e.svc.LockBudget(ctx, b); err != nil {
				return err
			}
			if !e.svc.HoldsBudgetLock(ctx, b) {
				t.Errorf("HoldsBudgetLock(%s) = false after LockBudget", b)
			}
		}
		return e.svc.ReleaseCategory(ctx, cat)
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	e.gone(t, paid.Id)
	e.gone(t, active.Id)
}

func idStrings(in []vo.Id) []string {
	out := make([]string, 0, len(in))
	for _, id := range in {
		out = append(out, id.String())
	}
	return out
}
