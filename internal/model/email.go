package model

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeEmail returns the form an address is stored, looked up, compared and rate
// limited by. It is computed in Go because the engines' lower() disagree: SQLite's folds ASCII only, PostgreSQL's depends on the
// database locale.
//
// A non-ASCII letter whose lowercase is ASCII keeps its form (U+0130 would become
// i, the Kelvin sign k): those spell a different mailbox than the ASCII letter does.
func NormalizeEmail(email string) string {
	return strings.Map(func(r rune) rune {
		lower := unicode.ToLower(r)
		if r >= utf8.RuneSelf && lower < utf8.RuneSelf {
			return r
		}
		return lower
	}, strings.TrimSpace(email))
}
