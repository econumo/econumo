package admin

import (
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/port"
)

type Service struct {
	users UserLookup
	conns ConnectionLookup
	clock port.Clock
}

func NewService(users UserLookup, conns ConnectionLookup, clk port.Clock) *Service {
	return &Service{users: users, conns: conns, clock: clk}
}

func (s *Service) view(r UserRecord) model.AdminUserView {
	return model.AdminUserView{
		Id:                   r.ID,
		Name:                 r.Name,
		Email:                r.Email,
		Avatar:               r.Avatar,
		AccessLevel:          string(r.AccessLevel),
		AccessUntil:          formatUntil(r.AccessUntil),
		EffectiveAccessLevel: string(model.EffectiveAccessLevel(r.AccessLevel, r.AccessUntil, s.clock.Now())),
	}
}

// formatUntil renders a nullable expiry as the wire/log value: the frozen
// datetime layout, or "" for no expiry.
func formatUntil(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(datetime.Layout)
}
