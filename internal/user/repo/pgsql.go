package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetUserByID(ctx context.Context, db backend.DBTX, id string) (userRow, error) {
	u, err := pgsqlgen.New(db).GetUserByID(ctx, id)
	return userRow(u), err
}

func (pgsqlQuerier) GetUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (userRow, error) {
	u, err := pgsqlgen.New(db).GetUserByIdentifier(ctx, identifier)
	return userRow(u), err
}

func (pgsqlQuerier) ExistsUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (bool, error) {
	return pgsqlgen.New(db).ExistsUserByIdentifier(ctx, identifier)
}

func (pgsqlQuerier) ListUserIDs(ctx context.Context, db backend.DBTX) ([]string, error) {
	return pgsqlgen.New(db).ListUserIDs(ctx)
}

func (pgsqlQuerier) UpsertUser(ctx context.Context, db backend.DBTX, p userParams) error {
	return pgsqlgen.New(db).UpsertUser(ctx, pgsqlgen.UpsertUserParams(p))
}

func (pgsqlQuerier) GetUserOptions(ctx context.Context, db backend.DBTX, userID string) ([]optionRow, error) {
	rows, err := pgsqlgen.New(db).GetUserOptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]optionRow, len(rows))
	for i, o := range rows {
		out[i] = optionRow(o)
	}
	return out, nil
}

func (pgsqlQuerier) UpsertUserOption(ctx context.Context, db backend.DBTX, p optionParams) error {
	return pgsqlgen.New(db).UpsertUserOption(ctx, pgsqlgen.UpsertUserOptionParams(p))
}

func (pgsqlQuerier) UpdateUserLanguage(ctx context.Context, db backend.DBTX, p languageParams) error {
	return pgsqlgen.New(db).UpdateUserLanguage(ctx, pgsqlgen.UpdateUserLanguageParams(p))
}

func (pgsqlQuerier) GetUserTimezone(ctx context.Context, db backend.DBTX, id string) (string, error) {
	return pgsqlgen.New(db).GetUserTimezone(ctx, id)
}

func (pgsqlQuerier) UpdateUserTimezone(ctx context.Context, db backend.DBTX, p timezoneParams) error {
	return pgsqlgen.New(db).UpdateUserTimezone(ctx, pgsqlgen.UpdateUserTimezoneParams(p))
}

func (pgsqlQuerier) GetUserLanguage(ctx context.Context, db backend.DBTX, id string) (string, error) {
	return pgsqlgen.New(db).GetUserLanguage(ctx, id)
}
