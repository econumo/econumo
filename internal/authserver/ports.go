package authserver

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Credentials is the slice of the user feature the authorization server
// needs; internal/server wires *user.Service in.
type Credentials interface {
	LockForOAuth(ctx context.Context, userID vo.Id) (int64, error)
	CredentialsGeneration(ctx context.Context, userID vo.Id) (int64, error)
	IssueOAuthAccessToken(ctx context.Context, userID, grantID vo.Id, name string, generation int64, ttl time.Duration) (string, bool, error)
	RevokeOAuthGrantTokens(ctx context.Context, grantID vo.Id) error
}

type Limiter interface{ Allow(scope, key string) error }

const (
	RateScopeRegister = "oauth-register"
)
