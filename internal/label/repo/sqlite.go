package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) GetLabelByID(ctx context.Context, db backend.DBTX, id string) (labelRow, error) {
	return sqlitegen.New(db).GetLabelByID(ctx, id)
}

func (sqliteQuerier) ListLabelsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]labelRow, error) {
	return sqlitegen.New(db).ListLabelsByOwner(ctx, userID)
}

func (sqliteQuerier) CountLabelsByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return sqlitegen.New(db).CountLabelsByOwner(ctx, userID)
}

func (sqliteQuerier) UpsertLabel(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertLabel(ctx, p)
}

func (sqliteQuerier) DeleteLabel(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteLabel(ctx, id)
}
