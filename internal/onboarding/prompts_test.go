package onboarding

import (
	"strings"
	"testing"
)

func TestReplySchemasFitStructuredOutputs(t *testing.T) {
	for _, s := range []string{string(categoryIconsSchema.JSON), string(accountIconsSchema.JSON)} {
		if !strings.Contains(s, `"enum":["home",`) {
			t.Fatalf("icon schema has no catalogue enum: %.120s", s)
		}
	}
	for _, s := range []interface{ Check() error }{*savingsSchema, *categoryIconsSchema, *accountIconsSchema} {
		if err := s.Check(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseIcons(t *testing.T) {
	got, err := parseIcons("```json\n"+`{"categories":[{"id":" ABC ","icon":" home "},{"id":"abc","icon":"pets"},{"id":"","icon":"cake"},{"id":"d","icon":null}]}`+"\n```", "categories")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["abc"] != "home" || got["d"] != "" {
		t.Fatalf("icons = %v", got)
	}
	if got, err := parseIcons(`{"accounts":[]}`, "categories"); err != nil || len(got) != 0 {
		t.Fatalf("other key: %v %v", got, err)
	}
	for _, bad := range []string{"no json", `{"categories":{"abc":"home"}}`} {
		if _, err := parseIcons(bad, "categories"); err == nil {
			t.Fatalf("%s: expected an error", bad)
		}
	}
}
