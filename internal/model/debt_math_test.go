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
	r, err := ImpliedRate(300000, 2000, 240, 2)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "r", r, 0.0042676, 1e-6)
}

func TestImpliedRate_InterestFree(t *testing.T) {
	r, err := ImpliedRate(12000, 1000, 12, 2)
	if err != nil || r != 0 {
		t.Fatalf("got %v, %v; want 0, nil", r, err)
	}
}

func TestImpliedRate_DoesNotCover(t *testing.T) {
	if _, err := ImpliedRate(12000, 900, 12, 2); !errors.Is(err, ErrPaymentsDontCover) {
		t.Fatalf("err = %v, want ErrPaymentsDontCover", err)
	}
}

// The interest-free tolerance is half the currency's smallest unit: in an
// 8-digit currency a thousandth short is a real shortfall, and a thousandth
// over is real interest.
func TestImpliedRate_ToleranceFollowsCurrencyPrecision(t *testing.T) {
	if _, err := ImpliedRate(0.01, 0.001, 9, 8); !errors.Is(err, ErrPaymentsDontCover) {
		t.Fatalf("8 digits, 9 x 0.001 for 0.01: err = %v, want ErrPaymentsDontCover", err)
	}
	if r, err := ImpliedRate(0.01, 0.001, 12, 8); err != nil || r <= 0 {
		t.Fatalf("8 digits, 12 x 0.001 for 0.01: got %v, %v; want a positive rate", r, err)
	}
	if r, err := ImpliedRate(0.01, 0.001, 10, 8); err != nil || r != 0 {
		t.Fatalf("8 digits, exact: got %v, %v; want 0", r, err)
	}
	// two digits: both differences are below half a cent
	if r, err := ImpliedRate(0.01, 0.001, 9, 2); err != nil || r != 0 {
		t.Fatalf("2 digits, 9 x 0.001: got %v, %v; want 0", r, err)
	}
	// zero digits: half a unit
	if r, err := ImpliedRate(1000, 100.04, 10, 0); err != nil || r != 0 {
		t.Fatalf("0 digits, 0.4 over: got %v, %v; want 0", r, err)
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
	r, err := SnapshotRate(s)
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
	return ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: asOf, Months: months, Plans: plans}), asOf
}

func TestProjectDebt_Reference(t *testing.T) {
	asOf := ymd(2026, time.January, 1)
	snap := refLoan()
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: asOf})
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
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: asOf, Months: months})
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
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: ymd(2026, time.July, 1)})
	if !p.PaidOff || !p.PayoffMonth.Equal(ymd(2026, time.June, 1)) || p.PaymentsLeft != 0 {
		t.Fatalf("got %+v", p)
	}
}

func TestProjectDebt_PaymentAfterZeroSnapshotReopens(t *testing.T) {
	asOf := ymd(2026, time.June, 10)
	snap := MathSnapshot{AsOf: asOf, Balance: 0, Payment: 2000, Schedule: RecurringScheduleMonthly, Last: ymd(2030, time.June, 1)}
	pays := []MathPayment{pay("a", ymd(2026, time.June, 20), 500)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in, Today: ymd(2026, time.July, 1)})
	if p.PaidOff {
		t.Fatalf("a payment after the zero snapshot must reopen: %+v", p)
	}
}

func TestProjectDebt_PaymentTakesBalanceToZero(t *testing.T) {
	asOf := ymd(2026, time.June, 10)
	snap := MathSnapshot{AsOf: asOf, Balance: 1500, Payment: 1500, Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.July, 1)}
	pays := []MathPayment{pay("a", ymd(2026, time.July, 1), 1500)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in, Today: ymd(2026, time.July, 15)})
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
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in,
		Today: ymd(2026, time.April, 10), Months: months})
	near(t, "February closing", p.Months["2026-02-01"].ClosingBalance, 300000-(2000-in["a"]), 1e-6)
	near(t, "overpaid so far", p.OverpaidSoFar, in["a"], 1e-9)
}

