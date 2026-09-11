// Package repo implements oauth.Identities, oauth.States and oauth.Handoffs.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	identityRow            = sqlitegen.UsersIdentity
	upsertIdentityParams   = sqlitegen.UpsertIdentityParams
	identityByUserProvider = sqlitegen.GetIdentityByUserProviderParams
	identityBySubject      = sqlitegen.GetIdentityByProviderSubjectParams
	deleteIdentityParams   = sqlitegen.DeleteIdentityByUserProviderParams
)

type identityQuerier interface {
	GetIdentityByProviderSubject(ctx context.Context, db backend.DBTX, p identityBySubject) (identityRow, error)
	GetIdentityByUserProvider(ctx context.Context, db backend.DBTX, p identityByUserProvider) (identityRow, error)
	ListIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) ([]identityRow, error)
	CountIdentitiesByUser(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertIdentity(ctx context.Context, db backend.DBTX, p upsertIdentityParams) error
	DeleteIdentityByUserProvider(ctx context.Context, db backend.DBTX, p deleteIdentityParams) (int64, error)
}

type IdentityRepo struct {
	tx *backend.TxManager
	q  identityQuerier
}

var _ appoauth.Identities = (*IdentityRepo)(nil)

func NewIdentityRepo(driver string, tx *backend.TxManager) *IdentityRepo {
	switch driver {
	case "sqlite":
		return &IdentityRepo{tx: tx, q: identitySqliteQuerier{}}
	case "postgresql":
		return &IdentityRepo{tx: tx, q: identityPgsqlQuerier{}}
	default:
		panic("oauthrepo: unknown database driver " + driver)
	}
}

func (r *IdentityRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *IdentityRepo) NextIdentity() vo.Id { return vo.NewId() }

func (r *IdentityRepo) GetByProviderSubject(ctx context.Context, provider, subject string) (*model.Identity, error) {
	row, err := r.q.GetIdentityByProviderSubject(ctx, r.db(ctx), identityBySubject{Provider: provider, Subject: subject})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Identity not found")
		}
		return nil, err
	}
	return identityFromRow(row)
}

func (r *IdentityRepo) GetByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.Identity, error) {
	row, err := r.q.GetIdentityByUserProvider(ctx, r.db(ctx), identityByUserProvider{UserID: userID.String(), Provider: provider})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Identity not found")
		}
		return nil, err
	}
	return identityFromRow(row)
}

func (r *IdentityRepo) ListByUser(ctx context.Context, userID vo.Id) ([]model.Identity, error) {
	rows, err := r.q.ListIdentitiesByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.Identity, 0, len(rows))
	for _, row := range rows {
		i, err := identityFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, nil
}

func (r *IdentityRepo) CountByUser(ctx context.Context, userID vo.Id) (int64, error) {
	return r.q.CountIdentitiesByUser(ctx, r.db(ctx), userID.String())
}

func (r *IdentityRepo) Save(ctx context.Context, i *model.Identity) error {
	return r.q.UpsertIdentity(ctx, r.db(ctx), upsertIdentityParams{
		ID: i.ID.String(), UserID: i.UserID.String(), Provider: i.Provider, Subject: i.Subject, Email: i.Email,
		CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	})
}

func (r *IdentityRepo) DeleteByUserProvider(ctx context.Context, userID vo.Id, provider string) (int64, error) {
	return r.q.DeleteIdentityByUserProvider(ctx, r.db(ctx), deleteIdentityParams{UserID: userID.String(), Provider: provider})
}

func identityFromRow(row identityRow) (*model.Identity, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	uid, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.Identity{ID: id, UserID: uid, Provider: row.Provider, Subject: row.Subject, Email: row.Email,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
