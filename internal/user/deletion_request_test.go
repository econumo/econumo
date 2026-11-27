package user_test

import (
	"context"
	"errors"
	"regexp"
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

const (
	selfDeleteEmail    = "self-delete@econumo.test"
	selfDeletePassword = "secretpass1"
)

type selfDeleteEnv struct {
	svc      *appuser.Service
	clk      *testClock
	mail     *passMailer
	tokens   *userrepo.AccessTokenRepo
	store    *userrepo.DeletionRepo
	requests *userrepo.DeletionRequestRepo
}

func newSelfDeleteEnv(t *testing.T) *selfDeleteEnv {
	t.Helper()
	return newSelfDeleteEnvWrapped(t, nil)
}

// newSelfDeleteEnvWrapped hands the Service a decorated user repository, so a
// test can act inside one of its calls.
func newSelfDeleteEnvWrapped(t *testing.T, wrap func(appuser.Repository, *testClock) appuser.Repository) *selfDeleteEnv {
	t.Helper()
	db := dbtest.New(t)
	clk := &testClock{now: authT0}
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	limiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{
			appuser.RateScopeRequestUserDeletion: 3,
			appuser.RateScopeConfirmUserDeletion: 5,
		},
		Window: time.Hour,
	}, clk)
	var repo appuser.Repository = userrepo.NewRepo(db.Engine, db.TX)
	if wrap != nil {
		repo = wrap(repo, clk)
	}
	svc := appuser.NewService(repo, db.TX, auth.NewEncodeService(""), auth.NewPasswordHasher(),
		tokens,
		server.NewUserCurrencyLookup(currencyrepo.New(db.Engine, db.TX)),
		server.NewUserBudgetAccess(db.Engine, db.TX),
		userrepo.NewPasswordRequestRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailVerificationRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, limiter, false, 0, false)
	e := &selfDeleteEnv{
		svc: svc, clk: clk, tokens: tokens,
		mail:     &passMailer{failTo: map[string]bool{}},
		store:    userrepo.NewDeletionRepo(db.Engine, db.TX),
		requests: userrepo.NewDeletionRequestRepo(db.Engine, db.TX),
	}
	svc.SetDeletion(e.store, userrepo.NewAuditRepo(db.Engine, db.TX), e.requests, appuser.DeletionConfig{InactiveDays: 365, DeletionDays: 30})
	svc.SetDeletionSender(mailer.NewDeletionSender(e.mail, "noreply@econumo.test", ""))
	return e
}

func (e *selfDeleteEnv) passwordUser(t *testing.T) vo.Id {
	t.Helper()
	id, err := e.svc.AdminCreateUser(context.Background(), "Self Deleter", selfDeleteEmail, selfDeletePassword)
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	return id
}

func (e *selfDeleteEnv) marked(t *testing.T, uid vo.Id) model.DeletionState {
	t.Helper()
	st, err := e.store.GetState(context.Background(), uid)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	return st
}

var deletionCodeRe = regexp.MustCompile(`delete your Econumo data: (\d{6})`)

func (e *selfDeleteEnv) lastCode(t *testing.T) string {
	t.Helper()
	if len(e.mail.msgs) == 0 {
		t.Fatal("no email was sent")
	}
	m := deletionCodeRe.FindStringSubmatch(e.mail.msgs[len(e.mail.msgs)-1].Text)
	if m == nil {
		t.Fatalf("no 6-digit code in %q", e.mail.msgs[len(e.mail.msgs)-1].Text)
	}
	return m[1]
}

func (e *selfDeleteEnv) request(t *testing.T, uid vo.Id, password string) {
	t.Helper()
	if _, err := e.svc.RequestUserDeletion(context.Background(), uid, model.RequestUserDeletionRequest{Password: password}); err != nil {
		t.Fatalf("RequestUserDeletion: %v", err)
	}
}

func wantValidationCode(t *testing.T, err error, code string) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.MsgCode != code {
		t.Fatalf("want ValidationError %s, got %v", code, err)
	}
}

func wantTooMany(t *testing.T, err error) {
	t.Helper()
	if _, ok := errs.AsTooManyRequests(err); !ok {
		t.Fatalf("want TooManyRequestsError, got %v", err)
	}
}

