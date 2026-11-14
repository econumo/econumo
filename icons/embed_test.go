package icons_test

import (
	"regexp"
	"testing"

	"github.com/econumo/econumo/icons"
)

func TestNames(t *testing.T) {
	if len(icons.Names) == 0 {
		t.Fatal("empty catalogue")
	}
	ligature := regexp.MustCompile(`^[a-z0-9_]+$`)
	seen := map[string]bool{}
	for _, n := range icons.Names {
		if !ligature.MatchString(n) {
			t.Errorf("%q is not a ligature name", n)
		}
		if seen[n] {
			t.Errorf("%q is listed twice", n)
		}
		seen[n] = true
	}
}
