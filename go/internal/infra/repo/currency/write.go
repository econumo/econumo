// Write side of the currency repository: implements app/currency.WriteModel for
// the CLI admin commands (the HTTP API has no currency mutations). Like lookup.go
// it selects the engine with a per-method driver switch — there are only a handful
// of small write queries, so the read.go canonical-type + shim machinery would be
// overkill here. Every method runs on the context-bound DBTX, so the WriteService
// transaction (TxManager.WithTx) wraps them transparently.
package currencyrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	appcurrency "github.com/econumo/econumo/internal/app/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// WriteRepo implements app/currency.WriteModel over the sqlc write queries.
type WriteRepo struct {
	tx     *backend.TxManager
	driver string
}

var _ appcurrency.WriteModel = (*WriteRepo)(nil)

// NewWriteRepo builds the currency write repository for the given engine.
func NewWriteRepo(driver string, tx *backend.TxManager) *WriteRepo {
	switch driver {
	case "sqlite", "postgresql":
		return &WriteRepo{tx: tx, driver: driver}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
}

func (r *WriteRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// CurrencyCodes returns stored code -> currency id for every currency.
func (r *WriteRepo) CurrencyCodes(ctx context.Context) (map[string]string, error) {
	db := r.db(ctx)
	out := map[string]string{}
	switch r.driver {
	case "sqlite":
		rows, err := sqlitegen.New(db).ListCurrencyCodes(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range rows {
			out[c.Code] = c.ID
		}
	default:
		rows, err := pgsqlgen.New(db).ListCurrencyCodes(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range rows {
			out[c.Code] = c.ID
		}
	}
	return out, nil
}

// CurrencyExists reports whether a currency with the code exists.
func (r *WriteRepo) CurrencyExists(ctx context.Context, code string) (bool, error) {
	db := r.db(ctx)
	var err error
	switch r.driver {
	case "sqlite":
		_, err = sqlitegen.New(db).GetCurrencyByCode(ctx, code)
	default:
		_, err = pgsqlgen.New(db).GetCurrencyByCode(ctx, code)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InsertCurrency adds a new currencies row.
func (r *WriteRepo) InsertCurrency(ctx context.Context, c appcurrency.CurrencyRow) error {
	db := r.db(ctx)
	switch r.driver {
	case "sqlite":
		return sqlitegen.New(db).InsertCurrency(ctx, sqlitegen.InsertCurrencyParams{
			ID:             c.ID,
			Code:           c.Code,
			Symbol:         c.Symbol,
			Name:           c.Name,
			FractionDigits: int16(c.FractionDigits),
			CreatedAt:      c.CreatedAt,
		})
	default:
		return pgsqlgen.New(db).InsertCurrency(ctx, pgsqlgen.InsertCurrencyParams{
			ID:             c.ID,
			Code:           c.Code,
			Symbol:         c.Symbol,
			Name:           c.Name,
			FractionDigits: int16(c.FractionDigits),
			CreatedAt:      c.CreatedAt,
		})
	}
}

// SetFractionDigits sets a currency's fraction_digits by code.
func (r *WriteRepo) SetFractionDigits(ctx context.Context, code string, digits int) error {
	db := r.db(ctx)
	switch r.driver {
	case "sqlite":
		return sqlitegen.New(db).UpdateCurrencyFractionDigitsByCode(ctx, sqlitegen.UpdateCurrencyFractionDigitsByCodeParams{
			FractionDigits: int16(digits),
			Code:           code,
		})
	default:
		return pgsqlgen.New(db).UpdateCurrencyFractionDigitsByCode(ctx, pgsqlgen.UpdateCurrencyFractionDigitsByCodeParams{
			FractionDigits: int16(digits),
			Code:           code,
		})
	}
}

// UpsertRate inserts or updates a single rate. The published date is truncated
// to midnight UTC so the value is stable per day (SQLite stores it ISO8601 via
// modernc; PostgreSQL as a native DATE) and the per-day ON CONFLICT upsert
// dedupes correctly.
func (r *WriteRepo) UpsertRate(ctx context.Context, rr appcurrency.RateRow) error {
	db := r.db(ctx)
	day := time.Date(rr.Date.Year(), rr.Date.Month(), rr.Date.Day(), 0, 0, 0, 0, time.UTC)
	switch r.driver {
	case "sqlite":
		return sqlitegen.New(db).UpsertCurrencyRate(ctx, sqlitegen.UpsertCurrencyRateParams{
			ID:             rr.ID,
			CurrencyID:     rr.CurrencyID,
			BaseCurrencyID: rr.BaseCurrencyID,
			PublishedAt:    day,
			Rate:           rr.Rate,
		})
	default:
		return pgsqlgen.New(db).UpsertCurrencyRate(ctx, pgsqlgen.UpsertCurrencyRateParams{
			ID:             rr.ID,
			CurrencyID:     rr.CurrencyID,
			BaseCurrencyID: rr.BaseCurrencyID,
			PublishedAt:    day,
			Rate:           rr.Rate,
		})
	}
}
