package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// VerifySender builds and sends the login email-verification code email,
// mirroring ResetSender so the app layer stays free of any mail dependency.
type VerifySender struct {
	m       Mailer
	from    string
	replyTo string
}

// NewVerifySender wires the verification email sender over a Mailer with the
// configured From / Reply-To addresses (the from / reply_to query params of
// MAILER_DSN).
func NewVerifySender(m Mailer, from, replyTo string) *VerifySender {
	return &VerifySender{m: m, from: from, replyTo: replyTo}
}

// SendVerificationCode emails the verification code to the user in the
// caller's language.
func (s *VerifySender) SendVerificationCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.verify.subject", nil)
	body := i18n.T(lang, "emails.verify.body", map[string]any{"name": name, "code": code})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}
