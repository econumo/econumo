// This file holds the ElementType/BudgetRole value objects. BudgetRole is a
// distinct type from the connection feature's Role (which lacks a synthetic
// owner value) — same alias strings, different Go type. Names
// (budget/folder/envelope) reuse the generic 3-64 rule, so they are plain
// validated strings via the shared name helper rather than dedicated types.
package model

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

// BudgetRole is a budget participant's role: owner=-1, admin=0, user=1, guest=2.
// owner is synthetic — never stored (only admin/user/guest are persisted); the
// meta builder stamps the budget's owner with it.
type BudgetRole int16

const (
	BudgetRoleOwner BudgetRole = -1
	BudgetRoleAdmin BudgetRole = 0
	BudgetRoleUser  BudgetRole = 1
	BudgetRoleGuest BudgetRole = 2
)

// roleAliases maps the four roles to their wire aliases. Stored roles are only
// admin/user/guest; owner is presentation-only.
var roleAliasByValue = map[BudgetRole]string{
	BudgetRoleOwner: "owner", BudgetRoleAdmin: "admin", BudgetRoleUser: "user", BudgetRoleGuest: "guest",
}

// BudgetRoleFromAlias parses a role alias (admin/user/guest; owner is not a valid
// input role).
func BudgetRoleFromAlias(alias string) (BudgetRole, error) {
	for v, a := range roleAliasByValue {
		if v == BudgetRoleOwner {
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
func (r BudgetRole) Alias() string { return roleAliasByValue[r] }

func (r BudgetRole) Int16() int16 { return int16(r) }

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
