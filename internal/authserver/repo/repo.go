// Package repo implements authserver.Repository.
package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	clientRow              = sqlitegen.OauthClient
	codeRow                = sqlitegen.OauthAuthorizationCode
	grantRow               = sqlitegen.OauthGrant
	listGrantRow           = sqlitegen.ListUnrevokedOAuthGrantsRow
	insertClientParams     = sqlitegen.InsertOAuthClientParams
	markClientUsedParams   = sqlitegen.MarkOAuthClientUsedParams
	insertCodeParams       = sqlitegen.InsertOAuthCodeParams
	insertGrantParams      = sqlitegen.InsertOAuthGrantParams
	rotateGrantParams      = sqlitegen.RotateOAuthGrantParams
	revokeGrantParams      = sqlitegen.RevokeOAuthGrantParams
	revokeUserGrantsParams = sqlitegen.RevokeUserOAuthGrantsParams
	deleteDeadParams       = sqlitegen.DeleteDeadOAuthGrantsParams
)

type querier interface {
	InsertOAuthClient(ctx context.Context, db backend.DBTX, p insertClientParams) error
	GetOAuthClient(ctx context.Context, db backend.DBTX, id string) (clientRow, error)
	MarkOAuthClientUsed(ctx context.Context, db backend.DBTX, p markClientUsedParams) error
	PurgeUnusedOAuthClients(ctx context.Context, db backend.DBTX, createdBefore time.Time) (int64, error)

	InsertOAuthCode(ctx context.Context, db backend.DBTX, p insertCodeParams) error
	ConsumeOAuthCode(ctx context.Context, db backend.DBTX, codeHash string) (codeRow, error)
	PurgeExpiredOAuthCodes(ctx context.Context, db backend.DBTX, before time.Time) error

	InsertOAuthGrant(ctx context.Context, db backend.DBTX, p insertGrantParams) error
	GetOAuthGrant(ctx context.Context, db backend.DBTX, id string) (grantRow, error)
	GetOAuthGrantByRefreshHash(ctx context.Context, db backend.DBTX, hash string) (grantRow, error)
	GetOAuthGrantByPrevRefreshHash(ctx context.Context, db backend.DBTX, hash *string) (grantRow, error)
	RotateOAuthGrant(ctx context.Context, db backend.DBTX, p rotateGrantParams) (int64, error)
	RevokeOAuthGrant(ctx context.Context, db backend.DBTX, p revokeGrantParams) (int64, error)
	RevokeUserOAuthGrants(ctx context.Context, db backend.DBTX, p revokeUserGrantsParams) (int64, error)
	ListUnrevokedOAuthGrants(ctx context.Context, db backend.DBTX, userID string) ([]listGrantRow, error)
	DeleteDeadOAuthGrants(ctx context.Context, db backend.DBTX, p deleteDeadParams) (int64, error)
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("authserverrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) InsertClient(ctx context.Context, c *model.OAuthClient) error {
	uris, err := json.Marshal(c.RedirectURIs)
	if err != nil {
		return err
	}
	return r.q.InsertOAuthClient(ctx, r.db(ctx), insertClientParams{
		ID: c.ID.String(), Name: c.Name, RedirectUris: string(uris),
		SecretHash: c.SecretHash, CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt,
	})
}

func (r *Repo) GetClient(ctx context.Context, id vo.Id) (*model.OAuthClient, error) {
	row, err := r.q.GetOAuthClient(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("OAuth client not found")
		}
		return nil, err
	}
	cid, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	uris, err := parseURIs(row.RedirectUris)
	if err != nil {
		return nil, err
	}
	return &model.OAuthClient{ID: cid, Name: row.Name, RedirectURIs: uris, SecretHash: row.SecretHash,
		CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt}, nil
}

func (r *Repo) MarkClientUsed(ctx context.Context, id vo.Id, now time.Time) error {
	return r.q.MarkOAuthClientUsed(ctx, r.db(ctx), markClientUsedParams{LastUsedAt: &now, ID: id.String()})
}

func (r *Repo) PurgeUnusedClients(ctx context.Context, createdBefore time.Time) (int64, error) {
	return r.q.PurgeUnusedOAuthClients(ctx, r.db(ctx), createdBefore)
}

func (r *Repo) InsertCode(ctx context.Context, c *model.OAuthAuthorizationCode) error {
	return r.q.InsertOAuthCode(ctx, r.db(ctx), insertCodeParams{
		CodeHash: c.CodeHash, ClientID: c.ClientID.String(), UserID: c.UserID.String(),
		RedirectUri: c.RedirectURI, CodeChallenge: c.CodeChallenge, Resource: c.Resource,
		CredentialsGeneration: c.CredentialsGeneration, CreatedAt: c.CreatedAt, ExpiresAt: c.ExpiresAt,
	})
}

