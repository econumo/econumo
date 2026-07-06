package vo

import (
	"encoding/json"
	"testing"
)

// TestFlexStringUnmarshal locks the string-or-number wire behavior: the frontend
// posts money fields as JSON numbers, the contract is decimal strings, and both
// must decode. Numbers are captured verbatim (no float rounding).
func TestFlexStringUnmarshal(t *testing.T) {
	type wrap struct {
		A FlexString  `json:"a"`
		B *FlexString `json:"b"`
	}
	cases := []struct {
		name, body, wantA string
		wantBNil          bool
		wantB             string
	}{
		{"string", `{"a":"123.45","b":"7"}`, "123.45", false, "7"},
		{"int number", `{"a":123,"b":99}`, "123", false, "99"},
		{"float number", `{"a":123.45,"b":0.5}`, "123.45", false, "0.5"},
		{"negative", `{"a":-42.5,"b":-1}`, "-42.5", false, "-1"},
		{"null pointer", `{"a":0,"b":null}`, "0", true, ""},
		{"absent pointer", `{"a":5}`, "5", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var w wrap
			if err := json.Unmarshal([]byte(c.body), &w); err != nil {
				t.Fatalf("unmarshal %s: %v", c.body, err)
			}
			if w.A.String() != c.wantA {
				t.Fatalf("A = %q, want %q", w.A, c.wantA)
			}
			if c.wantBNil {
				if w.B != nil {
					t.Fatalf("B = %v, want nil", *w.B)
				}
			} else if w.B == nil || w.B.String() != c.wantB {
				t.Fatalf("B = %v, want %q", w.B, c.wantB)
			}
		})
	}
}

// TestFlexStringStrPtr covers the nil-safe *FlexString -> *string helper.
func TestFlexStringStrPtr(t *testing.T) {
	var nilPtr *FlexString
	if nilPtr.StrPtr() != nil {
		t.Fatal("nil StrPtr() should be nil")
	}
	v := FlexString("42")
	if got := v.StrPtr(); got == nil || *got != "42" {
		t.Fatalf("StrPtr() = %v, want \"42\"", got)
	}
}
