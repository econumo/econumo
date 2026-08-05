package compat

import (
	"regexp"
	"testing"
)

// The init parse already panics on malformed data; this test pins the
// contract explicitly so a bad edit fails with a readable message instead of
// an init panic somewhere down the import graph.
func TestVersionsAreStrictSemver(t *testing.T) {
	semver := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	for name, v := range map[string]string{
		"minAppVersion":    MinAppVersion,
		"minServerVersion": MinServerVersion,
	} {
		if !semver.MatchString(v) {
			t.Errorf("%s = %q, want strict vX.Y.Z", name, v)
		}
	}
}
