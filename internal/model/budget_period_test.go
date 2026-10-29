package model

import (
	"testing"
	"time"
)

func periodDay(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s+" 00:00:00")
	if err != nil {
		panic(err)
	}
	return t
}

func periodTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPeriodWindow(t *testing.T) {
	cases := []struct {
		key        string
		startDay   int
		start, end string
	}{
		{"2026-10-01", 1, "2026-10-01", "2026-11-01"},
		{"2026-10-01", 5, "2026-10-05", "2026-11-05"},
		{"2026-10-01", 15, "2026-10-15", "2026-11-15"},
		{"2026-10-01", 16, "2026-09-16", "2026-10-16"},
		{"2026-11-01", 28, "2026-10-28", "2026-11-28"},
		{"2027-01-01", 28, "2026-12-28", "2027-01-28"},
		{"2028-03-01", 28, "2028-02-28", "2028-03-28"},
		{"2026-10-17", 5, "2026-10-05", "2026-11-05"},
		{"2026-10-01", 0, "2026-10-01", "2026-11-01"},
		{"2026-10-01", 29, "2026-10-01", "2026-11-01"},
	}
	for _, c := range cases {
		s, e := PeriodWindow(periodDay(c.key), c.startDay)
		if !s.Equal(periodDay(c.start)) || !e.Equal(periodDay(c.end)) {
			t.Errorf("PeriodWindow(%s, %d) = %s..%s, want %s..%s", c.key, c.startDay, s, e, c.start, c.end)
		}
	}
}

func TestPeriodWindowKeepsLocation(t *testing.T) {
	loc := time.FixedZone("x", 3*3600)
	s, e := PeriodWindow(time.Date(2026, 10, 1, 0, 0, 0, 0, loc), 5)
	if s.Location() != loc || e.Location() != loc {
		t.Fatalf("location lost: %v %v", s.Location(), e.Location())
	}
}

func TestPeriodOf(t *testing.T) {
	cases := []struct {
		at       string
		startDay int
		key      string
	}{
		{"2026-10-30 12:00:00", 28, "2026-11-01"},
		{"2026-10-27 23:59:59", 28, "2026-10-01"},
		{"2026-10-28 00:00:00", 28, "2026-11-01"},
		{"2026-10-04 00:00:00", 5, "2026-09-01"},
		{"2026-12-28 00:00:00", 28, "2027-01-01"},
		{"2028-02-29 00:00:00", 28, "2028-03-01"},
		{"2026-10-31 00:00:00", 1, "2026-10-01"},
	}
	for _, c := range cases {
		if got := PeriodOf(periodTime(c.at), c.startDay); !got.Equal(periodDay(c.key)) {
			t.Errorf("PeriodOf(%s, %d) = %s, want %s", c.at, c.startDay, got, c.key)
		}
	}
}

func TestPeriodRoundTrip(t *testing.T) {
	for sd := MinPeriodStartDay; sd <= MaxPeriodStartDay; sd++ {
		for day := periodDay("2026-01-01"); day.Before(periodDay("2029-01-01")); day = day.AddDate(0, 0, 1) {
			k := PeriodOf(day, sd)
			s, e := PeriodWindow(k, sd)
			if day.Before(s) || !day.Before(e) {
				t.Fatalf("day %s startDay %d: key %s window %s..%s misses it", day, sd, k, s, e)
			}
			if got := PeriodOf(s, sd); !got.Equal(k) {
				t.Fatalf("startDay %d: PeriodOf(start %s) = %s, want %s", sd, s, got, k)
			}
			if got := PeriodOf(e.Add(-time.Second), sd); !got.Equal(k) {
				t.Fatalf("startDay %d: PeriodOf(end-1s %s) = %s, want %s", sd, e, got, k)
			}
		}
	}
}

func TestPeriodKeyOfDay(t *testing.T) {
	cases := []struct {
		day      string
		startDay int
		want     string
	}{
		{"2026-10-28", 28, "2026-11-01"},
		{"2026-10-01", 1, "2026-10-01"},
		{"2026-10-01", 5, "2026-09-01"},
	}
	for _, c := range cases {
		got, err := PeriodKeyOfDay(c.day, c.startDay)
		if err != nil || got != c.want {
			t.Errorf("PeriodKeyOfDay(%s, %d) = %q, %v; want %q", c.day, c.startDay, got, err, c.want)
		}
	}
	if _, err := PeriodKeyOfDay("bad", 5); err == nil {
		t.Error("expected an error for an unparsable day")
	}
}

func TestValidPeriodStartDay(t *testing.T) {
	for _, v := range []int{0, 29, -1} {
		if ValidPeriodStartDay(v) {
			t.Errorf("%d should be invalid", v)
		}
	}
	for _, v := range []int{1, 15, 28} {
		if !ValidPeriodStartDay(v) {
			t.Errorf("%d should be valid", v)
		}
	}
}
