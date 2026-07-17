package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

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

func (pgsqlQuerier) UsageCounts(ctx context.Context, db backend.DBTX, userID string, since time.Time) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT g.id, COUNT(t.id) FROM tags g
		 JOIN transactions t ON t.tag_id = g.id AND t.spent_at >= $1
		 WHERE g.user_id = $2 GROUP BY g.id`, since, userID)
	if err != nil {
		return nil, err
	}
	return scanUsageCounts(rows)
}
