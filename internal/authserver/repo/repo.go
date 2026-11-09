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
	revokeOtherGrantsParam = sqlitegen.RevokeOtherOAuthGrantsParams
	deleteDeadParams       = sqlitegen.DeleteDeadOAuthGrantsParams
	insertSpentParams      = sqlitegen.InsertOAuthSpentRefreshHashParams
	claimClientParams      = sqlitegen.ClaimOAuthClientParams
	updateGlobalParams     = sqlitegen.UpdateGlobalOAuthClientParams
	setClientSecretParams  = sqlitegen.SetOAuthClientSecretParams
	revokeClientParams     = sqlitegen.RevokeClientOAuthGrantsParams
	deleteUserCodesParams  = sqlitegen.DeleteClientUserOAuthCodesParams
)

type querier interface {
	InsertOAuthClient(ctx context.Context, db backend.DBTX, p insertClientParams) error
	GetOAuthClient(ctx context.Context, db backend.DBTX, id string) (clientRow, error)
	MarkOAuthClientUsed(ctx context.Context, db backend.DBTX, p markClientUsedParams) error
	PurgeUnusedOAuthClients(ctx context.Context, db backend.DBTX, createdBefore time.Time) (int64, error)
	PurgeExcessUnusedOAuthClients(ctx context.Context, db backend.DBTX, keep int64) (int64, error)
	ClaimOAuthClient(ctx context.Context, db backend.DBTX, p claimClientParams) (int64, error)
	ListOAuthClients(ctx context.Context, db backend.DBTX) ([]clientRow, error)
	UpdateGlobalOAuthClient(ctx context.Context, db backend.DBTX, p updateGlobalParams) (int64, error)
	MakeOAuthClientGlobal(ctx context.Context, db backend.DBTX, id string) (int64, error)
	SetOAuthClientSecret(ctx context.Context, db backend.DBTX, p setClientSecretParams) (int64, error)
	LockOAuthClient(ctx context.Context, db backend.DBTX, id string) (string, error)
	DeleteOAuthClient(ctx context.Context, db backend.DBTX, id string) (int64, error)

	InsertOAuthCode(ctx context.Context, db backend.DBTX, p insertCodeParams) error
	ConsumeOAuthCode(ctx context.Context, db backend.DBTX, codeHash string) (codeRow, error)
	PurgeExpiredOAuthCodes(ctx context.Context, db backend.DBTX, before time.Time) error
	DeleteClientOAuthCodes(ctx context.Context, db backend.DBTX, clientID string) (int64, error)
	DeleteClientUserOAuthCodes(ctx context.Context, db backend.DBTX, p deleteUserCodesParams) (int64, error)

	InsertOAuthGrant(ctx context.Context, db backend.DBTX, p insertGrantParams) error
	GetOAuthGrant(ctx context.Context, db backend.DBTX, id string) (grantRow, error)
	GetOAuthGrantByRefreshHash(ctx context.Context, db backend.DBTX, hash string) (grantRow, error)
	GetOAuthGrantBySpentRefreshHash(ctx context.Context, db backend.DBTX, hash string) (grantRow, error)
	InsertOAuthSpentRefreshHash(ctx context.Context, db backend.DBTX, p insertSpentParams) error
	RotateOAuthGrant(ctx context.Context, db backend.DBTX, p rotateGrantParams) (int64, error)
	RevokeOAuthGrant(ctx context.Context, db backend.DBTX, p revokeGrantParams) (int64, error)
	RevokeUserOAuthGrants(ctx context.Context, db backend.DBTX, p revokeUserGrantsParams) (int64, error)
	RevokeOtherOAuthGrants(ctx context.Context, db backend.DBTX, p revokeOtherGrantsParam) ([]string, error)
	ListUnrevokedOAuthGrants(ctx context.Context, db backend.DBTX, userID string) ([]listGrantRow, error)
	DeleteDeadOAuthGrants(ctx context.Context, db backend.DBTX, p deleteDeadParams) (int64, error)
	ListOAuthClientGrants(ctx context.Context, db backend.DBTX, clientID string) ([]grantRow, error)
	ListAllUnrevokedOAuthGrants(ctx context.Context, db backend.DBTX) ([]grantRow, error)
	RevokeClientOAuthGrants(ctx context.Context, db backend.DBTX, p revokeClientParams) ([]string, error)
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
		OwnerUserID: idPtrString(c.OwnerUserID), IsGlobal: c.IsGlobal,
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
	return hydrateClient(row)
}

