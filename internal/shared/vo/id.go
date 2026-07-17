// Package vo holds shared domain value objects: typed wrappers that make
// invalid states unrepresentable and carry their own validation. They have no
// framework or persistence dependencies.
package vo

import (
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/google/uuid"
)

// Id is a UUID-backed identifier, stored and transported as the canonical
// lowercase hyphenated string form (matching the DB's CHAR(36)/TEXT columns and
// the JSON contract). The zero Id is invalid; construct via NewId/ParseId.
type Id struct {
	value string
}

// NewId generates a new time-ordered (UUIDv7) Id. The v7 timestamp prefix keeps
// B-tree index inserts local instead of scattering like random v4, giving
// consistent index growth. Existing rows keep their original (mostly v4) ids —
// only newly created ids are v7, which is safe because ids are FK targets held
// by clients that must not change for existing data. Falls back to v4 only if
// the OS entropy source fails (NewV7 error), which is effectively never.
func NewId() Id {
	if u, err := uuid.NewV7(); err == nil {
		return Id{value: u.String()}
	}
	return Id{value: uuid.NewString()}
}

// ParseId validates s as a UUID and returns the corresponding Id. An invalid
// UUID yields a ValidationError so it surfaces as a 400 rather than a 500.
func ParseId(s string) (Id, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return Id{}, &errs.ValidationError{Msg: "invalid id", MsgCode: errs.CodeInvalidID}
	}
	// Normalize to canonical lowercase string form.
	return Id{value: u.String()}, nil
}

// MustParseId is ParseId that panics on error; for tests and trusted constants.
// (Used by test fixtures and seed data across modules.)
func MustParseId(s string) Id {
	id, err := ParseId(s)
	if err != nil {
		panic(err)
	}
	return id
}

// String returns the canonical UUID string.
func (id Id) String() string { return id.value }

// Value returns the canonical UUID string.
func (id Id) Value() string { return id.value }

// IsZero reports whether the Id is the uninitialized zero value.
func (id Id) IsZero() bool { return id.value == "" }

// Equal reports whether two Ids are the same.
func (id Id) Equal(other Id) bool { return id.value == other.value }

// MarshalJSON encodes the Id as a bare JSON string.
func (id Id) MarshalJSON() ([]byte, error) {
	return []byte(`"` + id.value + `"`), nil
}
