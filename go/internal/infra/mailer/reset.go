package mailer

import (
	"context"
	"fmt"
)

// resetSubject mirrors translations/messages.en.yaml
// email.reset_password_confirmation_code.subject.
const resetSubject = "Reset password confirmation code"

// ResetSender builds and sends the password-reset confirmation-code email. It
// satisfies the user service's reset-mailer port (structurally) so the app layer
// stays free of any mail dependency.
type ResetSender struct {
	m       Mailer
	from    string
	replyTo string
}

// NewResetSender wires the reset email sender over a Mailer with the configured
// From / Reply-To addresses (ECONUMO_FROM_EMAIL / ECONUMO_REPLY_TO_EMAIL).
func NewResetSender(m Mailer, from, replyTo string) *ResetSender {
	return &ResetSender{m: m, from: from, replyTo: replyTo}
}

// SendResetPasswordCode emails the reset code to the user. With no From address
// configured it is a no-op (matching the PHP EmailService), so the remind flow
// still succeeds on a deployment without mail set up.
func (s *ResetSender) SendResetPasswordCode(ctx context.Context, to, name, code string) error {
	if s.from == "" {
		return nil
	}
	body := fmt.Sprintf(
		"Hi %s,\nYour confirmation code is: %s.\n\nIf you didn't request this code, please ignore this email.\n\n--\nEconumo — Manage money. Together.\n",
		name, code,
	)
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: resetSubject, Text: body})
}