func (r *Repo) ListClients(ctx context.Context) ([]model.OAuthClient, error) {
	rows, err := r.q.ListOAuthClients(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]model.OAuthClient, 0, len(rows))
	for _, row := range rows {
		c, err := hydrateClient(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

func (r *Repo) ClaimClient(ctx context.Context, id, userID vo.Id) (int64, error) {
	owner := userID.String()
	return r.q.ClaimOAuthClient(ctx, r.db(ctx), claimClientParams{OwnerUserID: &owner, ID: id.String()})
}

func (r *Repo) UpdateGlobalClient(ctx context.Context, id vo.Id, name string, redirectURIs []string) (int64, error) {
	uris, err := json.Marshal(redirectURIs)
	if err != nil {
		return 0, err
	}
	return r.q.UpdateGlobalOAuthClient(ctx, r.db(ctx), updateGlobalParams{Name: name, RedirectUris: string(uris), ID: id.String()})
}

func (r *Repo) MakeClientGlobal(ctx context.Context, id vo.Id) (int64, error) {
	return r.q.MakeOAuthClientGlobal(ctx, r.db(ctx), id.String())
}

func (r *Repo) SetClientSecret(ctx context.Context, id vo.Id, hash string) (int64, error) {
	return r.q.SetOAuthClientSecret(ctx, r.db(ctx), setClientSecretParams{SecretHash: &hash, ID: id.String()})
}

func (r *Repo) LockClient(ctx context.Context, id vo.Id) error {
	if _, err := r.q.LockOAuthClient(ctx, r.db(ctx), id.String()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewNotFound("OAuth client not found")
		}
		return err
	}
	return nil
}

func (r *Repo) DeleteClient(ctx context.Context, id vo.Id) (int64, error) {
	return r.q.DeleteOAuthClient(ctx, r.db(ctx), id.String())
}

func (r *Repo) MarkClientUsed(ctx context.Context, id vo.Id, now time.Time) error {
	return r.q.MarkOAuthClientUsed(ctx, r.db(ctx), markClientUsedParams{LastUsedAt: &now, ID: id.String()})
}

func (r *Repo) PurgeExcessUnusedClients(ctx context.Context, keep int64) (int64, error) {
	return r.q.PurgeExcessUnusedOAuthClients(ctx, r.db(ctx), keep)
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

func (r *Repo) DeleteClientCodes(ctx context.Context, clientID vo.Id, userID *vo.Id) (int64, error) {
	if userID == nil {
		return r.q.DeleteClientOAuthCodes(ctx, r.db(ctx), clientID.String())
	}
	return r.q.DeleteClientUserOAuthCodes(ctx, r.db(ctx), deleteUserCodesParams{ClientID: clientID.String(), UserID: userID.String()})
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

func (r *Repo) GetGrantBySpentRefreshHash(ctx context.Context, hash string) (*model.OAuthGrant, error) {
	row, err := r.q.GetOAuthGrantBySpentRefreshHash(ctx, r.db(ctx), hash)
	return hydrateGrantResult(row, err)
}

func (r *Repo) InsertSpentRefreshHash(ctx context.Context, grantID vo.Id, hash string, now time.Time) error {
	return r.q.InsertOAuthSpentRefreshHash(ctx, r.db(ctx), insertSpentParams{TokenHash: hash, GrantID: grantID.String(), SpentAt: now})
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

func (r *Repo) RevokeOtherGrants(ctx context.Context, userID, clientID, keepID vo.Id, now time.Time) ([]vo.Id, error) {
	rows, err := r.q.RevokeOtherOAuthGrants(ctx, r.db(ctx), revokeOtherGrantsParam{
		RevokedAt: &now, UserID: userID.String(), ClientID: clientID.String(), ID: keepID.String(),
	})
	if err != nil {
		return nil, err
	}
	return parseIDs(rows)
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
		out = append(out, model.ConnectedGrant{Grant: *g, ClientName: row.Name, RedirectURIs: uris, ClientIsGlobal: row.IsGlobal})
	}
	return out, nil
}

func (r *Repo) ListClientGrants(ctx context.Context, clientID vo.Id) ([]model.OAuthGrant, error) {
	rows, err := r.q.ListOAuthClientGrants(ctx, r.db(ctx), clientID.String())
	if err != nil {
		return nil, err
	}
	return hydrateGrants(rows)
}

func (r *Repo) ListAllUnrevokedGrants(ctx context.Context) ([]model.OAuthGrant, error) {
	rows, err := r.q.ListAllUnrevokedOAuthGrants(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	return hydrateGrants(rows)
}

func (r *Repo) RevokeClientGrants(ctx context.Context, clientID vo.Id, now time.Time) ([]vo.Id, error) {
	rows, err := r.q.RevokeClientOAuthGrants(ctx, r.db(ctx), revokeClientParams{RevokedAt: &now, ClientID: clientID.String()})
	if err != nil {
		return nil, err
	}
	return parseIDs(rows)
}

func (r *Repo) DeleteDeadGrants(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.q.DeleteDeadOAuthGrants(ctx, r.db(ctx), deleteDeadParams{RevokedAt: &cutoff, ExpiresAt: cutoff})
}

func hydrateClient(row clientRow) (*model.OAuthClient, error) {
	cid, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	uris, err := parseURIs(row.RedirectUris)
	if err != nil {
		return nil, err
	}
	c := &model.OAuthClient{ID: cid, Name: row.Name, RedirectURIs: uris, SecretHash: row.SecretHash,
		CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt, IsGlobal: row.IsGlobal}
	if row.OwnerUserID != nil {
		owner, err := vo.ParseId(*row.OwnerUserID)
		if err != nil {
			return nil, err
		}
		c.OwnerUserID = &owner
	}
	return c, nil
}

func idPtrString(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func parseIDs(raw []string) ([]vo.Id, error) {
	ids := make([]vo.Id, 0, len(raw))
	for _, r := range raw {
		id, err := vo.ParseId(r)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func hydrateGrants(rows []grantRow) ([]model.OAuthGrant, error) {
	out := make([]model.OAuthGrant, 0, len(rows))
	for _, row := range rows {
		g, err := hydrateGrant(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, nil
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
