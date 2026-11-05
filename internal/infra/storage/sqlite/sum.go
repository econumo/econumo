package sqlite

import (
	"math/big"
	"strconv"
)

// FormatSum renders a SQLite SUM, which is a float, with 8 decimals. It rounds
// the float's SHORTEST decimal form (half away from zero) so summation noise
// goes: 358.34999999999127 -> 358.35000000. The binary value would not do: from
// 2^26 a float64 holds no exact cent, and 123456789.01 prints as
// 123456789.01000001 at 'f', 8.
//
// This narrows the gap to PostgreSQL's exact NUMERIC but does not close it: from
// 2^26 the sum of fractional amounts can land a step or more from the float
// nearest the true value, and that noise survives (see #408).
func FormatSum(v float64) string {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
	if !ok {
		// A SUM that overflowed to ±Inf (or NaN) has no decimal form.
		return strconv.FormatFloat(v, 'f', 8, 64)
	}
	return r.FloatString(8)
}
