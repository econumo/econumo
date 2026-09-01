package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

type decodeTarget struct {
	Name string `json:"name"`
}

func TestDecode_WithinLimit(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok"}`))
	var dst decodeTarget
	if err := Decode(r, &dst); err != nil {
		t.Fatalf("Decode = %v, want nil", err)
	}
	if dst.Name != "ok" {
		t.Fatalf("name=%q want ok", dst.Name)
	}
}

func TestDecode_OversizedBody_ValidationError(t *testing.T) {
	// A JSON string value larger than MaxJSONBody must be rejected as a 400-level
	// validation error, not a 500 leaking "request body too large".
	big := `{"name":"` + strings.Repeat("a", MaxJSONBody+1) + `"}`
	r := httptest.NewRequest("POST", "/", strings.NewReader(big))
	var dst decodeTarget
	err := Decode(r, &dst)
	if err == nil {
		t.Fatal("Decode = nil, want oversized-body error")
	}
	if _, ok := errs.AsValidation(err); !ok {
		t.Fatalf("Decode error = %T (%v), want *errs.ValidationError", err, err)
	}
}

func TestDecode_MalformedBody_ValidationError(t *testing.T) {
	// Syntax errors and wrong-typed fields are client mistakes: they must become
	// 400-level validation errors, never the 500 exception envelope. The decoder's
	// own text names Go struct fields, so the message must not echo it.
	for _, tc := range []struct {
		name string
		body string
	}{
		{"syntax error", `{broken`},
		{"wrong field type", `{"name":12345}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			var dst decodeTarget
			err := Decode(r, &dst)
			if err == nil {
				t.Fatal("Decode = nil, want malformed-body error")
			}
			v, ok := errs.AsValidation(err)
			if !ok {
				t.Fatalf("Decode error = %T (%v), want *errs.ValidationError", err, err)
			}
			if v.Msg != "Request body is not valid JSON." {
				t.Fatalf("message = %q, want the fixed generic message", v.Msg)
			}
			if len(v.Fields) != 0 {
				t.Fatalf("fields = %v, want none (fieldless error surfaces its own message)", v.Fields)
			}
		})
	}
}

func TestDecode_EmptyBody_NoError(t *testing.T) {
	// Some endpoints accept {}; an empty body must still decode to the zero value
	// rather than tripping the new malformed-body branch.
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	var dst decodeTarget
	if err := Decode(r, &dst); err != nil {
		t.Fatalf("Decode = %v, want nil", err)
	}
}
