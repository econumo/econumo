package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) GetUserByID(ctx context.Context, db backend.DBTX, id string) (userRow, error) {
	row, err := sqlitegen.New(db).GetUserByID(ctx, id)
	return userRow(row), err
}

func (sqliteQuerier) GetUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (userRow, error) {
	row, err := sqlitegen.New(db).GetUserByIdentifier(ctx, identifier)
	return userRow(row), err
}

func (sqliteQuerier) ExistsUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (bool, error) {
	n, err := sqlitegen.New(db).ExistsUserByIdentifier(ctx, identifier)
	return n != 0, err
}

func (sqliteQuerier) ListUserIDs(ctx context.Context, db backend.DBTX) ([]string, error) {
	return sqlitegen.New(db).ListUserIDs(ctx)
}

func (sqliteQuerier) UpsertUser(ctx context.Context, db backend.DBTX, p userParams) error {
	return sqlitegen.New(db).UpsertUser(ctx, p)
}

func (sqliteQuerier) GetUserOptions(ctx context.Context, db backend.DBTX, userID string) ([]optionRow, error) {
	return sqlitegen.New(db).GetUserOptions(ctx, userID)
}

func (sqliteQuerier) UpsertUserOption(ctx context.Context, db backend.DBTX, p optionParams) error {
	return sqlitegen.New(db).UpsertUserOption(ctx, p)
}

func (sqliteQuerier) UpdateUserLanguage(ctx context.Context, db backend.DBTX, p languageParams) error {
	return sqlitegen.New(db).UpdateUserLanguage(ctx, p)
}
