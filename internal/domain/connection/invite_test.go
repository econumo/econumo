package connection

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
)

func TestNewConnectionCode_ExactLength(t *testing.T) {
	c, err := NewConnectionCode("abc12")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Value() != "abc12" {
		t.Errorf("Value()=%q want abc12", c.Value())
	}
	if c.IsZero() {
		t.Error("a 5-char code must not be zero")
	}
}

func TestNewConnectionCode_WrongLength(t *testing.T) {
	for _, bad := range []string{"", "abcd", "abcdef", "a", "abcdefgh"} {
		t.Run(bad, func(t *testing.T) {
			_, err := NewConnectionCode(bad)
			if err == nil {
				t.Fatalf("expected error for %q", bad)
			}
			ve, ok := err.(*errs.ValidationError)
			if !ok {
				t.Fatalf("want *errs.ValidationError, got %T", err)
			}
			if ve.Error() != "ConnectionCode is incorrect" {
				t.Errorf("message=%q want %q", ve.Error(), "ConnectionCode is incorrect")
			}
		})
	}
}

func TestNewConnectionCode_CountsRunesNotBytes(t *testing.T) {
	// Five multibyte runes is a valid length even though len(bytes) > 5.
	c, err := NewConnectionCode("héllo")
	if err != nil {
		t.Fatalf("5 runes should be valid: %v", err)
	}
	if c.Value() != "héllo" {
		t.Errorf("Value()=%q", c.Value())
	}
}

func TestGenerateConnectionCode_Length5(t *testing.T) {
	// Run several times since case-randomization uses crypto/rand.
	for i := 0; i < 50; i++ {
		c := GenerateConnectionCode()
		if n := len([]rune(c.Value())); n != 5 {
			t.Fatalf("generated code %q has length %d want 5", c.Value(), n)
		}
		// A generated code is always re-acceptable by the validating constructor.
		if _, err := NewConnectionCode(c.Value()); err != nil {
			t.Fatalf("generated code %q rejected by NewConnectionCode: %v", c.Value(), err)
		}
		if c.IsZero() {
			t.Fatal("generated code reported IsZero")
		}
	}
}

func TestConnectionCode_ZeroValue(t *testing.T) {
	var c ConnectionCode
	if !c.IsZero() {
		t.Error("zero-value ConnectionCode should report IsZero")
	}
	if c.Value() != "" {
		t.Errorf("zero-value Value()=%q want empty", c.Value())
	}
}

func TestNewConnectionInvite_EmptyAndExpired(t *testing.T) {
	uid := mustID(t, "11111111-1111-1111-1111-111111111111")
	inv := NewConnectionInvite(uid)
	if !inv.UserId().Equal(uid) {
		t.Error("userId did not round-trip")
	}
	if !inv.Code().IsZero() {
		t.Error("fresh invite must have no code")
	}
	if inv.ExpiredAt() != nil {
		t.Error("fresh invite must have no expiry")
	}
	// No expiry is treated as expired.
	if !inv.IsExpired(time.Now()) {
		t.Error("invite with no expiry must be reported expired")
	}
}

func TestInviteFromState_RoundTrip(t *testing.T) {
	uid := mustID(t, "11111111-1111-1111-1111-111111111111")
	exp := time.Date(2024, 3, 1, 0, 5, 0, 0, time.UTC)

	withCode := InviteFromState(uid, "abc12", &exp)
	if withCode.Code().Value() != "abc12" {
		t.Errorf("code=%q want abc12", withCode.Code().Value())
	}
	if withCode.ExpiredAt() == nil || !withCode.ExpiredAt().Equal(exp) {
		t.Errorf("expiredAt=%v want %v", withCode.ExpiredAt(), exp)
	}

	// Empty code -> zero code value.
	cleared := InviteFromState(uid, "", nil)
	if !cleared.Code().IsZero() {
		t.Error("empty-code state should yield a zero code")
	}
	if cleared.ExpiredAt() != nil {
		t.Error("nil expiry should round-trip as nil")
	}
}

func TestConnectionInvite_GenerateNewCode(t *testing.T) {
	uid := mustID(t, "11111111-1111-1111-1111-111111111111")
	inv := NewConnectionInvite(uid)
	now := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	inv.GenerateNewCode(now)

	if inv.Code().IsZero() {
		t.Fatal("GenerateNewCode must set a code")
	}
	if n := len([]rune(inv.Code().Value())); n != 5 {
		t.Errorf("generated code length=%d want 5", n)
	}
	if inv.ExpiredAt() == nil {
		t.Fatal("GenerateNewCode must set an expiry")
	}
	// Expiry is now + 5 minutes.
	wantExp := now.Add(5 * time.Minute)
	if !inv.ExpiredAt().Equal(wantExp) {
		t.Errorf("expiredAt=%v want %v", inv.ExpiredAt(), wantExp)
	}
}

func TestConnectionInvite_ClearCode(t *testing.T) {
	uid := mustID(t, "11111111-1111-1111-1111-111111111111")
	inv := NewConnectionInvite(uid)
	inv.GenerateNewCode(time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC))
	inv.ClearCode()
	if !inv.Code().IsZero() {
		t.Error("ClearCode must zero the code")
	}
	if inv.ExpiredAt() != nil {
		t.Error("ClearCode must clear the expiry")
	}
}

func TestConnectionInvite_IsExpired_Boundary(t *testing.T) {
	uid := mustID(t, "11111111-1111-1111-1111-111111111111")
	gen := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	inv := NewConnectionInvite(uid)
	inv.GenerateNewCode(gen)
	exp := gen.Add(5 * time.Minute) // the expiry instant

	cases := []struct {
		name        string
		now         time.Time
		wantExpired bool
	}{
		{"well before expiry", gen.Add(time.Minute), false},
		{"one ns before expiry", exp.Add(-time.Nanosecond), false},
		{"exactly at expiry", exp, false}, // expiredAt.Before(now) is false at equality
		{"one ns after expiry", exp.Add(time.Nanosecond), true},
		{"well after expiry", exp.Add(time.Hour), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inv.IsExpired(tc.now); got != tc.wantExpired {
				t.Errorf("IsExpired(%v)=%v want %v", tc.now, got, tc.wantExpired)
			}
		})
	}
}
