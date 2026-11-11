package debt_test

import (
	"context"
	"math"
	"strconv"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// originReq is a loan of 12000 taken on 2025-03-01, 24 monthly payments of
// 600 to 2027-03-01; today is 2026-03-10.
func (e *env) originReq() model.CreateDebtRequest {
	req := e.createReq()
	req.AmountBorrowed, req.StartDate, req.Payment = vo.NewFlexString("12000"), ptr("2025-03-01"), vo.NewFlexString("600")
	return req
}

func (e *env) createOrigin(t *testing.T, mod func(*model.CreateDebtRequest)) *model.DebtResult {
	t.Helper()
	req := e.originReq()
	if mod != nil {
		mod(&req)
	}
	res, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return res
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

func money(x float64) string {
	return vo.NewDecimal(strconv.FormatFloat(round2(x), 'f', 2, 64)).String()
}

func originRate(t *testing.T) float64 {
	t.Helper()
	r, err := model.ImpliedRate(12000, 600, 24, 2)
	if err != nil || r <= 0 {
		t.Fatalf("rate = %v, %v", r, err)
	}
	return r
}

func TestCreateDebt_OriginSnapshotFromAmountBorrowed(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	if res.IsSetUp != 1 || res.AmountBorrowed != "12000" || res.StartDate != "2025-03-01" || res.Balance != "12000" ||
		res.CurrentBalance != "12000" || res.LastPaymentDate != "2027-03-01" || res.Payment != "600" {
		t.Fatalf("result = %+v", res)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want the origin only", len(snaps))
	}
	if sn := snaps[0]; !sn.AsOf.Equal(ymd(2025, 3, 1)) || sn.Balance.String() != "12000" || sn.Payment.String() != "600" ||
		!sn.LastPaymentDate.Equal(ymd(2027, 3, 1)) {
		t.Fatalf("origin = %+v", sn)
	}
	if d := e.debtOf(t, res.Id); d.AmountBorrowed.String() != "12000" || !d.Current.AsOf.Equal(ymd(2025, 3, 1)) {
		t.Fatalf("stored debt = %+v / %+v", d, d.Current)
	}
	if v := e.viewOf(t, res.Id); !v.FirstDay.Equal(ymd(2025, 3, 1)) || !v.StartMonth.Equal(ymd(2025, 3, 1)) {
		t.Fatalf("planning starts %v, want the start date", v.FirstDay)
	}
}

// Without a balance now, what is owed is the amount borrowed less the debt
// part of the payments recorded since the start: a missed due accrues
// nothing, a payment before the start does not count.
func TestSetUpDebt_OwedIsBorrowedLessRecordedPrincipal(t *testing.T) {
	e := newEnv(t)
	e.expense(t, e.account, ymd(2025, 2, 1), "600", "")
	p1 := e.expense(t, e.account, ymd(2025, 4, 1), "600", "")
	p2 := e.expense(t, e.account, ymd(2025, 5, 1), "600", "")
	// June missed
	p3 := e.expense(t, e.account, ymd(2025, 7, 1), "600", "")
	res := e.createOrigin(t, nil)

	r := originRate(t)
	i1 := round2(12000 * r)
	b1 := 12000 - (600 - i1)
	i2 := round2(b1 * r)
	b2 := b1 - (600 - i2)
	i3 := round2(b2 * r)
	b3 := b2 - (600 - i3)
	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.Item.CurrentBalance != money(b3) {
		t.Fatalf("current balance = %s, want %s", got.Item.CurrentBalance, money(b3))
	}
	splits := e.splits(t, res.Id)
	if len(splits) != 3 || splits[p1] != money(i1) || splits[p2] != money(i2) || splits[p3] != money(i3) {
		t.Fatalf("splits = %v, want %s %s %s", splits, money(i1), money(i2), money(i3))
	}
	if got.Payments[0].Balance != money(b3) || got.Payments[2].Balance != money(b1) {
		t.Fatalf("payment balances = %+v", got.Payments)
	}
}

func TestCreateDebt_BalanceNowAppendsASnapshot(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, func(r *model.CreateDebtRequest) { r.Balance = vo.NewFlexString("6000") })
	if res.AmountBorrowed != "12000" || res.Balance != "6000" || res.CurrentBalance != "6000" {
		t.Fatalf("result = %+v", res)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 || !snaps[0].AsOf.Equal(ymd(2025, 3, 1)) || snaps[0].Balance.String() != "12000" ||
		!snaps[1].AsOf.Equal(ymd(2026, 3, 10)) || snaps[1].Balance.String() != "6000" || snaps[1].Payment.String() != "600" ||
		!snaps[1].LastPaymentDate.Equal(ymd(2027, 3, 1)) {
		t.Fatalf("snapshots = %+v %+v", snaps[0], snaps[len(snaps)-1])
	}
}

// A balance now alone overrides from today on: the payments before it keep
// their origin-based splits.
func TestUpdateDebt_BalanceNowAloneOverridesFromToday(t *testing.T) {
	e := newEnv(t)
	p1 := e.expense(t, e.account, ymd(2025, 4, 1), "600", "")
	res := e.createOrigin(t, nil)
	before := e.splits(t, res.Id)[p1]

	req := model.UpdateDebtRequest{Id: res.Id, Name: res.Name, CategoryId: res.CategoryId, StartDate: ptr(res.StartDate),
		Balance: vo.NewFlexString("6500")}
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentBalance != "6500" || got.AmountBorrowed != "12000" || got.Payment != "600" || got.LastPaymentDate != "2027-03-01" {
		t.Fatalf("result = %+v", got)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 || snaps[0].Balance.String() != "12000" || snaps[1].Balance.String() != "6500" {
		t.Fatalf("snapshots = %d", len(snaps))
	}
	if after := e.splits(t, res.Id)[p1]; after != before {
		t.Fatalf("split %s -> %s, want kept", before, after)
	}
	// the same balance now again writes nothing
	if _, err := e.svc.UpdateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 2 {
		t.Fatalf("snapshots = %d, want no new one", n)
	}
}

// Correcting the loan's facts rewrites the origin and splits every payment
// again; no snapshot is added.
func TestUpdateDebt_OriginCorrectionResplits(t *testing.T) {
	e := newEnv(t)
	p1 := e.expense(t, e.account, ymd(2025, 4, 1), "600", "")
	res := e.createOrigin(t, nil)
	before := e.splits(t, res.Id)[p1]

	req := e.updateReq(res)
	req.AmountBorrowed, req.StartDate = vo.NewFlexString("11000"), ptr("2025-03-05")
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 1 || !snaps[0].AsOf.Equal(ymd(2025, 3, 5)) || snaps[0].Balance.String() != "11000" {
		t.Fatalf("origin = %+v (of %d)", snaps[0], len(snaps))
	}
	if got.AmountBorrowed != "11000" || got.StartDate != "2025-03-05" || got.Balance != "11000" {
		t.Fatalf("result = %+v", got)
	}
	r, _ := model.SnapshotRate(model.MathSnapshot{AsOf: ymd(2025, 3, 5), Balance: 11000, Payment: 600,
		Schedule: model.RecurringScheduleMonthly, Last: ymd(2027, 3, 1), Digits: 2, Origin: true})
	// the first period runs from 5 Mar to 1 Apr: 27 of 31 days
	if after := e.splits(t, res.Id)[p1]; after == before || after != money(11000*r*27/31) {
		t.Fatalf("split %s -> %s, want %s", before, after, money(11000*r*27/31))
	}
}

// With a balance now on file, a corrected payment, schedule or last payment
// date reaches it too (it is the contract's), its balance kept.
func TestUpdateDebt_OriginCorrectionReachesALaterBalance(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, func(r *model.CreateDebtRequest) { r.Balance = vo.NewFlexString("6000") })
	req := e.updateReq(res)
	req.Payment, req.LastPaymentDate = vo.NewFlexString("700"), "2027-06-01"
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Payment != "700" || got.Balance != "6000" || got.AmountBorrowed != "12000" || got.LastPaymentDate != "2027-06-01" {
		t.Fatalf("result = %+v", got)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}
	for _, sn := range snaps {
		if sn.Payment.String() != "700" || !sn.LastPaymentDate.Equal(ymd(2027, 6, 1)) {
			t.Fatalf("snapshot = %+v", sn)
		}
	}
	if snaps[0].Balance.String() != "12000" || snaps[1].Balance.String() != "6000" {
		t.Fatalf("balances = %s, %s", snaps[0].Balance, snaps[1].Balance)
	}
}

func TestSetUpDebt_TermValidation(t *testing.T) {
	e := newEnv(t)
	for name, tc := range map[string]struct {
		mod       func(*model.CreateDebtRequest)
		key, code string
	}{
		"last before start": {func(r *model.CreateDebtRequest) { r.LastPaymentDate = "2025-03-01" }, "lastPaymentDate", errs.CodeDebtLastPaymentBeforeStart},
		"start in future":   {func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-03-11") }, "startDate", errs.CodeDebtStartInFuture},
		"not covered":       {func(r *model.CreateDebtRequest) { r.Payment = vo.NewFlexString("400") }, "payment", errs.CodeDebtPaymentsDontCover},
		"balance now after the last payment": {func(r *model.CreateDebtRequest) {
			r.LastPaymentDate, r.Balance = "2026-03-01", vo.NewFlexString("100")
			r.Payment = vo.NewFlexString("1000")
		}, "lastPaymentDate", errs.CodeDebtLastPaymentInPast},
	} {
		req := e.originReq()
		tc.mod(&req)
		_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
		if err == nil {
			t.Fatalf("%s: created", name)
		}
		requireField(t, err, tc.key, tc.code)
	}
	// a loan whose last payment is behind it, still owing what was not
	// recorded, is accepted without a balance now
	if _, err := e.svc.CreateDebt(context.Background(), e.uid(), func() model.CreateDebtRequest {
		r := e.originReq()
		r.StartDate, r.LastPaymentDate, r.Payment = ptr("2024-01-01"), "2026-01-01", vo.NewFlexString("1000")
		return r
	}()); err != nil {
		t.Fatalf("a finished term: %v", err)
	}
}

func TestUpdateDebt_NotSetUpNeedsTheLoanFactsForABalance(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	req := e.updateReq(res)
	req.Balance = vo.NewFlexString("100")
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	for _, k := range []string{"amountBorrowed", "startDate", "lastPaymentDate"} {
		requireField(t, err, k, errs.CodeIsBlank)
	}
}

// Finishing setup takes the payment and schedule stored when left out, and
// the loan's start becomes the planning start.
func TestFinishSetup_FromTheLoanFacts(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, func(r *model.CreateDebtRequest) { r.Payment = vo.NewFlexString("600") })
	req := e.updateReq(res)
	req.Payment, req.Schedule = vo.FlexString{}, ""
	req.AmountBorrowed, req.StartDate, req.LastPaymentDate = vo.NewFlexString("12000"), ptr("2025-03-01"), "2027-03-01"
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSetUp != 1 || got.AmountBorrowed != "12000" || got.Payment != "600" || got.CurrentBalance != "12000" {
		t.Fatalf("result = %+v", got)
	}
	if v := e.viewOf(t, res.Id); !v.FirstDay.Equal(ymd(2025, 3, 1)) {
		t.Fatalf("planning starts %v", v.FirstDay)
	}
}

// A payment on the start date is matched and comes fully off the amount
// borrowed: it accrues nothing and pays no due.
func TestSetUpDebt_StartDayPaymentReducesTheBalance(t *testing.T) {
	e := newEnv(t)
	down := e.expense(t, e.account, ymd(2025, 3, 1), "2000", "")
	res := e.createOrigin(t, nil)
	if got := e.splits(t, res.Id)[down]; got != "0" {
		t.Fatalf("start-day split = %q, want 0", got)
	}
	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.Item.CurrentBalance != "10000" || got.Item.PeriodsPaid != 0 || got.Payments[0].Balance != "10000" {
		t.Fatalf("item = %+v, payments = %+v", got.Item, got.Payments)
	}
}