func TestProjectDebt_OverdueDueCountsInCurrentMonth(t *testing.T) {
	snap := MathSnapshot{AsOf: ymd(2026, time.August, 28), Balance: 1000, Payment: 250,
		Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.December, 28)}
	months := []time.Time{ymd(2026, time.September, 1), ymd(2026, time.October, 1), ymd(2026, time.November, 1)}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: ymd(2026, time.October, 5), Months: months})
	sep, oct, nov := p.Months["2026-09-01"], p.Months["2026-10-01"], p.Months["2026-11-01"]
	near(t, "October still due", oct.StillDue, 500, 1e-9)
	if oct.PaymentsLeft != 2 {
		t.Fatalf("October PaymentsLeft = %d, want 2", oct.PaymentsLeft)
	}
	near(t, "November still due", nov.StillDue, 250, 1e-9)
	near(t, "September still due", sep.StillDue, 0, 1e-9)
	if p.PaymentsLeft != 4 {
		t.Fatalf("PaymentsLeft = %d, want 4", p.PaymentsLeft)
	}
	if !p.NextDueDate.Equal(ymd(2026, time.September, 28)) {
		t.Fatalf("NextDueDate = %v, want the overdue Sep 28", p.NextDueDate)
	}
}

func TestSplit_WindowContainingAsOfAlreadyAccrued(t *testing.T) {
	snap := refLoan()
	snap.AsOf = ymd(2026, time.February, 10)
	snap.Last = ymd(2046, time.February, 1)

	got := SplitPayments([]MathSnapshot{snap}, []MathPayment{pay("b", ymd(2026, time.February, 12), 2000)})
	near(t, "interest without an earlier payment in the window", got["b"], 300000*mustRate(t, snap), 0.01)

	ps := []MathPayment{pay("a", ymd(2026, time.February, 5), 2000), pay("b", ymd(2026, time.February, 12), 2000)}
	got = SplitPayments([]MathSnapshot{snap}, ps)
	near(t, "interest after a payment dated before the snapshot", got["b"], 0, 1e-9)
}

func TestSplit_MultipleSnapshotsResetBalance(t *testing.T) {
	s1 := refLoan()
	s2 := MathSnapshot{AsOf: ymd(2026, time.March, 1), Balance: 100000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2036, time.March, 1)}
	snaps := []MathSnapshot{s1, s2}
	ps := []MathPayment{pay("a", ymd(2026, time.February, 1), 2000), pay("b", ymd(2026, time.April, 1), 2000)}
	in := SplitPayments(snaps, ps)
	near(t, "first payment under the first snapshot", in["a"], 1280.29, 0.01)
	near(t, "second payment under the second snapshot", in["b"], 100000*mustRate(t, s2), 0.01)
	near(t, "balance after the second payment", BalanceAt(snaps, ps, in, ymd(2026, time.April, 1)), 100000-(2000-in["b"]), 1e-6)
}

func TestProjectDebt_NothingScheduledAfterProjectedPayoff(t *testing.T) {
	asOf := ymd(2026, time.October, 10)
	snap := MathSnapshot{AsOf: asOf, Balance: 10000, Payment: 1000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2027, time.September, 20)}
	months := []time.Time{ymd(2026, time.September, 1), ymd(2026, time.October, 1), ymd(2026, time.November, 1), ymd(2026, time.December, 1)}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: asOf, Months: months,
		Plans: map[string]float64{"2026-11-01": 20000}})
	if !p.PayoffDate.Equal(ymd(2026, time.November, 20)) {
		t.Fatalf("PayoffDate = %v, want the November due", p.PayoffDate)
	}
	for key, want := range map[string]float64{"2026-09-01": 1000, "2026-10-01": 1000, "2026-11-01": 1000, "2026-12-01": 0} {
		near(t, key+" scheduled", p.Months[key].ScheduledTotal, want, 1e-9)
	}

	paid := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{{AsOf: asOf, Balance: 0, Payment: 1000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2027, time.September, 20)}}, Today: asOf, Months: months})
	near(t, "paid off: September scheduled", paid.Months["2026-09-01"].ScheduledTotal, 1000, 1e-9)
	near(t, "paid off: October scheduled", paid.Months["2026-10-01"].ScheduledTotal, 0, 1e-9)
}

