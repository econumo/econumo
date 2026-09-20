// OAuthNotifier adapts the user service + the identity mailer to the oauth
// feature's Notifier port. It lives here because features never import each
// other.
package server

import (
	"context"

	"github.com/econumo/econumo/internal/infra/mailer"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

type OAuthNotifier struct {
	users *appuser.Service
	mail  *mailer.IdentitySender
}

var _ appoauth.Notifier = (*OAuthNotifier)(nil)

func NewOAuthNotifier(users *appuser.Service, mail *mailer.IdentitySender) *OAuthNotifier {
	return &OAuthNotifier{users: users, mail: mail}
}

// IdentityLinked loads the account owner's plaintext email and stored
// language and sends the notice in that language (falling back to the
// callback request's language when none is stored yet).
func (a *OAuthNotifier) IdentityLinked(ctx context.Context, userID vo.Id, providerName string) error {
	return a.notify(ctx, userID, providerName, a.mail.SendIdentityLinked)
}

// IdentityUnlinked is the same resolution for the removal notice.
func (a *OAuthNotifier) IdentityUnlinked(ctx context.Context, userID vo.Id, providerName string) error {
	return a.notify(ctx, userID, providerName, a.mail.SendIdentityUnlinked)
}

func (a *OAuthNotifier) notify(ctx context.Context, userID vo.Id, providerName string,
	send func(ctx context.Context, to, name, provider, lang string, cc []string) error) error {
	u, email, err := a.users.AdminUserByID(ctx, userID)
	if err != nil {
		return err
	}
	lang, err := a.users.GetLanguage(ctx, userID)
	if err != nil {
		lang = ""
	}
	return send(ctx, email, u.Name, providerName, lang, nil)
}
