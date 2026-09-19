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

func TestEmailVerificationRepoLifecycle(t *testing.T) {
	db := dbtest.New(t)
	users := repo.NewRepo(db.Engine, db.TX)
	r := repo.NewEmailVerificationRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	u := model.NewUser(users.NextIdentity(), "cipher", "EV", "face:blue", "hash", "salt", now)
	if err := users.Save(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("GetByUser on empty table must return NotFound")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFoundError, got %v", err)
	}

	ev := model.NewEmailVerification(vo.NewId(), u.ID, "hash-one", now)
	if err := r.Save(ctx, ev); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := r.GetByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if got.Code != "hash-one" || !got.ExpiredAt.Equal(now.Add(10*time.Minute)) {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.IsExpired(now.Add(9 * time.Minute)) {
		t.Error("code must be valid inside the TTL")
	}
	if !got.IsExpired(now.Add(11 * time.Minute)) {
		t.Error("code must expire after the TTL")
	}

	// Replace pattern: delete old, insert fresh (unique user_id).
	if err := r.DeleteByUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if err := r.Save(ctx, model.NewEmailVerification(vo.NewId(), u.ID, "hash-two", now)); err != nil {
		t.Fatalf("Save replacement: %v", err)
	}
	got, err = r.GetByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByUser after replace: %v", err)
	}
	if got.Code != "hash-two" {
		t.Errorf("Code = %q, want hash-two", got.Code)
	}
}

// Consume is row-counted and scoped to (id, user): it is how a confirmation
// learns that the code it checked was replaced by a resend, or already taken
// by a concurrent confirmation.
func TestEmailVerificationRepoConsumeIsRowCounted(t *testing.T) {
	db := dbtest.New(t)
	users := repo.NewRepo(db.Engine, db.TX)
	r := repo.NewEmailVerificationRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	u := model.NewUser(users.NextIdentity(), "cipher", "EV", "face:blue", "hash", "salt", now)
	if err := users.Save(ctx, u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	ev := model.NewEmailVerification(vo.NewId(), u.ID, "hash-one", now)
	if err := r.Save(ctx, ev); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Another user's id must never take this row.
	other := model.NewUser(users.NextIdentity(), "cipher2", "EV2", "face:red", "hash", "salt", now)
	if err := users.Save(ctx, other); err != nil {
		t.Fatalf("save other user: %v", err)
	}
	if taken, cerr := r.Consume(ctx, ev.ID, other.ID); cerr != nil || taken != 0 {
		t.Fatalf("Consume with a foreign user = %d, %v; want 0, nil", taken, cerr)
	}
	if taken, cerr := r.Consume(ctx, ev.ID, u.ID); cerr != nil || taken != 1 {
		t.Fatalf("Consume = %d, %v; want 1, nil", taken, cerr)
	}
	if taken, cerr := r.Consume(ctx, ev.ID, u.ID); cerr != nil || taken != 0 {
		t.Fatalf("second Consume = %d, %v; want 0, nil", taken, cerr)
	}
	if _, err := r.GetByUser(ctx, u.ID); err == nil {
		t.Fatal("the consumed row must be gone")
	}
}
