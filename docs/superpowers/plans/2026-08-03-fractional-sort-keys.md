# Fractional Sort Keys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `int16` `position` column and absolute-position reorder API with base-62 fractional sort keys and relative `move-*` endpoints, so one drag writes one row instead of N.

**Architecture:** A new dependency-free kernel package `internal/shared/sortkey` ports the `fractional-indexing` (Figma) algorithm. Six tables gain a `sort_key TEXT` column (pgsql `COLLATE "C"`), backfilled in pure SQL from the current `(position, id)` order. Seven `order-*-list` routes are replaced 1:1 by `move-*` routes taking `{id, afterId}`. The key never leaves the server; responses keep the frozen `position` integer, now a computed dense 0-based index.

**Tech Stack:** Go (stdlib + sqlc), SQLite (modernc) + PostgreSQL (pgx), React 19 + dnd-kit.

**Spec:** `docs/superpowers/specs/2026-08-02-fractional-sort-keys-design.md`

## Global Constraints

- **Green at every task boundary.** `position` is dropped only in Task 13; until then both columns coexist so `go build ./...` never breaks mid-plan.
- **Migration SQL comments must be ASCII-only.** An em dash mangles sqlc v1.30's sqlite codegen (byte-offset bug). Applies to `internal/infra/storage/sqlc/query/**/*.sql` too.
- **pgsql column is `TEXT COLLATE "C"`.** Without it PostgreSQL's `ORDER BY` folds case and `enginecompare` byte-equality fails.
- **Base-62 alphabet, ASCII-ascending:** `0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz`.
- **Seed policy:** append-oriented lists (categories, tags, payees, accounts, account folders) seed `a0`; prepend-oriented lists (budget folders, budget envelope elements) seed `c000`.
- **`METRICS` keys are frozen** — reuse `CATEGORY_ORDER_LIST`, `TAG_ORDER_LIST`, `PAYEE_ORDER_LIST`, `ACCOUNT_ORDER_LIST`, `ACCOUNT_FOLDER_ORDER_LIST`, `BUDGET_FOLDER_CHANGE_ORDER`, `BUDGET_CHANGE_ORDER_ELEMENT` unchanged. Renaming them breaks `web/src/lib/metrics-coverage.test.ts`.
- **No new `errs` codes.** An unresolvable `afterId` appends silently (matching today's silent-skip). A new code would mean 11 locale catalogues plus the `errs.AllCodes` ↔ `errors.*` two-way parity guard.
- **Test floors are floors, never lowered:** `internal/test/apiparity/guard_test.go` `minRoutes = 101`; `internal/test/apiparity/catalogue_test.go` `min = 36`; `internal/test/mcpparity/smoke_test.go` inline `< 6`.
- **Golden regeneration:** `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and `.../mcpparity/`. Always inspect the diff; never hand-edit a golden. A renamed scenario orphans its old golden — delete it or `guard_test.go:135` fails.
- **Frontend gate:** `pnpm exec tsc -b` in `web/`. vitest and oxlint do not type-check.
- **Commands:** `go generate ./internal/infra/storage/sqlc/...` (sqlc), `make go-test` (smoke + 80% coverage gate), `make test` (full, needs Postgres).

---

## File Structure

**New:**
- `internal/shared/sortkey/sortkey.go` — `Key`, alphabet, validation, `Between`, `Seed`
- `internal/shared/sortkey/sortkey_test.go`
- `internal/shared/sortkey/place.go` — `Item`, `Place` (list arithmetic)
- `internal/shared/sortkey/place_test.go`
- `internal/infra/storage/migrations/{sqlite,pgsql}/20260803000000.sql`
- `internal/infra/storage/migrations/migration_20260803_test.go`
- `internal/{category,tag,payee}/move.go`, `internal/account/move.go`, `internal/budget/move_folder.go`
- `internal/test/apiparity/catalogue_moves.go`

**Deleted (Task 13 unless noted):**
- `internal/{category,tag,payee}/order.go` (their own tasks)
- `internal/account/order.go`; `resetFolderPositions` in `internal/account/folder_usecase.go:202`
- `shiftElements` in `internal/budget/move.go:76-106`
- `MaxPosition` in `internal/account/repo/repo.go:155`; `CountByOwner` in category/tag/payee repos
- `validatePositionField` in `internal/model/budget_dto.go:509`; `internal/model/position_validation_test.go`
- `internal/test/apiparity/catalogue_orderlists.go` + `testdata/golden/order_lists.golden`

**Modified:** 6 model entities, 6 repo packages, 7 api route files, `internal/budget/move.go` (→ `syncElements`), `internal/budget/mcp/mcp.go`, `internal/test/fixture/entities.go`, and the frontend files in Task 12.

---

## Task 1: sortkey — Key, validation, Between

**Files:**
- Create: `internal/shared/sortkey/sortkey.go`
- Test: `internal/shared/sortkey/sortkey_test.go`

**Interfaces:**
- Consumes: nothing (kernel leaf; imports only stdlib + `internal/shared/errs`)
- Produces: `type Key string`; `func Between(prev, next Key) (Key, error)`; `func Seed(g Growth) Key`; `type Growth int` with `GrowsUp`, `GrowsDown`; `func Validate(k Key) error`

This is a port of the `fractional-indexing` algorithm (Figma / rocicorp). The magnitude head encodes the integer-part digit count: lowercase `a`=1 … `z`=26; uppercase inverts and counts down, `Z`=1 … `A`=26. `Between("", "")` is `"a0"`.

- [ ] **Step 1: Write the failing test**

Create `internal/shared/sortkey/sortkey_test.go`:

```go
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
		k, _ = Between(k, "")
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

// TestBetween_RejectsInverted guards the caller contract.
func TestBetween_RejectsInverted(t *testing.T) {
	if _, err := Between("a2", "a1"); err == nil {
		t.Fatal("Between with prev >= next must error")
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
	for _, bad := range []Key{"", "0", "a", "a!", "a0 ", "!0"} {
		if err := Validate(bad); err == nil {
			t.Errorf("Validate(%q) = nil, want error", bad)
		}
	}
	for _, ok := range []Key{"a0", "az", "b00", "c000", "Zz", "c000V"} {
		if err := Validate(ok); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", ok, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shared/sortkey/`
Expected: FAIL — the package does not exist yet (`no Go files` / undefined `Between`, `Seed`, `Validate`, `Key`, `GrowsUp`).

- [ ] **Step 3: Write the implementation**

Create `internal/shared/sortkey/sortkey.go`:

```go
// Package sortkey implements fractional index keys: short base-62 strings whose
// lexicographic byte order is the intended sort order, and between any two of
// which a new key always exists. Storing order as a key rather than a dense
// integer means moving one item writes one row and never renumbers its
// siblings.
//
// The encoding is the fractional-indexing algorithm (Figma / rocicorp). A key is
// a magnitude head, an integer part, and optional fractional digits. The head
// encodes how many integer digits follow: lowercase 'a'=1 through 'z'=26 for
// non-negative magnitudes, uppercase inverted ('Z'=1 through 'A'=26) for the
// negative side, which is why 'Z' < 'a' in ASCII gives the right global order.
// Appending increments the integer part (a0, a1, ... az, b00) rather than
// bisecting toward infinity, which is what keeps keys 2-4 characters under
// append-heavy use.
package sortkey

import "github.com/econumo/econumo/internal/shared/errs"

// alphabet is base-62 in ASCII-ascending order, so lexicographic order,
// byte order and digit order all coincide.
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Key is a fractional index key. The empty Key is the +/-infinity sentinel when
// passed to Between, and marks a row excluded from ordering when stored.
type Key string

// Growth selects the seed for an empty list, based on which end that list grows
// from when rows are created.
type Growth int

const (
	// GrowsUp seeds lists whose creates append (categories, tags, payees,
	// accounts, account folders).
	GrowsUp Growth = iota
	// GrowsDown seeds lists whose creates prepend (budget folders, budget
	// envelope elements). Seeding above the bottom of the space keeps the
	// inverted uppercase magnitudes off the common path.
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

// integerLen returns the total length of the integer part (head included) for a
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

// integerPart returns the leading integer portion of k (head + its digits).
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
// exact number of integer digits that head declares, only base-62 characters,
// and no trailing zero in the fractional part (which would make two distinct
// strings denote the same position).
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

// midpoint returns a fractional string strictly between a and b (b == "" means
// +infinity). Both arguments are fractions only, with no magnitude head.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/shared/sortkey/ -v`
Expected: PASS — all 9 tests.

- [ ] **Step 5: Verify the architecture rule still holds**

Run: `go test ./internal/test/archtest/`
Expected: PASS. `internal/shared/sortkey` is already kernel (`topOf` → `shared`), so no archtest edit is needed — this run proves the new package imports nothing outside the kernel.

- [ ] **Step 6: Commit**

```bash
git add internal/shared/sortkey/
git commit -m "feat(sortkey): fractional index keys with magnitude-prefixed base-62 encoding"
```

---

## Task 2: sortkey — Place

**Files:**
- Create: `internal/shared/sortkey/place.go`
- Test: `internal/shared/sortkey/place_test.go`

**Interfaces:**
- Consumes: `Key`, `Between`, `Seed`, `Growth` from Task 1
- Produces: `type Item struct { ID string; Key Key }`; `func Place(siblings []Item, afterID string, g Growth) (Key, error)`

`Place` is the single piece of list arithmetic all seven move use cases share. It lives in the kernel because it is pure — no DB, no feature imports — so the seven features reach it without importing each other.

- [ ] **Step 1: Write the failing test**

Create `internal/shared/sortkey/place_test.go`:

```go
package sortkey

import "testing"

func sibs(keys ...Key) []Item {
	out := make([]Item, 0, len(keys))
	for i, k := range keys {
		out = append(out, Item{ID: string(rune('a' + i)), Key: k})
	}
	return out
}

// TestPlace_EmptyListUsesSeed covers the first row in a list.
func TestPlace_EmptyListUsesSeed(t *testing.T) {
	got, err := Place(nil, "", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got != "a0" {
		t.Fatalf("Place(nil, \"\", GrowsUp) = %q, want \"a0\"", got)
	}
	got, err = Place(nil, "", GrowsDown)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got != "c000" {
		t.Fatalf("Place(nil, \"\", GrowsDown) = %q, want \"c000\"", got)
	}
}

// TestPlace_NilAnchorMovesToFront: afterID "" means "put it first".
func TestPlace_NilAnchorMovesToFront(t *testing.T) {
	got, err := Place(sibs("a0", "a1", "a2"), "", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got >= "a0" {
		t.Fatalf("Place = %q, want a key sorting before \"a0\"", got)
	}
}

// TestPlace_BetweenNeighbours anchors on a middle sibling.
func TestPlace_BetweenNeighbours(t *testing.T) {
	got, err := Place(sibs("a0", "a1", "a2"), "a", GrowsUp) // after the first item
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if !(got > "a0" && got < "a1") {
		t.Fatalf("Place = %q, want strictly between a0 and a1", got)
	}
}

// TestPlace_AfterLastAppends anchors on the tail.
func TestPlace_AfterLastAppends(t *testing.T) {
	got, err := Place(sibs("a0", "a1", "a2"), "c", GrowsUp) // after the third item
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got <= "a2" {
		t.Fatalf("Place = %q, want a key sorting after \"a2\"", got)
	}
}

// TestPlace_UnknownAnchorAppends is the documented degradation: an anchor that
// is not in the caller's scope (deleted concurrently, another user's row, a
// different folder) appends rather than erroring. This matches the pre-existing
// silent-skip behaviour of the order-*-list use cases and avoids a new errs code.
func TestPlace_UnknownAnchorAppends(t *testing.T) {
	got, err := Place(sibs("a0", "a1"), "no-such-id", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got <= "a1" {
		t.Fatalf("Place = %q, want an append past \"a1\"", got)
	}
}

// TestPlace_SkipsTheMovedItemsOwnKey: when an item is moved within its own list
// the caller passes the full sibling set including the item itself; anchoring on
// the item directly before it must not collide with the item's current key.
func TestPlace_ToleratesDuplicateKeys(t *testing.T) {
	// Legacy data can hold duplicates; Place must still produce a usable key.
	got, err := Place([]Item{{ID: "a", Key: "a0"}, {ID: "b", Key: "a0"}}, "a", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got == "" {
		t.Fatal("Place returned an empty key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shared/sortkey/ -run TestPlace -v`
Expected: FAIL — `undefined: Place`, `undefined: Item`.

- [ ] **Step 3: Write the implementation**

Create `internal/shared/sortkey/place.go`:

```go
package sortkey

// Item is one sibling in an ordered list: the id a caller anchors against and
// the key that decides its slot.
type Item struct {
	ID  string
	Key Key
}

// Place returns the key for an item dropped immediately after afterID within
// siblings, which must already be sorted by Key.
//
// An empty afterID means "move to the front". An afterID that is not present in
// siblings appends to the end rather than erroring: the anchor may have been
// deleted concurrently, or belong to another user or folder, and the pre-existing
// order-*-list use cases silently skipped such ids too. Degrading here keeps that
// behaviour and avoids a coded error that would need a translation in all 11
// locale catalogues.
//
// Siblings holding duplicate keys (possible in data backfilled from the legacy
// integer positions) are tolerated: the search for the following key skips any
// neighbour that does not sort strictly after the anchor.
func Place(siblings []Item, afterID string, g Growth) (Key, error) {
	if len(siblings) == 0 {
		return Seed(g), nil
	}

	if afterID == "" {
		return Between("", siblings[0].Key)
	}

	idx := -1
	for i, s := range siblings {
		if s.ID == afterID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Between(siblings[len(siblings)-1].Key, "")
	}

	prev := siblings[idx].Key
	for _, s := range siblings[idx+1:] {
		if s.Key > prev {
			return Between(prev, s.Key)
		}
	}
	return Between(prev, "")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/shared/sortkey/ -v`
Expected: PASS — Task 1's tests plus the six `TestPlace_*` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/shared/sortkey/place.go internal/shared/sortkey/place_test.go
git commit -m "feat(sortkey): Place resolves a relative-move anchor to a key"
```

---

## Task 3: Migration — add and backfill `sort_key`

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260803000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260803000000.sql`
- Test: `internal/infra/storage/migrations/migration_20260803_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (pure SQL)
- Produces: a `sort_key` column on `categories`, `tags`, `payees`, `folders`, `accounts_options`, `budgets_folders`, `budgets_elements`, backfilled in `(position, id)` order

`position` is **kept** here so the Go build stays green; it is dropped in Task 13 by editing this same (unreleased) migration.

The backfill is pure SQL because migrations are embedded `.sql` only — there is no Go migration hook. `row_number()` gives the rank; three `substr` calls encode it as base-62; the literal `'c'` is the magnitude head for a 3-digit integer part, so every row in a scope shares a magnitude and ordering is decided by the digits alone. 62³ = 238,328 rows per scope.

- [ ] **Step 1: Write the failing test**

Create `internal/infra/storage/migrations/migration_20260803_test.go`:

```go
package migrations_test

// Verifies the 20260803000000 migration: every ordered table gains a sort_key
// whose byte order reproduces the previous (position, id) order within its
// scope, and whose backfilled values are well-formed keys.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/shared/sortkey"
	_ "modernc.org/sqlite"
)

const sortKeyVersion = "20260803000000"

// runUpTo applies every migration strictly before version, returning the db.
func runUpTo(t *testing.T, name, version string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	var before []migrate.Migration
	for _, f := range migrations.SQLite() {
		if f.Version < version {
			before = append(before, migrate.Migration{Version: f.Version, SQL: f.SQL})
		}
	}
	if err := migrate.Run(context.Background(), db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	return db
}

func runAll(t *testing.T, db *sql.DB) {
	t.Helper()
	var all []migrate.Migration
	for _, f := range migrations.SQLite() {
		all = append(all, migrate.Migration{Version: f.Version, SQL: f.SQL})
	}
	if err := migrate.Run(context.Background(), db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}
}

func TestMigration20260803_BackfillPreservesOrderPerScope(t *testing.T) {
	db := runUpTo(t, "sortkey_order", sortKeyVersion)
	ctx := context.Background()
	seedUser(t, db, "u1")
	seedUser(t, db, "u2")

	// Deliberately sparse AND duplicated positions, which the schema allows.
	rows := []struct{ id, user string; pos int }{
		{"c1", "u1", 5}, {"c2", "u1", 0}, {"c3", "u1", 5}, {"c4", "u2", 3},
	}
	for _, r := range rows {
		if _, err := db.ExecContext(ctx, `INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at)
			VALUES (?, ?, 'n', ?, 0, 'i', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, r.id, r.user, r.pos); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
	}

	runAll(t, db)

	// u1's rows must come out ordered c2 (pos 0), then c1 and c3 (pos 5, id tie-break).
	got := []string{}
	cur, err := db.QueryContext(ctx, `SELECT id FROM categories WHERE user_id = 'u1' ORDER BY sort_key, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer cur.Close()
	for cur.Next() {
		var id string
		if err := cur.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	want := []string{"c2", "c1", "c3"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}

	// Scopes are independent: u2's single row also gets the first key.
	var k1, k2 string
	if err := db.QueryRowContext(ctx, `SELECT sort_key FROM categories WHERE id = 'c2'`).Scan(&k1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT sort_key FROM categories WHERE id = 'c4'`).Scan(&k2); err != nil {
		t.Fatal(err)
	}
	if k1 != k2 {
		t.Errorf("per-scope rank should restart: u1 first = %q, u2 first = %q", k1, k2)
	}
}

func TestMigration20260803_BackfilledKeysAreWellFormed(t *testing.T) {
	db := runUpTo(t, "sortkey_wellformed", sortKeyVersion)
	ctx := context.Background()
	seedUser(t, db, "u1")
	for i, id := range []string{"c1", "c2", "c3"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at)
			VALUES (?, 'u1', 'n', ?, 0, 'i', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, id, i); err != nil {
			t.Fatal(err)
		}
	}
	runAll(t, db)

	cur, err := db.QueryContext(ctx, `SELECT sort_key FROM categories ORDER BY sort_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer cur.Close()
	n := 0
	for cur.Next() {
		var k string
		if err := cur.Scan(&k); err != nil {
			t.Fatal(err)
		}
		if err := sortkey.Validate(sortkey.Key(k)); err != nil {
			t.Errorf("backfilled key %q is not well-formed: %v", k, err)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("scanned %d keys, want 3", n)
	}
}

// TestMigration20260803_EveryOrderedTableHasTheColumn guards against a table
// being forgotten in the six-table sweep.
func TestMigration20260803_EveryOrderedTableHasTheColumn(t *testing.T) {
	db := runUpTo(t, "sortkey_columns", sortKeyVersion)
	runAll(t, db)
	for _, table := range []string{"categories", "tags", "payees", "folders", "accounts_options", "budgets_folders", "budgets_elements"} {
		var n int
		q := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'sort_key'`
		if err := db.QueryRowContext(context.Background(), q, table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s is missing the sort_key column", table)
		}
	}
}
```

`seedUser` already exists at package scope in `internal/infra/storage/migrations/migration_20260730_test.go` and is shared across the `migrations_test` package — do not redefine it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260803 -v`
Expected: FAIL — `no such column: sort_key`.

- [ ] **Step 3: Write the sqlite migration**

Create `internal/infra/storage/migrations/sqlite/20260803000000.sql`. Comments must be ASCII-only.

```sql
-- Replace the int16 `position` ordering column with `sort_key`, a base-62
-- fractional index key (see internal/shared/sortkey). A key sorts byte-wise, and
-- a new key always exists between any two, so moving one row writes one row and
-- never renumbers siblings.
--
-- Backfill: row_number() gives each row's rank in the CURRENT (position, id)
-- order within its scope; three substr() calls encode that rank as base-62
-- digits. The literal 'c' is the magnitude head for a 3-digit integer part, so
-- every row in a scope shares a magnitude and ordering is decided by the digits
-- alone. Three digits hold 62^3 = 238328 rows per scope.
--
-- `position` is retained for now and dropped once every read path has moved over.

ALTER TABLE categories        ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE tags              ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE payees            ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE folders           ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts_options  ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE budgets_folders   ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE budgets_elements  ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';

UPDATE categories SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n FROM categories) r
  WHERE r.id = categories.id
);

UPDATE tags SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n FROM tags) r
  WHERE r.id = tags.id
);

UPDATE payees SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n FROM payees) r
  WHERE r.id = payees.id
);

UPDATE folders SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n FROM folders) r
  WHERE r.id = folders.id
);

-- accounts_options is keyed by (account_id, user_id), so the correlation needs
-- both columns.
UPDATE accounts_options SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT account_id, user_id,
               row_number() OVER (PARTITION BY user_id ORDER BY position, account_id) - 1 AS n
        FROM accounts_options) r
  WHERE r.account_id = accounts_options.account_id AND r.user_id = accounts_options.user_id
);

UPDATE budgets_folders SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY budget_id ORDER BY position, id) - 1 AS n FROM budgets_folders) r
  WHERE r.id = budgets_folders.id
);

