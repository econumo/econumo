package passwordrequestrepo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) DeleteUserPasswordRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).DeleteUserPasswordRequestsByUser(ctx, userID)
}

func (sqliteQuerier) InsertUserPasswordRequest(ctx context.Context, db backend.DBTX, p insertParams) error {
	return sqlitegen.New(db).InsertUserPasswordRequest(ctx, p)
}

func (sqliteQuerier) GetUserPasswordRequestByUserAndCode(ctx context.Context, db backend.DBTX, p getByUserAndCodeParams) (passwordRequestRow, error) {
	return sqlitegen.New(db).GetUserPasswordRequestByUserAndCode(ctx, p)
}

func (sqliteQuerier) DeleteUserPasswordRequest(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteUserPasswordRequest(ctx, id)
}
