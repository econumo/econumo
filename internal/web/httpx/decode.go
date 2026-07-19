package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
)

// MaxJSONBody caps a decoded JSON request body. No API payload approaches this;
// the bound exists so an unauthenticated client cannot exhaust memory with an
// arbitrarily large body (decoding happens before service-level rate limiting).
const MaxJSONBody = 1 << 20 // 1 MiB

// Validator is implemented by request DTOs that carry hand-written validation
// (tier-1: shape/format). Returning an *errs.ValidationError yields a 400 with
// a populated errors map.
type Validator interface {
	Validate() error
}

// Decode reads the JSON body into dst. It is intentionally tolerant of unknown
// fields — DisallowUnknownFields is NOT set, so extra fields are ignored. An
// empty body decodes to the zero value (some endpoints accept {}).
func Decode(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, MaxJSONBody))
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return nil // empty body -> zero value
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errs.NewValidation("Request body too large.")
		}
		return err
	}
	return nil
}

// DecodeValidate decodes the body then runs dst.Validate() if dst implements
// Validator. A JSON syntax error is surfaced as a generic 400.
func DecodeValidate(r *http.Request, dst any) error {
	if err := Decode(r, dst); err != nil {
		return err
	}
	if v, ok := dst.(Validator); ok {
		return v.Validate()
	}
	return nil
}
