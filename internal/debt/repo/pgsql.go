package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

// A missing budget must keep succeeding silently: a write on it fails later,
// with the error its own read gives.
func (pgsqlQuerier) LockBudget(ctx context.Context, db backend.DBTX, budgetID string) error {
	if _, err := pgsqlgen.New(db).LockDebtBudget(ctx, budgetID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func toDebtRow(d pgsqlgen.Debt) debtRow { return debtRow(d) }

func (pgsqlQuerier) GetDebtByID(ctx context.Context, db backend.DBTX, id string) (debtRow, error) {
	d, err := pgsqlgen.New(db).GetDebtByID(ctx, id)
	if err != nil {
		return debtRow{}, err
	}
	return toDebtRow(d), nil
}

func (pgsqlQuerier) ListDebtsByBudget(ctx context.Context, db backend.DBTX, budgetID string) ([]debtRow, error) {
	rows, err := pgsqlgen.New(db).ListDebtsByBudget(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	out := make([]debtRow, len(rows))
	for i, d := range rows {
		out[i] = toDebtRow(d)
	}
	return out, nil
}

func (pgsqlQuerier) UpsertDebt(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertDebt(ctx, pgsqlgen.UpsertDebtParams(p))
}

func (pgsqlQuerier) DeleteDebt(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteDebt(ctx, id)
}

func (pgsqlQuerier) MoveDebtsToElement(ctx context.Context, db backend.DBTX, p moveParams) error {
	return pgsqlgen.New(db).MoveDebtsToElement(ctx, pgsqlgen.MoveDebtsToElementParams(p))
}

func (pgsqlQuerier) InsertDebtSnapshot(ctx context.Context, db backend.DBTX, p snapshotParams) error {
	return pgsqlgen.New(db).InsertDebtSnapshot(ctx, pgsqlgen.InsertDebtSnapshotParams(p))
}

func (pgsqlQuerier) UpdateDebtSnapshot(ctx context.Context, db backend.DBTX, p updateSnapshotParams) error {
	return pgsqlgen.New(db).UpdateDebtSnapshot(ctx, pgsqlgen.UpdateDebtSnapshotParams(p))
}

func (pgsqlQuerier) DeleteDebtSnapshot(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteDebtSnapshot(ctx, id)
}

func (pgsqlQuerier) InsertDebtSplit(ctx context.Context, db backend.DBTX, p splitParams) error {
	return pgsqlgen.New(db).InsertDebtSplit(ctx, pgsqlgen.InsertDebtSplitParams(p))
}

func (pgsqlQuerier) UpsertDebtRateChange(ctx context.Context, db backend.DBTX, p rateChangeParams) error {
	return pgsqlgen.New(db).UpsertDebtRateChange(ctx, pgsqlgen.UpsertDebtRateChangeParams(p))
}

func (pgsqlQuerier) DeleteDebtRateChange(ctx context.Context, db backend.DBTX, p deleteRateChangeParams) (int64, error) {
	return pgsqlgen.New(db).DeleteDebtRateChange(ctx, pgsqlgen.DeleteDebtRateChangeParams(p))
}
