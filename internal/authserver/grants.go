package authserver

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

func grantNotFound() error {
	return &errs.ValidationError{Msg: "Connected app not found.", MsgCode: errs.CodeAuthServerGrantNotFound}
}

// The user row lock comes first, matching reclaim and deactivate (users, then
// access_tokens, then oauth_grants); the opposite order deadlocks on PostgreSQL.
// revokeGrant reports whether this call revoked the grant, as opposed to
// finding it already revoked.
func (s *Service) revokeGrant(ctx context.Context, userID, id vo.Id, now time.Time) (bool, error) {
	var n int64
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := s.creds.LockForOAuth(ctx, userID); err != nil {
			return err
		}
		var err error
		if n, err = s.repo.RevokeGrant(ctx, id, now); err != nil {
			return err
		}
		return s.creds.RevokeOAuthGrantTokens(ctx, id)
	})
	return n > 0, err
}

func (s *Service) ListConnectedApps(ctx context.Context, userID vo.Id) ([]model.ConnectedAppResult, error) {
	rows, err := s.repo.ListUnrevokedGrants(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := []model.ConnectedAppResult{}
	for _, r := range rows {
		if !r.Grant.IsLive(now) {
			continue
		}
		host, loopback := "", false
		if len(r.RedirectURIs) > 0 {
			host, loopback = RedirectHost(r.RedirectURIs[0])
		}
		out = append(out, model.ConnectedAppResult{
			ID: r.Grant.ID.String(), ClientName: r.ClientName, RedirectHost: host, IsLoopback: loopback, IsOfficial: r.ClientIsGlobal,
			CreatedAt: r.Grant.CreatedAt.UTC().Format(datetime.Layout), LastUsedAt: r.Grant.LastUsedAt.UTC().Format(datetime.Layout),
		})
	}
	return out, nil
}

func (s *Service) RevokeConnectedApp(ctx context.Context, userID, grantID vo.Id) error {
	g, err := s.repo.GetGrant(ctx, grantID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return grantNotFound()
		}
		return err
	}
	if !g.UserID.Equal(userID) || g.RevokedAt != nil {
		return grantNotFound()
	}
	reqctx.AddLogAttr(ctx, "grant_id", grantID.String())
	_, err = s.revokeGrant(ctx, g.UserID, grantID, s.clock.Now())
	return err
}

// RevokeAllForUser runs inside the reclaim/deactivate transaction, whose own
// token sweep already revokes every oauth access token.
func (s *Service) RevokeAllForUser(ctx context.Context, userID vo.Id) (int64, error) {
	return s.repo.RevokeUserGrants(ctx, userID, s.clock.Now())
}
