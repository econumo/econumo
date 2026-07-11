package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// seedTokenUser inserts a user row so access_tokens FK constraints hold.
func seedTokenUser(t *testing.T, db *dbtest.DB, id string) {
	t.Helper()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	u := newTestUser(
		vo.MustParseId(id), identA, "enc-email", "Alice", "https://av/a",
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
	if err := repo.Insert(ctx, tok); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := repo.GetByHash(ctx, "hash-1")
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

	if _, err := repo.GetByHash(ctx, "nope"); err == nil {
		t.Fatal("GetByHash(miss) must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("GetByHash(miss) = %T, want NotFound", err)
	}

	// Update persists touch + revoke.
	later := now.Add(10 * time.Minute)
	got.Touch(later, 30*24*time.Hour)
	got.Revoke(later)
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got2, err := repo.GetByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("GetByHash after update: %v", err)
	}
	if !got2.LastUsedAt.Equal(later) || got2.RevokedAt == nil || !got2.RevokedAt.Equal(later) {
		t.Errorf("update not persisted: %+v", got2)
	}

	// ListByUser: a PAT (nil expiry, has name) + kind filtering.
	name := "ci token"
	pat := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userA), Kind: model.TokenKindPersonal,
		TokenHash: "hash-2", Name: &name, CreatedAt: now.Add(time.Second), LastUsedAt: now.Add(time.Second),
	}
	if err := repo.Insert(ctx, pat); err != nil {
		t.Fatalf("Insert pat: %v", err)
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
	if err := repo.Insert(ctx, dup); err == nil {
		t.Error("duplicate token_hash insert must fail")
	}

	// Delete removes the row.
	if err := repo.Delete(ctx, tok.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByHash(ctx, "hash-1"); err == nil {
		t.Error("deleted row still found")
	}
}
