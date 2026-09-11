package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type stateSqliteQuerier struct{}

var _ stateQuerier = stateSqliteQuerier{}

func (stateSqliteQuerier) InsertOAuthState(ctx context.Context, db backend.DBTX, p insertStateParams) error {
	return sqlitegen.New(db).InsertOAuthState(ctx, p)
}

func (stateSqliteQuerier) GetOAuthState(ctx context.Context, db backend.DBTX, hash string) (stateRow, error) {
	return sqlitegen.New(db).GetOAuthState(ctx, hash)
}

func (stateSqliteQuerier) DeleteOAuthState(ctx context.Context, db backend.DBTX, hash string) (int64, error) {
	return sqlitegen.New(db).DeleteOAuthState(ctx, hash)
}

func (stateSqliteQuerier) DeleteExpiredOAuthStates(ctx context.Context, db backend.DBTX, cutoff time.Time) (int64, error) {
	return sqlitegen.New(db).DeleteExpiredOAuthStates(ctx, cutoff)
}
