package repo

import (
	"context"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// formatSQLiteBalance rounds a SQLite float SUM to 8 decimal places: e.g. the
// summed float 358.34999999999127 becomes "358.35000000", which downstream
// normalization then trims to "358.35". This is the SQLite-only path;
// PostgreSQL returns an exact NUMERIC string and needs no float rounding.
func formatSQLiteBalance(f float64) string {
	return strconv.FormatFloat(f, 'f', 8, 64)
}

// sqliteQuerier is the passthrough adapter (the canonical types ARE sqlite's).
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
	f, err := sqlitegen.New(db).GetAccountBalance(ctx, sqlitegen.GetAccountBalanceParams{
		AccountID:          acc,
		SpentAt:            before,
		AccountID_2:        acc,
		SpentAt_2:          before,
		AccountID_3:        acc,
		SpentAt_3:          before,
		AccountRecipientID: &acc,
		SpentAt_4:          before,
	})
	if err != nil {
		return "", err
	}
	return formatSQLiteBalance(f), nil
}

func (sqliteBalance) ListAccountBalancesForUser(ctx context.Context, db backend.DBTX, userID string, before time.Time) ([]balanceResult, error) {
	rows, err := sqlitegen.New(db).ListAccountBalancesForUser(ctx, sqlitegen.ListAccountBalancesForUserParams{
		SpentAt:   before,
		SpentAt_2: before,
		SpentAt_3: before,
		SpentAt_4: before,
		UserID:    userID, // own
		UserID_2:  userID, // shared (accounts_access)
	})
	if err != nil {
		return nil, err
	}
	out := make([]balanceResult, len(rows))
	for i, r := range rows {
		out[i] = balanceResult{AccountID: r.AccountID, Balance: formatSQLiteBalance(r.Balance)}
	}
	return out, nil
}
