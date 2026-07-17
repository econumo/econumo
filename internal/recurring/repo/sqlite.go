package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) GetRecurringTransactionByID(ctx context.Context, db backend.DBTX, id string) (rtRow, error) {
	return sqlitegen.New(db).GetRecurringTransactionByID(ctx, id)
}

func (sqliteQuerier) UpsertRecurringTransaction(ctx context.Context, db backend.DBTX, arg upsertParams) error {
	return sqlitegen.New(db).UpsertRecurringTransaction(ctx, arg)
}

func (sqliteQuerier) DeleteRecurringTransaction(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteRecurringTransaction(ctx, id)
}
