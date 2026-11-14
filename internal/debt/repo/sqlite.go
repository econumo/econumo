package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

// SQLite has no row-level lock modes; its single writer already serializes
// transactions.
func (sqliteQuerier) LockBudget(context.Context, backend.DBTX, string) error { return nil }

func (sqliteQuerier) GetDebtByID(ctx context.Context, db backend.DBTX, id string) (debtRow, error) {
	return sqlitegen.New(db).GetDebtByID(ctx, id)
}

func (sqliteQuerier) ListDebtsByBudget(ctx context.Context, db backend.DBTX, budgetID string) ([]debtRow, error) {
	return sqlitegen.New(db).ListDebtsByBudget(ctx, budgetID)
}

func (sqliteQuerier) UpsertDebt(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertDebt(ctx, p)
}

func (sqliteQuerier) DeleteDebt(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteDebt(ctx, id)
}

func (sqliteQuerier) DeleteDebtRules(ctx context.Context, db backend.DBTX, debtID string) error {
	return sqlitegen.New(db).DeleteDebtRules(ctx, debtID)
}

func (sqliteQuerier) InsertDebtRule(ctx context.Context, db backend.DBTX, p ruleParams) error {
	return sqlitegen.New(db).InsertDebtRule(ctx, p)
}

func (sqliteQuerier) InsertDebtSnapshot(ctx context.Context, db backend.DBTX, p snapshotParams) error {
	return sqlitegen.New(db).InsertDebtSnapshot(ctx, p)
}

func (sqliteQuerier) UpdateDebtSnapshot(ctx context.Context, db backend.DBTX, p updateSnapshotParams) error {
	return sqlitegen.New(db).UpdateDebtSnapshot(ctx, p)
}

func (sqliteQuerier) DeleteDebtSnapshot(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteDebtSnapshot(ctx, id)
}

func (sqliteQuerier) InsertDebtSplit(ctx context.Context, db backend.DBTX, p splitParams) error {
	return sqlitegen.New(db).InsertDebtSplit(ctx, p)
}

func (sqliteQuerier) UpsertDebtRateChange(ctx context.Context, db backend.DBTX, p rateChangeParams) error {
	return sqlitegen.New(db).UpsertDebtRateChange(ctx, p)
}

func (sqliteQuerier) DeleteDebtRateChange(ctx context.Context, db backend.DBTX, p deleteRateChangeParams) (int64, error) {
	return sqlitegen.New(db).DeleteDebtRateChange(ctx, p)
}
