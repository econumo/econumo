package model

import (
	"errors"
	"testing"
	"time"
)

func rateLoan() []MathSnapshot {
	return []MathSnapshot{{AsOf: ymd(2026, time.May, 1), Balance: 594078.30, Payment: 2647.82,
		Schedule: RecurringScheduleMonthly, Last: ymd(2056, time.May, 1), Digits: 2, Origin: true}}
}

// rateLoanPays are the regular payments on the 1st from June to November.
func rateLoanPays() []MathPayment {
	var out []MathPayment
	for m := time.June; m <= time.November; m++ {
		out = append(out, pay(m.String(), ymd(2026, m, 1), 2647.82))
	}
	return out
}

func TestRates_NoChangesKeepTodaysFigures(t *testing.T) {
	snaps, pays := rateLoan(), rateLoanPays()
	want := SplitPayments(snaps, pays)
	got := SplitPayments(WithRates(snaps, nil), pays)
	for id, w := range want {
		if got[id] != w {
			t.Fatalf("%s: interest %v, want %v", id, got[id], w)
		}
	}
}

// A change dated on a due applies to the period after it: the payment that
// day pays the period before at the old rate.
func TestRates_ChangeOnADueAppliesToTheNextPeriod(t *testing.T) {
	snaps, pays := rateLoan(), rateLoanPays()
	plain := SplitPayments(snaps, pays)
	withRates := WithRates(snaps, []MathRate{{From: ymd(2026, time.September, 1), PeriodRate: AnnualToPeriodRate(3.69, RecurringScheduleMonthly), Payment: 2647.82}})
	got := SplitPayments(withRates, pays)
	near(t, "September interest", got["September"], plain["September"], 1e-9)
	afterSep := BalanceAt(withRates, pays, got, ymd(2026, time.September, 1))
	near(t, "October interest", got["October"], afterSep*0.0369/12, 1e-6)
}

func TestRates_MidPeriodChangeIsProratedByDays(t *testing.T) {
	snaps, pays := rateLoan(), rateLoanPays()
	implied := mustRate(t, snaps[0])
	withRates := WithRates(snaps, []MathRate{{From: ymd(2026, time.September, 16), PeriodRate: AnnualToPeriodRate(3.69, RecurringScheduleMonthly), Payment: 2647.82}})
	got := SplitPayments(withRates, pays)
	near(t, "September interest", got["September"], SplitPayments(snaps, pays)["September"], 1e-9)
	afterSep := BalanceAt(withRates, pays, got, ymd(2026, time.September, 1))
	near(t, "October interest", got["October"], afterSep*(implied*15+0.0369/12*15)/30, 1e-6)
}

func TestRates_PaymentChangeDrivesTheProjection(t *testing.T) {
	snaps, pays := rateLoan(), rateLoanPays()
	implied := mustRate(t, snaps[0])
	today := ymd(2026, time.November, 2)
	project := func(s []MathSnapshot) Projection {
		return ProjectDebt(ProjectionInput{Digits: 2, Snaps: s, Pays: pays, Interest: SplitPayments(s, pays), Today: today})
	}
	before := project(snaps)
	p := project(WithRates(snaps, []MathRate{{From: ymd(2026, time.December, 1), PeriodRate: implied, Payment: 3000}}))
	if len(p.Payments) < 2 || !p.Payments[0].Date.Equal(ymd(2026, time.December, 1)) {
		t.Fatalf("payments = %+v", p.Payments[:min(2, len(p.Payments))])
	}
	for _, pp := range p.Payments[:len(p.Payments)-1] {
		if pp.Payment != 3000 {
			t.Fatalf("payment on %v = %v, want 3000", pp.Date, pp.Payment)
		}
	}
	if p.PaymentsLeft >= before.PaymentsLeft {
		t.Fatalf("payments left %d, want fewer than %d", p.PaymentsLeft, before.PaymentsLeft)
	}
	s := WithRates(snaps, []MathRate{{From: ymd(2026, time.December, 1), PeriodRate: implied, Payment: 3000}})
	m := ProjectDebt(ProjectionInput{Digits: 2, Snaps: s, Pays: pays, Interest: SplitPayments(s, pays), Today: today,
		Months: []time.Time{ymd(2026, time.November, 1)}}).Months["2026-11-01"]
	if m.PaymentsLeft != p.PaymentsLeft {
		t.Fatalf("November payments left %d, want %d", m.PaymentsLeft, p.PaymentsLeft)
	}
}

