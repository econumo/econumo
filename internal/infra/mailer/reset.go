package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// EmailKeys lists every catalogue key the mailer renders; the i18ntest guard
// asserts each exists in every language.
var EmailKeys = []string{"emails.reset.subject", "emails.reset.body", "emails.verify.subject", "emails.verify.body",
	"emails.change_email.subject", "emails.change_email.body",
	"emails.change_email_notice.subject", "emails.change_email_notice.body"}

// ResetSender builds and sends the password-reset confirmation-code email. It
// satisfies the user service's reset-mailer port (structurally) so the app layer
// stays free of any mail dependency.
type ResetSender struct {
	m       Mailer
	from    string
	replyTo string
}

// NewResetSender wires the reset email sender over a Mailer with the configured
// From / Reply-To addresses (the from / reply_to query params of MAILER_DSN).
func NewResetSender(m Mailer, from, replyTo string) *ResetSender {
	return &ResetSender{m: m, from: from, replyTo: replyTo}
}

// SendResetPasswordCode emails the reset code to the user. "Mail not configured"
// is now expressed by the console transport (the empty-MAILER_DSN default), which
// always renders the message and returns nil — so the remind flow still succeeds
// out of the box, and an empty From no longer silently swallows that output.
func (s *ResetSender) SendResetPasswordCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.reset.subject", nil)
	body := i18n.T(lang, "emails.reset.body", map[string]any{"name": name, "code": code})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}
