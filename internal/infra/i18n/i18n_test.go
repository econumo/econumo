package i18n

import "testing"

func TestTranslatesKnownKey(t *testing.T) {
	got := T("ru", "errors.common.is_blank", nil)
	if got == "" || got == "This value should not be blank." || got == "errors.common.is_blank" {
		t.Fatalf("expected Russian translation, got %q", got)
	}
}

func TestInterpolatesParams(t *testing.T) {
	got := T("en", "errors.common.is_blank", nil)
	if got != "This value should not be blank." {
		t.Fatalf("en lookup = %q", got)
	}
}

func TestUnknownLanguageFallsBackToEnglish(t *testing.T) {
	if got := T("xx", "errors.common.is_blank", nil); got != "This value should not be blank." {
		t.Fatalf("fallback = %q", got)
	}
}

func TestUnknownKeyReturnsKey(t *testing.T) {
	if got := T("en", "no.such.key", nil); got != "no.such.key" {
		t.Fatalf("missing-key = %q", got)
	}
}

func TestLookupSignalsMiss(t *testing.T) {
	if got, ok := Lookup("ru", "errors.common.is_blank", nil); !ok || got == "" {
		t.Fatalf("known key: got %q ok=%v", got, ok)
	}
	if got, ok := Lookup("xx", "errors.common.is_blank", nil); !ok || got != "This value should not be blank." {
		t.Fatalf("unknown language must fall back to English: got %q ok=%v", got, ok)
	}
	if _, ok := Lookup("en", "no.such.key", nil); ok {
		t.Fatal("missing key must report ok=false")
	}
}
