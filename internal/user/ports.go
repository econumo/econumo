// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
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

// BudgetAccess is the minimal budget lookup the update-budget use case needs:
// confirm the caller may use a budget before setting it as their default. Access
// means the caller owns the budget OR holds an accepted share on it; anything
// else (including a nonexistent budget) maps to the "Plan not found" validation
// error, so a foreign budget id cannot be stashed as a user's default. The full
// budget module owns the table; this is the read-only port the user service
// depends on.
type BudgetAccess interface {
	// HasAccess reports whether the user owns or has an accepted share on the
	// budget.
	HasAccess(ctx context.Context, userID vo.Id, budgetID string) (bool, error)
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
	// Mark records an event timestamp for the key with no cap check, for scopes
	// used as a per-key clock rather than a limit.
	Mark(scope, key string)
	// LastAttempt reports when the key last recorded an attempt, and whether
	// any is on record. It answers identically for existing and unknown keys,
	// which is what lets a cooldown be reported without leaking existence.
	LastAttempt(scope, key string) (time.Time, bool)
}

// Rate-limit scopes; the same strings key the limiter config in internal/server.
const (
	RateScopeLogin        = "login"
	RateScopeReset        = "reset"
	RateScopeRemind       = "remind"
	RateScopeRegister     = "register"
	RateScopeVerifyEmail  = "verify-email"
	RateScopeConfirmEmail = "confirm-email"
	// RateScopeVerifySent is a timestamp channel, not a cap: it records when a
	// verification code was last EMAILED for a username, so the resend cooldown
	// can be reported identically for real and unknown usernames. It carries no
	// configured limit, so it never rejects anything itself.
	RateScopeVerifySent = "verify-email-sent"
)
