package user_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

const testSalt = "0123456789abcdef" // AES-128 (16 bytes), like the seed devtool

// newUserSvc builds the user Service over a migrated SQLite DB exactly as
// server.BuildAPI does (minus the unused JWT collaborator), plus the encode/hash
// services tests assert against.
func newUserSvc(t *testing.T, db *dbtest.DB) (*appuser.Service, *auth.EncodeService, *auth.PasswordHasher) {
	t.Helper()
	enc := auth.NewEncodeService(testSalt)
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, nil, nil, appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, false)
	return svc, enc, hasher
}

func TestAdminCreateUser(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, hasher := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	id, err := svc.AdminCreateUser(ctx, "Synth Tester", "Synth@Econumo.test", "secretpass")
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	if id.String() == "" {
		t.Fatal("empty id")
	}

	// Lookup uses the lowercased-email identifier; the row must be there, active,
	// with a verifiable password and a decryptable email.
	u, err := repo.GetByIdentifier(ctx, enc.Hash("synth@econumo.test"))
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if !u.IsActive {
		t.Error("new user should be active")
	}
	if !hasher.Verify(u.Algorithm, u.Password, "secretpass", u.Salt) {
		t.Error("stored password does not verify")
	}
	email, err := enc.Decode(u.Email)
	if err != nil {
		t.Fatalf("decode email: %v", err)
	}
	if email != "Synth@Econumo.test" {
		t.Errorf("decoded email = %q, want original case preserved", email)
	}

	// Duplicate (same address, any case) -> validation error, regardless of the
	// registration gate (this is the admin path).
	_, err = svc.AdminCreateUser(ctx, "Dup", "synth@econumo.test", "x")
	if !isValidation(err) {
		t.Fatalf("duplicate create: want validation error, got %v", err)
	}
}

func TestAdminChangeEmail(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "A", "old@econumo.test", "pw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminChangeEmail(ctx, "old@econumo.test", "new@econumo.test"); err != nil {
		t.Fatalf("AdminChangeEmail: %v", err)
	}

	// Old identifier gone, new identifier resolves with the new (decryptable) email.
	if _, err := repo.GetByIdentifier(ctx, enc.Hash("old@econumo.test")); !isNotFound(err) {
		t.Errorf("old identifier should be gone, got %v", err)
	}
	u, err := repo.GetByIdentifier(ctx, enc.Hash("new@econumo.test"))
	if err != nil {
		t.Fatalf("new identifier lookup: %v", err)
	}
	if got, _ := enc.Decode(u.Email); got != "new@econumo.test" {
		t.Errorf("decoded email = %q, want new@econumo.test", got)
	}

	// Changing to an address already taken by another user -> validation error.
	if _, err := svc.AdminCreateUser(ctx, "B", "taken@econumo.test", "pw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminChangeEmail(ctx, "new@econumo.test", "taken@econumo.test"); !isValidation(err) {
		t.Fatalf("change to taken email: want validation error, got %v", err)
	}

	// Unknown source email -> not found.
	if err := svc.AdminChangeEmail(ctx, "ghost@econumo.test", "whatever@econumo.test"); !isNotFound(err) {
		t.Fatalf("change unknown email: want not-found, got %v", err)
	}
}

