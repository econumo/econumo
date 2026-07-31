package mailer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

func TestNew_ProviderSelection(t *testing.T) {
	if _, ok := New("resend", "re_test_123").(*resendMailer); !ok {
		t.Error(`provider "resend" should give a Resend-backed mailer`)
	}
	if _, ok := New("console", "").(console); !ok {
		t.Error(`provider "console" should give a console mailer`)
	}
	if _, ok := New("", "").(console); !ok {
		t.Error("the empty default provider should give a console mailer")
	}
}

func TestConsole_RendersMessage(t *testing.T) {
	var buf bytes.Buffer
	c := console{out: &buf}
	msg := Message{From: "from@x.test", To: "to@x.test", ReplyTo: "reply@x.test", Subject: "Hi", Text: "body line\nsecond line"}
	if err := c.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"from@x.test", "to@x.test", "reply@x.test", "Hi", "body line", "second line"} {
		if !strings.Contains(out, want) {
			t.Errorf("console output missing %q\ngot:\n%s", want, out)
		}
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
	if c.msg.From != "from@econumo.test" || c.msg.To != "user@x.test" || c.msg.ReplyTo != "reply@econumo.test" || c.msg.Subject != "Reset password confirmation code" {
		t.Errorf("message envelope = %+v", c.msg)
	}
	if !strings.Contains(c.msg.Text, "Alice") || !strings.Contains(c.msg.Text, "abc123def456") {
		t.Errorf("body should contain name + code: %q", c.msg.Text)
	}

	// With no From configured it still hands the message to the transport: the
	// console default must render it, so the From gate is gone.
	c2 := &captureMailer{}
	if err := NewResetSender(c2, "", "").SendResetPasswordCode(context.Background(), "u@x", "A", "code"); err != nil {
		t.Fatalf("no-from send: %v", err)
	}
	if !c2.called {
		t.Error("should still send when From is empty (console default renders it)")
	}
}

// sendResetCapture builds a ResetSender over the package's capturing test
// Mailer, sends a reset code through it, and returns the captured Message.
func sendResetCapture(t *testing.T, ctx context.Context, to, name, code string) Message {
	t.Helper()
	c := &captureMailer{}
	s := NewResetSender(c, "from@econumo.test", "reply@econumo.test")
	if err := s.SendResetPasswordCode(ctx, to, name, code); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !c.called {
		t.Fatal("expected the mailer to be called")
	}
	return c.msg
}

func TestResetEmailEnglishUnchanged(t *testing.T) {
	msg := sendResetCapture(t, context.Background(), "u@example.test", "Alice", "123456")
	if msg.Subject != "Reset password confirmation code" {
		t.Fatalf("subject = %q", msg.Subject)
	}
	want := "Hi Alice,\nYour confirmation code is: 123456.\n\nIf you didn't request this code, please ignore this email.\n\n--\nEconumo — Manage money. Together.\n"
	if msg.Text != want {
		t.Fatalf("en body drifted:\n%q\nwant:\n%q", msg.Text, want)
	}
}

func TestWithAppLink(t *testing.T) {
	c := &captureMailer{}
	m := WithAppLink(c, "https://money.example.com")

	// A body with a trailing newline (reset/verify shape) puts the bare URL on
	// the line directly below the body, no blank line between.
	if err := m.Send(context.Background(), Message{Text: "line one\nfooter\n"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if want := "line one\nfooter\nhttps://money.example.com"; c.msg.Text != want {
		t.Fatalf("trailing-newline body:\n%q\nwant:\n%q", c.msg.Text, want)
	}

	// A body with no trailing newline (change-email shape) gets the same
	// single-newline separator.
	if err := m.Send(context.Background(), Message{Text: "no footer here"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if want := "no footer here\nhttps://money.example.com"; c.msg.Text != want {
		t.Fatalf("no-newline body:\n%q\nwant:\n%q", c.msg.Text, want)
	}
}

func TestResetEmailRussian(t *testing.T) {
	ctx := reqctx.WithLanguage(context.Background(), "ru")
	msg := sendResetCapture(t, ctx, "u@example.test", "Алиса", "123456")
	if !strings.Contains(msg.Text, "Алиса") || !strings.Contains(msg.Text, "123456") {
		t.Fatalf("ru body missing name/code: %q", msg.Text)
	}
	if strings.Contains(msg.Text, "confirmation code is") {
		t.Fatalf("ru body still English: %q", msg.Text)
	}
}
