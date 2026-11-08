package i18ntest

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The onboarding defaults are sent as-is to create-category and create-budget,
// which both refuse names outside 3-64 runes (category newCategoryName,
// model.ValidateName); a default outside that range blocks the step.
func TestOnboardingDefaultNamesFitServerLimits(t *testing.T) {
	for _, lang := range languages {
		cat := catalogue(t, lang)
		for _, key := range sortedKeys(cat) {
			if !strings.HasPrefix(key, "onboarding.categories.") && key != "onboarding.budget_name" {
				continue
			}
			if n := utf8.RuneCountInString(strings.TrimSpace(cat[key])); n < 3 || n > 64 {
				t.Errorf("%s: %s = %q has %d characters, want 3-64", lang, key, cat[key], n)
			}
		}
	}
}
