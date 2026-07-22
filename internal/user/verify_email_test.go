package user_test

import (
	"context"
	"strings"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// captureMailer records outgoing messages so tests can read the emailed code.
type captureMailer struct{ msgs []mailer.Message }

func (c *captureMailer) Send(_ context.Context, m mailer.Message) error {
	c.msgs = append(c.msgs, m)
	return nil
}

// codeFrom extracts the 12-char hex code from the rendered email body
// ("Your confirmation code is: <code>.").
func codeFrom(t *testing.T, body string) string {
	t.Helper()
	const marker = "code is: "
	i := strings.Index(body, marker)
	if i < 0 || len(body) < i+len(marker)+12 {
		t.Fatalf("no code in email body: %q", body)
	}
	return body[i+len(marker) : i+len(marker)+12]
}

// newVerifySvcFlag builds the user service with registration enabled, a
// capture mailer, and the email-verification gate set by enabled, mirroring
// server.Build's wiring.
func newVerifySvcFlag(t *testing.T, db *dbtest.DB, cap *captureMailer, enabled bool) *appuser.Service {
	t.Helper()
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	prRepo := userrepo.NewPasswordRequestRepo(db.Engine, db.TX)
	evRepo := userrepo.NewEmailVerificationRepo(db.Engine, db.TX)
	return appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets,
		prRepo, mailer.NewResetSender(cap, "noreply@econumo.test", ""),
		evRepo, mailer.NewVerifySender(cap, "noreply@econumo.test", ""),
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, true, "", enabled)
}

func newVerifySvc(t *testing.T, db *dbtest.DB, cap *captureMailer) *appuser.Service {
	t.Helper()
	return newVerifySvcFlag(t, db, cap, true)
}

func isVerificationDenied(err error, code string) bool {
	v, ok := errs.AsAccessDenied(err)
	return ok && v.Code == code
}

func TestLoginBlockedUntilEmailVerified(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Verify Me", Email: "verify@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(cap.msgs) != 0 {
		t.Fatal("registration must not send any email")
	}

	// First login: correct password, unverified -> 403-coded error + one email.
	_, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserEmailVerificationRequired) {
		t.Fatalf("want email_verification_required, got %v", err)
	}
	if len(cap.msgs) != 1 {
		t.Fatalf("want exactly 1 verification email, got %d", len(cap.msgs))
	}

	// Second code-less login while the code is outstanding: NO new email.
	_, err = svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserEmailVerificationRequired) {
		t.Fatalf("want email_verification_required again, got %v", err)
	}
	if len(cap.msgs) != 1 {
		t.Fatalf("outstanding code must not be re-sent, got %d emails", len(cap.msgs))
	}

	// Wrong password NEVER reaches the verification layer.
	_, err = svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "wrong"}, "ua", time.Now())
	if _, ok := errs.AsUnauthorized(err); !ok {
		t.Fatalf("bad password must stay a 401, got %v", err)
	}

	// Wrong code -> invalid-code 403.
	_, err = svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1", Code: "000000000000"}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("want verification_code_invalid, got %v", err)
	}

	// Correct code -> full login result in ONE call, user persisted verified.
	code := codeFrom(t, cap.msgs[0].Text)
	res, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1", Code: code}, "ua", time.Now())
	if err != nil {
		t.Fatalf("verified login: %v", err)
	}
	if res.Token == "" {
		t.Fatal("verified login must mint a session token")
	}

	// Subsequent logins skip the gate entirely.
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now()); err != nil {
		t.Fatalf("post-verification login: %v", err)
	}
}

func TestLoginResendForcesFreshCode(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Resend Me", Email: "resend@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	_, err := svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1", Resend: true}, "ua", time.Now())
	if !isVerificationDenied(err, errs.CodeUserEmailVerificationRequired) {
		t.Fatalf("resend still answers verification_required, got %v", err)
	}
	if len(cap.msgs) != 2 {
		t.Fatalf("resend must send a fresh email, got %d", len(cap.msgs))
	}
	// The OLD code is dead, the NEW one works.
	oldCode := codeFrom(t, cap.msgs[0].Text)
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1", Code: oldCode}, "ua", time.Now()); !isVerificationDenied(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("replaced code must be invalid, got %v", err)
	}
	newCode := codeFrom(t, cap.msgs[1].Text)
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1", Code: newCode}, "ua", time.Now()); err != nil {
		t.Fatalf("fresh code must verify: %v", err)
	}
}

func TestFlagOffKeepsLoginUnchanged(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvcFlag(t, db, cap, false) // gate disabled, registration enabled
	ctx := context.Background()
	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Legacy Flow", Email: "legacy@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	// Flag off: users are born verified and login needs no code (and sends no email).
	u, err := userrepo.NewRepo(db.Engine, db.TX).GetByIdentifier(ctx, auth.NewEncodeService("").Hash("legacy@econumo.test"))
	if err != nil {
		t.Fatalf("load registered user: %v", err)
	}
	if !u.EmailVerified {
		t.Error("flag off: registration must create a VERIFIED user")
	}
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "legacy@econumo.test", Password: "secretpass1"}, "ua", time.Now()); err != nil {
		t.Fatalf("flag off: login must succeed without verification, got %v", err)
	}
	if len(cap.msgs) != 0 {
		t.Errorf("flag off must never send verification email, got %d", len(cap.msgs))
	}
}

func TestResetPasswordMarksEmailVerified(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Reset Me", Email: "reset-verify@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemindPassword(ctx, model.RemindPasswordRequest{Username: "reset-verify@econumo.test"}); err != nil {
		t.Fatal(err)
	}
	resetCode := codeFrom(t, cap.msgs[len(cap.msgs)-1].Text)
	if _, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
		Username: "reset-verify@econumo.test", Code: resetCode, Password: "newsecret1",
	}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	// A completed reset proved mailbox ownership: login needs no code now.
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "reset-verify@econumo.test", Password: "newsecret1"}, "ua", time.Now()); err != nil {
		t.Fatalf("login after reset must skip verification, got %v", err)
	}
}
