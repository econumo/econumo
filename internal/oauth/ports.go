// Ports: the user-feature capabilities this feature consumes. Implemented in
// internal/server over the user service — features never import each other.
package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Users interface {
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id vo.Id) (*model.User, error)
	// ProvisionExternal creates a passwordless, email-verified user with the
	// registration defaults (trial, options, currency).
	ProvisionExternal(ctx context.Context, name, email string) (*model.User, error)
	// ReplaceVerifiedEmail mirrors an IdP-side email change onto the primary email.
	ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error
	// MintSession opens a session stamped with the provider (and the ID token
	// for the custom slot) and returns the login-shaped result.
	MintSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error)
	// RevokeAllSessions ends every session of the user (PATs survive), as a
	// password reset does.
	RevokeAllSessions(ctx context.Context, userID vo.Id) error
	// MarkEmailVerified records the provider's assertion of mailbox ownership.
	MarkEmailVerified(ctx context.Context, userID vo.Id) error
}

// AttemptLimiter is the brute-force seam for start-login/start-link. Only Allow
// is needed: nothing identifies the caller before the provider answers, so
// there is no per-key counter to fail or clear. A nil limiter disables the
// check (tests, CLI).
type AttemptLimiter interface {
	Allow(scope, key string) error
}

// RateScopeOAuthStart caps the rate at which authorization requests (and their
// state rows) may be created. It carries no per-key limit — the key is empty
// because the caller is anonymous — so only the global per-minute cap
// (ECONUMO_RATE_LIMIT_GLOBAL) applies.
const RateScopeOAuthStart = "oauth-start"
