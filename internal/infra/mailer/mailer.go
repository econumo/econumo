// Package mailer sends transactional email through the Resend HTTP API
// (https://resend.com), configured by a single API key (RESEND_API_KEY).
//
// Resend is the only supported transport: there is deliberately no SMTP or
// other protocol path. The HTTP API needs no STARTTLS/auth-mechanism handling,
// propagates request context, and works with the addresses verified on the
// Resend account.
//
// When the API key is empty the mailer is a no-op (sends nothing), so a
// deployment without mail configured simply doesn't send rather than erroring.
// A no-op is likewise used by ResetSender when the From address is empty,
// matching the PHP EmailService, which skips sending when no from is set.
package mailer

import (
	"context"
	"fmt"
	"strings"

	"github.com/resend/resend-go/v3"
)

// Message is one email to send.
type Message struct {
	From    string
	To      string
	ReplyTo string
	Subject string
	Text    string
}

// Mailer sends a Message.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// New returns a Mailer backed by the Resend API. An empty API key yields a
// no-op mailer, so a deployment without mail configured simply doesn't send.
func New(apiKey string) Mailer {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return noop{}
	}
	return &resendMailer{client: resend.NewClient(apiKey)}
}

// noop is the disabled mailer.
type noop struct{}

func (noop) Send(context.Context, Message) error { return nil }

// resendMailer sends via the Resend API.
type resendMailer struct {
	client *resend.Client
}

func (m *resendMailer) Send(ctx context.Context, msg Message) error {
	params := &resend.SendEmailRequest{
		From:    msg.From,
		To:      []string{msg.To},
		Subject: msg.Subject,
		Text:    msg.Text,
	}
	if msg.ReplyTo != "" {
		params.ReplyTo = msg.ReplyTo
	}
	if _, err := m.client.Emails.SendWithContext(ctx, params); err != nil {
		return fmt.Errorf("mailer: resend send: %w", err)
	}
	return nil
}
