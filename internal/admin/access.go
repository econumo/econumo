package admin

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SetAccess is naturally idempotent — it assigns rather than accumulates — so
// the portal's retrying Stripe webhook needs no operation guard.
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
		return nil, err
	}
	var until *time.Time
	if req.Until != nil && *req.Until != "" {
		t, perr := time.ParseInLocation(datetime.Layout, *req.Until, time.UTC)
		if perr != nil {
			return nil, perr
		}
		until = &t
	}
	if err := s.users.SetAccess(ctx, id, level, until); err != nil {
		return nil, err
	}
	rec, err := s.users.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	v := s.view(rec)
	return &v, nil
}
