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
}

func NewChangeEmailSender(m Mailer, from, replyTo string) *ChangeEmailSender {
	return &ChangeEmailSender{m: m, from: from, replyTo: replyTo}
}

// SendEmailChangeCode emails the confirmation code to the proposed NEW address.
func (s *ChangeEmailSender) SendEmailChangeCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.change_email.subject", nil)
	body := i18n.T(lang, "emails.change_email.body", map[string]any{"name": name, "code": code})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}

// SendEmailChangeNotice emails the OLD address that a change was requested,
// naming the proposed new address so an unwanted change is noticeable.
func (s *ChangeEmailSender) SendEmailChangeNotice(ctx context.Context, to, name, newEmail string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.change_email_notice.subject", nil)
	body := i18n.T(lang, "emails.change_email_notice.body", map[string]any{"name": name, "email": newEmail})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}
