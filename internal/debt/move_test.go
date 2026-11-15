package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (e *env) move(id, elementID string) (*model.DebtResult, error) {
	return e.svc.MoveDebt(context.Background(), e.uid(), model.MoveDebtRequest{Id: id, ElementId: elementID})
}

// A debt moves to an envelope with its history; the envelope's categories'
// expenses become its payments, the old category's alone no longer do, and
// the category is then a plain element.
func TestMoveDebt_ToAnEnvelopeKeepsTheHistory(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.expense(t, e.account, ymd(2025, 4, 1), "600", "")
	if _, err := e.reconcile(res.Id, "11000", "2025-04-02"); err != nil {
		t.Fatal(err)
	}
	other := e.newCategory(e.user, true)
	env := e.newEnvelope("Mortgage", other)
	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: other, Type: 0, Amount: "700", SpentAt: ymd(2025, 5, 1)})

	got, err := e.move(res.Id, env)
	if err != nil {
		t.Fatal(err)
	}
	if got.ElementId != env || got.ElementType != string(model.DebtElementEnvelope) || got.AmountBorrowed != "12000" {
		t.Fatalf("moved = %+v", got)
	}
	d := e.details(t, res.Id)
	if len(d.Snapshots) != 2 {
		t.Fatalf("history = %+v, want the origin and the statement kept", d.Snapshots)
	}
	if len(d.Payments) != 1 || d.Payments[0].Amount != "700" {
		t.Fatalf("payments = %+v, want the envelope's expense only", d.Payments)
	}
}

func TestMoveDebt_ToItsOwnElementChangesNothing(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	got, err := e.move(res.Id, e.category)
	if err != nil || got.ElementId != e.category {
		t.Fatalf("moved = %+v, %v", got, err)
	}
}

func TestMoveDebt_Refusals(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)

	income := e.newCategory(e.user, false)
	_, err := e.move(res.Id, income)
	requireCode(t, err, errs.CodeDebtElementNotEligible)

	busy := e.newCategory(e.user, true)
	if _, err := e.tryCreate(createSpec{BudgetId: e.budget, ElementId: busy, Payment: vo.NewFlexString("100"), Schedule: "monthly"}); err != nil {
		t.Fatal(err)
	}
	_, err = e.move(res.Id, busy)
	requireCode(t, err, errs.CodeDebtElementHasDebt)

	foreign := e.newCategory(e.user, true)
	el := e.elements.items[e.budget][foreign]
	el.CurrencyID = vo.MustParseId(fixture.NewID())
	e.elements.items[e.budget][foreign] = el
	_, err = e.move(res.Id, foreign)
	requireCode(t, err, errs.CodeDebtElementCurrency)

	free := e.newCategory(e.user, true)
	e.markPaidOff(t, res.Id, "2025-06-01")
	_, err = e.move(res.Id, free)
	requireCode(t, err, errs.CodeDebtPaidOff)
}

// A target whose earlier debt ended after this debt's start would put two
// debts on one day of the element.
func TestMoveDebt_StartMustFollowTheTargetsPreviousDebt(t *testing.T) {
	e := newEnv(t)
	target := e.newCategory(e.user, true)
	old := e.createOrigin(t, func(r *createSpec) { r.ElementId, r.StartDate = target, ptr("2024-06-01") })
	e.markPaidOff(t, old.Id, "2025-06-01")
	res := e.createOrigin(t, nil)
	_, err := e.move(res.Id, target)
	requireCode(t, err, errs.CodeDebtStartBeforePreviousEnd)
}
