package vo

import "testing"

// TestNormalizeScientificNotation locks Go's expansion of scientific-notation
// input to match PHP DecimalNumber::normalize byte-for-byte. The expected
// values were verified against the live PHP DecimalNumber (bcmath scale 8) for
// rates as stored in the database (e.g. "2.586E-5"). Regression guard for the
// api-compare finding where Go emitted "1.2e-05" instead of "0.000012".
func TestNormalizeScientificNotation(t *testing.T) {
	cases := map[string]string{
		"1.2e-05":  "0.000012",
		"2.586E-5": "0.00002586",
		"2.285E-5": "0.00002285",
		"3.415E-5": "0.00003415",
		"4.636E-5": "0.00004636",
		"1.5e3":    "1500",
		"-2.5e-4":  "-0.00025",
		"1e-8":     "0.00000001",
		"1e-9":     "0", // below scale 8 -> truncates to 0 (bcdiv scale 8)
	}
	for in, want := range cases {
		if got := NewDecimal(in).String(); got != want {
			t.Errorf("NewDecimal(%q).String() = %q, want %q", in, got, want)
		}
	}
}
