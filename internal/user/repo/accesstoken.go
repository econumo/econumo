// AccessTokenRepo persists opaque bearer credentials (access_tokens): login
// sessions and personal access tokens. Liveness is evaluated in the domain
// (model.AccessToken.IsLive), not in SQL.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

type (
	accessTokenRow            = sqlitegen.AccessToken
	accessTokenWithAccessRow  = sqlitegen.GetAccessTokenByHashRow
	insertAccessTokenParams   = sqlitegen.InsertAccessTokenParams
	updateAccessTokenParams   = sqlitegen.UpdateAccessTokenParams
	listAccessTokensParams    = sqlitegen.ListAccessTokensByUserParams
	deleteDeadAccessTokParams = sqlitegen.DeleteDeadAccessTokensParams
)

type accessTokenQuerier interface {
	InsertAccessToken(ctx context.Context, db backend.DBTX, p insertAccessTokenParams) error
	GetAccessTokenByHash(ctx context.Context, db backend.DBTX, hash string) (accessTokenWithAccessRow, error)
	GetAccessTokenByID(ctx context.Context, db backend.DBTX, id string) (accessTokenRow, error)
	UpdateAccessToken(ctx context.Context, db backend.DBTX, p updateAccessTokenParams) error
	ListAccessTokensByUser(ctx context.Context, db backend.DBTX, p listAccessTokensParams) ([]accessTokenRow, error)
	DeleteAccessToken(ctx context.Context, db backend.DBTX, id string) error
	DeleteDeadAccessTokens(ctx context.Context, db backend.DBTX, p deleteDeadAccessTokParams) (int64, error)
}

type AccessTokenRepo struct {
	tx *backend.TxManager
	q  accessTokenQuerier
}

var _ user.AccessTokens = (*AccessTokenRepo)(nil)

func NewAccessTokenRepo(driver string, tx *backend.TxManager) *AccessTokenRepo {
	switch driver {
	case "sqlite":
		return &AccessTokenRepo{tx: tx, q: accessTokenSqliteQuerier{}}
	case "postgresql":
		return &AccessTokenRepo{tx: tx, q: accessTokenPgsqlQuerier{}}
	default:
		panic("accesstokenrepo: unknown database driver " + driver)
	}
}

func (r *AccessTokenRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *AccessTokenRepo) Insert(ctx context.Context, t *model.AccessToken) error {
	return r.q.InsertAccessToken(ctx, r.db(ctx), insertAccessTokenParams{
		ID: t.ID.String(), UserID: t.UserID.String(), Kind: t.Kind, TokenHash: t.TokenHash,
		Name: t.Name, UserAgent: t.UserAgent,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
	})
}

func (r *AccessTokenRepo) GetByHash(ctx context.Context, hash string) (*model.AccessToken, model.AccessLevel, *time.Time, error) {
	row, err := r.q.GetAccessTokenByHash(ctx, r.db(ctx), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", nil, errs.NewNotFound("Access token not found")
		}
		return nil, "", nil, err
	}
	t, err := accessTokenFromRow(tokenRowFromHashRow(row))
	if err != nil {
		return nil, "", nil, err
	}
	level, err := model.ParseAccessLevel(row.AccessLevel)
	if err != nil {
		return nil, "", nil, err
	}
	return t, level, row.AccessUntil, nil
}

// tokenRowFromHashRow strips the joined access_level/access_until columns
// back down to the plain access_tokens row shape shared by every other query.
func tokenRowFromHashRow(row accessTokenWithAccessRow) accessTokenRow {
	return accessTokenRow{
		ID: row.ID, UserID: row.UserID, Kind: row.Kind, TokenHash: row.TokenHash,
		Name: row.Name, UserAgent: row.UserAgent,
		CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
		ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt,
	}
}

func (r *AccessTokenRepo) GetByID(ctx context.Context, id vo.Id) (*model.AccessToken, error) {
	row, err := r.q.GetAccessTokenByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Access token not found")
		}
		return nil, err
	}
	return accessTokenFromRow(row)
}

func (r *AccessTokenRepo) Update(ctx context.Context, t *model.AccessToken) error {
	return r.q.UpdateAccessToken(ctx, r.db(ctx), updateAccessTokenParams{
		LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt, ID: t.ID.String(),
	})
}

func (r *AccessTokenRepo) ListByUser(ctx context.Context, userID vo.Id, kind string) ([]model.AccessToken, error) {
	rows, err := r.q.ListAccessTokensByUser(ctx, r.db(ctx), listAccessTokensParams{UserID: userID.String(), Kind: kind})
	if err != nil {
		return nil, err
	}
	out := make([]model.AccessToken, 0, len(rows))
	for _, row := range rows {
		t, err := accessTokenFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

func (r *AccessTokenRepo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteAccessToken(ctx, r.db(ctx), id.String())
}

func (r *AccessTokenRepo) DeleteDead(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.DeleteDeadAccessTokens(ctx, r.db(ctx), deleteDeadAccessTokParams{RevokedAt: &cutoff, ExpiresAt: &cutoff})
}

func accessTokenFromRow(row accessTokenRow) (*model.AccessToken, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	uid, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.AccessToken{
		ID: id, UserID: uid, Kind: row.Kind, TokenHash: row.TokenHash,
		Name: row.Name, UserAgent: row.UserAgent,
		CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
		ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt,
	}, nil
}
