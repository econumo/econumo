package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type passwordRequestPgsqlQuerier struct{}

var _ passwordRequestQuerier = passwordRequestPgsqlQuerier{}

func (passwordRequestPgsqlQuerier) DeleteUserPasswordRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteUserPasswordRequestsByUser(ctx, userID)
}

func (passwordRequestPgsqlQuerier) InsertUserPasswordRequest(ctx context.Context, db backend.DBTX, p insertParams) error {
	return pgsqlgen.New(db).InsertUserPasswordRequest(ctx, pgsqlgen.InsertUserPasswordRequestParams(p))
}

func (passwordRequestPgsqlQuerier) GetUserPasswordRequestByUserAndCode(ctx context.Context, db backend.DBTX, p getByUserAndCodeParams) (passwordRequestRow, error) {
	row, err := pgsqlgen.New(db).GetUserPasswordRequestByUserAndCode(ctx, pgsqlgen.GetUserPasswordRequestByUserAndCodeParams(p))
	return passwordRequestRow(row), err
}

func (passwordRequestPgsqlQuerier) DeleteUserPasswordRequest(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteUserPasswordRequest(ctx, id)
}
