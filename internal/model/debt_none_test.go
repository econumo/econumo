package model

import (
	"testing"
	"time"
)

// A loan from family: 5000 from March 1, returned by the end of 2027, no
// payment schedule. Two payments came off in full, and plans of 1000 a month
// from December repay the 3000 left in February.
func TestDebtMath_NoFixedPayments(t *testing.T) {
	origin := MathSnapshot{AsOf: ymd(2026, time.March, 1), Balance: 5000, Schedule: DebtScheduleNone,
		Last: ymd(2027, time.December, 31), Digits: 2, Origin: true}
	snaps := WithRates([]MathSnapshot{origin}, nil)
	pays := []MathPayment{
		{ID: "a", Date: ymd(2026, time.May, 10), Amount: 1000},
		{ID: "b", Date: ymd(2026, time.August, 1), Amount: 1000},
	}
	if r, err := SnapshotRate(snaps[0]); err != nil || r != 0 {
		t.Fatalf("rate = %v, %v; want 0, nil", r, err)
	}
	if err := StatementRepayable(snaps[0], MathSnapshot{AsOf: ymd(2026, time.June, 1), Balance: 4000,
		Schedule: DebtScheduleNone, Last: origin.Last, Digits: 2}); err != nil {
		t.Fatalf("StatementRepayable = %v, want nil", err)
	}
	if dues := DueDatesBetween(DebtScheduleNone, origin.Last, origin.AsOf, origin.Last); len(dues) != 0 {
		t.Fatalf("dues = %v, want none", dues)
	}
	interest := SplitPayments(snaps, pays)
	if interest["a"] != 0 || interest["b"] != 0 {
		t.Fatalf("interest = %v, want 0 on every payment", interest)
	}
	today := ymd(2026, time.November, 14)
	months := []time.Time{ymd(2026, time.November, 1), ymd(2026, time.December, 1), ymd(2027, time.January, 1),
		ymd(2027, time.February, 1), ymd(2027, time.March, 1)}
	in := ProjectionInput{Snaps: snaps, Pays: pays, Interest: interest, Today: today, Months: months, Digits: 2}

	base := ProjectDebt(in)
	if base.CurrentBalance != 3000 || base.PaymentsLeft != 0 || base.PeriodsPaid != 0 || base.ExtraPaid != 0 ||
		!base.NextDueDate.IsZero() || !base.PayoffDate.IsZero() || base.PaidOff {
		t.Fatalf("without plans = %+v", base)
	}
	for key, m := range base.Months {
		if m.ScheduledTotal != 0 || m.StillDue != 0 || m.PaymentsLeft != 0 {
			t.Errorf("%s = %+v, want nothing scheduled", key, m)
		}
	}

	in.Plans = map[string]float64{"2026-12-01": 1000, "2027-01-01": 1000, "2027-02-01": 1000, "2027-03-01": 1000}
	planned := ProjectDebt(in)
	if !planned.PayoffDate.Equal(ymd(2027, time.February, 28)) {
		t.Fatalf("payoff = %v, want 2027-02-28", planned.PayoffDate)
	}
	if planned.PaymentsLeft != 0 || planned.OverpaymentLeft != 0 {
		t.Fatalf("with plans = %+v", planned)
	}
	want := map[string]float64{"2026-11-01": 3000, "2026-12-01": 2000, "2027-01-01": 1000, "2027-02-01": 0, "2027-03-01": 0}
	for key, closing := range want {
		if m := planned.Months[key]; m.ClosingBalance != closing || m.ScheduledTotal != 0 {
			t.Errorf("%s = %+v, want closing %v and nothing scheduled", key, m, closing)
		}
	}

	// a statement still resets the balance
	stated := WithRates([]MathSnapshot{origin, {AsOf: ymd(2026, time.September, 1), Balance: 2500, Schedule: DebtScheduleNone,
		Last: origin.Last, Digits: 2}}, nil)
	if got := BalanceAt(stated, pays, interest, today); got != 2500 {
		t.Fatalf("balance after a statement = %v, want 2500", got)
	}
}

func TestMonthlyToFinish(t *testing.T) {
	today := ymd(2026, time.November, 14)
	if got, ok := MonthlyToFinish(3000, today, ymd(2027, time.October, 20), 1, 2); !ok || got != 250 {
		t.Fatalf("12 periods = %v, %v; want 250", got, ok)
	}
	if got, ok := MonthlyToFinish(1000, today, ymd(2027, time.January, 31), 1, 2); !ok || got != 333.33 {
		t.Fatalf("3 periods = %v, %v; want 333.33", got, ok)
	}
	if _, ok := MonthlyToFinish(3000, today, ymd(2026, time.October, 31), 1, 2); ok {
		t.Fatal("a return-by period already past has no monthly figure")
	}
	if _, ok := MonthlyToFinish(0.004, today, ymd(2027, time.October, 20), 1, 2); ok {
		t.Fatal("nothing owed has no monthly figure")
	}
	// periods starting on the 15th: today is in October's, the return-by day in September's
	if got, ok := MonthlyToFinish(1200, today, ymd(2027, time.October, 10), 15, 2); !ok || got != 100 {
		t.Fatalf("shifted periods = %v, %v; want 100", got, ok)
	}
}