// A current-month plan already met by this month's payments projects no
// further extra: the balance already holds them.
func TestProjectDebt_CurrentMonthExtraAlreadyPaid(t *testing.T) {
	snap := MathSnapshot{AsOf: ymd(2026, time.October, 20), Balance: 300000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2046, time.September, 25)}
	pays := []MathPayment{pay("a", ymd(2026, time.October, 25), 7000)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	months := []time.Time{ymd(2026, time.October, 1), ymd(2026, time.November, 1)}
	project := func(plans map[string]float64) Projection {
		return ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in,
			Today: ymd(2026, time.October, 26), Months: months, Plans: plans})
	}
	none, planned := project(nil), project(map[string]float64{"2026-10-01": 7000})
	near(t, "October closing", planned.Months["2026-10-01"].ClosingBalance, 300000-(7000-in["a"]), 0.01)
	near(t, "October closing vs no plan", planned.Months["2026-10-01"].ClosingBalance, none.Months["2026-10-01"].ClosingBalance, 1e-6)
	near(t, "MoneyLeft vs no plan", planned.MoneyLeft, none.MoneyLeft, 1e-6)
	near(t, "OverpaymentLeft vs no plan", planned.OverpaymentLeft, none.OverpaymentLeft, 1e-6)
	if !planned.PayoffDate.Equal(none.PayoffDate) {
		t.Fatalf("PayoffDate = %v, want %v", planned.PayoffDate, none.PayoffDate)
	}

	// Only the part of the plan above what was paid and what is still due
	// this month is a further extra.
	more := project(map[string]float64{"2026-10-01": 10000})
	near(t, "October closing with 3000 more planned", more.Months["2026-10-01"].ClosingBalance, none.Months["2026-10-01"].ClosingBalance-3000, 1e-6)
}

// A regular due still to come this month counts against the plan, next to
// what was already paid.
func TestProjectDebt_CurrentMonthPlanCountsPaidAndStillDue(t *testing.T) {
	snap := MathSnapshot{AsOf: ymd(2026, time.September, 1), Balance: 300000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2046, time.September, 28)}
	pays := []MathPayment{pay("sep", ymd(2026, time.September, 28), 2000), pay("x", ymd(2026, time.October, 5), 5000)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	months := []time.Time{ymd(2026, time.October, 1)}
	project := func(plans map[string]float64) Projection {
		return ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in,
			Today: ymd(2026, time.October, 10), Months: months, Plans: plans})
	}
	none, planned := project(nil), project(map[string]float64{"2026-10-01": 7000})
	near(t, "October closing vs no plan", planned.Months["2026-10-01"].ClosingBalance, none.Months["2026-10-01"].ClosingBalance, 1e-6)
	near(t, "MoneyLeft vs no plan", planned.MoneyLeft, none.MoneyLeft, 1e-6)
}

func TestImpliedRate_AboveOneHundredPercent(t *testing.T) {
	r, err := ImpliedRate(100, 300, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "r", r, 2, 1e-9)
}

// Backward estimation runs from the balance the stored splits actually leave,
// not from a freshly computed interest.
func TestSplit_BackwardUsesStoredInterest(t *testing.T) {
	snap := refLoan()
	stored := 500.0
	later := pay("b", ymd(2025, time.December, 1), 2000)
	later.Interest = &stored
	ps := []MathPayment{pay("a", ymd(2025, time.November, 1), 2000), later}
	got := SplitPayments([]MathSnapshot{snap}, ps)
	near(t, "stored interest kept", got["b"], 500, 1e-9)
	r := mustRate(t, snap)
	end := snap.Balance + (2000 - 500)
	near(t, "earlier payment's interest", got["a"], (end+2000)/(1+r)*r, 1e-6)
}

func TestProjectDebt_PastMonthPaymentsLeftAreBooked(t *testing.T) {
	snap := MathSnapshot{AsOf: ymd(2026, time.January, 1), Balance: 1200, Payment: 100,
		Schedule: RecurringScheduleMonthly, Last: ymd(2027, time.January, 1)}
	pays := []MathPayment{pay("a", ymd(2026, time.February, 1), 100), pay("b", ymd(2026, time.March, 1), 100)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	months := []time.Time{ymd(2026, time.January, 1), ymd(2026, time.February, 1), ymd(2026, time.March, 1)}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in,
		Today: ymd(2026, time.March, 15), Months: months})
	for key, want := range map[string]int{"2026-01-01": 12, "2026-02-01": 11, "2026-03-01": 10} {
		if got := p.Months[key].PaymentsLeft; got != want {
			t.Fatalf("%s PaymentsLeft = %d, want %d", key, got, want)
		}
	}
}

func TestProjectDebt_NoPayoffDateBeyondTheHorizon(t *testing.T) {
	asOf := ymd(2026, time.January, 1)
	snap := MathSnapshot{AsOf: asOf, Balance: 180000, Payment: 100, Schedule: RecurringScheduleMonthly, Last: asOf.AddDate(150, 0, 0)}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: asOf})
	if !p.PayoffDate.IsZero() || !p.BasePayoffDate.IsZero() {
		t.Fatalf("payoff dates = %v / %v, want none: the projection never clears the balance", p.PayoffDate, p.BasePayoffDate)
	}
	// what the horizon leaves unpaid is still owed: money left never falls
	// below the balance, so the overpayment is never negative
	near(t, "money left", p.MoneyLeft, 180000, 1e-6)
	near(t, "overpayment left", p.OverpaymentLeft, 0, 1e-6)
	near(t, "base overpayment left", p.BaseOverpaymentLeft, 0, 1e-6)
}

