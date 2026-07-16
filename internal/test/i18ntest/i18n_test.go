// Package i18ntest guards the translation catalogues: key parity across
// languages, placeholder parity, frontend t() coverage, and (once wired)
// backend error-code and email-key coverage.
package i18ntest

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/econumo/econumo/locales"
)

// Wired to errs.AllCodes in the envelope-codes task and mailer.EmailKeys in
// the localized-email task.
var registeredCodes []string
var emailKeys []string

var languages = []string{"en", "ru"}

func flatten(prefix string, node map[string]any, out map[string]string) {
	for k, v := range node {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case string:
			out[key] = val
		case map[string]any:
			flatten(key, val, out)
		default:
			panic(fmt.Sprintf("catalogue value at %s is neither string nor object", key))
		}
	}
}

func catalogue(t *testing.T, lang string) map[string]string {
	t.Helper()
	raw, err := locales.FS.ReadFile(lang + ".json")
	if err != nil {
		t.Fatalf("read %s.json: %v", lang, err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("parse %s.json: %v", lang, err)
	}
	flat := map[string]string{}
	flatten("", tree, flat)
	return flat
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestKeyParity(t *testing.T) {
	ref := catalogue(t, "en")
	for _, lang := range languages[1:] {
		got := catalogue(t, lang)
		for _, k := range sortedKeys(ref) {
			if _, ok := got[k]; !ok {
				t.Errorf("%s.json missing key %s", lang, k)
			}
		}
		for _, k := range sortedKeys(got) {
			if _, ok := ref[k]; !ok {
				t.Errorf("%s.json has extra key %s (not in en.json)", lang, k)
			}
		}
	}
}

var placeholderRe = regexp.MustCompile(`\{[a-zA-Z0-9_]+\}`)

func placeholders(s string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, 4)
	for _, ph := range placeholderRe.FindAllString(s, -1) {
		if !seen[ph] {
			seen[ph] = true
			unique = append(unique, ph)
		}
	}
	sort.Strings(unique)
	return unique
}

func TestPlaceholderParity(t *testing.T) {
	ref := catalogue(t, "en")
	for _, lang := range languages[1:] {
		got := catalogue(t, lang)
		for _, k := range sortedKeys(ref) {
			val, ok := got[k]
			if !ok {
				continue // TestKeyParity reports it
			}
			want, have := placeholders(ref[k]), placeholders(val)
			if strings.Join(want, ",") != strings.Join(have, ",") {
				t.Errorf("%s.json key %s: placeholders %v != en %v", lang, k, have, want)
			}
		}
	}
}

// tCallRe matches t('some.key' — string-literal i18next lookups. Dynamic keys
// (t('errors.' + code)) don't match and are covered by the code-registry check.
var tCallRe = regexp.MustCompile(`[^a-zA-Z0-9_]t\(\s*'([a-z0-9_]+(?:\.[a-z0-9_]+)+)'`)

func TestFrontendKeysExist(t *testing.T) {
	ref := catalogue(t, "en")
	root := filepath.Join("..", "..", "..", "web", "src")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range tCallRe.FindAllStringSubmatch(string(src), -1) {
			if _, ok := ref[m[1]]; !ok {
				t.Errorf("%s: t('%s') has no entry in locales/en.json", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk web/src: %v", err)
	}
}

func TestBackendCodesCovered(t *testing.T) {
	for _, lang := range languages {
		cat := catalogue(t, lang)
		for _, code := range registeredCodes {
			if _, ok := cat["errors."+code]; !ok {
				t.Errorf("%s.json missing errors.%s (registered in errs.AllCodes)", lang, code)
			}
		}
		known := map[string]bool{}
		for _, code := range registeredCodes {
			known[code] = true
		}
		for _, k := range sortedKeys(cat) {
			if strings.HasPrefix(k, "errors.") && !known[strings.TrimPrefix(k, "errors.")] {
				t.Errorf("%s.json orphan key %s: no matching code in errs.AllCodes", lang, k)
			}
		}
		for _, key := range emailKeys {
			if _, ok := cat[key]; !ok {
				t.Errorf("%s.json missing email key %s", lang, key)
			}
		}
	}
}
