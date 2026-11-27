package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// EmailKeys lists every catalogue key the mailer renders; the i18ntest guard
// asserts each exists in every language.
var EmailKeys = func() []string {
	keys := []string{"emails.common.greeting", "emails.common.greeting_anonymous", "emails.common.code_label",
		"emails.common.code_text", "emails.common.tagline", "emails.common.open_app"}
	for _, e := range []string{"reset", "verify", "change_email", "change_email_notice", "identity_linked", "identity_unlinked"} {
		for _, f := range []string{"subject", "heading", "body", "note"} {
			keys = append(keys, "emails."+e+"."+f)
		}
	}
	return keys
}()

// ResetSender builds and sends the password-reset confirmation-code email. It
// satisfies the user service's reset-mailer port (structurally) so the app layer
// stays free of any mail dependency.
type ResetSender struct {
	m       Mailer
	from    string
	replyTo string
	appURL  string
}

// NewResetSender wires the reset email sender over a Mailer with the configured
// From / Reply-To addresses (the from / reply_to query params of MAILER_DSN).
func NewResetSender(m Mailer, from, replyTo, appURL string) *ResetSender {
	return &ResetSender{m: m, from: from, replyTo: replyTo, appURL: appURL}
}

// SendResetPasswordCode emails the reset code to the user. "Mail not configured"
// is now expressed by the console transport (the empty-MAILER_DSN default), which
// always renders the message and returns nil — so the remind flow still succeeds
// out of the box, and an empty From no longer silently swallows that output.
func (s *ResetSender) SendResetPasswordCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	c := Compose(lang, "reset", nil, name, code, s.appURL)
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo,
		Subject: i18n.T(lang, "emails.reset.subject", nil), Text: RenderText(c)})
}
