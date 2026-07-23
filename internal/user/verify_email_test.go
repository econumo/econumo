package user_test

import (
	"context"
	"strings"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/infra/ratelimit"
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

// codeFrom extracts the 6-digit code from the rendered email body
// ("Your confirmation code is: <code>.").
func codeFrom(t *testing.T, body string) string {
	t.Helper()
	const marker = "code is: "
	i := strings.Index(body, marker)
	if i < 0 || len(body) < i+len(marker)+6 {
		t.Fatalf("no code in email body: %q", body)
	}
	return body[i+len(marker) : i+len(marker)+6]
}

// newVerifySvcFlag builds the user service with registration enabled, a
// capture mailer, and the email-verification gate set by enabled, mirroring
// server.Build's wiring.
func newVerifySvcFlag(t *testing.T, db *dbtest.DB, cap *captureMailer, enabled bool) *appuser.Service {
	t.Helper()
	svc, _ := newVerifySvcClock(t, db, cap, enabled)
	return svc
}

// newVerifySvcClock is newVerifySvcFlag with the clock handed back, so a test
// can step past the resend gap instead of sleeping through it.
func newVerifySvcClock(t *testing.T, db *dbtest.DB, cap *captureMailer, enabled bool) (*appuser.Service, *testClock) {
	t.Helper()
	clk := &testClock{now: authT0}
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
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, nil, true, "", enabled), clk
}

func newVerifySvc(t *testing.T, db *dbtest.DB, cap *captureMailer) *appuser.Service {
	t.Helper()
	return newVerifySvcFlag(t, db, cap, true)
}

// newVerifySvcLimited builds the user service wired with a REAL ratelimit.Limiter
// capping the verify-email scope at limit attempts per key, mirroring
// server.BuildAPI's ratelimit.New wiring, so tests can observe the 429 boundary.
func newVerifySvcLimited(t *testing.T, db *dbtest.DB, cap *captureMailer, limit int) (*appuser.Service, *testClock) {
	t.Helper()
	clk := &testClock{now: authT0}
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	prRepo := userrepo.NewPasswordRequestRepo(db.Engine, db.TX)
	evRepo := userrepo.NewEmailVerificationRepo(db.Engine, db.TX)
	limiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{appuser.RateScopeVerifyEmail: limit},
		Window: time.Hour,
		Global: 0,
	}, clk)
	return appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets,
		prRepo, mailer.NewResetSender(cap, "noreply@econumo.test", ""),
		evRepo, mailer.NewVerifySender(cap, "noreply@econumo.test", ""),
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, limiter, true, "", true), clk
}

// resendSeconds calls the resend use case and returns the advertised wait in
// whole seconds (what the HTTP edge puts on Retry-After).
func resendSeconds(t *testing.T, svc *appuser.Service, username string) int {
	t.Helper()
	_, wait, err := svc.ResendVerificationCode(context.Background(), model.ResendVerificationCodeRequest{Username: username})
	if err != nil {
		t.Fatalf("ResendVerificationCode(%s): %v", username, err)
	}
	return int(wait / time.Second)
}

func isVerificationDenied(err error, code string) bool {
	v, ok := errs.AsAccessDenied(err)
	return ok && v.Code == code
}

func isValidationCode(err error, code string) bool {
	v, ok := errs.AsValidation(err)
	return ok && v.MsgCode == code
}

func TestLoginBlockedUntilEmailConfirmed(t *testing.T) {
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

	// Wrong code -> generic invalid-code validation error.
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "verify@econumo.test", Code: "000000000000"}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("want verification_code_invalid, got %v", err)
	}

	// Unknown user -> the SAME generic error (anti-enumeration).
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "nobody@econumo.test", Code: "000000000000"}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("unknown user must be indistinguishable from a bad code, got %v", err)
	}

	// Correct code -> empty success; login then succeeds without any code.
	code := codeFrom(t, cap.msgs[0].Text)
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "verify@econumo.test", Code: code}); err != nil {
		t.Fatalf("ConfirmEmail: %v", err)
	}
	res, err := svc.Login(ctx, model.LoginRequest{Username: "verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if err != nil {
		t.Fatalf("login after confirm: %v", err)
	}
	if res.Token == "" {
		t.Fatal("login after confirm must mint a session token")
	}

	// The consumed code is dead.
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "verify@econumo.test", Code: code}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("consumed code must be invalid, got %v", err)
	}
}

