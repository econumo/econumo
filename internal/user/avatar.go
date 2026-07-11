// Avatar helpers: the "<icon>:<color>" avatar value format, the color
// allowlist, and the random default picker used at registration.

package user

import (
	"math/rand/v2"
	"regexp"
)

// DefaultAvatar is the standard value existing rows were backfilled to by the
// 20260710000000 migration; test harnesses also pin it as the deterministic
// registration default.
const DefaultAvatar = "diamond:sky"

// AvatarColors is the canonical accent-color allowlist. The frontend
// mirrors it (web/src/lib/avatars.ts) and a sync test asserts exact equality,
// so order and spelling are contract.
var AvatarColors = []string{
	"red", "orange", "amber", "emerald", "teal", "sky", "fuchsia",
}

// RandomAvatarIcons is the pool random registration defaults draw from — the
// avatar picker page, mirrored from the frontend (web/src/lib/avatars.ts
// avatarIcons; a sync test asserts exact equality, so names and order are
// contract). update-avatar itself accepts any well-formed name.
var RandomAvatarIcons = []string{
	"face", "sentiment_very_satisfied", "sentiment_very_dissatisfied", "sick",
	"psychology", "smart_toy",
	"owl", "cruelty_free", "flutter_dash", "emoji_nature", "bug_report",
	"savings", "egg",
	"auto_awesome", "all_inclusive", "extension", "toys", "gesture", "hive",
	"blur_on",
	"rocket_launch", "planet", "cloud", "bolt", "wb_sunny", "nightlight",
	"ac_unit", "water_drop", "local_fire_department", "star", "favorite",
	"diamond", "crown", "chess_knight", "motorcycle", "swords",
}

var avatarIconRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// IsValidAvatarIcon checks the Material-ligature name format only; the icon
// universe is owned by the frontend (same precedent as category icons).
func IsValidAvatarIcon(icon string) bool { return avatarIconRe.MatchString(icon) }

func IsValidAvatarColor(color string) bool {
	for _, c := range AvatarColors {
		if c == color {
			return true
		}
	}
	return false
}

func JoinAvatar(icon, color string) string { return icon + ":" + color }

// RandomAvatarPicker is the production AvatarPicker: a uniform random
// icon+color for each new user.
type RandomAvatarPicker struct{}

func NewRandomAvatarPicker() RandomAvatarPicker { return RandomAvatarPicker{} }

func (RandomAvatarPicker) Pick() string {
	return JoinAvatar(
		RandomAvatarIcons[rand.IntN(len(RandomAvatarIcons))],
		AvatarColors[rand.IntN(len(AvatarColors))],
	)
}

// FixedAvatarPicker always returns its literal value; test harnesses use it so
// golden responses stay deterministic.
type FixedAvatarPicker string

func (p FixedAvatarPicker) Pick() string { return string(p) }
