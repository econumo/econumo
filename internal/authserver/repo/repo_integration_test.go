package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/authserver"
	authrepo "github.com/econumo/econumo/internal/authserver/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// Asserted here, not in repo.go: the authserver package tests import repo.
var _ authserver.Repository = (*authrepo.Repo)(nil)

const userA = "11111111-1111-1111-1111-111111111111"

func TestRepo_ClientCodeGrantLifecycle(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	r := authrepo.NewRepo(db.Engine, db.TX)
	fixture.New(t, db).User(fixture.User{ID: userA, Name: "u"})
	userID := vo.MustParseId(userA)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	c := &model.OAuthClient{ID: vo.NewId(), Name: "Claude", RedirectURIs: []string{"https://claude.ai/api/mcp/auth_callback"}, CreatedAt: now}
	if err := r.InsertClient(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetClient(ctx, c.ID)
	if err != nil || got.Name != "Claude" || len(got.RedirectURIs) != 1 || got.LastUsedAt != nil {
		t.Fatalf("GetClient = %+v, %v", got, err)
	}
	if _, err := r.GetClient(ctx, vo.NewId()); !errors.As(err, new(*errs.NotFoundError)) {
		t.Fatalf("missing client: want NotFound, got %v", err)
	}

	code := &model.OAuthAuthorizationCode{CodeHash: "h1", ClientID: c.ID, UserID: userID, RedirectURI: c.RedirectURIs[0],
		CodeChallenge: "ch", Resource: "https://x.test/mcp", CredentialsGeneration: 3, CreatedAt: now, ExpiresAt: now.Add(2 * time.Minute)}
	if err := r.InsertCode(ctx, code); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ConsumeCode(ctx, "h1"); err != nil || got.CredentialsGeneration != 3 || got.UserID != userID {
		t.Fatalf("consume = %+v %v", got, err)
	}
	if _, err := r.ConsumeCode(ctx, "h1"); err == nil {
		t.Fatal("second consume must be NotFound")
	}

	g := &model.OAuthGrant{ID: vo.NewId(), UserID: userID, ClientID: c.ID, RefreshTokenHash: "r1", CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(90 * 24 * time.Hour)}
	if err := r.InsertGrant(ctx, g); err != nil {
		t.Fatal(err)
	}
	if n, err := r.RotateGrant(ctx, g.ID, "r1", "r2", now, now.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("rotate = %d %v", n, err)
	}
	if n, _ := r.RotateGrant(ctx, g.ID, "r1", "r3", now, now.Add(time.Hour)); n != 0 {
		t.Fatal("stale rotate must match nothing")
	}
	byPrev, err := r.GetGrantByPrevRefreshHash(ctx, "r1")
	if err != nil || byPrev.ID != g.ID || byPrev.RotatedAt == nil {
		t.Fatalf("by prev = %+v %v", byPrev, err)
	}
	byCur, err := r.GetGrantByRefreshHash(ctx, "r2")
	if err != nil || byCur.ID != g.ID {
		t.Fatalf("by refresh = %+v %v", byCur, err)
	}
	byID, err := r.GetGrant(ctx, g.ID)
	if err != nil || byID.RefreshTokenHash != "r2" || byID.PrevRefreshTokenHash == nil || *byID.PrevRefreshTokenHash != "r1" {
		t.Fatalf("by id = %+v %v", byID, err)
	}
	if _, err := r.GetGrant(ctx, vo.NewId()); !errors.As(err, new(*errs.NotFoundError)) {
		t.Fatalf("missing grant: want NotFound, got %v", err)
	}
	list, err := r.ListUnrevokedGrants(ctx, userID)
	if err != nil || len(list) != 1 || list[0].ClientName != "Claude" || len(list[0].RedirectURIs) != 1 {
		t.Fatalf("list = %+v %v", list, err)
	}
	if n, _ := r.RevokeUserGrants(ctx, userID, now); n != 1 {
		t.Fatal("revoke user grants")
	}
	if n, _ := r.RevokeGrant(ctx, g.ID, now); n != 0 {
		t.Fatal("revoking an already revoked grant must match nothing")
	}
	if list, _ := r.ListUnrevokedGrants(ctx, userID); len(list) != 0 {
		t.Fatal("revoked grant still listed")
	}
	if n, err := r.DeleteDeadGrants(ctx, now.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("delete dead = %d %v", n, err)
	}

	stale := &model.OAuthClient{ID: vo.NewId(), Name: "x", RedirectURIs: []string{"https://a.test/cb"}, CreatedAt: now.Add(-48 * time.Hour)}
	if err := r.InsertClient(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.PurgeUnusedClients(ctx, now.Add(-24*time.Hour)); n != 1 {
		t.Fatalf("purge = %d", n)
	}
	if err := r.MarkClientUsed(ctx, c.ID, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.GetClient(ctx, c.ID); got.LastUsedAt == nil || !got.LastUsedAt.Equal(now) {
		t.Fatalf("MarkClientUsed not persisted: %+v", got)
	}

	if err := r.InsertCode(ctx, &model.OAuthAuthorizationCode{CodeHash: "old", ClientID: c.ID, UserID: userID, RedirectURI: "x",
		CodeChallenge: "c", Resource: "r", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := r.PurgeExpiredCodes(ctx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ConsumeCode(ctx, "old"); err == nil {
		t.Fatal("expired code must have been purged")
	}
}
