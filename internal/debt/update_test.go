package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestUpdateDebt_SnapshotOnlyWhenValuesChange(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)

	req := e.updateReq(res)
	req.Name = "Renamed"
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 {
		t.Fatalf("a rename wrote a snapshot: %d", n)
	}

	e.clock.now = e.clock.now.AddDate(0, 0, 5)
	req.Balance = vo.NewFlexString("9000")
	got, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 || snaps[1].Balance.String() != "9000" || !snaps[1].AsOf.Equal(ymd(2026, 3, 15)) {
		t.Fatalf("snapshots = %+v", snaps)
	}
	if got.Name != "Renamed" || got.Balance != "9000" {
		t.Fatalf("result = %+v", got)
	}

	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 2 {
		t.Fatalf("an unchanged save wrote a snapshot: %d", n)
	}
}

func TestUpdateDebt_ValidatesChangedTermsOnly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)

	req := e.updateReq(res)
	req.Balance = vo.NewFlexString("30000")
	_, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)

	// Past the last payment date a rename must still work.
	e.clock.now = e.clock.now.AddDate(2, 0, 0)
	rename := e.updateReq(res)
	rename.Name = "Old loan"
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), rename); err != nil {
		t.Fatalf("rename of an old loan: %v", err)
	}
	rename.Balance = vo.NewFlexString("500")
	_, err = e.svc.UpdateDebt(ctx, e.uid(), rename)
	requireCode(t, err, errs.CodeDebtLastPaymentInPast)
}

func TestUpdateDebt_KeyChecksRunOnEdit(t *testing.T) {
	e := newEnv(t)
	payee := e.newPayee()
	e.create(t, nil)
	narrow := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-02-01") })
	req := e.updateReq(narrow)
	req.StartDate = nil
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	req = e.updateReq(narrow)
	req.PayeeId = &payee
	_, err = e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtKeyConflict)
}

type currentTerms struct {
	asOf, last, balance, payment, schedule string
}

func termsOf(asOf, last interface{ Format(string) string }, balance, payment vo.DecimalNumber, schedule model.RecurringSchedule) currentTerms {
	return currentTerms{asOf.Format("2006-01-02"), last.Format("2006-01-02"), balance.String(), payment.String(), string(schedule)}
}

func TestDebtCurrentMatchesLatestSnapshot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	id := vo.MustParseId(res.Id)

	check := func(step string) {
		t.Helper()
		d, err := e.repo.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		snaps := e.snapshots(t, res.Id)
		last := snaps[len(snaps)-1]
		got := termsOf(d.Current.AsOf, d.Current.LastPaymentDate, d.Current.Balance, d.Current.Payment, d.Current.Schedule)
		want := termsOf(last.AsOf, last.LastPaymentDate, last.Balance, last.Payment, last.Schedule)
		if got != want {
			t.Errorf("%s: Current = %+v, latest snapshot = %+v", step, got, want)
		}
	}
	check("create")

	e.clock.now = e.clock.now.AddDate(0, 0, 3)
	req := e.updateReq(res)
	req.Balance, req.Payment, req.Schedule = vo.NewFlexString("8000"), vo.NewFlexString("500"), "biweekly"
	req.LastPaymentDate = "2027-06-04"
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	check("update")

	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	check("no-op update")

	e.clock.now = e.clock.now.AddDate(0, 0, 3)
	if _, err := e.svc.MarkDebtPaidOff(ctx, e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	check("mark paid off")
	if n := len(e.snapshots(t, res.Id)); n != 3 {
		t.Fatalf("snapshots = %d, want 3", n)
	}
}

func TestMarkPaidOff_ZeroSnapshot(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = e.clock.now.AddDate(0, 0, 20)

	got, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsPaidOff != 1 || got.CurrentBalance != "0" || got.Balance != "0" || got.PaymentsLeft != 0 || got.PayoffDate != "" {
		t.Fatalf("result = %+v", got)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}
	z := snaps[1]
	if !z.AsOf.Equal(ymd(2026, 3, 30)) || !z.Balance.IsZero() || z.Payment.String() != "1000" ||
		z.Schedule != model.RecurringScheduleMonthly || !z.LastPaymentDate.Equal(ymd(2027, 3, 1)) {
		t.Fatalf("zero snapshot = %+v", z)
	}

	again, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	if err != nil || again.IsPaidOff != 1 || len(e.snapshots(t, res.Id)) != 2 {
		t.Fatalf("second mark: %+v, %v, %d snapshots", again, err, len(e.snapshots(t, res.Id)))
	}
}

func TestUpdateDebt_RematchKeepsStoredSplits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p1, p2 := e.newPayee(), e.newPayee()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.PayeeId = &p1 })
	e.clock.now = ymd(2026, 7, 1)
	a := e.expense(t, e.account, ymd(2026, 4, 1), "1000", p1)
	b := e.expense(t, e.account, ymd(2026, 5, 1), "1000", p2)
	e.sync(t)

	first := e.splits(t, res.Id)
	if len(first) != 1 || first[a] == "" || first[a] == "0" {
		t.Fatalf("splits = %v, want only a with interest", first)
	}

	// Widening to the whole category brings b in as a late entry; a keeps
	// its interest even though the balance was corrected.
	req := e.updateReq(res)
	req.PayeeId = nil
	req.Balance = vo.NewFlexString("5000")
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	second := e.splits(t, res.Id)
	if len(second) != 2 || second[a] != first[a] || second[b] == "" {
		t.Fatalf("after widening: %v (first %v)", second, first)
	}

	// Narrowing again drops b's split and keeps a's.
	req.PayeeId = &p1
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	third := e.splits(t, res.Id)
	if len(third) != 1 || third[a] != first[a] {
		t.Fatalf("after narrowing: %v", third)
	}
}

func TestUpdateDebt_CurrencyIsNotEditable(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	req := e.updateReq(res)
	req.CurrencyId = e.f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	got, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	if err != nil || got.CurrencyId != fixture.USD {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestUpdateDebt_PaymentRoundingToZeroRefused(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	req := e.updateReq(res)
	req.Payment = vo.NewFlexString("0.001")
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeOutOfRange)

	create := e.createReq()
	create.Payment = vo.NewFlexString("0.004")
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), create)
	requireCode(t, err, errs.CodeOutOfRange)
}

// A payment after a zero snapshot reopens the debt; marking it paid off again
// must then write a new zero snapshot rather than do nothing.
func TestMarkPaidOff_ReopenedDebtCanBeMarkedAgain(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	if _, err := e.svc.MarkDebtPaidOff(ctx, e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	e.expense(t, e.account, march10.AddDate(0, 0, 2), "50", "")
	e.clock.now = e.clock.now.AddDate(0, 0, 5)
	reopened, err := e.svc.GetDebt(ctx, e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil || reopened.Item.IsPaidOff != 0 {
		t.Fatalf("after a later payment: %+v, %v; want reopened", reopened, err)
	}
	got, err := e.svc.MarkDebtPaidOff(ctx, e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsPaidOff != 1 || len(e.snapshots(t, res.Id)) != 3 {
		t.Fatalf("second mark: %+v, %d snapshots; want paid off with a new zero snapshot", got, len(e.snapshots(t, res.Id)))
	}
}
