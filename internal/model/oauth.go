package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	OAuthProviderGoogle = "google"
	OAuthProviderApple  = "apple"
	OAuthProviderOIDC   = "oidc"

	OAuthClientWeb = "web"
	OAuthClientApp = "app"

	OAuthIntentLogin = "login"
	OAuthIntentLink  = "link"

	OAuthStateTTL   = 10 * time.Minute
	OAuthHandoffTTL = 60 * time.Second
)

// OAuthProviders is the fixed display order of the provider slots.
var OAuthProviders = []string{OAuthProviderGoogle, OAuthProviderApple, OAuthProviderOIDC}

func IsOAuthProvider(id string) bool {
	for _, p := range OAuthProviders {
		if p == id {
			return true
		}
	}
	return false
}

// Identity binds an external subject to a user. (provider, subject) is the
// key; email is the last value seen, display only.
type Identity struct {
	ID        vo.Id
	UserID    vo.Id
	Provider  string
	Subject   string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewIdentity(id, userID vo.Id, provider, subject, email string, now time.Time) *Identity {
	return &Identity{ID: id, UserID: userID, Provider: provider, Subject: subject, Email: email, CreatedAt: now, UpdatedAt: now}
}

func (i *Identity) UpdateEmail(email string, now time.Time) {
	if i.Email == email {
		return
	}
	i.Email = email
	i.UpdatedAt = now
}

// OAuthState is one in-flight authorization request, keyed by the sha256 of the
// state parameter; consumed (deleted) on the first callback that presents it.
type OAuthState struct {
	StateHash    string
	Provider     string
	Nonce        string
	CodeVerifier string
	// FlowHash binds the flow to the client that started it: sha256 of the
	// secret start-login/start-link returned. The callback carries no client
	// credential, so without it a forged callback could hand a victim's browser
	// a handoff minted for the attacker's account.
	FlowHash   string
	Client     string
	Intent     string
	LinkUserID vo.Id // zero unless Intent == OAuthIntentLink
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

func (s *OAuthState) IsExpired(now time.Time) bool { return !now.Before(s.ExpiresAt) }

// OAuthHandoff is the one-shot code the client exchanges for a session.
type OAuthHandoff struct {
	CodeHash string
	UserID   vo.Id
	Provider string
	// FlowHash is copied from the state row; only the client that started the
	// flow can present the matching secret at exchange time.
	FlowHash  string
	IDToken   *string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (h *OAuthHandoff) IsExpired(now time.Time) bool { return !now.Before(h.ExpiresAt) }
