package user_test

import (
	"context"
	"strings"
	"testing"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	userbudgetrepo "github.com/econumo/econumo/internal/infra/repo/userbudget"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	"github.com/econumo/econumo/pkg/jwt"
)

// newSaltFreeUserSvc builds a user Service with an EMPTY data salt — the state
// the deployment is in AFTER ECONUMO_DATA_SALT is removed. Login then computes
// the identifier as md5(lower(email)) and treats the email column as plaintext.
func newSaltFreeUserSvc(t *testing.T, db *dbtest.DB) (*appuser.Service, *auth.PasswordHasher) {
	t.Helper()
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo("sqlite", db.TX)
	lookup := currencyrepo.New("sqlite", db.TX)
	budgets := userbudgetrepo.New("sqlite", db.TX)
	// Login issues a JWT; reuse the embedded test keypair.
	priv, pub := testkeys.Paths(t)
	jwtSvc, err := jwt.New(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("NewJWT: %v", err)
	}
	svc := appuser.NewService(repo, db.TX, enc, hasher, jwtSvc, lookup, budgets, nil, nil, clock.New(), false)
	return svc, hasher
}

func TestMigrateRemoveDataSalt(t *testing.T) {
	db := dbtest.NewSQLite(t)
	svc, enc, _ := newUserSvc(t, db) // service still holds the OLD salt (testSalt)
	repo := userrepo.NewRepo("sqlite", db.TX)
	ctx := context.Background()

	// Seed encrypted users (real crypto with the SAME salt the service uses).
	emails := []string{"Alice@Econumo.test", "bob@econumo.test"}
	f := fixture.New(t, db).WithCrypto(testSalt)
	for _, e := range emails {
		f.User(fixture.User{Email: e})
	}

	// Sanity: emails are stored encrypted (not equal to plaintext) before migration.
	saltFree := auth.NewEncodeService("")
	for _, e := range emails {
		u, err := repo.GetByIdentifier(ctx, enc.Hash(strings.ToLower(e)))
		if err != nil {
			t.Fatalf("pre-migration lookup %s: %v", e, err)
		}
		if u.Email() == e {
			t.Fatalf("email %q stored in plaintext before migration", e)
		}
	}

	migrated, skipped, err := svc.MigrateRemoveDataSalt(ctx, testSalt)
	if err != nil {
		t.Fatalf("MigrateRemoveDataSalt: %v", err)
	}
	if migrated != len(emails) || skipped != 0 {
		t.Fatalf("migrated=%d skipped=%d, want %d/0", migrated, skipped, len(emails))
	}

	// Each row is now plaintext, looked up by the UNSALTED identifier.
	for _, e := range emails {
		u, err := repo.GetByIdentifier(ctx, saltFree.Hash(strings.ToLower(e)))
		if err != nil {
			t.Fatalf("post-migration lookup %s: %v", e, err)
		}
		if u.Email() != e {
			t.Errorf("email = %q, want plaintext %q (case preserved)", u.Email(), e)
		}
		if u.Identifier() != saltFree.Hash(strings.ToLower(e)) {
			t.Errorf("identifier for %s not the unsalted md5", e)
		}
		// The old (salted) identifier must no longer resolve.
		if _, err := repo.GetByIdentifier(ctx, enc.Hash(strings.ToLower(e))); err == nil {
			t.Errorf("old salted identifier for %s still resolves", e)
		}
	}

	// Idempotent: a second run with the same salt decrypts nothing new.
	migrated2, skipped2, err := svc.MigrateRemoveDataSalt(ctx, testSalt)
	if err != nil {
		t.Fatalf("second MigrateRemoveDataSalt: %v", err)
	}
	if migrated2 != 0 || skipped2 != len(emails) {
		t.Fatalf("second run migrated=%d skipped=%d, want 0/%d", migrated2, skipped2, len(emails))
	}
}

// TestMigrateThenLoginSaltFree is the end-to-end contract: after migration a
// salt-free service (ECONUMO_DATA_SALT unset) can log the user in.
func TestMigrateThenLoginSaltFree(t *testing.T) {
	db := dbtest.NewSQLite(t)
	saltedSvc, _, _ := newUserSvc(t, db) // holds testSalt, used only to migrate
	ctx := context.Background()

	const email = "Carol@Econumo.test"
	const password = "secret-pw" // the fixture default
	fixture.New(t, db).WithCrypto(testSalt).User(fixture.User{Email: email, Password: password})

	if _, _, err := saltedSvc.MigrateRemoveDataSalt(ctx, testSalt); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Now authenticate with a SALT-FREE service (post-removal state).
	saltFreeSvc, _ := newSaltFreeUserSvc(t, db)
	res, err := saltFreeSvc.Login(ctx, appuser.LoginRequest{Username: email, Password: password}, clock.New().Now())
	if err != nil {
		t.Fatalf("login after migration (salt removed): %v", err)
	}
	if res.Token == "" {
		t.Error("login returned an empty token")
	}
}

// TestMigrateRemoveDataSaltEmptySaltRefused guards the catastrophic case: with an
// empty salt Decode is a passthrough, so the sweep would store ciphertext AS
// plaintext. The method must refuse rather than corrupt the data.
func TestMigrateRemoveDataSaltEmptySaltRefused(t *testing.T) {
	db := dbtest.NewSQLite(t)
	svc, _, _ := newUserSvc(t, db)
	if _, _, err := svc.MigrateRemoveDataSalt(context.Background(), ""); err == nil {
		t.Fatal("expected an error for an empty salt, got nil")
	}
}
