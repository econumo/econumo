package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type identitySqliteQuerier struct{}

var _ identityQuerier = identitySqliteQuerier{}

func (identitySqliteQuerier) GetIdentityByProviderSubject(ctx context.Context, db backend.DBTX, p identityBySubject) (identityRow, error) {
	return sqlitegen.New(db).GetIdentityByProviderSubject(ctx, p)
}

func (identitySqliteQuerier) GetIdentityByUserProvider(ctx context.Context, db backend.DBTX, p identityByUserProvider) (identityRow, error) {
	return sqlitegen.New(db).GetIdentityByUserProvider(ctx, p)
}

func (identitySqliteQuerier) ListIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) ([]identityRow, error) {
	return sqlitegen.New(db).ListIdentitiesByUser(ctx, userID)
}

func (identitySqliteQuerier) CountIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return sqlitegen.New(db).CountIdentitiesByUser(ctx, userID)
}

func (identitySqliteQuerier) InsertIdentityIfGeneration(ctx context.Context, db backend.DBTX, p insertIdentityParams) (int64, error) {
	return sqlitegen.New(db).InsertIdentityIfGeneration(ctx, p)
}

func (identitySqliteQuerier) UpdateIdentityIfGeneration(ctx context.Context, db backend.DBTX, p updateIdentityParams) (int64, error) {
	return sqlitegen.New(db).UpdateIdentityIfGeneration(ctx, p)
}

func (identitySqliteQuerier) DeleteIdentityByUserProvider(ctx context.Context, db backend.DBTX, p deleteIdentityParams) (int64, error) {
	return sqlitegen.New(db).DeleteIdentityByUserProvider(ctx, p)
}
