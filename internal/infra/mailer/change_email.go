package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// ChangeEmailSender sends the two self-service change-email messages: the code
// to the NEW address, and a heads-up notice to the OLD address.
type ChangeEmailSender struct {
	m       Mailer
	from    string
	replyTo string
	appURL  string
}

func NewChangeEmailSender(m Mailer, from, replyTo, appURL string) *ChangeEmailSender {
	return &ChangeEmailSender{m: m, from: from, replyTo: replyTo, appURL: appURL}
}

// SendEmailChangeCode emails the confirmation code to the proposed NEW address.
func (s *ChangeEmailSender) SendEmailChangeCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	c := Compose(lang, "change_email", nil, name, code, s.appURL)
	msg, err := newMessage(s.from, to, s.replyTo, i18n.T(lang, "emails.change_email.subject", nil), nil, c)
	if err != nil {
		return err
	}
	return s.m.Send(ctx, msg)
}

// SendEmailChangeNotice emails the OLD address that a change was requested,
// naming the proposed new address so an unwanted change is noticeable. cc adds
// the account's linked-provider addresses; the proposed NEW address is
// deliberately not among them — it has proved nothing yet.
func (s *ChangeEmailSender) SendEmailChangeNotice(ctx context.Context, to, name, newEmail string, cc []string) error {
	lang := reqctx.Language(ctx)
	c := Compose(lang, "change_email_notice", map[string]any{"email": newEmail}, name, "", s.appURL)
	msg, err := newMessage(s.from, to, s.replyTo, i18n.T(lang, "emails.change_email_notice.subject", nil), ccAddresses(to, cc), c)
	if err != nil {
		return err
	}
	return s.m.Send(ctx, msg)
}