func TestResendVerificationCode(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	// A limiter is wired (as in production) because the reported countdown is
	// read from it; enforcement itself holds either way.
	svc, clk := newVerifySvcLimited(t, db, cap, 100)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Resend Me", Email: "resend@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "resend@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	if len(cap.msgs) != 1 {
		t.Fatalf("blocked login must have sent the first email, got %d", len(cap.msgs))
	}

	// Inside the gap: no new email, and the caller is told how long is left.
	if got := resendSeconds(t, svc, "resend@econumo.test"); got != 60 {
		t.Fatalf("wait inside the gap = %d, want the full 60s remaining", got)
	}
	if len(cap.msgs) != 1 {
		t.Fatalf("resend inside the gap must not send, got %d emails", len(cap.msgs))
	}
	// The countdown measures from the last SEND, so repeated attempts shorten it
	// as real time passes — clicking again can never renew a user's own lockout.
	clk.now = clk.now.Add(30 * time.Second)
	if got := resendSeconds(t, svc, "resend@econumo.test"); got != 30 {
		t.Fatalf("wait 30s after the send = %d, want 30", got)
	}
	if len(cap.msgs) != 1 {
		t.Fatalf("still inside the gap must not send, got %d emails", len(cap.msgs))
	}

	// Past the gap: a FRESH code replaces the old one.
	clk.now = clk.now.Add(31 * time.Second)
	if got := resendSeconds(t, svc, "resend@econumo.test"); got != 60 {
		t.Fatalf("wait after a real send = %d, want the full 60s", got)
	}
	if len(cap.msgs) != 2 {
		t.Fatalf("resend must send a fresh email, got %d", len(cap.msgs))
	}
	oldCode := codeFrom(t, cap.msgs[0].Text)
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "resend@econumo.test", Code: oldCode}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("replaced code must be invalid, got %v", err)
	}
	newCode := codeFrom(t, cap.msgs[1].Text)
	if _, err := svc.ConfirmEmail(ctx, model.ConfirmEmailRequest{Username: "resend@econumo.test", Code: newCode}); err != nil {
		t.Fatalf("fresh code must confirm: %v", err)
	}

	// Anti-enumeration: unknown user and now-verified user both silently succeed,
	// send no email, AND report the same retry time a real send would — so the
	// number can never be read as "an unverified account exists here".
	sent := len(cap.msgs)
	unknown := resendSeconds(t, svc, "nobody@econumo.test")
	verified := resendSeconds(t, svc, "resend@econumo.test")
	if unknown != 60 || verified != 60 {
		t.Fatalf("no-op resends must report the same 60s as a real send; unknown=%d verified=%d",
			unknown, verified)
	}
	if len(cap.msgs) != sent {
		t.Fatalf("no-op resends must not send email, got %d new", len(cap.msgs)-sent)
	}
}

// TestResendCooldownDoesNotLeakAccountExistence is the enumeration guard for the
// reported countdown. Reading it from the verification ROW would answer a flat
// 60 for unknown usernames while a real unverified account mid-cooldown counted
// down (53, 48, ...), so any value other than 60 would prove the account exists.
// Both must tick identically.
func TestResendCooldownDoesNotLeakAccountExistence(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc, clk := newVerifySvcLimited(t, db, cap, 1000)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Real", Email: "real@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}

	// Start both cooldowns at the same instant: the real account via a blocked
	// login, the unknown one via its own first resend.
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "real@econumo.test", Password: "secretpass1"}, "ua", time.Now())
	resendSeconds(t, svc, "ghost@econumo.test")

	for elapsed := 10; elapsed <= 50; elapsed += 10 {
		clk.now = clk.now.Add(10 * time.Second)
		real := resendSeconds(t, svc, "real@econumo.test")
		ghost := resendSeconds(t, svc, "ghost@econumo.test")
		if real != ghost {
			t.Fatalf("at +%ds the countdown leaks existence: real=%d unknown=%d", elapsed, real, ghost)
		}
	}
}

