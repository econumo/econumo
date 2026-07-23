package user_test

import (
	"context"
	"testing"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// newSaltFreeUserSvc builds a user Service with an EMPTY data salt — the state
// the deployment is in AFTER ECONUMO_DATA_SALT is removed. Login then looks the
// user up by email and treats the email column as plaintext.
func newSaltFreeUserSvc(t *testing.T, db *dbtest.DB) (*appuser.Service, *auth.PasswordHasher) {
	t.Helper()
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo("sqlite", db.TX)
	lookup := currencyrepo.New("sqlite", db.TX)
	budgets := server.NewUserBudgetAccess("sqlite", db.TX)
	// Login mints an opaque session token backed by the access_tokens table.
	tokens := userrepo.NewAccessTokenRepo("sqlite", db.TX)
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, nil, nil,
		userrepo.NewEmailVerificationRepo("sqlite", db.TX), nil,
		userrepo.NewEmailChangeRequestRepo("sqlite", db.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, false, 0, false)
	return svc, hasher
}

func TestMigrateRemoveDataSalt(t *testing.T) {
	db := dbtest.NewSQLite(t)
	svc, _, _ := newUserSvc(t, db) // salt-free, like production; MigrateRemoveDataSalt takes the salt as a parameter
	repo := userrepo.NewRepo("sqlite", db.TX)
	ctx := context.Background()

	// Seed encrypted users with a genuine salt (real crypto); the harness's own
	// encoder is salt-free, so this exercises actual ciphertext. Ids are
	// captured because the email column is ciphertext pre-migration, so
	// GetByEmail cannot find the row until after the sweep.
	emails := []string{"Alice@Econumo.test", "bob@econumo.test"}
	f := fixture.New(t, db).WithCrypto(testSalt)
	ids := make(map[string]string, len(emails)) // email -> id
	for _, e := range emails {
		ids[e] = f.User(fixture.User{Email: e})
	}

	// Sanity: emails are stored encrypted (not equal to plaintext) before migration.
	for _, e := range emails {
		u, err := repo.GetByID(ctx, vo.MustParseId(ids[e]))
		if err != nil {
			t.Fatalf("pre-migration lookup %s: %v", e, err)
		}
		if u.Email == e {
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

	// Each row is now plaintext, and findable by GetByEmail (its post-migration
	// lookup key).
	for _, e := range emails {
		u, err := repo.GetByID(ctx, vo.MustParseId(ids[e]))
		if err != nil {
			t.Fatalf("post-migration lookup by id %s: %v", e, err)
		}
		if u.Email != e {
			t.Errorf("email = %q, want plaintext %q (case preserved)", u.Email, e)
		}
		byEmail, err := repo.GetByEmail(ctx, e)
		if err != nil {
			t.Fatalf("post-migration GetByEmail(%s): %v", e, err)
		}
		if byEmail.ID.String() != ids[e] {
			t.Errorf("GetByEmail(%s) = %s, want %s", e, byEmail.ID, ids[e])
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
	saltedSvc, _, _ := newUserSvc(t, db) // salt-free; MigrateRemoveDataSalt takes the salt as a parameter
	ctx := context.Background()

	const email = "Carol@Econumo.test"
	const password = "secret-pw" // the fixture default
	fixture.New(t, db).WithCrypto(testSalt).User(fixture.User{Email: email, Password: password})

	if _, _, err := saltedSvc.MigrateRemoveDataSalt(ctx, testSalt); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Now authenticate with a SALT-FREE service (post-removal state).
	saltFreeSvc, _ := newSaltFreeUserSvc(t, db)
	res, err := saltFreeSvc.Login(ctx, model.LoginRequest{Username: email, Password: password}, "migrate-test", clock.New().Now())
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
