// Authenticate verifies an opaque bearer token against the access_tokens
// store; it is the hot path behind every authenticated request.
package user

import (
	"context"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) Authenticate(ctx context.Context, raw string) (vo.Id, vo.Id, error) {
	t, err := s.tokens.GetByHash(ctx, HashAccessToken(raw))
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return vo.Id{}, vo.Id{}, errs.NewUnauthorized("Invalid access token")
		}
		return vo.Id{}, vo.Id{}, err
	}
	now := s.clock.Now()
	if !t.IsLive(now) {
		return vo.Id{}, vo.Id{}, errs.NewUnauthorized("Invalid access token")
	}
	if t.NeedsTouch(now, touchInterval) {
		t.Touch(now, SessionTTL)
		if err := s.tokens.Update(ctx, t); err != nil {
			return vo.Id{}, vo.Id{}, err
		}
	}
	return t.UserID, t.ID, nil
}