// TestResendCooldownIsNotRenewedByAttempts guards the trap in measuring the gap
// from the last ATTEMPT: an impatient user clicking resend every few seconds
// would keep pushing their own deadline out and never receive a second code.
// The gap runs from the last SEND, so waiting it out always works.
func TestResendCooldownIsNotRenewedByAttempts(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc, clk := newVerifySvcLimited(t, db, cap, 1000)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Impatient", Email: "impatient@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "impatient@econumo.test", Password: "secretpass1"}, "ua", time.Now())

	// Click every 5s for the whole minute: no new email, and the countdown must
	// shrink monotonically rather than resetting.
	prev := 61
	for elapsed := 5; elapsed < 60; elapsed += 5 {
		clk.now = clk.now.Add(5 * time.Second)
		got := resendSeconds(t, svc, "impatient@econumo.test")
		if got >= prev {
			t.Fatalf("wait at +%ds = %d, did not shrink from %d — the gap is renewing itself",
				elapsed, got, prev)
		}
		prev = got
	}
	if len(cap.msgs) != 1 {
		t.Fatalf("hammering resend inside the gap must not send, got %d emails", len(cap.msgs))
	}

	// Once the gap truly elapses the next click delivers, despite all the noise.
	clk.now = clk.now.Add(10 * time.Second)
	resendSeconds(t, svc, "impatient@econumo.test")
	if len(cap.msgs) != 2 {
		t.Fatalf("a user who waited out the gap must get a new code, got %d emails", len(cap.msgs))
	}
}

// TestResendVerificationCodeRateLimitsUnknownUsers proves the enumeration-oracle
// fix: the verify-email limiter is consumed on EVERY ResendVerificationCode
// call, before the existence/verified check, so an unknown username hits the
// same 429 boundary, at the same cap, as a genuine unverified user.
func TestResendVerificationCodeRateLimitsUnknownUsers(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	const limit = 2
	svc, clk := newVerifySvcLimited(t, db, cap, limit)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Real User", Email: "limited@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatal(err)
	}

	// Unknown username: the first `limit` calls silently succeed (anti-enumeration
	// response), consuming the limiter; the next call hits the cap with a 429.
	for i := 0; i < limit; i++ {
		if _, _, err := svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "nobody@econumo.test"}); err != nil {
			t.Fatalf("unknown user call %d: want silent success, got %v", i+1, err)
		}
	}
	_, _, err := svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "nobody@econumo.test"})
	if _, ok := errs.AsTooManyRequests(err); !ok {
		t.Fatalf("unknown user: want TooManyRequestsError after %d attempts, got %v", limit, err)
	}

	// A real unverified user hits the SAME cap, at the SAME attempt count —
	// proving the limiter no longer treats unknown/verified callers specially.
	// The clock steps past the resend gap each time so this measures the
	// attempt cap alone, not the per-send cooldown.
	for i := 0; i < limit; i++ {
		if _, _, err := svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "limited@econumo.test"}); err != nil {
			t.Fatalf("real user call %d: %v", i+1, err)
		}
		clk.now = clk.now.Add(model.EmailVerificationResendGap + time.Second)
	}
	_, _, err = svc.ResendVerificationCode(ctx, model.ResendVerificationCodeRequest{Username: "limited@econumo.test"})
	if _, ok := errs.AsTooManyRequests(err); !ok {
		t.Fatalf("real user: want TooManyRequestsError after %d attempts, got %v", limit, err)
	}
	if len(cap.msgs) != limit {
		t.Fatalf("real user must have received exactly %d emails (the cap), got %d", limit, len(cap.msgs))
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
	u, err := userrepo.NewRepo(db.Engine, db.TX).GetByEmail(ctx, "legacy@econumo.test")
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

func TestAdminVerifyEmail(t *testing.T) {
	db := dbtest.New(t)
	cap := &captureMailer{}
	svc := newVerifySvc(t, db, cap)
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{Name: "Admin Verify", Email: "admin-verify@econumo.test", Password: "secretpass1"}); err != nil {
		t.Fatal(err)
	}
	// Trigger a pending code so the command also has a row to clean up.
	_, _ = svc.Login(ctx, model.LoginRequest{Username: "admin-verify@econumo.test", Password: "secretpass1"}, "ua", time.Now())

	if err := svc.AdminVerifyEmail(ctx, "admin-verify@econumo.test"); err != nil {
		t.Fatalf("AdminVerifyEmail: %v", err)
	}
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "admin-verify@econumo.test", Password: "secretpass1"}, "ua", time.Now()); err != nil {
		t.Fatalf("login after admin verify: %v", err)
	}
}
