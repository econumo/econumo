package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

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

func (sqliteQuerier) UsageCounts(ctx context.Context, db backend.DBTX, userID string, since time.Time) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT g.id, COUNT(t.id) FROM tags g
		 JOIN transactions t ON t.tag_id = g.id AND t.spent_at >= ?
		 WHERE g.user_id = ? GROUP BY g.id`, since, userID)
	if err != nil {
		return nil, err
	}
	return scanUsageCounts(rows)
}
