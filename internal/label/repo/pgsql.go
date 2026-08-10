package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetLabelByID(ctx context.Context, db backend.DBTX, id string) (labelRow, error) {
	l, err := pgsqlgen.New(db).GetLabelByID(ctx, id)
	return labelRow(l), err
}

func (pgsqlQuerier) ListLabelsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]labelRow, error) {
	rows, err := pgsqlgen.New(db).ListLabelsByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]labelRow, len(rows))
	for i, l := range rows {
		out[i] = labelRow(l)
	}
	return out, nil
}

func (pgsqlQuerier) CountLabelsByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountLabelsByOwner(ctx, userID)
}

func (pgsqlQuerier) UpsertLabel(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertLabel(ctx, pgsqlgen.UpsertLabelParams(p))
}

func (pgsqlQuerier) DeleteLabel(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteLabel(ctx, id)
}
