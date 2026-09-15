// External-identity entry points consumed by the oauth feature through its
// ports (wired in internal/server). They deliberately mirror Register/Login:
// same defaults, same trial, same session shape — only the credential differs.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ProvisionExternalUser creates a passwordless, email-verified user from a
// provider identity. The caller (the oauth callback) has already established
// the email is verified or trusted and that registration is allowed.
func (s *Service) ProvisionExternalUser(ctx context.Context, name, email string) (*model.User, error) {
	return s.persistNewUser(ctx, name, email, func(id vo.Id, encryptedEmail, avatar string, now time.Time) *model.User {
		return model.NewPasswordlessUser(id, encryptedEmail, name, avatar, now)
	}, true, false)
}

// CreateExternalSession mints the session an oauth handoff buys. generation is
// the credentials generation the FLOW resolved its user under, carried on the
// handoff row: a reclaim that lands while the flow is in the air bumps it, and
// the guarded insert refuses.
func (s *Service) CreateExternalSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string, generation int64) (*model.LoginResult, error) {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		return nil, &errs.UnauthorizedError{Msg: "Invalid credentials.", Code: errs.CodeInvalidCredentials}
	}
	now := s.clock.Now()
	if err := s.purgeDeadTokens(ctx, u.ID, now); err != nil {
		return nil, err
	}
	token, terr := s.createSession(ctx, u.ID, userAgent, provider, idToken, now, generation)
	if terr != nil {
		return nil, terr
	}
	cur, cerr := s.toCurrentUser(ctx, u)
	if cerr != nil {
		return nil, cerr
	}
	// Best-effort, exactly like Login.
	_ = s.repo.UpdateLanguage(ctx, u.ID, reqctx.Language(ctx))
	return &model.LoginResult{Token: token, User: cur}, nil
}

// ReplaceVerifiedEmail mirrors an IdP-side email change onto the primary email
// (the oauth feature applies the rest of the eligibility rule: one identity,
// address unused). The new address counts as verified. The write lands ONLY
// while the account is still passwordless and still at the generation the
// callback resolved it under; the database decides, so a reset committing
// after those checks leaves the recovered account's address alone. Returns the
// rows affected.
func (s *Service) ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string, generation int64) (int64, error) {
	encrypted, err := s.encode.Encode(strings.TrimSpace(email))
	if err != nil {
		return 0, err
	}
	return s.repo.ReplaceEmailIfPasswordless(ctx, userID, encrypted, s.clock.Now(), generation)
}

func (s *Service) GetByID(ctx context.Context, id vo.Id) (*model.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.repo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
}
