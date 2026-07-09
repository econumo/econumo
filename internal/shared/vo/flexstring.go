package vo

import (
	"bytes"
	"encoding/json"
)

// FlexString is a string that decodes from JSON as either a string OR a number.
//
// Existing web clients send money fields (amount, amountRecipient, balance, the
// budget limit) as JSON numbers — it runs Number(...) over them before posting —
// while the API contract treats those fields as normalized decimal strings. The
// frozen contract was set when scalars deserialized leniently, so a numeric body
// Just Worked; Go's strict encoding/json rejects a number decoded into a plain
// string field. FlexString restores the lenient behavior.
//
// A JSON number is captured VERBATIM (its source bytes), not via float parsing,
// so no precision is lost — "123.45" stays "123.45". The captured value flows
// into NewDecimal downstream, which already normalizes plain and scientific
// forms to the canonical decimal shape.
type FlexString string

// UnmarshalJSON accepts a JSON string, a JSON number, or null (-> ""). For a
// quoted string the quotes are stripped; any other scalar is captured verbatim.
func (s *FlexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = FlexString(str)
		return nil
	}
	// JSON number (or other bare scalar) — keep the literal as-is.
	*s = FlexString(b)
	return nil
}

// String returns the underlying string.
func (s FlexString) String() string { return string(s) }

// StrPtr dereferences a *FlexString to a *string, preserving nil. Convenient
// for passing an optional FlexString field where a *string is expected.
func (s *FlexString) StrPtr() *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}
