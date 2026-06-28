// CQRS read side for the currency module. CurrencyListView and
// LatestCurrencyRateListView run purpose-built read queries and return the
// app-layer view-row types directly (so they satisfy app/currency.ReadModel with
// no bridging adapter). The rate's published date is formatted "Y-m-d 00:00:00"
// here, at the edge of persistence (matching the PHP CurrencyRate publishedAt
// which is built from the DATE at midnight).
//
// Reads run through TxManager.Querier(ctx); these endpoints are not wrapped in a
// WithTx, so they run on the pooled connection.
package currencyrepo

import (
	"context"

	appcurrency "github.com/econumo/econumo/internal/app/currency"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// rateDatetimeLayout is the wire datetime form for a rate's published date:
// the DATE rendered as midnight, matching PHP's
// publishedAt->format('Y-m-d H:i:s') where publishedAt is the date at 00:00:00.
const rateDatetimeLayout = "2006-01-02 15:04:05"

// Canonical read-row types: the sqlite-generated ones (the pgsql shim copies
// into them). The list-view query selects a column subset, so sqlc emits a
// dedicated GetCurrencyListViewRow. The rate query selects the full table in
// column order, so sqlc reuses the CurrenciesRate model directly (sqlite); the
// pgsql engine emits its own GetLatestCurrencyRateListViewRow with identical
// fields, which the shim converts into CurrenciesRate.
type (
	currencyRow = sqlitegen.GetCurrencyListViewRow
	rateRow     = sqlitegen.CurrenciesRate
)

// readQuerier is the engine-agnostic read surface, in the canonical types.
type readQuerier interface {
	GetCurrencyListView(ctx context.Context, db backend.DBTX) ([]currencyRow, error)
	GetLatestCurrencyRateListView(ctx context.Context, db backend.DBTX) ([]rateRow, error)
}

// ReadRepo is the currency read model. It shares the TxManager + driver
// selection with the Lookup (lookup.go). It satisfies app/currency.ReadModel.
type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ appcurrency.ReadModel = (*ReadRepo)(nil)

// NewReadRepo selects the engine read querier by driver name.
func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// CurrencyListView returns all currencies ordered by code ASC.
func (r *ReadRepo) CurrencyListView(ctx context.Context) ([]appcurrency.CurrencyViewRow, error) {
	rows, err := r.q.GetCurrencyListView(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]appcurrency.CurrencyViewRow, 0, len(rows))
	for _, c := range rows {
		out = append(out, appcurrency.CurrencyViewRow{
			ID:             c.ID,
			Code:           c.Code,
			Symbol:         c.Symbol,
			Name:           c.Name,
			FractionDigits: c.FractionDigits,
		})
	}
	return out, nil
}

// LatestCurrencyRateListView returns every rate on the most-recent published
// date, with the date pre-formatted "Y-m-d 00:00:00".
func (r *ReadRepo) LatestCurrencyRateListView(ctx context.Context) ([]appcurrency.CurrencyRateViewRow, error) {
	rows, err := r.q.GetLatestCurrencyRateListView(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]appcurrency.CurrencyRateViewRow, 0, len(rows))
	for _, rt := range rows {
		out = append(out, appcurrency.CurrencyRateViewRow{
			CurrencyID:     rt.CurrencyID,
			BaseCurrencyID: rt.BaseCurrencyID,
			// Normalize the NUMERIC(19,8) string to the PHP DecimalNumber wire
			// form (trailing zeros trimmed). PostgreSQL returns "0.92000000";
			// SQLite affinity returns "0.92"; both must render PHP-identically.
			Rate:      vo.NewDecimal(rt.Rate).String(),
			UpdatedAt: rt.PublishedAt.Format(rateDatetimeLayout),
		})
	}
	return out, nil
}

// --- engine adapters -------------------------------------------------------

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetCurrencyListView(ctx context.Context, db backend.DBTX) ([]currencyRow, error) {
	return sqlitegen.New(db).GetCurrencyListView(ctx)
}

func (sqliteReadQuerier) GetLatestCurrencyRateListView(ctx context.Context, db backend.DBTX) ([]rateRow, error) {
	return sqlitegen.New(db).GetLatestCurrencyRateListView(ctx)
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetCurrencyListView(ctx context.Context, db backend.DBTX) ([]currencyRow, error) {
	rows, err := pgsqlgen.New(db).GetCurrencyListView(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]currencyRow, len(rows))
	for i, c := range rows {
		out[i] = currencyRow(c)
	}
	return out, nil
}

func (pgsqlReadQuerier) GetLatestCurrencyRateListView(ctx context.Context, db backend.DBTX) ([]rateRow, error) {
	rows, err := pgsqlgen.New(db).GetLatestCurrencyRateListView(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rateRow, len(rows))
	for i, rt := range rows {
		// pgsql emits GetLatestCurrencyRateListViewRow; sqlite reuses
		// CurrenciesRate. Fields are identical, so a field copy bridges them.
		out[i] = rateRow{
			ID:             rt.ID,
			CurrencyID:     rt.CurrencyID,
			BaseCurrencyID: rt.BaseCurrencyID,
			PublishedAt:    rt.PublishedAt,
			Rate:           rt.Rate,
		}
	}
	return out, nil
}
