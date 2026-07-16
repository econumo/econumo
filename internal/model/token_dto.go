package model

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
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
			errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// RevokeSessionResult is the revoke-session response (empty object).
type RevokeSessionResult struct{}

// RevokeOtherSessionsResult is the revoke-other-sessions response (empty object).
type RevokeOtherSessionsResult struct{}

// ---------------------------------------------------------------------------
// personal access tokens
// ---------------------------------------------------------------------------

// PersonalTokenItem is one row of get-personal-token-list. The raw token is
// NEVER included — it exists only in the create response.
type PersonalTokenItem struct {
	Id         string  `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt string  `json:"lastUsedAt"`
	ExpiresAt  *string `json:"expiresAt"` // null = never expires
}

// CreatePersonalTokenRequest is the create-personal-token request body.
// expiresAt is optional ("" = never expires); when set it must parse with the
// frozen datetime layout. The must-be-in-the-future check lives in the use
// case (it needs the clock).
type CreatePersonalTokenRequest struct {
	Name      string `json:"name"`
	ExpiresAt string `json:"expiresAt"`
}

// Validate enforces name 1-64 chars and a parseable expiresAt (when present).
func (r CreatePersonalTokenRequest) Validate() error {
	var fields []errs.FieldError
	if n := len([]rune(strings.TrimSpace(r.Name))); n < 1 || n > 64 {
		fields = append(fields, errs.FieldError{Key: "name", Message: "Token name must be 1-64 characters"})
	}
	if r.ExpiresAt != "" {
		if _, err := time.Parse(datetime.Layout, r.ExpiresAt); err != nil {
			fields = append(fields, errs.FieldError{Key: "expiresAt", Message: "Invalid expiration date"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// CreatePersonalTokenResult carries the raw token — the ONLY response that
// ever contains it.
type CreatePersonalTokenResult struct {
	Id        string  `json:"id"`
	Name      string  `json:"name"`
	Token     string  `json:"token"`
	CreatedAt string  `json:"createdAt"`
	ExpiresAt *string `json:"expiresAt"`
}

// RevokePersonalTokenRequest is the revoke-personal-token request body.
type RevokePersonalTokenRequest struct {
	Id string `json:"id"`
}

// Validate enforces id NotBlank.
func (r RevokePersonalTokenRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// RevokePersonalTokenResult is the revoke-personal-token response (empty object).
type RevokePersonalTokenResult struct{}
