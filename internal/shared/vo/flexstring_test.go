package vo

import (
	"encoding/json"
	"testing"
)

func TestFlexString_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		want       string
		fromNumber bool
	}{
		{"string", `"123.45"`, "123.45", false},
		{"number verbatim", `123.45`, "123.45", true},
		{"integer number", `100`, "100", true},
		{"scientific number verbatim", `1.5e3`, "1.5e3", true},
		{"null", `null`, "", false},
		{"empty string", `""`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s FlexString
			if err := json.Unmarshal([]byte(c.in), &s); err != nil {
				t.Fatalf("unmarshal %s: %v", c.in, err)
			}
			if s.String() != c.want {
				t.Errorf("String() = %q, want %q", s.String(), c.want)
			}
			if s.FromNumber() != c.fromNumber {
				t.Errorf("FromNumber() = %v, want %v", s.FromNumber(), c.fromNumber)
			}
		})
	}
}

func TestFlexString_UnmarshalJSON_ResetsPriorState(t *testing.T) {
	var s FlexString
	if err := json.Unmarshal([]byte(`100`), &s); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`"200"`), &s); err != nil {
		t.Fatal(err)
	}
	if s.FromNumber() {
		t.Error("FromNumber() should reset to false on a string decode")
	}
}

func TestFlexString_MarshalJSON(t *testing.T) {
	var s FlexString
	if err := json.Unmarshal([]byte(`123.45`), &s); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"123.45"` {
		t.Errorf("MarshalJSON = %s, want %q", b, `"123.45"`)
	}
}

func TestFlexString_StrPtr(t *testing.T) {
	if (*FlexString)(nil).StrPtr() != nil {
		t.Error("nil receiver should map to nil")
	}
	s := NewFlexString("9.99")
	if got := s.StrPtr(); got == nil || *got != "9.99" {
		t.Errorf("StrPtr() = %v, want 9.99", got)
	}
}

// TestFlexString_UnmarshalJSON_PointerField locks the wrapper-struct wire
// behavior: money fields decode both as JSON numbers and strings, including
// through *FlexString (null/absent -> nil pointer), with no float rounding.
func TestFlexString_UnmarshalJSON_PointerField(t *testing.T) {
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
				t.Fatalf("A = %q, want %q", w.A.String(), c.wantA)
			}
			if c.wantBNil {
				if w.B != nil {
					t.Fatalf("B = %v, want nil", w.B.String())
				}
			} else if w.B == nil || w.B.String() != c.wantB {
				t.Fatalf("B = %v, want %q", w.B, c.wantB)
			}
		})
	}
}
