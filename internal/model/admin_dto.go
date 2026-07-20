// This file holds the admin listener's request/result DTOs. The listener is
// private, unversioned, and single-consumer (the payment portal), so these are
// not part of the frozen public wire contract.
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
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

func (r AdminSetAccessRequest) Validate() error {
	var fields []errs.FieldError
	if r.UserId == "" {
		fields = append(fields, errs.FieldError{Key: "userId", Message: "This value should not be blank."})
	}
	if _, err := ParseAccessLevel(r.Level); err != nil {
		fields = append(fields, errs.FieldError{Key: "level", Message: "Level must be full or readonly"})
	}
	if r.Until != nil && *r.Until != "" {
		if _, err := time.Parse(datetime.Layout, *r.Until); err != nil {
			fields = append(fields, errs.FieldError{Key: "until", Message: "Until must be formatted as " + datetime.Layout})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Form validation error", fields...)
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
	AccessLevel          string `json:"accessLevel"`
	AccessUntil          string `json:"accessUntil"`
	EffectiveAccessLevel string `json:"effectiveAccessLevel"`
}

type AdminUserContextResult struct {
	User        AdminUserView   `json:"user"`
	Connections []AdminUserView `json:"connections"`
}
