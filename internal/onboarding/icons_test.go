package onboarding_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/onboarding"
)

// The AI may only pick icons the app's pickers offer.
func TestIcons_MatchTheSPACatalogue(t *testing.T) {
	src, err := os.ReadFile("../../web/src/lib/icons.ts")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	body = body[strings.Index(body, "export const icons = [") : strings.Index(body, "]")+1]
	var want []string
	for _, m := range regexp.MustCompile(`'([a-z0-9_]+)'`).FindAllStringSubmatch(body, -1) {
		if !slices.Contains(want, m[1]) {
			want = append(want, m[1])
		}
	}
	if !slices.Equal(onboarding.Icons, want) {
		t.Fatalf("internal/onboarding/icons.go is out of sync with web/src/lib/icons.ts: %d vs %d icons", len(onboarding.Icons), len(want))
	}
}

// The archive folder is created through folder create, which needs 3-64 characters.
func TestArchiveFolderName_IsAValidFolderNameInEveryLanguage(t *testing.T) {
	for _, lang := range i18n.Supported {
		name := i18n.T(lang, "onboarding.archive_folder", nil)
		if n := utf8.RuneCountInString(name); n < 3 || n > 64 || name == "onboarding.archive_folder" {
			t.Fatalf("%s: %q", lang, name)
		}
	}
}
