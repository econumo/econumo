// RateProvider adapter for the currency Convertor. It ports PHP
// CurrencyRateService::getAverageFullCurrencyRatesDtos + getAverageCurrencyRates:
// snap the requested [start,end) to the rate month (getLatestDate -> first of
// that month .. next month; fall back to the raw range when no rates exist),
// then AVG(rate) per currency over that snapped period for the base currency.
package currencyrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domcurrency "github.com/econumo/econumo/internal/domain/currency"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// avgRow is the canonical (sqlite) average-rate row.
type avgRow = sqlitegen.GetAverageCurrencyRatesRow

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
// rate over it for the base currency. Mirrors getAverageCurrencyRates.
func (p *RateProvider) AverageRates(ctx context.Context, start, end time.Time) ([]domcurrency.FullRate, error) {
	baseID, err := p.BaseCurrencyID(ctx)
	if err != nil {
		return nil, err
	}

	realStart, realEnd := start, end
	last, derr := p.q.GetLatestDate(ctx, p.db(ctx), baseID.String(), end)
	if derr == nil {
		// First of the latest-rate month .. next month (PHP Y-m-01 .. next month).
		realStart = time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, last.Location())
		realEnd = realStart.AddDate(0, 1, 0)
	} else if !errors.Is(derr, sql.ErrNoRows) {
		var nf *errs.NotFoundError
		if !errors.As(derr, &nf) {
			return nil, derr
		}
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

// --- engine adapters ---

type sqliteProviderQuerier struct{}

func (sqliteProviderQuerier) GetAverage(ctx context.Context, db backend.DBTX, start, end time.Time, baseID string) ([]avgRow, error) {
	return sqlitegen.New(db).GetAverageCurrencyRates(ctx, sqlitegen.GetAverageCurrencyRatesParams{
		PublishedAt: start, PublishedAt_2: end, BaseCurrencyID: baseID,
	})
}
func (sqliteProviderQuerier) GetLatestDate(ctx context.Context, db backend.DBTX, baseID string, before time.Time) (time.Time, error) {
	return sqlitegen.New(db).GetLatestCurrencyRateDate(ctx, sqlitegen.GetLatestCurrencyRateDateParams{BaseCurrencyID: baseID, PublishedAt: before})
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
		out[i] = avgRow(r)
	}
	return out, nil
}
func (pgsqlProviderQuerier) GetLatestDate(ctx context.Context, db backend.DBTX, baseID string, before time.Time) (time.Time, error) {
	return pgsqlgen.New(db).GetLatestCurrencyRateDate(ctx, pgsqlgen.GetLatestCurrencyRateDateParams{BaseCurrencyID: baseID, PublishedAt: before})
}
