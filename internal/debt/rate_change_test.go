package debt_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (e *env) addRate(id, date, rate, payment string) (*model.DebtResult, error) {
	return e.svc.AddDebtRateChange(context.Background(), e.uid(), model.AddDebtRateChangeRequest{
		Id: id, Date: date, AnnualRate: vo.NewFlexString(rate), Payment: vo.NewFlexString(payment)})
}

func (e *env) mustAddRate(t *testing.T, id, date, rate, payment string) *model.DebtResult {
	t.Helper()
	res, err := e.addRate(id, date, rate, payment)
	if err != nil {
		t.Fatalf("AddDebtRateChange: %v", err)
	}
	return res
}

func (e *env) deleteRate(id, changeID string) (*model.DebtResult, error) {
	return e.svc.DeleteDebtRateChange(context.Background(), e.uid(), model.DeleteDebtRateChangeRequest{Id: id, RateChangeId: changeID})
}

// impliedAnnual is the yearly rate the origin loan (12000 from 2025-03-01, 600
// monthly to 2027-03-01) implies, as the wire carries it.
func impliedAnnual(t *testing.T) string {
	t.Helper()
	r, err := model.SnapshotRate(model.MathSnapshot{AsOf: ymd(2025, 3, 1), Balance: 12000, Payment: 600,
		Schedule: model.RecurringScheduleMonthly, Last: ymd(2027, 3, 1), Digits: 2, Origin: true})
	if err != nil {
		t.Fatal(err)
	}
	return vo.NewDecimal(strconv.FormatFloat(model.PeriodToAnnualRate(r, model.RecurringScheduleMonthly), 'f', 8, 64)).String()
}

func TestAddRateChange_StoresTheStartRowFirst(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "600")
	got := e.details(t, res.Id).RateChanges
	if len(got) != 2 {
		t.Fatalf("rate changes = %+v", got)
	}
	if got[0].Date != "2025-03-01" || got[0].AnnualRate != impliedAnnual(t) || got[0].Payment != "600" || got[0].Id == "" {
		t.Fatalf("start row = %+v, want the implied %s", got[0], impliedAnnual(t))
	}
	if got[1].Date != "2025-09-01" || got[1].AnnualRate != "5" || got[1].Payment != "600" {
		t.Fatalf("change = %+v", got[1])
	}
}

func TestAddRateChange_SameDateReplaces(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "600")
	e.mustAddRate(t, res.Id, "2025-09-01", "6", "650")
	got := e.details(t, res.Id).RateChanges
	if len(got) != 2 || got[1].Date != "2025-09-01" || got[1].AnnualRate != "6" || got[1].Payment != "650" {
		t.Fatalf("rate changes = %+v", got)
	}
}

func TestAddRateChange_NoDetailsListWithoutChanges(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	if got := e.details(t, res.Id).RateChanges; got == nil || len(got) != 0 {
		t.Fatalf("rate changes = %#v, want []", got)
	}
}

func TestAddRateChange_ResplitsFromTheWindowHoldingTheDate(t *testing.T) {
	e := newEnv(t)
	sep := e.expense(t, e.account, ymd(2025, 9, 1), "600", "")
	oct := e.expense(t, e.account, ymd(2025, 10, 1), "600", "")
	for _, m := range []int{4, 5, 6, 7, 8, 11, 12} {
		e.expense(t, e.account, ymd(2025, time.Month(m), 1), "600", "")
	}
	res := e.createOrigin(t, nil)
	before := e.splits(t, res.Id)
	e.mustAddRate(t, res.Id, "2025-09-16", "8", "600")
	after := e.splits(t, res.Id)
	if after[sep] != before[sep] {
		t.Fatalf("split of 2025-09-01 %s -> %s, want kept", before[sep], after[sep])
	}
	if after[oct] == before[oct] || after[oct] == "" {
		t.Fatalf("split of 2025-10-01 %s -> %s, want recomputed", before[oct], after[oct])
	}
}

func TestAddRateChange_Refusals(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	draft := e.createWithoutTerms(t, func(r *createSpec) { r.ElementId = e.newCategory(e.user, true) })
	_, err := e.addRate(draft.Id, "2025-09-01", "5", "600")
	requireCode(t, err, errs.CodeDebtNotSetUp)

	_, err = e.addRate(res.Id, "2025-02-28", "5", "600")
	requireField(t, err, "date", errs.CodeOutOfRange)
	for _, rate := range []string{"-1", "101"} {
		_, err = e.addRate(res.Id, "2025-09-01", rate, "600")
		requireField(t, err, "annualRate", errs.CodeOutOfRange)
	}
	_, err = e.addRate(res.Id, "2025-09-01", "5", "0")
	requireField(t, err, "payment", errs.CodeOutOfRange)
	_, err = e.addRate(res.Id, "2025-09-01", "50", "1")
	requireField(t, err, "payment", errs.CodeDebtPaymentsDontCover)
	if got := e.details(t, res.Id).RateChanges; len(got) != 0 {
		t.Fatalf("a refused change was stored: %+v", got)
	}

	guest := e.partner(t, "guest", roleGuest)
	_, err = e.svc.AddDebtRateChange(context.Background(), vo.MustParseId(guest), model.AddDebtRateChangeRequest{
		Id: res.Id, Date: "2025-09-01", AnnualRate: vo.NewFlexString("5"), Payment: vo.NewFlexString("600")})
	requireForbidden(t, err)
	e.fakeBudget(e.budget).archived = true
	_, err = e.addRate(res.Id, "2025-09-01", "5", "600")
	requireArchived(t, err)
}

