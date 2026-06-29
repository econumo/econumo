package httpx

import (
	"net/http"

	"github.com/econumo/econumo/internal/domain/shared/errs"
)

// WriteError maps a domain/service error onto the correct envelope + HTTP
// status:
//   - *errs.ValidationError  -> 400, errors{} populated (field -> messages)
//   - *errs.AccessDeniedError -> 403, empty errors
//   - *errs.UnauthorizedError -> 401, empty errors
//   - *errs.NotFoundError     -> 400 (domain not-found goes through the generic
//     error envelope; revisit per-endpoint if a test expects 404)
//   - anything else           -> 500 exception envelope
//
// dev controls whether the 500 path includes a stack trace.
func WriteError(w http.ResponseWriter, err error, dev bool) {
	recordError(w, err)
	if v, ok := errs.AsValidation(err); ok {
		// Frozen wire contract: validation failures always carry the envelope
		// message "Form validation error" and code 400, regardless of the
		// per-field messages. Setting it centrally keeps every endpoint's
		// validation envelope byte-identical instead of relying on each call site.
		Err(w, "Form validation error", http.StatusBadRequest, fieldsToMap(v.Fields), http.StatusBadRequest)
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
	if v, ok := errs.AsNotFound(err); ok {
		Err(w, v.Error(), 0, nil, http.StatusBadRequest)
		return
	}
	// Unhandled: 500 exception envelope. We pass the Go type name as
	// exceptionType for debugging; stackTrace only in dev.
	Exception(w, err.Error(), typeName(err), nil, dev)
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
// (internal/ui/middleware). recordError surfaces the error that produced a
// response to the access log without coupling httpx to the middleware package or
// changing any handler signature; it is a no-op when the writer is not wrapped.
type errorRecorder interface{ SetError(error) }

func recordError(w http.ResponseWriter, err error) {
	if r, ok := w.(errorRecorder); ok {
		r.SetError(err)
	}
}
