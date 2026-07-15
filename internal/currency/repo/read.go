// CQRS read side for the currency module. UserCurrencyListView and
// LatestCurrencyRateListView run purpose-built read queries and return the
// app-layer view-row types directly (so they satisfy app/currency.ReadModel with
// no bridging adapter). The rate's published date is formatted "Y-m-d 00:00:00"
// here, at the edge of persistence (the stored value is the DATE at midnight).
//
// Reads run through TxManager.Querier(ctx); these endpoints are not wrapped in a
// WithTx, so they run on the pooled connection.
package repo

import (
	"context"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Canonical read-row types: the sqlite-generated ones (the pgsql shim copies
// into them). The list-view query selects a column subset, so sqlc emits a
// dedicated GetUserCurrencyListViewRow. The rate query selects the full table
// (aliased), so both engines emit dedicated row types with identical fields;
// the pgsql shim converts into the sqlite CurrenciesRate model.
type (
	currencyRow = sqlitegen.GetUserCurrencyListViewRow
	rateRow     = sqlitegen.CurrenciesRate
)

// readQuerier is the engine-agnostic read surface, in the canonical types.
type readQuerier interface {
	GetUserCurrencyListView(ctx context.Context, db backend.DBTX, userID string) ([]currencyRow, error)
	GetHiddenCurrencyIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error)
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

// UserCurrencyListView returns every currency visible to userID: all globals,
// the user's own customs, and foreign customs reachable via a shared
// account/budget/budget-element, ordered by code ASC then id ASC.
func (r *ReadRepo) UserCurrencyListView(ctx context.Context, userID string) ([]model.CurrencyViewRow, error) {
	rows, err := r.q.GetUserCurrencyListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.CurrencyViewRow, 0, len(rows))
	for _, c := range rows {
		out = append(out, model.CurrencyViewRow{
			ID:             c.ID,
			Code:           c.Code,
			Symbol:         c.Symbol,
			Name:           c.Name,
			FractionDigits: c.FractionDigits,
			UserID:         c.UserID,
			IsArchived:     c.IsArchived,
		})
	}
	return out, nil
}

// HiddenCurrencyIDs returns the ids of global currencies userID has hidden.
func (r *ReadRepo) HiddenCurrencyIDs(ctx context.Context, userID string) ([]string, error) {
	return r.q.GetHiddenCurrencyIDs(ctx, r.db(ctx), userID)
}

// LatestCurrencyRateListView returns the latest rate row per (currency, base)
// pair, with the date pre-formatted "Y-m-d 00:00:00".
func (r *ReadRepo) LatestCurrencyRateListView(ctx context.Context) ([]model.CurrencyRateViewRow, error) {
	rows, err := r.q.GetLatestCurrencyRateListView(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]model.CurrencyRateViewRow, 0, len(rows))
	for _, rt := range rows {
		out = append(out, model.CurrencyRateViewRow{
			CurrencyID:     rt.CurrencyID,
			BaseCurrencyID: rt.BaseCurrencyID,
			// Normalize the NUMERIC(19,8) string to the canonical decimal wire
			// form (trailing zeros trimmed). PostgreSQL returns "0.92000000";
			// SQLite affinity returns "0.92"; both must render identically.
			Rate: vo.NewDecimal(rt.Rate).String(),
			// PublishedAt is the rate DATE at midnight, rendered Y-m-d H:i:s.
			UpdatedAt: rt.PublishedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetUserCurrencyListView(ctx context.Context, db backend.DBTX, userID string) ([]currencyRow, error) {
	// c.user_id is nullable, so the first occurrence (c.user_id = ?) binds a
	// *string; the other three occurrences (accounts_access.user_id etc.) are
	// NOT NULL columns and bind plain strings.
	return sqlitegen.New(db).GetUserCurrencyListView(ctx, sqlitegen.GetUserCurrencyListViewParams{
		UserID:   &userID,
		UserID_2: userID,
		UserID_3: userID,
		UserID_4: userID,
	})
}

func (sqliteReadQuerier) GetHiddenCurrencyIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error) {
	return sqlitegen.New(db).GetHiddenCurrencyIDs(ctx, userID)
}

func (sqliteReadQuerier) GetLatestCurrencyRateListView(ctx context.Context, db backend.DBTX) ([]rateRow, error) {
	return sqlitegen.New(db).GetLatestCurrencyRateListView(ctx)
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetUserCurrencyListView(ctx context.Context, db backend.DBTX, userID string) ([]currencyRow, error) {
	rows, err := pgsqlgen.New(db).GetUserCurrencyListView(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]currencyRow, len(rows))
	for i, c := range rows {
		out[i] = currencyRow(c)
	}
	return out, nil
}

func (pgsqlReadQuerier) GetHiddenCurrencyIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error) {
	return pgsqlgen.New(db).GetHiddenCurrencyIDs(ctx, userID)
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
