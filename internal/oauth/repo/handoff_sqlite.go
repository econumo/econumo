package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type handoffSqliteQuerier struct{}

var _ handoffQuerier = handoffSqliteQuerier{}

func (handoffSqliteQuerier) InsertOAuthHandoff(ctx context.Context, db backend.DBTX, p insertHandoffParams) error {
	return sqlitegen.New(db).InsertOAuthHandoff(ctx, p)
}

func (handoffSqliteQuerier) GetOAuthHandoff(ctx context.Context, db backend.DBTX, codeHash string) (handoffRow, error) {
	return sqlitegen.New(db).GetOAuthHandoff(ctx, codeHash)
}

func (handoffSqliteQuerier) DeleteOAuthHandoff(ctx context.Context, db backend.DBTX, codeHash string) error {
	return sqlitegen.New(db).DeleteOAuthHandoff(ctx, codeHash)
}

func (handoffSqliteQuerier) DeleteExpiredOAuthHandoffs(ctx context.Context, db backend.DBTX, cutoff time.Time) (int64, error) {
	return sqlitegen.New(db).DeleteExpiredOAuthHandoffs(ctx, cutoff)
}
