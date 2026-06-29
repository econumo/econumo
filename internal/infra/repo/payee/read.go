package payeerepo

import (
	"context"

	apppayee "github.com/econumo/econumo/internal/app/payee"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type readQuerier interface {
	GetPayeeListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Payee, error)
}

type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ apppayee.ReadModel = (*ReadRepo)(nil)

func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("payeerepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *ReadRepo) PayeeListView(ctx context.Context, userID string) ([]apppayee.PayeeViewRow, error) {
	rows, err := r.q.GetPayeeListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]apppayee.PayeeViewRow, 0, len(rows))
	for _, p := range rows {
		out = append(out, apppayee.PayeeViewRow{
			ID:         p.ID,
			UserID:     p.UserID,
			Name:       p.Name,
			Position:   p.Position,
			IsArchived: p.IsArchived,
			CreatedAt:  p.CreatedAt.Format(datetime.Layout),
			UpdatedAt:  p.UpdatedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetPayeeListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Payee, error) {
	// own + shared: the user id is repeated positionally -> two-field param.
	return sqlitegen.New(db).GetPayeeListView(ctx, sqlitegen.GetPayeeListViewParams{UserID: userID, UserID_2: userID})
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetPayeeListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Payee, error) {
	rows, err := pgsqlgen.New(db).GetPayeeListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sqlitegen.Payee, len(rows))
	for i, p := range rows {
		out[i] = sqlitegen.Payee(p)
	}
	return out, nil
}
