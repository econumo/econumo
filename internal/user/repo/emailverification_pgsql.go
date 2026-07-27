package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type emailVerificationPgsqlQuerier struct{}

var _ emailVerificationQuerier = emailVerificationPgsqlQuerier{}

func (emailVerificationPgsqlQuerier) DeleteUserEmailVerificationsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteUserEmailVerificationsByUser(ctx, userID)
}

func (emailVerificationPgsqlQuerier) InsertUserEmailVerification(ctx context.Context, db backend.DBTX, p emailVerificationInsertParams) error {
	return pgsqlgen.New(db).InsertUserEmailVerification(ctx, pgsqlgen.InsertUserEmailVerificationParams(p))
}

func (emailVerificationPgsqlQuerier) GetUserEmailVerificationByUser(ctx context.Context, db backend.DBTX, userID string) (emailVerificationRow, error) {
	row, err := pgsqlgen.New(db).GetUserEmailVerificationByUser(ctx, userID)
	return emailVerificationRow(row), err
}
