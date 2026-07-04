// BudgetConvertor adapts currency.Convertor to budget.Convertor, and
// BudgetAverageRateLookup adapts currency's RateProvider (currencyrepo) to
// budget.AverageRateLookup. Now that budget's ConvertItem/FullRate have moved
// into internal/model (retired as structural twins of model.ConvertItem/
// model.FullRate), both wrappers are pure pass-throughs — kept for this
// commit so the type move lands independently green; the next commit
// recognizes the identity and deletes them, wiring the concrete currency
// types directly.
package server

import (
	"context"
	"time"

	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// budgetConvertorPort is the currency Convertor's own surface.
type budgetConvertorPort interface {
	BulkConvert(ctx context.Context, periodStart, periodEnd time.Time, items map[string][]model.ConvertItem) (map[string]vo.DecimalNumber, error)
}

// BudgetConvertor wraps a budgetConvertorPort (typically *currency.Convertor).
type BudgetConvertor struct {
	convertor budgetConvertorPort
}

var _ appbudget.Convertor = (*BudgetConvertor)(nil)

// NewBudgetConvertor wraps the currency convertor.
func NewBudgetConvertor(convertor budgetConvertorPort) *BudgetConvertor {
	return &BudgetConvertor{convertor: convertor}
}

// BulkConvert delegates (both sides are model.ConvertItem now).
func (c *BudgetConvertor) BulkConvert(ctx context.Context, periodStart, periodEnd time.Time, items map[string][]model.ConvertItem) (map[string]vo.DecimalNumber, error) {
	return c.convertor.BulkConvert(ctx, periodStart, periodEnd, items)
}

// budgetRateProviderPort is the currencyrepo.RateProvider surface app/budget's
// AverageRateLookup needs.
type budgetRateProviderPort interface {
	AverageRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error)
	SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error)
	BaseCurrencyID(ctx context.Context) (vo.Id, error)
}

// BudgetAverageRateLookup wraps a budgetRateProviderPort (typically
// *currencyrepo.RateProvider).
type BudgetAverageRateLookup struct {
	rates budgetRateProviderPort
}

var _ appbudget.AverageRateLookup = (*BudgetAverageRateLookup)(nil)

// NewBudgetAverageRateLookup wraps the currency rate provider.
func NewBudgetAverageRateLookup(rates budgetRateProviderPort) *BudgetAverageRateLookup {
	return &BudgetAverageRateLookup{rates: rates}
}

// AverageRates delegates (both sides are model.FullRate now).
func (l *BudgetAverageRateLookup) AverageRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error) {
	return l.rates.AverageRates(ctx, start, end)
}

// SnappedRatePeriod passes through (primitive types only, no leak).
func (l *BudgetAverageRateLookup) SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error) {
	return l.rates.SnappedRatePeriod(ctx, start, end)
}

// BaseCurrencyID passes through (primitive types only, no leak).
func (l *BudgetAverageRateLookup) BaseCurrencyID(ctx context.Context) (vo.Id, error) {
	return l.rates.BaseCurrencyID(ctx)
}
