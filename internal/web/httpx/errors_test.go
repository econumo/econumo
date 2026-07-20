package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
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

// TestWriteError_500SuppressesRawErrorInProd locks the security property that
// an unhandled error's raw text (which can carry DB driver / constraint /
// internal detail) never reaches the client in production, only under dev.
func TestWriteError_500SuppressesRawErrorInProd(t *testing.T) {
	secret := errPlainMsg("UNIQUE constraint failed: users.identifier")

	rec := httptest.NewRecorder()
	WriteError(rec, secret, false) // production
	var prod struct {
		Message string `json:"message"`
	}
	json.Unmarshal(rec.Body.Bytes(), &prod)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	if prod.Message != "Internal Server Error" {
		t.Fatalf("prod message=%q must be the static message, not the raw error", prod.Message)
	}
	if strings.Contains(rec.Body.String(), "users.identifier") {
		t.Fatalf("prod 500 body leaked internal error text: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteError(rec, secret, true) // dev
	var dev struct {
		Message string `json:"message"`
	}
	json.Unmarshal(rec.Body.Bytes(), &dev)
	if dev.Message != string(secret) {
		t.Fatalf("dev message=%q want the raw error", dev.Message)
	}
}

type errPlainMsg string

func (e errPlainMsg) Error() string { return string(e) }

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

func TestWriteErrorEmitsAdditiveCodes(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errs.NewValidation("",
		errs.FieldError{Key: "name", Message: "Category name must be 3-64 characters",
			Code: "category.name_length", Params: map[string]any{"min": 3, "max": 64}}), false)
	body := rec.Body.String()
	for _, want := range []string{
		`"errors":{"name":["Category name must be 3-64 characters"]}`,
		`"errorCodes":{"name":[{"code":"category.name_length","params":{"max":64,"min":3}}]}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s\nbody: %s", want, body)
		}
	}
}

func TestWriteErrorOmitsCodeKeysWhenAbsent(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errs.NewValidation("", errs.FieldError{Key: "name", Message: "msg"}), false)
	body := rec.Body.String()
	for _, banned := range []string{"errorCodes", "messageCode", "messageParams"} {
		if strings.Contains(body, banned) {
			t.Errorf("body must omit %q when no codes set\nbody: %s", banned, body)
		}
	}
}

func TestWriteErrorFieldlessMessageCode(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, &errs.UnauthorizedError{Msg: "Invalid credentials.", Code: "auth.invalid_credentials"}, false)
	body := rec.Body.String()
	if !strings.Contains(body, `"messageCode":"auth.invalid_credentials"`) {
		t.Errorf("missing messageCode\nbody: %s", body)
	}
	if !strings.Contains(body, `"message":"Invalid credentials."`) {
		t.Errorf("frozen message changed\nbody: %s", body)
	}
}

func TestWriteError_PaymentRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errs.NewPaymentRequired("Read-only access. Write operations are disabled."), false)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("HTTP status = %d, want 402", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	// messageCode is additive (omitempty): the frozen message/code/errors trio
	// is unchanged, and the code lets the SPA render a translated string.
	want := `{"success":false,"message":"Read-only access. Write operations are disabled.","code":402,"errors":{},"messageCode":"common.readonly_access"}`
	if body != want {
		t.Fatalf("body:\n got %s\nwant %s", body, want)
	}
	if env := decodeEnvelope(t, rec); env.Success {
		t.Fatalf("success = true, want false")
	}
}
