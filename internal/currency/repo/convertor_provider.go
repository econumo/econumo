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
	"github.com/econumo/econumo/internal/model"
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

// fixedRateRow is one custom currency's fixed rate.
type fixedRateRow struct {
	CurrencyID string
	Rate       string
}

// providerQuerier is the engine-agnostic surface the provider needs.
type providerQuerier interface {
	GetAverage(ctx context.Context, db backend.DBTX, start, end time.Time, baseID string) ([]avgRow, error)
	GetLatestDate(ctx context.Context, db backend.DBTX, baseID string, before time.Time) (time.Time, error)
	GetEarliestDate(ctx context.Context, db backend.DBTX, baseID string) (time.Time, error)
	GetNearestRates(ctx context.Context, db backend.DBTX, baseID string, before time.Time) ([]avgRow, error)
	GetFixedRates(ctx context.Context, db backend.DBTX) ([]fixedRateRow, error)
}

// RateProvider implements domain/currency.RateProvider.
type RateProvider struct {
	tx     *backend.TxManager
	q      providerQuerier
	lookup *Lookup
	baseID string
}

var _ domcurrency.RateProvider = (*RateProvider)(nil)

// NewRateProvider wires the provider. baseID is the boot-resolved base
// currency id; the lookup resolves fraction digits.
func NewRateProvider(driver string, tx *backend.TxManager, lookup *Lookup, baseID string) *RateProvider {
	var q providerQuerier
	switch driver {
	case "sqlite":
		q = sqliteProviderQuerier{}
	case "postgresql":
		q = pgsqlProviderQuerier{}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
	return &RateProvider{tx: tx, q: q, lookup: lookup, baseID: baseID}
}

func (p *RateProvider) db(ctx context.Context) backend.DBTX { return p.tx.Querier(ctx) }

// BaseCurrencyID returns the boot-resolved base currency's id.
func (p *RateProvider) BaseCurrencyID(ctx context.Context) (vo.Id, error) {
	return vo.ParseId(p.baseID)
}

// FractionDigits returns a currency's fraction digits.
func (p *RateProvider) FractionDigits(ctx context.Context, currencyID vo.Id) (int, error) {
	v, err := p.lookup.GetByID(ctx, currencyID.String())
	if err != nil {
		return 0, err
	}
	return v.FractionDigits, nil
}

// AverageRates is every conversion's rate set: the rate month's average per
// currency, its gaps filled. Rate histories start and stop at different dates, so
// a month can hold rates for some currencies only; the others take their latest
// earlier rate, else their first. With none a foreign amount would count at face
// value.
func (p *RateProvider) AverageRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error) {
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
	nearest, err := p.q.GetNearestRates(ctx, p.db(ctx), baseID.String(), end)
	if err != nil {
		return nil, err
	}
	covered := make(map[string]bool, len(rows))
	for _, r := range rows {
		covered[r.CurrencyID] = true
	}
	for _, r := range nearest {
		if !covered[r.CurrencyID] {
			rows = append(rows, r)
		}
	}
	return p.withFixedRates(ctx, rows)
}

// StoredRates holds only the rates actually published for the month of the
// latest rate at or before end, with no gap filled and nothing before the first
// rate: the import no-rate guard queues what this does not cover, rather than
// save a bank transaction at a rate from another time.
func (p *RateProvider) StoredRates(ctx context.Context, start, end time.Time) ([]model.FullRate, error) {
	baseID, err := p.BaseCurrencyID(ctx)
	if err != nil {
		return nil, err
	}
	realStart, realEnd, found, err := p.latestRateMonth(ctx, baseID, end)
	if err != nil {
		return nil, err
	}
	if !found {
		realStart, realEnd = start, end
	}
	rows, err := p.q.GetAverage(ctx, p.db(ctx), realStart, realEnd, baseID.String())
	if err != nil {
		return nil, err
	}
	return p.withFixedRates(ctx, rows)
}

// Custom currencies carry ONE fixed rate on the currency row, applied to any
// requested period (changing it re-values history by design). They never appear
// in the AVG result: their dated rows were removed by the 20260801000000 migration.
func (p *RateProvider) withFixedRates(ctx context.Context, rows []avgRow) ([]model.FullRate, error) {
	out := make([]model.FullRate, 0, len(rows))
	for _, r := range rows {
		id, perr := vo.ParseId(r.CurrencyID)
		if perr != nil {
			return nil, perr
		}
		out = append(out, model.FullRate{CurrencyID: id, Rate: vo.NewDecimal(r.Rate)})
	}
	fixed, err := p.q.GetFixedRates(ctx, p.db(ctx))
	if err != nil {
		return nil, err
	}
	for _, r := range fixed {
		id, perr := vo.ParseId(r.CurrencyID)
		if perr != nil {
			return nil, perr
		}
		out = append(out, model.FullRate{CurrencyID: id, Rate: vo.NewDecimal(r.Rate)})
	}
	return out, nil
}

