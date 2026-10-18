package model

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAppendNote(t *testing.T) {
	if got := AppendNote("", "note", 255); got != "note" {
		t.Errorf("empty description: %q", got)
	}
	if got := AppendNote("lunch  \n", "note", 255); got != "lunch\nnote" {
		t.Errorf("appended on its own line: %q", got)
	}
	long := strings.Repeat("я", 300)
	got := AppendNote(long, "Category removed.", 255)
	if utf8.RuneCountInString(got) != 255 || !strings.HasSuffix(got, "…\nCategory removed.") {
		t.Errorf("a long description is shortened, the note kept whole: %d %q", utf8.RuneCountInString(got), got[len(got)-30:])
	}
}
