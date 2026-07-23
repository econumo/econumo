package model

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func accessAt(t time.Time) *time.Time { return &t }

func TestEffectiveAccessLevel(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		level AccessLevel
		until *time.Time
		want  AccessLevel
	}{
		{"no expiry keeps the level", AccessLevelFull, nil, AccessLevelFull},
		{"future expiry keeps the level", AccessLevelFull, accessAt(now.Add(time.Hour)), AccessLevelFull},
		{"past expiry restricts", AccessLevelFull, accessAt(now.Add(-time.Hour)), AccessLevelReadonly},
		{"expiry exactly now restricts", AccessLevelFull, accessAt(now), AccessLevelReadonly},
		{"readonly with future expiry stays readonly", AccessLevelReadonly, accessAt(now.Add(time.Hour)), AccessLevelReadonly},
		{"readonly with no expiry stays readonly", AccessLevelReadonly, nil, AccessLevelReadonly},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := &User{AccessLevel: tc.level, AccessUntil: tc.until}
			if got := u.EffectiveAccessLevel(now); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseAccessLevel(t *testing.T) {
	if got, err := ParseAccessLevel("full"); err != nil || got != AccessLevelFull {
		t.Fatalf("full: got %q err %v", got, err)
	}
	if got, err := ParseAccessLevel("readonly"); err != nil || got != AccessLevelReadonly {
		t.Fatalf("readonly: got %q err %v", got, err)
	}
	if _, err := ParseAccessLevel("pro"); err == nil {
		t.Fatal("expected an error for an unknown level")
	}
}

func TestSetAccessBumpsUpdatedAt(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	u := &User{AccessLevel: AccessLevelFull, UpdatedAt: now.Add(-time.Hour)}

	u.SetAccess(AccessLevelReadonly, nil, now)

	if u.AccessLevel != AccessLevelReadonly {
		t.Fatalf("level: got %q", u.AccessLevel)
	}
	if u.AccessUntil != nil {
		t.Fatalf("until: got %v want nil", u.AccessUntil)
	}
	if !u.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt: got %v want %v", u.UpdatedAt, now)
	}
}

func TestNewUserDefaultsToFullAccess(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	u := NewUser(vo.NewId(), "email", "Name", "face:sky", "hash", "salt", now)
	if u.AccessLevel != AccessLevelFull {
		t.Fatalf("level: got %q want full", u.AccessLevel)
	}
	if u.AccessUntil != nil {
		t.Fatalf("until: got %v want nil", u.AccessUntil)
	}
}
