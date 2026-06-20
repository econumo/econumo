package vo

import (
	"math/big"
	"strings"
)

// DecimalNumber is the wire representation of a money/rate value. It mirrors the
// PHP DecimalNumber value object's *normalized string form* — the exact output
// of its getValue()/__toString() — so values serialize byte-identically to the
// existing API regardless of how the database stored them.
//
// Scope: this type currently covers NORMALIZATION ONLY (the form in which a
// stored NUMERIC(19,8) is rendered on the wire). PHP normalize() trims trailing
// zeros in the fraction and leading zeros in the integer part, so e.g. the
// stored "0.92000000" and "0.92" both render as "0.92" — which is what the API
// emits. The SQLite engine already strips trailing zeros via NUMERIC affinity,
// but PostgreSQL returns the padded "0.92000000"; normalizing here makes both
// engines byte-identical to PHP.
//
// Arithmetic (Add/Sub/Mul/Div/Round at bcmath scale 8) IS implemented, backed by
// math/big in exact integer space scaled by 10^8 (no shopspring needed). Mul/Div
// TRUNCATE toward zero at scale 8 (bcmul/bcdiv semantics); Round uses PHP
// round()'s half-away-from-zero. Golden vectors against PHP bcmath live in
// decimal_test.go + testdata. DecimalNumber is the flagged silent-drift hotspot
// (see COMPATIBILITY.md and the rewrite plan).
type DecimalNumber struct {
	value string // already normalized
}

// scale is PHP DecimalNumber::SCALE (bcmath scale).
const decimalScale = 8

// NewDecimal builds a DecimalNumber from a raw numeric string (as read from the
// DB), normalizing it to the PHP getValue() form. A non-numeric or empty input
// normalizes to "0", matching PHP's handling (empty/"0" -> "0").
func NewDecimal(raw string) DecimalNumber {
	return DecimalNumber{value: normalizeDecimal(raw)}
}

// String returns the normalized wire form (PHP getValue()).
func (d DecimalNumber) String() string {
	if d.value == "" {
		return "0"
	}
	return d.value
}

// scaled returns the value as a big.Int scaled by 10^8 (bcmath scale). It is
// exact for the fixed-point decimals the app handles. A malformed value scales
// to 0 (NewDecimal already normalized the input, so this should not happen).
func (d DecimalNumber) scaled() *big.Int {
	s := d.value
	if s == "" {
		return big.NewInt(0)
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, fracPart := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	if len(fracPart) > decimalScale {
		fracPart = fracPart[:decimalScale]
	}
	for len(fracPart) < decimalScale {
		fracPart += "0"
	}
	n, ok := new(big.Int).SetString(intPart+fracPart, 10)
	if !ok {
		return big.NewInt(0)
	}
	if neg {
		n.Neg(n)
	}
	return n
}

// fromScaled builds a DecimalNumber from a 10^8-scaled big.Int, rendering it as
// a fixed-point string and normalizing (so trailing zeros are trimmed).
func fromScaled(n *big.Int) DecimalNumber {
	neg := n.Sign() < 0
	abs := new(big.Int).Abs(n).String()
	for len(abs) <= decimalScale {
		abs = "0" + abs
	}
	cut := len(abs) - decimalScale
	raw := abs[:cut] + "." + abs[cut:]
	if neg {
		raw = "-" + raw
	}
	return NewDecimal(raw)
}

// Sub returns d - other at bcmath scale 8 (exact), matching PHP
// DecimalNumber::sub.
func (d DecimalNumber) Sub(other DecimalNumber) DecimalNumber {
	return fromScaled(new(big.Int).Sub(d.scaled(), other.scaled()))
}

// IsZero reports whether the value equals zero.
func (d DecimalNumber) IsZero() bool { return d.scaled().Sign() == 0 }

// IsNegative reports whether the value is strictly less than zero.
func (d DecimalNumber) IsNegative() bool { return d.scaled().Sign() < 0 }

// Equals reports whether d and other are numerically equal (PHP equals()).
func (d DecimalNumber) Equals(other DecimalNumber) bool {
	return d.scaled().Cmp(other.scaled()) == 0
}

// Abs returns the absolute value.
func (d DecimalNumber) Abs() DecimalNumber {
	return fromScaled(new(big.Int).Abs(d.scaled()))
}

// Add returns d + other at bcmath scale 8 (exact), matching PHP
// DecimalNumber::add.
func (d DecimalNumber) Add(other DecimalNumber) DecimalNumber {
	return fromScaled(new(big.Int).Add(d.scaled(), other.scaled()))
}

// Mul returns d * other at bcmath scale 8. bcmul computes the full product then
// TRUNCATES toward zero to 8 fraction digits (bcmath does not round). With both
// operands scaled by 10^8, the raw product is scaled by 10^16; truncating to
// scale 8 means dividing by 10^8 toward zero.
func (d DecimalNumber) Mul(other DecimalNumber) DecimalNumber {
	prod := new(big.Int).Mul(d.scaled(), other.scaled()) // scale 16
	q := new(big.Int).Quo(prod, decimalPow8)             // Quo truncates toward zero
	return fromScaled(q)
}

// Div returns d / other at bcmath scale 8, truncated toward zero (bcdiv
// semantics). Panics on division by zero (PHP throws DivisionByZeroError).
// Computed as floor_toward_zero((d_scaled * 10^8) / other_scaled).
func (d DecimalNumber) Div(other DecimalNumber) DecimalNumber {
	den := other.scaled()
	if den.Sign() == 0 {
		panic("vo.DecimalNumber: division by zero")
	}
	num := new(big.Int).Mul(d.scaled(), decimalPow8) // scale 16
	q := new(big.Int).Quo(num, den)                  // truncates toward zero -> scale 8
	return fromScaled(q)
}

// Round rounds to `precision` decimal places using PHP's round() semantics
// (round half AWAY from zero), matching DecimalNumber::round. precision may be 0.
// Implemented in exact integer space: the scale-8 value is taken to `precision`
// places by dividing by 10^(8-precision) with half-away rounding on the
// remainder, then re-scaled.
func (d DecimalNumber) Round(precision int) DecimalNumber {
	if precision < 0 {
		precision = 0
	}
	if precision >= decimalScale {
		return d // already within scale 8; nothing to round
	}
	val := d.scaled() // scale 8
	div := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimalScale-precision)), nil)

	q, r := new(big.Int).QuoRem(val, div, new(big.Int)) // q toward zero, r same sign as val
	// half-away-from-zero: if 2*|r| >= div, bump q away from zero.
	twoAbsR := new(big.Int).Abs(r)
	twoAbsR.Lsh(twoAbsR, 1) // *2
	if twoAbsR.Cmp(div) >= 0 {
		if val.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	// q is now at scale `precision`; re-scale back to 8.
	q.Mul(q, div)
	return fromScaled(q)
}

