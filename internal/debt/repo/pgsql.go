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

// A missing user must keep succeeding silently, as for user.Repository.LockRow.
func (pgsqlQuerier) LockOwner(ctx context.Context, db backend.DBTX, ownerID string) error {
	if _, err := pgsqlgen.New(db).LockDebtOwner(ctx, ownerID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func toDebtRow(d pgsqlgen.Debt) debtRow {
	row := debtRow{
		ID: d.ID, UserID: d.UserID, Name: d.Name, CurrencyID: d.CurrencyID, CategoryID: d.CategoryID,
		PayeeID: d.PayeeID, TagID: d.TagID, AsOf: d.AsOf, Balance: d.Balance, Payment: d.Payment,
		Schedule: d.Schedule, LastPaymentDate: d.LastPaymentDate, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.StartDate.Valid {
		t := d.StartDate.Time
		row.StartDate = &t
	}
	return row
}

func (pgsqlQuerier) GetDebtByID(ctx context.Context, db backend.DBTX, id string) (debtRow, error) {
	d, err := pgsqlgen.New(db).GetDebtByID(ctx, id)
	if err != nil {
		return debtRow{}, err
	}
	return toDebtRow(d), nil
}

func (pgsqlQuerier) ListDebtsByCategory(ctx context.Context, db backend.DBTX, categoryID string) ([]debtRow, error) {
	rows, err := pgsqlgen.New(db).ListDebtsByCategory(ctx, categoryID)
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

func (pgsqlQuerier) InsertDebtSnapshot(ctx context.Context, db backend.DBTX, p snapshotParams) error {
	return pgsqlgen.New(db).InsertDebtSnapshot(ctx, pgsqlgen.InsertDebtSnapshotParams(p))
}

func (pgsqlQuerier) InsertDebtSplit(ctx context.Context, db backend.DBTX, p splitParams) error {
	return pgsqlgen.New(db).InsertDebtSplit(ctx, pgsqlgen.InsertDebtSplitParams(p))
}

func (pgsqlQuerier) CandidateDebtTransactions(ctx context.Context, db backend.DBTX, categoryID *string) ([]candidateRow, error) {
	rows, err := pgsqlgen.New(db).CandidateDebtTransactions(ctx, categoryID)
	if err != nil {
		return nil, err
	}
	out := make([]candidateRow, len(rows))
	for i, r := range rows {
		out[i] = candidateRow(r)
	}
	return out, nil
}

func (pgsqlQuerier) DebtUsesCategory(ctx context.Context, db backend.DBTX, id string) (bool, error) {
	return pgsqlgen.New(db).DebtUsesCategory(ctx, id)
}

func (pgsqlQuerier) DebtUsesPayee(ctx context.Context, db backend.DBTX, id *string) (bool, error) {
	return pgsqlgen.New(db).DebtUsesPayee(ctx, id)
}

func (pgsqlQuerier) DebtUsesTag(ctx context.Context, db backend.DBTX, id *string) (bool, error) {
	return pgsqlgen.New(db).DebtUsesTag(ctx, id)
}

func (pgsqlQuerier) ReassignDebtCategory(ctx context.Context, db backend.DBTX, p reassignCatParam) error {
	return pgsqlgen.New(db).ReassignDebtCategory(ctx, pgsqlgen.ReassignDebtCategoryParams(p))
}

func (pgsqlQuerier) ReassignDebtPayee(ctx context.Context, db backend.DBTX, p reassignPayParam) error {
	return pgsqlgen.New(db).ReassignDebtPayee(ctx, pgsqlgen.ReassignDebtPayeeParams(p))
}

func (pgsqlQuerier) ReassignDebtTag(ctx context.Context, db backend.DBTX, p reassignTagParam) error {
	return pgsqlgen.New(db).ReassignDebtTag(ctx, pgsqlgen.ReassignDebtTagParams(p))
}
