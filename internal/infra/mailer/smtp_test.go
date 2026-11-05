package mailer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a minimal ESMTP server: EHLO, STARTTLS, AUTH PLAIN/LOGIN, MAIL,
// RCPT, DATA, QUIT. It records the session for assertions.
type fakeSMTP struct {
	ln          net.Listener
	tlsConfig   *tls.Config
	starttls    bool   // advertise STARTTLS
	authMechs   string // advertised AUTH mechanisms ("" = no AUTH)
	stall       bool   // accept and never greet
	mu          sync.Mutex
	from        string
	rcpts       []string
	data        string
	authUser    string
	authPass    string
	sawTLS      bool
	sessionDone chan struct{}
}

func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mail.x.test"},
		DNSNames:     []string{"mail.x.test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func startFakeSMTP(t *testing.T, implicitTLS bool, configure func(*fakeSMTP)) (*fakeSMTP, *x509.CertPool) {
	t.Helper()
	cert, pool := testCert(t)
	s := &fakeSMTP{
		tlsConfig:   &tls.Config{Certificates: []tls.Certificate{cert}},
		starttls:    true,
		authMechs:   "PLAIN LOGIN",
		sessionDone: make(chan struct{}),
	}
	if configure != nil {
		configure(s)
	}
	var err error
	if implicitTLS {
		s.ln, err = tls.Listen("tcp", "127.0.0.1:0", s.tlsConfig)
	} else {
		s.ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.ln.Close() })
	go s.serve(implicitTLS)
	return s, pool
}

func (s *fakeSMTP) serve(implicitTLS bool) {
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer close(s.sessionDone)
	defer conn.Close()
	if s.stall {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	s.mu.Lock()
	s.sawTLS = implicitTLS
	s.mu.Unlock()
	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			lines := []string{"250-fake"}
			s.mu.Lock()
			encrypted := s.sawTLS
			s.mu.Unlock()
			if s.starttls && !encrypted {
				lines = append(lines, "250-STARTTLS")
			}
			if s.authMechs != "" {
				lines = append(lines, "250-AUTH "+s.authMechs)
			}
			lines = append(lines, "250 8BITMIME")
			for _, l := range lines {
				_ = tp.PrintfLine("%s", l)
			}
		case "STARTTLS":
			_ = tp.PrintfLine("220 go ahead")
			tc := tls.Server(conn, s.tlsConfig)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			tp = textproto.NewConn(tc)
			s.mu.Lock()
			s.sawTLS = true
			s.mu.Unlock()
		case "AUTH":
			mech, initial, _ := strings.Cut(arg, " ")
			switch mech {
			case "PLAIN":
				raw, _ := base64.StdEncoding.DecodeString(initial)
				parts := strings.Split(string(raw), "\x00")
				s.mu.Lock()
				s.authUser, s.authPass = parts[1], parts[2]
				s.mu.Unlock()
			case "LOGIN":
				_ = tp.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
				u, _ := tp.ReadLine()
				_ = tp.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
				p, _ := tp.ReadLine()
				user, _ := base64.StdEncoding.DecodeString(u)
				pass, _ := base64.StdEncoding.DecodeString(p)
				s.mu.Lock()
				s.authUser, s.authPass = string(user), string(pass)
				s.mu.Unlock()
			}
			_ = tp.PrintfLine("235 ok")
		case "MAIL":
			s.mu.Lock()
			s.from, _, _ = strings.Cut(strings.TrimPrefix(arg, "FROM:<"), ">")
			s.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case "RCPT":
			s.mu.Lock()
			s.rcpts = append(s.rcpts, strings.TrimSuffix(strings.TrimPrefix(arg, "TO:<"), ">"))
			s.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case "DATA":
			_ = tp.PrintfLine("354 go")
			b, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = string(b)
			s.mu.Unlock()
			_ = tp.PrintfLine("250 queued")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
	}
}

func (s *fakeSMTP) port() string {
	_, p, _ := net.SplitHostPort(s.ln.Addr().String())
	return p
}

// strictMailer is the policy for a remote relay (TLS required and verified)
// pointed at the loopback fake, which would otherwise get the relaxed policy.
func strictMailer(s *fakeSMTP, pool *x509.CertPool, implicitTLS bool, user, pass string) *smtpMailer {
	m := newSMTPMailer(SMTPConfig{Host: "127.0.0.1", Port: s.port(), Username: user, Password: pass, ImplicitTLS: implicitTLS})
	m.requireTLS = true
	m.tlsConfig = &tls.Config{ServerName: "mail.x.test", RootCAs: pool}
	return m
}

func testMessage() Message {
	return Message{
		From:    "Econumo <noreply@x.test>",
		To:      "user@x.test",
		Cc:      []string{"alt@y.test"},
		ReplyTo: "support@x.test",
		Subject: "Код подтверждения",
		Text:    "Hello Алиса,\nyour code is 123456.\n.leading dot line",
	}
}

