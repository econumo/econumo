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
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// changeCodeFrom extracts the 6-digit code from a rendered change_email email
// body ("...confirm your new email address: <code>...").
func changeCodeFrom(t *testing.T, body string) string {
	t.Helper()
	const marker = "email address: "
	i := strings.Index(body, marker)
	if i < 0 || len(body) < i+len(marker)+6 {
		t.Fatalf("no code in email body: %q", body)
	}
	return body[i+len(marker) : i+len(marker)+6]
}

// newChangeEmailEnv builds the user Service with a real EmailChangeRequestRepo,
// a capturing ChangeEmailSender, and a real rate limiter (generous caps, so
// individual tests can tighten it or step the clock to hit cooldowns/limits
// deliberately), mirroring server.BuildAPI's wiring for the change-email scopes.
func newChangeEmailEnv(t *testing.T) (svc *appuser.Service, repo *userrepo.Repo, tokens *userrepo.AccessTokenRepo, ecRepo *userrepo.EmailChangeRequestRepo, enc *auth.EncodeService, cap *captureMailer, clk *testClock) {
	t.Helper()
	db := dbtest.New(t)
	clk = &testClock{now: authT0}
	enc = auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo = userrepo.NewRepo(db.Engine, db.TX)
	tokens = userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	evRepo := userrepo.NewEmailVerificationRepo(db.Engine, db.TX)
	ecRepo = userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX)
	cap = &captureMailer{}
	changeMailer := mailer.NewChangeEmailSender(cap, "noreply@econumo.test", "")
	limiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{
			appuser.RateScopeRequestEmailChange: 1000,
			appuser.RateScopeConfirmEmailChange: 1000,
		},
		Window: time.Hour,
		Global: 0,
	}, clk)
	svc = appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, nil, nil,
		evRepo, nil,
		ecRepo, changeMailer,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, limiter, false, "", false)
	return
}

func createChangeEmailUser(t *testing.T, svc *appuser.Service, name, email, password string) vo.Id {
	t.Helper()
	id, err := svc.AdminCreateUser(context.Background(), name, email, password)
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	return id
}

func TestRequestAndConfirmEmailChange_HappyPath(t *testing.T) {
	svc, repo, _, ecRepo, enc, cap, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "Change Me", "old@econumo.test", "secretpass1")

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "new@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}

	cr, err := ecRepo.GetByUser(ctx, uid)
	if err != nil {
		t.Fatalf("pending row must exist: %v", err)
	}
	if cr.NewEmail != "new@econumo.test" {
		t.Errorf("pending NewEmail = %q, want new@econumo.test", cr.NewEmail)
	}
	if len(cap.msgs) != 2 {
		t.Fatalf("want 2 emails (code + notice), got %d", len(cap.msgs))
	}
	if cap.msgs[0].To != "new@econumo.test" {
		t.Errorf("code email To = %q, want new@econumo.test", cap.msgs[0].To)
	}
	if cap.msgs[1].To != "old@econumo.test" {
		t.Errorf("notice email To = %q, want old@econumo.test", cap.msgs[1].To)
	}

	code := changeCodeFrom(t, cap.msgs[0].Text)
	result, err := svc.ConfirmEmailChange(ctx, uid, vo.Id{}, model.ConfirmEmailChangeRequest{Code: code})
	if err != nil {
		t.Fatalf("ConfirmEmailChange: %v", err)
	}
	if result.Email != "new@econumo.test" {
		t.Errorf("result email = %q, want new@econumo.test", result.Email)
	}

	if _, err := repo.GetByEmail(ctx, "new@econumo.test"); err != nil {
		t.Fatalf("new email must resolve: %v", err)
	}
	if _, err := repo.GetByEmail(ctx, "old@econumo.test"); !isNotFound(err) {
		t.Fatalf("old email must be gone, got %v", err)
	}
	u, err := repo.GetByID(ctx, uid)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !u.EmailVerified {
		t.Error("new email must be marked verified")
	}
	if got, _ := enc.Decode(u.Email); got != "new@econumo.test" {
		t.Errorf("decoded email = %q, want new@econumo.test", got)
	}
	if _, err := ecRepo.GetByUser(ctx, uid); !isNotFound(err) {
		t.Fatalf("pending row must be deleted, got %v", err)
	}
}

