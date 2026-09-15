package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// seedTokenUser inserts a user row so access_tokens FK constraints hold.
func seedTokenUser(t *testing.T, db *dbtest.DB, id string) {
	t.Helper()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	u := newTestUser(
		vo.MustParseId(id), "enc-email", "Alice", "https://av/a",
		"hash", "salt-a", true, fixedTime, fixedTime, nil,
	)
	if err := db.TX.WithTx(context.Background(), func(ctx context.Context) error { return repo.Save(ctx, u) }); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestAccessTokenRepo_RoundTrip(t *testing.T) {
	db := dbtest.New(t)
	seedTokenUser(t, db, userA)
	repo := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	exp := now.Add(30 * 24 * time.Hour)
	ua := "TestAgent/1.0"
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindSession,
		TokenHash: "hash-1", UserAgent: &ua,
		CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
	}
	if n, err := repo.InsertIfGeneration(ctx, tok, 0); err != nil || n != 1 {
		t.Fatalf("Insert: %d %v", n, err)
	}

	got, _, _, err := repo.GetByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if !got.ID.Equal(tok.ID) || got.Kind != model.TokenKindSession ||
		got.UserAgent == nil || *got.UserAgent != ua ||
		got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) || got.RevokedAt != nil {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Name != nil {
		t.Errorf("session Name must be nil, got %q", *got.Name)
	}

	if _, _, _, err := repo.GetByHash(ctx, "nope"); err == nil {
		t.Fatal("GetByHash(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetByHash(miss) = %T, want NotFound", err)
	}

	// Touch then Revoke persist the mutable lifecycle fields.
	later := now.Add(10 * time.Minute)
	got.Touch(later, 30*24*time.Hour)
	if n, err := repo.Touch(ctx, got.ID, got.LastUsedAt, got.ExpiresAt); err != nil || n != 1 {
		t.Fatalf("Touch: %d %v", n, err)
	}
	if err := repo.Revoke(ctx, got.ID, later); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got2, _, _, err := repo.GetByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("GetByHash after update: %v", err)
	}
	if !got2.LastUsedAt.Equal(later) || got2.RevokedAt == nil || !got2.RevokedAt.Equal(later) {
		t.Errorf("update not persisted: %+v", got2)
	}
	if got2.ExpiresAt == nil || !got2.ExpiresAt.Equal(later.Add(30*24*time.Hour)) {
		t.Errorf("touch did not slide expires_at: %+v", got2.ExpiresAt)
	}

	// ListByUser: a PAT (nil expiry, has name) + kind filtering.
	name := "ci token"
	pat := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindPersonal,
		TokenHash: "hash-2", Name: &name, CreatedAt: now.Add(time.Second), LastUsedAt: now.Add(time.Second),
	}
	if n, err := repo.InsertIfGeneration(ctx, pat, 0); err != nil || n != 1 {
		t.Fatalf("Insert pat: %d %v", n, err)
	}
	sessions, err := repo.ListByUser(ctx, vo.MustParseId(userA), model.TokenKindSession)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListByUser(session) = %d, %v; want 1", len(sessions), err)
	}
	pats, err := repo.ListByUser(ctx, vo.MustParseId(userA), model.TokenKindPersonal)
	if err != nil || len(pats) != 1 || pats[0].Name == nil || *pats[0].Name != name || pats[0].ExpiresAt != nil {
		t.Fatalf("ListByUser(personal) mismatch: %+v, %v", pats, err)
	}

	// GetByID round-trips; a random id is NotFound.
	byID, err := repo.GetByID(ctx, pat.ID)
	if err != nil || byID.TokenHash != "hash-2" {
		t.Fatalf("GetByID: %+v, %v", byID, err)
	}
	if _, err := repo.GetByID(ctx, vo.NewId()); err == nil {
		t.Fatal("GetByID(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetByID(miss) = %T, want NotFound", err)
	}

	// Duplicate hash violates the unique index.
	dup := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindSession,
		TokenHash: "hash-2", CreatedAt: now, LastUsedAt: now,
	}
	if _, err := repo.InsertIfGeneration(ctx, dup, 0); err == nil {
		t.Error("duplicate token_hash insert must fail")
	}

	// Delete removes the row.
	if err := repo.Delete(ctx, tok.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, _, err := repo.GetByHash(ctx, "hash-1"); err == nil {
		t.Error("deleted row still found")
	}
}

