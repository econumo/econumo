// Package connection is the connection feature: the AccountAccess entity (a
// per-account grant of a Role to a connected user), the Role value object
// (admin=0, user=1, guest=2), and the repository interfaces (domain), plus the
// request/result DTOs (with their tier-1 Validate() methods) and the Service
// (which owns the tx boundary and builds the response-shaped *Result directly).
// The live endpoints are set-account-access, revoke-account-access (mutations,
// owner-admin only) and get-connection-list (read); generate-invite,
// delete-invite, accept-invite and delete-connection are implemented here too
// (enabled in the cloud edition). The symmetric user-to-user link itself
// carries no behavior beyond existence, so it is modeled as plain ids at the
// repository boundary rather than an entity.
//
// JSON field names are frozen to the existing API wire contract; see
// CLAUDE.md.
package connection

import (
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Role is the access level a connected user has on a shared account.
// admin=0, user=1, guest=2 — the stored numeric values and wire aliases are a
// frozen contract.
type Role int16

const (
	RoleAdmin Role = 0
	RoleUser  Role = 1
	RoleGuest Role = 2
)

// roleAliases maps role -> wire alias (and back). Order matters: index == value.
var roleAliases = [...]string{RoleAdmin: "admin", RoleUser: "user", RoleGuest: "guest"}

// RoleFromAlias parses a role alias ("admin"/"user"/"guest"). Unknown aliases
// return a validation error.
func RoleFromAlias(alias string) (Role, error) {
	for v, a := range roleAliases {
		if a == alias {
			return Role(v), nil
		}
	}
	return 0, errs.NewValidation("Validation failed", errs.FieldError{
		Key: "role", Message: "AccountRole with alias " + alias + " not exists", Code: "VALIDATION_ERROR",
	})
}

// Alias returns the wire alias for the role.
func (r Role) Alias() string {
	if int(r) < 0 || int(r) >= len(roleAliases) {
		return ""
	}
	return roleAliases[r]
}

func (r Role) Int16() int16 { return int16(r) }

// AccountAccess is a grant: connected user UserID may act on account
// AccountID with Role. The owner of the account is NOT this user. Fields are
// exported for direct read access; the only write after construction goes
// through UpdateRole.
type AccountAccess struct {
	AccountID vo.Id
	UserID    vo.Id
	Role      Role
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewAccountAccess creates a fresh grant (CreatedAt == UpdatedAt == now).
func NewAccountAccess(accountID, userID vo.Id, role Role, now time.Time) *AccountAccess {
	return &AccountAccess{AccountID: accountID, UserID: userID, Role: role, CreatedAt: now, UpdatedAt: now}
}

// UpdateRole changes the role, bumping UpdatedAt only when it actually changes.
func (a *AccountAccess) UpdateRole(role Role, now time.Time) {
	if a.Role != role {
		a.Role = role
		a.UpdatedAt = now
	}
}
