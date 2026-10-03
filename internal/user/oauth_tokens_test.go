package user_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

const oauthTestEmail = "auth@econumo.test"

type fakeGrantRevoker struct{ users []vo.Id }

func (f *fakeGrantRevoker) RevokeAllForUser(_ context.Context, id vo.Id) (int64, error) {
	f.users = append(f.users, id)
	return 1, nil
}

func issueOAuth(t *testing.T, svc *appuser.Service, uid, grantID vo.Id) string {
	t.Helper()
	ctx := context.Background()
	gen, err := svc.CredentialsGeneration(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok, err := svc.IssueOAuthAccessToken(ctx, uid, grantID, "Claude", gen, time.Hour)
	if err != nil || !ok {
		t.Fatalf("issue = %v %v", ok, err)
	}
	return raw
}

func TestIssueOAuthAccessToken_AuthenticatesWithMCPScope(t *testing.T) {
	svc, tokens, clk, uid := newAuthEnv(t)
	ctx := context.Background()
	grantID := vo.NewId()

	raw := issueOAuth(t, svc, uid, grantID)
	if !strings.HasPrefix(raw, "eco_oat_") {
		t.Fatalf("token prefix = %q", raw)
	}
	p, err := svc.Authenticate(ctx, raw)
	if err != nil || p.Scope != model.TokenScopeMCP || !p.UserID.Equal(uid) {
		t.Fatalf("auth = %+v %v", p, err)
	}
	rows, err := tokens.ListByUser(ctx, uid, model.TokenKindOAuth)
	if err != nil || len(rows) != 1 || rows[0].GrantID == nil || !rows[0].GrantID.Equal(grantID) {
		t.Fatalf("rows = %+v %v", rows, err)
	}

	// The expiry is fixed at issue time; use slides it for sessions only.
	clk.now = clk.now.Add(30 * time.Minute)
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Fatalf("token must still be live at 30m: %v", err)
	}
	clk.now = authT0.Add(61 * time.Minute)
	if _, err := svc.Authenticate(ctx, raw); err == nil {
		t.Fatal("oauth token must expire after 1h")
	}
}

func TestIssueOAuthAccessToken_FencedByGeneration(t *testing.T) {
	svc, _, _, uid := newAuthEnv(t)
	ctx := context.Background()
	gen, err := svc.CredentialsGeneration(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminChangePassword(ctx, oauthTestEmail, "n3w-Secret!"); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := svc.IssueOAuthAccessToken(ctx, uid, vo.NewId(), "Claude", gen, time.Hour)
	if err != nil || ok || raw != "" {
		t.Fatalf("stale generation must mint nothing, raw=%q ok=%v err=%v", raw, ok, err)
	}
}

func TestLockForOAuth_ReturnsGenerationAndNotFound(t *testing.T) {
	svc, _, _, uid := newAuthEnv(t)
	ctx := context.Background()
	want, err := svc.CredentialsGeneration(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.LockForOAuth(ctx, uid)
	if err != nil || got != want {
		t.Fatalf("LockForOAuth = %d %v, want %d", got, err, want)
	}
	if _, err := svc.LockForOAuth(ctx, vo.NewId()); err == nil {
		t.Fatal("unknown user must fail")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestRevokeOAuthGrantTokens(t *testing.T) {
	svc, _, _, uid := newAuthEnv(t)
	ctx := context.Background()
	grantID, otherGrant := vo.NewId(), vo.NewId()
	raw := issueOAuth(t, svc, uid, grantID)
	other := issueOAuth(t, svc, uid, otherGrant)

	if err := svc.RevokeOAuthGrantTokens(ctx, grantID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, raw); err == nil {
		t.Fatal("revoked grant token still authenticates")
	}
	if _, err := svc.Authenticate(ctx, other); err != nil {
		t.Fatalf("another grant's token must survive: %v", err)
	}
}

func TestReclaimAndDeactivate_RevokeOAuthCredentials(t *testing.T) {
	svc, _, _, uid := newAuthEnv(t)
	ctx := context.Background()
	rev := &fakeGrantRevoker{}
	svc.SetMCPGrantRevoker(rev)
	raw := issueOAuth(t, svc, uid, vo.NewId())

	if err := svc.AdminChangePassword(ctx, oauthTestEmail, "n3w-Secret!"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, raw); err == nil {
		t.Fatal("reclaim must revoke oauth tokens")
	}
	if len(rev.users) != 1 || !rev.users[0].Equal(uid) {
		t.Fatalf("reclaim must revoke grants, got %v", rev.users)
	}

	raw = issueOAuth(t, svc, uid, vo.NewId())
	if err := svc.AdminDeactivate(ctx, oauthTestEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, raw); err == nil {
		t.Fatal("deactivate must revoke oauth tokens")
	}
	if len(rev.users) != 2 {
		t.Fatalf("deactivate must revoke grants, got %v", rev.users)
	}
}

func TestUpdatePassword_KeepsOAuthCredentials(t *testing.T) {
	svc, tokens, _, uid := newAuthEnv(t)
	ctx := context.Background()
	rev := &fakeGrantRevoker{}
	svc.SetMCPGrantRevoker(rev)
	exp := authT0.Add(appuser.SessionTTL)
	sess := seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_oauth-keep", &exp)
	raw := issueOAuth(t, svc, uid, vo.NewId())

	if _, err := svc.UpdatePassword(ctx, uid, sess, model.UpdatePasswordRequest{
		OldPassword: "secretpass", NewPassword: "next-secret",
	}); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Fatalf("update-password must keep oauth tokens: %v", err)
	}
	if len(rev.users) != 0 {
		t.Fatal("update-password must keep grants")
	}
}
