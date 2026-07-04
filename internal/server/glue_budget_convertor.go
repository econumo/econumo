// BudgetConvertor adapts currency.Convertor to budget.Convertor, and
// BudgetAverageRateLookup adapts currency's RateProvider (currencyrepo) to
// budget.AverageRateLookup. They live here, not in
// internal/budget/repo, because budget's own ConvertItem/FullRate
// are structural copies of model.ConvertItem/model.FullRate (an infra
// package must not import a feature, see archtest) — these adapters convert
// between the two shapes at the composition root.
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

// BulkConvert converts budget's own ConvertItem slices to model.ConvertItem
// and delegates.
func (c *BudgetConvertor) BulkConvert(ctx context.Context, periodStart, periodEnd time.Time, items map[string][]appbudget.ConvertItem) (map[string]vo.DecimalNumber, error) {
	converted := make(map[string][]model.ConvertItem, len(items))
	for k, v := range items {
		out := make([]model.ConvertItem, len(v))
		for i, item := range v {
			out[i] = model.ConvertItem{
				PeriodStart: item.PeriodStart,
				PeriodEnd:   item.PeriodEnd,
				From:        item.From,
				To:          item.To,
				Amount:      item.Amount,
			}
		}
		converted[k] = out
	}
	return c.convertor.BulkConvert(ctx, periodStart, periodEnd, converted)
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

// AverageRates converts model.FullRate to budget's own FullRate.
func (l *BudgetAverageRateLookup) AverageRates(ctx context.Context, start, end time.Time) ([]appbudget.FullRate, error) {
	rates, err := l.rates.AverageRates(ctx, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]appbudget.FullRate, len(rates))
	for i, r := range rates {
		out[i] = appbudget.FullRate{CurrencyID: r.CurrencyID, Rate: r.Rate}
	}
	return out, nil
}

// SnappedRatePeriod passes through (primitive types only, no leak).
func (l *BudgetAverageRateLookup) SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error) {
	return l.rates.SnappedRatePeriod(ctx, start, end)
}

// BaseCurrencyID passes through (primitive types only, no leak).
func (l *BudgetAverageRateLookup) BaseCurrencyID(ctx context.Context) (vo.Id, error) {
	return l.rates.BaseCurrencyID(ctx)
}
