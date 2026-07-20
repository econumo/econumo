package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
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
	WriteError(context.Background(), rec, err, false)

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
	WriteError(context.Background(), rec, secret, false) // production
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
	WriteError(context.Background(), rec, secret, true) // dev
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
	WriteError(context.Background(), rec, errs.NewAccessDenied("Access is not allowed"), false)
	if rec.Code != http.StatusForbidden {
		t.Errorf("HTTP status = %d, want 403", rec.Code)
	}
	if env := decodeEnvelope(t, rec); env.Success {
		t.Errorf("success = true, want false")
	}
}

func TestWriteErrorTranslatesFieldErrorsInPlace(t *testing.T) {
	codedErr := errs.NewValidation("",
		errs.FieldError{Key: "name", Message: "Category name must be 3-64 characters",
			Code: "category.name_length", Params: map[string]any{"min": 3, "max": 64}})

	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, codedErr, false) // no language in ctx -> en
	if want := `"errors":{"name":["Category name must be 3-64 characters"]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body missing %s\nbody: %s", want, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec, codedErr, false)
	if want := `"errors":{"name":["Название категории должно содержать от 3 до 64 символов."]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body missing %s\nbody: %s", want, rec.Body.String())
	}
}

func TestWriteErrorKeepsLiteralTextWhenNoCode(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec,
		errs.NewValidation("", errs.FieldError{Key: "name", Message: "msg"}), false)
	if want := `"errors":{"name":["msg"]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("code-less field text must stay literal\nbody: %s", rec.Body.String())
	}
}

// A code missing from every catalogue must keep the literal English text —
// never surface the dotted key on the wire.
func TestWriteErrorKeepsLiteralTextWhenCodeUnknown(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec, errs.NewValidation("",
		errs.FieldError{Key: "name", Message: "Something is wrong", Code: "no.such.code"}), false)
	if want := `"errors":{"name":["Something is wrong"]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("unknown-code field text must stay literal\nbody: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec,
		&errs.UnauthorizedError{Msg: "Something is wrong", Code: "no.such.code"}, false)
	if !strings.Contains(rec.Body.String(), `"message":"Something is wrong"`) {
		t.Errorf("unknown-code message must stay literal\nbody: %s", rec.Body.String())
	}
}

func TestWriteErrorFieldlessMessageTranslated(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec,
		&errs.UnauthorizedError{Msg: "Invalid credentials.", Code: "auth.invalid_credentials"}, false)
	body := rec.Body.String()
	if !strings.Contains(body, `"message":"Неверные учётные данные."`) {
		t.Errorf("message not translated in place\nbody: %s", body)
	}

	rec = httptest.NewRecorder()
	WriteError(context.Background(), rec,
		&errs.UnauthorizedError{Msg: "Invalid credentials.", Code: "auth.invalid_credentials"}, false)
	if !strings.Contains(rec.Body.String(), `"message":"Invalid credentials."`) {
		t.Errorf("en message must match the historical string\nbody: %s", rec.Body.String())
	}
}

func TestWriteError_PaymentRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec, &errs.PaymentRequiredError{
		Msg:  "Read-only access. Write operations are disabled.",
		Code: errs.CodeReadonlyAccess,
	}, false)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("HTTP status = %d, want 402", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	want := `{"success":false,"message":"Вам доступно только чтение.","code":402,"errors":{}}`
	if body != want {
		t.Fatalf("body:\n got %s\nwant %s", body, want)
	}
	if env := decodeEnvelope(t, rec); env.Success {
		t.Fatalf("success = true, want false")
	}

	// The code rides on the error value (like UnauthorizedError.Code), so a
	// code-less 402 keeps its literal English text in any language.
	rec = httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec,
		errs.NewPaymentRequired("Read-only access. Write operations are disabled."), false)
	body = strings.TrimSpace(rec.Body.String())
	want = `{"success":false,"message":"Read-only access. Write operations are disabled.","code":402,"errors":{}}`
	if body != want {
		t.Fatalf("code-less body:\n got %s\nwant %s", body, want)
	}
}
