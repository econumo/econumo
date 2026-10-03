package authserver

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

var errInvalidGrant = &OAuthError{400, "invalid_grant", "the grant is invalid, expired or revoked"}

func (s *Service) Token(ctx context.Context, req model.TokenRequest) (model.TokenResponse, error) {
	if !s.Enabled() {
		return model.TokenResponse{}, &OAuthError{404, "invalid_request", "not available"}
	}
	switch req.GrantType {
	case "authorization_code":
		return s.exchange(ctx, req)
	case "refresh_token":
		return s.refresh(ctx, req)
	}
	return model.TokenResponse{}, &OAuthError{400, "unsupported_grant_type", "unsupported grant_type"}
}

// A confidential client must present its secret (constant-time compare of
// hashes); a public client has none to present.
func (s *Service) authenticateClient(ctx context.Context, clientID, secret string) (*model.OAuthClient, error) {
	bad := errBadClient
	id, err := vo.ParseId(clientID)
	if err != nil {
		return nil, bad
	}
	c, err := s.repo.GetClient(ctx, id)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, bad
		}
		return nil, err
	}
	if c.SecretHash != nil && subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(*c.SecretHash)) != 1 {
		return nil, bad
	}
	return c, nil
}

var errBadClient = &OAuthError{401, "invalid_client", "client authentication failed"}

func missingParam(name string) *OAuthError {
	return &OAuthError{400, "invalid_request", name + " is required"}
}

func (s *Service) exchange(ctx context.Context, req model.TokenRequest) (model.TokenResponse, error) {
	if req.ClientID == "" {
		return model.TokenResponse{}, errBadClient
	}
	switch {
	case req.Code == "":
		return model.TokenResponse{}, missingParam("code")
	case req.RedirectURI == "":
		return model.TokenResponse{}, missingParam("redirect_uri")
	case req.CodeVerifier == "":
		return model.TokenResponse{}, missingParam("code_verifier")
	}
	c, err := s.authenticateClient(ctx, req.ClientID, req.ClientSecret)
	if err != nil {
		return model.TokenResponse{}, err
	}
	now := s.clock.Now()
	// Consumed in its own statement before any check, so every attempt,
	// including a failed one, burns the code.
	code, err := s.repo.ConsumeCode(ctx, hashSecret(req.Code))
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return model.TokenResponse{}, errInvalidGrant
		}
		return model.TokenResponse{}, err
	}
	if !now.Before(code.ExpiresAt) || !code.ClientID.Equal(c.ID) || code.RedirectURI != req.RedirectURI || !VerifyPKCE(req.CodeVerifier, code.CodeChallenge) {
		return model.TokenResponse{}, errInvalidGrant
	}
	if !s.resourceOK(req.Resource) {
		return model.TokenResponse{}, &OAuthError{400, "invalid_target", "unknown resource"}
	}
	var resp model.TokenResponse
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := s.creds.LockForOAuth(ctx, code.UserID); err != nil {
			return err
		}
		rawRefresh, refreshHash, err := newSecret()
		if err != nil {
			return err
		}
		g := &model.OAuthGrant{ID: vo.NewId(), UserID: code.UserID, ClientID: c.ID, RefreshTokenHash: refreshHash,
			CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(GrantIdleTTL)}
		if err := s.repo.InsertGrant(ctx, g); err != nil {
			return err
		}
		// The generation captured at approval, not the current one: a reclaim
		// that landed since then must make this write nothing.
		access, ok, err := s.creds.IssueOAuthAccessToken(ctx, code.UserID, g.ID, c.Name, code.CredentialsGeneration, AccessTokenTTL)
		if err != nil {
			return err
		}
		if !ok {
			return errInvalidGrant
		}
		// Re-authorizing a client replaces the user's earlier connection to it.
		replaced, err := s.repo.RevokeOtherGrants(ctx, code.UserID, c.ID, g.ID, now)
		if err != nil {
			return err
		}
		for _, id := range replaced {
			if err := s.creds.RevokeOAuthGrantTokens(ctx, id); err != nil {
				return err
			}
		}
		resp = tokenResponse(access, rawRefresh)
		reqctx.AddLogAttr(ctx, "grant_id", g.ID.String())
		reqctx.AddLogAttr(ctx, "user_id", code.UserID.String())
		return nil
	})
	if err != nil {
		return model.TokenResponse{}, err
	}
	reqctx.AddLogAttr(ctx, "client_id", c.ID.String())
	s.housekeeping(ctx, now)
	return resp, nil
}

func tokenResponse(access, refresh string) model.TokenResponse {
	return model.TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int(AccessTokenTTL / time.Second), RefreshToken: refresh, Scope: Scope}
}

