package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// captureMailer records the last message sent through it, mirroring the
// unexported one in internal/infra/mailer's own test file (not reusable
// across package boundaries).
type captureMailer struct {
	msg    mailer.Message
	called bool
}

func (c *captureMailer) Send(_ context.Context, m mailer.Message) error {
	c.msg, c.called = m, true
	return nil
}

// newTestUserSvc builds a real *appuser.Service the same way Build() does
// (see TestOAuthUsers_FullyHydratesFromRealUserService for the rationale: the
// notifier glue must observe the SAME hydration/decoding the rest of the user
// feature does, not a shortcut).
func newTestUserSvc(db *dbtest.DB) (*appuser.Service, *userrepo.Repo) {
	txm := backend.NewTxManager(db.Raw)
	userRepo := userrepo.NewRepo(db.Engine, txm)
	accessTokens := userrepo.NewAccessTokenRepo(db.Engine, txm)
	authLimiter := ratelimit.New(ratelimit.Config{Window: 15 * time.Minute, Global: 60}, clock.New())
	userSvc := appuser.NewService(
		userRepo, txm, auth.NewEncodeService(""), auth.NewPasswordHasher(), accessTokens,
		nil, nil, nil, nil, nil, nil, nil, nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), authLimiter, true, 0, false,
	)
	return userSvc, userRepo
}

func TestOAuthNotifier_IdentityLinked_RendersInTheStoredLanguage(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fx := fixture.New(t, db)
	userID := fx.User(fixture.User{Email: "owner@example.test", Name: "Bob", Algorithm: "argon2id"})

	userSvc, userRepo := newTestUserSvc(db)
	if err := userRepo.UpdateLanguage(context.Background(), vo.MustParseId(userID), "ru"); err != nil {
		t.Fatalf("seed language: %v", err)
	}

	c := &captureMailer{}
	sender := mailer.NewIdentityLinkedSender(c, "from@econumo.test", "reply@econumo.test")
	notifier := NewOAuthNotifier(userSvc, sender)

	if err := notifier.IdentityLinked(context.Background(), vo.MustParseId(userID), "Google"); err != nil {
		t.Fatalf("IdentityLinked: %v", err)
	}
	if !c.called {
		t.Fatal("expected the mailer to be called")
	}
	if c.msg.To != "owner@example.test" {
		t.Errorf("To = %q, want the decrypted account email", c.msg.To)
	}
	if c.msg.From != "from@econumo.test" || c.msg.ReplyTo != "reply@econumo.test" {
		t.Errorf("envelope = %+v", c.msg)
	}
	if !strings.Contains(c.msg.Text, "Bob") || !strings.Contains(c.msg.Text, "Google") {
		t.Errorf("body should contain the name and provider: %q", c.msg.Text)
	}
	// Rendered in the account's stored language (ru), not English.
	if strings.Contains(c.msg.Subject, "linked to your account") {
		t.Errorf("subject rendered in English despite a stored ru language: %q", c.msg.Subject)
	}
}

func TestOAuthNotifier_IdentityLinked_DefaultsToEnglish(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fx := fixture.New(t, db)
	userID := fx.User(fixture.User{Email: "owner2@example.test", Name: "Ann", Algorithm: "argon2id"})

	userSvc, _ := newTestUserSvc(db)
	c := &captureMailer{}
	sender := mailer.NewIdentityLinkedSender(c, "from@econumo.test", "reply@econumo.test")
	notifier := NewOAuthNotifier(userSvc, sender)

	if err := notifier.IdentityLinked(context.Background(), vo.MustParseId(userID), "Apple"); err != nil {
		t.Fatalf("IdentityLinked: %v", err)
	}
	if c.msg.Subject != "A new sign-in method was linked to your account" {
		t.Errorf("subject = %q, want the English default (users.language defaults to en)", c.msg.Subject)
	}
	if c.msg.To != "owner2@example.test" {
		t.Errorf("To = %q", c.msg.To)
	}
}
