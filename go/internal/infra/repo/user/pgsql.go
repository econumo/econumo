package userrepo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

// pgsqlQuerier implements querier over the pgsql-generated queries. The pgsql
// row/param structs are field-identical to the canonical (sqlite) types, but Go
// treats them as distinct types, so this shim field-copies across the boundary.
// That copy is the entire per-engine cost; there is no shape divergence to
// reconcile. Like sqliteQuerier it is stateless and binds a fresh *Queries to
// the caller-supplied DBTX on every call.
type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetUserByID(ctx context.Context, db backend.DBTX, id string) (userRow, error) {
	u, err := pgsqlgen.New(db).GetUserByID(ctx, id)
	return toUserRow(u), err
}

func (pgsqlQuerier) GetUserByIdentifier(ctx context.Context, db backend.DBTX, identifier string) (userRow, error) {
	u, err := pgsqlgen.New(db).GetUserByIdentifier(ctx, identifier)
	return toUserRow(u), err
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

// toUserRow converts a pgsql User into the canonical userRow. The struct
// conversion compiles only because the two types are field-identical.
func toUserRow(u pgsqlgen.User) userRow { return userRow(u) }
