package model

import (
	"testing"
	"time"
)

func TestTrialEnd(t *testing.T) {
	cases := []struct {
		name         string
		registeredAt time.Time
		days         int
		want         time.Time
	}{
		{"30 days", time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC), 30, time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)},
		{"1 day", time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC), 1, time.Date(2026, 8, 1, 23, 59, 0, 0, time.UTC)},
		{"across the year boundary", time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC), 30, time.Date(2027, 1, 14, 12, 0, 0, 0, time.UTC)},
		{"preserves time of day", time.Date(2026, 3, 10, 6, 15, 30, 0, time.UTC), 7, time.Date(2026, 3, 17, 6, 15, 30, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrialEnd(tc.registeredAt, tc.days); !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTrialEndNormalizesToUTC(t *testing.T) {
	loc := time.FixedZone("UTC+5", 5*3600)
	registered := time.Date(2026, 7, 2, 3, 0, 0, 0, loc)
	got := TrialEnd(registered, 10)
	want := registered.UTC().AddDate(0, 0, 10)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("got %v (%v), want %v (UTC)", got, got.Location(), want)
	}
}
