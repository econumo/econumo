package user_test

import (
	"context"
	"strings"
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
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, server.NewUserCurrencyLookup(lookup), budgets, nil, nil,
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

	// Each row is now plaintext in its normalized form, and findable by GetByEmail.
	for _, e := range emails {
		u, err := repo.GetByID(ctx, vo.MustParseId(ids[e]))
		if err != nil {
			t.Fatalf("post-migration lookup by id %s: %v", e, err)
		}
		if want := model.NormalizeEmail(e); u.Email != want {
			t.Errorf("email = %q, want plaintext %q", u.Email, want)
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
	f := fixture.New(t, db).WithCrypto(testSalt)
	f.DefaultOptions(f.User(fixture.User{Email: email, Password: password}))

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

func TestSeedAnalyticsOption(t *testing.T) {
	h := newRegisterHarness(t)
	a := h.insertUserWithoutOptions(t, "a@example.test") // raw repo insert, no analytics row
	b := h.insertUserWithoutOptions(t, "b@example.test")

	n, err := h.svc.SeedAnalyticsOption(context.Background(), false)
	if err != nil {
		t.Fatalf("SeedAnalyticsOption: %v", err)
	}
	if n != 2 {
		t.Fatalf("seeded = %d, want 2", n)
	}
	for _, id := range []vo.Id{a.ID, b.ID} {
		u, gerr := h.repo.GetByID(context.Background(), id)
		if gerr != nil {
			t.Fatalf("GetByID: %v", gerr)
		}
		if u.AnalyticsEnabled() {
			t.Fatalf("user %s: analytics enabled, want the passed-in default (off)", id)
		}
	}

	// Idempotent: a rerun writes nothing and leaves values alone.
	again, err := h.svc.SeedAnalyticsOption(context.Background(), false)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if again != 0 {
		t.Fatalf("rerun seeded = %d, want 0", again)
	}
}

// countingRepo wraps the real repo to count calls to the whole-aggregate
// path (GetByID+Save). The boot-time seeding sweep must not use it: on a
// large instance, a GetByID+Save per user (user row plus every one of that
// user's other option rows, ~6 statements) run synchronously before the
// listener binds does not scale.
type countingRepo struct {
	*userrepo.Repo
	getByIDCalls int
	saveCalls    int
}

func (r *countingRepo) GetByID(ctx context.Context, id vo.Id) (*model.User, error) {
	r.getByIDCalls++
	return r.Repo.GetByID(ctx, id)
}

func (r *countingRepo) Save(ctx context.Context, u *model.User) error {
	r.saveCalls++
	return r.Repo.Save(ctx, u)
}

func TestSeedAnalyticsOptionWritesOnlyTheOptionRow(t *testing.T) {
	db := dbtest.New(t)
	base := userrepo.NewRepo(db.Engine, db.TX)
	spy := &countingRepo{Repo: base}
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	svc := appuser.NewService(spy, db.TX, enc, hasher, tokens, server.NewUserCurrencyLookup(lookup), budgets, nil, nil,
		userrepo.NewEmailVerificationRepo(db.Engine, db.TX), nil,
		userrepo.NewEmailChangeRequestRepo(db.Engine, db.TX), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, true, 0, false)

	ctx := context.Background()
	now := clock.New().Now()
	id := base.NextIdentity()
	u := model.NewUser(id, "seed@example.test", "Seed User", "face:fuchsia", "hash", "salt", now)
	// Seeded through the base repo directly, bypassing the spy's counters, so
	// only SeedAnalyticsOption's own repo calls are measured below.
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error { return base.Save(ctx, u) }); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	n, err := svc.SeedAnalyticsOption(ctx, true)
	if err != nil {
		t.Fatalf("SeedAnalyticsOption: %v", err)
	}
	if n != 1 {
		t.Fatalf("seeded = %d, want 1", n)
	}
	if spy.getByIDCalls != 0 {
		t.Errorf("GetByID called %d times, want 0 — the migration must not hydrate the full user aggregate", spy.getByIDCalls)
	}
	if spy.saveCalls != 0 {
		t.Errorf("Save called %d times, want 0 — the migration must write only the option row via UpsertOption", spy.saveCalls)
	}

	got, gerr := base.GetByID(ctx, id)
	if gerr != nil {
		t.Fatalf("GetByID: %v", gerr)
	}
	if !got.AnalyticsEnabled() {
		t.Error("analytics option not written correctly by UpsertOption")
	}
}

func TestNormalizeEmails(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()
	seed := func(id, email, createdAt string) {
		t.Helper()
		if _, err := db.Raw.Exec(db.Rebind(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?,?,?,'U','','x','',?,?)`),
			id, id, email, createdAt, createdAt); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
	}
	ciphertext, err := auth.NewEncodeService(testSalt).Encode("Encrypted@Example.test")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	add := func(id, email, createdAt, after string) {
		t.Helper()
		seed(id, email, createdAt)
		want[id] = after
	}
	add("01a00000-0000-7000-8000-0000000000a1", "JÖRG@example.test", "2026-01-01 00:00:00", "JÖRG@example.test")
	add("01a00000-0000-7000-8000-0000000000a2", "jörg@example.test", "2026-02-01 00:00:00", "jörg@example.test")
	add("01a00000-0000-7000-8000-0000000000a3", "Üö@example.test", "2026-02-01 00:00:00", "Üö@example.test")
	add("01a00000-0000-7000-8000-0000000000a4", "ÜÖ@example.test", "2026-01-01 00:00:00", "üö@example.test")
	add("01a00000-0000-7000-8000-0000000000a5", " Plain@Example.test", "2026-03-01 00:00:00", "plain@example.test")
	add("01a00000-0000-7000-8000-0000000000a6", ciphertext, "2026-03-01 00:00:00", ciphertext)
	// user:create does not check the format, so plaintext without an @ can exist.
	add("01a00000-0000-7000-8000-0000000000a7", "Admin", "2026-03-01 00:00:00", "admin")

	for run, wantChanged := range []int{3, 0} {
		n, err := svc.NormalizeEmails(ctx)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if n != wantChanged {
			t.Fatalf("run %d changed %d, want %d", run, n, wantChanged)
		}
		for id, after := range want {
			var got string
			if err := db.Raw.QueryRow(db.Rebind(`SELECT email FROM users WHERE id = ?`), id).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != after {
				t.Fatalf("email of %s = %q, want %q", id, got, after)
			}
		}
	}
}

// Rerunning repairs a colliding account once the address it wanted is free.
func TestNormalizeEmails_ACollidingAccountIsNormalizedOnceTheHolderMoves(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()
	f := fixture.New(t, db)
	holder := f.User(fixture.User{Email: "jörg@example.test"})
	other := f.User(fixture.User{Email: "JÖRG@example.test"})
	if _, err := svc.NormalizeEmails(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminChangeEmail(ctx, "jörg@example.test", "moved@example.test"); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.NormalizeEmails(ctx); err != nil || n != 1 {
		t.Fatalf("rerun: %d, %v", n, err)
	}
	u, err := userrepo.NewRepo(db.Engine, db.TX).GetByEmail(ctx, "JöRG@example.test")
	if err != nil || u.ID.String() != other {
		t.Fatalf("lookup: %v, %v (holder %s)", u, err, holder)
	}
}

func TestMigrateRemoveDataSalt_AddressesThatNormalizeAlikeDoNotStopTheSweep(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()
	f := fixture.New(t, db).WithCrypto(testSalt)
	older := f.User(fixture.User{Email: "JÖRG@example.test"})
	newer := f.User(fixture.User{Email: "jörg@example.test"})
	setUserCreatedAt(t, db, older, "2026-01-01 00:00:00")
	setUserCreatedAt(t, db, newer, "2026-02-01 00:00:00")

	migrated, _, err := svc.MigrateRemoveDataSalt(ctx, testSalt)
	if err != nil {
		t.Fatalf("MigrateRemoveDataSalt: %v", err)
	}
	if migrated != 2 {
		t.Fatalf("migrated = %d, want 2", migrated)
	}
	repo := userrepo.NewRepo(db.Engine, db.TX)
	for email, id := range map[string]string{"JÖRG@example.test": older, "jörg@example.test": newer} {
		u, err := repo.GetByEmail(ctx, email)
		if err != nil {
			t.Fatalf("GetByEmail(%s): %v", email, err)
		}
		if u.ID.String() != id || u.Email != email {
			t.Fatalf("%s resolved to %s (%s), want its own account %s", email, u.ID, u.Email, id)
		}
	}
}

func TestMigrateRemoveDataSalt_StoresTheNormalizedForm(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()
	id := fixture.New(t, db).WithCrypto(testSalt).User(fixture.User{Email: "JÖRG@Example.test"})
	if _, _, err := svc.MigrateRemoveDataSalt(ctx, testSalt); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.Raw.QueryRow(db.Rebind(`SELECT email FROM users WHERE id = ?`), id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "jörg@example.test" {
		t.Fatalf("email = %q", got)
	}
}

// A database can hold both kinds of row. A plaintext address already normalized has no
// other spelling to fall back to, so it keeps its place over an older encrypted one.
func TestMigrateRemoveDataSalt_AnAddressAlreadyNormalizedKeepsItsPlace(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()
	older := fixture.New(t, db).WithCrypto(testSalt).User(fixture.User{Email: "JÖRG@example.test"})
	newer := fixture.New(t, db).User(fixture.User{Email: "jörg@example.test"})
	setUserCreatedAt(t, db, older, "2026-01-01 00:00:00")
	setUserCreatedAt(t, db, newer, "2026-02-01 00:00:00")

	if _, _, err := svc.MigrateRemoveDataSalt(ctx, testSalt); err != nil {
		t.Fatalf("MigrateRemoveDataSalt: %v", err)
	}
	for id, want := range map[string]string{older: "JÖRG@example.test", newer: "jörg@example.test"} {
		var got string
		if err := db.Raw.QueryRow(db.Rebind(`SELECT email FROM users WHERE id = ?`), id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("email of %s = %q, want %q", id, got, want)
		}
	}
}

// A user locked out of a salted database can register the same address again. Two
// rows with identical bytes cannot be told apart by any lookup, so the operator
// decides, and nothing is half-done until then.
func TestMigrateRemoveDataSalt_RefusesTwoAccountsWithTheSameAddress(t *testing.T) {
	db := dbtest.New(t)
	svc, _, _ := newUserSvc(t, db)
	ctx := context.Background()
	const email = "same@example.test"
	encrypted := fixture.New(t, db).WithCrypto(testSalt).User(fixture.User{Email: email})
	plain := fixture.New(t, db).User(fixture.User{Email: email})

	_, _, err := svc.MigrateRemoveDataSalt(ctx, testSalt)
	if err == nil {
		t.Fatal("the sweep must refuse two accounts with one address")
	}
	if !strings.Contains(err.Error(), encrypted) || !strings.Contains(err.Error(), plain) {
		t.Fatalf("error %q must name both accounts", err)
	}
	var stored string
	if err := db.Raw.QueryRow(db.Rebind(`SELECT email FROM users WHERE id = ?`), encrypted).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == email {
		t.Fatal("the refused sweep left a decrypted row behind")
	}

	if err := svc.AdminChangeEmail(ctx, email, "moved@example.test"); err != nil {
		t.Fatalf("moving the plaintext account: %v", err)
	}
	if _, _, err := svc.MigrateRemoveDataSalt(ctx, testSalt); err != nil {
		t.Fatalf("rerun after the move: %v", err)
	}
	u, err := userrepo.NewRepo(db.Engine, db.TX).GetByEmail(ctx, email)
	if err != nil || u.ID.String() != encrypted {
		t.Fatalf("the address must resolve to the decrypted account: %v, %v", u, err)
	}
}

func setUserCreatedAt(t *testing.T, db *dbtest.DB, id, at string) {
	t.Helper()
	if _, err := db.Raw.Exec(db.Rebind(`UPDATE users SET created_at = '`+at+`' WHERE id = ?`), id); err != nil {
		t.Fatal(err)
	}
}
