package debt_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// noFixedPayments is a loan from family: 3000 from March 1, 2026, to return
// by February 15, 2027, with no payment schedule.
func (e *env) noFixedPayments(t *testing.T) *model.DebtResult {
	t.Helper()
	return e.create(t, func(r *createSpec) {
		r.AmountBorrowed, r.StartDate, r.Payment, r.Schedule, r.LastPaymentDate =
			vo.NewFlexString("3000"), ptr("2026-03-01"), vo.NewFlexString("0"), "none", "2027-02-15"
	})
}

func TestCreateDebt_NoFixedPaymentsAllowsZeroPayment(t *testing.T) {
	e := newEnv(t)
	req := e.createDebtReq()
	req.Payment, req.Schedule = vo.NewFlexString("0"), "none"
	res, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Payment != "0" || res.Schedule != "none" || res.IsSetUp != 0 {
		t.Fatalf("result = %+v", res)
	}

	cat := e.newCategory(e.user, true)
	monthly := e.createDebtReq()
	monthly.ElementId, monthly.Payment = cat, vo.NewFlexString("0")
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), monthly)
	requireField(t, err, "payment", errs.CodeOutOfRange)
}

// Setup asks for the amount borrowed, the start date and the return-by date
// only: there are no dues to repay it, so nothing is refused as not covering
// the balance, and every payment comes off it in full.
func TestUpdateDebt_SetUpNoFixedPayments(t *testing.T) {
	e := newEnv(t)
	res := e.noFixedPayments(t)
	if res.IsSetUp != 1 || res.CurrentBalance != "3000" || res.AmountBorrowed != "3000" || res.LastPaymentDate != "2027-02-15" ||
		res.Schedule != "none" || res.Payment != "0" {
		t.Fatalf("set up = %+v", res)
	}
	if res.PaymentsLeft != 0 || res.PeriodsPaid != 0 || res.ExtraPaid != "0" || res.PayoffDate != "" || res.NextDueDate != "" ||
		res.AnnualRate != "0" || res.ImpliedAnnualRate != "0" || res.OverpaidSoFar != "0" {
		t.Fatalf("figures = %+v", res)
	}

	tx := e.expense(t, e.account, ymd(2026, 3, 5), "700", "")
	e.sync(t)
	if got := e.splits(t, res.Id)[tx]; got != "0" {
		t.Fatalf("interest = %q, want 0", got)
	}
	got := e.details(t, res.Id).Item
	if got.CurrentBalance != "2300" || got.ExtraPaid != "0" || got.PeriodsPaid != 0 {
		t.Fatalf("after a payment = %+v", got)
	}

	// a statement still sets what is owed
	stated, err := e.reconcile(res.Id, "2000", "")
	if err != nil {
		t.Fatal(err)
	}
	if stated.CurrentBalance != "2000" {
		t.Fatalf("after a statement = %+v", stated)
	}

	// a balance no monthly schedule could repay by that date is fine too
	cat := e.newCategory(e.user, true)
	big := e.create(t, func(r *createSpec) {
		r.ElementId, r.AmountBorrowed, r.Payment, r.Schedule, r.LastPaymentDate =
			cat, vo.NewFlexString("300000"), vo.NewFlexString("0"), "none", "2026-04-01"
	})
	if big.CurrentBalance != "300000" {
		t.Fatalf("big = %+v", big)
	}
}

// What a month finishes the loan on time: 3000 over the twelve months from
// March 2026 through February 2027, the current one included.
func TestRead_MonthlyToFinish(t *testing.T) {
	e := newEnv(t)
	res := e.noFixedPayments(t)
	if res.MonthlyToFinish != "250" {
		t.Fatalf("monthlyToFinish = %q, want 250", res.MonthlyToFinish)
	}
	tl, err := e.svc.Timelines(context.Background(), model.DebtTimelineRequest{Scope: e.scope(e.budget), DebtIDs: []string{res.Id},
		Months: []time.Time{ymd(2026, 3, 1)}, Today: ymd(2026, 3, 10), PeriodStartDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := tl[res.Id].MonthlyToFinish; got != "250" {
		t.Fatalf("timeline monthlyToFinish = %q, want 250", got)
	}

	cat := e.newCategory(e.user, true)
	monthly := e.create(t, func(r *createSpec) { r.ElementId = cat })
	if monthly.MonthlyToFinish != "" {
		t.Fatalf("a scheduled debt has monthlyToFinish %q", monthly.MonthlyToFinish)
	}

	// once the return-by month is past there is no monthly figure
	e.clock.now = ymd(2027, 3, 2)
	if got := e.details(t, res.Id).Item.MonthlyToFinish; got != "" {
		t.Fatalf("past the date = %q, want empty", got)
	}
	e.clock.now = march10
	paid := e.markPaidOff(t, res.Id, "")
	if paid.MonthlyToFinish != "" {
		t.Fatalf("paid off = %q, want empty", paid.MonthlyToFinish)
	}
}

func TestAddRateChange_RefusedForNoFixedPayments(t *testing.T) {
	e := newEnv(t)
	res := e.noFixedPayments(t)
	_, err := e.addRate(res.Id, "2026-03-05", "5", "100")
	requireCode(t, err, errs.CodeDebtNoFixedPayments)
}

// Plans are the only repayments: the debt can be planned to the return-by
// date, or later while the plans repay it later; while they never repay it
// there is no end.
func TestViews_NoFixedPaymentsPlanningEnd(t *testing.T) {
	e := newEnv(t)
	res := e.noFixedPayments(t)
	lastDay := func() time.Time {
		t.Helper()
		views, err := e.svc.Views(context.Background(), e.scope(e.budget))
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range views {
			if v.ID == res.Id {
				return v.LastDay
			}
		}
		t.Fatal("no view")
		return time.Time{}
	}
	if got := lastDay(); !got.IsZero() {
		t.Fatalf("no plans: last day = %v, want none", got)
	}
	for _, m := range []string{"2026-04-01", "2026-05-01", "2026-06-01"} {
		e.setPlan(e.budget, e.category, m, 1000)
	}
	if got := lastDay(); !got.Equal(ymd(2027, 2, 15)) {
		t.Fatalf("repaid by June: last day = %v, want the return-by date", got)
	}
	e.setPlan(e.budget, e.category, "2026-06-01", 0)
	e.setPlan(e.budget, e.category, "2027-04-01", 1000)
	if got := lastDay(); !got.Equal(ymd(2027, 4, 30)) {
		t.Fatalf("repaid in April 2027: last day = %v, want 2027-04-30", got)
	}
}