// The payoff rule rounds to the debt currency: 0.4 yen is nothing, 0.004
// dinar is still owed.
func TestProjectDebt_PaidOffRoundsToTheCurrency(t *testing.T) {
	project := func(balance float64, digits int) Projection {
		snap := MathSnapshot{AsOf: ymd(2026, time.January, 1), Balance: balance, Payment: 100,
			Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.March, 1)}
		zero := 0.0
		p := pay("a", ymd(2026, time.February, 1), 100)
		p.Interest = &zero
		return ProjectDebt(ProjectionInput{Digits: digits, Snaps: []MathSnapshot{snap}, Pays: []MathPayment{p},
			Interest: map[string]float64{"a": 0}, Today: ymd(2026, time.February, 15)})
	}
	if yen := project(100.4, 0); !yen.PaidOff || yen.PaymentsLeft != 0 {
		t.Fatalf("yen: %+v, want paid off", yen)
	}
	if dinar := project(100.004, 3); dinar.PaidOff || dinar.PaymentsLeft != 1 {
		t.Fatalf("dinar: %+v, want 0.004 still owed in one payment", dinar)
	}
}

// Marking a debt paid off after a payment that came once the balance was
// already zero settles it: the zero snapshot is the latest event.
func TestProjectDebt_ZeroSnapshotAfterAnOverpaymentIsPaidOff(t *testing.T) {
	snaps := []MathSnapshot{{AsOf: ymd(2026, time.January, 1), Balance: 200, Payment: 100,
		Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.March, 1)}}
	pays := []MathPayment{pay("a", ymd(2026, time.February, 1), 200), pay("b", ymd(2026, time.February, 5), 50)}
	in := map[string]float64{"a": 0, "b": 0}
	today := ymd(2026, time.February, 10)
	if p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: snaps, Pays: pays, Interest: in, Today: today}); p.PaidOff {
		t.Fatalf("a payment after the zero balance reopens: %+v", p)
	}
	marked := append(snaps, MathSnapshot{AsOf: today, Balance: 0, Payment: 100, Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.March, 1)})
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: marked, Pays: pays, Interest: in, Today: today})
	if !p.PaidOff || !p.PayoffMonth.Equal(ymd(2026, time.February, 1)) {
		t.Fatalf("got %+v, want paid off in February", p)
	}
}

// A due paid early, in the previous month, still counts as this month's
// scheduled payment: planning that amount is no extra.
func TestProjectDebt_DuePaidEarlyLastMonthIsNoExtra(t *testing.T) {
	snap := MathSnapshot{AsOf: ymd(2026, time.October, 1), Balance: 20000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2027, time.August, 1)}
	pays := []MathPayment{pay("early", ymd(2026, time.October, 30), 2000)}
	in := SplitPayments([]MathSnapshot{snap}, pays)
	months := []time.Time{ymd(2026, time.November, 1)}
	project := func(plans map[string]float64) Projection {
		return ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays, Interest: in,
			Today: ymd(2026, time.November, 5), Months: months, Plans: plans})
	}
	none, planned := project(nil), project(map[string]float64{"2026-11-01": 2000})
	near(t, "November closing vs no plan", planned.Months["2026-11-01"].ClosingBalance, none.Months["2026-11-01"].ClosingBalance, 1e-6)
	if !planned.PayoffDate.Equal(none.PayoffDate) {
		t.Fatalf("PayoffDate = %v, want %v: the plan is the scheduled amount, not an extra", planned.PayoffDate, none.PayoffDate)
	}
	more := project(map[string]float64{"2026-11-01": 3000})
	near(t, "November closing with 1000 above the schedule", more.Months["2026-11-01"].ClosingBalance, none.Months["2026-11-01"].ClosingBalance-1000, 1e-6)
}

