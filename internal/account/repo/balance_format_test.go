package repo

import "testing"

// TestFormatSQLiteBalance locks the SQLite float-balance rendering to the frozen
// wire format: a float rounded to 8 decimals via "%.8f". The inputs are the exact
// doubles SQLite's SUM produces for the seed DB; e.g. the summed balance
// 358.34999999999127 must render "358.35000000" (which vo.DecimalNumber then
// normalizes to "358.35"), NOT the full-precision "358.34999999999127".
func TestFormatSQLiteBalance(t *testing.T) {
	cases := map[float64]string{
		// Float-drift sums round at the 8th decimal to a clean value:
		358.34999999999127: "358.35000000",
		23.3299999999:      "23.33000000",
		4.3199999999:       "4.32000000",
		// A value already exact to 8 decimals is unchanged:
		11101.11999998: "11101.11999998",
		0:              "0.00000000",
		-12.5:          "-12.50000000",
		50000:          "50000.00000000",
	}
	for in, want := range cases {
		if got := formatSQLiteBalance(in); got != want {
			t.Errorf("formatSQLiteBalance(%v) = %q, want %q", in, got, want)
		}
	}
}