func (r *Repo) ConsumeCode(ctx context.Context, codeHash string) (*model.OAuthAuthorizationCode, error) {
	row, err := r.q.ConsumeOAuthCode(ctx, r.db(ctx), codeHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Authorization code not found")
		}
		return nil, err
	}
	clientID, err := vo.ParseId(row.ClientID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.OAuthAuthorizationCode{
		CodeHash: row.CodeHash, ClientID: clientID, UserID: userID, RedirectURI: row.RedirectUri,
		CodeChallenge: row.CodeChallenge, Resource: row.Resource, CredentialsGeneration: row.CredentialsGeneration,
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
	}, nil
}

func (r *Repo) PurgeExpiredCodes(ctx context.Context, before time.Time) error {
	return r.q.PurgeExpiredOAuthCodes(ctx, r.db(ctx), before)
}

func (r *Repo) InsertGrant(ctx context.Context, g *model.OAuthGrant) error {
	return r.q.InsertOAuthGrant(ctx, r.db(ctx), insertGrantParams{
		ID: g.ID.String(), UserID: g.UserID.String(), ClientID: g.ClientID.String(),
		RefreshTokenHash: g.RefreshTokenHash, PrevRefreshTokenHash: g.PrevRefreshTokenHash, RotatedAt: g.RotatedAt,
		CreatedAt: g.CreatedAt, LastUsedAt: g.LastUsedAt, ExpiresAt: g.ExpiresAt, RevokedAt: g.RevokedAt,
	})
}

func (r *Repo) GetGrant(ctx context.Context, id vo.Id) (*model.OAuthGrant, error) {
	row, err := r.q.GetOAuthGrant(ctx, r.db(ctx), id.String())
	return hydrateGrantResult(row, err)
}

func (r *Repo) GetGrantByRefreshHash(ctx context.Context, hash string) (*model.OAuthGrant, error) {
	row, err := r.q.GetOAuthGrantByRefreshHash(ctx, r.db(ctx), hash)
	return hydrateGrantResult(row, err)
}

func (r *Repo) GetGrantByPrevRefreshHash(ctx context.Context, hash string) (*model.OAuthGrant, error) {
	row, err := r.q.GetOAuthGrantByPrevRefreshHash(ctx, r.db(ctx), &hash)
	return hydrateGrantResult(row, err)
}

func (r *Repo) RotateGrant(ctx context.Context, id vo.Id, oldHash, newHash string, now, expiresAt time.Time) (int64, error) {
	return r.q.RotateOAuthGrant(ctx, r.db(ctx), rotateGrantParams{
		RefreshTokenHash: newHash, RotatedAt: &now, LastUsedAt: now, ExpiresAt: expiresAt,
		ID: id.String(), RefreshTokenHash_2: oldHash,
	})
}

func (r *Repo) RevokeGrant(ctx context.Context, id vo.Id, now time.Time) (int64, error) {
	return r.q.RevokeOAuthGrant(ctx, r.db(ctx), revokeGrantParams{RevokedAt: &now, ID: id.String()})
}

func (r *Repo) RevokeUserGrants(ctx context.Context, userID vo.Id, now time.Time) (int64, error) {
	return r.q.RevokeUserOAuthGrants(ctx, r.db(ctx), revokeUserGrantsParams{RevokedAt: &now, UserID: userID.String()})
}

func (r *Repo) ListUnrevokedGrants(ctx context.Context, userID vo.Id) ([]model.ConnectedGrant, error) {
	rows, err := r.q.ListUnrevokedOAuthGrants(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ConnectedGrant, 0, len(rows))
	for _, row := range rows {
		g, err := hydrateGrant(grantRow{
			ID: row.ID, UserID: row.UserID, ClientID: row.ClientID, RefreshTokenHash: row.RefreshTokenHash,
			PrevRefreshTokenHash: row.PrevRefreshTokenHash, RotatedAt: row.RotatedAt, CreatedAt: row.CreatedAt,
			LastUsedAt: row.LastUsedAt, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt,
		})
		if err != nil {
			return nil, err
		}
		uris, err := parseURIs(row.RedirectUris)
		if err != nil {
			return nil, err
		}
		out = append(out, model.ConnectedGrant{Grant: *g, ClientName: row.Name, RedirectURIs: uris})
	}
	return out, nil
}

func (r *Repo) DeleteDeadGrants(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.DeleteDeadOAuthGrants(ctx, r.db(ctx), deleteDeadParams{RevokedAt: &cutoff, ExpiresAt: cutoff})
}

func hydrateGrantResult(row grantRow, err error) (*model.OAuthGrant, error) {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Grant not found")
		}
		return nil, err
	}
	return hydrateGrant(row)
}

func hydrateGrant(row grantRow) (*model.OAuthGrant, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	clientID, err := vo.ParseId(row.ClientID)
	if err != nil {
		return nil, err
	}
	return &model.OAuthGrant{
		ID: id, UserID: userID, ClientID: clientID,
		RefreshTokenHash: row.RefreshTokenHash, PrevRefreshTokenHash: row.PrevRefreshTokenHash, RotatedAt: row.RotatedAt,
		CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt,
	}, nil
}

func parseURIs(raw string) ([]string, error) {
	var uris []string
	if err := json.Unmarshal([]byte(raw), &uris); err != nil {
		return nil, err
	}
	return uris, nil
}