// An overdue last installment clears the balance in the projection, but the
// debt is not paid off before today: the overdue date stays the next due.
func TestProjectDebt_OverduePayoffIsNotInThePast(t *testing.T) {
	snap := MathSnapshot{AsOf: ymd(2026, time.August, 28), Balance: 250, Payment: 250,
		Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.September, 28)}
	today := ymd(2026, time.October, 5)
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: today})
	if !p.NextDueDate.Equal(ymd(2026, time.September, 28)) {
		t.Fatalf("NextDueDate = %v, want the overdue Sep 28", p.NextDueDate)
	}
	if !p.PayoffDate.Equal(today) || !p.BasePayoffDate.Equal(today) {
		t.Fatalf("payoff dates = %v / %v, want today", p.PayoffDate, p.BasePayoffDate)
	}
	extra := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{{AsOf: snap.AsOf, Balance: 500, Payment: 250,
		Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.October, 28)}}, Today: today,
		Plans: map[string]float64{"2026-10-01": 500}})
	if extra.PayoffDate.Before(today) {
		t.Fatalf("PayoffDate = %v, an extra clearing the debt cannot pay it off before today", extra.PayoffDate)
	}
}

// A payment entered late, dated before one already split in the same window,
// takes only what the stored split left of the window's interest.
func TestSplit_LateEntryInASplitWindowReservesTheStoredInterest(t *testing.T) {
	stored := 1280.29
	later := pay("b", ymd(2026, time.February, 5), 2000)
	later.Interest = &stored
	got := SplitPayments([]MathSnapshot{refLoan()}, []MathPayment{pay("a", ymd(2026, time.February, 3), 500), later})
	near(t, "stored kept", got["b"], 1280.29, 1e-9)
	near(t, "late entry", got["a"], 0, 1e-9)

	partial := 1000.0
	later.Interest = &partial
	got = SplitPayments([]MathSnapshot{refLoan()}, []MathPayment{pay("a", ymd(2026, time.February, 3), 500), later})
	near(t, "window total", got["a"]+got["b"], 1280.29, 0.01)

	// the same before the first snapshot
	snap := refLoan()
	snap.AsOf = ymd(2026, time.March, 1)
	snap.Last = snap.AsOf.AddDate(20, 0, 0)
	fresh := SplitPayments([]MathSnapshot{snap}, []MathPayment{pay("b", ymd(2026, time.February, 5), 2000)})["b"]
	backStored := fresh
	back := pay("b", ymd(2026, time.February, 5), 2000)
	back.Interest = &backStored
	got = SplitPayments([]MathSnapshot{snap}, []MathPayment{pay("a", ymd(2026, time.February, 3), 500), back})
	r := mustRate(t, snap)
	near(t, "backward window total", got["a"]+got["b"], (snap.Balance+2500)/(1+r)*r, 1e-6)
}

func TestProjectDebt_PeriodStartDay(t *testing.T) {
	// 12000 interest-free, 1000 on the 10th through 2027-06-10; today is
	// 2026-06-15, so the next due is 10 Jul.
	in := ProjectionInput{
		Snaps: []MathSnapshot{{AsOf: ymd(2026, time.June, 15), Balance: 12000, Payment: 1000,
			Schedule: RecurringScheduleMonthly, Last: ymd(2027, time.June, 10)}},
		Today:  ymd(2026, time.June, 15),
		Months: []time.Time{ymd(2026, time.June, 1), ymd(2026, time.July, 1)},
		Plans:  map[string]float64{"2026-06-01": 1500},
		Digits: 2,
	}
	calendar := ProjectDebt(in)
	in.PeriodStartDay = 1
	if got := ProjectDebt(in); got.Months["2026-06-01"] != calendar.Months["2026-06-01"] || !got.PayoffDate.Equal(calendar.PayoffDate) {
		t.Fatalf("day 1 = %+v, want the calendar projection %+v", got.Months, calendar.Months)
	}
	if m := calendar.Months["2026-06-01"]; m.StillDue != 0 || m.ClosingBalance != 11500 {
		t.Errorf("calendar June = %+v, want nothing still due and the extra at month end", m)
	}

	in.PeriodStartDay = 15
	p := ProjectDebt(in)
	june, july := p.Months["2026-06-01"], p.Months["2026-07-01"]
	if june.StillDue != 1000 || june.ScheduledTotal != 1000 || june.ClosingBalance != 10500 || june.PaymentsLeft != 11 {
		t.Errorf("period June (15 Jun - 14 Jul) = %+v, want the 10 Jul due, 500 extra after it", june)
	}
	if july.StillDue != 1000 || july.ClosingBalance != 9500 {
		t.Errorf("period July (15 Jul - 14 Aug) = %+v, want the 10 Aug due", july)
	}
}

