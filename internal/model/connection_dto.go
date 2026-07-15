// This file holds the connection module's request/result DTOs with tier-1
// Validate(). JSON field names are frozen to the wire contract.
package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

// AccountAccessResult is one shared-account grant in the connection list:
// {id (the account id), ownerUserId, role (alias)}.
type AccountAccessResult struct {
	Id          string `json:"id"`
	OwnerUserId string `json:"ownerUserId"`
	Role        string `json:"role"`
}

// ConnectionResult is one connected user plus the accounts shared between them
// and the requesting user.
type ConnectionResult struct {
	User           UserResult            `json:"user"`
	SharedAccounts []AccountAccessResult `json:"sharedAccounts"`
}

// GetConnectionListResult is the response: {items: [...]}.
type GetConnectionListResult struct {
	Items []ConnectionResult `json:"items"`
}

// GenerateInviteRequest has no body fields; the invite is keyed by the
// authenticated user.
type GenerateInviteRequest struct{}

// Validate is a no-op (no fields).
func (r GenerateInviteRequest) Validate() error { return nil }

// ConnectionInviteResult is {code, expiredAt} — the generated invite.
type ConnectionInviteResult struct {
	Code      string `json:"code"`
	ExpiredAt string `json:"expiredAt"`
}

// GenerateInviteResult is the response: {item: {code, expiredAt}}.
type GenerateInviteResult struct {
	Item ConnectionInviteResult `json:"item"`
}

// DeleteInviteRequest has no body fields.
type DeleteInviteRequest struct{}

// Validate is a no-op.
func (r DeleteInviteRequest) Validate() error { return nil }

// DeleteInviteResult is the (empty) response.
type DeleteInviteResult struct{}

// AcceptInviteRequest carries the invite code to redeem (NotBlank).
type AcceptInviteRequest struct {
	Code string `json:"code"`
}

// Validate enforces NotBlank on code (the 5-char length is enforced in the
// service via the ConnectionCode constructor).
func (r AcceptInviteRequest) Validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{
			Key: "code", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR",
		})
	}
	return nil
}

// AcceptInviteResult is the response: {items: [...connections...]} — the same
// connection-list shape get-connection-list returns.
type AcceptInviteResult struct {
	Items []ConnectionResult `json:"items"`
}

// DeleteConnectionRequest carries the connected user's id to disconnect from.
type DeleteConnectionRequest struct {
	Id string `json:"id"`
}

// Validate enforces NotBlank on id (UUID validity checked in the service).
func (r DeleteConnectionRequest) Validate() error {
	if strings.TrimSpace(r.Id) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{
			Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR",
		})
	}
	return nil
}

// DeleteConnectionResult is the (empty) response.
type DeleteConnectionResult struct{}
