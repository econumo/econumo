package debt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/debt"
	debtrepo "github.com/econumo/econumo/internal/debt/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (e *env) debtOf(t *testing.T, id string) *model.Debt {
	t.Helper()
	d, err := e.repo.GetByID(context.Background(), vo.MustParseId(id))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (e *env) gone(t *testing.T, id string) {
	t.Helper()
	if _, err := e.repo.GetByID(context.Background(), vo.MustParseId(id)); err == nil {
		t.Fatalf("debt %s still exists", id)
	}
}

// release runs ReleaseCategory in a transaction, as a category delete does.
func (e *env) release(t *testing.T, id string) {
	t.Helper()
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.svc.LockForCategories(ctx, vo.MustParseId(id)); err != nil {
			return err
		}
		return e.svc.ReleaseCategory(ctx, vo.MustParseId(id))
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
}

func (e *env) reassign(t *testing.T, oldID, newID string) {
	t.Helper()
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.svc.LockForCategories(ctx, vo.MustParseId(oldID), vo.MustParseId(newID)); err != nil {
			return err
		}
		return e.svc.ReassignCategory(ctx, vo.MustParseId(oldID), vo.MustParseId(newID))
	})
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
}

// Deleting a category deletes every debt attached to it, its history
// included; a debt on another element stays. A merge moves the source's
// debts to a target that carries none, can carry one and is in the same
// currency, and deletes them otherwise.
func TestClassification_DeleteAndMerge(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		e := newEnv(t)
		paid := e.create(t, nil)
		e.clock.now = ymd(2026, 4, 1)
		e.markPaidOff(t, paid.Id, "2026-03-20")
		active := e.create(t, func(r *createSpec) { r.StartDate = ptr("2026-03-25") })
		other := e.create(t, func(r *createSpec) { r.ElementId = e.newCategory(e.user, true) })

		e.release(t, e.category)
		e.gone(t, paid.Id)
		e.gone(t, active.Id)
		if d := e.debtOf(t, other.Id); d.ElementID.String() == e.category {
			t.Fatalf("the other debt moved: %+v", d)
		}
	})
	t.Run("merge into a target without debts moves them", func(t *testing.T) {
		e := newEnv(t)
		target := e.newCategory(e.user, true)
		res := e.create(t, nil)

		e.reassign(t, e.category, target)
		if d := e.debtOf(t, res.Id); d.ElementID.String() != target || d.ElementType != model.DebtElementCategory {
			t.Fatalf("debt = %+v, want it on the target", d)
		}
	})
	t.Run("merge into a target with any debt deletes them", func(t *testing.T) {
		e := newEnv(t)
		target := e.newCategory(e.user, true)
		res := e.create(t, nil)
		onTarget := e.create(t, func(r *createSpec) { r.ElementId = target })
		e.clock.now = ymd(2026, 4, 1)
		e.markPaidOff(t, onTarget.Id, "2026-03-20")

		e.reassign(t, e.category, target)
		e.gone(t, res.Id)
		if d := e.debtOf(t, onTarget.Id); d.ElementID.String() != target {
			t.Fatalf("the target's debt moved: %+v", d)
		}
	})
	t.Run("merge into a target that cannot carry a debt deletes them", func(t *testing.T) {
		e := newEnv(t)
		target := e.newCategory(e.user, true)
		e.newEnvelope("Housing", target)
		res := e.create(t, nil)

		e.reassign(t, e.category, target)
		e.gone(t, res.Id)
	})
	t.Run("merge into a target in another currency deletes them", func(t *testing.T) {
		e := newEnv(t)
		target := e.newCategory(e.user, true)
		el := e.elements.items[e.budget][target]
		el.CurrencyID = vo.MustParseId(e.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"}))
		e.elements.items[e.budget][target] = el
		res := e.create(t, nil)

		e.reassign(t, e.category, target)
		e.gone(t, res.Id)
	})
}

// The payments of a deleted debt's category stop being split: the debt and
// its splits go together.
func TestReleaseCategory_DeletesTheSplits(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	tx := e.expense(t, e.account, ymd(2026, 3, 10), "100", "")
	e.sync(t)
	if _, ok := e.splits(t, res.Id)[tx]; !ok {
		t.Fatal("setup: the payment has no split")
	}
	e.release(t, e.category)
	if got := e.splits(t, res.Id); len(got) != 0 {
		t.Fatalf("splits = %v, want none", got)
	}
}

// A debt committed after Prepare read the category's debts names a budget
// the delete did not lock: the release fails, retryably, instead of locking
// it after the category row.
func TestReleaseCategory_UnlockedBudgetIsARace(t *testing.T) {
	e := newEnv(t)
	err := e.db.TX.WithTx(context.Background(), func(ctx context.Context) error {
		if err := e.svc.LockForCategories(ctx, vo.MustParseId(e.category)); err != nil {
			return err
		}
		// the debt lands between Prepare and the release
		d := &model.Debt{ID: vo.NewId(), BudgetID: vo.MustParseId(e.budget), ElementID: vo.MustParseId(e.category),
			ElementType: model.DebtElementCategory, Status: model.DebtActive, CurrencyID: vo.MustParseId(fixture.USD),
			Payment: vo.NewDecimal("100"), Schedule: model.RecurringScheduleMonthly, CreatedDay: march10, CreatedAt: march10, UpdatedAt: march10}
		if err := debtrepo.NewRepo(e.db.Engine, e.db.TX).Save(ctx, d); err != nil {
			return err
		}
		return e.svc.ReleaseCategory(ctx, vo.MustParseId(e.category))
	})
	if !errors.Is(err, debt.ErrDebtsChangedConcurrently) {
		t.Fatalf("err = %v, want ErrDebtsChangedConcurrently", err)
	}
}
