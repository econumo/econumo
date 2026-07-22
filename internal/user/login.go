// Login use case: authenticate by identifier and mint a session token.
package user

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// Login authenticates by identifier (md5 of the lowercased username), verifies
// the password against the stored hash+salt, mints a session token, and returns
// the token + current user. A bad username or password yields an
// UnauthorizedError (HTTP 401, "Invalid credentials.").
func (s *Service) Login(ctx context.Context, req model.LoginRequest, userAgent string, now time.Time) (*model.LoginResult, error) {
	limitKey := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeLogin, limitKey); err != nil {
		return nil, err
	}
	identifier := s.encode.Hash(strings.ToLower(req.Username))
	u, err := s.repo.GetByIdentifier(ctx, identifier)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			s.failAttempt(RateScopeLogin, limitKey)
			return nil, &errs.UnauthorizedError{Msg: "Invalid credentials.", Code: errs.CodeInvalidCredentials}
		}
		return nil, err
	}
	if !u.IsActive || !s.hasher.Verify(u.Algorithm, u.Password, req.Password, u.Salt) {
		s.failAttempt(RateScopeLogin, limitKey)
		return nil, &errs.UnauthorizedError{Msg: "Invalid credentials.", Code: errs.CodeInvalidCredentials}
	}

	// Transparently upgrade a legacy (sha512) hash to argon2id now that we hold the
	// verified plaintext. Best-effort: a failure must never block a valid login.
	s.rehashLegacyPassword(ctx, u, req.Password, now)

	email, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return nil, derr
	}
	if s.emailVerification && !u.EmailVerified {
		if err := s.requireVerifiedEmail(ctx, u, email, limitKey); err != nil {
			return nil, err
		}
	}
	if err := s.purgeDeadTokens(ctx, u.ID, now); err != nil {
		return nil, err
	}
	token, terr := s.createSession(ctx, u.ID, userAgent, now)
	if terr != nil {
		return nil, terr
	}
	cur, cerr := s.toCurrentUserWithEmail(ctx, u, email)
	if cerr != nil {
		return nil, cerr
	}
	s.clearAttempt(RateScopeLogin, limitKey)
	// Best-effort: the language preference must never block a login.
	_ = s.repo.UpdateLanguage(ctx, u.ID, reqctx.Language(ctx))
	return &model.LoginResult{Token: token, User: cur}, nil
}

// rehashLegacyPassword upgrades a still-sha512 hash to argon2id in place. It runs
// only on a verified login (the plaintext is proven correct) and is best-effort:
// any failure is logged and swallowed so a valid login always succeeds.
func (s *Service) rehashLegacyPassword(ctx context.Context, u *model.User, plaintext string, now time.Time) {
	if u.Algorithm == model.AlgorithmArgon2id {
		return
	}
	newHash, err := s.hasher.Hash(plaintext)
	if err != nil {
		slog.WarnContext(ctx, "legacy password rehash: hashing failed", "err", err.Error())
		return
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		u.UpdatePassword(newHash, model.AlgorithmArgon2id, now)
		return s.repo.Save(txCtx, u)
	}); err != nil {
		slog.WarnContext(ctx, "legacy password rehash: persist failed", "err", err.Error())
	}
}
