// Ports: the user-feature capabilities this feature consumes. Implemented in
// internal/server over the user service — features never import each other.
package oauth

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Users interface {
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id vo.Id) (*model.User, error)
	// ProvisionExternal creates a passwordless, email-verified user with the
	// registration defaults (trial, options, currency).
	ProvisionExternal(ctx context.Context, name, email string) (*model.User, error)
	// ReplaceVerifiedEmail mirrors an IdP-side email change onto the primary email.
	ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error
	// MintSession opens a session stamped with the provider (and the ID token
	// for the custom slot) and returns the login-shaped result.
	MintSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error)
}