func TestRequestEmailChange_WrongPassword(t *testing.T) {
	svc, _, _, ecRepo, _, _, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "correctpass")

	_, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "new@econumo.test", Password: "wrongpass",
	})
	if !isValidationCode(err, errs.CodeUserPasswordIncorrect) {
		t.Fatalf("want password_incorrect, got %v", err)
	}
	if _, err := ecRepo.GetByUser(ctx, uid); !isNotFound(err) {
		t.Fatalf("no pending row should exist, got %v", err)
	}
}

func TestRequestEmailChange_SameEmail(t *testing.T) {
	svc, _, _, _, _, _, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "same@econumo.test", "secretpass1")

	_, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "same@econumo.test", Password: "secretpass1",
	})
	if !isValidationCode(err, errs.CodeUserEmailUnchanged) {
		t.Fatalf("want email_unchanged, got %v", err)
	}
}

func TestRequestEmailChange_DuplicateEmail(t *testing.T) {
	svc, _, _, _, _, _, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "secretpass1")
	createChangeEmailUser(t, svc, "B", "taken@econumo.test", "secretpass1")

	_, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "taken@econumo.test", Password: "secretpass1",
	})
	if !isValidationCode(err, errs.CodeUserAlreadyExists) {
		t.Fatalf("want already_exists, got %v", err)
	}
}

// TestConfirmEmailChange_DuplicateAtConfirm covers the commit-time race: the
// target address is free at request time but taken before the code is
// confirmed.
func TestConfirmEmailChange_DuplicateAtConfirm(t *testing.T) {
	svc, repo, _, _, enc, cap, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "secretpass1")

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "race@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	code := changeCodeFrom(t, cap.msgs[0].Text)

	// The target address is taken by someone else after the request.
	encrypted, err := enc.Encode("race@econumo.test")
	if err != nil {
		t.Fatal(err)
	}
	other := model.NewUser(vo.NewId(), encrypted, "Racer", "face:fuchsia", "hash", "salt", authT0)
	if err := repo.Save(ctx, other); err != nil {
		t.Fatalf("seed colliding user: %v", err)
	}

	if _, err := svc.ConfirmEmailChange(ctx, uid, vo.Id{}, model.ConfirmEmailChangeRequest{Code: code}); !isValidationCode(err, errs.CodeUserAlreadyExists) {
		t.Fatalf("want already_exists at confirm, got %v", err)
	}
}

func TestConfirmEmailChange_WrongCode(t *testing.T) {
	svc, _, _, _, _, cap, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "secretpass1")

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "new@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	_ = changeCodeFrom(t, cap.msgs[0].Text) // sanity: a real code exists

	if _, err := svc.ConfirmEmailChange(ctx, uid, vo.Id{}, model.ConfirmEmailChangeRequest{Code: "000000"}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("want verification_code_invalid, got %v", err)
	}
}

func TestConfirmEmailChange_ExpiredCode(t *testing.T) {
	svc, _, _, _, _, cap, clk := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "secretpass1")

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "new@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	code := changeCodeFrom(t, cap.msgs[0].Text)

	clk.now = clk.now.Add(11 * time.Minute) // past the 10-minute TTL
	if _, err := svc.ConfirmEmailChange(ctx, uid, vo.Id{}, model.ConfirmEmailChangeRequest{Code: code}); !isValidationCode(err, errs.CodeUserVerificationCodeExpired) {
		t.Fatalf("want verification_code_expired, got %v", err)
	}
}

