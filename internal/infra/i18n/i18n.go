// Package i18n serves backend translations (emails, and the error
// message/errors strings on both HTTP edges) from the shared locales/
// catalogues; see httpx.WriteError and internal/web/mcp's MapErr.
package i18n

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/econumo/econumo/locales"
)

// Supported lists the catalogue languages; index 0 is the fallback.
var Supported = []string{"en", "ru"}

var (
	once     sync.Once
	catalogs map[string]map[string]string
)

func load() {
	catalogs = make(map[string]map[string]string, len(Supported))
	for _, lang := range Supported {
		raw, err := locales.FS.ReadFile(lang + ".json")
		if err != nil {
			panic(fmt.Sprintf("i18n: embedded catalogue %s.json: %v", lang, err))
		}
		var tree map[string]any
		if err := json.Unmarshal(raw, &tree); err != nil {
			panic(fmt.Sprintf("i18n: parse %s.json: %v", lang, err))
		}
		flat := map[string]string{}
		flatten("", tree, flat)
		catalogs[lang] = flat
	}
}

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
		}
	}
}

// T translates key into lang, interpolating {param} placeholders. Unknown
// languages and missing keys fall back to English; a key absent there too is
// returned verbatim so the failure is visible rather than silent.
func T(lang, key string, params map[string]any) string {
	val, ok := Lookup(lang, key, params)
	if !ok {
		return key
	}
	return val
}

// Lookup is T with an explicit miss signal: ok is false when the key exists in
// neither lang nor the English fallback, so callers that have their own
// literal text (the error renderers) can use it instead of surfacing the
// dotted key.
func Lookup(lang, key string, params map[string]any) (string, bool) {
	once.Do(load)
	val, ok := catalogs[lang][key]
	if !ok {
		val, ok = catalogs[Supported[0]][key]
	}
	if !ok {
		return "", false
	}
	for k, v := range params {
		val = strings.ReplaceAll(val, "{"+k+"}", fmt.Sprint(v))
	}
	return val, true
}
