package model

import (
	"errors"
	"math"
	"testing"
	"time"
)

func ymd(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %v, want %v (+-%v)", name, got, want, tol)
	}
}

// refLoan is the reference loan: 300000 at 2000 a month over 240 dues, due on
// the 1st, snapshot taken on a due date.
func refLoan() MathSnapshot {
	asOf := ymd(2026, time.January, 1)
	return MathSnapshot{AsOf: asOf, Balance: 300000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: asOf.AddDate(20, 0, 0)}
}

func TestImpliedRate_ReferenceMortgage(t *testing.T) {
	r, err := ImpliedRate(300000, 2000, 240)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "r", r, 0.0042676, 1e-6)
}

func TestImpliedRate_InterestFree(t *testing.T) {
	r, err := ImpliedRate(12000, 1000, 12)
	if err != nil || r != 0 {
		t.Fatalf("got %v, %v; want 0, nil", r, err)
	}
}

func TestImpliedRate_DoesNotCover(t *testing.T) {
	if _, err := ImpliedRate(12000, 900, 12); !errors.Is(err, ErrPaymentsDontCover) {
		t.Fatalf("err = %v, want ErrPaymentsDontCover", err)
	}
}

func TestDueDatesBetween_MonthEndClamp(t *testing.T) {
	last := ymd(2026, time.March, 31)
	got := DueDatesBetween(RecurringScheduleMonthly, last, ymd(2025, time.December, 31), last)
	want := []time.Time{ymd(2026, time.January, 31), ymd(2026, time.February, 28), ymd(2026, time.March, 31)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDueDatesBetween_ExtendsPastLast(t *testing.T) {
	last := ymd(2026, time.January, 31)
	got := DueDatesBetween(RecurringScheduleMonthly, last, last, ymd(2026, time.April, 30))
	want := []time.Time{ymd(2026, time.February, 28), ymd(2026, time.March, 31), ymd(2026, time.April, 30)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDueDatesBetween_Weekly(t *testing.T) {
	last := ymd(2026, time.March, 20)
	got := DueDatesBetween(RecurringScheduleWeekly, last, ymd(2026, time.March, 1), ymd(2026, time.March, 31))
	want := []time.Time{ymd(2026, time.March, 6), ymd(2026, time.March, 13), ymd(2026, time.March, 20), ymd(2026, time.March, 27)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDueWindow(t *testing.T) {
	last := ymd(2030, time.January, 1)
	start, end := DueWindow(RecurringScheduleMonthly, last, ymd(2026, time.February, 1))
	if !start.Equal(ymd(2026, time.January, 16)) || !end.Equal(ymd(2026, time.February, 15)) {
		t.Fatalf("window = [%v, %v)", start, end)
	}
}

func pay(id string, date time.Time, amount float64) MathPayment {
	return MathPayment{ID: id, Date: date, Amount: amount}
}

func TestSplit_FirstRegularPayment(t *testing.T) {
	p := pay("a", ymd(2026, time.February, 1), 2000)
	got := SplitPayments([]MathSnapshot{refLoan()}, []MathPayment{p})
	near(t, "interest", got["a"], 1280.29, 0.01)
}

func TestSplit_ExtraPaymentGoesToDebt(t *testing.T) {
	p := pay("a", ymd(2026, time.February, 1), 5000)
	got := SplitPayments([]MathSnapshot{refLoan()}, []MathPayment{p})
	near(t, "interest", got["a"], 1280.29, 0.01)
	near(t, "debt part", 5000-got["a"], 3719.71, 0.01)
}

func TestSplit_SecondPaymentSameWindowNoInterest(t *testing.T) {
	ps := []MathPayment{pay("a", ymd(2026, time.February, 1), 2000), pay("b", ymd(2026, time.February, 5), 3000)}
	got := SplitPayments([]MathSnapshot{refLoan()}, ps)
	near(t, "first", got["a"], 1280.29, 0.01)
	near(t, "second", got["b"], 0, 1e-9)
}

func TestSplit_EarlyPaymentLandsOnItsDue(t *testing.T) {
	p := pay("a", ymd(2026, time.January, 28), 2000)
	snap := refLoan()
	snap.AsOf = ymd(2025, time.December, 20)
	snap.Last = ymd(2045, time.December, 1)
	got := SplitPayments([]MathSnapshot{snap}, []MathPayment{p})
	near(t, "interest", got["a"], 300000*mustRate(t, snap), 0.01)
	if got["a"] <= 0 {
		t.Fatal("early payment should take the Feb 1 due's interest")
	}
}

func mustRate(t *testing.T, s MathSnapshot) float64 {
	t.Helper()
	n := len(DueDatesBetween(s.Schedule, s.Last, s.AsOf, s.Last))
	r, err := ImpliedRate(s.Balance, s.Payment, n)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSplit_StoredInterestKeptAndCapped(t *testing.T) {
	stored := 1280.29
	p := pay("a", ymd(2026, time.February, 1), 1000)
	p.Interest = &stored
	got := SplitPayments([]MathSnapshot{refLoan()}, []MathPayment{p})
	near(t, "interest", got["a"], 1000, 1e-9)

	p.Amount = 2000
	got = SplitPayments([]MathSnapshot{refLoan()}, []MathPayment{p})
	near(t, "interest kept", got["a"], 1280.29, 1e-9)
}

func TestSplit_MissedWindowStaysFlat(t *testing.T) {
	snap := refLoan()
	ps := []MathPayment{pay("a", ymd(2026, time.February, 1), 2000), pay("c", ymd(2026, time.April, 1), 2000)}
	in := SplitPayments([]MathSnapshot{snap}, ps)
	afterFirst := BalanceAt([]MathSnapshot{snap}, ps, in, ymd(2026, time.February, 1))
	near(t, "flat over the missed March window", BalanceAt([]MathSnapshot{snap}, ps, in, ymd(2026, time.March, 20)), afterFirst, 1e-9)
	r := mustRate(t, snap)
	near(t, "interest of the April payment", in["c"], afterFirst*r, 0.01)
}

func TestSplit_BackwardBeforeFirstSnapshot(t *testing.T) {
	snap := refLoan()
	snap.AsOf = ymd(2026, time.March, 1)
	snap.Last = snap.AsOf.AddDate(20, 0, 0)
	ps := []MathPayment{pay("a", ymd(2026, time.January, 1), 2000), pay("b", ymd(2026, time.February, 1), 2000)}
	in := SplitPayments([]MathSnapshot{snap}, ps)
	if in["a"] <= 0 || in["b"] <= 0 {
		t.Fatalf("interest = %v, want > 0", in)
	}
	if before := BalanceAt([]MathSnapshot{snap}, ps, in, ymd(2025, time.December, 15)); before <= snap.Balance {
		t.Fatalf("balance before the payments = %v, want > %v", before, snap.Balance)
	}
	near(t, "balance at the snapshot", BalanceAt([]MathSnapshot{snap}, ps, in, snap.AsOf), snap.Balance, 1e-9)
}

func projectRef(plans map[string]float64) (Projection, time.Time) {
	asOf := ymd(2026, time.October, 20)
	snap := MathSnapshot{AsOf: asOf, Balance: 300000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2046, time.September, 25)}
	months := []time.Time{ymd(2026, time.October, 1), ymd(2026, time.November, 1)}
	return ProjectDebt(ProjectionInput{Snaps: []MathSnapshot{snap}, Today: asOf, Months: months, Plans: plans}), asOf
}

func TestProjectDebt_Reference(t *testing.T) {
	asOf := ymd(2026, time.January, 1)
	snap := refLoan()
	p := ProjectDebt(ProjectionInput{Snaps: []MathSnapshot{snap}, Today: asOf})
	if p.PaymentsLeft != 240 {
		t.Fatalf("PaymentsLeft = %d, want 240", p.PaymentsLeft)
	}
	near(t, "MoneyLeft", p.MoneyLeft, 480000, 1)
	near(t, "OverpaymentLeft", p.OverpaymentLeft, 180000, 1)
	near(t, "CurrentBalance", p.CurrentBalance, 300000, 1e-9)
	if !p.PayoffDate.Equal(snap.Last) || !p.BasePayoffDate.Equal(snap.Last) {
		t.Fatalf("payoff %v / base %v, want %v", p.PayoffDate, p.BasePayoffDate, snap.Last)
	}
	if p.PaidOff {
		t.Fatal("not paid off")
	}
}

func TestProjectDebt_PlannedExtraShortens(t *testing.T) {
	p, _ := projectRef(map[string]float64{"2026-10-01": 5000})
	if p.PaymentsLeft != 236 {
		t.Fatalf("PaymentsLeft = %d, want 236", p.PaymentsLeft)
	}
	near(t, "MoneyLeft", p.MoneyLeft, 474754.34, 1)
	want := p.PayoffDate
	for i := 0; i < 4; i++ {
		want = NextOccurrence(want, RecurringScheduleMonthly, 25)
	}
	if !p.BasePayoffDate.Equal(want) {
		t.Fatalf("BasePayoffDate = %v, want %v", p.BasePayoffDate, want)
	}
	if p.OverpaymentLeft >= p.BaseOverpaymentLeft {
		t.Fatalf("overpayment %v should be below base %v", p.OverpaymentLeft, p.BaseOverpaymentLeft)
	}
	oct := p.Months["2026-10-01"]
	near(t, "October scheduled", oct.ScheduledTotal, 2000, 1e-9)
	near(t, "October still due", oct.StillDue, 2000, 1e-9)
	if oct.ClosingBalance >= 300000-3000 {
		t.Fatalf("October closing = %v, extra not applied", oct.ClosingBalance)
	}
}

func TestProjectDebt_NoPaymentsYet(t *testing.T) {
	asOf := ymd(2026, time.October, 10)
	snap := MathSnapshot{AsOf: asOf, Balance: 300000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2046, time.October, 1)}
	months := []time.Time{ymd(2026, time.October, 1), ymd(2026, time.November, 1)}
	p := ProjectDebt(ProjectionInput{Snaps: []MathSnapshot{snap}, Today: asOf, Months: months})
	oct, nov := p.Months["2026-10-01"], p.Months["2026-11-01"]
	near(t, "October still due", oct.StillDue, 0, 1e-9)
	near(t, "November still due", nov.StillDue, 2000, 1e-9)
	near(t, "October closing", oct.ClosingBalance, 300000, 1e-9)
	if p.PaymentsLeft != 240 {
		t.Fatalf("PaymentsLeft = %d, want 240", p.PaymentsLeft)
	}
}

func TestProjectDebt_PaidOff(t *testing.T) {
	asOf := ymd(2026, time.June, 10)
	snap := MathSnapshot{AsOf: asOf, Balance: 0, Payment: 2000, Schedule: RecurringScheduleMonthly, Last: ymd(2030, time.June, 1)}
	p := ProjectDebt(ProjectionInput{Snaps: []MathSnapshot{snap}, Today: ymd(2026, time.July, 1)})
	if !p.PaidOff || !p.PayoffMonth.Equal(ymd(2026, time.June, 1)) || p.PaymentsLeft != 0 {
		t.Fatalf("got %+v", p)
	}
}

func TestProjectDebt_PaymentAfterZeroSnapshotReopens(t *testing.T) {
	asOf := ymd(2026, time.June, 10)
	snap := MathSnapshot{AsOf: asOf, Balance: 0, Payment: 2000, Schedule: RecurringScheduleMonthly, Last: ymd(2030, time.June, 1)}
	pays := []MathPayment{pay("a", ymd(2026, time.June, 20), 500)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	p := ProjectDebt(ProjectionInput{Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in, Today: ymd(2026, time.July, 1)})
	if p.PaidOff {
		t.Fatalf("a payment after the zero snapshot must reopen: %+v", p)
	}
}

func TestProjectDebt_PaymentTakesBalanceToZero(t *testing.T) {
	asOf := ymd(2026, time.June, 10)
	snap := MathSnapshot{AsOf: asOf, Balance: 1500, Payment: 1500, Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.July, 1)}
	pays := []MathPayment{pay("a", ymd(2026, time.July, 1), 1500)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	p := ProjectDebt(ProjectionInput{Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in, Today: ymd(2026, time.July, 15)})
	if !p.PaidOff || !p.PayoffMonth.Equal(ymd(2026, time.July, 1)) {
		t.Fatalf("got %+v", p)
	}
	near(t, "current balance", p.CurrentBalance, 0, 1e-6)
}

func TestProjectDebt_PastMonthClosesAtBookedBalance(t *testing.T) {
	snap := refLoan()
	pays := []MathPayment{pay("a", ymd(2026, time.February, 1), 2000)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	months := []time.Time{ymd(2026, time.February, 1)}
	p := ProjectDebt(ProjectionInput{Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in,
		Today: ymd(2026, time.April, 10), Months: months})
	near(t, "February closing", p.Months["2026-02-01"].ClosingBalance, 300000-(2000-in["a"]), 1e-6)
	near(t, "overpaid so far", p.OverpaidSoFar, in["a"], 1e-9)
}
