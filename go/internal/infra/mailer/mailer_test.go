package mailer

import (
	"context"
	"strings"
	"testing"
)

func TestNew_DSNParsing(t *testing.T) {
	if _, ok := New("").(noop); !ok {
		t.Error("empty DSN should give a no-op mailer")
	}
	if _, ok := New("null://null").(noop); !ok {
		t.Error("null:// should give a no-op mailer")
	}

	m, ok := New("smtp://user:pass@mail.example.com:587").(*smtpMailer)
	if !ok {
		t.Fatal("smtp:// should give an smtpMailer")
	}
	if m.addr != "mail.example.com:587" || m.host != "mail.example.com" || m.user != "user" || m.pass != "pass" || m.implicitTLS {
		t.Errorf("smtp parse = %+v", m)
	}

	s, ok := New("smtps://user:pass@mail.example.com:465").(*smtpMailer)
	if !ok || !s.implicitTLS {
		t.Errorf("smtps should set implicitTLS: %+v", s)
	}

	// Default ports by scheme.
	if d := New("smtp://h").(*smtpMailer); d.addr != "h:587" {
		t.Errorf("smtp default port = %q, want h:587", d.addr)
	}
	if d := New("smtps://h").(*smtpMailer); d.addr != "h:465" {
		t.Errorf("smtps default port = %q, want h:465", d.addr)
	}
	// encryption=ssl forces implicit TLS.
	if d := New("smtp://h:465?encryption=ssl").(*smtpMailer); !d.implicitTLS {
		t.Error("encryption=ssl should force implicit TLS")
	}
	// No userinfo -> no auth.
	if d := New("smtp://relay:25").(*smtpMailer); d.user != "" {
		t.Errorf("expected no auth, got user %q", d.user)
	}
}

func TestBuildMessage(t *testing.T) {
	raw := string(buildMessage(Message{From: "f@x", To: "t@x", ReplyTo: "r@x", Subject: "Subj", Text: "line1\nline2"}))
	for _, want := range []string{
		"From: f@x\r\n", "To: t@x\r\n", "Reply-To: r@x\r\n", "Subject: Subj\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n", "\r\n\r\nline1\r\nline2",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("message missing %q\n---\n%s", want, raw)
		}
	}
	// No Reply-To header when unset.
	if strings.Contains(string(buildMessage(Message{From: "f", To: "t"})), "Reply-To:") {
		t.Error("Reply-To header should be omitted when empty")
	}
}

// captureMailer records the last Message instead of sending it.
type captureMailer struct {
	msg    Message
	called bool
}

func (c *captureMailer) Send(_ context.Context, m Message) error {
	c.msg, c.called = m, true
	return nil
}

func TestResetSender(t *testing.T) {
	c := &captureMailer{}
	s := NewResetSender(c, "from@econumo.test", "reply@econumo.test")
	if err := s.SendResetPasswordCode(context.Background(), "user@x.test", "Alice", "abc123def456"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !c.called {
		t.Fatal("expected the mailer to be called")
	}
	if c.msg.From != "from@econumo.test" || c.msg.To != "user@x.test" || c.msg.ReplyTo != "reply@econumo.test" || c.msg.Subject != resetSubject {
		t.Errorf("message envelope = %+v", c.msg)
	}
	if !strings.Contains(c.msg.Text, "Alice") || !strings.Contains(c.msg.Text, "abc123def456") {
		t.Errorf("body should contain name + code: %q", c.msg.Text)
	}

	// With no From configured, it must not send (matching PHP) and not error.
	c2 := &captureMailer{}
	if err := NewResetSender(c2, "", "").SendResetPasswordCode(context.Background(), "u@x", "A", "code"); err != nil {
		t.Fatalf("no-from send: %v", err)
	}
	if c2.called {
		t.Error("should not send when From is empty")
	}
}
