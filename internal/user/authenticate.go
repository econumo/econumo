// Authenticate verifies an opaque bearer token against the access_tokens
// store; it is the hot path behind every authenticated request.
package user

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) Authenticate(ctx context.Context, raw string) (vo.Id, vo.Id, model.AccessLevel, error) {
	t, level, until, err := s.tokens.GetByHash(ctx, HashAccessToken(raw))
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return vo.Id{}, vo.Id{}, "", errs.NewUnauthorized("Invalid access token")
		}
		return vo.Id{}, vo.Id{}, "", err
	}
	now := s.clock.Now()
	if !t.IsLive(now) {
		return vo.Id{}, vo.Id{}, "", errs.NewUnauthorized("Invalid access token")
	}
	if t.NeedsTouch(now, touchInterval) {
		t.Touch(now, SessionTTL)
		n, err := s.tokens.Touch(ctx, t.ID, t.LastUsedAt, t.ExpiresAt)
		if err != nil {
			return vo.Id{}, vo.Id{}, "", err
		}
		// The touch only matches an unrevoked row, so zero rows means a reclaim
		// revoked (or purged) this credential between the read above and the
		// write: the request is holding a token that no longer authenticates.
		if n == 0 {
			return vo.Id{}, vo.Id{}, "", errs.NewUnauthorized("Invalid access token")
		}
	}
	u := model.User{AccessLevel: level, AccessUntil: until}
	return t.UserID, t.ID, u.EffectiveAccessLevel(now), nil
}
