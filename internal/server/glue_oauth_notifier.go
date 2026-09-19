// OAuthNotifier adapts the user service + the identity-linked mailer to the
// oauth feature's Notifier port. It lives here because features never import
// each other.
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
	mail  *mailer.IdentityLinkedSender
}

var _ appoauth.Notifier = (*OAuthNotifier)(nil)

func NewOAuthNotifier(users *appuser.Service, mail *mailer.IdentityLinkedSender) *OAuthNotifier {
	return &OAuthNotifier{users: users, mail: mail}
}

// IdentityLinked loads the account owner's plaintext email and stored
// language and sends the notice in that language (falling back to the
// callback request's language when none is stored yet).
func (a *OAuthNotifier) IdentityLinked(ctx context.Context, userID vo.Id, providerName string) error {
	u, email, err := a.users.AdminUserByID(ctx, userID)
	if err != nil {
		return err
	}
	lang, err := a.users.GetLanguage(ctx, userID)
	if err != nil {
		lang = ""
	}
	return a.mail.SendIdentityLinked(ctx, email, u.Name, providerName, lang)
}
