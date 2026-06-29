package categoryrepo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetCategoryByID(ctx context.Context, db backend.DBTX, id string) (categoryRow, error) {
	c, err := pgsqlgen.New(db).GetCategoryByID(ctx, id)
	return categoryRow(c), err
}

func (pgsqlQuerier) ListCategoriesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]categoryRow, error) {
	rows, err := pgsqlgen.New(db).ListCategoriesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]categoryRow, len(rows))
	for i, c := range rows {
		out[i] = categoryRow(c)
	}
	return out, nil
}

func (pgsqlQuerier) CountCategoriesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountCategoriesByOwner(ctx, userID)
}

func (pgsqlQuerier) UpsertCategory(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertCategory(ctx, pgsqlgen.UpsertCategoryParams(p))
}

func (pgsqlQuerier) DeleteCategory(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteCategory(ctx, id)
}

func (pgsqlQuerier) ReassignCategoryTransactions(ctx context.Context, db backend.DBTX, p reassignParams) error {
	return pgsqlgen.New(db).ReassignCategoryTransactions(ctx, pgsqlgen.ReassignCategoryTransactionsParams(p))
}

func (pgsqlQuerier) GetOperationId(ctx context.Context, db backend.DBTX, id string) (opRow, error) {
	o, err := pgsqlgen.New(db).GetOperationId(ctx, id)
	return opRow(o), err
}

func (pgsqlQuerier) InsertOperationId(ctx context.Context, db backend.DBTX, p insertOpParams) error {
	return pgsqlgen.New(db).InsertOperationId(ctx, pgsqlgen.InsertOperationIdParams(p))
}

func (pgsqlQuerier) MarkOperationHandled(ctx context.Context, db backend.DBTX, p markOpParams) error {
	return pgsqlgen.New(db).MarkOperationHandled(ctx, pgsqlgen.MarkOperationHandledParams(p))
}
