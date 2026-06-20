// Package connection is the connection module's application layer. In the
// self-hosted product (EconumoBundle) four endpoints are stubs returning 501
// ("Not supported in Econumo One") -- generate-invite, delete-invite,
// accept-invite, delete-connection. The three live endpoints are
// set-account-access, revoke-account-access (mutations, owner-admin only) and
// get-connection-list (read). This file holds their request/result DTOs with
// tier-1 Validate(). JSON field names are frozen to the wire contract.
package connection

import (
	"strings"

	"github.com/econumo/econumo/internal/domain/shared/errs"
)

// UserResult is the embedded connected-user shape: {id, avatar, name}.
type UserResult struct {
	Id     string `json:"id"`
	Avatar string `json:"avatar"`
	Name   string `json:"name"`
}

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

// ---------------------------------------------------------------------------
// get-connection-list
// ---------------------------------------------------------------------------

// GetConnectionListResult is the response: {items: [...]}.
type GetConnectionListResult struct {
	Items []ConnectionResult `json:"items"`
}

// ---------------------------------------------------------------------------
// set-account-access
// ---------------------------------------------------------------------------

// SetAccountAccessRequest grants/updates a connected user's role on an owned
// account.
type SetAccountAccessRequest struct {
	AccountId string `json:"accountId"`
	UserId    string `json:"userId"`
	Role      string `json:"role"`
}

// Validate enforces NotBlank on accountId/userId/role (UUID + role-alias
// validity are checked in the service via the value-object constructors).
func (r SetAccountAccessRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"accountId", r.AccountId}, {"userId", r.UserId}, {"role", r.Role},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// SetAccountAccessResult is the (empty) response.
type SetAccountAccessResult struct{}

// ---------------------------------------------------------------------------
// revoke-account-access
// ---------------------------------------------------------------------------

// RevokeAccountAccessRequest removes a connected user's grant on an owned
// account.
type RevokeAccountAccessRequest struct {
	AccountId string `json:"accountId"`
	UserId    string `json:"userId"`
}

// Validate enforces NotBlank on accountId/userId.
func (r RevokeAccountAccessRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{
		{"accountId", r.AccountId}, {"userId", r.UserId},
	} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// RevokeAccountAccessResult is the (empty) response.
type RevokeAccountAccessResult struct{}
