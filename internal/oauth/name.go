package oauth

import (
	"strings"
	"unicode/utf8"
)

const (
	nameMinRunes = 3
	nameMaxRunes = 20 // the register/update-name rule (model.RegisterRequest.Validate)
	fallbackName = "User"
)

// deriveName turns a provider's display name into one the name rule accepts:
// the claim, else the email local part, else a constant; clamped to 20 runes.
func deriveName(claim, email string) string {
	pick := strings.TrimSpace(claim)
	if utf8.RuneCountInString(pick) < nameMinRunes {
		local := email
		if at := strings.Index(email, "@"); at > 0 {
			local = email[:at]
		}
		pick = strings.TrimSpace(local)
	}
	if utf8.RuneCountInString(pick) < nameMinRunes {
		pick = fallbackName
	}
	if r := []rune(pick); len(r) > nameMaxRunes {
		pick = strings.TrimSpace(string(r[:nameMaxRunes]))
	}
	return pick
}