-- Budget elements are ordered per (budget, folder); NULL folder_id groups
-- together in both engines.
UPDATE budgets_elements SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY budget_id, folder_id ORDER BY position, id) - 1 AS n FROM budgets_elements) r
  WHERE r.id = budgets_elements.id
);
```

- [ ] **Step 4: Write the pgsql migration**

Create `internal/infra/storage/migrations/pgsql/20260803000000.sql`. Identical to the sqlite file except every `ADD COLUMN` carries `COLLATE "C"`:

```sql
-- PostgreSQL variant. See the sqlite sibling for the semantics.
--
-- COLLATE "C" is load-bearing: SQLite sorts TEXT byte-wise (BINARY) while
-- PostgreSQL sorts by database collation, which folds case and orders digits
-- against letters differently. The enginecompare suite requires byte-identical
-- responses from both engines, so the column must sort by raw bytes.

ALTER TABLE categories        ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE tags              ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE payees            ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE folders           ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE accounts_options  ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE budgets_folders   ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE budgets_elements  ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
```

followed by the **same seven `UPDATE` statements verbatim** from the sqlite file (`substr`, `||`, `%` and integer `/` behave identically in PostgreSQL).

- [ ] **Step 5: Run the migration tests**

Run: `go test ./internal/infra/storage/migrations/ -v`
Expected: PASS — the three new `TestMigration20260803_*` tests plus the existing ones.

- [ ] **Step 6: Confirm nothing else broke**

Run: `go build ./... && go test ./...`
Expected: PASS. `position` still exists and every read path still uses it, so this migration is purely additive.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/storage/migrations/
git commit -m "feat(db): add sort_key column and backfill from (position, id) order"
```

