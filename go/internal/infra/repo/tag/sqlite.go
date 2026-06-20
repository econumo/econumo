package tagrepo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// sqliteQuerier implements querier over the sqlite-generated queries. Because
// the canonical types ARE the sqlite types, every method is a direct
// passthrough. It is stateless: each call binds a fresh *Queries to the
// caller-supplied DBTX (the pool or the active tx).
type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) GetTagByID(ctx context.Context, db backend.DBTX, id string) (tagRow, error) {
	return sqlitegen.New(db).GetTagByID(ctx, id)
}

func (sqliteQuerier) ListTagsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]tagRow, error) {
	return sqlitegen.New(db).ListTagsByOwner(ctx, userID)
}

func (sqliteQuerier) CountTagsByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return sqlitegen.New(db).CountTagsByOwner(ctx, userID)
}

func (sqliteQuerier) UpsertTag(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertTag(ctx, p)
}

func (sqliteQuerier) DeleteTag(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteTag(ctx, id)
}
