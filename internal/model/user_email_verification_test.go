package model

import (
	"testing"
	"time"
)

func TestEmailVerificationRetryAfter(t *testing.T) {
	sent := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ev := &EmailVerification{CreatedAt: sent}

	cases := []struct {
		name  string
		after time.Duration
		want  time.Duration
	}{
		{"at the moment of sending", 0, 60 * time.Second},
		{"mid-gap", 30 * time.Second, 30 * time.Second},
		// Sub-second remainders round UP, so a client that trusts the number
		// never retries a moment early and trips the server-side refusal.
		{"partial second remaining", 59500 * time.Millisecond, 1 * time.Second},
		{"exactly at the gap", 60 * time.Second, 0},
		{"past the gap", 90 * time.Second, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ev.RetryAfter(sent.Add(tc.after)); got != tc.want {
				t.Errorf("RetryAfter(+%v) = %v, want %v", tc.after, got, tc.want)
			}
		})
	}
}

// A clock that has drifted backwards must never yield a wait longer than the
// gap itself, which would strand the user behind a countdown they cannot wait out.
func TestEmailVerificationRetryAfterCapsAtTheGap(t *testing.T) {
	sent := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ev := &EmailVerification{CreatedAt: sent}
	if got := ev.RetryAfter(sent.Add(-5 * time.Minute)); got > EmailVerificationResendGap {
		t.Errorf("RetryAfter with a backwards clock = %v, want at most %v", got, EmailVerificationResendGap)
	}
}