// SnappedRatePeriod returns the [start, end) AverageRates actually uses for the
// requested period: the month of the latest published rate at or before `end`,
// the earliest rate's month before the first rate, or the requested period when
// no rate exists. The budget currencyRates block
// reports THIS period.
func (p *RateProvider) SnappedRatePeriod(ctx context.Context, start, end time.Time) (time.Time, time.Time, error) {
	baseID, err := p.BaseCurrencyID(ctx)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return p.snapPeriod(ctx, baseID, start, end)
}

// snapPeriod resolves [start,end) to the latest-rate month at or before it, the
// earliest-rate month for a period before the first rate, or the request
// unchanged when no rate is stored at all.
func (p *RateProvider) snapPeriod(ctx context.Context, baseID vo.Id, start, end time.Time) (time.Time, time.Time, error) {
	realStart, realEnd, found, err := p.latestRateMonth(ctx, baseID, end)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if found {
		return realStart, realEnd, nil
	}
	first, ferr := p.q.GetEarliestDate(ctx, p.db(ctx), baseID.String())
	if ferr == nil {
		realStart = time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, first.Location())
		return realStart, realStart.AddDate(0, 1, 0), nil
	}
	if !rateNotFound(ferr) {
		return time.Time{}, time.Time{}, ferr
	}
	return start, end, nil
}

// latestRateMonth is the month of the latest published rate before end.
func (p *RateProvider) latestRateMonth(ctx context.Context, baseID vo.Id, end time.Time) (time.Time, time.Time, bool, error) {
	last, err := p.q.GetLatestDate(ctx, p.db(ctx), baseID.String(), end)
	if err != nil {
		if rateNotFound(err) {
			return time.Time{}, time.Time{}, false, nil
		}
		return time.Time{}, time.Time{}, false, err
	}
	realStart := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, last.Location())
	return realStart, realStart.AddDate(0, 1, 0), true, nil
}

func rateNotFound(err error) bool {
	var nf *errs.NotFoundError
	return errors.Is(err, sql.ErrNoRows) || errors.As(err, &nf)
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
	// datetime(published_at) < datetime(?): bind the bound as 'Y-m-d H:i:s'
	// text so rows at/after the boundary are excluded.
	return sqlitegen.New(db).GetLatestCurrencyRateDate(ctx, sqlitegen.GetLatestCurrencyRateDateParams{BaseCurrencyID: baseID, Datetime: before.Format(datetime.Layout)})
}

func (sqliteProviderQuerier) GetNearestRates(ctx context.Context, db backend.DBTX, baseID string, before time.Time) ([]avgRow, error) {
	rows, err := sqlitegen.New(db).GetNearestCurrencyRates(ctx, sqlitegen.GetNearestCurrencyRatesParams{BaseCurrencyID: baseID, Before: before.Format(datetime.Layout)})
	if err != nil {
		return nil, err
	}
	out := make([]avgRow, len(rows))
	for i, r := range rows {
		out[i] = avgRow{CurrencyID: r.CurrencyID, Rate: strconv.FormatFloat(r.Rate, 'f', 8, 64)}
	}
	return out, nil
}

func (sqliteProviderQuerier) GetEarliestDate(ctx context.Context, db backend.DBTX, baseID string) (time.Time, error) {
	return sqlitegen.New(db).GetEarliestCurrencyRateDate(ctx, baseID)
}

func (sqliteProviderQuerier) GetFixedRates(ctx context.Context, db backend.DBTX) ([]fixedRateRow, error) {
	rows, err := sqlitegen.New(db).GetFixedCurrencyRates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]fixedRateRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, fixedRateRow{CurrencyID: r.ID, Rate: *r.Rate})
	}
	return out, nil
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

func (pgsqlProviderQuerier) GetNearestRates(ctx context.Context, db backend.DBTX, baseID string, before time.Time) ([]avgRow, error) {
	rows, err := pgsqlgen.New(db).GetNearestCurrencyRates(ctx, pgsqlgen.GetNearestCurrencyRatesParams{BaseCurrencyID: baseID, Before: before})
	if err != nil {
		return nil, err
	}
	out := make([]avgRow, len(rows))
	for i, r := range rows {
		out[i] = avgRow{CurrencyID: r.CurrencyID, Rate: r.Rate}
	}
	return out, nil
}

func (pgsqlProviderQuerier) GetEarliestDate(ctx context.Context, db backend.DBTX, baseID string) (time.Time, error) {
	return pgsqlgen.New(db).GetEarliestCurrencyRateDate(ctx, baseID)
}

func (pgsqlProviderQuerier) GetFixedRates(ctx context.Context, db backend.DBTX) ([]fixedRateRow, error) {
	rows, err := pgsqlgen.New(db).GetFixedCurrencyRates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]fixedRateRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, fixedRateRow{CurrencyID: r.ID, Rate: *r.Rate})
	}
	return out, nil
}
