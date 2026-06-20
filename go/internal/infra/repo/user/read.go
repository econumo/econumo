// CQRS read side for the user module. These methods bypass the domain aggregate
// entirely: they run purpose-built read queries and return lightweight row
// structs shaped for the response, which the app-layer query-service assembles
// into the DTO. This avoids the load-aggregate-then-enrich pattern (and its
// extra round trips) that the read endpoints would otherwise incur.
//
// The same engine-selection mechanism as the write Repo is used: a small
// readQuerier interface implemented per engine, chosen at construction. Read
// queries also run through TxManager.Querier(ctx), so a read issued inside a
// WithTx sees that transaction (used by the per-test outer-tx harness).
package userrepo

import (
	"context"
	"database/sql"
	"errors"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// ReadRepo returns the app-layer read-model row types directly so it satisfies
// app/user.ReadModel with no bridging adapter. (infra adapters may depend inward
// on app; app does not import infra, so there is no cycle.)

// readQuerier is the engine-agnostic read surface, expressed in canonical
// (sqlite-generated) shapes where applicable. Implemented per engine below.
type readQuerier interface {
	GetUserView(ctx context.Context, db backend.DBTX, id string) (sqlitegen.GetUserViewRow, error)
	GetUserOptionsView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.GetUserOptionsViewRow, error)
	GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error)
}

// ReadRepo is the user read model. It is a separate type from the write Repo to
// keep the CQRS boundary explicit, but shares the TxManager + driver-selection.
// It satisfies app/user.ReadModel.
type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ appuser.ReadModel = (*ReadRepo)(nil)

// NewReadRepo selects the engine read querier by driver name.
func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("userrepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// UserView returns the user's display fields, or a NotFound error.
func (r *ReadRepo) UserView(ctx context.Context, id string) (appuser.UserViewRow, error) {
	row, err := r.q.GetUserView(ctx, r.db(ctx), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appuser.UserViewRow{}, errs.NewNotFound("User not found")
		}
		return appuser.UserViewRow{}, err
	}
	return appuser.UserViewRow{ID: row.ID, Email: row.Email, Name: row.Name, AvatarURL: row.AvatarUrl}, nil
}

// OptionViews returns the user's persisted options in stable order.
func (r *ReadRepo) OptionViews(ctx context.Context, userID string) ([]appuser.OptionViewRow, error) {
	rows, err := r.q.GetUserOptionsView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]appuser.OptionViewRow, 0, len(rows))
	for _, o := range rows {
		out = append(out, appuser.OptionViewRow{Name: o.Name, Value: o.Value})
	}
	return out, nil
}

// CurrencyIDByCode resolves a currency code to its id; sql.ErrNoRows when absent
// so the caller can apply the USD fallback.
func (r *ReadRepo) CurrencyIDByCode(ctx context.Context, code string) (string, error) {
	return r.q.GetCurrencyIDByCode(ctx, r.db(ctx), code)
}

// --- engine adapters -------------------------------------------------------

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetUserView(ctx context.Context, db backend.DBTX, id string) (sqlitegen.GetUserViewRow, error) {
	return sqlitegen.New(db).GetUserView(ctx, id)
}
func (sqliteReadQuerier) GetUserOptionsView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.GetUserOptionsViewRow, error) {
	return sqlitegen.New(db).GetUserOptionsView(ctx, userID)
}
func (sqliteReadQuerier) GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error) {
	return sqlitegen.New(db).GetCurrencyIDByCode(ctx, code)
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetUserView(ctx context.Context, db backend.DBTX, id string) (sqlitegen.GetUserViewRow, error) {
	row, err := pgsqlgen.New(db).GetUserView(ctx, id)
	return sqlitegen.GetUserViewRow(row), err
}
func (pgsqlReadQuerier) GetUserOptionsView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.GetUserOptionsViewRow, error) {
	rows, err := pgsqlgen.New(db).GetUserOptionsView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sqlitegen.GetUserOptionsViewRow, len(rows))
	for i, r := range rows {
		out[i] = sqlitegen.GetUserOptionsViewRow(r)
	}
	return out, nil
}
func (pgsqlReadQuerier) GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error) {
	return pgsqlgen.New(db).GetCurrencyIDByCode(ctx, code)
}
