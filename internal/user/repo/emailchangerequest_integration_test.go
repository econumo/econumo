package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/user/repo"
)

func TestEmailChangeRequestRepoLifecycle(t *testing.T) {
	db := dbtest.New(t)
	users := repo.NewRepo(db.Engine, db.TX)
	r := repo.NewEmailChangeRequestRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	u := model.NewUser(users.NextIdentity(), "cipher", "EC", "face:blue", "hash", "salt", now)
	if err := users.Save(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("GetByUser on empty table must return NotFound")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFoundError, got %v", err)
	}

	cr := model.NewEmailChangeRequest(vo.NewId(), u.ID, "new-one@example.test", "hash-one", now)
	if rows, err := r.Save(ctx, cr, u.CredentialsGeneration); err != nil {
		t.Fatalf("Save: %v", err)
	} else if rows != 1 {
		t.Fatalf("Save wrote %d rows, want 1", rows)
	}
	got, err := r.GetByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if got.NewEmail != "new-one@example.test" || got.Code != "hash-one" || !got.ExpiredAt.Equal(now.Add(10*time.Minute)) {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.IsExpired(now.Add(9 * time.Minute)) {
		t.Error("code must be valid inside the TTL")
	}
	if !got.IsExpired(now.Add(11 * time.Minute)) {
		t.Error("code must expire after the TTL")
	}

	// A second Save for the same user without DeleteByUser first violates the
	// UNIQUE(user_id) constraint - the app layer always deletes-then-saves in
	// one tx (the replace pattern), never updates in place.
	if _, err := r.Save(ctx, model.NewEmailChangeRequest(vo.NewId(), u.ID, "new-two@example.test", "hash-two", now), u.CredentialsGeneration); err == nil {
		t.Fatal("Save without DeleteByUser must violate UNIQUE(user_id)")
	}

	// Replace pattern: delete old, insert fresh (unique user_id).
	if err := r.DeleteByUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if _, err := r.Save(ctx, model.NewEmailChangeRequest(vo.NewId(), u.ID, "new-two@example.test", "hash-two", now), u.CredentialsGeneration); err != nil {
		t.Fatalf("Save replacement: %v", err)
	}
	got, err = r.GetByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByUser after replace: %v", err)
	}
	if got.NewEmail != "new-two@example.test" || got.Code != "hash-two" {
		t.Errorf("NewEmail/Code = %q/%q, want new-two@example.test/hash-two", got.NewEmail, got.Code)
	}
}

// Save is fenced on the user's credentials generation and Consume is
// row-counted: together they are how the change-email flow survives a
// concurrent account reclaim.
func TestEmailChangeRequestRepoFenceAndConsume(t *testing.T) {
	db := dbtest.New(t)
	users := repo.NewRepo(db.Engine, db.TX)
	r := repo.NewEmailChangeRequestRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	u := model.NewUser(users.NextIdentity(), "cipher", "EC", "face:blue", "hash", "salt", now)
	if err := users.Save(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}

	stale := model.NewEmailChangeRequest(vo.NewId(), u.ID, "attacker@example.test", "hash-stale", now)
	rows, err := r.Save(ctx, stale, u.CredentialsGeneration+1)
	if err != nil {
		t.Fatalf("Save under a stale generation: %v", err)
	}
	if rows != 0 {
		t.Fatalf("Save under a stale generation wrote %d rows, want 0", rows)
	}
	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("a fenced-out Save must leave no pending row")
	}

	cr := model.NewEmailChangeRequest(vo.NewId(), u.ID, "new@example.test", "hash-one", now)
	if _, err := r.Save(ctx, cr, u.CredentialsGeneration); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if taken, cerr := r.Consume(ctx, cr.ID, u.ID); cerr != nil || taken != 1 {
		t.Fatalf("Consume = %d, %v; want 1, nil", taken, cerr)
	}
	if taken, cerr := r.Consume(ctx, cr.ID, u.ID); cerr != nil || taken != 0 {
		t.Fatalf("second Consume = %d, %v; want 0, nil", taken, cerr)
	}
	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("the consumed row must be gone")
	}
}