func TestRequestUserDeletion_PasswordUser(t *testing.T) {
	e := newSelfDeleteEnv(t)
	ctx := context.Background()
	uid := e.passwordUser(t)

	_, err := e.svc.RequestUserDeletion(ctx, uid, model.RequestUserDeletionRequest{Password: "wrong-password"})
	wantValidationCode(t, err, errs.CodeUserPasswordIncorrect)
	if len(e.mail.msgs) != 0 {
		t.Fatalf("a wrong password sent %d emails", len(e.mail.msgs))
	}
	if _, err := e.svc.RequestUserDeletion(ctx, uid, model.RequestUserDeletionRequest{}); err == nil {
		t.Fatal("an empty password must be refused for a user with a password")
	}

	e.request(t, uid, selfDeletePassword)
	if len(e.mail.msgs) != 1 || e.mail.msgs[0].To != selfDeleteEmail {
		t.Fatalf("want one email to %s, got %+v", selfDeleteEmail, e.mail.msgs)
	}
	code := e.lastCode(t)
	r, err := e.requests.GetByUser(ctx, uid)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if r.Code != appuser.HashResetCode(code) {
		t.Errorf("stored code = %q, want the hash of the emailed code", r.Code)
	}
	if r.Email != selfDeleteEmail {
		t.Errorf("stored email = %q, want %q", r.Email, selfDeleteEmail)
	}
}

func TestRequestUserDeletion_Passwordless(t *testing.T) {
	e := newSelfDeleteEnv(t)
	u, err := e.svc.ProvisionExternalUser(context.Background(), "No Password", "no-password@econumo.test")
	if err != nil {
		t.Fatalf("ProvisionExternalUser: %v", err)
	}
	e.request(t, u.ID, "")
	if len(e.mail.msgs) != 1 || e.mail.msgs[0].To != "no-password@econumo.test" {
		t.Fatalf("want one code email, got %+v", e.mail.msgs)
	}
}

func TestRequestUserDeletion_PasswordLoginDisabled(t *testing.T) {
	e := newSelfDeleteEnv(t)
	uid := e.passwordUser(t)
	e.svc.DisablePasswordLogin()
	e.request(t, uid, "")
	if len(e.mail.msgs) != 1 {
		t.Fatalf("want one code email, got %d", len(e.mail.msgs))
	}
}

func TestRequestUserDeletion_RateLimited(t *testing.T) {
	e := newSelfDeleteEnv(t)
	uid := e.passwordUser(t)
	for i := 0; i < 3; i++ {
		e.request(t, uid, selfDeletePassword)
	}
	_, err := e.svc.RequestUserDeletion(context.Background(), uid, model.RequestUserDeletionRequest{Password: selfDeletePassword})
	wantTooMany(t, err)
	if len(e.mail.msgs) != 3 {
		t.Fatalf("emails = %d, want 3", len(e.mail.msgs))
	}
}

func TestRequestUserDeletion_Unwired(t *testing.T) {
	e := newSelfDeleteEnv(t)
	uid := e.passwordUser(t)
	e.svc.SetDeletionSender(nil)
	if _, err := e.svc.RequestUserDeletion(context.Background(), uid, model.RequestUserDeletionRequest{Password: selfDeletePassword}); err == nil {
		t.Fatal("RequestUserDeletion without a sender must fail")
	}
	e.svc.SetDeletion(nil, nil, nil, appuser.DeletionConfig{})
	if _, err := e.svc.ConfirmUserDeletion(context.Background(), uid, model.ConfirmUserDeletionRequest{Code: "123456"}); err == nil {
		t.Fatal("ConfirmUserDeletion without the deletion stores must fail")
	}
}

func TestConfirmUserDeletion_Marks(t *testing.T) {
	e := newSelfDeleteEnv(t)
	ctx := context.Background()
	uid := e.passwordUser(t)
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, e.tokens, uid, model.TokenKindSession, "eco_ses_self-delete", &exp)
	e.request(t, uid, selfDeletePassword)
	code := e.lastCode(t)

	if _, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: code}); err != nil {
		t.Fatalf("ConfirmUserDeletion: %v", err)
	}
	st := e.marked(t, uid)
	if st.MarkedAt == nil || st.Reason != model.DeletionReasonSelf {
		t.Fatalf("state = %+v, want marked self_initiated", st)
	}
	if _, err := e.svc.Authenticate(ctx, "eco_ses_self-delete"); !isUnauthorized(err) {
		t.Errorf("the presenting session must stop authenticating, got %v", err)
	}
	if len(e.mail.msgs) != 1 {
		t.Errorf("the confirm sent an email: %d messages", len(e.mail.msgs))
	}
	if _, err := e.requests.GetByUser(ctx, uid); err == nil {
		t.Error("the request row must be consumed")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("GetByUser: %v", err)
	}
}

