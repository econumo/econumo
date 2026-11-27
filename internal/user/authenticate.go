// Authenticate verifies an opaque bearer token against the access_tokens
// store; it is the hot path behind every authenticated request.
package user

import (
	"context"
	"fmt"
	"log/slog"

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
		// The stamp goes first: an inactivity mark holds the user row lock for
		// its whole transaction, so the stamp either lands before the mark's
		// re-check (and prevents it) or waits behind it, after which the touch
		// below matches the revoked token and the request is refused. A stamp
		// failure must not sign the user out.
		if err := s.stampActive(ctx, t.UserID, now); err != nil {
			slog.WarnContext(ctx, "activity stamp failed", "user_id", t.UserID.String(), "err_type", fmt.Sprintf("%T", err))
		}
		t.Touch(now, SessionTTL)
		n, err := s.tokens.Touch(ctx, t.ID, t.LastUsedAt, t.ExpiresAt)
		if err != nil {
			return model.Principal{}, err
		}
		// The touch only matches an unrevoked row, so zero rows means a reclaim
		// revoked (or purged) this credential between the read above and the
		// write: the request is holding a token that no longer authenticates.
		if n == 0 {
			return model.Principal{}, errs.NewUnauthorized("Invalid access token")
		}
	}
	u := model.User{AccessLevel: level, AccessUntil: until}
	return model.Principal{UserID: t.UserID, TokenID: t.ID, Level: u.EffectiveAccessLevel(now), Scope: t.Scope}, nil
}
