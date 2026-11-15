package prompts

import (
	"io/fs"
	"strings"
	"testing"
)

func TestPromptFiles(t *testing.T) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 8 {
		t.Fatalf("want 8 prompt files, got %d", len(entries))
	}
	for _, e := range entries {
		if strings.TrimSpace(load(e.Name())) == "" {
			t.Errorf("%s is empty", e.Name())
		}
	}
}

func TestPlaceholdersSubstituted(t *testing.T) {
	for name, text := range map[string]string{
		"Suggest": Suggest, "Payee": Payee, "MatchText": MatchText, "Savings": Savings,
		"CategoryIcons": CategoryIcons, "AccountIcons": AccountIcons, "SuggestIcon": SuggestIcon,
	} {
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s is empty", name)
		}
		if strings.Contains(text, "{{") || strings.Contains(text, "}}") {
			t.Errorf("%s keeps a placeholder", name)
		}
	}
	if !strings.Contains(CategoryIcons, "Use only icons from this list: ") {
		t.Error("icon rules missing from CategoryIcons")
	}
}
