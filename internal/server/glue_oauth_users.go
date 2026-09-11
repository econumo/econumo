// OAuthUsers adapts the user service to the oauth feature's Users port. It
// lives here because features never import each other.
package server

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

type OAuthUsers struct{ users *appuser.Service }

var _ appoauth.Users = (*OAuthUsers)(nil)

func NewOAuthUsers(users *appuser.Service) *OAuthUsers { return &OAuthUsers{users: users} }

func (a *OAuthUsers) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	return a.users.GetByEmail(ctx, email)
}
func (a *OAuthUsers) FindByID(ctx context.Context, id vo.Id) (*model.User, error) {
	return a.users.GetByID(ctx, id)
}
func (a *OAuthUsers) ProvisionExternal(ctx context.Context, name, email string) (*model.User, error) {
	return a.users.ProvisionExternalUser(ctx, name, email)
}
func (a *OAuthUsers) ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string) error {
	return a.users.ReplaceVerifiedEmail(ctx, userID, email)
}
func (a *OAuthUsers) MintSession(ctx context.Context, userID vo.Id, userAgent, provider string, idToken *string) (*model.LoginResult, error) {
	return a.users.CreateExternalSession(ctx, userID, userAgent, provider, idToken)
}
func (a *OAuthUsers) RevokeAllSessions(ctx context.Context, userID vo.Id) error {
	return a.users.RevokeAllSessions(ctx, userID)
}
func (a *OAuthUsers) MarkEmailVerified(ctx context.Context, userID vo.Id) error {
	return a.users.MarkEmailVerified(ctx, userID)
}
