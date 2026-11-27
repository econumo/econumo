package mailer

import (
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

func TestDeletionSender_InactivityNotice(t *testing.T) {
	c := &captureMailer{}
	s := NewDeletionSender(c, "from@econumo.test", "reply@econumo.test")
	if err := s.SendInactivityNotice(context.Background(), "de", "user@x.test", "Alice", 365); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !c.called {
		t.Fatal("expected the mailer to be called")
	}
	m := c.msg
	if m.From != "from@econumo.test" || m.To != "user@x.test" || m.ReplyTo != "reply@econumo.test" {
		t.Errorf("addressing = %+v", m)
	}
	if m.Subject != "Ihre Econumo-Daten sind zur Löschung vorgemerkt" {
		t.Errorf("subject = %q, want the German one", m.Subject)
	}
	for _, want := range []string{"Alice", "365"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("body missing %q:\n%s", want, m.Text)
		}
	}
	if len(m.Cc) != 0 {
		t.Errorf("the notice must not be copied, got cc %v", m.Cc)
	}
}

func TestDeletionSender_InactivityNoticeCarriesAppLinkOnce(t *testing.T) {
	c := &captureMailer{}
	s := NewDeletionSender(WithAppLink(c, "https://app.econumo.test"), "from@econumo.test", "")
	if err := s.SendInactivityNotice(context.Background(), "en", "user@x.test", "Alice", 365); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n := strings.Count(c.msg.Text, "https://app.econumo.test"); n != 1 {
		t.Errorf("app URL appears %d times, want once:\n%s", n, c.msg.Text)
	}
	if !strings.HasSuffix(c.msg.Text, "\nhttps://app.econumo.test") {
		t.Errorf("app URL should end the body:\n%s", c.msg.Text)
	}
}

func TestDeletionSender_DeletionCode(t *testing.T) {
	c := &captureMailer{}
	s := NewDeletionSender(c, "from@econumo.test", "reply@econumo.test")
	ctx := reqctx.WithLanguage(context.Background(), "en")
	if err := s.SendDeletionCode(ctx, "user@x.test", "Alice", "abc123def456"); err != nil {
		t.Fatalf("send: %v", err)
	}
	m := c.msg
	if m.To != "user@x.test" || m.Subject != "Confirm deleting your data" {
		t.Errorf("message = %+v", m)
	}
	for _, want := range []string{"Alice", "abc123def456"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("body missing %q:\n%s", want, m.Text)
		}
	}
	if len(m.Cc) != 0 {
		t.Errorf("a code email must never be copied, got cc %v", m.Cc)
	}
}