func TestSMTP_StartTLSDelivers(t *testing.T) {
	s, pool := startFakeSMTP(t, false, nil)
	m := strictMailer(s, pool, false, "user@x.test", "p@ss")
	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	<-s.sessionDone
	if !s.sawTLS {
		t.Error("the session never upgraded to TLS")
	}
	if s.authUser != "user@x.test" || s.authPass != "p@ss" {
		t.Errorf("auth = %q/%q", s.authUser, s.authPass)
	}
	if s.from != "noreply@x.test" {
		t.Errorf("MAIL FROM = %q", s.from)
	}
	if strings.Join(s.rcpts, ",") != "user@x.test,alt@y.test" {
		t.Errorf("RCPT TO = %v", s.rcpts)
	}

	parsed, err := mail.ReadMessage(strings.NewReader(s.data))
	if err != nil {
		t.Fatalf("parse delivered message: %v\n%s", err, s.data)
	}
	h := parsed.Header
	subj, err := new(mime.WordDecoder).DecodeHeader(h.Get("Subject"))
	if err != nil || subj != "Код подтверждения" {
		t.Errorf("Subject = %q (%v), raw %q", subj, err, h.Get("Subject"))
	}
	if from, _ := h.AddressList("From"); len(from) != 1 || from[0].Name != "Econumo" || from[0].Address != "noreply@x.test" {
		t.Errorf("From = %v", from)
	}
	if h.Get("To") != "<user@x.test>" || h.Get("Cc") != "<alt@y.test>" || h.Get("Reply-To") != "<support@x.test>" {
		t.Errorf("To/Cc/Reply-To = %q / %q / %q", h.Get("To"), h.Get("Cc"), h.Get("Reply-To"))
	}
	if !strings.HasSuffix(h.Get("Message-ID"), "@x.test>") || h.Get("Date") == "" {
		t.Errorf("Message-ID = %q, Date = %q", h.Get("Message-ID"), h.Get("Date"))
	}
	body, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSuffix(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n"), "Hello Алиса,\nyour code is 123456.\n.leading dot line"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestSMTP_ImplicitTLSDelivers(t *testing.T) {
	s, pool := startFakeSMTP(t, true, func(s *fakeSMTP) { s.starttls = false })
	m := strictMailer(s, pool, true, "u", "p")
	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	<-s.sessionDone
	if s.authUser != "u" || s.data == "" {
		t.Errorf("auth user %q, data %d bytes", s.authUser, len(s.data))
	}
}

func TestSMTP_LoginAuthFallback(t *testing.T) {
	s, pool := startFakeSMTP(t, false, func(s *fakeSMTP) { s.authMechs = "LOGIN" })
	m := strictMailer(s, pool, false, "u@x.test", "secret")
	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	<-s.sessionDone
	if s.authUser != "u@x.test" || s.authPass != "secret" {
		t.Errorf("auth = %q/%q", s.authUser, s.authPass)
	}
}

func TestSMTP_RefusesPlaintextWhenTLSRequired(t *testing.T) {
	s, pool := startFakeSMTP(t, false, func(s *fakeSMTP) { s.starttls = false })
	m := strictMailer(s, pool, false, "u", "secret")
	err := m.Send(context.Background(), testMessage())
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err = %v, want a STARTTLS refusal", err)
	}
	<-s.sessionDone
	if s.authPass != "" || s.data != "" {
		t.Error("credentials or message went out over plaintext")
	}
}

func TestSMTP_RejectsUntrustedCertificate(t *testing.T) {
	s, _ := startFakeSMTP(t, false, nil)
	m := strictMailer(s, x509.NewCertPool(), false, "u", "secret")
	if err := m.Send(context.Background(), testMessage()); err == nil {
		t.Fatal("want a certificate verification error")
	}
}

func TestSMTP_LoopbackAllowsPlaintext(t *testing.T) {
	s, _ := startFakeSMTP(t, false, func(s *fakeSMTP) { s.starttls = false; s.authMechs = "" })
	m := newSMTPMailer(SMTPConfig{Host: "127.0.0.1", Port: s.port()})
	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	<-s.sessionDone
	if s.sawTLS || s.data == "" {
		t.Errorf("sawTLS=%v data=%d bytes", s.sawTLS, len(s.data))
	}
}

func TestSMTP_LoopbackUpgradesWithoutVerifying(t *testing.T) {
	// The fake's certificate is self-signed; a loopback relay's snakeoil
	// certificate must not block delivery.
	s, _ := startFakeSMTP(t, false, nil)
	m := newSMTPMailer(SMTPConfig{Host: "127.0.0.1", Port: s.port(), Username: "u", Password: "p"})
	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	<-s.sessionDone
	if !s.sawTLS {
		t.Error("an offered STARTTLS should still be used")
	}
}

func TestSMTP_InsecurePolicy(t *testing.T) {
	m := newSMTPMailer(SMTPConfig{Host: "postfix", Insecure: true})
	if m.requireTLS || !m.tlsConfig.InsecureSkipVerify {
		t.Error("insecure should allow plaintext and skip verification")
	}
	m = newSMTPMailer(SMTPConfig{Host: "mail.x.test"})
	if !m.requireTLS || m.tlsConfig.InsecureSkipVerify || m.addr != "mail.x.test:587" {
		t.Errorf("remote default = requireTLS %v, skipVerify %v, addr %q", m.requireTLS, m.tlsConfig.InsecureSkipVerify, m.addr)
	}
	if m = newSMTPMailer(SMTPConfig{Host: "mail.x.test", ImplicitTLS: true}); m.addr != "mail.x.test:465" {
		t.Errorf("smtps default addr = %q", m.addr)
	}
}

func TestSMTP_ContextCancelStopsAStalledRelay(t *testing.T) {
	s, _ := startFakeSMTP(t, false, func(s *fakeSMTP) { s.stall = true })
	m := newSMTPMailer(SMTPConfig{Host: "127.0.0.1", Port: s.port()})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := m.Send(ctx, testMessage()); err == nil {
		t.Fatal("want an error from a stalled relay")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("send took %v after the context expired", time.Since(start))
	}
}

func TestSMTP_RejectsBadAddresses(t *testing.T) {
	m := newSMTPMailer(SMTPConfig{Host: "127.0.0.1", Port: "1"})
	msg := testMessage()
	msg.To = "not an address\r\nBcc: evil@x.test"
	if err := m.Send(context.Background(), msg); err == nil {
		t.Fatal("want an address error")
	}
}
