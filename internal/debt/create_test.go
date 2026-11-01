package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestCreateDebt_ValidatesAndSnapshots(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)

	if res.Name != "Car loan" || res.Icon != "i" || res.CurrencyId != fixture.USD || res.CategoryId != e.category {
		t.Errorf("identity = %+v", res)
	}
	if res.Balance != "10000" || res.Payment != "1000" || res.Schedule != "monthly" || res.LastPaymentDate != "2027-03-01" || res.StartDate != "" {
		t.Errorf("terms = %+v", res)
	}
	if res.CurrentBalance != "10000" || res.PaymentsLeft != 12 || res.IsPaidOff != 0 || res.PayeeId != nil || res.TagId != nil {
		t.Errorf("figures = %+v", res)
	}
	if res.PeriodRate == "" || res.PeriodRate == "0.0000000000" || res.NextDueDate != "2026-04-01" || res.PayoffDate != "2027-03-01" {
		t.Errorf("rate/dates = %q %q %q", res.PeriodRate, res.NextDueDate, res.PayoffDate)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if sn := snaps[0]; !sn.AsOf.Equal(ymd(2026, 3, 10)) || sn.Balance.String() != "10000" || sn.Payment.String() != "1000" ||
		sn.Schedule != model.RecurringScheduleMonthly || !sn.LastPaymentDate.Equal(ymd(2027, 3, 1)) {
		t.Errorf("snapshot = %+v", sn)
	}
}

func TestCreateDebt_RefusesLastPaymentInPast(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.LastPaymentDate = "2026-03-10"
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtLastPaymentInPast)
}

func TestCreateDebt_RefusesIncomeCategory(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.CategoryId = e.newCategory(e.user, false)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtCategoryNotExpense)
}

func TestCreateDebt_RefusesForeignCategory(t *testing.T) {
	e := newEnv(t)
	other := e.f.User(fixture.User{Name: "other", Email: "other@example.test"})
	req := e.createReq()
	req.CategoryId = e.newCategory(other, true)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireNotFound(t, err)
}

func TestCreateDebt_RefusesPaymentsDontCover(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.Balance = vo.NewFlexString("20000")
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)
}

func TestCreateDebt_KeyConflict(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.newPayee()
	e.create(t, nil)
	req := e.createReq()
	req.PayeeId = &payee
	_, err := e.svc.CreateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtKeyConflict)

	// A payee-only debt blocks a category-only one, and a tag-only debt
	// overlaps a payee-only one.
	cat2 := e.newCategory(e.user, true)
	payee2, tag2 := e.newPayee(), e.newTag()
	e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = cat2; r.PayeeId = &payee2 })
	plain := e.createReq()
	plain.CategoryId = cat2
	_, err = e.svc.CreateDebt(ctx, e.uid(), plain)
	requireCode(t, err, errs.CodeDebtKeyConflict)
	tagReq := e.createReq()
	tagReq.CategoryId, tagReq.TagId = cat2, &tag2
	_, err = e.svc.CreateDebt(ctx, e.uid(), tagReq)
	requireCode(t, err, errs.CodeDebtKeyConflict)

	// Disjoint payees coexist.
	other := e.newPayee()
	e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = cat2; r.PayeeId = &other })
}

func TestCreateDebt_SameKeyNeedsDistinctStartDates(t *testing.T) {
	e := newEnv(t)
	e.create(t, nil)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), e.createReq())
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-01-01") })
	dup := e.createReq()
	dup.StartDate = ptr("2026-01-01")
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), dup)
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-02-01") })
}

func TestCreateDebt_WholeCategoryTransfersPlans(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	want := transferCall{from: e.category, fromType: 1, to: res.Id, toType: 6, currency: fixture.USD}
	if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.budget.transfers, want)
	}

	payee := e.newPayee()
	e.budget.transfers = nil
	e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = e.newCategory(e.user, true); r.PayeeId = &payee })
	if len(e.budget.transfers) != 0 {
		t.Fatalf("a payee-narrowed debt must not move the category's plans: %+v", e.budget.transfers)
	}
}

func TestCreateDebt_WorksWithoutBudgetPeer(t *testing.T) {
	e := newEnv(t)
	e.svc.SetBudgetElements(nil)
	e.create(t, nil)
}
