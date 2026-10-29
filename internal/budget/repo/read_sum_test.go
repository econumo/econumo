package repo

import (
	"math"
	"testing"
)

func TestSqliteSum(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1339.8899999999999, "1339.89000000"},
		{123456789.01, "123456789.01000000"},
		{-1339.8899999999999, "-1339.89000000"},
		{0, "0.00000000"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{math.NaN(), "NaN"},
	}
	for _, c := range cases {
		if got := sqliteSum(c.in); got != c.want {
			t.Errorf("sqliteSum(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