// A statement is checked at the rate in force: a change the payment no longer
// covers is refused, a lower one passes.
func TestRates_StatementRepayableUsesTheRateInForce(t *testing.T) {
	origin := carLoan()
	st := statementOf(origin, ymd(2026, time.November, 12), 15000)
	if err := StatementRepayable(origin, st); err != nil {
		t.Fatalf("without changes: %v", err)
	}
	high := WithRates([]MathSnapshot{origin, st}, []MathRate{{From: ymd(2026, time.December, 1), PeriodRate: 0.04, Payment: 450}})
	if err := StatementRepayable(high[0], high[1]); !errors.Is(err, ErrPaymentsDontCover) {
		t.Fatalf("at 4%% a period: err = %v, want ErrPaymentsDontCover", err)
	}
	raised := WithRates([]MathSnapshot{origin, st}, []MathRate{{From: ymd(2026, time.December, 1), PeriodRate: 0.04, Payment: 700}})
	if err := StatementRepayable(raised[0], raised[1]); err != nil {
		t.Fatalf("at 4%% with 700: %v", err)
	}
}

func TestRates_HigherRateSamePaymentMovesThePayoffLater(t *testing.T) {
	snaps, pays := rateLoan(), rateLoanPays()
	s := WithRates(snaps, []MathRate{{From: ymd(2026, time.December, 1), PeriodRate: AnnualToPeriodRate(3.69, RecurringScheduleMonthly), Payment: 2647.82}})
	p := ProjectDebt(ProjectionInput{Digits: 2, Snaps: s, Pays: pays, Interest: SplitPayments(s, pays), Today: ymd(2026, time.November, 2)})
	if !p.PayoffDate.After(ymd(2056, time.May, 1)) {
		t.Fatalf("payoff %v, want after the contract's 2056-05-01", p.PayoffDate)
	}
}

func TestRates_ZeroRateSplitsNoInterest(t *testing.T) {
	snaps := rateLoan()
	pays := append([]MathPayment{pay("adj", ymd(2026, time.May, 1), 1175.27)}, rateLoanPays()...)
	got := SplitPayments(WithRates(snaps, []MathRate{{From: ymd(2026, time.May, 1), PeriodRate: 0, Payment: 2647.82}}), pays)
	for id, v := range got {
		if v != 0 {
			t.Fatalf("%s: interest %v, want 0", id, v)
		}
	}
}

func TestRates_StartDateChangeReplacesTheImpliedRate(t *testing.T) {
	snaps := WithRates(rateLoan(), []MathRate{{From: ymd(2026, time.May, 1), PeriodRate: AnnualToPeriodRate(3.5, RecurringScheduleMonthly), Payment: 2647.82}})
	near(t, "rate in force", PeriodRateOn(snaps, ymd(2026, time.June, 1)), 0.035/12, 1e-12)
	pays := []MathPayment{pay("adj", ymd(2026, time.May, 1), 1175.27), pay("jun", ymd(2026, time.June, 1), 2647.82)}
	got := SplitPayments(snaps, pays)
	near(t, "start-day interest", got["adj"], min(1175.27, 594078.30*0.035/12), 1e-9)
	near(t, "June interest", got["jun"], 594078.30*0.035/12, 1e-6)
}

func TestRates_ExtraPaidAndScheduledUseThePaymentInForce(t *testing.T) {
	snaps := rateLoan()
	implied := mustRate(t, snaps[0])
	pays := append(rateLoanPays(), pay("dec", ymd(2026, time.December, 1), 3000))
	today := ymd(2026, time.December, 2)
	dec := ymd(2026, time.December, 1)
	project := func(s []MathSnapshot) Projection {
		return ProjectDebt(ProjectionInput{Digits: 2, Snaps: s, Pays: pays, Interest: SplitPayments(s, pays), Today: today, Months: []time.Time{dec}})
	}
	near(t, "extra paid without the change", project(snaps).ExtraPaid, 3000-2647.82, 1e-9)
	s := WithRates(snaps, []MathRate{{From: dec, PeriodRate: implied, Payment: 3000}})
	near(t, "payment in force", PaymentOn(s, dec), 3000, 0)
	near(t, "payment before the change", PaymentOn(s, dec.AddDate(0, 0, -1)), 2647.82, 0)
	p := project(s)
	near(t, "extra paid", p.ExtraPaid, 0, 1e-9)
	near(t, "December scheduled", p.Months["2026-12-01"].ScheduledTotal, 3000, 1e-9)
}

func TestRates_PeriodRateConversions(t *testing.T) {
	near(t, "weekly", AnnualToPeriodRate(5.2, RecurringScheduleWeekly), 0.001, 1e-15)
	near(t, "monthly", PeriodToAnnualRate(0.003075, RecurringScheduleMonthly), 3.69, 1e-12)
	near(t, "biweekly round trip", PeriodToAnnualRate(AnnualToPeriodRate(4.2, RecurringScheduleBiweekly), RecurringScheduleBiweekly), 4.2, 1e-12)
	near(t, "quarterly", AnnualToPeriodRate(8, RecurringScheduleQuarterly), 0.02, 1e-15)
	near(t, "yearly", AnnualToPeriodRate(3, RecurringScheduleYearly), 0.03, 1e-15)
}