func TestAddRateChange_StartDateBoundAndZeroRate(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-03-01", "0", "600")
	got := e.details(t, res.Id).RateChanges
	if len(got) != 1 || got[0].Date != "2025-03-01" || got[0].AnnualRate != "0" {
		t.Fatalf("rate changes = %+v", got)
	}
}

func TestDeleteRateChange_StartRowFallsBackToImplied(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "600")
	rows := e.details(t, res.Id).RateChanges
	if _, err := e.deleteRate(res.Id, rows[0].Id); err != nil {
		t.Fatal(err)
	}
	left := e.details(t, res.Id).RateChanges
	if len(left) != 1 || left[0].Date != "2025-09-01" {
		t.Fatalf("rate changes = %+v", left)
	}
	e.clock.now = ymd(2025, 8, 15)
	d := e.details(t, res.Id)
	if want := strconv.FormatFloat(originRate(t), 'f', 10, 64); d.Item.PeriodRate != want || d.Item.AnnualRate != impliedAnnual(t) {
		t.Fatalf("periodRate = %s annual %s, want the implied %s / %s", d.Item.PeriodRate, d.Item.AnnualRate, want, impliedAnnual(t))
	}
}

func TestDeleteRateChange_OtherDebtsRowIsNotFound(t *testing.T) {
	e := newEnv(t)
	a := e.createOrigin(t, nil)
	b := e.createOrigin(t, func(r *createSpec) { r.ElementId = e.newCategory(e.user, true) })
	e.mustAddRate(t, a.Id, "2025-09-01", "5", "600")
	rows := e.details(t, a.Id).RateChanges
	_, err := e.deleteRate(b.Id, rows[1].Id)
	requireNotFound(t, err)
	_, err = e.deleteRate(a.Id, b.Id)
	requireNotFound(t, err)
	if n := len(e.details(t, a.Id).RateChanges); n != 2 {
		t.Fatalf("rate changes = %d, want both kept", n)
	}
}

func TestDeleteRateChange_RefusedWhenThePaymentsNoLongerCover(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	// a payment that keeps up with a higher rate, then the rate before it
	e.mustAddRate(t, res.Id, "2025-10-01", "60", "900")
	e.mustAddRate(t, res.Id, "2025-09-01", "60", "600")
	rows := e.details(t, res.Id).RateChanges
	_, err := e.deleteRate(res.Id, rows[2].Id)
	requireField(t, err, "payment", errs.CodeDebtPaymentsDontCover)
}

func TestUpdateDebt_StartAfterARateChangeIsRefused(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "600")
	req := e.updateReq(res)
	req.StartDate = ptr("2025-10-01")
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "startDate", errs.CodeDebtConflictsWithRateChange)
}

// The change on the start date is the start's: it moves with a corrected
// start, its rate and payment kept.
func TestUpdateDebt_StartRowMovesWithTheStart(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "600")
	req := e.updateReq(res)
	req.StartDate = ptr("2025-04-01")
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	rows := e.details(t, res.Id).RateChanges
	if got.StartDate != "2025-04-01" || len(rows) != 2 {
		t.Fatalf("start = %s, rate changes = %+v", got.StartDate, rows)
	}
	if rows[0].Date != "2025-04-01" || rows[0].AnnualRate != impliedAnnual(t) || rows[0].Payment != "600" {
		t.Fatalf("start row = %+v, want it on 2025-04-01 at %s with 600", rows[0], impliedAnnual(t))
	}
	if rows[1].Date != "2025-09-01" || rows[1].AnnualRate != "5" {
		t.Fatalf("change = %+v", rows[1])
	}
}

