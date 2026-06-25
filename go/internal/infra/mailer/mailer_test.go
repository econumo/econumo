package mailer

import (
	"context"
	"strings"
	"testing"
)

func TestNew_EmptyKeyIsNoop(t *testing.T) {
	if _, ok := New("").(noop); !ok {
		t.Error("empty API key should give a no-op mailer")
	}
	if _, ok := New("   ").(noop); !ok {
		t.Error("blank API key should give a no-op mailer")
	}
	if _, ok := New("re_test_123").(*resendMailer); !ok {
		t.Error("a non-empty API key should give a Resend-backed mailer")
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
