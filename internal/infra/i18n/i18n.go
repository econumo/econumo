// Package i18n serves backend translations (emails, and any future
// server-rendered text) from the shared locales/ catalogues. The REST API does
// NOT translate error messages here — they stay frozen English with additive
// codes, rendered by the SPA. The MCP edge is the exception: its clients are
// LLMs with no catalogue, so internal/web/mcp renders error messages in the
// caller's language via T (see MapErr).
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
	once.Do(load)
	val, ok := catalogs[lang][key]
	if !ok {
		val, ok = catalogs[Supported[0]][key]
	}
	if !ok {
		return key
	}
	for k, v := range params {
		val = strings.ReplaceAll(val, "{"+k+"}", fmt.Sprint(v))
	}
	return val
}
