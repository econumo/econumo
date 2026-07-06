// Convertor converts amounts between currencies through the base currency,
// using period-averaged rates. It is consumed by the budget builder
// (BulkConvert) and by ad-hoc single conversions (Convert).
//
// Conversion algorithm: to convert `amount` from currency F to currency T with
// rates expressed as units-of-X per one base unit,
//   - if F != base: amount = amount / rate[F]   (F -> base)
//   - if T != base: amount = amount * rate[T]    (base -> T)
//   - round to T's fraction digits (half away from zero)
//
// All arithmetic is scale-8 (vo.DecimalNumber); Mul/Div truncate, Round is
// half-away-from-zero.
package currency

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// RateProvider supplies period-averaged rates and the base-currency id. The
// infra adapter implements it over the stored rate rows.
type RateProvider interface {
	// AverageRates returns the period-averaged FullRate per currency for the
	// base currency, with the period snapped to the rate month (latest rate date
	// -> first-of-month .. next month; falls back to the raw [start,end) when no
	// rates exist).
	AverageRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error)
	// BaseCurrencyID returns the base currency's id.
	BaseCurrencyID(ctx context.Context) (vo.Id, error)
	// FractionDigits returns a currency's fraction digits (for result rounding).
	FractionDigits(ctx context.Context, currencyID vo.Id) (int, error)
}

// Convertor converts amounts between currencies. It is stateless beyond the
// injected RateProvider; callers may reuse one instance.
type Convertor struct {
	rates RateProvider
}

func NewConvertor(rates RateProvider) *Convertor { return &Convertor{rates: rates} }

// monthKey is the "Ym" period index used to group conversions.
func monthKey(t time.Time) string { return t.Format("200601") }

// BulkConvert converts a batch of items, summing per result key. The result map
// mirrors the input keys. Items whose From == To pass through unconverted. The
// rate period used for each item is the month of its PeriodStart if rates for
// that month were loaded, else the top-level [periodStart, periodEnd) period.
//
// items maps an arbitrary key -> the list of ConvertItem that sum into that
// key's result.
func (c *Convertor) BulkConvert(ctx context.Context, periodStart, periodEnd time.Time, items map[string][]model.ConvertItem) (map[string]vo.DecimalNumber, error) {
	// Determine which rate periods are needed: always the top-level period, plus
	// each distinct month of a cross-currency sub-item.
	currentKey := monthKey(periodStart)
	needed := map[string][2]time.Time{currentKey: {periodStart, periodEnd}}
	for _, list := range items {
		for _, it := range list {
			if it.From.Equal(it.To) {
				continue
			}
			k := monthKey(it.PeriodStart)
			if _, ok := needed[k]; ok {
				continue
			}
			needed[k] = [2]time.Time{it.PeriodStart, it.PeriodEnd}
		}
	}

	// Load the average rates for each needed period, keyed by month.
	ratesByKey := map[string][]model.FullRate{}
	for k, rng := range needed {
		rs, err := c.rates.AverageRates(ctx, rng[0], rng[1])
		if err != nil {
			return nil, err
		}
		ratesByKey[k] = rs
	}

	baseID, err := c.rates.BaseCurrencyID(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]vo.DecimalNumber, len(items))
	for key, list := range items {
		acc := vo.NewDecimal("0")
		for _, it := range summarize(list) {
			k := currentKey
			if _, ok := ratesByKey[monthKey(it.PeriodStart)]; ok {
				k = monthKey(it.PeriodStart)
			}
			conv, cerr := c.convert(ctx, ratesByKey[k], baseID, it.From, it.To, it.Amount)
			if cerr != nil {
				return nil, cerr
			}
			acc = acc.Add(conv)
		}
		result[key] = acc
	}
	return result, nil
}

// Convert converts a single amount from -> to using the period-averaged rates
// for the given period. Callers wanting a one-off conversion pass a single
// period.
func (c *Convertor) Convert(ctx context.Context, periodStart, periodEnd time.Time, from, to vo.Id, sum vo.DecimalNumber) (vo.DecimalNumber, error) {
	if from.Equal(to) {
		return sum, nil
	}
	rates, err := c.rates.AverageRates(ctx, periodStart, periodEnd)
	if err != nil {
		return vo.DecimalNumber{}, err
	}
	baseID, err := c.rates.BaseCurrencyID(ctx)
	if err != nil {
		return vo.DecimalNumber{}, err
	}
	return c.convert(ctx, rates, baseID, from, to, sum)
}

// convert is the core two-hop conversion through the base currency, rounded to
// the result currency's fraction digits.
func (c *Convertor) convert(ctx context.Context, rates []model.FullRate, baseID, from, to vo.Id, amount vo.DecimalNumber) (vo.DecimalNumber, error) {
	if from.Equal(to) {
		return amount, nil
	}
	result := amount
	if !from.Equal(baseID) {
		for _, r := range rates {
			if r.CurrencyID.Equal(from) {
				result = result.Div(r.Rate)
				break
			}
		}
	}
	if !to.Equal(baseID) {
		for _, r := range rates {
			if r.CurrencyID.Equal(to) {
				result = result.Mul(r.Rate)
				break
			}
		}
	}
	digits, err := c.rates.FractionDigits(ctx, to)
	if err != nil {
		var nf *errs.NotFoundError
		if !errors.As(err, &nf) {
			return vo.DecimalNumber{}, err
		}
		digits = 2 // currency vanished; default like a missing lookup would
	}
	return result.Round(digits), nil
}

// summarize collapses items sharing (from, to, periodStart-date, periodEnd-date)
// into one, summing their amounts. Order is preserved by first appearance.
func summarize(items []model.ConvertItem) []model.ConvertItem {
	type key struct{ from, to, ps, pe string }
	idx := map[key]int{}
	var out []model.ConvertItem
	for _, it := range items {
		k := key{it.From.String(), it.To.String(), it.PeriodStart.Format(datetime.DateLayout), it.PeriodEnd.Format(datetime.DateLayout)}
		if i, ok := idx[k]; ok {
			out[i].Amount = out[i].Amount.Add(it.Amount)
			continue
		}
		idx[k] = len(out)
		out = append(out, it)
	}
	return out
}
