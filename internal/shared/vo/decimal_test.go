package vo

import "testing"

// TestNewDecimal_Normalize covers the frozen normalize behaviour for DB-sourced
// fixed-point strings: trailing-zero trimming in the fraction, leading-zero
// trimming in the integer part, sign handling, and the empty/zero collapse.
// These are the exact byte outputs the API emits.
func TestNewDecimal_Normalize(t *testing.T) {
	cases := []struct{ in, want string }{
		// trailing-zero trimming (the engine-divergence case: pgsql NUMERIC pads
		// to scale 8, sqlite affinity strips; both must render identically).
		{"0.92000000", "0.92"},
		{"0.92", "0.92"},
		{"95.00000000", "95"},
		{"1.00000000", "1"},
		{"1.50000000", "1.5"},
		// integer leading-zero trimming.
		{"007", "7"},
		{"00.5", "0.5"},
		{"0.50", "0.5"},
		// fraction longer than scale is truncated to 8 (no rounding).
		{"0.123456789", "0.12345678"},
		// empty / zero collapse.
		{"", "0"},
		{"0", "0"},
		{"0.00000000", "0"},
		{"0.0", "0"},
		// negative-zero PRESERVES the sign: "-0" stays "-0". Only the literal ""
		// and "0" inputs collapse to "0" (caught before sign handling). SQLite
		// SUM() emits "-0" for some netted-to-zero balances and the API surfaces
		// that, so this must match byte-for-byte.
		{"-0", "-0"},
		{"-0.00000000", "-0"},
		// negatives keep the sign.
		{"-12.34000000", "-12.34"},
		{"-0.50000000", "-0.5"},
		// plain integers pass through.
		{"100", "100"},
		{"0.123", "0.123"},
	}
	for _, c := range cases {
		if got := NewDecimal(c.in).String(); got != c.want {
			t.Errorf("NewDecimal(%q).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDecimal_ZeroValueString(t *testing.T) {
	var d DecimalNumber
	if got := d.String(); got != "0" {
		t.Errorf("zero-value DecimalNumber.String() = %q, want %q", got, "0")
	}
}

// TestDecimal_Sub covers exact scale-8 subtraction, the operation update-account
// uses to compute a balance correction (actual - requested). Results are
// normalized (trailing zeros trimmed).
func TestDecimal_Sub(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"100", "30", "70"},
		{"100.50000000", "0.50000000", "100"},
		{"0", "5", "-5"},
		{"5", "5", "0"},
		{"0.00000001", "0.00000002", "-0.00000001"}, // scale-8 precision
		{"13.07", "10", "3.07"},
		{"-5.5", "2.5", "-8"},
		{"1000000000.12345678", "0.00000078", "1000000000.123456"},
	}
	for _, c := range cases {
		got := NewDecimal(c.a).Sub(NewDecimal(c.b)).String()
		if got != c.want {
			t.Errorf("NewDecimal(%q).Sub(%q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestDecimal_IsZeroNegativeAbs(t *testing.T) {
	if !NewDecimal("0.00000000").IsZero() {
		t.Error("0.00000000 should be zero")
	}
	if NewDecimal("0.00000001").IsZero() {
		t.Error("0.00000001 should not be zero")
	}
	if !NewDecimal("-3.5").IsNegative() {
		t.Error("-3.5 should be negative")
	}
	if NewDecimal("3.5").IsNegative() {
		t.Error("3.5 should not be negative")
	}
	if got := NewDecimal("-12.34").Abs().String(); got != "12.34" {
		t.Errorf("abs(-12.34) = %q, want 12.34", got)
	}
	if !NewDecimal("1.5").Equals(NewDecimal("1.50000000")) {
		t.Error("1.5 should equal 1.50000000")
	}
}