func TestProjectDebt_PeriodsPaid(t *testing.T) {
	asOf := ymd(2026, time.January, 1)
	snap := MathSnapshot{AsOf: asOf, Balance: 12000, Payment: 1000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.December, 15)}
	pays := []MathPayment{
		{ID: "jan", Date: ymd(2026, time.January, 15), Amount: 1000},
		{ID: "feb1", Date: ymd(2026, time.February, 14), Amount: 500},
		{ID: "feb2", Date: ymd(2026, time.February, 16), Amount: 500},
		{ID: "apr", Date: ymd(2026, time.April, 15), Amount: 1000},
		{ID: "future", Date: ymd(2026, time.June, 15), Amount: 1000},
	}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: pays,
		Interest: SplitPayments([]MathSnapshot{snap}, pays), Today: ymd(2026, time.May, 1)})
	if p.PeriodsPaid != 3 {
		t.Fatalf("PeriodsPaid = %d, want 3", p.PeriodsPaid)
	}
}

// A past month's payments left are bounded by the projection's 100-year
// horizon, not by 1200 payments: a 30-year weekly loan has about 1565.
func TestProjectDebt_PastMonthPaymentsLeftOfALongWeeklyLoan(t *testing.T) {
	asOf := ymd(2026, time.January, 1)
	snap := MathSnapshot{AsOf: asOf, Payment: 100, Schedule: RecurringScheduleWeekly, Last: ymd(2056, time.January, 1), Digits: 2}
	n := len(DueDatesBetween(snap.Schedule, snap.Last, snap.AsOf, snap.Last))
	if n <= 1200 {
		t.Fatalf("n = %d, want a loan longer than 1200 payments", n)
	}
	snap.Balance = 100 * float64(n)
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: ymd(2026, time.March, 15),
		Months: []time.Time{ymd(2025, time.December, 1), ymd(2026, time.March, 1)}})
	if got := p.Months["2025-12-01"].PaymentsLeft; got != n {
		t.Fatalf("December PaymentsLeft = %d, want %d", got, n)
	}
	if p.PaymentsLeft <= 1200 {
		t.Fatalf("current PaymentsLeft = %d, want above 1200", p.PaymentsLeft)
	}
}

// From the current period on, a period's scheduled figure is what the
// projection actually pays in it: a smaller final payment is all that is
// scheduled, so a plan of exactly that is not below schedule. A due already
// paid this period keeps counting.
func TestProjectDebt_ScheduledTotalUsesTheFinalSmallerPayment(t *testing.T) {
	snap := MathSnapshot{AsOf: ymd(2026, time.March, 1), Balance: 1500, Payment: 500, Schedule: RecurringScheduleMonthly,
		Last: ymd(2026, time.June, 1), Digits: 2}
	paid := []MathPayment{{ID: "p", Date: ymd(2026, time.April, 1), Amount: 880}}
	months := []time.Time{ymd(2026, time.April, 1), ymd(2026, time.May, 1), ymd(2026, time.June, 1), ymd(2026, time.July, 1)}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: paid, Interest: map[string]float64{"p": 0},
		Today: ymd(2026, time.April, 10), Months: months})
	for key, want := range map[string]float64{"2026-04-01": 500, "2026-05-01": 500, "2026-06-01": 120, "2026-07-01": 0} {
		near(t, key+" scheduled", p.Months[key].ScheduledTotal, want, 1e-9)
	}
	// a past period keeps the term-based figure
	past := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Pays: paid, Interest: map[string]float64{"p": 0},
		Today: ymd(2026, time.July, 10), Months: []time.Time{ymd(2026, time.June, 1)}})
	near(t, "past June scheduled", past.Months["2026-06-01"].ScheduledTotal, 500, 1e-9)
}

// The projected payments are the run behind the figures: one per regular
// payment, interest first, the rest off the balance, down to a smaller last one.
func TestProjectDebt_PaymentsAreTheRunItself(t *testing.T) {
	asOf := ymd(2026, time.January, 1)
	snap := MathSnapshot{AsOf: asOf, Balance: 1000, Payment: 300, Schedule: RecurringScheduleMonthly, Last: ymd(2026, time.April, 1), Digits: 2}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: asOf})
	if len(p.Payments) != p.PaymentsLeft || p.PaymentsLeft == 0 {
		t.Fatalf("payments = %d, PaymentsLeft = %d", len(p.Payments), p.PaymentsLeft)
	}
	rate := PeriodRate(snap)
	balance, total := 1000.0, 0.0
	for i, pay := range p.Payments {
		near(t, "interest", pay.Interest, balance*rate, 1e-9)
		near(t, "principal", pay.Principal, pay.Payment-pay.Interest, 1e-9)
		balance -= pay.Principal
		near(t, "balance", pay.Balance, balance, 1e-9)
		total += pay.Payment
		if i > 0 && !pay.Date.After(p.Payments[i-1].Date) {
			t.Fatalf("payment %d dated %v, not after %v", i, pay.Date, p.Payments[i-1].Date)
		}
	}
	last := p.Payments[len(p.Payments)-1]
	near(t, "last balance", last.Balance, 0, 1e-9)
	if last.Payment >= 300 {
		t.Fatalf("last payment = %v, want the smaller remainder", last.Payment)
	}
	if !last.Date.Equal(p.PayoffDate) {
		t.Fatalf("last payment on %v, payoff %v", last.Date, p.PayoffDate)
	}
	near(t, "paid", total, p.MoneyLeft, 1e-9)
}

