package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type emailChangePgsqlQuerier struct{}

var _ emailChangeQuerier = emailChangePgsqlQuerier{}

func (emailChangePgsqlQuerier) DeleteUserEmailChangeRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteUserEmailChangeRequestsByUser(ctx, userID)
}

func (emailChangePgsqlQuerier) InsertUserEmailChangeRequestIfGeneration(ctx context.Context, db backend.DBTX, p emailChangeInsertParams) (int64, error) {
	return pgsqlgen.New(db).InsertUserEmailChangeRequestIfGeneration(ctx, pgsqlgen.InsertUserEmailChangeRequestIfGenerationParams(p))
}

func (emailChangePgsqlQuerier) ConsumeUserEmailChangeRequest(ctx context.Context, db backend.DBTX, p emailChangeConsumeParams) (int64, error) {
	return pgsqlgen.New(db).ConsumeUserEmailChangeRequest(ctx, pgsqlgen.ConsumeUserEmailChangeRequestParams(p))
}

func (emailChangePgsqlQuerier) GetUserEmailChangeRequestByUser(ctx context.Context, db backend.DBTX, userID string) (emailChangeRow, error) {
	row, err := pgsqlgen.New(db).GetUserEmailChangeRequestByUser(ctx, userID)
	return emailChangeRow(row), err
}
