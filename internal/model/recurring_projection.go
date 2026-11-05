package model

import "time"

type RecurringOccurrence struct {
	Template *RecurringTransaction
	// DueAt is the payment's own date; an overdue occurrence keeps its original one.
	DueAt time.Time
	// FiledAt is the date used for month bucketing and range filters: DueAt,
	// or today's midnight for the overdue occurrence.
	FiledAt   time.Time
	IsNext    bool
	IsOverdue bool
}

const maxProjectedOccurrences = 1000

// ProjectOccurrences expands a template into its occurrences filed before
// until (exclusive). today is the caller's local midnight in the same wall
// clock as NextPaymentAt. Only the first occurrence can be overdue; the
// occurrences the schedule would have produced between it and today are
// dropped, not repeated.
func ProjectOccurrences(rt *RecurringTransaction, today, until time.Time) []RecurringOccurrence {
	var out []RecurringOccurrence
	cur := rt.NextPaymentAt
	for i := 0; i < maxProjectedOccurrences; i++ {
		overdue := cur.Before(today)
		filed := cur
		if overdue {
			filed = today
		}
		if !filed.Before(until) {
			break
		}
		if i == 0 || !overdue {
			out = append(out, RecurringOccurrence{
				Template:  rt,
				DueAt:     cur,
				FiledAt:   filed,
				IsNext:    i == 0,
				IsOverdue: overdue,
			})
		}
		cur = NextOccurrence(cur, rt.Schedule, rt.ScheduledDay)
	}
	return out
}
