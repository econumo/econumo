package accountrepo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// sqliteQuerier is the passthrough adapter over the sqlite-generated queries
// (the canonical types ARE the sqlite types).
type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) GetAccount(ctx context.Context, db backend.DBTX, id string) (accountRow, error) {
	return sqlitegen.New(db).GetAccountByID(ctx, id)
}

func (sqliteQuerier) ListAvailableAccounts(ctx context.Context, db backend.DBTX, userID string) ([]accountRow, error) {
	// own + shared: the query repeats the user id positionally (a.user_id OR
	// aa.user_id), so sqlc generates a two-field param struct.
	return sqlitegen.New(db).ListAvailableAccounts(ctx, sqlitegen.ListAvailableAccountsParams{UserID: userID, UserID_2: userID})
}

func (sqliteQuerier) CountAvailableAccounts(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return sqlitegen.New(db).CountAvailableAccounts(ctx, sqlitegen.CountAvailableAccountsParams{UserID: userID, UserID_2: userID})
}

func (sqliteQuerier) UpsertAccount(ctx context.Context, db backend.DBTX, p upsertAccountP) error {
	return sqlitegen.New(db).UpsertAccount(ctx, p)
}

func (sqliteQuerier) GetAccountOption(ctx context.Context, db backend.DBTX, p getOptionP) (optionRow, error) {
	return sqlitegen.New(db).GetAccountOption(ctx, p)
}

func (sqliteQuerier) ListAccountOptionsByUser(ctx context.Context, db backend.DBTX, userID string) ([]optionRow, error) {
	return sqlitegen.New(db).ListAccountOptionsByUser(ctx, userID)
}

func (sqliteQuerier) UpsertAccountOption(ctx context.Context, db backend.DBTX, p upsertOptionP) error {
	return sqlitegen.New(db).UpsertAccountOption(ctx, p)
}

func (sqliteQuerier) InsertCorrectionTransaction(ctx context.Context, db backend.DBTX, p insertCorrectionP) error {
	return sqlitegen.New(db).InsertCorrectionTransaction(ctx, p)
}

// sqliteBalance builds the EXPANDED sqlite balance params (repeated positional
// args). The single-balance query references the account id 4× (the 4th is the
// recipient subquery, typed *string) and spent_at 4×; the bulk query references
// spent_at 4× then user_id.
type sqliteBalance struct{}

var _ balanceQuerier = sqliteBalance{}

func (sqliteBalance) GetAccountBalance(ctx context.Context, db backend.DBTX, accountID string, before time.Time) (string, error) {
	acc := accountID
	return sqlitegen.New(db).GetAccountBalance(ctx, sqlitegen.GetAccountBalanceParams{
		AccountID:          acc,
		SpentAt:            before,
		AccountID_2:        acc,
		SpentAt_2:          before,
		AccountID_3:        acc,
		SpentAt_3:          before,
		AccountRecipientID: &acc,
		SpentAt_4:          before,
	})
}

func (sqliteBalance) ListAccountBalancesForUser(ctx context.Context, db backend.DBTX, userID string, before time.Time) ([]balanceRow, error) {
	return sqlitegen.New(db).ListAccountBalancesForUser(ctx, sqlitegen.ListAccountBalancesForUserParams{
		SpentAt:   before,
		SpentAt_2: before,
		SpentAt_3: before,
		SpentAt_4: before,
		UserID:    userID,
	})
}
