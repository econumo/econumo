package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// ---------------------------------------------------------------------------
// get-session-list
// ---------------------------------------------------------------------------

// SessionItem is one row of get-session-list. isCurrent marks the session
// whose token authenticated THIS request.
type SessionItem struct {
	Id         string `json:"id"`
	UserAgent  string `json:"userAgent"` // "" when the login sent no User-Agent
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt"`
	IsCurrent  bool   `json:"isCurrent"`
}

// ---------------------------------------------------------------------------
// revoke-session / revoke-other-sessions
// ---------------------------------------------------------------------------

// RevokeSessionRequest is the revoke-session request body.
type RevokeSessionRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r RevokeSessionRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}

// RevokeSessionResult is the revoke-session response (empty object).
type RevokeSessionResult struct{}

// RevokeOtherSessionsResult is the revoke-other-sessions response (empty object).
type RevokeOtherSessionsResult struct{}
