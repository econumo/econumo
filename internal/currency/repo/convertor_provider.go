// RateProvider adapter for the currency Convertor: snap the requested
// [start,end) to the rate month (the latest published rate's month -> first of
// that month .. next month; fall back to the raw range when no rates exist),
// then AVG(rate) per currency over that snapped period for the base currency.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	domcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// avgRow is one average-rate row with the rate already rendered to a decimal
// STRING. SQLite's AVG is float, formatted with %.8f (round at the 8th decimal);
// PostgreSQL's AVG(NUMERIC) is exact and passes through.
type avgRow struct {
	CurrencyID string
	Rate       string
}

// providerQuerier is the engine-agnostic surface the provider needs.
type providerQuerier interface {
	GetAverage(ctx context.Context, db backend.DBTX, start, end time.Time, baseID string) ([]avgRow, error)
	GetLatestDate(ctx context.Context, db backend.DBTX, baseID string, before time.Time) (time.Time, error)
}

// RateProvider implements domain/currency.RateProvider.
type RateProvider struct {
	tx       *backend.TxManager
	q        providerQuerier
	lookup   *Lookup
	baseCode string
}

var _ domcurrency.RateProvider = (*RateProvider)(nil)

// NewRateProvider wires the provider. baseCode is config.CurrencyBase (e.g.
// "USD"); the lookup resolves base id + fraction digits.
func NewRateProvider(driver string, tx *backend.TxManager, lookup *Lookup, baseCode string) *RateProvider {
	var q providerQuerier
	switch driver {
	case "sqlite":
		q = sqliteProviderQuerier{}
	case "postgresql":
		q = pgsqlProviderQuerier{}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
	return &RateProvider{tx: tx, q: q, lookup: lookup, baseCode: baseCode}
}

func (p *RateProvider) db(ctx context.Context) backend.DBTX { return p.tx.Querier(ctx) }

// BaseCurrencyID resolves the base currency's id.
func (p *RateProvider) BaseCurrencyID(ctx context.Context) (vo.Id, error) {
	id, err := p.lookup.GetIDByCode(ctx, p.baseCode)
	if err != nil {
		return vo.Id{}, err
	}
	return vo.ParseId(id)
}

// FractionDigits returns a currency's fraction digits.
func (p *RateProvider) FractionDigits(ctx context.Context, currencyID vo.Id) (int, error) {
	v, err := p.lookup.GetByID(ctx, currencyID.String())
	if err != nil {
		return 0, err
	}
	return v.FractionDigits, nil
}

// AverageRates snaps the period to the rate month and averages each currency's
// rate over it for the base currency.
func (p *RateProvider) AverageRates(ctx context.Context, start, end time.Time) ([]domcurrency.FullRate, error) {
	baseID, err := p.BaseCurrencyID(ctx)
	if err != nil {
		return nil, err
	}

	realStart, realEnd, err := p.snapPeriod(ctx, baseID, start, end)
	if err != nil {
		return nil, err
	}

	rows, err := p.q.GetAverage(ctx, p.db(ctx), realStart, realEnd, baseID.String())
	if err != nil {
		return nil, err
	}
	out := make([]domcurrency.FullRate, 0, len(rows))
	for _, r := range rows {
		id, perr := vo.ParseId(r.CurrencyID)
		if perr != nil {
			return nil, perr
		}
		out = append(out, domcurrency.FullRate{CurrencyID: id, Rate: vo.NewDecimal(r.Rate)})
	}
	return out, nil
}

// SnappedRatePeriod returns the [start, end) AverageRates actually uses for the
// requested period: the month of the latest published rate at or before `end`,
// or the requested period when no rate exists. The budget currencyRates block
// reports THIS period.
func (p *RateProvider) SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error) {
	baseID, err := p.BaseCurrencyID(ctx)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return p.snapPeriod(ctx, baseID, start, end)
}

// snapPeriod resolves [start,end) to the latest-rate month, or returns the
// request unchanged when no rate is found.
func (p *RateProvider) snapPeriod(ctx context.Context, baseID vo.Id, start, end time.Time) (time.Time, time.Time, error) {
	last, derr := p.q.GetLatestDate(ctx, p.db(ctx), baseID.String(), end)
	if derr == nil {
		realStart := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, last.Location())
		return realStart, realStart.AddDate(0, 1, 0), nil
	}
	if !errors.Is(derr, sql.ErrNoRows) {
		var nf *errs.NotFoundError
		if !errors.As(derr, &nf) {
			return time.Time{}, time.Time{}, derr
		}
	}
	return start, end, nil
}

type sqliteProviderQuerier struct{}

func (sqliteProviderQuerier) GetAverage(ctx context.Context, db backend.DBTX, start, end time.Time, baseID string) ([]avgRow, error) {
	// date(published_at) >= date(?) — pass 'Y-m-d' bounds so the date-only row
	// "2025-04-01" is INCLUDED (a time.Time renders "...00:00:00", which lexically
	// excludes it). AVG is a float -> %.8f (round to 8 decimals).
	rows, err := sqlitegen.New(db).GetAverageCurrencyRates(ctx, sqlitegen.GetAverageCurrencyRatesParams{
		Date: start.Format(datetime.DateLayout), Date_2: end.Format(datetime.DateLayout), BaseCurrencyID: baseID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]avgRow, len(rows))
	for i, r := range rows {
		out[i] = avgRow{CurrencyID: r.CurrencyID, Rate: strconv.FormatFloat(r.Rate, 'f', 8, 64)}
	}
	return out, nil
}
func (sqliteProviderQuerier) GetLatestDate(ctx context.Context, db backend.DBTX, baseID string, before time.Time) (time.Time, error) {
	// datetime(published_at) < datetime(?): bind the bound as a 'Y-m-d H:i:s'
	// string so rows at/after the boundary are excluded (a time.Time bound leaks
	// them in, snapping the rate period to the wrong month).
	return sqlitegen.New(db).GetLatestCurrencyRateDate(ctx, sqlitegen.GetLatestCurrencyRateDateParams{BaseCurrencyID: baseID, Datetime: before.Format(datetime.Layout)})
}

type pgsqlProviderQuerier struct{}

func (pgsqlProviderQuerier) GetAverage(ctx context.Context, db backend.DBTX, start, end time.Time, baseID string) ([]avgRow, error) {
	rows, err := pgsqlgen.New(db).GetAverageCurrencyRates(ctx, pgsqlgen.GetAverageCurrencyRatesParams{
		PublishedAt: start, PublishedAt_2: end, BaseCurrencyID: baseID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]avgRow, len(rows))
	for i, r := range rows {
		// PostgreSQL AVG(NUMERIC) is exact; pass the text through.
		out[i] = avgRow{CurrencyID: r.CurrencyID, Rate: r.Rate}
	}
	return out, nil
}
func (pgsqlProviderQuerier) GetLatestDate(ctx context.Context, db backend.DBTX, baseID string, before time.Time) (time.Time, error) {
	return pgsqlgen.New(db).GetLatestCurrencyRateDate(ctx, pgsqlgen.GetLatestCurrencyRateDateParams{BaseCurrencyID: baseID, PublishedAt: before})
}
