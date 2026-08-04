// Package sortkey implements fractional index keys: short base-62 strings whose
// lexicographic byte order is the intended sort order, and between any two of
// which a new key always exists. Storing order as a key rather than a dense
// integer means moving one item writes one row and never renumbers its siblings.
//
// A key is a magnitude head, an integer part, and optional fractional digits.
// The head encodes how many integer digits follow: lowercase 'a'=1 through
// 'z'=26 for the non-negative magnitudes, uppercase inverted ('Z'=1 through
// 'A'=26) for the negative side, which works because 'Z' < 'a' in ASCII.
// Appending increments the integer part (a0, a1, ... az, b10) rather than
// bisecting toward infinity, which is what keeps keys 2-4 characters under
// append-heavy use; inserting between two keys appends fractional digits, which
// is why a midpoint always exists.
package sortkey

import "github.com/econumo/econumo/internal/shared/errs"

// alphabet is base-62 in ASCII-ascending order, so digit order, lexicographic
// order and byte order all coincide. The stored column relies on this: SQLite
// compares TEXT byte-wise and the PostgreSQL column is declared COLLATE "C" to
// match, so ORDER BY sort_key is identical on both engines.
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Key is a fractional index key. The empty Key is the +/-infinity sentinel when
// passed to Between, and marks a row excluded from ordering when stored.
type Key string

// Growth selects the seed for an empty list, based on which end that list grows
// from when rows are created.
type Growth int

const (
	// GrowsUp seeds lists whose creates append: categories, tags, payees,
	// accounts and account folders.
	GrowsUp Growth = iota
	// GrowsDown seeds lists whose creates prepend: budget folders and budget
	// envelope elements. Seeding above the bottom of the space buys roughly 3844
	// prepends before keys reach the inverted uppercase magnitudes, keeping the
	// algorithm's hardest branch off the common path.
	GrowsDown
)

// Seed returns the key for the first row of an empty list.
func Seed(g Growth) Key {
	if g == GrowsDown {
		return "c000"
	}
	return "a0"
}

func invalid(msg string) error {
	return &errs.ValidationError{Msg: msg, MsgCode: errs.CodeInvalidID}
}

// digitIndex returns b's value in the base-62 alphabet, or -1 if b is not a
// base-62 digit.
func digitIndex(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'A' && b <= 'Z':
		return int(b-'A') + 10
	case b >= 'a' && b <= 'z':
		return int(b-'a') + 36
	}
	return -1
}

// integerLen returns the total length of the integer part, head included, for a
// magnitude head byte.
func integerLen(head byte) (int, error) {
	switch {
	case head >= 'a' && head <= 'z':
		return int(head-'a') + 2, nil
	case head >= 'A' && head <= 'Z':
		return int('Z'-head) + 2, nil
	}
	return 0, invalid("invalid sort key magnitude")
}

// integerPart returns the leading integer portion of k: the head plus exactly
// the number of digits that head declares.
func integerPart(k Key) (Key, error) {
	if len(k) == 0 {
		return "", invalid("empty sort key")
	}
	n, err := integerLen(k[0])
	if err != nil {
		return "", err
	}
	if len(k) < n {
		return "", invalid("sort key shorter than its magnitude declares")
	}
	return k[:n], nil
}

// Validate reports whether k is a well-formed key: a valid magnitude head, the
// exact digit count that head declares, only base-62 characters, and no trailing
// zero in the fractional part. A trailing zero is rejected because it would let
// two distinct strings denote the same position, and a later midpoint could then
// collide with an existing key.
func Validate(k Key) error {
	ip, err := integerPart(k)
	if err != nil {
		return err
	}
	for i := 1; i < len(ip); i++ {
		if digitIndex(ip[i]) < 0 {
			return invalid("invalid character in sort key integer part")
		}
	}
	frac := k[len(ip):]
	for i := 0; i < len(frac); i++ {
		if digitIndex(frac[i]) < 0 {
			return invalid("invalid character in sort key fraction")
		}
	}
	if len(frac) > 0 && frac[len(frac)-1] == alphabet[0] {
		return invalid("sort key fraction must not end in a zero digit")
	}
	return nil
}

// incrementInteger returns the next integer part, rolling the magnitude up when
// the digits overflow. It returns "" when the space is exhausted at 'z'.
func incrementInteger(x Key) (Key, error) {
	if _, err := integerPart(x); err != nil {
		return "", err
	}
	head := x[0]
	digs := []byte(x[1:])
	carry := true
	for i := len(digs) - 1; carry && i >= 0; i-- {
		d := digitIndex(digs[i]) + 1
		if d == len(alphabet) {
			digs[i] = alphabet[0]
		} else {
			digs[i] = alphabet[d]
			carry = false
		}
	}
	if !carry {
		return Key(append([]byte{head}, digs...)), nil
	}
	if head == 'Z' {
		return "a0", nil
	}
	if head == 'z' {
		return "", nil
	}
	h := head + 1
	if h > 'a' {
		digs = append(digs, alphabet[0])
	} else {
		digs = digs[:len(digs)-1]
	}
	return Key(append([]byte{h}, digs...)), nil
}

