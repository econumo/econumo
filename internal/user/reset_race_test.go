package user_test

import (
	"context"
	"testing"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// hookedPasswordRequests fires a hook once, right after GetByUserAndCode has
// produced the row the reset treats as its evidence: the window a replacement
// code (or a competing reset) has to win for the race tests below.
type hookedPasswordRequests struct {
	appuser.PasswordRequests
	afterGetByUserAndCode func()
}

func (h *hookedPasswordRequests) GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*model.PasswordRequest, error) {
	pr, err := h.PasswordRequests.GetByUserAndCode(ctx, userID, code)
	if err == nil && h.afterGetByUserAndCode != nil {
		fire := h.afterGetByUserAndCode
		h.afterGetByUserAndCode = nil
		fire()
	}
	return pr, err
}

// newResetRaceSvc wires the user Service over the given reset-code store (a
// race test hands in a decorated one) and a real reset sender, so a test can
// drive the production remind/reset pair and read the emailed code.
func newResetRaceSvc(t *testing.T, db *dbtest.DB, reqs appuser.PasswordRequests, cap *captureMailer) (*appuser.Service, *userrepo.Repo, *auth.PasswordHasher) {
	t.Helper()
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, server.NewUserCurrencyLookup(lookup), budgets,
		reqs, mailer.NewResetSender(cap, "noreply@econumo.test", ""),
		userrepo.NewEmailVerificationRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, false, 0, false)
	return svc, repo, hasher
}

// newResetRaceEnv builds that service plus one user holding a fresh reset code,
// and returns the code the remind flow emailed.
func newResetRaceEnv(t *testing.T, email, password string) (*appuser.Service, *userrepo.Repo, *auth.PasswordHasher, *hookedPasswordRequests, *captureMailer, vo.Id, string) {
	t.Helper()
	db := dbtest.New(t)
	cap := &captureMailer{}
	reqs := &hookedPasswordRequests{PasswordRequests: userrepo.NewPasswordRequestRepo(db.Engine, db.TX)}
	svc, repo, hasher := newResetRaceSvc(t, db, reqs, cap)
	ctx := context.Background()
	uid, err := svc.AdminCreateUser(ctx, "Owner", email, password)
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	if _, err := svc.RemindPassword(ctx, model.RemindPasswordRequest{Username: email}); err != nil {
		t.Fatalf("RemindPassword: %v", err)
	}
	return svc, repo, hasher, reqs, cap, uid, codeFrom(t, cap.msgs[len(cap.msgs)-1].Text)
}

// A code is evidence only while it is still the account's outstanding code: a
// remind that replaced it between the reset's lookup and its row lock must
// leave the old code dead, not merely superseded.
func TestResetPassword_CodeReplacedBeforeTheLockIsRefused(t *testing.T) {
	const email = "reset-replaced@econumo.test"
	const password = "secretpass1"
	svc, repo, hasher, reqs, _, uid, code := newResetRaceEnv(t, email, password)
	ctx := context.Background()

	reqs.afterGetByUserAndCode = func() {
		if _, err := svc.RemindPassword(ctx, model.RemindPasswordRequest{Username: email}); err != nil {
			t.Fatalf("RemindPassword (replacement): %v", err)
		}
	}

	_, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
		Username: email, Code: code, Password: "replaced-password",
	})
	if !isValidationCode(err, errs.CodeUserResetPasswordError) {
		t.Fatalf("want the reset-password error, got %v", err)
	}
	u, gerr := repo.GetByID(ctx, uid)
	if gerr != nil {
		t.Fatalf("GetByID: %v", gerr)
	}
	if !hasher.Verify(u.Algorithm, u.Password, password, u.Salt) {
		t.Fatal("the replaced code still reset the password")
	}
}

// One code is one reset. Two resets presenting the same code must not both
// succeed: the second to reach the lock has to find the code already consumed,
// or the later password silently overwrites the earlier one.
func TestResetPassword_ConcurrentResetWithTheSameCodeIsRefused(t *testing.T) {
	const email = "reset-concurrent@econumo.test"
	svc, repo, hasher, reqs, _, uid, code := newResetRaceEnv(t, email, "secretpass1")
	ctx := context.Background()

	reqs.afterGetByUserAndCode = func() {
		if _, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
			Username: email, Code: code, Password: "first-password",
		}); err != nil {
			t.Fatalf("the competing ResetPassword must succeed: %v", err)
		}
	}

	_, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
		Username: email, Code: code, Password: "second-password",
	})
	if !isValidationCode(err, errs.CodeUserResetPasswordError) {
		t.Fatalf("want the reset-password error, got %v", err)
	}
	u, gerr := repo.GetByID(ctx, uid)
	if gerr != nil {
		t.Fatalf("GetByID: %v", gerr)
	}
	if hasher.Verify(u.Algorithm, u.Password, "second-password", u.Salt) {
		t.Fatal("both resets landed: the stored password is the later one")
	}
	if !hasher.Verify(u.Algorithm, u.Password, "first-password", u.Salt) {
		t.Fatal("the completed reset's password is not the stored one")
	}
}
