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
	appURL  string
}

// NewVerifySender wires the verification email sender over a Mailer with the
// configured From / Reply-To addresses (the from / reply_to query params of
// MAILER_DSN).
func NewVerifySender(m Mailer, from, replyTo, appURL string) *VerifySender {
	return &VerifySender{m: m, from: from, replyTo: replyTo, appURL: appURL}
}

// SendVerificationCode emails the verification code to the user in the
// caller's language.
func (s *VerifySender) SendVerificationCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	c := Compose(lang, "verify", nil, name, code, s.appURL)
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo,
		Subject: i18n.T(lang, "emails.verify.subject", nil), Text: RenderText(c)})
}
