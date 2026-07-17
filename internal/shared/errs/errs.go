// Package errs defines the domain-level error taxonomy. Each kind maps to a
// specific HTTP status + response-envelope shape in ui/httpx. Keeping the
// taxonomy in the domain layer (not the HTTP layer) lets services and value
// objects signal intent without importing net/http.
package errs

import (
	"errors"
	"fmt"
)

// FieldError is a single field-level validation failure. It is the shape of one
// entry in the response envelope's errors[] array.
type FieldError struct {
	// Key is the field/path the error applies to (may be empty for form-wide
	// errors). Some errors are grouped by message rather than field; the
	// translation layer decides which to use per endpoint.
	Key string
	// Message is the human-readable, already-translated message. The exact
	// strings are frozen (see CLAUDE.md) and asserted by the test suite.
	Message string
	// Code is a catalogue key under errors.* in locales/*.json (e.g.
	// "category.name_length"), the machine-readable, additive sibling of
	// Message. Every emitted code must be registered in AllCodes (codes.go);
	// the i18ntest guard asserts a two-way match with the catalogues.
	Code string
	// Params carries the message's interpolation values (e.g. {"min":3,"max":64})
	// for the frontend to substitute into the localized string.
	Params map[string]any
}

// ValidationError carries one or more field errors. Maps to HTTP 400 with a
// populated errors[] array in the envelope.
type ValidationError struct {
	Msg       string
	MsgCode   string
	MsgParams map[string]any
	Fields    []FieldError
}

func (e *ValidationError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("validation failed: %d field error(s)", len(e.Fields))
}

// NewValidation builds a ValidationError from field errors.
func NewValidation(msg string, fields ...FieldError) *ValidationError {
	return &ValidationError{Msg: msg, Fields: fields}
}

// NotFoundError signals that a requested entity was not found. HTTP status is
// decided by the HTTP layer (most domain not-found cases surface as 400 via the
// generic error envelope; confirmed per-endpoint against golden output).
type NotFoundError struct {
	Msg string
}

func (e *NotFoundError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "not found"
}

// NewNotFound builds a NotFoundError.
func NewNotFound(msg string) *NotFoundError { return &NotFoundError{Msg: msg} }

// AccessDeniedError maps to HTTP 403.
type AccessDeniedError struct {
	Msg string
}

func (e *AccessDeniedError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "access denied"
}

// NewAccessDenied builds an AccessDeniedError.
// (Used by the connection/sharing access-control modules.)
func NewAccessDenied(msg string) *AccessDeniedError { return &AccessDeniedError{Msg: msg} }

// UnauthorizedError maps to HTTP 401 (missing/invalid credentials or token).
type UnauthorizedError struct {
	Msg  string
	Code string
}

func (e *UnauthorizedError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "unauthorized"
}

// NewUnauthorized builds an UnauthorizedError.
func NewUnauthorized(msg string) *UnauthorizedError { return &UnauthorizedError{Msg: msg} }

// AsValidation reports whether err is (or wraps) a *ValidationError.
func AsValidation(err error) (*ValidationError, bool) {
	var v *ValidationError
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}

// AsNotFound reports whether err is (or wraps) a *NotFoundError.
func AsNotFound(err error) (*NotFoundError, bool) {
	var v *NotFoundError
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}

// AsAccessDenied reports whether err is (or wraps) an *AccessDeniedError.
func AsAccessDenied(err error) (*AccessDeniedError, bool) {
	var v *AccessDeniedError
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}

// AsUnauthorized reports whether err is (or wraps) an *UnauthorizedError.
func AsUnauthorized(err error) (*UnauthorizedError, bool) {
	var v *UnauthorizedError
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}

// TooManyRequestsError maps to HTTP 429 (a rate-limited auth attempt).
type TooManyRequestsError struct {
	Msg string
}

func (e *TooManyRequestsError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "too many requests"
}

// NewTooManyRequests builds a TooManyRequestsError.
func NewTooManyRequests(msg string) *TooManyRequestsError { return &TooManyRequestsError{Msg: msg} }

// AsTooManyRequests reports whether err is (or wraps) a *TooManyRequestsError.
func AsTooManyRequests(err error) (*TooManyRequestsError, bool) {
	var v *TooManyRequestsError
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}