func (s *Service) housekeeping(ctx context.Context, now time.Time) {
	if err := s.repo.PurgeExpiredCodes(ctx, now); err != nil {
		slog.WarnContext(ctx, "oauth code purge failed", "err", err)
	}
	if _, err := s.repo.DeleteDeadGrants(ctx, now.Add(-DeadRetention)); err != nil {
		slog.WarnContext(ctx, "oauth grant purge failed", "err", err)
	}
	if _, err := s.creds.PurgeDeadOAuthTokens(ctx, now.Add(-DeadRetention)); err != nil {
		slog.WarnContext(ctx, "oauth access token purge failed", "err", err)
	}
}

func (s *Service) refresh(ctx context.Context, req model.TokenRequest) (model.TokenResponse, error) {
	if req.ClientID == "" {
		return model.TokenResponse{}, errBadClient
	}
	if req.RefreshToken == "" {
		return model.TokenResponse{}, missingParam("refresh_token")
	}
	c, err := s.authenticateClient(ctx, req.ClientID, req.ClientSecret)
	if err != nil {
		return model.TokenResponse{}, err
	}
	if !s.resourceOK(req.Resource) {
		return model.TokenResponse{}, &OAuthError{400, "invalid_target", "unknown resource"}
	}
	now := s.clock.Now()
	hash := hashSecret(req.RefreshToken)
	g, err := s.repo.GetGrantByRefreshHash(ctx, hash)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return model.TokenResponse{}, s.handleRotatedRefresh(ctx, hash, c.ID, now)
		}
		return model.TokenResponse{}, err
	}
	if !g.ClientID.Equal(c.ID) || !g.IsLive(now) {
		return model.TokenResponse{}, errInvalidGrant
	}
	var resp model.TokenResponse
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		gen, err := s.creds.LockForOAuth(ctx, g.UserID)
		if err != nil {
			return err
		}
		rawRefresh, newHash, err := newSecret()
		if err != nil {
			return err
		}
		// Conditional on the presented hash still being current and the grant
		// unrevoked: a concurrent refresh, or a reclaim that won the lock first,
		// makes this match nothing.
		n, err := s.repo.RotateGrant(ctx, g.ID, hash, newHash, now, now.Add(GrantIdleTTL))
		if err != nil {
			return err
		}
		if n == 0 {
			return errInvalidGrant
		}
		if err := s.repo.InsertSpentRefreshHash(ctx, g.ID, hash, now); err != nil {
			return err
		}
		access, ok, err := s.creds.IssueOAuthAccessToken(ctx, g.UserID, g.ID, c.Name, gen, AccessTokenTTL)
		if err != nil {
			return err
		}
		if !ok {
			return errInvalidGrant
		}
		resp = tokenResponse(access, rawRefresh)
		return nil
	})
	if err != nil {
		return model.TokenResponse{}, err
	}
	reqctx.AddLogAttr(ctx, "client_id", c.ID.String())
	reqctx.AddLogAttr(ctx, "grant_id", g.ID.String())
	reqctx.AddLogAttr(ctx, "user_id", g.UserID.String())
	return resp, nil
}

// A refresh token already rotated away, found in the spent table however many
// rotations ago. Only the token rotated away MOST RECENTLY, and only inside
// the grace window, is a concurrent refresh (two processes sharing stored
// credentials) or a retry whose response was lost, so it is only refused.
// Any other spent token, at any age, is circulating somewhere it should not
// (a thief can rotate more than once before the legitimate client retries),
// so the whole grant goes.
func (s *Service) handleRotatedRefresh(ctx context.Context, hash string, clientID vo.Id, now time.Time) error {
	g, err := s.repo.GetGrantBySpentRefreshHash(ctx, hash)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return errInvalidGrant
		}
		return err
	}
	if !g.ClientID.Equal(clientID) || g.RevokedAt != nil {
		return errInvalidGrant
	}
	latest := g.PrevRefreshTokenHash != nil && *g.PrevRefreshTokenHash == hash
	if latest && g.RotatedAt != nil && now.Sub(*g.RotatedAt) <= RefreshGrace {
		slog.WarnContext(ctx, "oauth-refresh-reuse", "grant_id", g.ID.String(), "revoked", false)
		return errInvalidGrant
	}
	if err := s.revokeGrant(ctx, g.UserID, g.ID, now); err != nil {
		return err
	}
	slog.WarnContext(ctx, "oauth-refresh-reuse", "grant_id", g.ID.String(), "revoked", true)
	return errInvalidGrant
}