// A planned extra rides on its period's first payment, fully to the debt.
func TestProjectDebt_PaymentsCarryThePlannedExtra(t *testing.T) {
	p, _ := projectRef(map[string]float64{"2026-10-01": 5000})
	var oct *ProjectedPayment
	for i := range p.Payments {
		if p.Payments[i].Date.Year() == 2026 && p.Payments[i].Date.Month() == time.October {
			oct = &p.Payments[i]
			break
		}
	}
	if oct == nil {
		t.Fatal("no October payment")
	}
	near(t, "October payment", oct.Payment, 2000+3000, 1e-9)
	near(t, "October extra", oct.Extra, 3000, 1e-9)
	near(t, "first payment's extra", p.Payments[len(p.Payments)-1].Extra, 0, 1e-9)
	near(t, "October principal", oct.Principal, oct.Payment-oct.Interest, 1e-9)
	near(t, "October closing", oct.Balance, p.Months["2026-10-01"].ClosingBalance, 1e-9)
	if len(p.Payments) != p.PaymentsLeft {
		t.Fatalf("payments = %d, PaymentsLeft = %d", len(p.Payments), p.PaymentsLeft)
	}
}

// A period with an extra but no regular payment pays it at the period's end,
// as a payment of its own with no interest.
func TestProjectDebt_PaymentsHoldAnExtraOfAPeriodWithoutADue(t *testing.T) {
	asOf := ymd(2026, time.January, 1)
	snap := MathSnapshot{AsOf: asOf, Balance: 10000, Payment: 1000, Schedule: RecurringScheduleQuarterly, Last: ymd(2028, time.October, 1), Digits: 2}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{snap}, Today: asOf, Plans: map[string]float64{"2026-02-01": 500}})
	var feb *ProjectedPayment
	for i := range p.Payments {
		if p.Payments[i].Date.Equal(ymd(2026, time.February, 28)) {
			feb = &p.Payments[i]
		}
	}
	if feb == nil {
		t.Fatalf("no extra on Feb 28 in %+v", p.Payments[:3])
	}
	near(t, "extra", feb.Payment, 500, 1e-9)
	near(t, "extra part", feb.Extra, 500, 1e-9)
	near(t, "extra interest", feb.Interest, 0, 1e-9)
	near(t, "extra principal", feb.Principal, 500, 1e-9)
	if len(p.Payments) != p.PaymentsLeft+1 {
		t.Fatalf("payments = %d, want the %d regular ones and the extra", len(p.Payments), p.PaymentsLeft)
	}
}

// originLoan: 12000 taken on 15 Jan 2026, 24 payments of 600 on the 20th from
// 20 Jan 2026 (a first period of 5 of December 20 - January 20's 31 days).
func originLoan() MathSnapshot {
	return MathSnapshot{AsOf: ymd(2026, time.January, 15), Balance: 12000, Payment: 600,
		Schedule: RecurringScheduleMonthly, Last: ymd(2027, time.December, 20), Digits: 2, Origin: true}
}

// The rate solves the contract with its short first period: amortizing with
// the prorated first period ends at 0 after the last due.
func TestSnapshotRate_OriginSolvesTheProratedFirstPeriod(t *testing.T) {
	s := originLoan()
	r, err := SnapshotRate(s)
	if err != nil || r <= 0 {
		t.Fatalf("rate = %v, %v", r, err)
	}
	balance := s.Balance*(1+r*5.0/31.0) - s.Payment
	for i := 1; i < 24; i++ {
		balance = balance*(1+r) - s.Payment
	}
	near(t, "balance after the last due", balance, 0, 1e-6)
	if full, _ := ImpliedRate(12000, 600, 24, 2); r <= full {
		t.Fatalf("rate %v, want above the full-period rate %v (less interest in the first period)", r, full)
	}
	if _, err := SnapshotRate(MathSnapshot{AsOf: s.AsOf, Balance: 20000, Payment: 600, Schedule: s.Schedule, Last: s.Last, Digits: 2, Origin: true}); !errors.Is(err, ErrPaymentsDontCover) {
		t.Fatalf("err = %v, want ErrPaymentsDontCover", err)
	}
}

