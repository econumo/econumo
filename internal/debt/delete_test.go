package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (e *env) exists(t *testing.T, id string) bool {
	t.Helper()
	_, err := e.repo.GetByID(context.Background(), vo.MustParseId(id))
	if err == nil {
		return true
	}
	requireNotFound(t, err)
	return false
}

// Deleting removes the debt with its snapshots, splits and rate changes; a
// paid-off debt is deleted the same way.
func TestDeleteDebt_RemovesTheHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, func(r *createSpec) { r.StartDate = ptr("2026-01-01") })
	tx := e.expense(t, e.account, ymd(2026, 1, 20), "1000", "")
	e.sync(t)
	if _, ok := e.splits(t, res.Id)[tx]; !ok {
		t.Fatal("setup: the payment has no split")
	}
	if _, err := e.svc.DeleteDebt(ctx, e.uid(), model.DeleteDebtRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	if e.exists(t, res.Id) || len(e.snapshots(t, res.Id)) != 0 || len(e.splits(t, res.Id)) != 0 {
		t.Fatal("the debt or its history survived")
	}

	paid := e.create(t, func(r *createSpec) { r.StartDate = ptr("2026-01-01") })
	e.markPaidOff(t, paid.Id, "2026-02-01")
	if _, err := e.svc.DeleteDebt(ctx, e.uid(), model.DeleteDebtRequest{Id: paid.Id}); err != nil {
		t.Fatalf("delete a paid-off debt: %v", err)
	}
}

func TestDeleteDebt_GuestAndArchived(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	guest := e.partner(t, "guest", roleGuest)
	_, err := e.svc.DeleteDebt(context.Background(), uidOf(guest), model.DeleteDebtRequest{Id: res.Id})
	requireForbidden(t, err)
	e.fakeBudget(e.budget).archived = true
	_, err = e.svc.DeleteDebt(context.Background(), e.uid(), model.DeleteDebtRequest{Id: res.Id})
	requireArchived(t, err)
	if !e.exists(t, res.Id) {
		t.Fatal("a refused delete removed the debt")
	}
}

// DeleteElementDebts (an envelope delete) removes every debt on the element,
// active and paid off; MoveElementDebts follows the merge rule.
func TestElementDebts_DeleteAndMove(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	child := e.newCategory(e.user, true)
	env := e.newEnvelope("Housing", child)
	onEnv := e.create(t, func(r *createSpec) { withoutTerms(r); r.ElementId = env })
	other := e.create(t, nil)

	if err := e.svc.DeleteElementDebts(ctx, uidOf(e.budget), uidOf(env)); err != nil {
		t.Fatal(err)
	}
	if e.exists(t, onEnv.Id) || !e.exists(t, other.Id) {
		t.Fatal("DeleteElementDebts touched the wrong debts")
	}

	target := e.newCategory(e.user, true)
	if err := e.svc.MoveElementDebts(ctx, uidOf(e.budget), uidOf(e.category), uidOf(target)); err != nil {
		t.Fatal(err)
	}
	if d := e.debtOf(t, other.Id); d.ElementID.String() != target {
		t.Fatalf("debt = %+v, want it moved to the target", d)
	}
}
