package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetRecurringTransactionByID(ctx context.Context, db backend.DBTX, id string) (rtRow, error) {
	row, err := pgsqlgen.New(db).GetRecurringTransactionByID(ctx, id)
	if err != nil {
		return rtRow{}, err
	}
	return rtRow(row), nil
}

func (pgsqlQuerier) UpsertRecurringTransaction(ctx context.Context, db backend.DBTX, arg upsertParams) error {
	return pgsqlgen.New(db).UpsertRecurringTransaction(ctx, pgsqlgen.UpsertRecurringTransactionParams(arg))
}

func (pgsqlQuerier) DeleteRecurringTransaction(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteRecurringTransaction(ctx, id)
}
