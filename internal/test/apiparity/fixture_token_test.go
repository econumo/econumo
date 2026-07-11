package apiparity

import "testing"

// The seeded raw tokens must match the production token shape exactly
// (prefix + 43 url-safe chars) so the golden normalizer's redaction regex
// covers them the day one ever appears in a response body.
func TestSeededTokenShape(t *testing.T) {
	for _, tok := range []string{OwnerToken, GuestToken} {
		if !tokenRe.MatchString(tok) {
			t.Errorf("seeded token %q does not match %s", tok, tokenRe)
		}
	}
}
