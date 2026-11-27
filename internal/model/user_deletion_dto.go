package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// RequestUserDeletionRequest has no Validate: a passwordless user, or anyone
// on a server with password sign-in disabled, sends an empty password.
type RequestUserDeletionRequest struct {
	Password string `json:"password"`
}

type RequestUserDeletionResult struct{}

type ConfirmUserDeletionRequest struct {
	Code string `json:"code"`
}

func (r ConfirmUserDeletionRequest) Validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type ConfirmUserDeletionResult struct{}
