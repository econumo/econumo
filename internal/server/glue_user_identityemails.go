package server

import (
	"context"

	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

// oauthIdentityEmails adapts the oauth service to the user feature's
// IdentityEmailLister port (the change-email notice is copied to every address
// the owner's providers vouched for).
type oauthIdentityEmails struct{ oauth *appoauth.Service }

var _ appuser.IdentityEmailLister = oauthIdentityEmails{}

func (a oauthIdentityEmails) ListEmails(ctx context.Context, userID vo.Id) ([]string, error) {
	if a.oauth == nil {
		return nil, nil
	}
	return a.oauth.ListIdentityEmails(ctx, userID)
}

// NewIdentityEmailLister exposes the adapter the same way NewOAuthReclaimer
// does, so the CLI container can wire it if it ever sends the notice.
func NewIdentityEmailLister(svc *appoauth.Service) appuser.IdentityEmailLister {
	return oauthIdentityEmails{oauth: svc}
}
