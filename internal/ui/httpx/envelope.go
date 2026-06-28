// Package httpx implements the frozen JSON response envelope and the
// domain-error -> HTTP-status mapping. The envelope shape is wire-compatible
// with existing API clients (see CLAUDE.md).
//
// IMPORTANT: the validation errors[] payload is NOT an array of
// {key,message,code} — it is a MAP of field-name -> []string (list of
// messages). So `errors` serializes as a JSON object, e.g.
// {"name":["Category name must be 3-64 characters"]}.
package httpx

import (
	"encoding/json"
	"net/http"
)

// okEnvelope is the success response.
type okEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// errEnvelope is the error response (validation / handled HTTP errors).
// Errors is a map field-name -> messages (see package doc). The key is always
// present, even when empty.
type errEnvelope struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Code    int                 `json:"code"`
	Errors  map[string][]string `json:"errors"`
}

// exceptionEnvelope is the unhandled-exception response (HTTP 500). It omits
// the errors[] key and adds exceptionType, plus stackTrace only in dev.
type exceptionEnvelope struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	Code          int    `json:"code"`
	ExceptionType string `json:"exceptionType,omitempty"`
	StackTrace    any    `json:"stackTrace,omitempty"`
}

func writeJSON(w http.ResponseWriter, httpCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	// Encoder is used over Marshal to stream; disable HTML escaping so '/' etc.
	// are not \u-encoded (wire-compatible with existing API clients).
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

// OK writes a 200 success envelope wrapping data.
func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, okEnvelope{Success: true, Message: "", Data: data})
}

// Raw writes a 200 with the payload serialized AT THE TOP LEVEL — no
// {success,message,data} envelope. This mirrors the few PHP controllers that
// return `new JsonResponse($result)` directly instead of going through
// ResponseFactory: the login endpoint (LoginUserV1Controller) emits the raw
// {token,user} object, and the Vue SPA reads response.token off the top level
// (web/src/stores/users.ts), so wrapping it would break login.
func Raw(w http.ResponseWriter, payload any) {
	writeJSON(w, http.StatusOK, payload)
}

// Err writes an error envelope (default HTTP 400). errors may be nil; it is
// always serialized as an object ({} when empty) for wire compatibility.
func Err(w http.ResponseWriter, message string, code int, errors map[string][]string, httpCode int) {
	if httpCode == 0 {
		httpCode = http.StatusBadRequest
	}
	if errors == nil {
		errors = map[string][]string{}
	}
	writeJSON(w, httpCode, errEnvelope{Success: false, Message: message, Code: code, Errors: errors})
}

// accessDeniedEnvelope is the 403 response. PHP renders it via
// ResponseFactory::createErrorResponse(msg, code, [], HTTP_FORBIDDEN) — note the
// errors argument is an empty PHP ARRAY, which serializes as [] (NOT the {} that
// the validation path's field-map produces). The message is the domain
// exception's own message, which for resource-ownership denials is empty.
type accessDeniedEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    int    `json:"code"`
	Errors  []any  `json:"errors"`
}

// AccessDenied writes the 403 envelope: errors serialized as [] (an empty array,
// matching PHP), message taken verbatim from the domain error (empty for bare
// ownership denials).
func AccessDenied(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusForbidden, accessDeniedEnvelope{
		Success: false, Message: message, Code: 0, Errors: []any{},
	})
}

// Exception writes the 500 exception envelope. stackTrace is included only when
// dev is true (matching ECONUMO_DEBUG=true behavior).
func Exception(w http.ResponseWriter, message, exceptionType string, stackTrace any, dev bool) {
	env := exceptionEnvelope{Success: false, Message: message, Code: 0, ExceptionType: exceptionType}
	if dev {
		env.StackTrace = stackTrace
	}
	writeJSON(w, http.StatusInternalServerError, env)
}

// NotImplemented writes the 501 envelope: success:false, code:0, errors:[].
// Note errors is emitted as [] here, not {} — see CLAUDE.md.
func NotImplemented(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{
		"success": false,
		"message": message,
		"code":    0,
		"errors":  []any{},
	})
}
