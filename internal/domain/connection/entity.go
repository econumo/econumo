// Package connection is the connection aggregate's domain layer: the
// AccountAccess entity (a per-account grant of a Role to a connected user) and
// the AccountUserRole value object (admin=0, user=1, guest=2). The symmetric
// users_connections link itself carries no behavior beyond existence, so it is
// modeled as plain ids at the repository boundary rather than an entity.
package connection

import (
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Role is the access level a connected user has on a shared account.
// admin=0, user=1, guest=2 (matches PHP AccountUserRole int values + aliases).
type Role int16

const (
	RoleAdmin Role = 0
	RoleUser  Role = 1
	RoleGuest Role = 2
)

// roleAliases maps role -> wire alias (and back). Order matters: index == value.
var roleAliases = [...]string{RoleAdmin: "admin", RoleUser: "user", RoleGuest: "guest"}

// RoleFromAlias parses a role alias ("admin"/"user"/"guest"). Unknown aliases
// return a validation error matching the PHP DomainException path.
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

// Int16 returns the stored numeric value.
func (r Role) Int16() int16 { return int16(r) }

// AccountAccess is a grant: connected user `userID` may act on account
// `accountID` with `role`. The owner of the account is NOT this user.
type AccountAccess struct {
	accountID vo.Id
	userID    vo.Id
	role      Role
	createdAt time.Time
	updatedAt time.Time
}

// NewAccountAccess creates a fresh grant (created_at == updated_at == now).
func NewAccountAccess(accountID, userID vo.Id, role Role, now time.Time) *AccountAccess {
	return &AccountAccess{accountID: accountID, userID: userID, role: role, createdAt: now, updatedAt: now}
}

// FromState rehydrates an AccountAccess from storage.
func FromState(accountID, userID vo.Id, role Role, createdAt, updatedAt time.Time) *AccountAccess {
	return &AccountAccess{accountID: accountID, userID: userID, role: role, createdAt: createdAt, updatedAt: updatedAt}
}

// UpdateRole changes the role, bumping updated_at only when it actually changes
// (mirrors PHP AccountAccess::updateRole).
func (a *AccountAccess) UpdateRole(role Role, now time.Time) {
	if a.role != role {
		a.role = role
		a.updatedAt = now
	}
}

func (a *AccountAccess) AccountId() vo.Id     { return a.accountID }
func (a *AccountAccess) UserId() vo.Id        { return a.userID }
func (a *AccountAccess) Role() Role           { return a.role }
func (a *AccountAccess) CreatedAt() time.Time { return a.createdAt }
func (a *AccountAccess) UpdatedAt() time.Time { return a.updatedAt }
