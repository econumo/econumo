package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) InsertOAuthClient(ctx context.Context, db backend.DBTX, p insertClientParams) error {
	return sqlitegen.New(db).InsertOAuthClient(ctx, p)
}

func (sqliteQuerier) GetOAuthClient(ctx context.Context, db backend.DBTX, id string) (clientRow, error) {
	return sqlitegen.New(db).GetOAuthClient(ctx, id)
}

func (sqliteQuerier) MarkOAuthClientUsed(ctx context.Context, db backend.DBTX, p markClientUsedParams) error {
	return sqlitegen.New(db).MarkOAuthClientUsed(ctx, p)
}

func (sqliteQuerier) PurgeExcessUnusedOAuthClients(ctx context.Context, db backend.DBTX, keep int64) (int64, error) {
	return sqlitegen.New(db).PurgeExcessUnusedOAuthClients(ctx, keep)
}

func (sqliteQuerier) PurgeUnusedOAuthClients(ctx context.Context, db backend.DBTX, createdBefore time.Time) (int64, error) {
	return sqlitegen.New(db).PurgeUnusedOAuthClients(ctx, createdBefore)
}

func (sqliteQuerier) InsertOAuthCode(ctx context.Context, db backend.DBTX, p insertCodeParams) error {
	return sqlitegen.New(db).InsertOAuthCode(ctx, p)
}

func (sqliteQuerier) ConsumeOAuthCode(ctx context.Context, db backend.DBTX, codeHash string) (codeRow, error) {
	return sqlitegen.New(db).ConsumeOAuthCode(ctx, codeHash)
}

func (sqliteQuerier) PurgeExpiredOAuthCodes(ctx context.Context, db backend.DBTX, before time.Time) error {
	return sqlitegen.New(db).PurgeExpiredOAuthCodes(ctx, before)
}

func (sqliteQuerier) DeleteClientOAuthCodes(ctx context.Context, db backend.DBTX, clientID string) (int64, error) {
	return sqlitegen.New(db).DeleteClientOAuthCodes(ctx, clientID)
}

func (sqliteQuerier) DeleteClientUserOAuthCodes(ctx context.Context, db backend.DBTX, p deleteUserCodesParams) (int64, error) {
	return sqlitegen.New(db).DeleteClientUserOAuthCodes(ctx, p)
}

func (sqliteQuerier) InsertOAuthGrant(ctx context.Context, db backend.DBTX, p insertGrantParams) error {
	return sqlitegen.New(db).InsertOAuthGrant(ctx, p)
}

func (sqliteQuerier) GetOAuthGrant(ctx context.Context, db backend.DBTX, id string) (grantRow, error) {
	return sqlitegen.New(db).GetOAuthGrant(ctx, id)
}

func (sqliteQuerier) GetOAuthGrantByRefreshHash(ctx context.Context, db backend.DBTX, hash string) (grantRow, error) {
	return sqlitegen.New(db).GetOAuthGrantByRefreshHash(ctx, hash)
}

func (sqliteQuerier) GetOAuthGrantBySpentRefreshHash(ctx context.Context, db backend.DBTX, hash string) (grantRow, error) {
	return sqlitegen.New(db).GetOAuthGrantBySpentRefreshHash(ctx, hash)
}

func (sqliteQuerier) InsertOAuthSpentRefreshHash(ctx context.Context, db backend.DBTX, p insertSpentParams) error {
	return sqlitegen.New(db).InsertOAuthSpentRefreshHash(ctx, p)
}

func (sqliteQuerier) RotateOAuthGrant(ctx context.Context, db backend.DBTX, p rotateGrantParams) (int64, error) {
	return sqlitegen.New(db).RotateOAuthGrant(ctx, p)
}

func (sqliteQuerier) RevokeOAuthGrant(ctx context.Context, db backend.DBTX, p revokeGrantParams) (int64, error) {
	return sqlitegen.New(db).RevokeOAuthGrant(ctx, p)
}

func (sqliteQuerier) RevokeUserOAuthGrants(ctx context.Context, db backend.DBTX, p revokeUserGrantsParams) (int64, error) {
	return sqlitegen.New(db).RevokeUserOAuthGrants(ctx, p)
}

func (sqliteQuerier) RevokeOtherOAuthGrants(ctx context.Context, db backend.DBTX, p revokeOtherGrantsParam) ([]string, error) {
	return sqlitegen.New(db).RevokeOtherOAuthGrants(ctx, p)
}

func (sqliteQuerier) ListUnrevokedOAuthGrants(ctx context.Context, db backend.DBTX, userID string) ([]listGrantRow, error) {
	return sqlitegen.New(db).ListUnrevokedOAuthGrants(ctx, userID)
}

func (sqliteQuerier) DeleteDeadOAuthGrants(ctx context.Context, db backend.DBTX, p deleteDeadParams) (int64, error) {
	return sqlitegen.New(db).DeleteDeadOAuthGrants(ctx, p)
}

func (sqliteQuerier) ClaimOAuthClient(ctx context.Context, db backend.DBTX, p claimClientParams) (int64, error) {
	return sqlitegen.New(db).ClaimOAuthClient(ctx, p)
}

func (sqliteQuerier) ListOAuthClients(ctx context.Context, db backend.DBTX) ([]clientRow, error) {
	return sqlitegen.New(db).ListOAuthClients(ctx)
}

func (sqliteQuerier) UpdateGlobalOAuthClient(ctx context.Context, db backend.DBTX, p updateGlobalParams) (int64, error) {
	return sqlitegen.New(db).UpdateGlobalOAuthClient(ctx, p)
}

func (sqliteQuerier) MakeOAuthClientGlobal(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	return sqlitegen.New(db).MakeOAuthClientGlobal(ctx, id)
}

func (sqliteQuerier) SetOAuthClientSecret(ctx context.Context, db backend.DBTX, p setClientSecretParams) (int64, error) {
	return sqlitegen.New(db).SetOAuthClientSecret(ctx, p)
}

func (sqliteQuerier) LockOAuthClient(ctx context.Context, db backend.DBTX, id string) (string, error) {
	return sqlitegen.New(db).LockOAuthClient(ctx, id)
}

func (sqliteQuerier) DeleteOAuthClient(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	return sqlitegen.New(db).DeleteOAuthClient(ctx, id)
}

func (sqliteQuerier) ListOAuthClientGrants(ctx context.Context, db backend.DBTX, clientID string) ([]grantRow, error) {
	return sqlitegen.New(db).ListOAuthClientGrants(ctx, clientID)
}

func (sqliteQuerier) ListAllUnrevokedOAuthGrants(ctx context.Context, db backend.DBTX) ([]grantRow, error) {
	return sqlitegen.New(db).ListAllUnrevokedOAuthGrants(ctx)
}

func (sqliteQuerier) RevokeClientOAuthGrants(ctx context.Context, db backend.DBTX, p revokeClientParams) ([]string, error) {
	return sqlitegen.New(db).RevokeClientOAuthGrants(ctx, p)
}
