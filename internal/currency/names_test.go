package currency

import "testing"

func TestDisplayName_KnownCode(t *testing.T) {
	cases := map[string]string{
		"USD": "US Dollar",
		"EUR": "Euro",
		"AED": "United Arab Emirates Dirham",
	}
	for code, want := range cases {
		if got := DisplayName(code); got != want {
			t.Errorf("DisplayName(%q)=%q want %q", code, got, want)
		}
	}
}

func TestDisplayName_UnknownFallsBackToCode(t *testing.T) {
	// An unknown code returns the code itself.
	for _, code := range []string{"ZZZ", "", "not-a-code"} {
		if got := DisplayName(code); got != code {
			t.Errorf("DisplayName(%q)=%q want fallback to the code", code, got)
		}
	}
}