---

## Task 4: Carry `sort_key` through model, sqlc and repos

**Files:**
- Modify: `internal/model/{category,tag,payee,account_folder,budget}.go` — add `SortKey`, `UpdateSortKey`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/{categories,tags,payees,folders,accounts,budgets}.sql` — add `sort_key` to every SELECT list and every upsert
- Modify: `internal/{category,tag,payee,account,budget}/repo/*.go` — hydrate and persist `SortKey`
- Modify: `internal/test/fixture/entities.go` — add `SortKey` to the six builders
- Test: extend `internal/category/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: `sortkey.Key` from Task 1
- Produces: `model.Category.SortKey sortkey.Key` (and the same field on `Tag`, `Payee`, `Folder`, `BudgetFolder`, `BudgetElement`); `func (c *Category) UpdateSortKey(k sortkey.Key, now time.Time)`; `func (c *Category) SetSortKey(k sortkey.Key)`

Both columns coexist after this task. `ORDER BY` still uses `position`, so behaviour is unchanged — this is pure plumbing, which is why it is one task rather than three.

- [ ] **Step 1: Write the failing test**

Append to `internal/category/repo/repo_integration_test.go`:

```go
// TestCategoryRepo_SortKeyRoundTrip pins that the new ordering column survives a
// save/load cycle on both engines.
func TestCategoryRepo_SortKeyRoundTrip(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)

	c := cat(catA1, userA, "Food", 2, model.TypeExpense)
	c.SetSortKey("c00A")
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(catA1))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SortKey != "c00A" {
		t.Errorf("SortKey = %q, want \"c00A\"", got.SortKey)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/category/repo/ -run TestCategoryRepo_SortKeyRoundTrip`
Expected: FAIL — `c.SetSortKey undefined`.

- [ ] **Step 3: Add the model field and mutators**

In `internal/model/category.go`, add the import `"github.com/econumo/econumo/internal/shared/sortkey"`, add the field to the `Category` struct after `Position`, and add the two mutators next to `SetPosition`/`UpdatePosition`:

```go
	// SortKey is the fractional index key that decides this category's slot in
	// the owner's list. Position is retained until every read path has moved
	// over; see docs/superpowers/specs/2026-08-02-fractional-sort-keys-design.md.
	SortKey sortkey.Key
```

```go
// SetSortKey sets the initial sort key at creation. Like SetPosition it does not
// bump UpdatedAt, because it is part of construction.
func (c *Category) SetSortKey(k sortkey.Key) { c.SortKey = k }

func (c *Category) UpdateSortKey(k sortkey.Key, now time.Time) {
	if c.SortKey != k {
		c.SortKey = k
		c.UpdatedAt = now
	}
}
```

Repeat the identical field and the two mutators (renaming the receiver and type) on:
- `model.Tag` in `internal/model/tag.go`
- `model.Payee` in `internal/model/payee.go`
- `model.Folder` in `internal/model/account_folder.go`
- `model.BudgetFolder` in `internal/model/budget.go`
- `model.BudgetElement` in `internal/model/budget.go`

`model.Account` gets **no** field — an account's position is per-user and lives in `accounts_options`.

- [ ] **Step 4: Add `sort_key` to the SQL**

In each of `internal/infra/storage/sqlc/query/sqlite/{categories,tags,payees,folders,accounts,budgets}.sql` and the `pgsql` siblings, plus the `*_read.sql` files:

1. Add `sort_key` to every `SELECT` column list that already names `position`.
2. Add `sort_key` to each upsert's column list, its `VALUES` placeholder list, and its `ON CONFLICT DO UPDATE SET` block (`sort_key = excluded.sort_key`).
3. Leave every `ORDER BY position, id` **unchanged** — reads flip per feature in Tasks 5-10.

Keep all comments ASCII-only.

- [ ] **Step 5: Regenerate sqlc**

Run: `go generate ./internal/infra/storage/sqlc/...`
Expected: `gen/sqlite/*.sql.go` and `gen/pgsql/*.sql.go` gain a `SortKey string` field on the affected row and param structs. TEXT maps to Go `string` natively on both engines, so no `sqlc.yaml` override is needed.

- [ ] **Step 6: Wire the repos**

In each repo's `hydrate` function set `SortKey: sortkey.Key(row.SortKey)`, and in each `Save` add `SortKey: string(x.SortKey)` to the upsert params. Files:
`internal/category/repo/repo.go`, `internal/tag/repo/repo.go`, `internal/payee/repo/repo.go`, `internal/account/repo/repo.go` (accounts_options), `internal/account/repo/folder.go`, `internal/budget/repo/repo.go`.

Also add `sort_key` to the read-side row structs and their hydration in `internal/{category,tag,payee}/repo/read.go`.

- [ ] **Step 7: Update the fixtures**

In `internal/test/fixture/entities.go`, add a `SortKey string` field to the `Folder`, `Category`, `Tag`, `Payee`, `BudgetElement` and `BudgetFolder` structs, default it to `"c000"` when empty, and add the column to each `INSERT`.

- [ ] **Step 8: Run the suite**

Run: `go build ./... && go test ./...`
Expected: PASS. Behaviour is unchanged — nothing reads `sort_key` yet.

- [ ] **Step 9: Commit**

```bash
git add -A internal/
git commit -m "feat(db): plumb sort_key through model, sqlc and repositories"
```

---

## Task 5: Category — `move-category`

**Files:**
- Create: `internal/category/move.go`
- Delete: `internal/category/order.go`
- Modify: `internal/model/category_dto.go`, `internal/category/{create,read,usecase}.go`, `internal/category/repository.go`, `internal/category/api/{categorylist,routes}.go`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/{categories,category_read}.sql` — `ORDER BY sort_key, id`
- Test: `internal/category/api/move_test.go`

**Interfaces:**
- Consumes: `sortkey.Place`, `sortkey.GrowsUp`, `model.Category.SortKey`
- Produces: `func (s *Service) MoveCategory(ctx context.Context, userID vo.Id, req model.MoveCategoryRequest) (*model.MoveCategoryResult, error)`; `model.MoveCategoryRequest{Id string; AfterId *string}`; `model.MoveCategoryResult{Items []CategoryResult}`

This is the reference implementation for Tasks 6-7.

- [ ] **Step 1: Write the failing test**

Create `internal/category/api/move_test.go` following the harness in `internal/category/api/harness_test.go`:

```go
package api_test

import (
	"net/http"
	"testing"
)

// TestMoveCategory_ToFront moves the last category to the head of the list and
// asserts the response reports dense 0-based positions in the new order.
func TestMoveCategory_ToFront(t *testing.T) {
	h := newHarness(t)
	ids := h.seedCategories(t, "A", "B", "C")

	body := map[string]any{"id": ids[2], "afterId": nil}
	res := h.post(t, "/api/v1/category/move-category", body)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	got := h.itemOrder(t, res)
	want := []string{ids[2], ids[0], ids[1]}
	assertOrder(t, got, want)
	assertDensePositions(t, res)
}

// TestMoveCategory_AfterAnchor places an item directly after a named sibling.
func TestMoveCategory_AfterAnchor(t *testing.T) {
	h := newHarness(t)
	ids := h.seedCategories(t, "A", "B", "C")

	body := map[string]any{"id": ids[0], "afterId": ids[1]}
	res := h.post(t, "/api/v1/category/move-category", body)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	assertOrder(t, h.itemOrder(t, res), []string{ids[1], ids[0], ids[2]})
}

// TestMoveCategory_WritesExactlyOneRow is the whole point of the change.
func TestMoveCategory_WritesExactlyOneRow(t *testing.T) {
	h := newHarness(t)
	ids := h.seedCategories(t, "A", "B", "C", "D", "E")
	before := h.sortKeys(t)

	h.post(t, "/api/v1/category/move-category", map[string]any{"id": ids[4], "afterId": nil})

	after := h.sortKeys(t)
	changed := 0
	for id, k := range after {
		if before[id] != k {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("%d rows changed sort_key, want exactly 1", changed)
	}
}

// TestMoveCategory_UnknownAnchorAppends pins the documented degradation.
func TestMoveCategory_UnknownAnchorAppends(t *testing.T) {
	h := newHarness(t)
	ids := h.seedCategories(t, "A", "B", "C")
	missing := "00000000-0000-0000-0000-0000000000ff"

	res := h.post(t, "/api/v1/category/move-category", map[string]any{"id": ids[0], "afterId": missing})
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unknown anchor appends, never errors)", res.Code)
	}
	assertOrder(t, h.itemOrder(t, res), []string{ids[1], ids[2], ids[0]})
}
```

Add the helpers `seedCategories`, `itemOrder`, `sortKeys`, `assertOrder` and `assertDensePositions` to `internal/category/api/harness_test.go`, matching its existing style.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/category/api/ -run TestMoveCategory -v`
Expected: FAIL — 404, the route does not exist.

- [ ] **Step 3: Replace the DTOs**

In `internal/model/category_dto.go`, delete `PositionChange`, `OrderCategoryListRequest` and `OrderCategoryListResult`, and add:

```go
// MoveCategoryRequest is the move-category request body. AfterId is the id of
// the sibling this category should land immediately after; null means "move to
// the front". An AfterId that is not one of the caller's own categories appends
// to the end rather than erroring (see sortkey.Place).
type MoveCategoryRequest struct {
	Id      string  `json:"id"`
	AfterId *string `json:"afterId"`
}

func (r MoveCategoryRequest) Validate() error {
	if _, err := vo.ParseId(r.Id); err != nil {
		return err
	}
	if r.AfterId != nil {
		if _, err := vo.ParseId(*r.AfterId); err != nil {
			return err
		}
	}
	return nil
}

// MoveCategoryResult is the move-category response: {items: [...]}.
type MoveCategoryResult struct {
	Items []CategoryResult `json:"items"`
}
```

`PositionChange` is shared by the tag and payee order requests, so it can only be deleted once Tasks 6 and 7 land — leave it in place here and delete it in Task 7.

- [ ] **Step 4: Write the use case**

Create `internal/category/move.go`, deleting `internal/category/order.go`:

```go
// Move use case: place one category relative to a sibling.
package category

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveCategory places the category immediately after req.AfterId (nil = first)
// and returns the full available list.
//
// Only the OWNER's categories are repositioned: a shared category belongs to
// another user's list, so a sharee's move silently no-ops (issue #108). The
// RESPONSE is still the full available list (own + shared) via the read view.
func (s *Service) MoveCategory(ctx context.Context, userID vo.Id, req model.MoveCategoryRequest) (*model.MoveCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	var items []model.CategoryResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		cats, lerr := s.repo.ListByOwner(ctx, userID)
		if lerr != nil {
			return lerr
		}
		var moved *model.Category
		siblings := make([]sortkey.Item, 0, len(cats))
		for _, c := range cats {
			if c.ID.Equal(id) {
				moved = c
				continue // never anchor an item against itself
			}
			siblings = append(siblings, sortkey.Item{ID: c.ID.String(), Key: c.SortKey})
		}
		if moved != nil {
			key, perr := sortkey.Place(siblings, afterID, sortkey.GrowsUp)
			if perr != nil {
				return perr
			}
			moved.UpdateSortKey(key, s.clock.Now())
			if serr := s.repo.Save(ctx, moved); serr != nil {
				return serr
			}
		}
		built, berr := s.listResults(ctx, userID)
		if berr != nil {
			return berr
		}
		items = built
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.MoveCategoryResult{Items: items}, nil
}
```

- [ ] **Step 5: Seed the key on create and emit a dense index on read**

In `internal/category/create.go`, replace the `CountByOwner` + `SetPosition` block with:

```go
		cats, lerr := s.repo.ListByOwner(ctx, ownerID)
		if lerr != nil {
			return lerr
		}
		last := sortkey.Key("")
		if n := len(cats); n > 0 {
			last = cats[n-1].SortKey
		}
		key, kerr := sortkey.Place(nil, "", sortkey.GrowsUp)
		if n := len(cats); n > 0 {
			key, kerr = sortkey.Between(last, "")
		}
		if kerr != nil {
			return kerr
		}
		now := s.clock.Now()
		c := model.NewCategory(id, ownerID, name, typ, icon, now)
		c.SetSortKey(key)
```

Remove `CountByOwner` from `internal/category/repository.go`, `internal/category/repo/repo.go` (method, querier entry), `internal/category/repo/{sqlite,pgsql}.go`, and delete the `CountCategoriesByOwner` query from both `categories.sql` files. Drop its assertion from `internal/category/repo/repo_integration_test.go:97`.

In `internal/category/usecase.go` and `internal/category/read.go`, the two `toResult`/`toViewResult` builders currently set `Position: int(...)`. Change `listResults` and `GetCategoryList` to assign the index after building the slice:

```go
	for i := range items {
		items[i].Position = i
	}
```

and have both builders leave `Position` at zero.

- [ ] **Step 6: Flip the SQL ordering**

In `query/{sqlite,pgsql}/categories.sql` and `category_read.sql`, change `ORDER BY position, id` to `ORDER BY sort_key, id` (and `ORDER BY c.position, c.id` to `ORDER BY c.sort_key, c.id`). Run `go generate ./internal/infra/storage/sqlc/...`.

- [ ] **Step 7: Swap the route and handler**

In `internal/category/api/routes.go` replace the `order-category-list` line with:

```go
		mux.Handle("POST /api/v1/category/move-category", auth(h.MoveCategory))
```

In `internal/category/api/categorylist.go` rename `OrderCategoryList` to `MoveCategory`, update the swag block (`@Router /api/v1/category/move-category [post]`, `@Param request body model.MoveCategoryRequest`, `data=model.MoveCategoryResult`), and change the body to `endpoint.Handle(w, r, h.svc.MoveCategory)`.

- [ ] **Step 8: Run the tests**

Run: `go test ./internal/category/... ./internal/shared/sortkey/ -v`
Expected: PASS.

- [ ] **Step 9: Regenerate the affected goldens and inspect**

Run:
```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
git diff internal/test/apiparity/testdata/golden/
```
Expected: `category_reads.golden`, `category_write_read.golden` and others show `position` values collapsing to dense indices. **Any change to the relative order of items is a bug — stop and investigate.** `order_lists.golden` will fail until Task 7 rewrites its scenario; that is expected here.

- [ ] **Step 10: Commit**

```bash
make swagger && git add -A
git commit -m "feat(category): replace order-category-list with move-category"
```

---

## Task 6: Tag — `move-tag`

**Files:** Create `internal/tag/move.go`; delete `internal/tag/order.go`; modify `internal/model/tag_dto.go`, `internal/tag/{create,read,usecase,repository}.go`, `internal/tag/repo/{repo,sqlite,pgsql,read}.go`, `internal/tag/api/{taglist,routes}.go`, `query/{sqlite,pgsql}/{tags,tag_read}.sql`; rename `internal/tag/api/order_shared_test.go` to `move_shared_test.go`; test `internal/tag/api/move_test.go`.

**Interfaces:**
- Consumes: `sortkey.Place`, `sortkey.Between`, `sortkey.GrowsUp`, `model.Tag.SortKey`
- Produces: `func (s *Service) MoveTag(ctx context.Context, userID vo.Id, req model.MoveTagRequest) (*model.MoveTagResult, error)`; `model.MoveTagRequest{Id string; AfterId *string}`; `model.MoveTagResult{Items []TagResult}`

- [ ] **Step 1:** Write `internal/tag/api/move_test.go` with the four cases from Task 5 Step 1 (`ToFront`, `AfterAnchor`, `WritesExactlyOneRow`, `UnknownAnchorAppends`), targeting `/api/v1/tag/move-tag` and seeding tags.
- [ ] **Step 2:** Run `go test ./internal/tag/api/ -run TestMoveTag -v`. Expected: FAIL (404).
- [ ] **Step 3:** In `internal/model/tag_dto.go` delete `OrderTagListRequest`/`OrderTagListResult` and add `MoveTagRequest`/`MoveTagResult` with the same field set and `Validate()` body as `MoveCategoryRequest` in Task 5 Step 3.
- [ ] **Step 4:** Create `internal/tag/move.go` as a copy of Task 5's `internal/category/move.go` with `Category`→`Tag`, `cats`→`tags`, `MoveCategory`→`MoveTag`; delete `internal/tag/order.go`.
- [ ] **Step 5:** In `internal/tag/create.go` replace the `CountByOwner`+`SetPosition` block with the `ListByOwner`+`sortkey.Between(last, "")` block from Task 5 Step 5; remove `CountByOwner` from `internal/tag/repository.go`, `internal/tag/repo/{repo,sqlite,pgsql}.go` and delete `CountTagsByOwner` from both `tags.sql` files. Assign `items[i].Position = i` in the list builders in `internal/tag/read.go` and `internal/tag/usecase.go`.
- [ ] **Step 6:** Change `ORDER BY position, id` to `ORDER BY sort_key, id` in `query/{sqlite,pgsql}/tags.sql` and `tag_read.sql`; run `go generate ./internal/infra/storage/sqlc/...`.
- [ ] **Step 7:** In `internal/tag/api/routes.go` replace the `order-tag-list` line with `mux.Handle("POST /api/v1/tag/move-tag", auth(h.MoveTag))`; rename the handler in `internal/tag/api/taglist.go` and update its swag block.
- [ ] **Step 8:** Rename `internal/tag/api/order_shared_test.go` to `move_shared_test.go` and adapt its two cases (lines 33 and 65) to post `{id, afterId}` to `/api/v1/tag/move-tag`. The assertion is unchanged: a sharee's move must leave the owner's order untouched (issue #108).
- [ ] **Step 9:** Run `go test ./internal/tag/...`. Expected: PASS.
- [ ] **Step 10:** Commit: `git add -A && git commit -m "feat(tag): replace order-tag-list with move-tag"`

---

## Task 7: Payee — `move-payee`

**Files:** the payee mirror of Task 6 — `internal/payee/move.go`, delete `internal/payee/order.go`, `internal/model/payee_dto.go`, `internal/payee/{create,read,usecase,repository}.go`, `internal/payee/repo/*`, `internal/payee/api/{payeelist,routes}.go`, `query/{sqlite,pgsql}/{payees,payee_read}.sql`, rename `internal/payee/api/order_shared_test.go`, test `internal/payee/api/move_test.go`.

**Interfaces:**
- Produces: `func (s *Service) MovePayee(ctx context.Context, userID vo.Id, req model.MovePayeeRequest) (*model.MovePayeeResult, error)`; `model.MovePayeeRequest{Id string; AfterId *string}`; `model.MovePayeeResult{Items []PayeeResult}`

- [ ] **Step 1-9:** Identical to Task 6 Steps 1-9 with `Tag`→`Payee`, `tags`→`payees`, route `/api/v1/payee/move-payee`.
- [ ] **Step 10: Delete the now-unused shared DTO.** `PositionChange` in `internal/model/category_dto.go:170` had three consumers (category, tag, payee) and now has none. Delete it. Run `go build ./...` to confirm.
- [ ] **Step 11: Rewrite the apiparity scenario.** Delete `internal/test/apiparity/catalogue_orderlists.go` and `testdata/golden/order_lists.golden`; create `internal/test/apiparity/catalogue_moves.go`:

```go
package apiparity

// move_lists exercises the move-{category,tag,payee} routes: one relative move
// per module, each followed by a read that must reflect the new order. A move
// writes exactly one row, so the closing read is what proves the surviving rows
// still sort correctly around it.
func init() {
	register(Scenario{Name: "move_lists", Calls: func() []Call {
		return []Call{
			{Label: "move-category", Method: "POST", Path: "/api/v1/category/move-category", Auth: "owner",
				Body: map[string]any{"id": CatFood, "afterId": CatSalary}},
			{Label: "get-category-list-after", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner"},
			{Label: "move-tag", Method: "POST", Path: "/api/v1/tag/move-tag", Auth: "owner",
				Body: map[string]any{"id": TagWork, "afterId": nil}},
			{Label: "move-payee", Method: "POST", Path: "/api/v1/payee/move-payee", Auth: "owner",
				Body: map[string]any{"id": PayeeShop, "afterId": nil}},
		}
	}})
}
```

The scenario count is unchanged (one out, one in), so `catalogue_test.go`'s `min = 36` stays. Route count is unchanged too (three out, three in), so `guard_test.go`'s `minRoutes = 101` stays.

- [ ] **Step 12:** Run `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, then `git diff internal/test/apiparity/testdata/golden/` and inspect. Confirm `order_lists.golden` is gone and `move_lists.golden` exists.
- [ ] **Step 13:** Run `go test ./...`. Expected: PASS.
- [ ] **Step 14:** Commit: `make swagger && git add -A && git commit -m "feat(payee): replace order-payee-list with move-payee"`

---

## Task 8: Accounts and account folders

**Files:** Create `internal/account/move.go`; delete `internal/account/order.go`; modify `internal/account/folder_usecase.go` (replace `OrderFolderList`, delete `resetFolderPositions`), `internal/account/{create,read,usecase,repository}.go`, `internal/account/repo/{repo,folder}.go`, `internal/account/api/{account,folder,routes}.go`, `internal/model/account_dto.go`, `query/{sqlite,pgsql}/{accounts,folders}.sql`; test `internal/account/api/move_test.go`.

**Interfaces:**
- Produces:
  - `func (s *Service) MoveAccount(ctx context.Context, userID vo.Id, req model.MoveAccountRequest) (*model.MoveAccountResult, error)`
  - `func (s *Service) MoveFolder(ctx context.Context, userID vo.Id, req model.MoveAccountFolderRequest) (*model.MoveAccountFolderResult, error)`
  - `model.MoveAccountRequest{Id string; AfterId *string; FolderId *string}`
  - `model.MoveAccountFolderRequest{Id string; AfterId *string}`

Accounts differ from classifications in three ways: the key lives in `accounts_options` (per-user, so `PositionStore.SavePosition` becomes `SaveSortKey`), a move also rewrites folder membership, and the response list must be rebuilt **inside** the transaction — `internal/account/order.go:93` currently rebuilds outside it.

- [ ] **Step 1:** Write `internal/account/api/move_test.go` with the four Task 5 cases against `/api/v1/account/move-account`, plus two account-specific cases: `TestMoveAccount_ChangesFolder` (asserts the account leaves its old folder and joins `folderId`) and `TestMoveAccount_NullFolderRemovesFromFolder`. Add the four Task 5 cases again for `/api/v1/account/move-folder`.
- [ ] **Step 2:** Run `go test ./internal/account/api/ -run TestMove -v`. Expected: FAIL (404).
- [ ] **Step 3:** In `internal/model/account_dto.go` delete `AccountPositionChange`, `OrderAccountListRequest`, `FolderPositionChange`, `OrderFolderListRequest` and their `Result` types; add `MoveAccountRequest`, `MoveAccountResult{Items []AccountResult}`, `MoveAccountFolderRequest`, `MoveAccountFolderResult{Items []AccountFolderResult}` with `Validate()` bodies parsing `Id`, and `AfterId`/`FolderId` when non-nil.
- [ ] **Step 4:** In `internal/account/repository.go` replace `PositionStore.SavePosition`/`GetPosition`/`MaxPosition` with `SaveSortKey(ctx, accountID, userID vo.Id, key sortkey.Key, now time.Time) error` and `SortKeysByUser(ctx, userID vo.Id) (map[string]sortkey.Key, error)`. Implement them in `internal/account/repo/repo.go` over the existing `UpsertAccountOption` / `ListAccountOptionsByUser` queries. Delete `MaxPosition` (`repo.go:155`) — it was a full Go-side scan used only to seed a position.
- [ ] **Step 5:** Create `internal/account/move.go`. Inside one `WithTx`: load `accounts.ListAvailable`, `folders.ListByUser`, `memberships.MembershipsByUser` and `SortKeysByUser`; if the moved account is not in the available set, return the rebuilt list unchanged (matching today's silent skip at `order.go:62`); otherwise reconcile folder membership exactly as `internal/account/order.go:62-82` does (add to the target folder, remove from every other), compute the key with `sortkey.Place(siblings, afterID, sortkey.GrowsUp)`, call `SaveSortKey`, and rebuild the response via `buildAccountList` **inside** the transaction. Delete `internal/account/order.go`.
- [ ] **Step 6:** In `internal/account/folder_usecase.go` replace `OrderFolderList` (`:220-259`) with `MoveFolder` using the Task 5 shape over `folders.ListByUser`; delete `resetFolderPositions` (`:202-216`) and its call site in the delete path (`:193`); replace the create-time `max(existing.Position)+1` (`:45-55`) with `sortkey.Between(lastKey, "")`.
- [ ] **Step 7:** In `internal/account/create.go` (`:65-87`) replace the `MaxPosition`+fallback block with `sortkey.Between(lastKey, "")` over `SortKeysByUser`.
- [ ] **Step 8:** Assign dense indices: in `internal/account/usecase.go` set `items[i].Position = i` after the account list is sorted (replacing the per-account lookups at `:147-163`), and likewise for folders at `:364`. In `internal/account/read.go:97`, `usecase.go:313` and `folder_usecase.go:207,250` change the `sort.SliceStable` comparators from `Position` to `SortKey`, **adding an `ID` tie-break** — those three sites have none today.
- [ ] **Step 9:** Change `ORDER BY position` to `ORDER BY sort_key` in `query/{sqlite,pgsql}/{accounts,folders}.sql`; run `go generate ./internal/infra/storage/sqlc/...`.
- [ ] **Step 10:** In `internal/account/api/routes.go` replace the two order routes with `mux.Handle("POST /api/v1/account/move-account", auth(h.MoveAccount))` and `mux.Handle("POST /api/v1/account/move-folder", auth(h.MoveFolder))`; rename the handlers in `account.go:106` and `folder.go:139` and update their swag blocks.
- [ ] **Step 11:** Run `go test ./internal/account/...`. Expected: PASS.
- [ ] **Step 12:** Add the two account moves to `internal/test/apiparity/catalogue_moves.go`, regenerate goldens, inspect the diff.
- [ ] **Step 13:** Commit: `make swagger && git add -A && git commit -m "feat(account): replace order-account-list and order-folder-list with move routes"`

---

## Task 9: Budget folders — `move-folder`

**Files:** Create `internal/budget/move_folder.go`; modify `internal/budget/folders.go` (delete both renumber loops), `internal/model/budget_dto.go`, `internal/budget/api/{budget_more,routes}.go`, `internal/budget/builder.go`, `query/{sqlite,pgsql}/budgets.sql`; test `internal/budget/api/move_folder_test.go`.

**Interfaces:**
- Produces: `func (s *Service) MoveFolder(ctx context.Context, userID vo.Id, req model.MoveBudgetFolderRequest) (*model.MoveBudgetFolderResult, error)`; `model.MoveBudgetFolderRequest{BudgetId string; Id string; AfterId *string}`; `model.MoveBudgetFolderResult struct{}`

Budget folders **prepend** on create, so they use `sortkey.GrowsDown`.

- [ ] **Step 1:** Write `internal/budget/api/move_folder_test.go` with the four Task 5 cases against `/api/v1/budget/move-folder`, plus `TestMoveFolder_ForeignBudgetIsForbidden` asserting 403 for a non-`canUpdate` caller.
- [ ] **Step 2:** Run `go test ./internal/budget/api/ -run TestMoveFolder -v`. Expected: FAIL (404).
- [ ] **Step 3:** In `internal/model/budget_dto.go` delete `OrderFolderListItem` and `OrderBudgetFolderListRequest` (`:235-249`); add `MoveBudgetFolderRequest` with a `Validate()` that parses `BudgetId` and `Id`, and `AfterId` when non-nil. This request previously validated only `budgetId`, so out-of-range positions truncated silently — the new shape has no position to truncate.
- [ ] **Step 4:** Create `internal/budget/move_folder.go` using the Task 5 shape over `ListFolders`, gated on the existing `canUpdate` check, with `sortkey.GrowsDown`. Unlike `internal/budget/folders.go:153-187` it saves only the moved row (that use case saved every row unconditionally).
- [ ] **Step 5:** In `internal/budget/folders.go` replace the `CreateFolder` renumber loop (`:33-57`) with `sortkey.Between("", firstKey)` — or `sortkey.Seed(sortkey.GrowsDown)` when the budget has no folders — preserving today's front-insert. Delete the `DeleteFolder` renumber block (`:126-145`) entirely; removing a row no longer disturbs its siblings.
- [ ] **Step 6:** In `internal/budget/builder.go:205-214` rename `sortByPositionThenID` to `sortBySortKeyThenID` and compare `SortKey`. Assign `items[i].Position = i` where `BudgetFolderResult` is built.
- [ ] **Step 7:** Change `ORDER BY position ASC, id ASC` to `ORDER BY sort_key ASC, id ASC` in `query/{sqlite,pgsql}/budgets.sql` (the `ListFolders` query); run `go generate ./internal/infra/storage/sqlc/...`.
- [ ] **Step 8:** In `internal/budget/api/routes.go:26` replace `order-folder-list` with `mux.Handle("POST /api/v1/budget/move-folder", auth(h.MoveFolder))`; rename the handler at `budget_more.go:83` and update its swag block.
- [ ] **Step 9:** Run `go test ./internal/budget/...`. Expected: PASS.
- [ ] **Step 10:** Commit: `make swagger && git add -A && git commit -m "feat(budget): replace order-folder-list with move-folder"`

---

## Task 10: Budget elements — `move-element` and `syncElements`

**Files:** Modify `internal/budget/move.go` (rewrite `MoveElementList` → `MoveElement`, shrink `restoreElementsOrder` → `syncElements`, delete `shiftElements`), `internal/budget/{envelopes,accounts}.go`, `internal/model/budget_dto.go`, `internal/budget/{builder,builder_structure_build}.go`, `internal/budget/api/{budget_more,routes}.go`, `query/{sqlite,pgsql}/budgets.sql`; test `internal/budget/api/move_element_test.go`.

**Interfaces:**
- Produces: `func (s *Service) MoveElement(ctx context.Context, userID vo.Id, req model.MoveElementRequest) (*model.MoveElementResult, error)`; `model.MoveElementRequest{BudgetId string; Id string; FolderId *string; AfterId *string}`; `model.MoveElementResult struct{}`

This is the hardest task. `restoreElementsOrder` (`internal/budget/move.go:122-292`) does four things; only one of them is ordering. Keep the other three.

- [ ] **Step 1:** Write `internal/budget/api/move_element_test.go` covering: move within a folder; move to a different folder; move to no folder (`folderId: null`); unknown anchor appends; a foreign folder returns 403; and `TestMoveElement_ArchivedElementsStayUnset` asserting an archived element keeps the empty key and no folder after an unrelated move.
- [ ] **Step 2:** Run `go test ./internal/budget/api/ -run TestMoveElement -v`. Expected: FAIL (404).
- [ ] **Step 3:** In `internal/model/budget_dto.go` delete `MoveElementListItem` and `MoveElementListRequest` (`:426-452`) and add `MoveElementRequest` with a `Validate()` parsing `BudgetId` and `Id`, plus `FolderId`/`AfterId` when non-nil. Delete `validatePositionField` (`:501-515`) and `internal/model/position_validation_test.go` — nothing calls them once every `Order*Request` is gone. Delete `model.PositionUnset` (`budget.go:14-15`) and replace `IsPositionUnset()` with `IsSortKeyUnset() bool { return e.SortKey == "" }`.
- [ ] **Step 4:** Rewrite `MoveElementList` as `MoveElement` in `internal/budget/move.go`: resolve the element by external id (first-seen wins, as at `:29-35`), reject a foreign folder with 403, set the folder, compute the key with `sortkey.Place` over the siblings **in the target folder** using `sortkey.GrowsDown`, save one row, then call `syncElements` in the same transaction.
- [ ] **Step 5:** Shrink `restoreElementsOrder` into `syncElements`. **Delete** the `sort.SliceStable` block and the `renumber` closure (`:~250-282`) and the `posMax = 0x7fff` sentinel (`:111`). **Keep**: creating missing `budgets_elements` rows for participating envelopes / non-income categories / tags, forcing archived and envelope-child rows to the empty key and no folder, and pruning rows whose entity no longer participates (`:283-290`). Missing rows are created with `sortkey.Between(lastKeyInScope, "")` instead of the sentinel. Update the four call sites (`envelopes.go:76,161,204`, `accounts.go:195`) to the new name.
- [ ] **Step 6:** Delete `shiftElements` (`internal/budget/move.go:76-106`) and change its only caller, envelope create (`internal/budget/envelopes.go:43-63`), from "position 0 + shift" to `sortkey.Between("", firstKeyInFolder)` — or `sortkey.Seed(sortkey.GrowsDown)` when the folder is empty.
- [ ] **Step 7:** In `internal/budget/builder_structure_build.go` delete `uncategorizedPosition = math.MaxInt16` (`:14-17`); the synthetic "Uncategorized" element is appended after the real ones and receives the next dense index. Assign `Position = i` per folder group (and per the no-folder group) where `ParentElementResult` is built. `ChildElementResult` has no position and stays as it is.
- [ ] **Step 8:** Change the element query's `ORDER BY` to `sort_key, id` in `query/{sqlite,pgsql}/budgets.sql`; run `go generate ./internal/infra/storage/sqlc/...`.
- [ ] **Step 9:** In `internal/budget/api/routes.go:41` replace `move-element-list` with `mux.Handle("POST /api/v1/budget/move-element", auth(h.MoveElement))`; rename the handler at `budget_more.go:285` and update its swag block.
- [ ] **Step 10:** Run `go test ./internal/budget/...`. Expected: PASS.
- [ ] **Step 11:** Add a `move-element` call to `catalogue_moves.go`, regenerate apiparity goldens, inspect. `budget_structure_writes.golden` and `budget_uncategorized_element.golden` will change — confirm the Uncategorized element is still **last**, only its `position` value differs.
- [ ] **Step 12:** Commit: `make swagger && git add -A && git commit -m "feat(budget): replace move-element-list with move-element; syncElements no longer renumbers"`

---

## Task 11: MCP — `move_element`

**Files:** Modify `internal/budget/mcp/mcp.go:89-97,326-352`, `internal/budget/mcp/mcp_test.go:439,512`, `internal/test/mcpparity/catalogue.go:117-156`.

**Interfaces:**
- Consumes: `model.MoveElementRequest` from Task 10
- Produces: the `move_element` tool with input `{budget_id, element_id, folder_id?, after_element_id?}`; `moveElementResult{budget_id, element_ids}` unchanged

- [ ] **Step 1:** Update the two unit tests at `internal/budget/mcp/mcp_test.go:439` and `:512` to the singular argument shape. Run `go test ./internal/budget/mcp/`. Expected: FAIL.
- [ ] **Step 2:** Replace `moveElementInput` (`mcp.go:89-97`) with a flat struct carrying `budget_id`, `element_id`, `folder_id` (`*string`), `after_element_id` (`*string`), and map it onto `model.MoveElementRequest` at `mcp.go:334-344`.
- [ ] **Step 3:** Run `go test ./internal/budget/mcp/`. Expected: PASS.
- [ ] **Step 4:** Update the `move-element` step in `internal/test/mcpparity/catalogue.go:117-156` to the new arguments, keeping the `{{budget_id}}`/`{{element_id}}`/`{{folder_id}}` capture slots.
- [ ] **Step 5:** Run `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`, then inspect `git diff internal/test/mcpparity/testdata/golden/`. Both `budget_write.golden` (the call itself) and `lifecycle.golden` (the `tools/list` input schema) change.
- [ ] **Step 6:** Commit: `git add -A && git commit -m "feat(mcp): move_element takes a single relative move"`

---

## Task 12: Frontend

**Files:** Modify `web/src/lib/ordering.ts`, `web/src/lib/ordering.test.ts`, `web/src/features/classifications/{ClassificationList.tsx,queries.ts}`, `web/src/features/accounts/{accountOrdering.ts,AccountsSettingsPage.tsx,queries.ts}`, `web/src/features/budgets/{elementMove.ts,BudgetPage.tsx,queries.ts}`, `web/src/api/{category,tag,payee,account,budget}.ts`, `web/src/api/dto/*.ts`.

**Interfaces:**
- Consumes: the seven `move-*` routes
- Produces: `afterIdFromDrop(orderedIds: string[], movedId: string): string | null`; `moveCategory(id, afterId)`, `moveTag`, `movePayee`, `moveAccount(id, afterId, folderId)`, `moveAccountFolder`, `moveBudgetFolder(budgetId, id, afterId)`, `moveBudgetElement(budgetId, id, folderId, afterId)`

- [ ] **Step 1:** Replace `web/src/lib/ordering.test.ts` with cases for `afterIdFromDrop`: first position returns `null`; middle position returns the preceding id; an id absent from the list returns `null`.
- [ ] **Step 2:** Run `cd web && pnpm test ordering`. Expected: FAIL.
- [ ] **Step 3:** Replace `getChangedPositions` in `web/src/lib/ordering.ts` with:

```ts
// The anchor a relative move needs: the id the dragged item now sits after, or
// null when it landed first. The server derives the sort key from this, so the
// client no longer computes or diffs positions.
export function afterIdFromDrop(orderedIds: string[], movedId: string): string | null {
  const i = orderedIds.indexOf(movedId)
  return i > 0 ? orderedIds[i - 1] : null
}
```

- [ ] **Step 4:** Rewrite the API clients. Each `order*List(changes)` becomes a single-move call, e.g. in `web/src/api/category.ts`:

```ts
export async function moveCategory(id: Id, afterId: Id | null): Promise<CategoryDto[]> {
  const response = await api.post<Envelope<{ items: CategoryDto[] }>>(apiUrl('/api/v1/category/move-category'), { id, afterId })
  return response.data.data.items
}
```

- [ ] **Step 5:** In `ClassificationList.tsx:186-201` delete `rebuildFullOrder` and `commitOrder`; the drag handler now calls `onMove({ id: movedId, afterId: afterIdFromDrop(rebuiltIds, movedId) })`. Keep the existing rebuild of the full id order — it is still needed to translate a within-section drag into a whole-list anchor — but it no longer diffs.
- [ ] **Step 6:** In `web/src/features/classifications/queries.ts:156,211,266` rename the hooks to `useMoveCategory`/`useMovePayee`/`useMoveTag`, keep `ops.replaceAll(items)` and keep the **existing** `METRICS` keys.
- [ ] **Step 7:** In `web/src/features/accounts/accountOrdering.ts` delete `buildAccountChanges`; keep `bucketsFromAccounts` and `moveAccount` for dnd-kit bucketing but drop the flat cross-folder position sequence. Update `AccountsSettingsPage.tsx:289-290,374,408` and the optimistic mutations in `queries.ts:124-153,176-205`.
- [ ] **Step 8:** In `web/src/features/budgets/elementMove.ts` reduce `computeElementMove` to producing `{id, folderId, afterId}`. Attempt to delete the `arrangement` scaffolding (`arrangementFromBuckets`, `moveElementInArrangement`, `arrangementItem`) and replace it with a normal optimistic update in `queries.ts:259-269` — it existed only to stop the table snapping back during server renumbering. **If the table still flickers**, keep the scaffolding and note why; do not force the deletion.
- [ ] **Step 9:** Update `web/src/api/dto/{account,budget,category,folder,payee,tag}.ts` request types. `position: number` stays on every **result** type.
- [ ] **Step 10:** Run `cd web && pnpm exec tsc -b && pnpm test && pnpm lint`. Expected: all PASS. `tsc -b` is the gate that matters — vitest and oxlint do not type-check.
- [ ] **Step 11:** Commit: `git add -A && git commit -m "feat(web): drive reordering with relative moves"`

---

## Task 13: Drop `position` and final verification

**Files:** Modify `internal/infra/storage/migrations/{sqlite,pgsql}/20260803000000.sql`, the six model entities, `internal/test/fixture/entities.go`, every `query/**/*.sql` still naming `position`, and any remaining `Position int16` references.

- [ ] **Step 1:** Append the seven `DROP COLUMN` statements to **both** migration files (the migration is unreleased, so editing it is correct — do not add a second migration):

```sql
ALTER TABLE categories        DROP COLUMN position;
ALTER TABLE tags              DROP COLUMN position;
ALTER TABLE payees            DROP COLUMN position;
ALTER TABLE folders           DROP COLUMN position;
ALTER TABLE accounts_options  DROP COLUMN position;
ALTER TABLE budgets_folders   DROP COLUMN position;
ALTER TABLE budgets_elements  DROP COLUMN position;
```

`position` is not indexed anywhere, so sqlite's `ALTER TABLE ... DROP COLUMN` works without a table rebuild. This also removes pgsql's `budgets_folders CHECK (position >= 0)` and the `SMALLINT UNSIGNED` engine skew.

- [ ] **Step 2:** Remove `position` from every remaining SELECT list and upsert in `query/{sqlite,pgsql}/*.sql`; run `go generate ./internal/infra/storage/sqlc/...`.
- [ ] **Step 3:** Delete the `Position int16` field, `SetPosition` and `UpdatePosition` from `model.Category`, `Tag`, `Payee`, `Folder`, `BudgetFolder` and `BudgetElement`. Delete the `Position` field from the six fixture builders. Fix the resulting compile errors — `go build ./...` names every one.
- [ ] **Step 4:** Run `grep -rn "\.Position\b" internal/ --include=*.go | grep -v "_dto.go"`. Every remaining hit should be a `Result` DTO assignment (`items[i].Position = i`). Anything else is a missed read path.
- [ ] **Step 5:** Run the migration tests: `go test ./internal/infra/storage/migrations/`. Expected: PASS — the Task 3 tests seed via `position` **before** the migration runs, so they are unaffected by the drop.
- [ ] **Step 6:** Run the full smoke suite: `make go-test`. Expected: PASS including the 80% coverage gate and the swagger-freshness check.
- [ ] **Step 7:** Regenerate every golden one final time and inspect:

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/
git diff --stat internal/test/apiparity/testdata/golden/ internal/test/mcpparity/testdata/golden/
```

Expected: no further changes — the per-task regenerations already captured everything. **A diff here means a read path was still using `position` at the time its golden was last written.**

- [ ] **Step 8:** Run the engine-parity suite, which is the only check that proves `COLLATE "C"` works:

```bash
make test
```

Expected: PASS, including `test-engines` (byte-identical sqlite vs PostgreSQL) and `test-repo-pgsql`. **If the two engines disagree on ordering, the pgsql column is missing `COLLATE "C"`** — that is the failure this whole design most needs to catch.

- [ ] **Step 9:** Commit: `git add -A && git commit -m "feat(db): drop the position column"`

---

## Self-Review

**Spec coverage.** §1 storage → Tasks 3, 4, 13. §1 `sortkey` package and seed policy → Tasks 1, 2. §2 routes, requests, dense-index responses → Tasks 5-10. §2 MCP → Task 11. §3 migration and backfill → Tasks 3, 13. §4 deletions → Tasks 5-10, 13. §4 `syncElements` → Task 10. §5 frontend → Task 12. §6 testing → the test step of every task plus Task 13 Steps 5-8. §7 build order → the task order. §8 rollout → release-note item below, not code.

**Known gaps deliberately left to the implementer:**
- Task 12 Step 8 leaves the budget `arrangement` deletion conditional on observed behaviour rather than asserting it can go. The spec flagged this as "verify during implementation rather than assuming".
- Task 8 Step 5 describes the folder-membership reconciliation rather than repeating `internal/account/order.go:62-82`; that block is transplanted verbatim, so copying it into the plan would risk it drifting from the source.

**Release note (Task 13 follow-up, not a code task):** breaking API change — seven `order-*-list` routes removed, `position` dropped, no down migration. Needs a minor version bump and a "back up your database first" line.