func TestDueWindowContaining(t *testing.T) {
	last := ymd(2056, time.May, 1)
	start, end := DueWindowContaining(RecurringScheduleMonthly, last, ymd(2026, time.September, 20))
	if !start.Equal(ymd(2026, time.September, 16)) || !end.Equal(ymd(2026, time.October, 16)) {
		t.Fatalf("window = %v..%v", start, end)
	}
}

// A change inside the origin's first window splits that window's interest by
// days, from the start date.
func TestRates_ChangeInsideTheOriginsFirstPeriod(t *testing.T) {
	origin := carLoan()
	implied := mustRate(t, origin)
	r2 := AnnualToPeriodRate(6, RecurringScheduleMonthly)
	snaps := WithRates([]MathSnapshot{origin}, []MathRate{{From: ymd(2026, time.May, 25), PeriodRate: r2, Payment: 450}})
	pays := []MathPayment{pay("jun", ymd(2026, time.June, 10), 450)}
	got := SplitPayments(snaps, pays)
	near(t, "June interest", got["jun"], 16500*(implied*10+r2*16)/31, 1e-6)
	near(t, "without the change", SplitPayments([]MathSnapshot{origin}, pays)["jun"], 16500*implied*26/31, 1e-6)
}

func TestRates_TwoChangesInOneWindow(t *testing.T) {
	snaps, pays := rateLoan(), rateLoanPays()
	implied := mustRate(t, snaps[0])
	r1, r2 := AnnualToPeriodRate(3, RecurringScheduleMonthly), AnnualToPeriodRate(5, RecurringScheduleMonthly)
	s := WithRates(snaps, []MathRate{
		{From: ymd(2026, time.September, 11), PeriodRate: r1, Payment: 2647.82},
		{From: ymd(2026, time.September, 21), PeriodRate: r2, Payment: 2647.82},
	})
	got := SplitPayments(s, pays)
	afterSep := BalanceAt(s, pays, got, ymd(2026, time.September, 1))
	near(t, "October interest", got["October"], afterSep*(implied*10+r1*10+r2*10)/30, 1e-6)
}

func TestRates_WeeklyScheduleWindowFormula(t *testing.T) {
	start := ymd(2026, time.January, 1)
	snap := MathSnapshot{AsOf: start, Balance: 1000, Payment: 50, Schedule: RecurringScheduleWeekly, Last: start.AddDate(0, 0, 364), Digits: 2}
	r0 := mustRate(t, snap)
	r1 := AnnualToPeriodRate(5.2, RecurringScheduleWeekly)
	s := WithRates([]MathSnapshot{snap}, []MathRate{{From: start.AddDate(0, 0, 10), PeriodRate: r1, Payment: 50}})
	pays := []MathPayment{pay("w1", start.AddDate(0, 0, 7), 50), pay("w2", start.AddDate(0, 0, 14), 50)}
	got := SplitPayments(s, pays)
	after1 := BalanceAt(s, pays, got, start.AddDate(0, 0, 7))
	near(t, "second week interest", got["w2"], after1*(r0*3+r1*4)/7, 1e-6)
}

// A payment that does not cover the interest now is no refusal while a later
// change repays the balance.
func TestRates_RelaxedEarlyExitWaitsForALaterChange(t *testing.T) {
	origin := carLoan()
	st := statementOf(origin, ymd(2026, time.November, 12), 15000)
	now := MathRate{From: ymd(2026, time.November, 12), PeriodRate: 0.04, Payment: 450}
	later := MathRate{From: ymd(2027, time.March, 1), PeriodRate: 0.04, Payment: 900}

	alone := WithRates([]MathSnapshot{origin, st}, []MathRate{now})
	if err := StatementRepayable(alone[0], alone[1]); !errors.Is(err, ErrPaymentsDontCover) {
		t.Fatalf("without the later change: err = %v, want ErrPaymentsDontCover", err)
	}
	both := WithRates([]MathSnapshot{origin, st}, []MathRate{now, later})
	if err := StatementRepayable(both[0], both[1]); err != nil {
		t.Fatalf("with the later change: %v", err)
	}
	eps := settledBelow(2)
	terms := snapshotOn(both, st.AsOf)
	horizon := len(DueDatesBetween(terms.Schedule, terms.Last, st.AsOf, st.AsOf.AddDate(0, maxProjectMonths, 0)))
	if n := paymentsLeftAt(both, 15000, st.AsOf, eps); n <= 0 || n >= horizon {
		t.Fatalf("payments left = %d, want a finite count below the horizon %d", n, horizon)
	}
	if n := paymentsLeftAt(alone, 15000, st.AsOf, eps); n != horizon {
		t.Fatalf("payments left without the later change = %d, want the horizon %d", n, horizon)
	}
}