// TestRequestEmailChange_SupersedesPending proves a second request replaces
// the first: only one pending row survives, and only the latest code confirms.
func TestRequestEmailChange_SupersedesPending(t *testing.T) {
	svc, _, _, ecRepo, _, cap, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "secretpass1")

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "first@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("first RequestEmailChange: %v", err)
	}
	oldCode := changeCodeFrom(t, cap.msgs[0].Text)

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "second@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("second RequestEmailChange: %v", err)
	}

	cr, err := ecRepo.GetByUser(ctx, uid)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if cr.NewEmail != "second@econumo.test" {
		t.Fatalf("pending NewEmail = %q, want second@econumo.test (the latest wins)", cr.NewEmail)
	}

	if _, err := svc.ConfirmEmailChange(ctx, uid, vo.Id{}, model.ConfirmEmailChangeRequest{Code: oldCode}); !isValidationCode(err, errs.CodeUserVerificationCodeInvalid) {
		t.Fatalf("superseded code must be invalid, got %v", err)
	}
}

// TestConfirmEmailChange_RevokesOtherSessionsKeepsPAT covers the session
// cascade: on confirm, every OTHER session is revoked, the presenting session
// survives, and PATs are untouched entirely.
func TestConfirmEmailChange_RevokesOtherSessionsKeepsPAT(t *testing.T) {
	svc, _, tokens, _, _, cap, _ := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "secretpass1")

	exp := authT0.Add(appuser.SessionTTL)
	presentingID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_presenting", &exp)
	otherID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_other", &exp)
	patID := seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_survivor", nil)

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "new@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	code := changeCodeFrom(t, cap.msgs[0].Text)

	if _, err := svc.ConfirmEmailChange(ctx, uid, presentingID, model.ConfirmEmailChangeRequest{Code: code}); err != nil {
		t.Fatalf("ConfirmEmailChange: %v", err)
	}

	presenting, err := tokens.GetByID(ctx, presentingID)
	if err != nil {
		t.Fatalf("GetByID presenting: %v", err)
	}
	if presenting.RevokedAt != nil {
		t.Error("the presenting session must survive")
	}
	other, err := tokens.GetByID(ctx, otherID)
	if err != nil {
		t.Fatalf("GetByID other: %v", err)
	}
	if other.RevokedAt == nil {
		t.Error("the other session must be revoked")
	}
	pat, err := tokens.GetByID(ctx, patID)
	if err != nil {
		t.Fatalf("GetByID pat: %v", err)
	}
	if pat.RevokedAt != nil {
		t.Error("PATs must never be revoked by a change-email confirm")
	}
}

func TestResendEmailChangeCode_Cooldown(t *testing.T) {
	svc, _, _, _, _, cap, clk := newChangeEmailEnv(t)
	ctx := context.Background()
	uid := createChangeEmailUser(t, svc, "A", "a@econumo.test", "secretpass1")

	if _, err := svc.RequestEmailChange(ctx, uid, model.RequestEmailChangeRequest{
		NewEmail: "new@econumo.test", Password: "secretpass1",
	}); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	// RequestEmailChange itself starts the cooldown (code + notice = 2 emails).
	if len(cap.msgs) != 2 {
		t.Fatalf("want 2 emails after request, got %d", len(cap.msgs))
	}

	_, wait, err := svc.ResendEmailChangeCode(ctx, uid)
	if err != nil {
		t.Fatalf("ResendEmailChangeCode: %v", err)
	}
	if wait <= 0 {
		t.Fatalf("wait inside the gap = %v, want > 0", wait)
	}
	if len(cap.msgs) != 2 {
		t.Fatalf("resend inside the gap must not send, got %d messages", len(cap.msgs))
	}

	clk.now = clk.now.Add(model.EmailVerificationResendGap + time.Second)
	_, wait2, err := svc.ResendEmailChangeCode(ctx, uid)
	if err != nil {
		t.Fatalf("ResendEmailChangeCode after gap: %v", err)
	}
	if wait2 <= 0 {
		t.Fatalf("wait after a real send = %v, want > 0 (the fresh gap)", wait2)
	}
	if len(cap.msgs) != 3 {
		t.Fatalf("resend past the gap must send a fresh code, got %d messages", len(cap.msgs))
	}
}
