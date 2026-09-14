package server

import (
	"context"

	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

// oauthIdentityReclaimer adapts the oauth service to the user feature's
// OAuthReclaimer port (the password-reset cascade drops sign-in methods and
// pending grants that never proved the account's address).
type oauthIdentityReclaimer struct{ oauth *appoauth.Service }

var _ appuser.OAuthReclaimer = oauthIdentityReclaimer{}

func (a oauthIdentityReclaimer) ReclaimAccount(ctx context.Context, userID vo.Id, provenEmail string) (int64, int64, error) {
	return a.oauth.ReclaimAccount(ctx, userID, provenEmail)
}
