package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// cascadeEnv: a user with two live sessions and one live PAT.
type cascadeEnv struct {
	svc      *appuser.Service
	tokens   *userrepo.AccessTokenRepo
	uid      vo.Id
	sessionA vo.Id
	sessionB vo.Id
	pat      vo.Id
}

func newCascadeEnv(t *testing.T) *cascadeEnv {
	t.Helper()
	svc, tokens, _, uid := newAuthEnv(t)
	exp := authT0.Add(appuser.SessionTTL)
	patExp := authT0.Add(90 * 24 * time.Hour)
	return &cascadeEnv{
		svc: svc, tokens: tokens, uid: uid,
		sessionA: seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_cascade-a", &exp),
		sessionB: seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_cascade-b", &exp),
		pat:      seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_cascade", &patExp),
	}
}

// liveness returns whether each of the three seeded tokens is currently live.
func (e *cascadeEnv) liveness(t *testing.T, now time.Time) (a, b, pat bool) {
	t.Helper()
	ctx := context.Background()
	check := func(id vo.Id) bool {
		tok, err := e.tokens.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("GetByID(%s): %v", id, err)
		}
		return tok.IsLive(now)
	}
	return check(e.sessionA), check(e.sessionB), check(e.pat)
}

func TestUpdatePassword_RevokesOtherSessionsKeepsCurrentAndPATs(t *testing.T) {
	e := newCascadeEnv(t)
	ctx := context.Background()

	_, err := e.svc.UpdatePassword(ctx, e.uid, e.sessionA, model.UpdatePasswordRequest{
		OldPassword: "secretpass", NewPassword: "next-secret",
	})
	if err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	a, b, pat := e.liveness(t, authT0.Add(time.Minute))
	if !a {
		t.Error("current session must survive a password change")
	}
	if b {
		t.Error("other session must be revoked on password change")
	}
	if !pat {
		t.Error("PATs must survive a password change")
	}
}

func TestAdminChangePassword_RevokesAllSessionsKeepsPATs(t *testing.T) {
	e := newCascadeEnv(t)

	if err := e.svc.AdminChangePassword(context.Background(), "auth@econumo.test", "next-secret"); err != nil {
		t.Fatalf("AdminChangePassword: %v", err)
	}
	a, b, pat := e.liveness(t, authT0.Add(time.Minute))
	if a || b {
		t.Errorf("all sessions must be revoked (a=%v b=%v)", a, b)
	}
	if !pat {
		t.Error("PATs must survive an admin password change")
	}
}

func TestAdminDeactivate_RevokesEverything(t *testing.T) {
	e := newCascadeEnv(t)

	if err := e.svc.AdminDeactivate(context.Background(), "auth@econumo.test"); err != nil {
		t.Fatalf("AdminDeactivate: %v", err)
	}
	a, b, pat := e.liveness(t, authT0.Add(time.Minute))
	if a || b || pat {
		t.Errorf("deactivate must revoke everything (a=%v b=%v pat=%v)", a, b, pat)
	}
}

func TestResetPassword_RevokesAllSessionsKeepsPATs(t *testing.T) {
	svc, tokens, _, uid, pwreqs := newAuthEnvFull(t)
	ctx := context.Background()
	exp := authT0.Add(appuser.SessionTTL)
	patExp := authT0.Add(90 * 24 * time.Hour)
	sesA := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_reset-a", &exp)
	sesB := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_reset-b", &exp)
	pat := seedToken(t, tokens, uid, model.TokenKindPersonal, "eco_pat_reset", &patExp)

	// Seed a valid reset code directly (the remind flow's persistence shape): the
	// stored value is the hash, the plaintext is what the reset request submits.
	pr := &model.PasswordRequest{
		ID: vo.NewId(), UserID: uid, Code: appuser.HashResetCode("abcdef123456"),
		CreatedAt: authT0, UpdatedAt: authT0, ExpiredAt: authT0.Add(10 * time.Minute),
	}
	if err := pwreqs.Save(ctx, pr); err != nil {
		t.Fatalf("seed password request: %v", err)
	}

	_, err := svc.ResetPassword(ctx, model.ResetPasswordRequest{
		Username: "auth@econumo.test", Code: "abcdef123456", Password: "next-secret",
	})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	now := authT0.Add(time.Minute)
	for _, tc := range []struct {
		id   vo.Id
		live bool
		name string
	}{{sesA, false, "session a"}, {sesB, false, "session b"}, {pat, true, "pat"}} {
		tok, gerr := tokens.GetByID(ctx, tc.id)
		if gerr != nil {
			t.Fatalf("GetByID(%s): %v", tc.name, gerr)
		}
		if tok.IsLive(now) != tc.live {
			t.Errorf("%s live=%v, want %v", tc.name, tok.IsLive(now), tc.live)
		}
	}
}

func TestPurgeDeadTokens(t *testing.T) {
	svc, tokens, clk, uid := newAuthEnv(t)
	ctx := context.Background()

	liveExp := authT0.Add(appuser.SessionTTL)
	oldExp := authT0.Add(-40 * 24 * time.Hour)
	liveID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_purge-live", &liveExp)
	deadID := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_purge-dead", &oldExp)

	clk.now = authT0
	n, err := svc.PurgeDeadTokens(ctx, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeDeadTokens: %v", err)
	}
	if n != 1 {
		t.Errorf("purged = %d, want 1", n)
	}
	if _, err := tokens.GetByID(ctx, liveID); err != nil {
		t.Error("live session must survive the purge")
	}
	if _, err := tokens.GetByID(ctx, deadID); err == nil {
		t.Error("dead session must be purged")
	}

	if _, err := svc.PurgeDeadTokens(ctx, -time.Hour); err == nil {
		t.Error("negative retention must be rejected")
	}
}
