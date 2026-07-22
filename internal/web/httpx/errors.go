package httpx

import (
	"context"
	"net/http"
	"strconv"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// WriteError maps a domain/service error onto the correct envelope + HTTP
// status:
//   - *errs.ValidationError  -> 400, errors{} populated (field -> messages)
//   - *errs.AccessDeniedError -> 403, empty errors
//   - *errs.UnauthorizedError -> 401, empty errors
//   - *errs.PaymentRequiredError -> 402, code 402, empty errors
//   - *errs.TooManyRequestsError -> 429, code 429, empty errors
//   - *errs.NotFoundError     -> 400 (domain not-found goes through the generic
//     error envelope; revisit per-endpoint if a test expects 404)
//   - anything else           -> 500 exception envelope
//
// ctx supplies the caller's language (reqctx.Language): message and the
// per-field errors{} strings are rendered from the errors.* catalogue when the
// underlying error carries a code; errors without a code keep their literal
// English text.
func WriteError(ctx context.Context, w http.ResponseWriter, err error) {
	recordError(w, err)
	lang := reqctx.Language(ctx)
	if v, ok := errs.AsValidation(err); ok {
		// Field-level (form) validation keeps the generic "Form validation error"
		// label: the actionable detail is the per-field errors{} map, which clients
		// parse. A FIELDLESS validation error (e.g. "User already exists", "The code
		// is expired") carries its message as the only signal, so surface that
		// message instead of masking it behind the generic label.
		msg := translated(lang, v.Msg, v.MsgCode, v.MsgParams)
		if len(v.Fields) > 0 {
			msg = "Form validation error"
		}
		Err(w, msg, http.StatusBadRequest, fieldsToMap(lang, v.Fields), http.StatusBadRequest)
		return
	}
	if v, ok := errs.AsAccessDenied(err); ok {
		// 403 with errors:[] (empty ARRAY, not {}); a coded message renders in
		// the caller's language, a code-less one keeps its literal text.
		if v.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(v.RetryAfter))
		}
		AccessDenied(w, translated(lang, v.Msg, v.Code, nil))
		return
	}
	if v, ok := errs.AsUnauthorized(err); ok {
		Err(w, translated(lang, v.Msg, v.Code, nil), 0, nil, http.StatusUnauthorized)
		return
	}
	if v, ok := errs.AsPaymentRequired(err); ok {
		Err(w, translated(lang, v.Msg, v.Code, nil), http.StatusPaymentRequired, nil, http.StatusPaymentRequired)
		return
	}
	if v, ok := errs.AsTooManyRequests(err); ok {
		if v.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(v.RetryAfter))
		}
		Err(w, translated(lang, v.Error(), errs.CodeTooManyAttempts, nil),
			http.StatusTooManyRequests, nil, http.StatusTooManyRequests)
		return
	}
	if v, ok := errs.AsNotFound(err); ok {
		Err(w, v.Error(), 0, nil, http.StatusBadRequest)
		return
	}
	// Unhandled: 500 exception envelope. The real error was already captured to
	// the access log via recordError above (it lands on the operation line's
	// err/err_type fields), so the client receives only a generic message —
	// internal detail (DB driver/constraint text, parse internals) must not leak.
	Exception(w, "Internal Server Error", typeName(err))
}

// fieldsToMap converts the flat field-error list into the wire map shape
// (field name -> list of messages), rendering each entry in the caller's
// language when it carries a catalogue code. Form-wide errors (empty Key) are
// grouped under the empty string key.
func fieldsToMap(lang string, fields []errs.FieldError) map[string][]string {
	out := make(map[string][]string, len(fields))
	for _, f := range fields {
		out[f.Key] = append(out[f.Key], translated(lang, f.Message, f.Code, f.Params))
	}
	return out
}

// translated renders the catalogue text for code in the caller's language
// (i18n.Lookup itself falls back to English for a language missing the key).
// An error with no code — or a code absent from every catalogue — keeps its
// literal English message rather than leaking a dotted key.
func translated(lang, literal, code string, params map[string]any) string {
	if code == "" {
		return literal
	}
	if msg, ok := i18n.Lookup(lang, "errors."+code, params); ok {
		return msg
	}
	return literal
}

func typeName(err error) string {
	if err == nil {
		return ""
	}
	return "" // populated later if a specific exceptionType string is needed
}

// errorRecorder is satisfied by the access-log response writer
// (internal/web/middleware). recordError surfaces the error that produced a
// response to the access log without coupling httpx to the middleware package or
// changing any handler signature; it is a no-op when the writer is not wrapped.
type errorRecorder interface{ SetError(error) }

func recordError(w http.ResponseWriter, err error) {
	if r, ok := w.(errorRecorder); ok {
		r.SetError(err)
	}
}
