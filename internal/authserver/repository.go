// Package authserver is the OAuth 2.1 authorization server that lets MCP clients obtain tokens for /mcp.
package authserver

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Repository interface {
	InsertClient(ctx context.Context, c *model.OAuthClient) error
	GetClient(ctx context.Context, id vo.Id) (*model.OAuthClient, error) // errs.NotFound when absent
	MarkClientUsed(ctx context.Context, id vo.Id, now time.Time) error
	PurgeUnusedClients(ctx context.Context, createdBefore time.Time) (int64, error)
	// PurgeExcessUnusedClients keeps only the newest never-approved clients.
	PurgeExcessUnusedClients(ctx context.Context, keep int64) (int64, error)
	// ClaimClient sets the owner of a user app that has none yet; 0 rows for an owned or official app.
	ClaimClient(ctx context.Context, id, userID vo.Id) (int64, error)
	ListClients(ctx context.Context) ([]model.OAuthClient, error)
	UpdateGlobalClient(ctx context.Context, id vo.Id, name string, redirectURIs []string) (int64, error)
	MakeClientGlobal(ctx context.Context, id vo.Id) (int64, error)
	// SetClientSecret replaces the secret of a confidential client only; a public client matches nothing.
	SetClientSecret(ctx context.Context, id vo.Id, hash string) (int64, error)
	// LockClient takes a row lock on the client (errs.NotFound when absent).
	LockClient(ctx context.Context, id vo.Id) error
	// ShareLockClient takes a lock that conflicts only with LockClient (errs.NotFound when absent).
	ShareLockClient(ctx context.Context, id vo.Id) error
	DeleteClient(ctx context.Context, id vo.Id) (int64, error)

	InsertCode(ctx context.Context, c *model.OAuthAuthorizationCode) error
	ConsumeCode(ctx context.Context, codeHash string) (*model.OAuthAuthorizationCode, error) // DELETE ... RETURNING; errs.NotFound when absent
	GetCode(ctx context.Context, codeHash string) (*model.OAuthAuthorizationCode, error)     // errs.NotFound when absent
	PurgeExpiredCodes(ctx context.Context, before time.Time) error
	// DeleteClientCodes drops the client's outstanding codes, only userID's when it is set.
	DeleteClientCodes(ctx context.Context, clientID vo.Id, userID *vo.Id) (int64, error)

	InsertGrant(ctx context.Context, g *model.OAuthGrant) error
	GetGrant(ctx context.Context, id vo.Id) (*model.OAuthGrant, error)
	GetGrantByRefreshHash(ctx context.Context, hash string) (*model.OAuthGrant, error)
	// GetGrantBySpentRefreshHash finds the grant that rotated the token away,
	// however many rotations ago (errs.NotFound when never spent).
	GetGrantBySpentRefreshHash(ctx context.Context, hash string) (*model.OAuthGrant, error)
	InsertSpentRefreshHash(ctx context.Context, grantID vo.Id, hash string, now time.Time) error
	RotateGrant(ctx context.Context, id vo.Id, oldHash, newHash string, now, expiresAt time.Time) (int64, error)
	RevokeGrant(ctx context.Context, id vo.Id, now time.Time) (int64, error)
	RevokeUserGrants(ctx context.Context, userID vo.Id, now time.Time) (int64, error)
	// RevokeOtherGrants revokes the user's unrevoked grants for the client
	// except keepID, returning the ids it revoked.
	RevokeOtherGrants(ctx context.Context, userID, clientID, keepID vo.Id, now time.Time) ([]vo.Id, error)
	ListUnrevokedGrants(ctx context.Context, userID vo.Id) ([]model.ConnectedGrant, error)
	ListClientGrants(ctx context.Context, clientID vo.Id) ([]model.OAuthGrant, error)
	ListAllUnrevokedGrants(ctx context.Context) ([]model.OAuthGrant, error)
	// RevokeClientGrants revokes every unrevoked grant of the client, returning the ids it revoked.
	RevokeClientGrants(ctx context.Context, clientID vo.Id, now time.Time) ([]vo.Id, error)
	DeleteDeadGrants(ctx context.Context, cutoff time.Time) (int64, error)
}
