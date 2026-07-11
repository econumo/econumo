package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

func TestOK_SuccessEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	OK(rec, map[string]string{"k": "v"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type=%q want application/json", ct)
	}
	var env struct {
		Success bool              `json:"success"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if !env.Success || env.Message != "" || env.Data["k"] != "v" {
		t.Fatalf("envelope=%+v", env)
	}
}

func TestOK_DoesNotEscapeHTMLChars(t *testing.T) {
	rec := httptest.NewRecorder()
	OK(rec, map[string]string{"url": "https://x/y?a=b&c=d"})
	if got := rec.Body.String(); !strings.Contains(got, "https://x/y?a=b&c=d") {
		t.Fatalf("HTML chars were escaped; body=%s", got)
	}
}

// Raw is the login shape: the payload sits at the top level, un-enveloped.
func TestRaw_TopLevelPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	Raw(rec, map[string]string{"token": "abc", "x": "y"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, hasSuccess := m["success"]; hasSuccess {
		t.Fatalf("Raw must not wrap in an envelope; body=%s", rec.Body.String())
	}
	if _, hasToken := m["token"]; !hasToken {
		t.Fatalf("Raw payload missing token; body=%s", rec.Body.String())
	}
}

func TestErr_DefaultsToBadRequestAndEmptyErrorsObject(t *testing.T) {
	rec := httptest.NewRecorder()
	Err(rec, "nope", 7, nil, 0) // httpCode 0 -> defaults to 400
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (default)", rec.Code)
	}
	// errors must serialize as an object {} when nil, not null.
	var probe struct {
		Errors json.RawMessage `json:"errors"`
	}
	json.Unmarshal(rec.Body.Bytes(), &probe)
	if string(probe.Errors) != "{}" {
		t.Fatalf("errors=%s want {} (empty object)", probe.Errors)
	}
}

func TestAccessDenied_403WithEmptyArrayErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	AccessDenied(rec, "Access denied")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
	var probe struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	json.Unmarshal(rec.Body.Bytes(), &probe)
	if probe.Success {
		t.Fatal("success=true want false")
	}
	if probe.Message != "Access denied" {
		t.Fatalf("message=%q want %q", probe.Message, "Access denied")
	}
	// errors is the empty ARRAY [] here, not the usual {} object — a frozen quirk.
	if string(probe.Errors) != "[]" {
		t.Fatalf("errors=%s want [] (empty array)", probe.Errors)
	}
}

func TestException_NonDevOmitsStackTrace(t *testing.T) {
	rec := httptest.NewRecorder()
	Exception(rec, "boom", "SomeType", "the-stack", false)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	var m map[string]any
	json.Unmarshal(rec.Body.Bytes(), &m)
	if _, ok := m["stackTrace"]; ok {
		t.Fatalf("non-dev must omit stackTrace; body=%s", rec.Body.String())
	}
	if m["exceptionType"] != "SomeType" {
		t.Fatalf("exceptionType=%v want SomeType", m["exceptionType"])
	}
}

func TestException_DevIncludesStackTrace(t *testing.T) {
	rec := httptest.NewRecorder()
	Exception(rec, "boom", "SomeType", "the-stack", true)
	var m map[string]any
	json.Unmarshal(rec.Body.Bytes(), &m)
	if m["stackTrace"] != "the-stack" {
		t.Fatalf("stackTrace=%v want the-stack (dev)", m["stackTrace"])
	}
}

func TestNotImplemented_501WithEmptyArrayErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	NotImplemented(rec, "later")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status=%d want 501", rec.Code)
	}
	var probe struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	json.Unmarshal(rec.Body.Bytes(), &probe)
	if probe.Success || probe.Message != "later" {
		t.Fatalf("probe=%+v", probe)
	}
	if string(probe.Errors) != "[]" {
		t.Fatalf("errors=%s want [] (array)", probe.Errors)
	}
}

func TestWriteError_StatusMappingMatrix(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string // "" = don't assert
		wantBody   string // "" = don't assert; exact envelope match (success/message/code/errors)
	}{
		{"validation", errs.NewValidation("ignored", errs.FieldError{Key: "f", Message: "bad"}), http.StatusBadRequest, "Form validation error", ""},
		{"access denied", errs.NewAccessDenied("nope"), http.StatusForbidden, "nope", ""},
		{"unauthorized", errs.NewUnauthorized("no token"), http.StatusUnauthorized, "no token", ""},
		{"not found", errs.NewNotFound("Plan not found"), http.StatusBadRequest, "Plan not found", ""},
		{"unknown", errPlain("kaboom"), http.StatusInternalServerError, "Internal Server Error", ""}, // raw error suppressed in prod (dev=false)
		{
			name:       "too many requests -> 429 envelope",
			err:        errs.NewTooManyRequests("Too many attempts. Try again later."),
			wantStatus: http.StatusTooManyRequests,
			wantBody:   `{"success":false,"message":"Too many attempts. Try again later.","code":429,"errors":{}}` + "\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, c.err, false)
			if rec.Code != c.wantStatus {
				t.Fatalf("status=%d want %d; body=%s", rec.Code, c.wantStatus, rec.Body.String())
			}
			if c.wantMsg != "" {
				var probe struct {
					Message string `json:"message"`
				}
				json.Unmarshal(rec.Body.Bytes(), &probe)
				if probe.Message != c.wantMsg {
					t.Fatalf("message=%q want %q", probe.Message, c.wantMsg)
				}
			}
			if c.wantBody != "" && rec.Body.String() != c.wantBody {
				t.Fatalf("body=%s want %s", rec.Body.String(), c.wantBody)
			}
		})
	}
}

// errPlain is a non-domain error to drive the WriteError 500 fallback.
type errPlain string

func (e errPlain) Error() string { return string(e) }
