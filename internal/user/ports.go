// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package user

import (
	"context"
)

// CurrencyLookup resolves a currency code to its currency-id (the synthetic
// currency_id option in CurrentUserResult). DefaultCode returns the fallback
// used when the user's code can't be resolved.
type CurrencyLookup interface {
	// GetIDByCode returns the currency uuid for the given code, or an error if
	// not found. The service falls back to DefaultCode on error.
	GetIDByCode(ctx context.Context, code string) (string, error)
	// DefaultCode returns the fallback currency code (USD).
	DefaultCode() string
}

// BudgetExistence is the minimal budget lookup the update-budget use case needs:
// confirm a budget id exists before setting it as the user's default. The check
// is existence-only (no ownership/access check) and a miss maps to the "Plan not
// found" validation error. The full budget module owns the table; this is the
// read-only port the user service depends on.
type BudgetExistence interface {
	// Exists reports whether a budget with the given id exists.
	Exists(ctx context.Context, budgetID string) (bool, error)
}

// AvatarPicker supplies the avatar value for newly created users. Production
// wiring picks randomly (RandomAvatarPicker); test harnesses pin a fixed value
// (FixedAvatarPicker) so golden responses stay deterministic.
type AvatarPicker interface {
	Pick() string
}

// AttemptLimiter is the brute-force-protection seam for the public auth use
// cases. Keys are the lowercased+trimmed submitted username/email; scopes are
// the RateScope* constants. A nil limiter disables protection (CLI, tests).
type AttemptLimiter interface {
	// Allow reports whether another attempt may proceed; over-limit yields an
	// *errs.TooManyRequestsError (HTTP 429).
	Allow(scope, key string) error
	// Fail records a failed attempt.
	Fail(scope, key string)
	// Clear wipes the key's failure counter after a successful attempt.
	Clear(scope, key string)
}

// Rate-limit scopes; the same strings key the limiter config in internal/server.
const (
	RateScopeLogin    = "login"
	RateScopeReset    = "reset"
	RateScopeRemind   = "remind"
	RateScopeRegister = "register"
)