func TestSplit_OriginFirstPeriodIsProratedByDays(t *testing.T) {
	s := originLoan()
	r := mustRate(t, s)
	pays := []MathPayment{pay("jan", ymd(2026, time.January, 20), 600), pay("feb", ymd(2026, time.February, 20), 600)}
	got := SplitPayments([]MathSnapshot{s}, pays)
	first := 12000 * r * 5 / 31
	near(t, "first period interest", got["jan"], first, 1e-9)
	near(t, "second period interest", got["feb"], (12000-(600-first))*r, 1e-9)
}

// A payment on the start day accrues nothing: all of it comes off the amount
// borrowed, and the balance after that day shows it.
func TestSplit_OriginDayPaymentIsAllPrincipal(t *testing.T) {
	s := originLoan()
	r := mustRate(t, s)
	pays := []MathPayment{pay("down", ymd(2026, time.January, 15), 1000), pay("jan", ymd(2026, time.January, 20), 600)}
	got := SplitPayments([]MathSnapshot{s}, pays)
	if got["down"] != 0 {
		t.Fatalf("start-day interest = %v, want 0", got["down"])
	}
	near(t, "first period interest", got["jan"], 11000*r*5/31, 1e-9)
	near(t, "balance on the start day", BalanceAt([]MathSnapshot{s}, pays, got, s.AsOf), 11000, 1e-9)
	near(t, "balance before the start", BalanceAt([]MathSnapshot{s}, pays, got, s.AsOf.AddDate(0, 0, -1)), 12000, 1e-9)
}

// A window keyed by a due on or before the start accrues nothing: the loan
// had not begun at that due.
func TestSplit_OriginWindowOfAnEarlierDueAccruesNothing(t *testing.T) {
	s := originLoan()
	s.AsOf = ymd(2026, time.January, 1) // in December 20's window
	pays := []MathPayment{pay("early", ymd(2026, time.January, 2), 600)}
	if got := SplitPayments([]MathSnapshot{s}, pays); got["early"] != 0 {
		t.Fatalf("interest = %v, want 0", got["early"])
	}
}

func TestProjectDebt_OriginFirstProjectedPaymentIsProrated(t *testing.T) {
	s := originLoan()
	r := mustRate(t, s)
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: []MathSnapshot{s}, Today: ymd(2026, time.January, 16)})
	if len(p.Payments) == 0 || !p.Payments[0].Date.Equal(ymd(2026, time.January, 20)) {
		t.Fatalf("payments = %+v", p.Payments)
	}
	near(t, "first projected interest", p.Payments[0].Interest, 12000*r*5/31, 1e-9)
	if p.PaymentsLeft != 24 || !p.PayoffDate.Equal(s.Last) {
		t.Fatalf("payments left %d, payoff %v; want the contract's 24 to its last date", p.PaymentsLeft, p.PayoffDate)
	}
}

// After a reconciliation every due up to its date counts as paid, plus the
// windows after it holding a recorded payment.
func TestProjectDebt_ReconciliationCountsTheDuesBeforeIt(t *testing.T) {
	origin := MathSnapshot{AsOf: ymd(2021, time.January, 1), Balance: 300000, Payment: 2000,
		Schedule: RecurringScheduleMonthly, Last: ymd(2051, time.January, 1), Digits: 2, Origin: true}
	rec := origin
	rec.Origin, rec.AsOf, rec.Balance = false, ymd(2026, time.January, 1), 270000
	pays := []MathPayment{pay("feb", ymd(2026, time.February, 1), 2000)}
	snaps := []MathSnapshot{origin, rec}
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: snaps, Pays: pays, Interest: SplitPayments(snaps, pays), Today: ymd(2026, time.February, 10)})
	if p.PeriodsPaid != 61 {
		t.Fatalf("PeriodsPaid = %d, want 60 dues to the statement and February's", p.PeriodsPaid)
	}
	// with the origin alone (no statement) only the recorded payment counts
	q := ProjectDebt(ProjectionInput{Digits: 2, Snaps: snaps[:1], Pays: pays, Interest: SplitPayments(snaps[:1], pays), Today: ymd(2026, time.February, 10)})
	if q.PeriodsPaid != 1 {
		t.Fatalf("PeriodsPaid without a statement = %d, want 1", q.PeriodsPaid)
	}
}
