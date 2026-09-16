package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type passwordRequestSqliteQuerier struct{}

var _ passwordRequestQuerier = passwordRequestSqliteQuerier{}

func (passwordRequestSqliteQuerier) DeleteUserPasswordRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).DeleteUserPasswordRequestsByUser(ctx, userID)
}

func (passwordRequestSqliteQuerier) InsertUserPasswordRequest(ctx context.Context, db backend.DBTX, p insertParams) error {
	return sqlitegen.New(db).InsertUserPasswordRequest(ctx, p)
}

func (passwordRequestSqliteQuerier) GetUserPasswordRequestByUserAndCode(ctx context.Context, db backend.DBTX, p getByUserAndCodeParams) (passwordRequestRow, error) {
	return sqlitegen.New(db).GetUserPasswordRequestByUserAndCode(ctx, p)
}

func (passwordRequestSqliteQuerier) ConsumeUserPasswordRequest(ctx context.Context, db backend.DBTX, p consumeParams) (int64, error) {
	return sqlitegen.New(db).ConsumeUserPasswordRequest(ctx, p)
}
