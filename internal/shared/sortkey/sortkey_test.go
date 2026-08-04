package sortkey

import (
	"sort"
	"testing"
)

// TestBetween_EmptyList pins the documented seed for an empty list.
func TestBetween_EmptyList(t *testing.T) {
	got, err := Between("", "")
	if err != nil {
		t.Fatalf("Between(\"\",\"\"): %v", err)
	}
	if got != "a0" {
		t.Fatalf("Between(\"\",\"\") = %q, want \"a0\"", got)
	}
}

// TestBetween_AppendIncrementsInteger confirms appending walks the integer part
// instead of bisecting toward +infinity, which is what keeps keys short.
func TestBetween_AppendIncrementsInteger(t *testing.T) {
	// Magnitude 'b' carries two digits and spans b00-bzz, so 'az' rolls over to
	// 'b00' (not 'b10' -- there is no leading-digit-nonzero rule).
	cases := []struct{ prev, want Key }{
		{"a0", "a1"},
		{"a1", "a2"},
		{"az", "b00"},
		{"bzz", "c000"},
		{"c000", "c001"},
	}
	for _, c := range cases {
		got, err := Between(c.prev, "")
		if err != nil {
			t.Fatalf("Between(%q,\"\"): %v", c.prev, err)
		}
		if got != c.want {
			t.Errorf("Between(%q,\"\") = %q, want %q", c.prev, got, c.want)
		}
	}
}

// TestBetween_PrependDecrementsInteger covers the inverted uppercase magnitudes
// the seed policy deliberately keeps off the common path.
func TestBetween_PrependDecrementsInteger(t *testing.T) {
	cases := []struct{ next, want Key }{
		{"a1", "a0"},
		{"c000", "bzz"},
		{"b00", "az"},
		{"a0", "Zz"},
	}
	for _, c := range cases {
		got, err := Between("", c.next)
		if err != nil {
			t.Fatalf("Between(\"\",%q): %v", c.next, err)
		}
		if got != c.want {
			t.Errorf("Between(\"\",%q) = %q, want %q", c.next, got, c.want)
		}
	}
}

// TestBetween_MidpointAppendsFractionalDigit is the no-exhaustion property: a
// midpoint always exists between adjacent keys, so nothing ever needs
// renumbering.
func TestBetween_MidpointAppendsFractionalDigit(t *testing.T) {
	got, err := Between("c000", "c001")
	if err != nil {
		t.Fatalf("Between: %v", err)
	}
	if !(got > "c000" && got < "c001") {
		t.Fatalf("Between(\"c000\",\"c001\") = %q, want strictly between", got)
	}
}

// TestBetween_RepeatedSameSpotNeverExhausts hammers the one insertion point that
// would break a numeric scheme.
func TestBetween_RepeatedSameSpotNeverExhausts(t *testing.T) {
	lo, hi := Key("a0"), Key("a1")
	for i := 0; i < 200; i++ {
		mid, err := Between(lo, hi)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if !(mid > lo && mid < hi) {
			t.Fatalf("iteration %d: %q not strictly between %q and %q", i, mid, lo, hi)
		}
		hi = mid
	}
}

// TestBetween_AppendKeyLengthStaysBounded is the property the magnitude prefix
// exists to provide. Naive bisection toward +infinity passes every ordering
// test above while failing this one.
func TestBetween_AppendKeyLengthStaysBounded(t *testing.T) {
	k := Seed(GrowsUp)
	for i := 0; i < 1000; i++ {
		next, err := Between(k, "")
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if next <= k {
			t.Fatalf("append %d: %q does not sort after %q", i, next, k)
		}
		k = next
	}
	if len(k) > 4 {
		t.Fatalf("after 1000 appends key = %q (len %d), want <= 4", k, len(k))
	}
}

