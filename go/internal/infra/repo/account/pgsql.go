package accountrepo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

// pgsqlQuerier converts between the canonical (sqlite) param/row types and the
// field-identical pgsql ones (whole-struct conversion).
type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetAccount(ctx context.Context, db backend.DBTX, id string) (accountRow, error) {
	a, err := pgsqlgen.New(db).GetAccountByID(ctx, id)
	return accountRow(a), err
}

func (pgsqlQuerier) ListAvailableAccounts(ctx context.Context, db backend.DBTX, userID string) ([]accountRow, error) {
	rows, err := pgsqlgen.New(db).ListAvailableAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]accountRow, len(rows))
	for i, a := range rows {
		out[i] = accountRow(a)
	}
	return out, nil
}

func (pgsqlQuerier) CountAvailableAccounts(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountAvailableAccounts(ctx, userID)
}

func (pgsqlQuerier) UpsertAccount(ctx context.Context, db backend.DBTX, p upsertAccountP) error {
	return pgsqlgen.New(db).UpsertAccount(ctx, pgsqlgen.UpsertAccountParams(p))
}

func (pgsqlQuerier) GetAccountOption(ctx context.Context, db backend.DBTX, p getOptionP) (optionRow, error) {
	o, err := pgsqlgen.New(db).GetAccountOption(ctx, pgsqlgen.GetAccountOptionParams(p))
	return optionRow(o), err
}

func (pgsqlQuerier) ListAccountOptionsByUser(ctx context.Context, db backend.DBTX, userID string) ([]optionRow, error) {
	rows, err := pgsqlgen.New(db).ListAccountOptionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]optionRow, len(rows))
	for i, o := range rows {
		out[i] = optionRow(o)
	}
	return out, nil
}

func (pgsqlQuerier) UpsertAccountOption(ctx context.Context, db backend.DBTX, p upsertOptionP) error {
	return pgsqlgen.New(db).UpsertAccountOption(ctx, pgsqlgen.UpsertAccountOptionParams(p))
}

func (pgsqlQuerier) InsertCorrectionTransaction(ctx context.Context, db backend.DBTX, p insertCorrectionP) error {
	return pgsqlgen.New(db).InsertCorrectionTransaction(ctx, pgsqlgen.InsertCorrectionTransactionParams(p))
}

// pgsqlBalance builds the DEDUPED pgsql balance params (sqlc.arg names collapse
// repeated references to a single field each).
type pgsqlBalance struct{}

var _ balanceQuerier = pgsqlBalance{}

func (pgsqlBalance) GetAccountBalance(ctx context.Context, db backend.DBTX, accountID string, before time.Time) (string, error) {
	return pgsqlgen.New(db).GetAccountBalance(ctx, pgsqlgen.GetAccountBalanceParams{
		AccountID: accountID,
		Before:    before,
	})
}

func (pgsqlBalance) ListAccountBalancesForUser(ctx context.Context, db backend.DBTX, userID string, before time.Time) ([]balanceRow, error) {
	rows, err := pgsqlgen.New(db).ListAccountBalancesForUser(ctx, pgsqlgen.ListAccountBalancesForUserParams{
		Before: before,
		UserID: userID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]balanceRow, len(rows))
	for i, b := range rows {
		out[i] = balanceRow(b)
	}
	return out, nil
}
