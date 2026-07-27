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

func (emailChangeSqliteQuerier) InsertUserEmailChangeRequest(ctx context.Context, db backend.DBTX, p emailChangeInsertParams) error {
	return sqlitegen.New(db).InsertUserEmailChangeRequest(ctx, p)
}

func (emailChangeSqliteQuerier) GetUserEmailChangeRequestByUser(ctx context.Context, db backend.DBTX, userID string) (emailChangeRow, error) {
	return sqlitegen.New(db).GetUserEmailChangeRequestByUser(ctx, userID)
}
