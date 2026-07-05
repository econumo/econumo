package account

// White-box tests for the timezone-aware balance day boundary. tomorrowIn is the
// heart of the fix: the account balance is "as of end of TODAY", and "today"
// must be the caller's local day (from the request timezone), not the server's
// UTC day — otherwise a user behind UTC sees their next-day, future transactions
// counted once UTC rolls past midnight.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v (is time/tzdata embedded?)", name, err)
	}
	return l
}

func TestTomorrowIn(t *testing.T) {
	const layout = "2006-01-02 15:04:05"
	ny := mustLoc(t, "America/New_York") // UTC-4 in June (EDT)
	tokyo := mustLoc(t, "Asia/Tokyo")    // UTC+9

	cases := []struct {
		name string
		now  time.Time
		loc  *time.Location
		want string
	}{
		// 02:00 UTC on the 22nd: UTC day is the 22nd -> cutoff = start of the 23rd.
		{"utc", time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC), time.UTC, "2026-06-23 00:00:00"},
		// Same instant is 22:00 on the 21st in New York -> their day is the 21st
		// -> cutoff = start of the 22nd (so a tx dated the 22nd is correctly future).
		{"behind-utc", time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC), ny, "2026-06-22 00:00:00"},
		// Same instant is 11:00 on the 22nd in Tokyo -> day is the 22nd -> the 23rd.
		{"ahead-utc-same-day", time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC), tokyo, "2026-06-23 00:00:00"},
		// 20:00 on the 21st UTC is already 05:00 on the 22nd in Tokyo -> their day is
		// the 22nd -> cutoff the 23rd (UTC would give the 22nd): ahead-of-UTC users
		// get a later boundary, including the rest of their actual day.
		{"ahead-utc-next-day", time.Date(2026, 6, 21, 20, 0, 0, 0, time.UTC), tokyo, "2026-06-23 00:00:00"},
		// nil location is treated as UTC.
		{"nil-loc", time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC), nil, "2026-06-23 00:00:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tomorrowIn(c.now, c.loc)
			// Must be UTC-typed so the DB driver serializes the bare wall-clock that
			// matches naive spent_at strings (no offset conversion).
			if got.Location() != time.UTC {
				t.Errorf("cutoff location = %v, want UTC", got.Location())
			}
			if g := got.Format(layout); g != c.want {
				t.Errorf("tomorrowIn(%s, %v) = %q, want %q", c.now.Format(layout), c.loc, g, c.want)
			}
		})
	}
}

func TestBalanceBefore_UsesRequestTimezone(t *testing.T) {
	const layout = "2006-01-02 15:04:05"
	ny := mustLoc(t, "America/New_York")
	// 02:00 UTC on the 22nd == 22:00 on the 21st in New York.
	s := &Service{clock: fixedClock{t: time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC)}}

	// No location in context -> server UTC day.
	if g := s.balanceBefore(context.Background()).Format(layout); g != "2026-06-23 00:00:00" {
		t.Errorf("default balanceBefore = %q, want 2026-06-23 00:00:00 (UTC)", g)
	}
	// Caller's New York timezone -> the user's day boundary (one day earlier here).
	ctx := reqctx.WithLocation(context.Background(), ny)
	if g := s.balanceBefore(ctx).Format(layout); g != "2026-06-22 00:00:00" {
		t.Errorf("New York balanceBefore = %q, want 2026-06-22 00:00:00", g)
	}
}
