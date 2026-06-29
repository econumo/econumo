package categoryrepo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) GetCategoryByID(ctx context.Context, db backend.DBTX, id string) (categoryRow, error) {
	return sqlitegen.New(db).GetCategoryByID(ctx, id)
}

func (sqliteQuerier) ListCategoriesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]categoryRow, error) {
	return sqlitegen.New(db).ListCategoriesByOwner(ctx, userID)
}

func (sqliteQuerier) CountCategoriesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return sqlitegen.New(db).CountCategoriesByOwner(ctx, userID)
}

func (sqliteQuerier) UpsertCategory(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertCategory(ctx, p)
}

func (sqliteQuerier) DeleteCategory(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteCategory(ctx, id)
}

func (sqliteQuerier) ReassignCategoryTransactions(ctx context.Context, db backend.DBTX, p reassignParams) error {
	return sqlitegen.New(db).ReassignCategoryTransactions(ctx, p)
}

func (sqliteQuerier) GetOperationId(ctx context.Context, db backend.DBTX, id string) (opRow, error) {
	return sqlitegen.New(db).GetOperationId(ctx, id)
}

func (sqliteQuerier) InsertOperationId(ctx context.Context, db backend.DBTX, p insertOpParams) error {
	return sqlitegen.New(db).InsertOperationId(ctx, p)
}

func (sqliteQuerier) MarkOperationHandled(ctx context.Context, db backend.DBTX, p markOpParams) error {
	return sqlitegen.New(db).MarkOperationHandled(ctx, p)
}
