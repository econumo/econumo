package model

import (
	"regexp"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// OptionAffiliate is the referral code that brought the user to sign up. It is
// server-only: never returned by any browser-facing payload.
const OptionAffiliate = "affiliate"

var affiliatePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// NormalizeAffiliate returns raw when it is a well-formed code and "" otherwise:
// the code comes from an untrusted client and a bad one must never fail signup.
func NormalizeAffiliate(raw string) string {
	if affiliatePattern.MatchString(raw) {
		return raw
	}
	return ""
}

// IsServerOnlyOption reports whether an option is withheld from every
// browser-facing payload.
func IsServerOnlyOption(name string) bool { return name == OptionAffiliate }

// SetAffiliate records the referral code once; the first attribution wins, so
// an existing option or an empty value changes nothing.
func (u *User) SetAffiliate(value string, id vo.Id, now time.Time) {
	if value == "" || u.Option(OptionAffiliate) != nil {
		return
	}
	u.Options = append(u.Options, NewUserOption(id, OptionAffiliate, &value, now))
}

// Affiliate returns the stored referral code, or "" when there is none.
func (u *User) Affiliate() string {
	if o := u.Option(OptionAffiliate); o != nil && o.Value != nil {
		return *o.Value
	}
	return ""
}
