package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type statePgsqlQuerier struct{}

var _ stateQuerier = statePgsqlQuerier{}

func (statePgsqlQuerier) InsertOAuthState(ctx context.Context, db backend.DBTX, p insertStateParams) error {
	return pgsqlgen.New(db).InsertOAuthState(ctx, pgsqlgen.InsertOAuthStateParams(p))
}

func (statePgsqlQuerier) GetOAuthState(ctx context.Context, db backend.DBTX, hash string) (stateRow, error) {
	row, err := pgsqlgen.New(db).GetOAuthState(ctx, hash)
	return stateRow(row), err
}

func (statePgsqlQuerier) DeleteOAuthState(ctx context.Context, db backend.DBTX, hash string) (int64, error) {
	return pgsqlgen.New(db).DeleteOAuthState(ctx, hash)
}

func (statePgsqlQuerier) DeleteExpiredOAuthStates(ctx context.Context, db backend.DBTX, cutoff time.Time) (int64, error) {
	return pgsqlgen.New(db).DeleteExpiredOAuthStates(ctx, cutoff)
}
