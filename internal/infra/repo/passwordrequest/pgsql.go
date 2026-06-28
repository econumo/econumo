package passwordrequestrepo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

// pgsqlQuerier implements querier over the pgsql-generated queries. The pgsql
// row/param structs are field-identical to the canonical (sqlite) types, but Go
// treats them as distinct types, so this shim whole-struct-converts across the
// boundary. Like sqliteQuerier it is stateless.
type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) DeleteUserPasswordRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteUserPasswordRequestsByUser(ctx, userID)
}

func (pgsqlQuerier) InsertUserPasswordRequest(ctx context.Context, db backend.DBTX, p insertParams) error {
	return pgsqlgen.New(db).InsertUserPasswordRequest(ctx, pgsqlgen.InsertUserPasswordRequestParams(p))
}

func (pgsqlQuerier) GetUserPasswordRequestByUserAndCode(ctx context.Context, db backend.DBTX, p getByUserAndCodeParams) (passwordRequestRow, error) {
	row, err := pgsqlgen.New(db).GetUserPasswordRequestByUserAndCode(ctx, pgsqlgen.GetUserPasswordRequestByUserAndCodeParams(p))
	return passwordRequestRow(row), err
}

func (pgsqlQuerier) DeleteUserPasswordRequest(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteUserPasswordRequest(ctx, id)
}
