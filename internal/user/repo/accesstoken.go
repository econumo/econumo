// AccessTokenRepo persists opaque bearer credentials (access_tokens): login
// sessions and personal access tokens. Liveness is evaluated in the domain
// (model.AccessToken.IsLive), not in SQL.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

type (
	accessTokenRow                   = sqlitegen.AccessToken
	accessTokenWithAccessRow         = sqlitegen.GetAccessTokenByHashRow
	touchAccessTokenParams           = sqlitegen.TouchAccessTokenParams
	revokeAccessTokenParams          = sqlitegen.RevokeAccessTokenParams
	revokeUserAccessTokensParams     = sqlitegen.RevokeUserAccessTokensParams
	listAccessTokensParams           = sqlitegen.ListAccessTokensByUserParams
	deleteDeadAccessTokParams        = sqlitegen.DeleteDeadAccessTokensParams
	insertTokenIfGenParams           = sqlitegen.InsertAccessTokenIfGenerationParams
	insertTokenIfPresenterLiveParams = sqlitegen.InsertAccessTokenIfPresenterLiveParams
)

type accessTokenQuerier interface {
	InsertAccessTokenIfGeneration(ctx context.Context, db backend.DBTX, p insertTokenIfGenParams) (int64, error)
	InsertAccessTokenIfPresenterLive(ctx context.Context, db backend.DBTX, p insertTokenIfPresenterLiveParams) (int64, error)
	GetAccessTokenByHash(ctx context.Context, db backend.DBTX, hash string) (accessTokenWithAccessRow, error)
	GetAccessTokenByID(ctx context.Context, db backend.DBTX, id string) (accessTokenRow, error)
	TouchAccessToken(ctx context.Context, db backend.DBTX, p touchAccessTokenParams) (int64, error)
	RevokeAccessToken(ctx context.Context, db backend.DBTX, p revokeAccessTokenParams) error
	RevokeUserAccessTokens(ctx context.Context, db backend.DBTX, p revokeUserAccessTokensParams) error
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

func (r *AccessTokenRepo) InsertIfGeneration(ctx context.Context, t *model.AccessToken, generation int64) (int64, error) {
	if _, err := model.ParseTokenScope(string(t.Scope)); err != nil {
		return 0, fmt.Errorf("access token %s: %w", t.ID, err)
	}
	return r.q.InsertAccessTokenIfGeneration(ctx, r.db(ctx), insertTokenIfGenParams{
		ID: t.ID.String(), UserID: t.UserID.String(), Kind: t.Kind, TokenHash: t.TokenHash,
		Scope: string(t.Scope), Name: t.Name, UserAgent: t.UserAgent,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
		Provider: t.Provider, IDToken: t.IDToken,
		ID_2: t.UserID.String(), CredentialsGeneration: generation,
	})
}

func (r *AccessTokenRepo) InsertIfPresenterLive(ctx context.Context, t *model.AccessToken, presentingTokenID vo.Id) (int64, error) {
	if _, err := model.ParseTokenScope(string(t.Scope)); err != nil {
		return 0, fmt.Errorf("access token %s: %w", t.ID, err)
	}
	// ID_2 is the presenting token's id (the guard condition); UserID_2 is the
	// owner the new row is inserted for — t.UserID, not the presenter's user.
	return r.q.InsertAccessTokenIfPresenterLive(ctx, r.db(ctx), insertTokenIfPresenterLiveParams{
		ID: t.ID.String(), UserID: t.UserID.String(), Kind: t.Kind, TokenHash: t.TokenHash,
		Scope: string(t.Scope), Name: t.Name, UserAgent: t.UserAgent,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
		Provider: t.Provider, IDToken: t.IDToken,
		ID_2: presentingTokenID.String(), UserID_2: t.UserID.String(),
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
// The hot-path query does not select provider/id_token (see
// GetAccessTokenByHash's SQL comment), so both stay nil here.
func tokenRowFromHashRow(row accessTokenWithAccessRow) accessTokenRow {
	return accessTokenRow{
		ID: row.ID, UserID: row.UserID, Kind: row.Kind, TokenHash: row.TokenHash,
		Scope: row.Scope, Name: row.Name, UserAgent: row.UserAgent,
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

func (r *AccessTokenRepo) Touch(ctx context.Context, id vo.Id, lastUsedAt time.Time, expiresAt *time.Time) (int64, error) {
	return r.q.TouchAccessToken(ctx, r.db(ctx), touchAccessTokenParams{
		LastUsedAt: lastUsedAt, ExpiresAt: expiresAt, ID: id.String(),
	})
}

func (r *AccessTokenRepo) Revoke(ctx context.Context, id vo.Id, now time.Time) error {
	return r.q.RevokeAccessToken(ctx, r.db(ctx), revokeAccessTokenParams{RevokedAt: &now, ID: id.String()})
}

// noExceptedTokenID stands in for the zero id in RevokeAll's `id <> ?` guard.
// The zero id's own string is empty, which PostgreSQL rejects as uuid syntax
// before the statement ever runs; the nil UUID is valid uuid text on both
// engines and can never collide with a real UUIDv7 id.
const noExceptedTokenID = "00000000-0000-0000-0000-000000000000"

func (r *AccessTokenRepo) RevokeAll(ctx context.Context, userID vo.Id, kind string, exceptID vo.Id, now time.Time) error {
	except := noExceptedTokenID
	if !exceptID.IsZero() {
		except = exceptID.String()
	}
	return r.q.RevokeUserAccessTokens(ctx, r.db(ctx), revokeUserAccessTokensParams{
		RevokedAt: &now, UserID: userID.String(), Kind: kind, ID: except,
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
		Scope: model.TokenScope(row.Scope), Name: row.Name, UserAgent: row.UserAgent,
		CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
		ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt,
		Provider: row.Provider, IDToken: row.IDToken,
	}, nil
}
