package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type handoffPgsqlQuerier struct{}

var _ handoffQuerier = handoffPgsqlQuerier{}

func (handoffPgsqlQuerier) InsertOAuthHandoff(ctx context.Context, db backend.DBTX, p insertHandoffParams) error {
	return pgsqlgen.New(db).InsertOAuthHandoff(ctx, pgsqlgen.InsertOAuthHandoffParams(p))
}

func (handoffPgsqlQuerier) GetOAuthHandoff(ctx context.Context, db backend.DBTX, codeHash string) (handoffRow, error) {
	row, err := pgsqlgen.New(db).GetOAuthHandoff(ctx, codeHash)
	return handoffRow(row), err
}

func (handoffPgsqlQuerier) DeleteOAuthHandoff(ctx context.Context, db backend.DBTX, codeHash string) (int64, error) {
	return pgsqlgen.New(db).DeleteOAuthHandoff(ctx, codeHash)
}

func (handoffPgsqlQuerier) DeleteExpiredOAuthHandoffs(ctx context.Context, db backend.DBTX, cutoff time.Time) (int64, error) {
	return pgsqlgen.New(db).DeleteExpiredOAuthHandoffs(ctx, cutoff)
}
