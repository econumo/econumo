package vo

import (
	"bytes"
	"encoding/json"
)

// FlexString is a decimal-string request field that decodes from JSON as either
// a string OR a number.
//
// The frozen wire contract treats money fields (amount, amountRecipient,
// balance, the budget limit) as normalized decimal strings, but it was set when
// scalars deserialized leniently, so numeric bodies were always accepted and
// third-party clients may still send them. FlexString keeps that leniency and
// records which form arrived (FromNumber), so the HTTP edge can log the
// deprecated numeric form without rejecting it.
//
// A JSON number is captured VERBATIM (its source bytes), not via float parsing,
// so no precision is lost — 123.45 stays "123.45". The captured value flows
// into NewDecimal downstream, which normalizes plain and scientific forms.
type FlexString struct {
	value      string
	fromNumber bool
}

// NewFlexString builds a FlexString holding s (for tests and fixtures).
func NewFlexString(s string) FlexString { return FlexString{value: s} }

// UnmarshalJSON accepts a JSON string, a JSON number, or null (-> ""). For a
// quoted string the quotes are stripped; any other scalar is captured verbatim
// and flagged as numeric.
func (s *FlexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*s = FlexString{}
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		s.value = str
		return nil
	}
	s.value = string(b)
	s.fromNumber = true
	return nil
}

// MarshalJSON renders the canonical form: always a JSON string.
func (s FlexString) MarshalJSON() ([]byte, error) { return json.Marshal(s.value) }

// String returns the underlying string.
func (s FlexString) String() string { return s.value }

// FromNumber reports whether the value decoded from a JSON number (the
// deprecated lenient form) rather than a string.
func (s FlexString) FromNumber() bool { return s.fromNumber }

// StrPtr dereferences a *FlexString to a *string, preserving nil.
func (s *FlexString) StrPtr() *string {
	if s == nil {
		return nil
	}
	v := s.value
	return &v
}
