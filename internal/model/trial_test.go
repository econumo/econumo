package model

import (
	"testing"
	"time"
)

func TestTrialEnd(t *testing.T) {
	cases := []struct {
		name         string
		registeredAt time.Time
		want         time.Time
	}{
		{"first of the month", time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"second of the month", time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"last day of a 31-day month", time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"february", time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"across the year boundary", time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC), time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"november wraps to january", time.Date(2026, 11, 30, 12, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrialEnd(tc.registeredAt); !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTrialEndAlwaysSpansAFullCalendarMonth(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 400; i++ {
		day := start.AddDate(0, 0, i)
		end := TrialEnd(day)
		nextMonth := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		lastInstant := nextMonth.AddDate(0, 1, 0).Add(-time.Nanosecond)
		if end.Before(lastInstant) {
			t.Fatalf("registered %v: trial ends %v, before the full month ending %v", day, end, lastInstant)
		}
	}
}
