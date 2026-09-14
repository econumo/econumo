package server

import (
	"context"

	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

// oauthIdentityReclaimer adapts the oauth service to the user feature's
// IdentityReclaimer port (the password-reset cascade unlinks sign-in methods
// that never proved the account's address).
type oauthIdentityReclaimer struct{ oauth *appoauth.Service }

var _ appuser.IdentityReclaimer = oauthIdentityReclaimer{}

func (a oauthIdentityReclaimer) UnlinkForeignIdentities(ctx context.Context, userID vo.Id, provenEmail string) (int64, error) {
	return a.oauth.UnlinkForeignIdentities(ctx, userID, provenEmail)
}
