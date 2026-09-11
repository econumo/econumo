package server

import (
	"context"

	appoauth "github.com/econumo/econumo/internal/oauth"
	appuser "github.com/econumo/econumo/internal/user"
)

// oauthLogoutURLs adapts the oauth service to the user feature's
// LogoutURLBuilder port (RP-initiated logout for the custom slot).
type oauthLogoutURLs struct{ oauth *appoauth.Service }

var _ appuser.LogoutURLBuilder = oauthLogoutURLs{}

func (a oauthLogoutURLs) EndSessionURL(ctx context.Context, provider, idToken string) (string, error) {
	return a.oauth.EndSessionURL(ctx, provider, idToken)
}
