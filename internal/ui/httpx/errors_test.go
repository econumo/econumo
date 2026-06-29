package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/domain/shared/errs"
)

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Errors  json.RawMessage `json:"errors"`
}

// errs decodes the validation-form errors object (field -> messages). The
// access-denied/exception paths emit an empty array ([]) instead; that leaves
// the returned map empty.
func (e envelope) errs() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) > 0 {
		_ = json.Unmarshal(e.Errors, &m)
	}
	return m
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, rec.Body.String())
	}
	return e
}

// TestWriteError_ValidationEnvelope locks the frozen validation envelope:
// message "Form validation error", code 400, HTTP 400, with the field errors
// carried through — regardless of the ValidationError's own Msg (clients parse
// this exact shape; not "Validation failed" / code 0).
func TestWriteError_ValidationEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	err := errs.NewValidation("anything-here-is-ignored",
		errs.FieldError{Key: "accountId", Message: "This value is not a valid UUID."},
	)
	WriteError(rec, err, false)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HTTP status = %d, want 400", rec.Code)
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Errorf("success = true, want false")
	}
	if env.Message != "Form validation error" {
		t.Errorf("message = %q, want %q", env.Message, "Form validation error")
	}
	if env.Code != 400 {
		t.Errorf("code = %d, want 400", env.Code)
	}
	if got := env.errs()["accountId"]; len(got) != 1 || got[0] != "This value is not a valid UUID." {
		t.Errorf("errors[accountId] = %v, want [%q]", got, "This value is not a valid UUID.")
	}
}

// TestWriteError_AccessDenied maps to HTTP 403.
func TestWriteError_AccessDenied(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errs.NewAccessDenied("Access is not allowed"), false)
	if rec.Code != http.StatusForbidden {
		t.Errorf("HTTP status = %d, want 403", rec.Code)
	}
	if env := decodeEnvelope(t, rec); env.Success {
		t.Errorf("success = true, want false")
	}
}
