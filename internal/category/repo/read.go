package repo

import (
	"context"

	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
)

type readQuerier interface {
	GetCategoryListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Category, error)
}

type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ appcategory.ReadModel = (*ReadRepo)(nil)

func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("categoryrepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *ReadRepo) CategoryListView(ctx context.Context, userID string) ([]model.CategoryViewRow, error) {
	rows, err := r.q.GetCategoryListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.CategoryViewRow, 0, len(rows))
	for _, c := range rows {
		out = append(out, model.CategoryViewRow{
			ID:         c.ID,
			UserID:     c.UserID,
			Name:       c.Name,
			Position:   c.Position,
			Type:       c.Type,
			Icon:       c.Icon,
			IsArchived: c.IsArchived,
			CreatedAt:  c.CreatedAt.Format(datetime.Layout),
			UpdatedAt:  c.UpdatedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetCategoryListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Category, error) {
	// own + shared: the query repeats the user id positionally (own user_id, and
	// again in the shared-owners subquery), so sqlc generates a two-field param.
	return sqlitegen.New(db).GetCategoryListView(ctx, sqlitegen.GetCategoryListViewParams{UserID: userID, UserID_2: userID})
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetCategoryListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Category, error) {
	rows, err := pgsqlgen.New(db).GetCategoryListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sqlitegen.Category, len(rows))
	for i, c := range rows {
		out[i] = sqlitegen.Category(c)
	}
	return out, nil
}
