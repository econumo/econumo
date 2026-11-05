package model

import (
	"testing"
	"time"
)

func projDate(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, time.UTC)
}

func dueDates(occ []RecurringOccurrence) []time.Time {
	out := make([]time.Time, len(occ))
	for i, o := range occ {
		out[i] = o.DueAt
	}
	return out
}

func assertDueDates(t *testing.T, occ []RecurringOccurrence, want ...time.Time) {
	t.Helper()
	got := dueDates(occ)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("occurrence %d: got %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestProjectOccurrences_MonthlyFuture(t *testing.T) {
	rt := &RecurringTransaction{Schedule: RecurringScheduleMonthly, NextPaymentAt: projDate(2026, 11, 15, 9), ScheduledDay: 15}
	occ := ProjectOccurrences(rt, projDate(2026, 10, 22, 0), projDate(2027, 1, 1, 0))
	assertDueDates(t, occ, projDate(2026, 11, 15, 9), projDate(2026, 12, 15, 9))
	if !occ[0].IsNext || occ[1].IsNext {
		t.Fatalf("IsNext wrong: %+v", occ)
	}
	for _, o := range occ {
		if o.IsOverdue || !o.FiledAt.Equal(o.DueAt) || o.Template != rt {
			t.Fatalf("unexpected occurrence %+v", o)
		}
	}
}

func TestProjectOccurrences_WeeklyInMonth(t *testing.T) {
	rt := &RecurringTransaction{Schedule: RecurringScheduleWeekly, NextPaymentAt: projDate(2026, 11, 4, 0), ScheduledDay: 4}
	occ := ProjectOccurrences(rt, projDate(2026, 10, 22, 0), projDate(2026, 12, 1, 0))
	assertDueDates(t, occ, projDate(2026, 11, 4, 0), projDate(2026, 11, 11, 0), projDate(2026, 11, 18, 0), projDate(2026, 11, 25, 0))
}

func TestProjectOccurrences_OverdueCollapses(t *testing.T) {
	rt := &RecurringTransaction{Schedule: RecurringScheduleWeekly, NextPaymentAt: projDate(2026, 9, 30, 0), ScheduledDay: 30}
	today := projDate(2026, 10, 22, 0)
	occ := ProjectOccurrences(rt, today, projDate(2026, 11, 1, 0))
	assertDueDates(t, occ, projDate(2026, 9, 30, 0), projDate(2026, 10, 28, 0))
	if !occ[0].IsOverdue || !occ[0].IsNext || !occ[0].FiledAt.Equal(today) {
		t.Fatalf("overdue occurrence wrong: %+v", occ[0])
	}
	if occ[1].IsOverdue || occ[1].IsNext || !occ[1].FiledAt.Equal(occ[1].DueAt) {
		t.Fatalf("follow-up occurrence wrong: %+v", occ[1])
	}
}

func TestProjectOccurrences_DueToday(t *testing.T) {
	rt := &RecurringTransaction{Schedule: RecurringScheduleMonthly, NextPaymentAt: projDate(2026, 10, 22, 10), ScheduledDay: 22}
	occ := ProjectOccurrences(rt, projDate(2026, 10, 22, 0), projDate(2026, 11, 1, 0))
	assertDueDates(t, occ, projDate(2026, 10, 22, 10))
	if occ[0].IsOverdue || !occ[0].IsNext || !occ[0].FiledAt.Equal(occ[0].DueAt) {
		t.Fatalf("due-today occurrence wrong: %+v", occ[0])
	}
}

func TestProjectOccurrences_MonthEndClamp(t *testing.T) {
	rt := &RecurringTransaction{Schedule: RecurringScheduleMonthly, NextPaymentAt: projDate(2027, 1, 31, 0), ScheduledDay: 31}
	occ := ProjectOccurrences(rt, projDate(2026, 10, 22, 0), projDate(2027, 4, 1, 0))
	assertDueDates(t, occ, projDate(2027, 1, 31, 0), projDate(2027, 2, 28, 0), projDate(2027, 3, 31, 0))
}

func TestProjectOccurrences_UntilBeforeNext(t *testing.T) {
	rt := &RecurringTransaction{Schedule: RecurringScheduleMonthly, NextPaymentAt: projDate(2026, 12, 15, 0), ScheduledDay: 15}
	occ := ProjectOccurrences(rt, projDate(2026, 10, 22, 0), projDate(2026, 12, 1, 0))
	if len(occ) != 0 {
		t.Fatalf("want none, got %v", dueDates(occ))
	}
}

func TestProjectOccurrences_OverdueOutsideWindow(t *testing.T) {
	rt := &RecurringTransaction{Schedule: RecurringScheduleWeekly, NextPaymentAt: projDate(2026, 9, 30, 0), ScheduledDay: 30}
	today := projDate(2026, 10, 22, 0)
	if occ := ProjectOccurrences(rt, today, today); len(occ) != 0 {
		t.Fatalf("want none, got %v", dueDates(occ))
	}
}
