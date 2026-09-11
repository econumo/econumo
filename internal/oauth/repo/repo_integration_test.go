package repo

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestIdentityRepo(t *testing.T) {
	db := dbtest.New(t)
	uid := vo.MustParseId(fixture.New(t, db).User(fixture.User{}))
	r := NewIdentityRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	if _, err := r.GetByProviderSubject(ctx, "google", "s1"); err == nil {
		t.Fatal("want not found")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want *errs.NotFoundError, got %T", err)
	}
	id := model.NewIdentity(r.NextIdentity(), uid, "google", "s1", "a@example.test", now)
	if err := r.Save(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByProviderSubject(ctx, "google", "s1")
	if err != nil || !got.UserID.Equal(uid) || got.Email != "a@example.test" {
		t.Fatalf("%+v %v", got, err)
	}
	id.UpdateEmail("b@example.test", now.Add(time.Minute))
	if err := r.Save(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByUserProvider(ctx, uid, "google")
	if got.Email != "b@example.test" || !got.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("upsert must refresh email/updated_at: %+v", got)
	}
	if n, _ := r.CountByUser(ctx, uid); n != 1 {
		t.Fatalf("count %d", n)
	}
	list, _ := r.ListByUser(ctx, uid)
	if len(list) != 1 || list[0].Provider != "google" {
		t.Fatalf("list %+v", list)
	}
	if n, err := r.DeleteByUserProvider(ctx, uid, "google"); err != nil || n != 1 {
		t.Fatalf("delete %d %v", n, err)
	}
	if n, _ := r.DeleteByUserProvider(ctx, uid, "google"); n != 0 {
		t.Fatal("second delete affects nothing")
	}
}

func TestStateAndHandoffRepos(t *testing.T) {
	db := dbtest.New(t)
	uid := vo.MustParseId(fixture.New(t, db).User(fixture.User{}))
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	states := NewStateRepo(db.Engine, db.TX)
	st := &model.OAuthState{StateHash: "h1", Provider: "oidc", Nonce: "n", CodeVerifier: "v", FlowHash: "fh1", Client: "web", Intent: "link",
		LinkUserID: uid, CreatedAt: now, ExpiresAt: now.Add(model.OAuthStateTTL)}
	if err := states.Insert(ctx, st); err != nil {
		t.Fatal(err)
	}
	got, err := states.Get(ctx, "h1")
	if err != nil || !got.LinkUserID.Equal(uid) || got.Intent != "link" || got.CodeVerifier != "v" || got.FlowHash != "fh1" {
		t.Fatalf("%+v %v", got, err)
	}
	st2 := &model.OAuthState{StateHash: "h2", Provider: "google", Nonce: "n", Client: "app", Intent: "login",
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-50 * time.Minute)}
	if err := states.Insert(ctx, st2); err != nil {
		t.Fatal(err)
	}
	got2, _ := states.Get(ctx, "h2")
	if !got2.LinkUserID.IsZero() {
		t.Fatal("login state has no link user")
	}
	if n, _ := states.DeleteExpired(ctx, now); n != 1 {
		t.Fatalf("expired purge %d", n)
	}
	if n, err := states.Delete(ctx, "h1"); err != nil || n != 1 {
		t.Fatalf("first delete %d %v", n, err)
	}
	if _, err := states.Get(ctx, "h1"); err == nil {
		t.Fatal("deleted state must be gone")
	}
	if n, err := states.Delete(ctx, "h1"); err != nil || n != 0 {
		t.Fatalf("second delete must affect no rows: %d %v", n, err)
	}

	handoffs := NewHandoffRepo(db.Engine, db.TX)
	tok := "id.tok"
	h := &model.OAuthHandoff{CodeHash: "c1", UserID: uid, Provider: "oidc", FlowHash: "fh1", IDToken: &tok, CreatedAt: now, ExpiresAt: now.Add(model.OAuthHandoffTTL)}
	if err := handoffs.Insert(ctx, h); err != nil {
		t.Fatal(err)
	}
	hg, err := handoffs.Get(ctx, "c1")
	if err != nil || hg.IDToken == nil || *hg.IDToken != tok || !hg.UserID.Equal(uid) || hg.FlowHash != "fh1" {
		t.Fatalf("%+v %v", hg, err)
	}
	if n, err := handoffs.Delete(ctx, "c1"); err != nil || n != 1 {
		t.Fatalf("first delete %d %v", n, err)
	}
	if _, err := handoffs.Get(ctx, "c1"); err == nil {
		t.Fatal("deleted handoff must be gone")
	}
	if n, err := handoffs.Delete(ctx, "c1"); err != nil || n != 0 {
		t.Fatalf("second delete must affect no rows: %d %v", n, err)
	}
	old := &model.OAuthHandoff{CodeHash: "c2", UserID: uid, Provider: "google", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Hour)}
	_ = handoffs.Insert(ctx, old)
	if n, _ := handoffs.DeleteExpired(ctx, now); n != 1 {
		t.Fatalf("expired purge %d", n)
	}
}