// decrementInteger returns the previous integer part, rolling the magnitude down
// when the digits underflow. It returns "" when the space is exhausted at 'A'.
func decrementInteger(x Key) (Key, error) {
	if _, err := integerPart(x); err != nil {
		return "", err
	}
	last := alphabet[len(alphabet)-1]
	head := x[0]
	digs := []byte(x[1:])
	borrow := true
	for i := len(digs) - 1; borrow && i >= 0; i-- {
		d := digitIndex(digs[i]) - 1
		if d == -1 {
			digs[i] = last
		} else {
			digs[i] = alphabet[d]
			borrow = false
		}
	}
	if !borrow {
		return Key(append([]byte{head}, digs...)), nil
	}
	if head == 'a' {
		return Key([]byte{'Z', last}), nil
	}
	if head == 'A' {
		return "", nil
	}
	h := head - 1
	if h < 'Z' {
		digs = append(digs, last)
	} else {
		digs = digs[:len(digs)-1]
	}
	return Key(append([]byte{h}, digs...)), nil
}

// midpoint returns a fractional string strictly between a and b, where b == ""
// means +infinity. Both arguments are fractions only, carrying no magnitude head.
func midpoint(a, b Key) (Key, error) {
	zero := alphabet[0]
	if b != "" && a >= b {
		return "", invalid("sort key bounds are inverted")
	}
	if (len(a) > 0 && a[len(a)-1] == zero) || (len(b) > 0 && b[len(b)-1] == zero) {
		return "", invalid("sort key fraction must not end in a zero digit")
	}
	if b != "" {
		n := 0
		for n < len(b) {
			ac := zero
			if n < len(a) {
				ac = a[n]
			}
			if ac != b[n] {
				break
			}
			n++
		}
		if n > 0 {
			var tail Key
			if n < len(a) {
				tail = a[n:]
			}
			rest, err := midpoint(tail, b[n:])
			if err != nil {
				return "", err
			}
			return b[:n] + rest, nil
		}
	}
	digitA := 0
	if len(a) > 0 {
		digitA = digitIndex(a[0])
	}
	digitB := len(alphabet)
	if b != "" {
		digitB = digitIndex(b[0])
	}
	if digitB-digitA > 1 {
		mid := (digitA + digitB) / 2
		return Key(alphabet[mid : mid+1]), nil
	}
	if len(b) > 1 {
		return b[:1], nil
	}
	var tail Key
	if len(a) > 0 {
		tail = a[1:]
	}
	rest, err := midpoint(tail, "")
	if err != nil {
		return "", err
	}
	return Key(alphabet[digitA:digitA+1]) + rest, nil
}

// Between returns a key strictly between prev and next. An empty prev means
// "before everything" and an empty next means "after everything", so
// Between("", "") seeds an empty list, Between(last, "") appends and
// Between("", first) prepends.
func Between(prev, next Key) (Key, error) {
	if prev != "" {
		if err := Validate(prev); err != nil {
			return "", err
		}
	}
	if next != "" {
		if err := Validate(next); err != nil {
			return "", err
		}
	}
	if prev != "" && next != "" && prev >= next {
		return "", invalid("sort key bounds are inverted")
	}

	if prev == "" {
		if next == "" {
			return Seed(GrowsUp), nil
		}
		ib, err := integerPart(next)
		if err != nil {
			return "", err
		}
		// next carries a fraction, so its bare integer part already sorts before it.
		if ib < next {
			return ib, nil
		}
		dec, err := decrementInteger(ib)
		if err != nil {
			return "", err
		}
		if dec == "" {
			return "", invalid("sort key space exhausted below")
		}
		return dec, nil
	}

	ia, err := integerPart(prev)
	if err != nil {
		return "", err
	}
	fa := prev[len(ia):]

	if next == "" {
		inc, ierr := incrementInteger(ia)
		if ierr != nil {
			return "", ierr
		}
		if inc == "" {
			mid, merr := midpoint(fa, "")
			if merr != nil {
				return "", merr
			}
			return ia + mid, nil
		}
		return inc, nil
	}

	ib, err := integerPart(next)
	if err != nil {
		return "", err
	}
	fb := next[len(ib):]
	if ia == ib {
		mid, merr := midpoint(fa, fb)
		if merr != nil {
			return "", merr
		}
		return ia + mid, nil
	}
	inc, err := incrementInteger(ia)
	if err != nil {
		return "", err
	}
	if inc == "" {
		return "", invalid("sort key space exhausted above")
	}
	if inc < next {
		return inc, nil
	}
	mid, err := midpoint(fa, "")
	if err != nil {
		return "", err
	}
	return ia + mid, nil
}