func TestAccessTokenRepo_DeleteDead(t *testing.T) {
	db := dbtest.New(t)
	seedTokenUser(t, db, userA)
	repo := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-30 * 24 * time.Hour)

	futureExp := now.Add(time.Hour)
	oldExp := cutoff.Add(-time.Hour)
	recentExp := now.Add(-time.Hour) // expired, but within retention
	oldRevoked := cutoff.Add(-time.Hour)

	insert := func(hash string, exp, revoked *time.Time) *model.AccessToken {
		tok := &model.AccessToken{
			ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindSession,
			TokenHash: hash, CreatedAt: now, LastUsedAt: now, ExpiresAt: exp, RevokedAt: revoked,
		}
		if n, err := repo.InsertIfGeneration(ctx, tok, 0); err != nil || n != 1 {
			t.Fatalf("Insert %s: %d %v", hash, n, err)
		}
		return tok
	}

	live := insert("dead-live", &futureExp, nil)
	never := insert("dead-never", nil, nil) // PAT-style: no expiry, must survive
	expiredOld := insert("dead-expired-old", &oldExp, nil)
	expiredRecent := insert("dead-expired-recent", &recentExp, nil)
	revokedOld := insert("dead-revoked-old", &futureExp, &oldRevoked)

	n, err := repo.DeleteDead(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteDead: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteDead = %d rows, want 2", n)
	}
	for _, tc := range []struct {
		tok  *model.AccessToken
		gone bool
		name string
	}{
		{live, false, "live"},
		{never, false, "never-expires"},
		{expiredRecent, false, "expired-within-retention"},
		{expiredOld, true, "expired-past-retention"},
		{revokedOld, true, "revoked-past-retention"},
	} {
		_, err := repo.GetByID(ctx, tc.tok.ID)
		gone := err != nil
		if gone != tc.gone {
			t.Errorf("%s: gone=%v, want %v", tc.name, gone, tc.gone)
		}
	}
}

func TestAccessTokenRepo_ProviderAndIDTokenRoundTrip(t *testing.T) {
	db := dbtest.New(t)
	userID := fixture.New(t, db).User(fixture.User{})
	repo := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	provider, idTok := "oidc", "eyJ.header.sig"
	exp := now.Add(time.Hour)
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userID), Kind: model.TokenKindSession,
		TokenHash: "h-provider", CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
		Provider: &provider, IDToken: &idTok,
	}
	if n, err := repo.InsertIfGeneration(context.Background(), tok, 0); err != nil || n != 1 {
		t.Fatalf("insert: %d %v", n, err)
	}
	got, err := repo.GetByID(context.Background(), tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider == nil || *got.Provider != "oidc" || got.IDToken == nil || *got.IDToken != idTok {
		t.Fatalf("provider/id_token not persisted: %+v", got)
	}
	byHash, _, _, err := repo.GetByHash(context.Background(), "h-provider")
	if err != nil || byHash.Provider != nil || byHash.IDToken != nil {
		t.Fatalf("GetByHash (the per-request auth path) must not carry provider/id_token: %+v %v", byHash, err)
	}
	list, err := repo.ListByUser(context.Background(), tok.UserID, model.TokenKindSession)
	if err != nil || len(list) != 1 || list[0].Provider == nil {
		t.Fatalf("ListByUser must carry provider: %+v %v", list, err)
	}
}

