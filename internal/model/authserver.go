package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

type OAuthClient struct {
	ID           vo.Id
	Name         string
	RedirectURIs []string
	SecretHash   *string
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

type OAuthAuthorizationCode struct {
	CodeHash              string
	ClientID, UserID      vo.Id
	RedirectURI           string
	CodeChallenge         string
	Resource              string
	CredentialsGeneration int64
	CreatedAt, ExpiresAt  time.Time
}

// OAuthGrant is one user approval of one client; it holds the rotating refresh
// token. PrevRefreshTokenHash/RotatedAt remember the token rotated away from so
// a duplicate refresh inside the grace window is told apart from a replay.
type OAuthGrant struct {
	ID, UserID, ClientID  vo.Id
	RefreshTokenHash      string
	PrevRefreshTokenHash  *string
	RotatedAt             *time.Time
	CreatedAt, LastUsedAt time.Time
	ExpiresAt             time.Time
	RevokedAt             *time.Time
}

func (g *OAuthGrant) IsLive(now time.Time) bool { return g.RevokedAt == nil && g.ExpiresAt.After(now) }

// ConnectedGrant is a grant joined with its client, for the Settings list.
type ConnectedGrant struct {
	Grant        OAuthGrant
	ClientName   string
	RedirectURIs []string
}