// A corrected payment is the start row's payment too; a later change keeps
// its own.
func TestUpdateDebt_PaymentCorrectionReachesTheStartRow(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "700")
	req := e.updateReq(res)
	req.Payment = vo.NewFlexString("650")
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	rows := e.details(t, res.Id).RateChanges
	if got.Payment != "650" || len(rows) != 2 || rows[0].Payment != "650" || rows[0].AnnualRate != impliedAnnual(t) || rows[1].Payment != "700" {
		t.Fatalf("payment = %s, rate changes = %+v", got.Payment, rows)
	}
	e.clock.now = ymd(2025, 5, 10)
	f, err := e.svc.GetDebtForecast(context.Background(), e.uid(), model.GetDebtForecastRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	pays := map[string]string{}
	for _, it := range f.Items {
		pays[it.Date] = it.Payment
	}
	if pays["2025-06-01"] != "650" || pays["2025-08-01"] != "650" || pays["2025-09-01"] != "700" {
		t.Fatalf("forecast = %+v, want 650 before September and 700 from it", f.Items)
	}
}

// A correction is checked with the rate changes applied: from the change on,
// its own rate and payment stay.
func TestUpdateDebt_CorrectionCheckedWithTheRateChanges(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "10", "600")
	req := e.updateReq(res)
	req.AmountBorrowed, req.Payment = vo.NewFlexString("100000"), vo.NewFlexString("5000")
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "payment", errs.CodeDebtPaymentsDontCover)
}

func TestRead_AnnualRates(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	if res.AnnualRate == "" || res.ImpliedAnnualRate != res.AnnualRate || res.ImpliedAnnualRate != impliedAnnual(t) {
		t.Fatalf("annual = %q implied %q", res.AnnualRate, res.ImpliedAnnualRate)
	}
	got := e.mustAddRate(t, res.Id, "2025-09-01", "5", "600")
	if got.AnnualRate != "5" || got.ImpliedAnnualRate != impliedAnnual(t) {
		t.Fatalf("annual = %q implied %q", got.AnnualRate, got.ImpliedAnnualRate)
	}
	if want := strconv.FormatFloat(model.AnnualToPeriodRate(5, model.RecurringScheduleMonthly), 'f', 10, 64); got.PeriodRate != want {
		t.Fatalf("periodRate = %s, want %s", got.PeriodRate, want)
	}
	// announced, not in force yet
	got = e.mustAddRate(t, res.Id, "2026-06-01", "4", "600")
	if got.AnnualRate != "5" {
		t.Fatalf("annual = %q, want today's 5", got.AnnualRate)
	}
	draft := e.createWithoutTerms(t, func(r *createSpec) { r.ElementId = e.newCategory(e.user, true) })
	if draft.AnnualRate != "" || draft.ImpliedAnnualRate != "" {
		t.Fatalf("not set up: annual = %q implied %q", draft.AnnualRate, draft.ImpliedAnnualRate)
	}
}

// A statement stores the payment in force on its date, but the math never
// reads a statement's payment: once the change is gone the origin's payment
// is back, which takes the math snapshots through WithRates even with no
// change left.
func TestRateChanges_StatementPaymentIsNeverRead(t *testing.T) {
	e := newEnv(t)
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "700")
	if _, err := e.reconcile(res.Id, "5000", "2026-03-01"); err != nil {
		t.Fatal(err)
	}
	if snaps := e.snapshots(t, res.Id); len(snaps) != 2 || snaps[1].Payment.String() != "700" {
		t.Fatalf("statement = %+v, want the payment in force 700", snaps[len(snaps)-1])
	}
	for _, c := range e.details(t, res.Id).RateChanges {
		if _, err := e.deleteRate(res.Id, c.Id); err != nil {
			t.Fatal(err)
		}
	}
	f, err := e.svc.GetDebtForecast(context.Background(), e.uid(), model.GetDebtForecastRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) == 0 || f.Items[0].Payment != "600" {
		t.Fatalf("forecast = %+v, want the origin's payment 600", f.Items)
	}
}

func TestCloneDebts_CopiesRateChanges(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.createOrigin(t, nil)
	e.mustAddRate(t, res.Id, "2025-09-01", "5", "650")
	copyTo := e.newBudget(e.user, "Copy", e.account)
	var ids map[vo.Id]vo.Id
	err := e.db.TX.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ids, err = e.svc.CloneDebts(ctx, vo.MustParseId(e.budget), vo.MustParseId(copyTo), nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	newID := ids[vo.MustParseId(res.Id)]
	src, err := e.repo.RateChanges(ctx, []vo.Id{vo.MustParseId(res.Id)})
	if err != nil {
		t.Fatal(err)
	}
	cp, err := e.repo.RateChanges(ctx, []vo.Id{newID})
	if err != nil {
		t.Fatal(err)
	}
	if len(cp) != 2 || len(src) != 2 {
		t.Fatalf("copies = %d, source %d", len(cp), len(src))
	}
	for i := range cp {
		c, o := cp[i], src[i]
		if c.ID.Equal(o.ID) || !c.DebtID.Equal(newID) || !c.EffectiveDate.Equal(o.EffectiveDate) ||
			!c.AnnualRate.Equals(o.AnnualRate) || !c.Payment.Equals(o.Payment) {
			t.Errorf("copy %d = %+v, source %+v", i, c, o)
		}
	}
}
