// Authenticate verifies an opaque bearer token against the access_tokens
// store; it is the hot path behind every authenticated request.
package user

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func (s *Service) Authenticate(ctx context.Context, raw string) (model.Principal, error) {
	t, level, until, err := s.tokens.GetByHash(ctx, HashAccessToken(raw))
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return model.Principal{}, errs.NewUnauthorized("Invalid access token")
		}
		return model.Principal{}, err
	}
	now := s.clock.Now()
	if !t.IsLive(now) {
		return model.Principal{}, errs.NewUnauthorized("Invalid access token")
	}
	if t.NeedsTouch(now, touchInterval) {
		t.Touch(now, SessionTTL)
		if err := s.tokens.Update(ctx, t); err != nil {
			return model.Principal{}, err
		}
	}
	u := model.User{AccessLevel: level, AccessUntil: until}
	return model.Principal{UserID: t.UserID, TokenID: t.ID, Level: u.EffectiveAccessLevel(now), Scope: t.Scope}, nil
}
