package locales

import (
	"encoding/json"
	"testing"
)

func TestCataloguesEmbedAndParse(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		raw, err := FS.ReadFile(lang + ".json")
		if err != nil {
			t.Fatalf("read %s.json: %v", lang, err)
		}
		var tree map[string]any
		if err := json.Unmarshal(raw, &tree); err != nil {
			t.Fatalf("parse %s.json: %v", lang, err)
		}
		if len(tree) == 0 {
			t.Fatalf("%s.json is empty", lang)
		}
	}
}