// IsGreaterThan reports whether d > other (PHP isGreaterThan).
func (d DecimalNumber) IsGreaterThan(other DecimalNumber) bool {
	return d.scaled().Cmp(other.scaled()) == 1
}

// IsLessThan reports whether d < other (PHP isLessThan).
func (d DecimalNumber) IsLessThan(other DecimalNumber) bool {
	return d.scaled().Cmp(other.scaled()) == -1
}

// decimalPow8 is 10^8 (the scale factor) as a big.Int, computed once.
var decimalPow8 = new(big.Int).Exp(big.NewInt(10), big.NewInt(decimalScale), nil)

// normalizeDecimal reproduces PHP DecimalNumber::normalize() for string inputs
// that are plain decimals (the only form the DB yields: a fixed-point
// NUMERIC(19,8) string, optionally signed). Scientific notation and float
// formatting are not needed for DB-sourced values, so they are intentionally
// omitted; the integer/fraction zero-trimming rules match PHP exactly.
//
// Rules (from PHP normalize()):
//   - "" or "0" -> "0"
//   - strip a leading '-' (re-applied at the end), so "-0" cases collapse to "0"
//   - if it has a '.': trim the integer part's leading zeros (keeping one "0"),
//     truncate the fraction to scale 8, then trim the fraction's trailing zeros;
//     drop the '.' entirely if the fraction becomes empty
//   - if no '.': trim leading zeros (keeping one "0")
//   - a value of just "0" after trimming is returned unsigned ("0", never "-0")
func normalizeDecimal(num string) string {
	num = strings.TrimSpace(num)
	if num == "" || num == "0" {
		return "0"
	}

	negative := strings.HasPrefix(num, "-")
	if negative {
		num = num[1:]
	}

	if strings.Contains(num, ".") {
		parts := strings.SplitN(num, ".", 2)
		intPart, fracPart := parts[0], parts[1]
		if intPart == "" || intPart == "0" {
			intPart = "0"
		} else {
			intPart = strings.TrimLeft(intPart, "0")
			if intPart == "" {
				intPart = "0"
			}
		}
		if len(fracPart) > decimalScale {
			fracPart = fracPart[:decimalScale]
		}
		fracPart = strings.TrimRight(fracPart, "0")
		if fracPart == "" {
			num = intPart
		} else {
			num = intPart + "." + fracPart
		}
	} else {
		num = strings.TrimLeft(num, "0")
		if num == "" {
			num = "0"
		}
	}

	// PHP adds a leading zero for ".x" forms; our integer-part handling already
	// guarantees a leading "0", so this is belt-and-suspenders.
	if strings.HasPrefix(num, ".") {
		num = "0" + num
	}

	if negative && num != "0" {
		return "-" + num
	}
	return num
}
