package user_test

import (
	"strings"
	"testing"

	appuser "github.com/econumo/econumo/internal/user"
)

func TestJoinAvatar(t *testing.T) {
	if got := appuser.JoinAvatar("face", "fuchsia"); got != "face:fuchsia" {
		t.Fatalf("JoinAvatar = %q, want face:fuchsia", got)
	}
}

func TestDefaultAvatarIsValid(t *testing.T) {
	icon, color, ok := strings.Cut(appuser.DefaultAvatar, ":")
	if !ok || !appuser.IsValidAvatarIcon(icon) || !appuser.IsValidAvatarColor(color) {
		t.Fatalf("DefaultAvatar %q is not a valid icon:color", appuser.DefaultAvatar)
	}
}

func TestIsValidAvatarIcon(t *testing.T) {
	valid := []string{"face", "account_circle", "gamepad_2", "a"}
	for _, v := range valid {
		if !appuser.IsValidAvatarIcon(v) {
			t.Errorf("IsValidAvatarIcon(%q) = false, want true", v)
		}
	}
	invalid := []string{"", "Face", "with space", "semi:colon", "dash-name", strings.Repeat("a", 65)}
	for _, v := range invalid {
		if appuser.IsValidAvatarIcon(v) {
			t.Errorf("IsValidAvatarIcon(%q) = true, want false", v)
		}
	}
}

func TestIsValidAvatarColor(t *testing.T) {
	if len(appuser.AvatarColors) != 7 {
		t.Fatalf("AvatarColors len = %d, want 7", len(appuser.AvatarColors))
	}
	for _, c := range appuser.AvatarColors {
		if !appuser.IsValidAvatarColor(c) {
			t.Errorf("IsValidAvatarColor(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"", "magenta", "FUCHSIA", "neon", "purple", "blue"} {
		if appuser.IsValidAvatarColor(c) {
			t.Errorf("IsValidAvatarColor(%q) = true, want false", c)
		}
	}
}

func TestRandomAvatarPickerAlwaysValid(t *testing.T) {
	p := appuser.NewRandomAvatarPicker()
	seen := map[string]bool{}
	for range 500 {
		v := p.Pick()
		seen[v] = true
		icon, color, ok := strings.Cut(v, ":")
		if !ok || !appuser.IsValidAvatarIcon(icon) || !appuser.IsValidAvatarColor(color) {
			t.Fatalf("Pick() = %q, not a valid icon:color", v)
		}
	}
	if len(seen) < 10 {
		t.Errorf("Pick() produced only %d distinct values in 500 draws — not random", len(seen))
	}
}

func TestFixedAvatarPicker(t *testing.T) {
	if got := appuser.FixedAvatarPicker("pets:teal").Pick(); got != "pets:teal" {
		t.Fatalf("FixedAvatarPicker.Pick() = %q, want pets:teal", got)
	}
}
