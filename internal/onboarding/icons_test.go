package onboarding_test

import (
	"testing"
	"unicode/utf8"

	"github.com/econumo/econumo/internal/infra/i18n"
)

// The archive folder is created through folder create, which needs 3-64 characters.
func TestArchiveFolderName_IsAValidFolderNameInEveryLanguage(t *testing.T) {
	for _, lang := range i18n.Supported {
		name := i18n.T(lang, "onboarding.archive_folder", nil)
		if n := utf8.RuneCountInString(name); n < 3 || n > 64 || name == "onboarding.archive_folder" {
			t.Fatalf("%s: %q", lang, name)
		}
	}
}
