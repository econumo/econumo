package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SMTPConfig is the smtp:// / smtps:// half of MAILER_DSN.
type SMTPConfig struct {
	Host        string
	Port        string
	Username    string
	Password    string
	ImplicitTLS bool // smtps://: TLS from the first byte (port 465) instead of STARTTLS
	Insecure    bool // ?insecure=true: plaintext allowed, certificate not verified
}

// smtpTimeout bounds a whole send when the caller's context carries no
// deadline, so a relay that accepts the connection and then stalls cannot
// hang a request forever.
const smtpTimeout = 30 * time.Second

type smtpMailer struct {
	addr        string
	host        string
	username    string
	password    string
	implicitTLS bool
	// requireTLS refuses to send over a connection that never became TLS. It
	// is off only on loopback hosts and with ?insecure=true: a reset code is a
	// credential, and STARTTLS that silently falls back to plaintext hands it
	// to anyone on the path.
	requireTLS bool
	tlsConfig  *tls.Config
	now        func() time.Time
}

func newSMTPMailer(c SMTPConfig) *smtpMailer {
	port := c.Port
	if port == "" {
		port = "587"
		if c.ImplicitTLS {
			port = "465"
		}
	}
	// TLS to a relay on the same host protects nothing, and such relays
	// commonly present a snakeoil certificate.
	relaxed := c.Insecure || isLoopback(c.Host)
	return &smtpMailer{
		addr:        net.JoinHostPort(c.Host, port),
		host:        c.Host,
		username:    c.Username,
		password:    c.Password,
		implicitTLS: c.ImplicitTLS,
		requireTLS:  !relaxed,
		tlsConfig:   &tls.Config{ServerName: c.Host, InsecureSkipVerify: relaxed, MinVersion: tls.VersionTLS12},
		now:         time.Now,
	}
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (m *smtpMailer) Send(ctx context.Context, msg Message) error {
	from, err := mail.ParseAddress(msg.From)
	if err != nil {
		return fmt.Errorf("mailer: smtp from address: %w", err)
	}
	to, err := mail.ParseAddress(msg.To)
	if err != nil {
		return fmt.Errorf("mailer: smtp to address: %w", err)
	}
	cc := make([]*mail.Address, 0, len(msg.Cc))
	for _, c := range msg.Cc {
		a, err := mail.ParseAddress(c)
		if err != nil {
			return fmt.Errorf("mailer: smtp cc address: %w", err)
		}
		cc = append(cc, a)
	}
	var replyTo *mail.Address
	if msg.ReplyTo != "" {
		if replyTo, err = mail.ParseAddress(msg.ReplyTo); err != nil {
			return fmt.Errorf("mailer: smtp reply-to address: %w", err)
		}
	}
	body, err := m.render(from, to, cc, replyTo, msg)
	if err != nil {
		return err
	}
	rcpts := []string{to.Address}
	for _, a := range cc {
		rcpts = append(rcpts, a.Address)
	}
	if err := m.deliver(ctx, from.Address, rcpts, body); err != nil {
		return fmt.Errorf("mailer: smtp send: %w", err)
	}
	return nil
}

func (m *smtpMailer) deliver(ctx context.Context, from string, rcpts []string, body []byte) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, smtpTimeout)
		defer cancel()
	}
	var conn net.Conn
	var err error
	if m.implicitTLS {
		d := &tls.Dialer{Config: m.tlsConfig}
		conn, err = d.DialContext(ctx, "tcp", m.addr)
	} else {
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", m.addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	// Cancellation unblocks a read or write in flight by expiring the deadline.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer c.Close()
	encrypted := m.implicitTLS
	if !encrypted {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(m.tlsConfig); err != nil {
				return err
			}
			encrypted = true
		} else if m.requireTLS {
			return errors.New("server does not offer STARTTLS (use smtps:// for implicit TLS, or ?insecure=true on a trusted network)")
		}
	}
	if m.username != "" {
		auth, err := m.auth(c, encrypted)
		if err != nil {
			return err
		}
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// auth picks PLAIN, or LOGIN for servers that advertise only that (Office
// 365, some Exchange setups). Both send the password as-is, so the TLS
// policy above has already decided whether that is acceptable.
func (m *smtpMailer) auth(c *smtp.Client, encrypted bool) (smtp.Auth, error) {
	ok, params := c.Extension("AUTH")
	if !ok {
		return nil, errors.New("server does not support AUTH but credentials are configured")
	}
	mechs := strings.Fields(strings.ToUpper(params))
	switch {
	case containsMech(mechs, "PLAIN"):
		return plainAuth{username: m.username, password: m.password}, nil
	case containsMech(mechs, "LOGIN"):
		return &loginAuth{username: m.username, password: m.password}, nil
	default:
		return nil, fmt.Errorf("no supported AUTH mechanism (server offers %q, want PLAIN or LOGIN)", params)
	}
}

func containsMech(mechs []string, want string) bool {
	for _, m := range mechs {
		if m == want {
			return true
		}
	}
	return false
}

// plainAuth is smtp.PlainAuth without its own TLS check: that check only
// exempts "localhost", and the transport already enforces its TLS policy
// (including the explicit ?insecure=true opt-in) before authenticating.
type plainAuth struct{ username, password string }

func (a plainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.username + "\x00" + a.password), nil
}

func (a plainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}

type loginAuth struct {
	username, password string
	step               int
}

func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	a.step = 0
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(_ []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	a.step++
	switch a.step {
	case 1:
		return []byte(a.username), nil
	case 2:
		return []byte(a.password), nil
	default:
		return nil, errors.New("unexpected server challenge")
	}
}

func (m *smtpMailer) render(from, to *mail.Address, cc []*mail.Address, replyTo *mail.Address, msg Message) ([]byte, error) {
	var b bytes.Buffer
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", from.String())
	header("To", to.String())
	if len(cc) > 0 {
		list := make([]string, len(cc))
		for i, a := range cc {
			list[i] = a.String()
		}
		header("Cc", strings.Join(list, ", "))
	}
	if replyTo != nil {
		header("Reply-To", replyTo.String())
	}
	// Q-encoding also encodes any CR/LF, so a subject cannot inject headers.
	header("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header("Date", m.now().Format(time.RFC1123Z))
	id, err := messageID(from.Address)
	if err != nil {
		return nil, err
	}
	header("Message-ID", id)
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(msg.Text)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func messageID(from string) (string, error) {
	domain := "localhost"
	if i := strings.LastIndexByte(from, '@'); i >= 0 && i < len(from)-1 {
		domain = from[i+1:]
	}
	var r [16]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	return "<" + hex.EncodeToString(r[:]) + "@" + domain + ">", nil
}