func TestConfirmUserDeletion_AddressChanged(t *testing.T) {
	e := newSelfDeleteEnv(t)
	ctx := context.Background()
	uid := e.passwordUser(t)
	e.request(t, uid, selfDeletePassword)
	code := e.lastCode(t)
	if err := e.svc.AdminChangeEmail(ctx, selfDeleteEmail, "moved-away@econumo.test"); err != nil {
		t.Fatalf("AdminChangeEmail: %v", err)
	}
	if _, err := e.requests.GetByUser(ctx, uid); err != nil {
		t.Fatalf("the request must survive the email change for this test to mean anything: %v", err)
	}

	_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: code})
	wantValidationCode(t, err, errs.CodeUserVerificationCodeInvalid)
	if st := e.marked(t, uid); st.MarkedAt != nil {
		t.Fatalf("a code sent to the old address marked the user: %+v", st)
	}
}

func TestConfirmUserDeletion_Refusals(t *testing.T) {
	ctx := context.Background()

	t.Run("wrong code", func(t *testing.T) {
		e := newSelfDeleteEnv(t)
		uid := e.passwordUser(t)
		e.request(t, uid, selfDeletePassword)
		wrong := "000000"
		if e.lastCode(t) == wrong {
			wrong = "111111"
		}
		_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: wrong})
		wantValidationCode(t, err, errs.CodeUserVerificationCodeInvalid)
		if st := e.marked(t, uid); st.MarkedAt != nil {
			t.Fatalf("marked: %+v", st)
		}
	})

	t.Run("no pending request", func(t *testing.T) {
		e := newSelfDeleteEnv(t)
		uid := e.passwordUser(t)
		_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: "123456"})
		wantValidationCode(t, err, errs.CodeUserVerificationCodeInvalid)
	})

	t.Run("expired", func(t *testing.T) {
		e := newSelfDeleteEnv(t)
		uid := e.passwordUser(t)
		e.request(t, uid, selfDeletePassword)
		code := e.lastCode(t)
		e.clk.now = e.clk.now.Add(model.DeletionRequestTTL + time.Second)
		_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: code})
		wantValidationCode(t, err, errs.CodeUserVerificationCodeExpired)
		if st := e.marked(t, uid); st.MarkedAt != nil {
			t.Fatalf("marked: %+v", st)
		}
	})

	t.Run("attempt cap", func(t *testing.T) {
		e := newSelfDeleteEnv(t)
		uid := e.passwordUser(t)
		e.request(t, uid, selfDeletePassword)
		code := e.lastCode(t)
		wrong := "000000"
		if code == wrong {
			wrong = "111111"
		}
		for i := 0; i < 5; i++ {
			_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: wrong})
			wantValidationCode(t, err, errs.CodeUserVerificationCodeInvalid)
		}
		_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: code})
		wantTooMany(t, err)
		if st := e.marked(t, uid); st.MarkedAt != nil {
			t.Fatalf("marked: %+v", st)
		}
	})
}

// expiringLock moves the clock past the code's expiry when the confirm takes
// the user row lock: the confirm waited behind another writer meanwhile.
type expiringLock struct {
	appuser.Repository
	clk   *testClock
	armed *bool
}

func (r expiringLock) LockRow(ctx context.Context, id vo.Id) error {
	if *r.armed {
		*r.armed = false
		r.clk.now = r.clk.now.Add(model.DeletionRequestTTL + time.Second)
	}
	return r.Repository.LockRow(ctx, id)
}

func TestConfirmUserDeletion_CodeExpiresWhileWaitingForTheLock(t *testing.T) {
	ctx := context.Background()
	armed := false
	e := newSelfDeleteEnvWrapped(t, func(inner appuser.Repository, clk *testClock) appuser.Repository {
		return expiringLock{Repository: inner, clk: clk, armed: &armed}
	})
	uid := e.passwordUser(t)
	e.request(t, uid, selfDeletePassword)
	code := e.lastCode(t)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}

	armed = true
	_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: code})
	wantValidationCode(t, err, errs.CodeUserVerificationCodeExpired)
	if st := e.marked(t, uid); st.MarkedAt != nil {
		t.Fatalf("an expired code marked the user: %+v", st)
	}

	// the refusal counted as an attempt: four more fill the cap of five
	for i := 0; i < 4; i++ {
		_, err := e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: wrong})
		wantValidationCode(t, err, errs.CodeUserVerificationCodeInvalid)
	}
	_, err = e.svc.ConfirmUserDeletion(ctx, uid, model.ConfirmUserDeletionRequest{Code: wrong})
	wantTooMany(t, err)
}
