package admin

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SetAccess is naturally idempotent — it assigns rather than accumulates — so
// the portal's retrying Stripe webhook needs no operation guard. level and
// until are parsed HERE, not in the DTO's Validate: one parse site means the
// accepted formats cannot drift between validation and use.
func (s *Service) SetAccess(ctx context.Context, req model.AdminSetAccessRequest) (*model.AdminUserView, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	id, err := vo.ParseId(req.UserId)
	if err != nil {
		return nil, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "userId", Message: "Invalid user id"})
	}
	level, err := model.ParseAccessLevel(req.Level)
	if err != nil {
		return nil, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "level", Message: "Level must be full or readonly"})
	}
	var until *time.Time
	if req.Until != nil && *req.Until != "" {
		t, perr := time.ParseInLocation(datetime.Layout, *req.Until, time.UTC)
		if perr != nil {
			return nil, errs.NewValidation("Form validation error",
				errs.FieldError{Key: "until", Message: "Until must be formatted as " + datetime.Layout})
		}
		until = &t
	}
	// Logged BEFORE the write: a failed attempt must still record what was
	// attempted, on the WARN/ERROR operation line. The old values are added by
	// the user service, which has the record in hand.
	reqctx.AddLogAttr(ctx, "user_id", id.String())
	reqctx.AddLogAttr(ctx, "access_level", string(level))
	reqctx.AddLogAttr(ctx, "access_until", formatUntil(until))
	rec, err := s.users.SetAccess(ctx, id, level, until)
	if err != nil {
		return nil, err
	}
	v := s.view(rec)
	return &v, nil
}
