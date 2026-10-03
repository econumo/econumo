package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) InsertOAuthClient(ctx context.Context, db backend.DBTX, p insertClientParams) error {
	return pgsqlgen.New(db).InsertOAuthClient(ctx, pgsqlgen.InsertOAuthClientParams(p))
}

func (pgsqlQuerier) GetOAuthClient(ctx context.Context, db backend.DBTX, id string) (clientRow, error) {
	row, err := pgsqlgen.New(db).GetOAuthClient(ctx, id)
	return clientRow(row), err
}

func (pgsqlQuerier) MarkOAuthClientUsed(ctx context.Context, db backend.DBTX, p markClientUsedParams) error {
	return pgsqlgen.New(db).MarkOAuthClientUsed(ctx, pgsqlgen.MarkOAuthClientUsedParams(p))
}

func (pgsqlQuerier) PurgeUnusedOAuthClients(ctx context.Context, db backend.DBTX, createdBefore time.Time) (int64, error) {
	return pgsqlgen.New(db).PurgeUnusedOAuthClients(ctx, createdBefore)
}

func (pgsqlQuerier) InsertOAuthCode(ctx context.Context, db backend.DBTX, p insertCodeParams) error {
	return pgsqlgen.New(db).InsertOAuthCode(ctx, pgsqlgen.InsertOAuthCodeParams(p))
}

func (pgsqlQuerier) ConsumeOAuthCode(ctx context.Context, db backend.DBTX, codeHash string) (codeRow, error) {
	row, err := pgsqlgen.New(db).ConsumeOAuthCode(ctx, codeHash)
	return codeRow(row), err
}

func (pgsqlQuerier) PurgeExpiredOAuthCodes(ctx context.Context, db backend.DBTX, before time.Time) error {
	return pgsqlgen.New(db).PurgeExpiredOAuthCodes(ctx, before)
}

func (pgsqlQuerier) InsertOAuthGrant(ctx context.Context, db backend.DBTX, p insertGrantParams) error {
	return pgsqlgen.New(db).InsertOAuthGrant(ctx, pgsqlgen.InsertOAuthGrantParams(p))
}

func (pgsqlQuerier) GetOAuthGrant(ctx context.Context, db backend.DBTX, id string) (grantRow, error) {
	row, err := pgsqlgen.New(db).GetOAuthGrant(ctx, id)
	return grantRow(row), err
}

func (pgsqlQuerier) GetOAuthGrantByRefreshHash(ctx context.Context, db backend.DBTX, hash string) (grantRow, error) {
	row, err := pgsqlgen.New(db).GetOAuthGrantByRefreshHash(ctx, hash)
	return grantRow(row), err
}

func (pgsqlQuerier) GetOAuthGrantByPrevRefreshHash(ctx context.Context, db backend.DBTX, hash *string) (grantRow, error) {
	row, err := pgsqlgen.New(db).GetOAuthGrantByPrevRefreshHash(ctx, hash)
	return grantRow(row), err
}

func (pgsqlQuerier) RotateOAuthGrant(ctx context.Context, db backend.DBTX, p rotateGrantParams) (int64, error) {
	return pgsqlgen.New(db).RotateOAuthGrant(ctx, pgsqlgen.RotateOAuthGrantParams(p))
}

func (pgsqlQuerier) RevokeOAuthGrant(ctx context.Context, db backend.DBTX, p revokeGrantParams) (int64, error) {
	return pgsqlgen.New(db).RevokeOAuthGrant(ctx, pgsqlgen.RevokeOAuthGrantParams(p))
}

func (pgsqlQuerier) RevokeUserOAuthGrants(ctx context.Context, db backend.DBTX, p revokeUserGrantsParams) (int64, error) {
	return pgsqlgen.New(db).RevokeUserOAuthGrants(ctx, pgsqlgen.RevokeUserOAuthGrantsParams(p))
}

func (pgsqlQuerier) ListUnrevokedOAuthGrants(ctx context.Context, db backend.DBTX, userID string) ([]listGrantRow, error) {
	rows, err := pgsqlgen.New(db).ListUnrevokedOAuthGrants(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]listGrantRow, len(rows))
	for i, row := range rows {
		out[i] = listGrantRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteDeadOAuthGrants(ctx context.Context, db backend.DBTX, p deleteDeadParams) (int64, error) {
	return pgsqlgen.New(db).DeleteDeadOAuthGrants(ctx, pgsqlgen.DeleteDeadOAuthGrantsParams(p))
}
