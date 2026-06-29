package tagrepo

import (
	"context"

	apptag "github.com/econumo/econumo/internal/app/tag"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type readQuerier interface {
	GetTagListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Tag, error)
}

type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ apptag.ReadModel = (*ReadRepo)(nil)

func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("tagrepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *ReadRepo) TagListView(ctx context.Context, userID string) ([]apptag.TagViewRow, error) {
	rows, err := r.q.GetTagListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]apptag.TagViewRow, 0, len(rows))
	for _, t := range rows {
		out = append(out, apptag.TagViewRow{
			ID:         t.ID,
			UserID:     t.UserID,
			Name:       t.Name,
			Position:   t.Position,
			IsArchived: t.IsArchived,
			CreatedAt:  t.CreatedAt.Format(datetime.Layout),
			UpdatedAt:  t.UpdatedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetTagListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Tag, error) {
	// own + shared: the user id is repeated positionally -> two-field param.
	return sqlitegen.New(db).GetTagListView(ctx, sqlitegen.GetTagListViewParams{UserID: userID, UserID_2: userID})
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetTagListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Tag, error) {
	rows, err := pgsqlgen.New(db).GetTagListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sqlitegen.Tag, len(rows))
	for i, t := range rows {
		out[i] = sqlitegen.Tag(t)
	}
	return out, nil
}
