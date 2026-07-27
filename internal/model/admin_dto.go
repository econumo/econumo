// This file holds the admin listener's request/result DTOs. The listener is
// private, unversioned, and single-consumer (the payment portal), so these are
// not part of the frozen public wire contract.
package model

import (
	"github.com/econumo/econumo/internal/shared/errs"
)

// AdminSetAccessRequest addresses a user by id only: every purchase originates
// from a handoff link, so the portal holds the id before checkout. Until is a
// pointer so an explicit JSON null is distinguishable from an absent field;
// both mean "no expiry".
type AdminSetAccessRequest struct {
	UserId string  `json:"userId"`
	Level  string  `json:"level"`
	Until  *string `json:"until"`
}

// Validate checks presence only. Format checks on level and until live in
// admin.SetAccess, which needs the parsed values anyway — a single parse site
// keeps validation and use from drifting apart.
func (r AdminSetAccessRequest) Validate() error {
	if r.UserId == "" {
		return errs.NewValidation("Form validation error",
			errs.FieldError{Key: "userId", Message: "This value should not be blank."})
	}
	return nil
}

// AdminUserView carries the stored access columns AND the effective level. The
// portal needs the raw level to tell a LAPSED user (offer a purchase) from a
// MANUALLY RESTRICTED one (do not), and the effective level to answer "can this
// person write right now" without re-implementing the collapse rule.
type AdminUserView struct {
	Id                   string `json:"id"`
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Avatar               string `json:"avatar"`
	AccessLevel          string `json:"accessLevel"`
	AccessUntil          string `json:"accessUntil"`
	EffectiveAccessLevel string `json:"effectiveAccessLevel"`
}

type AdminUserContextResult struct {
	User        AdminUserView   `json:"user"`
	Connections []AdminUserView `json:"connections"`
}
