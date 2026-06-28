// Write side of the currency repository: implements app/currency.WriteModel for
// the CLI admin commands (the HTTP API has no currency mutations). It follows the
// same canonical-type + engine-adapter shape as read.go: each method is written
// once against a writeQuerier interface expressed in the canonical (sqlite-
// generated) types, the engine is chosen once in NewWriteRepo, and the pgsql
// adapter whole-struct-converts at the boundary. Every method runs on the
// context-bound DBTX, so the WriteService transaction (TxManager.WithTx) wraps
// them transparently.
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

// Canonical write row/param types (the sqlite-generated ones).
type (
	codeRow         = sqlitegen.ListCurrencyCodesRow
	insertCurrencyP = sqlitegen.InsertCurrencyParams
	upsertRateP     = sqlitegen.UpsertCurrencyRateParams
)

// writeQuerier is the engine-agnostic write surface, in the canonical types.
type writeQuerier interface {
	ListCurrencyCodes(ctx context.Context, db backend.DBTX) ([]codeRow, error)
	GetCurrencyByCode(ctx context.Context, db backend.DBTX, code string) error
	InsertCurrency(ctx context.Context, db backend.DBTX, p insertCurrencyP) error
	UpsertCurrencyRate(ctx context.Context, db backend.DBTX, p upsertRateP) error
}

// WriteRepo implements app/currency.WriteModel over the sqlc write queries.
type WriteRepo struct {
	tx *backend.TxManager
	q  writeQuerier
}

var _ appcurrency.WriteModel = (*WriteRepo)(nil)

// NewWriteRepo selects the engine write querier by driver name.
func NewWriteRepo(driver string, tx *backend.TxManager) *WriteRepo {
	switch driver {
	case "sqlite":
		return &WriteRepo{tx: tx, q: sqliteWriteQuerier{}}
	case "postgresql":
		return &WriteRepo{tx: tx, q: pgsqlWriteQuerier{}}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
}

func (r *WriteRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// CurrencyCodes returns stored code -> currency id for every currency.
func (r *WriteRepo) CurrencyCodes(ctx context.Context) (map[string]string, error) {
	rows, err := r.q.ListCurrencyCodes(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, c := range rows {
		out[c.Code] = c.ID
	}
	return out, nil
}

// CurrencyExists reports whether a currency with the code exists.
func (r *WriteRepo) CurrencyExists(ctx context.Context, code string) (bool, error) {
	if err := r.q.GetCurrencyByCode(ctx, r.db(ctx), code); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InsertCurrency adds a new currencies row.
func (r *WriteRepo) InsertCurrency(ctx context.Context, c appcurrency.CurrencyRow) error {
	return r.q.InsertCurrency(ctx, r.db(ctx), insertCurrencyP{
		ID:             c.ID,
		Code:           c.Code,
		Symbol:         c.Symbol,
		Name:           c.Name,
		FractionDigits: int16(c.FractionDigits),
		CreatedAt:      c.CreatedAt,
	})
}

// UpsertRate inserts or updates a single rate. The published date is truncated
// to midnight UTC so the value is stable per day (SQLite stores it ISO8601 via
// modernc; PostgreSQL as a native DATE) and the per-day ON CONFLICT upsert
// dedupes correctly.
func (r *WriteRepo) UpsertRate(ctx context.Context, rr appcurrency.RateRow) error {
	day := time.Date(rr.Date.Year(), rr.Date.Month(), rr.Date.Day(), 0, 0, 0, 0, time.UTC)
	return r.q.UpsertCurrencyRate(ctx, r.db(ctx), upsertRateP{
		ID:             rr.ID,
		CurrencyID:     rr.CurrencyID,
		BaseCurrencyID: rr.BaseCurrencyID,
		PublishedAt:    day,
		Rate:           rr.Rate,
	})
}

// sqliteWriteQuerier is the native passthrough (canonical types ARE sqlite's).
type sqliteWriteQuerier struct{}

var _ writeQuerier = sqliteWriteQuerier{}

func (sqliteWriteQuerier) ListCurrencyCodes(ctx context.Context, db backend.DBTX) ([]codeRow, error) {
	return sqlitegen.New(db).ListCurrencyCodes(ctx)
}

func (sqliteWriteQuerier) GetCurrencyByCode(ctx context.Context, db backend.DBTX, code string) error {
	_, err := sqlitegen.New(db).GetCurrencyByCode(ctx, code)
	return err
}

func (sqliteWriteQuerier) InsertCurrency(ctx context.Context, db backend.DBTX, p insertCurrencyP) error {
	return sqlitegen.New(db).InsertCurrency(ctx, p)
}

func (sqliteWriteQuerier) UpsertCurrencyRate(ctx context.Context, db backend.DBTX, p upsertRateP) error {
	return sqlitegen.New(db).UpsertCurrencyRate(ctx, p)
}

// pgsqlWriteQuerier is the thin whole-struct conversion shim.
type pgsqlWriteQuerier struct{}

var _ writeQuerier = pgsqlWriteQuerier{}

func (pgsqlWriteQuerier) ListCurrencyCodes(ctx context.Context, db backend.DBTX) ([]codeRow, error) {
	rows, err := pgsqlgen.New(db).ListCurrencyCodes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]codeRow, len(rows))
	for i, r := range rows {
		out[i] = codeRow(r)
	}
	return out, nil
}

func (pgsqlWriteQuerier) GetCurrencyByCode(ctx context.Context, db backend.DBTX, code string) error {
	_, err := pgsqlgen.New(db).GetCurrencyByCode(ctx, code)
	return err
}

func (pgsqlWriteQuerier) InsertCurrency(ctx context.Context, db backend.DBTX, p insertCurrencyP) error {
	return pgsqlgen.New(db).InsertCurrency(ctx, pgsqlgen.InsertCurrencyParams(p))
}

func (pgsqlWriteQuerier) UpsertCurrencyRate(ctx context.Context, db backend.DBTX, p upsertRateP) error {
	return pgsqlgen.New(db).UpsertCurrencyRate(ctx, pgsqlgen.UpsertCurrencyRateParams(p))
}
