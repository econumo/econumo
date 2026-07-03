package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetPayeeByID(ctx context.Context, db backend.DBTX, id string) (payeeRow, error) {
	p, err := pgsqlgen.New(db).GetPayeeByID(ctx, id)
	return payeeRow(p), err
}

func (pgsqlQuerier) ListPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]payeeRow, error) {
	rows, err := pgsqlgen.New(db).ListPayeesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]payeeRow, len(rows))
	for i, p := range rows {
		out[i] = payeeRow(p)
	}
	return out, nil
}

func (pgsqlQuerier) CountPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountPayeesByOwner(ctx, userID)
}

func (pgsqlQuerier) UpsertPayee(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertPayee(ctx, pgsqlgen.UpsertPayeeParams(p))
}

func (pgsqlQuerier) DeletePayee(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeletePayee(ctx, id)
}
