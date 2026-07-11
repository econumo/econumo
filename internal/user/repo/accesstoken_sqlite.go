package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type accessTokenSqliteQuerier struct{}

var _ accessTokenQuerier = accessTokenSqliteQuerier{}

func (accessTokenSqliteQuerier) InsertAccessToken(ctx context.Context, db backend.DBTX, p insertAccessTokenParams) error {
	return sqlitegen.New(db).InsertAccessToken(ctx, p)
}

func (accessTokenSqliteQuerier) GetAccessTokenByHash(ctx context.Context, db backend.DBTX, hash string) (accessTokenRow, error) {
	return sqlitegen.New(db).GetAccessTokenByHash(ctx, hash)
}

func (accessTokenSqliteQuerier) GetAccessTokenByID(ctx context.Context, db backend.DBTX, id string) (accessTokenRow, error) {
	return sqlitegen.New(db).GetAccessTokenByID(ctx, id)
}

func (accessTokenSqliteQuerier) UpdateAccessToken(ctx context.Context, db backend.DBTX, p updateAccessTokenParams) error {
	return sqlitegen.New(db).UpdateAccessToken(ctx, p)
}

func (accessTokenSqliteQuerier) ListAccessTokensByUser(ctx context.Context, db backend.DBTX, p listAccessTokensParams) ([]accessTokenRow, error) {
	return sqlitegen.New(db).ListAccessTokensByUser(ctx, p)
}

func (accessTokenSqliteQuerier) DeleteAccessToken(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteAccessToken(ctx, id)
}