func TestGetByHash_DoesNotCarryTheIDToken(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	userID := fixture.New(t, db).User(fixture.User{})
	ctx := context.Background()
	idToken, provider := "eyJ.stub.token", "oidc"
	exp := time.Now().Add(time.Hour)
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userID), Kind: model.TokenKindSession, TokenHash: "h-hot-path",
		CreatedAt: time.Now(), LastUsedAt: time.Now(), ExpiresAt: &exp, Provider: &provider, IDToken: &idToken,
	}
	if n, err := r.InsertIfGeneration(ctx, tok, 0); err != nil || n != 1 {
		t.Fatalf("insert: %d %v", n, err)
	}
	got, _, _, err := r.GetByHash(ctx, "h-hot-path")
	if err != nil {
		t.Fatal(err)
	}
	if got.IDToken != nil {
		t.Fatal("the per-request auth path must not load id_token")
	}
	if got.Provider != nil {
		t.Fatal("the per-request auth path must not load provider")
	}
	byID, err := r.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.IDToken == nil || *byID.IDToken != idToken || byID.Provider == nil {
		t.Fatal("GetByID must still carry provider and id_token for logout")
	}
}

// A touch that lost the race to a reclaim must not write its stale
// revoked_at = NULL snapshot back and hand the caller a live credential again.
func TestTouch_DoesNotResurrectARevokedToken(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	userID := fixture.New(t, db).User(fixture.User{})
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	exp := now.Add(time.Hour)
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userID), Kind: model.TokenKindSession, TokenHash: "h-touch",
		CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
	}
	if n, err := r.InsertIfGeneration(ctx, tok, 0); err != nil || n != 1 {
		t.Fatalf("insert: %d %v", n, err)
	}
	// The request read the row (revoked_at NULL) ...
	loaded, _, _, err := r.GetByHash(ctx, "h-touch")
	if err != nil {
		t.Fatal(err)
	}
	// ... the reclaim revokes it ...
	if err := r.Revoke(ctx, tok.ID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// ... and the request's touch lands afterwards with the stale snapshot.
	later := now.Add(10 * time.Minute)
	loaded.Touch(later, 30*24*time.Hour)
	n, err := r.Touch(ctx, loaded.ID, loaded.LastUsedAt, loaded.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("touch wrote %d rows on a revoked token", n)
	}
	after, err := r.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.RevokedAt == nil {
		t.Fatal("the touch resurrected the revoked token")
	}
}

// RevokeAll sweeps one kind in a single statement, skipping the presenting
// token and leaving the other kind alone.
func TestRevokeAll_SweepsOneKindAndSkipsTheException(t *testing.T) {
	db := dbtest.New(t)
	r := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	userID := fixture.New(t, db).User(fixture.User{})
	uid := vo.MustParseId(userID)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	exp := now.Add(time.Hour)

	insert := func(hash, kind string) *model.AccessToken {
		tok := &model.AccessToken{
			ID: vo.NewId(), UserID: uid, Kind: kind, TokenHash: hash,
			CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
		}
		if n, err := r.InsertIfGeneration(ctx, tok, 0); err != nil || n != 1 {
			t.Fatalf("insert %s: %d %v", hash, n, err)
		}
		return tok
	}
	current := insert("h-current", model.TokenKindSession)
	other := insert("h-other", model.TokenKindSession)
	pat := insert("h-pat", model.TokenKindPersonal)

	if err := r.RevokeAll(ctx, uid, model.TokenKindSession, current.ID, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		tok     *model.AccessToken
		revoked bool
		name    string
	}{
		{current, false, "presenting session"},
		{other, true, "other session"},
		{pat, false, "personal token"},
	} {
		row, err := r.GetByID(ctx, tc.tok.ID)
		if err != nil {
			t.Fatal(err)
		}
		if (row.RevokedAt != nil) != tc.revoked {
			t.Errorf("%s: revoked=%v, want %v", tc.name, row.RevokedAt != nil, tc.revoked)
		}
	}

	// The zero id excepts nothing, so a second sweep takes the presenting row
	// too; already-revoked rows keep their original stamp.
	if err := r.RevokeAll(ctx, uid, model.TokenKindSession, vo.Id{}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	row, err := r.GetByID(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.RevokedAt == nil || !row.RevokedAt.Equal(now.Add(time.Hour)) {
		t.Errorf("presenting session revoked_at = %v, want %v", row.RevokedAt, now.Add(time.Hour))
	}
	if row, err := r.GetByID(ctx, other.ID); err != nil || row.RevokedAt == nil || !row.RevokedAt.Equal(now) {
		t.Errorf("an already-revoked row moved its stamp: %v %v", row.RevokedAt, err)
	}
}
