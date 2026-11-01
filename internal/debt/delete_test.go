package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
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

func TestDeleteDebt_PartialNeedsConfirmation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &payee })
	e.budget.hasData = true

	_, err := e.svc.DeleteDebt(ctx, e.uid(), model.DeleteDebtRequest{Id: res.Id})
	requireCode(t, err, errs.CodeDebtRemovalUnconfirmed)
	if !e.exists(t, res.Id) {
		t.Fatal("the unconfirmed delete removed the debt")
	}

	if _, err := e.svc.DeleteDebt(ctx, e.uid(), model.DeleteDebtRequest{Id: res.Id, ConfirmDataRemoval: true}); err != nil {
		t.Fatal(err)
	}
	if e.exists(t, res.Id) {
		t.Fatal("the confirmed delete kept the debt")
	}
	if len(e.budget.transfers) != 0 {
		t.Fatalf("a partial debt must not hand data back: %+v", e.budget.transfers)
	}
}

func TestDeleteDebt_WithoutBudgetDataNeedsNoConfirmation(t *testing.T) {
	e := newEnv(t)
	payee := e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &payee })
	if _, err := e.svc.DeleteDebt(context.Background(), e.uid(), model.DeleteDebtRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	if e.exists(t, res.Id) {
		t.Fatal("debt not deleted")
	}
}

func TestDeleteDebt_WholeCategoryTransfersBack(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.budget.transfers = nil
	e.budget.hasData = true

	if _, err := e.svc.DeleteDebt(context.Background(), e.uid(), model.DeleteDebtRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	want := transferCall{from: res.Id, fromType: 6, to: e.category, toType: 1, currency: fixture.USD}
	if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.budget.transfers, want)
	}
	if e.exists(t, res.Id) {
		t.Fatal("debt not deleted")
	}
}

func TestDeleteDebt_WidensNeighbourRange(t *testing.T) {
	e := newEnv(t)
	old := e.create(t, nil)
	newer := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-02-01") })
	e.clock.now = ymd(2026, 7, 1)
	tx := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	if got := e.splits(t, newer.Id); len(got) != 1 || got[tx] == "" {
		t.Fatalf("newer splits = %v", got)
	}

	if _, err := e.svc.DeleteDebt(context.Background(), e.uid(), model.DeleteDebtRequest{Id: newer.Id}); err != nil {
		t.Fatal(err)
	}
	if got := e.splits(t, old.Id); len(got) != 1 || got[tx] == "" {
		t.Fatalf("old splits after the delete = %v", got)
	}
}
