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
	got, err := parseIcons("```json\n"+`{"categories":[{"key":" C1 ","icon":" home "},{"key":"c1","icon":"pets"},{"key":"","icon":"cake"},{"key":"c2","icon":null}]}`+"\n```", "categories")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["c1"] != "home" || got["c2"] != "" {
		t.Fatalf("icons = %v", got)
	}
	if got, err := parseIcons(`{"accounts":[]}`, "categories"); err != nil || len(got) != 0 {
		t.Fatalf("other key: %v %v", got, err)
	}
	for _, bad := range []string{"no json", `{"categories":{"c1":"home"}}`} {
		if _, err := parseIcons(bad, "categories"); err == nil {
			t.Fatalf("%s: expected an error", bad)
		}
	}
}
