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
