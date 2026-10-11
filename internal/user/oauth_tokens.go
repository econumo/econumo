// Tokens the MCP OAuth server mints. They live in access_tokens so the
// per-request authenticator and every revocation cascade see them, but only
// the /mcp edge admits their scope (see middleware.Auth).
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// LockForOAuth takes the user row lock in the caller's transaction and returns
// the credentials generation as read under it, so a grant approval or refresh
// can fence its token mint against a concurrent reclaim.
func (s *Service) LockForOAuth(ctx context.Context, userID vo.Id) (int64, error) {
	if err := s.repo.LockRow(ctx, userID); err != nil {
		return 0, err
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return 0, err
	}
	return u.CredentialsGeneration, nil
}

// IsTokenLive reports whether the user's access token is still unrevoked and
// unexpired. Called under LockForOAuth, it tells an approval whether a
// reclaim revoked the presenting session after the auth middleware accepted it.
func (s *Service) IsTokenLive(ctx context.Context, userID, tokenID vo.Id) (bool, error) {
	t, err := s.tokens.GetByID(ctx, tokenID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return false, nil
		}
		return false, err
	}
	return t.UserID.Equal(userID) && t.IsLive(s.clock.Now()), nil
}

// IssueOAuthAccessToken mints an eco_oat_ token for the grant. The expiry is
// fixed at ttl (Touch slides sessions only). ok=false means the generation
// fence matched no row: a reclaim landed after the caller read its evidence.
func (s *Service) IssueOAuthAccessToken(ctx context.Context, userID, grantID vo.Id, name string, generation int64, ttl time.Duration) (string, bool, error) {
	raw, hash, err := generateAccessToken(model.TokenKindOAuth)
	if err != nil {
		return "", false, err
	}
	now := s.clock.Now()
	exp := now.Add(ttl)
	t := &model.AccessToken{
		ID: vo.NewId(), UserID: userID, Kind: model.TokenKindOAuth, TokenHash: hash,
		Scope: model.TokenScopeMCP, Name: &name, CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp, GrantID: &grantID,
	}
	n, err := s.tokens.InsertOAuthIfGeneration(ctx, t, generation)
	if err != nil {
		return "", false, err
	}
	if n != 1 {
		return "", false, nil
	}
	return raw, true, nil
}

// RevokeOAuthGrantTokens revokes every live access token minted for the grant.
func (s *Service) RevokeOAuthGrantTokens(ctx context.Context, grantID vo.Id) error {
	return s.tokens.RevokeByGrant(ctx, grantID, s.clock.Now())
}

// PurgeDeadOAuthTokens deletes oauth access tokens revoked or expired before
// cutoff. They expire within the hour, so the login-path purge, which walks
// rows one by one, is not where they go.
func (s *Service) PurgeDeadOAuthTokens(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.tokens.DeleteDeadOAuth(ctx, cutoff)
}
