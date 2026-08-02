package i18ntest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// A language reaches the SPA through three lists that nothing else ties
// together: i18next resources, the locale picker, and the calendar locale.
// Missing the last one fails silently — captions and weekday headers just stay
// English — so these are checked against i18n.Supported rather than left to
// review.
var (
	localeOptionRe = regexp.MustCompile(`\{\s*value:\s*'([a-z]{2})'`)
	i18nResourceRe = regexp.MustCompile(`(?m)^\s+([a-z]{2}):\s*\{\s*translation:`)
	calendarMapRe  = regexp.MustCompile(`const locales: Record<string, Locale> = \{([^}]*)\}`)
	calendarKeyRe  = regexp.MustCompile(`([a-zA-Z]+)\s*(?::|,|$)`)
)

func webSource(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", rel))
	if err != nil {
		t.Fatalf("read web/src/%s: %v", rel, err)
	}
	return string(raw)
}

func captures(re *regexp.Regexp, src string) map[string]bool {
	found := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		found[m[1]] = true
	}
	return found
}

func assertCovers(t *testing.T, what string, got map[string]bool, want []string) {
	t.Helper()
	for _, lang := range want {
		if !got[lang] {
			t.Errorf("%s is missing %q (listed in i18n.Supported)", what, lang)
		}
	}
	extra := make([]string, 0, len(got))
	for lang := range got {
		known := false
		for _, s := range want {
			if s == lang {
				known = true
				break
			}
		}
		if !known {
			extra = append(extra, lang)
		}
	}
	sort.Strings(extra)
	for _, lang := range extra {
		t.Errorf("%s lists %q, which is not in i18n.Supported", what, lang)
	}
}

func TestFrontendLocaleOptionsMatchSupported(t *testing.T) {
	src := webSource(t, filepath.Join("lib", "config.ts"))
	start := regexp.MustCompile(`export function getLocaleOptions\(\)[^{]*\{`).FindStringIndex(src)
	if start == nil {
		t.Fatal("getLocaleOptions() not found in web/src/lib/config.ts")
	}
	assertCovers(t, "getLocaleOptions()", captures(localeOptionRe, src[start[1]:]), languages)
}

func TestFrontendI18nResourcesMatchSupported(t *testing.T) {
	src := webSource(t, filepath.Join("app", "i18n.ts"))
	assertCovers(t, "i18next resources", captures(i18nResourceRe, src), languages)
}

func TestCalendarLocaleCoversSupported(t *testing.T) {
	src := webSource(t, filepath.Join("lib", "calendarLocale.ts"))
	body := calendarMapRe.FindStringSubmatch(src)
	if body == nil {
		t.Fatal("the locales map was not found in web/src/lib/calendarLocale.ts")
	}
	mapped := captures(calendarKeyRe, body[1])
	// English is the enUS fallback and is deliberately absent from the map.
	for _, lang := range languages[1:] {
		if !mapped[lang] {
			t.Errorf("calendarLocale has no entry for %q, so its calendar silently stays English", lang)
		}
	}
}
