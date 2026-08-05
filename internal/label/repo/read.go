package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	domlabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
)

type readQuerier interface {
	GetLabelListView(ctx context.Context, db backend.DBTX, userID string) ([]labelRow, error)
}

type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ domlabel.ReadModel = (*ReadRepo)(nil)

func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("labelrepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *ReadRepo) LabelListView(ctx context.Context, userID string) ([]model.LabelViewRow, error) {
	rows, err := r.q.GetLabelListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.LabelViewRow, 0, len(rows))
	for _, l := range rows {
		out = append(out, model.LabelViewRow{
			ID:         l.ID,
			UserID:     l.UserID,
			Name:       l.Name,
			Icon:       l.Icon,
			IsArchived: l.IsArchived,
			CreatedAt:  l.CreatedAt.Format(datetime.Layout),
			UpdatedAt:  l.UpdatedAt.Format(datetime.Layout),
		})
	}
	return out, nil
}

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetLabelListView(ctx context.Context, db backend.DBTX, userID string) ([]labelRow, error) {
	// own + shared: the user id is repeated positionally -> two-field param.
	return sqlitegen.New(db).GetLabelListView(ctx, sqlitegen.GetLabelListViewParams{UserID: userID, UserID_2: userID})
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetLabelListView(ctx context.Context, db backend.DBTX, userID string) ([]labelRow, error) {
	rows, err := pgsqlgen.New(db).GetLabelListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]labelRow, len(rows))
	for i, l := range rows {
		out[i] = labelRow(l)
	}
	return out, nil
}
