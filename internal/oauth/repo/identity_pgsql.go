package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type identityPgsqlQuerier struct{}

var _ identityQuerier = identityPgsqlQuerier{}

func (identityPgsqlQuerier) GetIdentityByProviderSubject(ctx context.Context, db backend.DBTX, p identityBySubject) (identityRow, error) {
	row, err := pgsqlgen.New(db).GetIdentityByProviderSubject(ctx, pgsqlgen.GetIdentityByProviderSubjectParams(p))
	return identityRow(row), err
}

func (identityPgsqlQuerier) GetIdentityByUserProvider(ctx context.Context, db backend.DBTX, p identityByUserProvider) (identityRow, error) {
	row, err := pgsqlgen.New(db).GetIdentityByUserProvider(ctx, pgsqlgen.GetIdentityByUserProviderParams(p))
	return identityRow(row), err
}

func (identityPgsqlQuerier) ListIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) ([]identityRow, error) {
	rows, err := pgsqlgen.New(db).ListIdentitiesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]identityRow, len(rows))
	for i, row := range rows {
		out[i] = identityRow(row)
	}
	return out, nil
}

func (identityPgsqlQuerier) CountIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountIdentitiesByUser(ctx, userID)
}

func (identityPgsqlQuerier) InsertIdentityIfGeneration(ctx context.Context, db backend.DBTX, p insertIdentityParams) (int64, error) {
	return pgsqlgen.New(db).InsertIdentityIfGeneration(ctx, pgsqlgen.InsertIdentityIfGenerationParams(p))
}

func (identityPgsqlQuerier) UpdateIdentityIfGeneration(ctx context.Context, db backend.DBTX, p updateIdentityParams) (int64, error) {
	return pgsqlgen.New(db).UpdateIdentityIfGeneration(ctx, pgsqlgen.UpdateIdentityIfGenerationParams(p))
}

func (identityPgsqlQuerier) DeleteIdentityByUserProvider(ctx context.Context, db backend.DBTX, p deleteIdentityParams) (int64, error) {
	return pgsqlgen.New(db).DeleteIdentityByUserProvider(ctx, pgsqlgen.DeleteIdentityByUserProviderParams(p))
}
