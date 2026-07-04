// CQRS read side for the user module: purpose-built read queries returning
// lightweight row structs the app-layer query-service assembles into the DTO,
// bypassing the domain aggregate to avoid the load-then-enrich round trips.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/user"
)

type readQuerier interface {
	GetUserView(ctx context.Context, db backend.DBTX, id string) (sqlitegen.GetUserViewRow, error)
	GetUserOptionsView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.GetUserOptionsViewRow, error)
	GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error)
}

type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ user.ReadModel = (*ReadRepo)(nil)

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

func (r *ReadRepo) UserView(ctx context.Context, id string) (model.UserViewRow, error) {
	row, err := r.q.GetUserView(ctx, r.db(ctx), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.UserViewRow{}, errs.NewNotFound("User not found")
		}
		return model.UserViewRow{}, err
	}
	return model.UserViewRow{ID: row.ID, Email: row.Email, Name: row.Name, AvatarURL: row.AvatarUrl}, nil
}

func (r *ReadRepo) OptionViews(ctx context.Context, userID string) ([]model.OptionViewRow, error) {
	rows, err := r.q.GetUserOptionsView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.OptionViewRow, 0, len(rows))
	for _, o := range rows {
		out = append(out, model.OptionViewRow{Name: o.Name, Value: o.Value})
	}
	return out, nil
}

// CurrencyIDByCode resolves a currency code to its id; sql.ErrNoRows when absent
// so the caller can apply the USD fallback.
func (r *ReadRepo) CurrencyIDByCode(ctx context.Context, code string) (string, error) {
	return r.q.GetCurrencyIDByCode(ctx, r.db(ctx), code)
}

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
