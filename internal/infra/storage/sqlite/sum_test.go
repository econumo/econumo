package sqlite

import (
	"math"
	"testing"
)

func TestFormatSum(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		// Summation noise rounds away at the 8th decimal.
		{358.34999999999127, "358.35000000"},
		{1339.8899999999999, "1339.89000000"},
		{-1339.8899999999999, "-1339.89000000"},
		{23.3299999999, "23.33000000"},
		{4.3199999999, "4.32000000"},
		// A value already exact to 8 decimals is unchanged.
		{11101.11999998, "11101.11999998"},
		{0, "0.00000000"},
		{-12.5, "-12.50000000"},
		{50000, "50000.00000000"},
		// From 2^26 the binary value carries noise of its own into the 8th decimal.
		{123456789.01, "123456789.01000000"},
		{-123456789.01, "-123456789.01000000"},
		{67108864.07, "67108864.07000000"},
		{1876543210.46, "1876543210.46000000"},
		// Non-finite sums have no decimal form and keep the plain rendering.
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{math.NaN(), "NaN"},
	}
	for _, tc := range cases {
		if got := FormatSum(tc.in); got != tc.want {
			t.Errorf("FormatSum(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
