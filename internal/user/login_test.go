package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestLoginPersistsLanguage(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)

	id, err := svc.AdminCreateUser(context.Background(), "Login Language", "login-lang@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	ctx := reqctx.WithLanguage(context.Background(), "ru")
	if _, err := svc.Login(ctx, model.LoginRequest{Username: "login-lang@econumo.test", Password: "secretpass"}, "test-agent", time.Now()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	var got string
	if err := db.Raw.QueryRowContext(context.Background(), db.Rebind("SELECT language FROM users WHERE id = ?"), id.String()).Scan(&got); err != nil {
		t.Fatalf("read back language: %v", err)
	}
	if got != "ru" {
		t.Fatalf("language = %q, want ru", got)
	}
}

func TestLoginDefaultsLanguageWithoutHeader(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)

	id, err := svc.AdminCreateUser(context.Background(), "Login Default", "login-default@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	if _, err := svc.Login(context.Background(), model.LoginRequest{Username: "login-default@econumo.test", Password: "secretpass"}, "test-agent", time.Now()); err != nil {
		t.Fatalf("Login: %v", err)
	}

	var got string
	if err := db.Raw.QueryRowContext(context.Background(), db.Rebind("SELECT language FROM users WHERE id = ?"), id.String()).Scan(&got); err != nil {
		t.Fatalf("read back language: %v", err)
	}
	if got != "en" {
		t.Fatalf("language = %q, want en", got)
	}
}

// resettingRepo lands a completed password reset in the window between Login's
// evidence read and its session insert — the race the fence exists for. Armed
// once, like a single reset committing.
type resettingRepo struct {
	appuser.Repository
	armed bool
}

func (r *resettingRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u, err := r.Repository.GetByEmail(ctx, email)
	if err != nil || !r.armed {
		return u, err
	}
	r.armed = false
	return u, r.Repository.BumpCredentialsGeneration(ctx, u.ID)
}

// Login must present the generation that came out of the SAME row read as the
// password hash it verified. A reset committing after that read bumps past it,
// so the guarded insert writes nothing and the login fails closed. Reading the
// generation again later would pick up the post-reset value and hand the
// sign-in a live session the reset was supposed to deny.
func TestLogin_ResetBetweenTheEvidenceReadAndTheSessionInsertMintsNothing(t *testing.T) {
	db := dbtest.New(t)
	repo := &resettingRepo{Repository: userrepo.NewRepo(db.Engine, db.TX)}
	svc, _, _ := newUserSvcWithRepo(t, db, repo)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Raced", "raced-login@econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	repo.armed = true

	_, err = svc.Login(ctx, model.LoginRequest{Username: "raced-login@econumo.test", Password: "secretpass"}, "test-agent", time.Now())
	var unauthorized *errs.UnauthorizedError
	if !errors.As(err, &unauthorized) || unauthorized.Msg != "Invalid credentials." {
		t.Fatalf("Login err = %v, want *errs.UnauthorizedError %q", err, "Invalid credentials.")
	}
	var n int
	if err := db.Raw.QueryRowContext(ctx, db.Rebind(
		"SELECT COUNT(*) FROM access_tokens WHERE user_id = ? AND kind = ?"), id.String(), model.TokenKindSession).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 0 {
		t.Fatalf("the reset was outrun: %d session rows", n)
	}
}

// newLegacyUserService builds a Service (via newUserSvc) with one active user
// whose stored hash is forced to the legacy sha512 scheme, mirroring the
// seeding in admin_integration_test.go's TestAdminChangePassword. Returns the
// plaintext password alongside so a caller can drive the rehash path.
func newLegacyUserService(t *testing.T) (svc *appuser.Service, repo appuser.Repository, u *model.User, plaintext string) {
	t.Helper()
	db := dbtest.New(t)
	svc, _, hasher := newUserSvc(t, db)
	repo = userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	plaintext = "legacy-secret-pw"
	id, err := svc.AdminCreateUser(ctx, "Legacy", "legacy-rehash@econumo.test", plaintext)
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	u, err = repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	legacyHash := hasher.HashSHA512(plaintext, u.Salt)
	if _, err := db.Raw.Exec(db.Rebind(`UPDATE users SET password = ?, algorithm = 'sha512' WHERE id = ?`),
		legacyHash, id.String()); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	u, err = repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID after seeding legacy row: %v", err)
	}
	return svc, repo, u, plaintext
}

// TestLogin_LegacyRehashDoesNotOverwriteAConcurrentReset covers the second
// half of the reclaim race: rehashLegacyPassword runs against the row Login
// already read (its evidence), so a reset that commits a fresh argon2id
// password and bumps the generation in between must survive — the stale
// legacy aggregate's write must not resurrect argon2id(old password) over it.
func TestLogin_LegacyRehashDoesNotOverwriteAConcurrentReset(t *testing.T) {
	s, repo, u, plaintext := newLegacyUserService(t)
	ctx := context.Background()

	// Simulate the reset committing after Login read its evidence: bump the
	// fence and write a fresh argon2id hash for a DIFFERENT password.
	resetHash, err := s.HashForTest("new-password-after-reset")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	loaded, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	loaded.UpdatePassword(resetHash, model.AlgorithmArgon2id, time.Now())
	if err := repo.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	if err := repo.BumpCredentialsGeneration(ctx, u.ID); err != nil {
		t.Fatal(err)
	}

	// A stale aggregate, as Login would hold it: the pre-reset legacy hash and
	// generation, read before the reset committed.
	stale, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	stale.Password, stale.Salt, stale.Algorithm, stale.CredentialsGeneration = u.Password, u.Salt, model.AlgorithmSHA512, 0

	s.RehashLegacyPasswordForTest(ctx, stale, plaintext, time.Now())

	after, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.Password != resetHash {
		t.Fatal("the legacy rehash overwrote the password written by the reset")
	}
}
