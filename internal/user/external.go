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

// CreateExternalSession mints a session for a user resolved by the oauth
// feature, stamping the provider (and, for the custom slot, the ID token for
// RP-initiated logout). Inactive users are refused like a password login.
func (s *Service) CreateExternalSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error) {
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
	token, terr := s.createSession(ctx, u.ID, userAgent, provider, idToken, now)
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
// (the oauth feature applies the eligibility rule: passwordless, one identity,
// address unused). The new address counts as verified.
func (s *Service) ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error {
	encrypted, err := s.encode.Encode(strings.TrimSpace(email))
	if err != nil {
		return err
	}
	_, err = s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.UpdateEmail(encrypted, now)
		u.MarkEmailVerified(now)
		return nil
	})
	return err
}

// RevokeAllSessions ends every session of the user, PATs untouched — the
// reset-password cascade, reused by the oauth feature when a provider takes
// over an account that was pre-registered with a password.
func (s *Service) RevokeAllSessions(ctx context.Context, userID vo.Id) error {
	return s.revokeSessions(ctx, userID, vo.Id{}, s.clock.Now())
}

// MarkEmailVerified records proof of mailbox ownership established elsewhere
// (a provider's verified email claim).
func (s *Service) MarkEmailVerified(ctx context.Context, userID vo.Id) error {
	_, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.MarkEmailVerified(now)
		return nil
	})
	return err
}

func (s *Service) GetByID(ctx context.Context, id vo.Id) (*model.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.repo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
}
