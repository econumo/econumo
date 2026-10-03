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

	InsertCode(ctx context.Context, c *model.OAuthAuthorizationCode) error
	ConsumeCode(ctx context.Context, codeHash string) (*model.OAuthAuthorizationCode, error) // DELETE ... RETURNING; errs.NotFound when absent
	PurgeExpiredCodes(ctx context.Context, before time.Time) error

	InsertGrant(ctx context.Context, g *model.OAuthGrant) error
	GetGrant(ctx context.Context, id vo.Id) (*model.OAuthGrant, error)
	GetGrantByRefreshHash(ctx context.Context, hash string) (*model.OAuthGrant, error)
	GetGrantByPrevRefreshHash(ctx context.Context, hash string) (*model.OAuthGrant, error)
	RotateGrant(ctx context.Context, id vo.Id, oldHash, newHash string, now, expiresAt time.Time) (int64, error)
	RevokeGrant(ctx context.Context, id vo.Id, now time.Time) (int64, error)
	RevokeUserGrants(ctx context.Context, userID vo.Id, now time.Time) (int64, error)
	// RevokeOtherGrants revokes the user's unrevoked grants for the client
	// except keepID, returning the ids it revoked.
	RevokeOtherGrants(ctx context.Context, userID, clientID, keepID vo.Id, now time.Time) ([]vo.Id, error)
	ListUnrevokedGrants(ctx context.Context, userID vo.Id) ([]model.ConnectedGrant, error)
	DeleteDeadGrants(ctx context.Context, cutoff time.Time) (int64, error)
}
