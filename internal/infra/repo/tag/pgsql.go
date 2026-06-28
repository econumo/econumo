package tagrepo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

// pgsqlQuerier implements querier over the pgsql-generated queries. The pgsql
// row/param structs are field-identical to the canonical (sqlite) types, but Go
// treats them as distinct types, so this shim field-copies (whole-struct
// conversion) across the boundary. Like sqliteQuerier it is stateless.
type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetTagByID(ctx context.Context, db backend.DBTX, id string) (tagRow, error) {
	t, err := pgsqlgen.New(db).GetTagByID(ctx, id)
	return tagRow(t), err
}

func (pgsqlQuerier) ListTagsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]tagRow, error) {
	rows, err := pgsqlgen.New(db).ListTagsByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]tagRow, len(rows))
	for i, t := range rows {
		out[i] = tagRow(t)
	}
	return out, nil
}

func (pgsqlQuerier) CountTagsByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountTagsByOwner(ctx, userID)
}

func (pgsqlQuerier) UpsertTag(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertTag(ctx, pgsqlgen.UpsertTagParams(p))
}

func (pgsqlQuerier) DeleteTag(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteTag(ctx, id)
}
