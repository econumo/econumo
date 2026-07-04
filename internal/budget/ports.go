// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserLookup resolves a budget participant's id/name/avatar + their currency code.
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (model.OwnerView, error)
	// CurrencyCode returns the user's default currency code (for createBudget when
	// no currencyId is supplied).
	CurrencyCode(ctx context.Context, userID string) (string, error)
	// SetActiveBudget records the user's active budget id.
	SetActiveBudget(ctx context.Context, userID, budgetID vo.Id) error
}

// AccountLookup resolves accounts owned by the budget participants + ownership.
type AccountLookup interface {
	AccountsForOwners(ctx context.Context, userIDs []vo.Id) ([]model.AccountView, error)
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
}

// CurrencyLookup resolves a currency id by code (createBudget default currency).
type CurrencyLookup interface {
	GetIDByCode(ctx context.Context, code string) (string, error)
}

// MetadataLookup resolves the categories + tags (+ payees) owned by a set of
// users (the budget participants), for the structure builder and the
// transaction list. Categories include both income and expense; the builder
// filters.
type MetadataLookup interface {
	CategoriesByOwners(ctx context.Context, userIDs []vo.Id) ([]model.CategoryMeta, error)
	TagsByOwners(ctx context.Context, userIDs []vo.Id) ([]model.TagMeta, error)
	PayeesByOwners(ctx context.Context, userIDs []vo.Id) ([]model.PayeeMeta, error)
}

// Convertor is the currency convertor port (internal/server's BudgetConvertor
// adapts currency.Convertor to it).
type Convertor interface {
	BulkConvert(ctx context.Context, periodStart, periodEnd time.Time, items map[string][]model.ConvertItem) (map[string]vo.DecimalNumber, error)
}

// AverageRateLookup resolves period-average rates for the financial summary's
// currencyRates block (internal/server's BudgetAverageRateLookup adapts
// currency.RateProvider to it).
type AverageRateLookup interface {
	AverageRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error)
	// SnappedRatePeriod returns the [start,end) AverageRates actually used (the
	// latest-rate month <= end, or the requested period when no rate exists).
	// The currencyRates block reports THIS period.
	SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error)
	BaseCurrencyID(ctx context.Context) (vo.Id, error)
}
