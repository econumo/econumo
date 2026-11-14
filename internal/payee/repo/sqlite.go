package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) GetPayeeByID(ctx context.Context, db backend.DBTX, id string) (payeeRow, error) {
	return sqlitegen.New(db).GetPayeeByID(ctx, id)
}

func (sqliteQuerier) ListPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]payeeRow, error) {
	return sqlitegen.New(db).ListPayeesByOwner(ctx, userID)
}

func (sqliteQuerier) CountPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return sqlitegen.New(db).CountPayeesByOwner(ctx, userID)
}

func (sqliteQuerier) UpsertPayee(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertPayee(ctx, p)
}

func (sqliteQuerier) DeletePayee(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeletePayee(ctx, id)
}

func (sqliteQuerier) ReassignPayeeTransactions(ctx context.Context, db backend.DBTX, p reassignTxParams) error {
	return sqlitegen.New(db).ReassignPayeeTransactions(ctx, p)
}

func (sqliteQuerier) ReassignPayeeRecurring(ctx context.Context, db backend.DBTX, p reassignRecParams) error {
	return sqlitegen.New(db).ReassignPayeeRecurring(ctx, p)
}
