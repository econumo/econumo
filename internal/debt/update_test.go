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
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	if n := len(e.snapshots(t, res.Id)); n != 1 {
		t.Fatalf("an unchanged save wrote a snapshot: %d", n)
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
	if got.Balance != "9000" {
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
	e.clock.now = e.clock.now.AddDate(0, 0, 1)

	// a balance whose interest at the loan's rate (about 3% a month) exceeds
	// the payment of 1000 is never repaid
	req := e.updateReq(res)
	req.Balance = vo.NewFlexString("50000")
	_, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)

	// Past the last payment date an unchanged save must still work.
	e.clock.now = e.clock.now.AddDate(2, 0, 0)
	rename := e.updateReq(res)
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), rename); err != nil {
		t.Fatalf("save of an old loan: %v", err)
	}
	rename.Balance = vo.NewFlexString("500")
	_, err = e.svc.UpdateDebt(ctx, e.uid(), rename)
	requireCode(t, err, errs.CodeDebtLastPaymentInPast)
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

func TestUpdateDebt_PaymentRoundingToZeroRefused(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	req := e.updateReq(res)
	req.Payment = vo.NewFlexString("0.001")
	_, err := e.svc.UpdateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeOutOfRange)

	create := e.createDebtReq()
	create.ElementId = e.newCategory(e.user, true)
	create.Payment = vo.NewFlexString("0.004")
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), create)
	requireCode(t, err, errs.CodeOutOfRange)
}

func (e *env) eur() string {
	return e.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
}

func TestUpdateDebt_GuestAndArchived(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	guest := e.partner(t, "guest", roleGuest)
	_, err := e.svc.UpdateDebt(context.Background(), vo.MustParseId(guest), e.updateReq(res))
	requireForbidden(t, err)
	e.fakeBudget(e.budget).archived = true
	_, err = e.svc.UpdateDebt(context.Background(), e.uid(), e.updateReq(res))
	requireArchived(t, err)
	_, err = e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id})
	requireArchived(t, err)
}
