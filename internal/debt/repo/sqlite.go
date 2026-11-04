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
func (sqliteQuerier) LockOwner(context.Context, backend.DBTX, string) error { return nil }

func (sqliteQuerier) GetDebtByID(ctx context.Context, db backend.DBTX, id string) (debtRow, error) {
	return sqlitegen.New(db).GetDebtByID(ctx, id)
}

func (sqliteQuerier) ListDebtsByCategory(ctx context.Context, db backend.DBTX, categoryID string) ([]debtRow, error) {
	return sqlitegen.New(db).ListDebtsByCategory(ctx, categoryID)
}

func (sqliteQuerier) UpsertDebt(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return sqlitegen.New(db).UpsertDebt(ctx, p)
}

func (sqliteQuerier) DeleteDebt(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteDebt(ctx, id)
}

func (sqliteQuerier) InsertDebtSnapshot(ctx context.Context, db backend.DBTX, p snapshotParams) error {
	return sqlitegen.New(db).InsertDebtSnapshot(ctx, p)
}

func (sqliteQuerier) InsertDebtSplit(ctx context.Context, db backend.DBTX, p splitParams) error {
	return sqlitegen.New(db).InsertDebtSplit(ctx, p)
}

func (sqliteQuerier) CandidateDebtTransactions(ctx context.Context, db backend.DBTX, categoryID *string) ([]candidateRow, error) {
	return sqlitegen.New(db).CandidateDebtTransactions(ctx, categoryID)
}

func (sqliteQuerier) DebtUsesCategory(ctx context.Context, db backend.DBTX, id string) (bool, error) {
	n, err := sqlitegen.New(db).DebtUsesCategory(ctx, id)
	return n != 0, err
}

func (sqliteQuerier) DebtUsesPayee(ctx context.Context, db backend.DBTX, id *string) (bool, error) {
	n, err := sqlitegen.New(db).DebtUsesPayee(ctx, id)
	return n != 0, err
}

func (sqliteQuerier) DebtUsesTag(ctx context.Context, db backend.DBTX, id *string) (bool, error) {
	n, err := sqlitegen.New(db).DebtUsesTag(ctx, id)
	return n != 0, err
}

func (sqliteQuerier) ReassignDebtCategory(ctx context.Context, db backend.DBTX, p reassignCatParam) error {
	return sqlitegen.New(db).ReassignDebtCategory(ctx, p)
}

func (sqliteQuerier) ReassignDebtPayee(ctx context.Context, db backend.DBTX, p reassignPayParam) error {
	return sqlitegen.New(db).ReassignDebtPayee(ctx, p)
}

func (sqliteQuerier) ReassignDebtTag(ctx context.Context, db backend.DBTX, p reassignTagParam) error {
	return sqlitegen.New(db).ReassignDebtTag(ctx, p)
}
