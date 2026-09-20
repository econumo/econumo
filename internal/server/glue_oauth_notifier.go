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

// identityEmailLister is the notifier's half of the oauth service: the linked
// addresses to copy a notice to. An interface rather than *appoauth.Service so
// the glue's own test can drive the lookup-failure path.
type identityEmailLister interface {
	ListIdentityEmails(ctx context.Context, userID vo.Id) ([]string, error)
}

type OAuthNotifier struct {
	users      *appuser.Service
	mail       *mailer.IdentitySender
	identities identityEmailLister
}

var _ appoauth.Notifier = (*OAuthNotifier)(nil)

func NewOAuthNotifier(users *appuser.Service, mail *mailer.IdentitySender, identities identityEmailLister) *OAuthNotifier {
	return &OAuthNotifier{users: users, mail: mail, identities: identities}
}

// IdentityLinked loads the account owner's plaintext email and stored
// language and sends the notice in that language (falling back to the
// callback request's language when none is stored yet), copied to the
// account's other linked-provider addresses.
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
	// Best-effort: the copy list is a nicety, so losing it must not cost the
	// owner the notice itself. For the unlink notice this is the set that
	// REMAINS — the removed identity's row is already gone.
	var cc []string
	if a.identities != nil {
		if addrs, lerr := a.identities.ListIdentityEmails(ctx, userID); lerr == nil {
			cc = addrs
		}
	}
	return send(ctx, email, u.Name, providerName, lang, cc)
}
