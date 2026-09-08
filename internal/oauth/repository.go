// Package oauth is the OAuth/OIDC login feature: linking external identities
// to users and mediating the authorization-code exchange (states, handoffs).
package oauth

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Identities persists users_identities. Lookups on a missing row return
// *errs.NotFoundError.
type Identities interface {
	NextIdentity() vo.Id
	GetByProviderSubject(ctx context.Context, provider, subject string) (*model.Identity, error)
	GetByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.Identity, error)
	ListByUser(ctx context.Context, userID vo.Id) ([]model.Identity, error)
	CountByUser(ctx context.Context, userID vo.Id) (int64, error)
	Save(ctx context.Context, i *model.Identity) error
	DeleteByUserProvider(ctx context.Context, userID vo.Id, provider string) (int64, error)
}

// States persists in-flight authorization requests (oauth_states).
type States interface {
	Insert(ctx context.Context, s *model.OAuthState) error
	Get(ctx context.Context, stateHash string) (*model.OAuthState, error)
	// Delete returns the number of rows removed, so a caller can tell a real
	// delete from a concurrent replay that found the row already gone.
	Delete(ctx context.Context, stateHash string) (int64, error)
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}

// Handoffs persists the one-shot codes exchanged for a session (oauth_handoffs).
type Handoffs interface {
	Insert(ctx context.Context, h *model.OAuthHandoff) error
	Get(ctx context.Context, codeHash string) (*model.OAuthHandoff, error)
	// Delete returns the number of rows removed, so a caller can tell a real
	// delete from a concurrent replay that found the row already gone.
	Delete(ctx context.Context, codeHash string) (int64, error)
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}
