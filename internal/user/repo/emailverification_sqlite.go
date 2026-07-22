package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type emailVerificationSqliteQuerier struct{}

var _ emailVerificationQuerier = emailVerificationSqliteQuerier{}

func (emailVerificationSqliteQuerier) DeleteUserEmailVerificationsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).DeleteUserEmailVerificationsByUser(ctx, userID)
}

func (emailVerificationSqliteQuerier) InsertUserEmailVerification(ctx context.Context, db backend.DBTX, p emailVerificationInsertParams) error {
	return sqlitegen.New(db).InsertUserEmailVerification(ctx, p)
}

func (emailVerificationSqliteQuerier) GetUserEmailVerificationByUser(ctx context.Context, db backend.DBTX, userID string) (emailVerificationRow, error) {
	return sqlitegen.New(db).GetUserEmailVerificationByUser(ctx, userID)
}
