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