func TestAdminChangePassword(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, hasher := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "A", "pw@econumo.test", "oldpw"); err != nil {
		t.Fatal(err)
	}

	// Force the account to the legacy scheme so the test proves the transition.
	u, err := repo.GetByIdentifier(ctx, enc.Hash("pw@econumo.test"))
	if err != nil {
		t.Fatal(err)
	}
	legacyHash := hasher.HashSHA512("oldpw", u.Salt)
	if _, err := db.Raw.Exec(db.Rebind(`UPDATE users SET password = ?, algorithm = 'sha512' WHERE id = ?`), legacyHash, u.ID.String()); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if err := svc.AdminChangePassword(ctx, "pw@econumo.test", "brandnew"); err != nil {
		t.Fatalf("AdminChangePassword: %v", err)
	}

	u, err = repo.GetByIdentifier(ctx, enc.Hash("pw@econumo.test"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasher.Verify(u.Algorithm, u.Password, "brandnew", u.Salt) {
		t.Error("new password does not verify")
	}
	if hasher.Verify(u.Algorithm, u.Password, "oldpw", u.Salt) {
		t.Error("old password still verifies")
	}
	if u.Algorithm != model.AlgorithmArgon2id {
		t.Errorf("algorithm after admin change-password = %q, want %q", u.Algorithm, model.AlgorithmArgon2id)
	}

	if err := svc.AdminChangePassword(ctx, "ghost@econumo.test", "x"); !isNotFound(err) {
		t.Fatalf("change unknown: want not-found, got %v", err)
	}
}

func TestAdminActivateDeactivate(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	// Seed two real (crypto) users at controlled creation times.
	f := fixture.New(t, db).WithCrypto(testSalt)
	f.At(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	f.User(fixture.User{Email: "old@econumo.test"})
	f.At(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	f.User(fixture.User{Email: "recent@econumo.test"})

	// Deactivate one user by email; the other stays active.
	if err := svc.AdminDeactivate(ctx, "old@econumo.test"); err != nil {
		t.Fatalf("AdminDeactivate: %v", err)
	}
	if isActive(t, repo, enc, "old@econumo.test") {
		t.Error("old user should be deactivated")
	}
	if !isActive(t, repo, enc, "recent@econumo.test") {
		t.Error("recent user should remain active")
	}

	// Re-deactivating an already-inactive user is a no-op (still inactive, no error).
	if err := svc.AdminDeactivate(ctx, "old@econumo.test"); err != nil {
		t.Fatalf("second AdminDeactivate: %v", err)
	}
	if isActive(t, repo, enc, "old@econumo.test") {
		t.Error("old user should still be deactivated")
	}

	// Activate restores it.
	if err := svc.AdminActivate(ctx, "old@econumo.test"); err != nil {
		t.Fatalf("AdminActivate: %v", err)
	}
	if !isActive(t, repo, enc, "old@econumo.test") {
		t.Error("old user should be active again")
	}

	if err := svc.AdminActivate(ctx, "ghost@econumo.test"); !isNotFound(err) {
		t.Fatalf("activate unknown: want not-found, got %v", err)
	}
	if err := svc.AdminDeactivate(ctx, "ghost@econumo.test"); !isNotFound(err) {
		t.Fatalf("deactivate unknown: want not-found, got %v", err)
	}
}

func isActive(t *testing.T, repo *userrepo.Repo, enc *auth.EncodeService, email string) bool {
	t.Helper()
	u, err := repo.GetByIdentifier(context.Background(), enc.Hash(strings.ToLower(email)))
	if err != nil {
		t.Fatalf("lookup %s: %v", email, err)
	}
	return u.IsActive
}

func isValidation(err error) bool {
	var v *errs.ValidationError
	return errors.As(err, &v)
}

func isNotFound(err error) bool {
	var nf *errs.NotFoundError
	return errors.As(err, &nf)
}

func TestAdminCreateUserAssignsPickedAvatar(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Avatar Tester", "avatar@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	u, err := repo.GetByIdentifier(ctx, enc.Hash("avatar@econumo.test"))
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if u.Avatar != appuser.DefaultAvatar {
		t.Fatalf("Avatar = %q, want the stub picker value %q", u.Avatar, appuser.DefaultAvatar)
	}
}

func TestAdminChangeEmailKeepsAvatar(t *testing.T) {
	db := dbtest.New(t)
	svc, enc, _ := newUserSvc(t, db)
	repo := userrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Keep Avatar", "keep@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	if err := svc.AdminChangeEmail(ctx, "keep@econumo.test", "kept@econumo.test"); err != nil {
		t.Fatalf("AdminChangeEmail: %v", err)
	}
	u, err := repo.GetByIdentifier(ctx, enc.Hash("kept@econumo.test"))
	if err != nil {
		t.Fatalf("GetByIdentifier: %v", err)
	}
	if u.Avatar != appuser.DefaultAvatar {
		t.Fatalf("Avatar = %q after email change, want unchanged %q", u.Avatar, appuser.DefaultAvatar)
	}
}
