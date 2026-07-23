// Package mailer sends transactional email through one of two transports,
// selected by the MAILER_DSN scheme (parsed in internal/config):
//
//	(empty)              -> console: render the message to stdout
//	console:// | log://  -> console: render the message to stdout
//	resend://<api_key>   -> Resend HTTP API (https://resend.com)
//
// The console transport is the default so a deployment without mail configured
// still surfaces what it would have sent (e.g. a password-reset code during
// local development) instead of silently dropping it. The Resend HTTP API needs
// no STARTTLS/auth-mechanism handling, propagates request context, and works
// with the addresses verified on the Resend account; there is deliberately no
// SMTP or other protocol path.
package mailer

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/resend/resend-go/v3"
)

type Message struct {
	From    string
	To      string
	ReplyTo string
	Subject string
	Text    string
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// New returns the Mailer for the given provider (as derived from MAILER_DSN by
// internal/config). "resend" yields a Resend-backed mailer using apiKey; every
// other value (notably "console" and the empty default) yields the console
// mailer, which renders each message to stdout.
func New(provider, apiKey string) Mailer {
	switch provider {
	case "resend":
		return &resendMailer{client: resend.NewClient(apiKey)}
	default:
		return console{out: os.Stdout}
	}
}

// WithAppLink wraps a Mailer so every message body ends with the instance's
// public URL (ECONUMO_URL) on the line directly below the body. Applied once at composition time,
// it covers every current and future email without per-sender plumbing; when
// the URL is unset the wrapper is not installed, so bodies are unchanged.
func WithAppLink(inner Mailer, appURL string) Mailer {
	return linkMailer{inner: inner, appURL: appURL}
}

type linkMailer struct {
	inner  Mailer
	appURL string
}

func (m linkMailer) Send(ctx context.Context, msg Message) error {
	msg.Text = strings.TrimRight(msg.Text, "\n") + "\n" + m.appURL
	return m.inner.Send(ctx, msg)
}

// console renders each message to out (stdout in production) instead of sending
// it — the default transport, and a readable dev aid. It writes plaintext (To,
// Subject, body and friends) directly rather than through the structured logger,
// which deliberately keeps such PII out of its fields.
type console struct {
	out io.Writer
}

func (c console) Send(_ context.Context, msg Message) error {
	_, err := fmt.Fprintf(c.out,
		"--- email (console transport) ---\nFrom: %s\nTo: %s\nReply-To: %s\nSubject: %s\n\n%s\n---------------------------------\n",
		msg.From, msg.To, msg.ReplyTo, msg.Subject, msg.Text,
	)
	return err
}

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
