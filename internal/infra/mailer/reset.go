package mailer

import (
	"context"
	"fmt"
)

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
// From / Reply-To addresses (the from / reply_to query params of MAILER_DSN).
func NewResetSender(m Mailer, from, replyTo string) *ResetSender {
	return &ResetSender{m: m, from: from, replyTo: replyTo}
}

// SendResetPasswordCode emails the reset code to the user. "Mail not configured"
// is now expressed by the console transport (the empty-MAILER_DSN default), which
// always renders the message and returns nil — so the remind flow still succeeds
// out of the box, and an empty From no longer silently swallows that output.
func (s *ResetSender) SendResetPasswordCode(ctx context.Context, to, name, code string) error {
	body := fmt.Sprintf(
		"Hi %s,\nYour confirmation code is: %s.\n\nIf you didn't request this code, please ignore this email.\n\n--\nEconumo — Manage money. Together.\n",
		name, code,
	)
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: resetSubject, Text: body})
}
