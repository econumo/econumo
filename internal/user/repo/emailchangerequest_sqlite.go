package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type emailChangeSqliteQuerier struct{}

var _ emailChangeQuerier = emailChangeSqliteQuerier{}

func (emailChangeSqliteQuerier) DeleteUserEmailChangeRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).DeleteUserEmailChangeRequestsByUser(ctx, userID)
}

func (emailChangeSqliteQuerier) InsertUserEmailChangeRequestIfGeneration(ctx context.Context, db backend.DBTX, p emailChangeInsertParams) (int64, error) {
	return sqlitegen.New(db).InsertUserEmailChangeRequestIfGeneration(ctx, p)
}

func (emailChangeSqliteQuerier) ConsumeUserEmailChangeRequest(ctx context.Context, db backend.DBTX, p emailChangeConsumeParams) (int64, error) {
	return sqlitegen.New(db).ConsumeUserEmailChangeRequest(ctx, p)
}

func (emailChangeSqliteQuerier) GetUserEmailChangeRequestByUser(ctx context.Context, db backend.DBTX, userID string) (emailChangeRow, error) {
	return sqlitegen.New(db).GetUserEmailChangeRequestByUser(ctx, userID)
}
