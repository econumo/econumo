package admin

import (
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
	until := ""
	if r.AccessUntil != nil {
		until = r.AccessUntil.UTC().Format(datetime.Layout)
	}
	return model.AdminUserView{
		Id:                   r.ID,
		Name:                 r.Name,
		Email:                r.Email,
		AccessLevel:          string(r.AccessLevel),
		AccessUntil:          until,
		EffectiveAccessLevel: string(model.EffectiveAccessLevel(r.AccessLevel, r.AccessUntil, s.clock.Now())),
	}
}
