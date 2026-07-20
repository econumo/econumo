package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type accessTokenPgsqlQuerier struct{}

var _ accessTokenQuerier = accessTokenPgsqlQuerier{}

func (accessTokenPgsqlQuerier) InsertAccessToken(ctx context.Context, db backend.DBTX, p insertAccessTokenParams) error {
	return pgsqlgen.New(db).InsertAccessToken(ctx, pgsqlgen.InsertAccessTokenParams(p))
}

func (accessTokenPgsqlQuerier) GetAccessTokenByHash(ctx context.Context, db backend.DBTX, hash string) (accessTokenWithAccessRow, error) {
	row, err := pgsqlgen.New(db).GetAccessTokenByHash(ctx, hash)
	return accessTokenWithAccessRow(row), err
}

func (accessTokenPgsqlQuerier) GetAccessTokenByID(ctx context.Context, db backend.DBTX, id string) (accessTokenRow, error) {
	row, err := pgsqlgen.New(db).GetAccessTokenByID(ctx, id)
	return accessTokenRow(row), err
}

func (accessTokenPgsqlQuerier) UpdateAccessToken(ctx context.Context, db backend.DBTX, p updateAccessTokenParams) error {
	return pgsqlgen.New(db).UpdateAccessToken(ctx, pgsqlgen.UpdateAccessTokenParams(p))
}

func (accessTokenPgsqlQuerier) ListAccessTokensByUser(ctx context.Context, db backend.DBTX, p listAccessTokensParams) ([]accessTokenRow, error) {
	rows, err := pgsqlgen.New(db).ListAccessTokensByUser(ctx, pgsqlgen.ListAccessTokensByUserParams(p))
	if err != nil {
		return nil, err
	}
	out := make([]accessTokenRow, len(rows))
	for i, row := range rows {
		out[i] = accessTokenRow(row)
	}
	return out, nil
}

func (accessTokenPgsqlQuerier) DeleteAccessToken(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteAccessToken(ctx, id)
}

func (accessTokenPgsqlQuerier) DeleteDeadAccessTokens(ctx context.Context, db backend.DBTX, p deleteDeadAccessTokParams) (int64, error) {
	return pgsqlgen.New(db).DeleteDeadAccessTokens(ctx, pgsqlgen.DeleteDeadAccessTokensParams(p))
}
