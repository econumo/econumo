// Package mailer sends transactional email via an SMTP server described by a
// Symfony-style MAILER_DSN, using only the standard library (net/smtp + crypto
// /tls). It supports the schemes a self-hosted Econumo realistically uses:
//
//	smtp://user:pass@host:port      STARTTLS when the server advertises it (587/25)
//	smtps://user:pass@host:port     implicit TLS (465)
//	null:// or "" (empty)           a no-op mailer (sends nothing)
//
// A no-op mailer is also returned when the configured From address is empty,
// matching the PHP EmailService, which skips sending when no from is set.
package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"
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

// New parses a MAILER_DSN and returns a Mailer. An empty/null/unparseable DSN
// yields a no-op mailer (so a deployment without mail configured simply doesn't
// send, rather than erroring).
func New(dsn string) Mailer {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return noop{}
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return noop{}
	}
	switch strings.ToLower(u.Scheme) {
	case "smtp", "smtps":
		host := u.Hostname()
		if host == "" {
			return noop{}
		}
		port := u.Port()
		implicitTLS := u.Scheme == "smtps"
		if q := u.Query().Get("encryption"); strings.EqualFold(q, "ssl") || strings.EqualFold(q, "tls") {
			// Symfony uses encryption=ssl for implicit TLS; tls historically meant
			// STARTTLS but is commonly used for 465 too — treat ssl as implicit.
			if strings.EqualFold(q, "ssl") {
				implicitTLS = true
			}
		}
		if port == "" {
			if implicitTLS {
				port = "465"
			} else {
				port = "587"
			}
		}
		var user, pass string
		if u.User != nil {
			user = u.User.Username()
			pass, _ = u.User.Password()
		}
		return &smtpMailer{
			addr:        net.JoinHostPort(host, port),
			host:        host,
			user:        user,
			pass:        pass,
			implicitTLS: implicitTLS,
		}
	default: // null:// and anything else -> no-op
		return noop{}
	}
}

// noop is the disabled mailer.
type noop struct{}

func (noop) Send(context.Context, Message) error { return nil }

// smtpMailer sends via net/smtp.
type smtpMailer struct {
	addr        string // host:port
	host        string // for TLS ServerName + auth
	user, pass  string
	implicitTLS bool
}

func (m *smtpMailer) Send(_ context.Context, msg Message) error {
	raw := buildMessage(msg)
	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.pass, m.host)
	}

	if !m.implicitTLS {
		// STARTTLS path: net/smtp.SendMail dials, upgrades via STARTTLS when the
		// server advertises it, authenticates, and sends.
		return smtp.SendMail(m.addr, auth, msg.From, []string{msg.To}, raw)
	}

	// Implicit-TLS path (465): dial a TLS socket first, then speak SMTP.
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", m.addr, &tls.Config{ServerName: m.host})
	if err != nil {
		return fmt.Errorf("mailer: tls dial %s: %w", m.addr, err)
	}
	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("mailer: smtp client: %w", err)
	}
	defer func() { _ = c.Quit() }()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}
	if err := c.Mail(msg.From); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(msg.To); err != nil {
		return fmt.Errorf("mailer: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("mailer: write body: %w", err)
	}
	return w.Close()
}

// buildMessage assembles an RFC 5322 text/plain (UTF-8) message. CRLF line
// endings; the body's bare newlines are normalized to CRLF.
func buildMessage(msg Message) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", msg.From)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	if msg.ReplyTo != "" {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", msg.ReplyTo)
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", msg.Subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(msg.Text, "\n", "\r\n"))
	return []byte(b.String())
}
