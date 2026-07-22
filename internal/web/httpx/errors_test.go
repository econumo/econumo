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
	WriteError(context.Background(), rec, err)

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

// TestWriteError_500SuppressesRawError locks the security property that an
// unhandled error's raw text (which can carry DB driver / constraint /
// internal detail) never reaches the client — it goes to the logs only.
func TestWriteError_500SuppressesRawError(t *testing.T) {
	secret := errPlainMsg("UNIQUE constraint failed: users.identifier")

	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, secret)
	var got struct {
		Message string `json:"message"`
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	if got.Message != "Internal Server Error" {
		t.Fatalf("message=%q must be the static message, not the raw error", got.Message)
	}
	if strings.Contains(rec.Body.String(), "users.identifier") {
		t.Fatalf("500 body leaked internal error text: %s", rec.Body.String())
	}
}

type errPlainMsg string

func (e errPlainMsg) Error() string { return string(e) }

// A rate-limited caller must be told how long to wait; without the header a
// 429 is a dead end. The envelope itself is frozen, so the wait rides on the
// standard header.
func TestWriteErrorTooManyRequestsRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, errs.NewTooManyRequestsRetryAfter("Too many attempts. Try again later.", 900))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "900" {
		t.Errorf("Retry-After = %q, want %q", got, "900")
	}
	// The frozen 429 envelope is unchanged.
	if body := rec.Body.String(); !strings.Contains(body, `"code":429`) {
		t.Errorf("envelope changed: %s", body)
	}

	bare := httptest.NewRecorder()
	WriteError(context.Background(), bare, errs.NewTooManyRequests("Too many attempts. Try again later."))
	if got := bare.Header().Get("Retry-After"); got != "" {
		t.Errorf("Retry-After = %q with no wait known, want it absent", got)
	}
}

// TestWriteErrorAccessDeniedRetryAfter covers the email-verification 403: the
// wait rides on the standard Retry-After header (the envelope is frozen), and
// the header is omitted entirely when no wait applies.
func TestWriteErrorAccessDeniedRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, &errs.AccessDeniedError{
		Msg: "Please verify your email address.", Code: errs.CodeUserEmailVerificationRequired, RetryAfter: 42,
	})
	if got := rec.Header().Get("Retry-After"); got != "42" {
		t.Errorf("Retry-After = %q, want %q", got, "42")
	}

	bare := httptest.NewRecorder()
	WriteError(context.Background(), bare, errs.NewAccessDenied("Access is not allowed"))
	if got := bare.Header().Get("Retry-After"); got != "" {
		t.Errorf("Retry-After = %q on a denial with no wait, want it absent", got)
	}
}

// TestWriteError_AccessDenied maps to HTTP 403.
func TestWriteError_AccessDenied(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, errs.NewAccessDenied("Access is not allowed"))
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
	WriteError(context.Background(), rec, codedErr) // no language in ctx -> en
	if want := `"errors":{"name":["Category name must be 3-64 characters"]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body missing %s\nbody: %s", want, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec, codedErr)
	if want := `"errors":{"name":["Название категории должно содержать от 3 до 64 символов."]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body missing %s\nbody: %s", want, rec.Body.String())
	}
}

func TestWriteErrorKeepsLiteralTextWhenNoCode(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec,
		errs.NewValidation("", errs.FieldError{Key: "name", Message: "msg"}))
	if want := `"errors":{"name":["msg"]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("code-less field text must stay literal\nbody: %s", rec.Body.String())
	}
}

// A code missing from every catalogue must keep the literal English text —
// never surface the dotted key on the wire.
func TestWriteErrorKeepsLiteralTextWhenCodeUnknown(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec, errs.NewValidation("",
		errs.FieldError{Key: "name", Message: "Something is wrong", Code: "no.such.code"}))
	if want := `"errors":{"name":["Something is wrong"]}`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("unknown-code field text must stay literal\nbody: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec,
		&errs.UnauthorizedError{Msg: "Something is wrong", Code: "no.such.code"})
	if !strings.Contains(rec.Body.String(), `"message":"Something is wrong"`) {
		t.Errorf("unknown-code message must stay literal\nbody: %s", rec.Body.String())
	}
}

func TestWriteErrorFieldlessMessageTranslated(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec,
		&errs.UnauthorizedError{Msg: "Invalid credentials.", Code: "auth.invalid_credentials"})
	body := rec.Body.String()
	if !strings.Contains(body, `"message":"Неверные учётные данные."`) {
		t.Errorf("message not translated in place\nbody: %s", body)
	}

	rec = httptest.NewRecorder()
	WriteError(context.Background(), rec,
		&errs.UnauthorizedError{Msg: "Invalid credentials.", Code: "auth.invalid_credentials"})
	if !strings.Contains(rec.Body.String(), `"message":"Invalid credentials."`) {
		t.Errorf("en message must match the historical string\nbody: %s", rec.Body.String())
	}
}

func TestWriteError_PaymentRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(reqctx.WithLanguage(context.Background(), "ru"), rec, &errs.PaymentRequiredError{
		Msg:  "Read-only access. Write operations are disabled.",
		Code: errs.CodeReadonlyAccess,
	})

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
		errs.NewPaymentRequired("Read-only access. Write operations are disabled."))
	body = strings.TrimSpace(rec.Body.String())
	want = `{"success":false,"message":"Read-only access. Write operations are disabled.","code":402,"errors":{}}`
	if body != want {
		t.Fatalf("code-less body:\n got %s\nwant %s", body, want)
	}
}

func TestWriteErrorAccessDeniedTranslatesCodedMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx := reqctx.WithLanguage(context.Background(), "ru")
	WriteError(ctx, rec, &errs.AccessDeniedError{
		Msg:  "Please verify your email address.",
		Code: errs.CodeUserEmailVerificationRequired,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Подтвердите адрес электронной почты.") {
		t.Errorf("message not translated: %s", body)
	}
	if !strings.Contains(body, `"errors":[]`) {
		t.Errorf("403 envelope must keep errors as an empty ARRAY: %s", body)
	}
}

func TestWriteErrorAccessDeniedWithoutCodeKeepsLiteral(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, errs.NewAccessDenied("You are not allowed"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "You are not allowed") {
		t.Errorf("code-less 403 must keep its literal message: %s", rec.Body.String())
	}
}
