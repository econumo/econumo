package repo

import (
	"context"
	"time"

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

func (sqliteQuerier) UsageCounts(ctx context.Context, db backend.DBTX, userID string, since time.Time) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT p.id, COUNT(t.id) FROM payees p
		 JOIN transactions t ON t.payee_id = p.id AND t.spent_at >= ?
		 WHERE p.user_id = ? GROUP BY p.id`, since, userID)
	if err != nil {
		return nil, err
	}
	return scanUsageCounts(rows)
}
