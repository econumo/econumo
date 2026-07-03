// This file holds the ElementType/UserRole value objects. Names
// (budget/folder/envelope) reuse the generic 3-64 rule, so they are plain
// validated strings via the shared name helper rather than dedicated types.
package budget

import "github.com/econumo/econumo/internal/shared/errs"

// ElementType is a budget element's kind: envelope=0, category=1, tag=2.
type ElementType int16

const (
	ElementEnvelope ElementType = 0
	ElementCategory ElementType = 1
	ElementTag      ElementType = 2
)

var elementAliases = [...]string{ElementEnvelope: "envelope", ElementCategory: "category", ElementTag: "tag"}

// ElementTypeFromAlias parses an element type alias.
func ElementTypeFromAlias(alias string) (ElementType, error) {
	for v, a := range elementAliases {
		if a == alias {
			return ElementType(v), nil
		}
	}
	return 0, errs.NewValidation("Validation failed", errs.FieldError{
		Key: "type", Message: "BudgetElementType with alias " + alias + " not exists", Code: "VALIDATION_ERROR",
	})
}

// Alias returns the wire alias for the element type.
func (t ElementType) Alias() string {
	if int(t) < 0 || int(t) >= len(elementAliases) {
		return ""
	}
	return elementAliases[t]
}

func (t ElementType) Int16() int16 { return int16(t) }

// UserRole is a budget participant's role: owner=-1, admin=0, user=1, guest=2.
// owner is synthetic — never stored (only admin/user/guest are persisted); the
// meta builder stamps the budget's owner with it.
type UserRole int16

const (
	RoleOwner UserRole = -1
	RoleAdmin UserRole = 0
	RoleUser  UserRole = 1
	RoleGuest UserRole = 2
)

// roleAliases maps the four roles to their wire aliases. Stored roles are only
// admin/user/guest; owner is presentation-only.
var roleAliasByValue = map[UserRole]string{
	RoleOwner: "owner", RoleAdmin: "admin", RoleUser: "user", RoleGuest: "guest",
}

// RoleFromAlias parses a role alias (admin/user/guest; owner is not a valid
// input role).
func RoleFromAlias(alias string) (UserRole, error) {
	for v, a := range roleAliasByValue {
		if v == RoleOwner {
			continue
		}
		if a == alias {
			return v, nil
		}
	}
	return 0, errs.NewValidation("Validation failed", errs.FieldError{
		Key: "role", Message: "BudgetUserRole with alias " + alias + " not exists", Code: "VALIDATION_ERROR",
	})
}

// Alias returns the wire alias for the role.
func (r UserRole) Alias() string { return roleAliasByValue[r] }

func (r UserRole) Int16() int16 { return int16(r) }

// ValidateName enforces the generic 3-64 char rule shared by budget, folder, and
// envelope names. label personalizes the error ("Budget"/"Folder"/"Envelope").
func ValidateName(label, name string) error {
	if n := len([]rune(name)); n < 3 || n > 64 {
		return errs.NewValidation("Validation failed", errs.FieldError{
			Key: "name", Message: label + " name must be 3-64 characters", Code: "VALIDATION_ERROR",
		})
	}
	return nil
}
