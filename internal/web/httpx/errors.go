package httpx

import (
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
)

// WriteError maps a domain/service error onto the correct envelope + HTTP
// status:
//   - *errs.ValidationError  -> 400, errors{} populated (field -> messages)
//   - *errs.AccessDeniedError -> 403, empty errors
//   - *errs.UnauthorizedError -> 401, empty errors
//   - *errs.TooManyRequestsError -> 429, code 429, empty errors
//   - *errs.NotFoundError     -> 400 (domain not-found goes through the generic
//     error envelope; revisit per-endpoint if a test expects 404)
//   - anything else           -> 500 exception envelope
//
// dev controls whether the 500 path includes a stack trace.
func WriteError(w http.ResponseWriter, err error, dev bool) {
	recordError(w, err)
	if v, ok := errs.AsValidation(err); ok {
		// Field-level (form) validation keeps the generic "Form validation error"
		// label: the actionable detail is the per-field errors{} map, which clients
		// parse. A FIELDLESS validation error (e.g. "User already exists", "The code
		// is expired") carries its message as the only signal, so surface that
		// message instead of masking it behind the generic label.
		msg := v.Msg
		if len(v.Fields) > 0 {
			msg = "Form validation error"
		}
		Err(w, msg, http.StatusBadRequest, fieldsToMap(v.Fields), http.StatusBadRequest)
		return
	}
	if v, ok := errs.AsAccessDenied(err); ok {
		// 403 with errors:[] (empty ARRAY, not {}) and the domain message verbatim.
		AccessDenied(w, v.Msg)
		return
	}
	if v, ok := errs.AsUnauthorized(err); ok {
		Err(w, v.Error(), 0, nil, http.StatusUnauthorized)
		return
	}
	if v, ok := errs.AsTooManyRequests(err); ok {
		Err(w, v.Error(), http.StatusTooManyRequests, nil, http.StatusTooManyRequests)
		return
	}
	if v, ok := errs.AsNotFound(err); ok {
		Err(w, v.Error(), 0, nil, http.StatusBadRequest)
		return
	}
	// Unhandled: 500 exception envelope. The real error was already captured to
	// the access log via recordError above, so the client receives a generic
	// message — internal detail (DB driver/constraint text, parse internals)
	// must not leak in production. dev surfaces the real error for local use,
	// matching the static-message discipline of the panic recovery path.
	msg := "Internal Server Error"
	if dev {
		msg = err.Error()
	}
	Exception(w, msg, typeName(err), nil, dev)
}

// fieldsToMap converts the flat field-error list into the wire map shape
// (field name -> list of messages). Form-wide errors (empty Key) are grouped
// under the empty string key.
func fieldsToMap(fields []errs.FieldError) map[string][]string {
	out := make(map[string][]string, len(fields))
	for _, f := range fields {
		out[f.Key] = append(out[f.Key], f.Message)
	}
	return out
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