// TestBetween_SequenceSortsByteWise confirms lexicographic byte order matches
// insertion order, which is what the SQL ORDER BY relies on.
func TestBetween_SequenceSortsByteWise(t *testing.T) {
	var keys []string
	k := Seed(GrowsUp)
	keys = append(keys, string(k))
	for i := 0; i < 50; i++ {
		var err error
		k, err = Between(k, "")
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		keys = append(keys, string(k))
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for i := range keys {
		if keys[i] != sorted[i] {
			t.Fatalf("byte order diverges at %d: %q vs %q", i, keys[i], sorted[i])
		}
	}
}

// TestBetween_PrependSequenceSortsByteWise is the mirror property for the
// direction budget folders and envelopes grow in.
func TestBetween_PrependSequenceSortsByteWise(t *testing.T) {
	var keys []string
	k := Seed(GrowsDown)
	keys = append(keys, string(k))
	for i := 0; i < 50; i++ {
		var err error
		k, err = Between("", k)
		if err != nil {
			t.Fatalf("prepend %d: %v", i, err)
		}
		keys = append(keys, string(k))
	}
	// Keys were produced newest-first, so the reverse is ascending.
	for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
		keys[i], keys[j] = keys[j], keys[i]
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for i := range keys {
		if keys[i] != sorted[i] {
			t.Fatalf("byte order diverges at %d: %q vs %q", i, keys[i], sorted[i])
		}
	}
}

// TestBetween_PrependFromSeedStaysOutOfUppercase pins the seed policy's payoff.
// c000 decrements through magnitude 'b' (62*62 = 3844 keys) and then magnitude
// 'a' (62 keys) before reaching the inverted uppercase magnitudes: 3906
// prepends of headroom. Budget folders and envelopes, the only prepend-oriented
// lists, number in the tens, so that branch stays rare in practice -- but it is
// exercised by TestBetween_PrependDecrementsInteger rather than left untested.
func TestBetween_PrependFromSeedStaysOutOfUppercase(t *testing.T) {
	const wantHeadroom = 3906
	k := Seed(GrowsDown)
	n := 0
	for {
		var err error
		k, err = Between("", k)
		if err != nil {
			t.Fatalf("prepend %d: %v", n, err)
		}
		if k[0] >= 'A' && k[0] <= 'Z' {
			break
		}
		n++
		if n > wantHeadroom*2 {
			t.Fatalf("no uppercase magnitude after %d prepends", n)
		}
	}
	if n != wantHeadroom {
		t.Fatalf("headroom = %d prepends, want %d", n, wantHeadroom)
	}
}

// TestBetween_RejectsInverted guards the caller contract.
func TestBetween_RejectsInverted(t *testing.T) {
	if _, err := Between("a2", "a1"); err == nil {
		t.Fatal("Between with prev >= next must error")
	}
	if _, err := Between("a1", "a1"); err == nil {
		t.Fatal("Between with prev == next must error")
	}
}

// TestSeed pins the two growth-direction seeds.
func TestSeed(t *testing.T) {
	if got := Seed(GrowsUp); got != "a0" {
		t.Errorf("Seed(GrowsUp) = %q, want \"a0\"", got)
	}
	if got := Seed(GrowsDown); got != "c000" {
		t.Errorf("Seed(GrowsDown) = %q, want \"c000\"", got)
	}
}

// TestValidate rejects keys that would corrupt ordering.
func TestValidate(t *testing.T) {
	for _, bad := range []Key{"", "0", "a", "a!", "a0 ", "!0", "c00"} {
		if err := Validate(bad); err == nil {
			t.Errorf("Validate(%q) = nil, want error", bad)
		}
	}
	for _, ok := range []Key{"a0", "az", "b10", "c000", "Zz", "c000V"} {
		if err := Validate(ok); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", ok, err)
		}
	}
}

// TestValidate_RejectsTrailingZeroFraction: two strings must never denote the
// same position, or a later midpoint can land on an existing key.
func TestValidate_RejectsTrailingZeroFraction(t *testing.T) {
	if err := Validate("a0V0"); err == nil {
		t.Error("Validate(\"a0V0\") = nil, want error (trailing zero digit)")
	}
}
