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
	// LockRow holds the user row for the rest of the caller's transaction, so a
	// decision taken over the account's sign-in methods still holds when the
	// caller writes. Unlinking an identity, redeeming a sign-in handoff, and
	// every identity write (complete-link, auto-link, provisioning) take it,
	// which is what serializes them against each other and against an account
	// reclaim, whose transaction takes the same lock first.
	LockRow(ctx context.Context, userID vo.Id) error
	// ProvisionExternal creates a passwordless, email-verified user with the
	// registration defaults (trial, options, currency).
	ProvisionExternal(ctx context.Context, name, email string) (*model.User, error)
	// ReplaceVerifiedEmail mirrors an IdP-side email change onto the primary
	// email, but only while the account is still passwordless and still at the
	// given generation — the checks the caller made are re-decided by the
	// database, so a reset committing in between wins. Returns the rows written.
	ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string, generation int64) (int64, error)
	// MintSession opens a session stamped with the provider (and the ID token
	// for the custom slot) and returns the login-shaped result. generation is
	// the credentials generation the flow resolved its user under; the write is
	// refused if a reclaim has bumped it since (an account whose owner reset the
	// password must not be reachable by a sign-in already in the air).
	MintSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string, generation int64) (*model.LoginResult, error)
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

// Notifier tells the account owner a provider gained a sign-in method on their
// account: auto-linked to an existing passwordless account (step 6 of
// Callback), or linked from Settings (CompleteLink). A nil Notifier on Service
// disables the notification (tests, and any composition root that opts out).
type Notifier interface {
	IdentityLinked(ctx context.Context, userID vo.Id, providerName string) error
}
