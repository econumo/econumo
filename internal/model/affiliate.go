package model

import (
	"regexp"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// OptionAffiliate is the partner program that brought the user to sign up and
// OptionAffiliateCode the referral code within it. Both are server-only: never
// returned by any browser-facing payload.
const (
	OptionAffiliate     = "affiliate"
	OptionAffiliateCode = "affiliate_code"
)

var (
	affiliateCodePattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	affiliateProgramPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
)

// AffiliateCookie maps a partner program to the cookie it sets on the root domain.
type AffiliateCookie struct {
	Program string
	Cookie  string
}

// ValidAffiliateProgram reports whether s is a well-formed program name.
func ValidAffiliateProgram(s string) bool { return affiliateProgramPattern.MatchString(s) }

// NormalizeAffiliateCode returns raw when it is a well-formed code and "" otherwise:
// the code comes from an untrusted client and a bad one must never fail signup.
func NormalizeAffiliateCode(raw string) string {
	if affiliateCodePattern.MatchString(raw) {
		return raw
	}
	return ""
}

// IsServerOnlyOption reports whether an option is withheld from every
// browser-facing payload.
func IsServerOnlyOption(name string) bool {
	return name == OptionAffiliate || name == OptionAffiliateCode
}

// SetAffiliate records the program and code together, once; the first
// attribution wins, so an empty value or either option already present
// changes nothing.
func (u *User) SetAffiliate(program, code string, newID func() vo.Id, now time.Time) {
	if program == "" || code == "" || u.Option(OptionAffiliate) != nil || u.Option(OptionAffiliateCode) != nil {
		return
	}
	u.Options = append(u.Options,
		NewUserOption(newID(), OptionAffiliate, &program, now),
		NewUserOption(newID(), OptionAffiliateCode, &code, now))
}

// Affiliate returns the stored program, or "" when there is none.
func (u *User) Affiliate() string { return u.optionValue(OptionAffiliate) }

// AffiliateCode returns the stored referral code, or "" when there is none.
func (u *User) AffiliateCode() string { return u.optionValue(OptionAffiliateCode) }

func (u *User) optionValue(name string) string {
	if o := u.Option(name); o != nil && o.Value != nil {
		return *o.Value
	}
	return ""
}
