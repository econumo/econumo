package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// testClock is an advanceable port.Clock for exercising the touch throttle
// and expiry windows deterministically.
type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

var authT0 = time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

// newAuthEnv builds the user Service over sqlite with a controllable clock
// plus direct handles on the token + password-request repos for
// seeding/asserting rows.
func newAuthEnv(t *testing.T) (*appuser.Service, *userrepo.AccessTokenRepo, *testClock, vo.Id) {
	svc, tokens, clk, uid, _ := newAuthEnvFull(t)
	return svc, tokens, clk, uid
}

func newAuthEnvFull(t *testing.T) (*appuser.Service, *userrepo.AccessTokenRepo, *testClock, vo.Id, *userrepo.PasswordRequestRepo) {
	t.Helper()
	return newAuthEnvFullOn(t, dbtest.New(t))
}

func newAuthEnvOn(t *testing.T, db *dbtest.DB) (*appuser.Service, *userrepo.AccessTokenRepo, *testClock, vo.Id) {
	svc, tokens, clk, uid, _ := newAuthEnvFullOn(t, db)
	return svc, tokens, clk, uid
}

func newAuthEnvFullOn(t *testing.T, db *dbtest.DB) (*appuser.Service, *userrepo.AccessTokenRepo, *testClock, vo.Id, *userrepo.PasswordRequestRepo) {
	t.Helper()
	clk := &testClock{now: authT0}
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	pwreqs := userrepo.NewPasswordRequestRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, pwreqs, nil, appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, nil, false, "")

	uid, err := svc.AdminCreateUser(context.Background(), "Auth Tester", "auth@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return svc, tokens, clk, uid, pwreqs
}

func seedToken(t *testing.T, tokens *userrepo.AccessTokenRepo, userID vo.Id, kind, raw string, exp *time.Time) vo.Id {
	t.Helper()
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: userID, Kind: kind, TokenHash: appuser.HashAccessToken(raw),
		CreatedAt: authT0, LastUsedAt: authT0, ExpiresAt: exp,
	}
	if err := tokens.Insert(context.Background(), tok); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return tok.ID
}

func isUnauthorized(err error) bool {
	var ue *errs.UnauthorizedError
	return errors.As(err, &ue)
}

func TestAuthenticate_HappyPathAndCurrentIds(t *testing.T) {
	svc, tokens, _, uid := newAuthEnv(t)
	exp := authT0.Add(appuser.SessionTTL)
	tokID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_happy", &exp)

	gotUser, gotTok, _, err := svc.Authenticate(context.Background(), "eco_ses_happy")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !gotUser.Equal(uid) || !gotTok.Equal(tokID) {
		t.Errorf("ids mismatch: user %s token %s", gotUser, gotTok)
	}
}

func TestAuthenticate_UnknownToken401(t *testing.T) {
	svc, _, _, _ := newAuthEnv(t)
	_, _, _, err := svc.Authenticate(context.Background(), "eco_ses_bogus")
	if !isUnauthorized(err) {
		t.Fatalf("want Unauthorized, got %v", err)
	}
	if err.Error() != "Invalid access token" {
		t.Errorf("message = %q", err.Error())
	}
}

func TestAuthenticate_ExpiredSession401(t *testing.T) {
	svc, tokens, _, uid := newAuthEnv(t)
	past := authT0.Add(-time.Hour)
	seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_expired", &past)

	if _, _, _, err := svc.Authenticate(context.Background(), "eco_ses_expired"); !isUnauthorized(err) {
		t.Fatalf("want Unauthorized, got %v", err)
	}
}

func TestAuthenticate_RevokedToken401(t *testing.T) {
	svc, tokens, _, uid := newAuthEnv(t)
	exp := authT0.Add(appuser.SessionTTL)
	tokID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_revoked", &exp)
	ctx := context.Background()
	tok, err := tokens.GetByID(ctx, tokID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	tok.Revoke(authT0)
	if err := tokens.Update(ctx, tok); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if _, _, _, err := svc.Authenticate(ctx, "eco_ses_revoked"); !isUnauthorized(err) {
		t.Fatalf("want Unauthorized, got %v", err)
	}
}

func TestAuthenticate_SlidingTouchThrottled(t *testing.T) {
	svc, tokens, clk, uid := newAuthEnv(t)
	exp := authT0.Add(appuser.SessionTTL)
	tokID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_touch", &exp)
	ctx := context.Background()

	// +2min: inside the throttle window; nothing persisted.
	clk.now = authT0.Add(2 * time.Minute)
	if _, _, _, err := svc.Authenticate(ctx, "eco_ses_touch"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	row, _ := tokens.GetByID(ctx, tokID)
	if !row.LastUsedAt.Equal(authT0) {
		t.Errorf("last_used_at moved inside throttle window: %v", row.LastUsedAt)
	}

	// +6min: past the throttle; last_used_at and expires_at slide.
	clk.now = authT0.Add(6 * time.Minute)
	if _, _, _, err := svc.Authenticate(ctx, "eco_ses_touch"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	row, _ = tokens.GetByID(ctx, tokID)
	if !row.LastUsedAt.Equal(clk.now) {
		t.Errorf("last_used_at = %v, want %v", row.LastUsedAt, clk.now)
	}
	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(clk.now.Add(appuser.SessionTTL)) {
		t.Errorf("expires_at = %v, want %v", row.ExpiresAt, clk.now.Add(appuser.SessionTTL))
	}
}

func TestAuthenticate_PATExpiryNeverSlides(t *testing.T) {
	svc, tokens, clk, uid := newAuthEnv(t)
	patExp := authT0.Add(48 * time.Hour)
	tokID := seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_fixed", &patExp)
	ctx := context.Background()

	clk.now = authT0.Add(6 * time.Minute)
	if _, _, _, err := svc.Authenticate(ctx, "eco_pat_fixed"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	row, _ := tokens.GetByID(ctx, tokID)
	if !row.LastUsedAt.Equal(clk.now) {
		t.Errorf("last_used_at = %v, want %v", row.LastUsedAt, clk.now)
	}
	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(patExp) {
		t.Errorf("PAT expires_at moved: %v, want %v", row.ExpiresAt, patExp)
	}
}

func TestAuthenticate_ReturnsReadonlyWhenAccessLapsed(t *testing.T) {
	db := dbtest.New(t)
	svc, tokens, _, uid := newAuthEnvOn(t, db)
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_lapsed", &exp)

	past := authT0.Add(-24 * time.Hour)
	if _, err := db.Raw.ExecContext(context.Background(),
		db.Rebind("UPDATE users SET access_until = ? WHERE id = ?"),
		past.Format(datetime.Layout), uid.String()); err != nil {
		t.Fatalf("lapse access: %v", err)
	}

	_, _, level, err := svc.Authenticate(context.Background(), "eco_ses_lapsed")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if level != model.AccessLevelReadonly {
		t.Fatalf("level: got %q want readonly", level)
	}
}

func TestAuthenticate_ReturnsFullForUnexpiredAccess(t *testing.T) {
	db := dbtest.New(t)
	svc, tokens, _, uid := newAuthEnvOn(t, db)
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_live", &exp)

	_, _, level, err := svc.Authenticate(context.Background(), "eco_ses_live")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if level != model.AccessLevelFull {
		t.Fatalf("level: got %q want full", level)
	}
}
