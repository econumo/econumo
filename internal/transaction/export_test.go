package transaction

import "testing"

// TestSanitizeExportValue_DefusesFormulaInjection locks the CSV formula-injection
// defense: a free-text export cell a spreadsheet would evaluate as a formula
// (leading =, +, -, @) is prefixed with a single quote so it renders as text.
func TestSanitizeExportValue_DefusesFormulaInjection(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`=HYPERLINK("https://evil.tld")`, `'=HYPERLINK("https://evil.tld")`},
		{"=1+1", "'=1+1"},
		{"+cmd", "'+cmd"},
		{"-2+3", "'-2+3"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"Groceries", "Groceries"}, // ordinary value untouched
		{"Café — dinner", "Café — dinner"},
		{"", ""},
		{"  =evil  ", "'=evil"}, // trimmed, then defused
	}
	for _, c := range cases {
		if got := sanitizeExportValue(c.in); got != c.want {
			t.Errorf("sanitizeExportValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
