package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/econumo/econumo/internal/infra/auth"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	appuser "github.com/econumo/econumo/internal/user"

	_ "modernc.org/sqlite" // register the pure-Go sqlite driver for tests

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"

	// Blank-import the sqlite backend so its init() registers it with
	// backend.Get, the same way cmd/econumo does. Without this, newContainer's
	// backend.Get(cfg.DatabaseDriver) fails even though the migrated file DB
	// this package's tests build is fine.
	"github.com/econumo/econumo/internal/infra/storage/sqlite"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// cliEnv points the container at an isolated sqlite DB + JWT dir. Unlike the
// server, the CLI container does NOT migrate on open (see container.go's
// doc comment: "it assumes an already-migrated database"), so this helper
// migrates the fresh file itself before any command touches it — the same way
// dbtest does for repo/app tests, just against a file DB instead of in-memory.
func cliEnv(t *testing.T) { cliEnvDB(t) }

// cliEnvDB is cliEnv for tests that also inspect the rows a command wrote: it
// returns the path of the migrated database the container will open.
func cliEnvDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	// The production opener, so the migrated file holds exactly what serve
	// would write (frozen datetime layout, foreign keys on).
	raw, err := sqlite.New().Open(context.Background(), "sqlite://"+dbPath)
	if err != nil {
		t.Fatalf("cliEnv: open sqlite: %v", err)
	}
	migs := migrations.SQLite()
	runnerMigs := make([]migrate.Migration, len(migs))
	for i, m := range migs {
		runnerMigs[i] = migrate.Migration{Version: m.Version, SQL: m.SQL, Command: m.Command}
	}
	if err := migrate.Run(context.Background(), raw, runnerMigs, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("cliEnv: migrate sqlite: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("cliEnv: close migration connection: %v", err)
	}

	t.Setenv("DATABASE_URL", "sqlite://"+dbPath)
	return dbPath
}

// TestUserCommandLifecycle drives the user management commands end to end
// against a fresh, migrated sqlite file: create, reject a duplicate, change
// password/email, activate, and reject an unknown user. Run's documented exit
// codes are 0 (success), 1 (command/runtime error), 2 (usage error) — every
// step below expects 0 or 1 (usage errors are covered by cli_test.go).
func TestUserCommandLifecycle(t *testing.T) {
	cliEnv(t)
	steps := []struct {
		name string
		args []string
		want int
	}{
		{"create", []string{"user:create", "Test User", "cli@example.test", "secret-pw"}, 0},
		{"create-duplicate", []string{"user:create", "Test User", "cli@example.test", "secret-pw"}, 1},
		{"change-password", []string{"user:change-password", "cli@example.test", "new-pw"}, 0},
		{"change-email", []string{"user:change-email", "cli@example.test", "cli2@example.test"}, 0},
		{"activate", []string{"user:activate", "cli2@example.test"}, 0},
		{"deactivate", []string{"user:deactivate", "cli2@example.test"}, 0},
		{"change-password-unknown", []string{"user:change-password", "nobody@example.test", "x"}, 1},
		{"activate-unknown", []string{"user:activate", "nobody@example.test"}, 1},
		{"change-email-unknown", []string{"user:change-email", "nobody@example.test", "x@example.test"}, 1},
	}
	for _, s := range steps {
		if got := Run(s.args); got != s.want {
			t.Fatalf("%s: Run(%v) = %d, want %d", s.name, s.args, got, s.want)
		}
	}
}

// TestUserCreateUsageError covers a command's own arg-count validation
// (usageErr). It is a plain error returned from cmd.run, so Run reports it
// via the same exit-1 path as any other command failure; exit 2 is reserved
// for Run's own dispatch failures (no args / unknown command), covered by
// cli_test.go's TestRunUsagePaths.
func TestUserCreateUsageError(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"user:create", "only-one-arg"}); got != 1 {
		t.Fatalf("user:create with wrong arity = %d, want 1", got)
	}
}

func TestCurrencyAdd(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"currency:add", "XTS", "Test Currency", "2"}); got != 0 {
		t.Fatalf("currency:add = %d, want 0", got)
	}
	// Re-adding the same currency is a success no-op (created=false path),
	// not an error.
	if got := Run([]string{"currency:add", "XTS", "Test Currency", "2"}); got != 0 {
		t.Fatalf("currency:add (already exists) = %d, want 0", got)
	}
}

// TestCurrencyAddUsageError covers the arg-count usage-error branch.
// Without a foreign currency in use there is nothing to load, so neither
// command reaches Open Exchange Rates.
func TestCurrencyRateGapCommands(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"currency:show-rate-gaps"}); got != 0 {
		t.Fatalf("currency:show-rate-gaps = %d, want 0", got)
	}
	if got := Run([]string{"currency:show-rate-gaps", "2026-01", "--daily"}); got != 0 {
		t.Fatalf("currency:show-rate-gaps --daily = %d, want 0", got)
	}
	if got := Run([]string{"currency:backfill-rates", "2026-01"}); got != 0 {
		t.Fatalf("currency:backfill-rates = %d, want 0", got)
	}
	// The start is required: every gap costs an API request.
	if got := Run([]string{"currency:backfill-rates", "--daily"}); got != 1 {
		t.Fatalf("currency:backfill-rates without a start = %d, want 1", got)
	}
	if got := Run([]string{"currency:show-rate-gaps", "--monthly"}); got != 1 {
		t.Fatalf("currency:show-rate-gaps --monthly = %d, want 1", got)
	}
}

func TestCurrencyAddUsageError(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"currency:add"}); got != 1 {
		t.Fatalf("currency:add (no args) = %d, want 1", got)
	}
	if got := Run([]string{"currency:add", "A", "B", "C", "D"}); got != 1 {
		t.Fatalf("currency:add (too many args) = %d, want 1", got)
	}
}

// TestCurrencyAddInvalidFractionDigits covers the strconv.Atoi error branch.
func TestCurrencyAddInvalidFractionDigits(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"currency:add", "XTS", "Test Currency", "not-a-number"}); got != 1 {
		t.Fatalf("currency:add invalid fraction-digits = %d, want 1", got)
	}
}

func TestDataRemoveSaltRefusesEmptySalt(t *testing.T) {
	cliEnv(t)
	t.Setenv("ECONUMO_DATA_SALT", "")
	if got := Run([]string{"data:remove-salt"}); got == 0 {
		t.Error("data:remove-salt must refuse to run with an empty salt")
	}
}

// TestDataRemoveSaltNoUsers exercises the actual migration path (non-empty
// salt, nothing to migrate) so MigrateRemoveDataSalt's success branch is
// covered too, not just the guard.
func TestDataRemoveSaltNoUsers(t *testing.T) {
	cliEnv(t)
	t.Setenv("ECONUMO_DATA_SALT", "some-legacy-salt")
	if got := Run([]string{"data:remove-salt"}); got != 0 {
		t.Fatalf("data:remove-salt with no users = %d, want 0", got)
	}
}

// TestQuietFlag mirrors main.go's actual flow for management commands: the
// binary strips -v/-vv/-vvv/-q via cli.ConfigureLogging BEFORE calling
// cli.Run — Run itself has no flag parsing and treats a leading "-q" as an
// unknown command (exit 2). So the flag must be stripped first, same as
// cmd/econumo does.
func TestQuietFlag(t *testing.T) {
	cliEnv(t)
	args := ConfigureLogging([]string{"-q", "user:create", "Quiet User", "quiet@example.test", "secret-pw"})
	if got := Run(args); got != 0 {
		t.Fatalf("-q user:create = %d, want 0", got)
	}
}

// TestUserDeactivate covers user:deactivate's success path: a single <email>
// positional (no flags), matching the CLI doc list in CLAUDE.md.
func TestUserDeactivate(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"user:create", "Old User", "old@example.test", "secret-pw"}); got != 0 {
		t.Fatalf("user:create = %d, want 0", got)
	}
	if got := Run([]string{"user:deactivate", "old@example.test"}); got != 0 {
		t.Fatalf("user:deactivate = %d, want 0", got)
	}
}

// The MCP grant's refresh token outlives the access tokens the cascade sweeps,
// so the CLI container must install the grant revoker for both commands.
func TestUserDeactivate_RevokesMCPGrants(t *testing.T) {
	cases := []struct {
		name string
		args func(email string) []string
	}{
		{"deactivate", func(email string) []string { return []string{"user:deactivate", email} }},
		{"change-password", func(email string) []string { return []string{"user:change-password", email, "operator-set-pw"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := cliEnvDB(t)
			const email = "mcp-user@example.test"
			if got := Run([]string{"user:create", "MCP User", email, "secret-pw"}); got != 0 {
				t.Fatalf("user:create = %d, want 0", got)
			}

			ctx := context.Background()
			raw, err := sqlite.New().Open(ctx, "sqlite://"+dbPath)
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			defer raw.Close()
			var uid string
			if err := raw.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&uid); err != nil {
				t.Fatalf("user id: %v", err)
			}
			f := fixture.New(t, &dbtest.DB{Raw: raw, Engine: "sqlite"})
			clientID := f.OAuthClient(fixture.OAuthClient{Name: "Claude", RedirectURIs: []string{"http://127.0.0.1:33418/callback"}, OwnerUserID: uid})
			grantID := f.OAuthGrant(fixture.OAuthGrant{UserID: uid, ClientID: clientID, RefreshHash: "refresh-hash-" + tc.name})
			tokenID := f.AccessToken(fixture.AccessToken{UserID: uid, Kind: "oauth", Scope: "mcp", TokenHash: "oat-hash-" + tc.name})
			if _, err := raw.ExecContext(ctx, `UPDATE access_tokens SET grant_id = ? WHERE id = ?`, grantID, tokenID); err != nil {
				t.Fatalf("link token to grant: %v", err)
			}

			if got := Run(tc.args(email)); got != 0 {
				t.Fatalf("%v = %d, want 0", tc.args(email), got)
			}

			var revokedAt sql.NullString
			if err := raw.QueryRowContext(ctx, `SELECT revoked_at FROM oauth_grants WHERE id = ?`, grantID).Scan(&revokedAt); err != nil {
				t.Fatalf("read grant: %v", err)
			}
			if !revokedAt.Valid || revokedAt.String == "" {
				t.Fatalf("oauth_grants.revoked_at is NULL after %s; the MCP client can still refresh", tc.name)
			}
		})
	}
}

// TestUnknownDatabaseDriver covers Run's container-open error path (exit 1)
// when DATABASE_URL points at an unregistered engine scheme.
func TestUnknownDatabaseDriver(t *testing.T) {
	cliEnv(t)
	t.Setenv("DATABASE_URL", "mysql://unused")
	if got := Run([]string{"user:create", "X", "x@example.test", "pw"}); got != 1 {
		t.Fatalf("Run with unsupported DATABASE_URL scheme = %d, want 1", got)
	}
}

// TestMissingDatabaseURL covers config.Load's validation error path, also
// surfaced as exit 1 by Run's container-build failure branch.
func TestMissingDatabaseURL(t *testing.T) {
	cliEnv(t)
	t.Setenv("DATABASE_URL", "")
	if got := Run([]string{"user:create", "X", "x@example.test", "pw"}); got != 1 {
		t.Fatalf("Run with empty DATABASE_URL = %d, want 1", got)
	}
}

func TestUserSetAccessAndShowExitCodes(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"user:create", "Access User", "access@example.test", "secret-pw"}); got != 0 {
		t.Fatalf("user:create = %d, want 0", got)
	}

	steps := []struct {
		name string
		args []string
		want int
	}{
		{"set-readonly", []string{"user:set-access", "access@example.test", "readonly"}, 0},
		{"set-full-no-date", []string{"user:set-access", "access@example.test", "full"}, 0},
		{"set-full-with-date", []string{"user:set-access", "access@example.test", "full", "2027-01-01"}, 0},
		{"show", []string{"user:show", "access@example.test"}, 0},
		{"unknown-level", []string{"user:set-access", "access@example.test", "pro"}, 1},
		{"bad-date", []string{"user:set-access", "access@example.test", "full", "01-01-2027"}, 1},
		{"unknown-email", []string{"user:set-access", "nobody@example.test", "full"}, 1},
		{"show-unknown-email", []string{"user:show", "nobody@example.test"}, 1},
		{"set-access-too-few-args", []string{"user:set-access", "access@example.test"}, 1},
		{"show-too-many-args", []string{"user:show", "a@example.test", "b"}, 1},
		{"verify-email", []string{"user:verify-email", "access@example.test"}, 0},
		{"verify-email-unknown", []string{"user:verify-email", "nobody@example.test"}, 1},
		{"verify-email-too-few-args", []string{"user:verify-email"}, 1},
	}
	for _, s := range steps {
		if got := Run(s.args); got != s.want {
			t.Fatalf("%s: Run(%v) = %d, want %d", s.name, s.args, got, s.want)
		}
	}
}

// TestMigrationZeroDeletedAccounts_CommandAndRunner drives the CLI command end
// to end (a deleted account holding a non-zero balance gets a correction, and
// a second run is a no-op), then exercises MigrationCommandRunner's dispatch
// rules directly: it accepts only migration:* names and shares the caller's
// db without closing it. Ids are real UUIDs (vo.NewId), not the brief's
// literal 'u1'/'acc-del': the command lists accounts through the repo, whose
// hydrateAccount calls vo.ParseId (strict uuid.Parse), so a non-UUID id would
// fail there rather than exercise the behavior under test.
func TestMigrationZeroDeletedAccounts_CommandAndRunner(t *testing.T) {
	cliEnv(t)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()

	userID := vo.NewId().String()
	accountID := vo.NewId().String()
	txID := vo.NewId().String()

	// seed straight through the container's db: a deleted account holding +5
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := c.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?,?,?,'U','','x','','2026-01-01 00:00:00','2026-01-01 00:00:00')`, userID, userID, userID+"@e.test")
	mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?,'dffc2a06-6f29-4704-8575-31709adee926',?,'A',2,'wallet',1,'2026-01-01 00:00:00','2026-02-01 00:00:00')`, accountID, userID)
	mustExec(`INSERT INTO transactions (id, user_id, account_id, description, type, amount, spent_at, created_at, updated_at) VALUES (?,?,?,'',1,5,'2026-01-10 00:00:00','2026-01-10 00:00:00','2026-01-10 00:00:00')`, txID, userID, accountID)

	if code := Run([]string{"migration:zero-deleted-accounts"}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	var n int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE account_id = ? AND type = 0 AND amount = 5`, accountID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("corrections=%d err=%v", n, err)
	}
	// second run is a no-op
	if code := Run([]string{"migration:zero-deleted-accounts"}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE account_id = ?`, accountID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("rows after second run=%d err=%v (want 2: original + one correction)", n, err)
	}

	// the runner dispatches by name over a caller-owned db and refuses non-migration commands
	runner := MigrationCommandRunner(c.cfg, c.db)
	if err := runner(ctx, "migration:zero-deleted-accounts"); err != nil {
		t.Fatalf("runner: %v", err)
	}
	if err := runner(ctx, "user:create"); err == nil {
		t.Fatal("runner must refuse non-migration commands")
	}
	if err := runner(ctx, "migration:nope"); err == nil {
		t.Fatal("runner must fail on an unknown command")
	}
}

// TestMigrationSeedAnalyticsOption_CommandAndRunner drives
// migration:seed-analytics-option end to end: users predating the per-user
// preference (raw rows, no users_options row at all — user:create always
// seeds one, so it can't stand in for this pre-migration shape) get the
// analytics option written from the config default (ECONUMO_ANALYTICS unset
// here, so the default is "1"/enabled), and a second run writes nothing more.
// It then exercises MigrationCommandRunner's dispatch directly, mirroring
// TestMigrationZeroDeletedAccounts_CommandAndRunner.
func TestMigrationSeedAnalyticsOption_CommandAndRunner(t *testing.T) {
	cliEnv(t)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()

	userA := vo.NewId().String()
	userB := vo.NewId().String()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := c.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	insertUser := func(id string) {
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?,?,?,'U','','x','','2026-01-01 00:00:00','2026-01-01 00:00:00')`, id, id, id+"@e.test")
	}
	insertUser(userA)
	insertUser(userB)

	if code := Run([]string{"migration:seed-analytics-option"}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	var n int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users_options WHERE name = 'analytics'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("analytics rows=%d err=%v, want 2", n, err)
	}
	// ECONUMO_ANALYTICS is unset here, and config.Load defaults it to true, so
	// the seeded rows must read "1" — proving the command actually threads
	// c.cfg.Analytics into SeedAnalyticsOption rather than hardcoding a value.
	var value string
	if err := c.db.QueryRowContext(ctx, `SELECT value FROM users_options WHERE name = 'analytics' AND user_id = ?`, userA).Scan(&value); err != nil || value != "1" {
		t.Fatalf("analytics value=%q err=%v, want \"1\" (config default)", value, err)
	}
	// second run is a no-op: no new rows, existing ones untouched
	if code := Run([]string{"migration:seed-analytics-option"}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users_options WHERE name = 'analytics'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("analytics rows after second run=%d err=%v, want 2", n, err)
	}

	// the runner dispatches by name over a caller-owned db
	userC := vo.NewId().String()
	insertUser(userC)
	runner := MigrationCommandRunner(c.cfg, c.db)
	if err := runner(ctx, "migration:seed-analytics-option"); err != nil {
		t.Fatalf("runner: %v", err)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users_options WHERE name = 'analytics'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("analytics rows after runner=%d err=%v, want 3", n, err)
	}
}

// TestMigrationSeedAnalyticsOption_PassesConfigValueThrough proves the
// command reads ECONUMO_ANALYTICS (via c.cfg.Analytics) rather than a
// hardcoded default: with the variable explicitly set to false, the seeded
// rows must read "0".
func TestMigrationSeedAnalyticsOption_PassesConfigValueThrough(t *testing.T) {
	cliEnv(t)
	t.Setenv("ECONUMO_ANALYTICS", "false")
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()

	userA := vo.NewId().String()
	if _, err := c.db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?,?,?,'U','','x','','2026-01-01 00:00:00','2026-01-01 00:00:00')`, userA, userA, userA+"@e.test"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if code := Run([]string{"migration:seed-analytics-option"}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	var value string
	if err := c.db.QueryRowContext(ctx, `SELECT value FROM users_options WHERE name = 'analytics' AND user_id = ?`, userA).Scan(&value); err != nil || value != "0" {
		t.Fatalf("analytics value=%q err=%v, want \"0\" (ECONUMO_ANALYTICS=false)", value, err)
	}
}

func TestTokenPurge(t *testing.T) {
	cliEnv(t)
	if got := Run([]string{"user:create", "Purge Tester", "purge@example.test", "secretpass"}); got != 0 {
		t.Fatalf("user:create = %d, want 0", got)
	}
	// Fresh DB: nothing dead to purge, but the command must succeed with the
	// default retention and with an explicit day count.
	if got := Run([]string{"token:purge"}); got != 0 {
		t.Fatalf("token:purge = %d, want 0", got)
	}
	if got := Run([]string{"token:purge", "0"}); got != 0 {
		t.Fatalf("token:purge 0 = %d, want 0", got)
	}
	// Usage errors (extra args / non-numeric / negative days) exit 1, like
	// every in-command usage error (2 is reserved for unknown commands).
	for _, args := range [][]string{
		{"token:purge", "7", "extra"},
		{"token:purge", "soon"},
		{"token:purge", "-1"},
	} {
		if got := Run(args); got != 1 {
			t.Fatalf("Run(%v) = %d, want 1", args, got)
		}
	}
}

// The operator's user:change-password is the account reclaim, so the CLI
// container has to wire the oauth side of it: an intruder's linked identity is
// a way back into the account that the lost password does not close. Nothing
// else in the CLI needs the oauth service, which is exactly why it is easy to
// leave unwired — this test is the guard.
func TestUserChangePasswordUnlinksAForeignIdentity(t *testing.T) {
	dbPath := cliEnvDB(t)
	if got := Run([]string{"user:create", "Victim", "victim@example.test", "victim-pw"}); got != 0 {
		t.Fatalf("user:create = %d, want 0", got)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	var uid string
	if err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE lower(email) = 'victim@example.test'`).Scan(&uid); err != nil {
		t.Fatalf("user id: %v", err)
	}
	// The identity the intruder linked: their own provider account, vouching for
	// an address that is not this one.
	if _, err := db.ExecContext(ctx, `INSERT INTO users_identities
		(id, user_id, provider, issuer, subject, email, created_at, updated_at)
		VALUES (?, ?, 'google', 'https://accounts.google.com', 'squatter-sub', 'squatter@example.test', ?, ?)`,
		vo.NewId().String(), uid, "2026-01-01 00:00:00", "2026-01-01 00:00:00"); err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	if got := Run([]string{"user:change-password", "victim@example.test", "operator-set-pw"}); got != 0 {
		t.Fatalf("user:change-password = %d, want 0", got)
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users_identities WHERE user_id = ?`, uid).Scan(&n); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if n != 0 {
		t.Fatalf("%d foreign identity row(s) survived user:change-password", n)
	}
}

// TestMigrationNormalizeSQLiteDatetimes_CommandAndRunner drives the command
// end to end: a value in the driver's old time.Time.String() form is rewritten
// to the frozen layout, a second run is a no-op, and the boot runner reaches
// the command by name. The rewrite rules themselves are covered in
// internal/infra/storage/sqlite.
func TestMigrationNormalizeSQLiteDatetimes_CommandAndRunner(t *testing.T) {
	cliEnv(t)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()

	userID := vo.NewId().String()
	if _, err := c.db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at)
		VALUES (?,?,?,'U','','x','',CAST(? AS TEXT),'2026-01-01 00:00:00')`,
		userID, userID, userID+"@e.test", "2026-09-14 13:00:00.5 +0300 MSK"); err != nil {
		t.Fatal(err)
	}
	createdAt := func() string {
		t.Helper()
		var s string
		if err := c.db.QueryRowContext(ctx, `SELECT CAST(created_at AS TEXT) FROM users WHERE id = ?`, userID).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	for run := 1; run <= 2; run++ {
		if code := Run([]string{"migration:normalize-sqlite-datetimes"}); code != 0 {
			t.Fatalf("run %d: exit code %d", run, code)
		}
		if got := createdAt(); got != "2026-09-14 10:00:00" {
			t.Fatalf("run %d: created_at = %q, want 2026-09-14 10:00:00", run, got)
		}
	}

	if err := MigrationCommandRunner(c.cfg, c.db)(ctx, "migration:normalize-sqlite-datetimes"); err != nil {
		t.Fatalf("runner: %v", err)
	}
}

// TestFreshDatabaseMigratesWithCommandSteps boots an empty database the way
// serve does: SQL files and migration:* command steps interleaved by version.
// A command step runs against the schema of its own version, so any column
// the repositories it calls select must already exist by then.
func TestFreshDatabaseMigratesWithCommandSteps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "db.sqlite")
	t.Setenv("DATABASE_URL", "sqlite://"+dbPath)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()
	migs := migrations.SQLite()
	runnerMigs := make([]migrate.Migration, len(migs))
	for i, m := range migs {
		runnerMigs[i] = migrate.Migration{Version: m.Version, SQL: m.SQL, Command: m.Command}
	}
	if err := migrate.Run(ctx, c.db, runnerMigs, migrate.WithCommandRunner(MigrationCommandRunner(c.cfg, c.db))); err != nil {
		t.Fatalf("fresh boot migration: %v", err)
	}
}

// TestPreMembershipDatabaseMigratesWithCommandSteps boots a database last
// migrated by v1.3.x (before 20260817000000) that holds a deleted account with
// a balance. Command steps run today's code, so the zeroing step must come
// after every migration its correction write depends on; at 20260817000001 it
// failed on the sort key columns 20261024000000 adds.
func TestPreMembershipDatabaseMigratesWithCommandSteps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "db.sqlite")
	t.Setenv("DATABASE_URL", "sqlite://"+dbPath)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()
	var all, released []migrate.Migration
	for _, m := range migrations.SQLite() {
		mig := migrate.Migration{Version: m.Version, SQL: m.SQL, Command: m.Command}
		all = append(all, mig)
		if m.Version < "20260817000000" {
			released = append(released, mig)
		}
	}
	if err := migrate.Run(ctx, c.db, released, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("v1.3 schema: %v", err)
	}
	userID, accountID := vo.NewId().String(), vo.NewId().String()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?,?,?,'U','','x','','2026-01-01 00:00:00','2026-01-01 00:00:00')`, []any{userID, userID, userID + "@e.test"}},
		{`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?,'dffc2a06-6f29-4704-8575-31709adee926',?,'A',2,'wallet',1,'2026-01-01 00:00:00','2026-02-01 00:00:00')`, []any{accountID, userID}},
		{`INSERT INTO transactions (id, user_id, account_id, description, type, amount, spent_at, created_at, updated_at) VALUES (?,?,?,'',1,5,'2026-01-10 00:00:00','2026-01-10 00:00:00','2026-01-10 00:00:00')`, []any{vo.NewId().String(), userID, accountID}},
	} {
		if _, err := c.db.ExecContext(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
	if err := migrate.Run(ctx, c.db, all, migrate.WithCommandRunner(MigrationCommandRunner(c.cfg, c.db))); err != nil {
		t.Fatalf("upgrade boot migration: %v", err)
	}
	var n int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE account_id = ? AND type = 0 AND amount = 5 AND sort_key IS NOT NULL`, accountID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("zeroing corrections with a sort key = %d (err %v), want 1", n, err)
	}
}

// TestEmailNormalizingUpgrade_CollidingAccountsDoNotStopTheBoot upgrades a database
// holding pairs SQLite's lower() let through: failing on them would take the instance
// down. Encrypted rows must come through untouched for data:remove-salt to read.
func TestEmailNormalizingUpgrade_CollidingAccountsDoNotStopTheBoot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "db.sqlite")
	t.Setenv("DATABASE_URL", "sqlite://"+dbPath)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()
	var all, released []migrate.Migration
	for _, m := range migrations.SQLite() {
		mig := migrate.Migration{Version: m.Version, SQL: m.SQL, Command: m.Command}
		all = append(all, mig)
		if m.Version < "20261103000000" {
			released = append(released, mig)
		}
	}
	if err := migrate.Run(ctx, c.db, released, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("released schema: %v", err)
	}
	ciphertext, err := auth.NewEncodeService("0123456789abcdef").Encode("Encrypted@Example.test")
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ id, email, createdAt, want string }
	rows := []row{
		{vo.NewId().String(), "JÖRG@example.test", "2026-01-01 00:00:00", "JÖRG@example.test"},
		{vo.NewId().String(), "jörg@example.test", "2026-02-01 00:00:00", "jörg@example.test"},
		{vo.NewId().String(), "ÜÖ@example.test", "2026-01-01 00:00:00", "üö@example.test"},
		{vo.NewId().String(), "Üö@example.test", "2026-02-01 00:00:00", "Üö@example.test"},
		{vo.NewId().String(), " Plain@Example.test", "2026-03-01 00:00:00", "plain@example.test"},
		{vo.NewId().String(), ciphertext, "2026-03-01 00:00:00", ciphertext},
	}
	for _, u := range rows {
		if _, err := c.db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?,?,?,'U','','x','',?,?)`,
			u.id, u.id, u.email, u.createdAt, u.createdAt); err != nil {
			t.Fatalf("seed %s: %v", u.email, err)
		}
	}
	if err := migrate.Run(ctx, c.db, all, migrate.WithCommandRunner(MigrationCommandRunner(c.cfg, c.db))); err != nil {
		t.Fatalf("upgrade boot migration: %v", err)
	}
	assertEmails := func() {
		t.Helper()
		for _, u := range rows {
			var got string
			if err := c.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, u.id).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != u.want {
				t.Fatalf("%q became %q, want %q", u.email, got, u.want)
			}
		}
	}
	assertEmails()
	if err := MigrationCommandRunner(c.cfg, c.db)(ctx, "migration:normalize-emails"); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	assertEmails()
}

// TestMigrationCopyAccessTokens_CommandAndRunner drives step 2 of the
// access_tokens repair by name: with access_tokens_new pending it copies every
// row and a second run replaces rather than duplicates them; once the tables
// are swapped it has nothing to do.
func TestMigrationCopyAccessTokens_CommandAndRunner(t *testing.T) {
	cliEnv(t)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()

	// The container's database is fully migrated, so the swap has happened.
	if code := Run([]string{"migration:copy-access-tokens"}); code != 0 {
		t.Fatalf("after the swap: exit code %d", code)
	}

	u := "01a00000-0000-7000-8000-0000000000c1"
	if _, err := c.db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?, ?, 'copy@x.test', 'U', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`, u, u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := c.db.ExecContext(ctx, `INSERT INTO access_tokens (id, user_id, kind, token_hash, scope, created_at, last_used_at, provider)
		VALUES ('tok-1', ?, 'session', 'h-1', 'full', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 'oidc')`, u); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	var step1 string
	for _, f := range migrations.SQLite() {
		if f.Version == "20261031000010" {
			step1 = f.SQL
		}
	}
	if _, err := c.db.ExecContext(ctx, step1); err != nil {
		t.Fatalf("create access_tokens_new: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if code := Run([]string{"migration:copy-access-tokens"}); code != 0 {
			t.Fatalf("run %d: exit code %d", run, code)
		}
		var n int
		var provider string
		if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*), MAX(provider) FROM access_tokens_new`).Scan(&n, &provider); err != nil {
			t.Fatal(err)
		}
		if n != 1 || provider != "oidc" {
			t.Fatalf("run %d: access_tokens_new holds %d row(s), provider %q; want 1, oidc", run, n, provider)
		}
	}

	if err := MigrationCommandRunner(c.cfg, c.db)(ctx, "migration:copy-access-tokens"); err != nil {
		t.Fatalf("runner: %v", err)
	}
}

func deletionTestDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	raw, err := sqlite.New().Open(context.Background(), "sqlite://"+dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return raw
}

func userIDByEmail(t *testing.T, raw *sql.DB, email string) string {
	t.Helper()
	var id string
	if err := raw.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id); err != nil {
		t.Fatalf("user id of %s: %v", email, err)
	}
	return id
}

func userExists(t *testing.T, raw *sql.DB, id string) bool {
	t.Helper()
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n == 1
}

func markUser(t *testing.T, raw *sql.DB, id, reason string, at time.Time) {
	t.Helper()
	if _, err := raw.Exec(`UPDATE users SET deletion_marked_at = ?, deletion_reason = ? WHERE id = ?`,
		at.UTC().Format(datetime.Layout), reason, id); err != nil {
		t.Fatalf("mark user: %v", err)
	}
}

func TestUserDeletionCommands(t *testing.T) {
	dbPath := cliEnvDB(t)
	t.Setenv("ECONUMO_USER_DELETION_DAYS", "")
	const email, unmarked = "del-a@example.test", "del-unmarked@example.test"
	for _, e := range []string{email, unmarked} {
		if code, _, stderr := runApp(t, "user:create", "Del User", e, "secret-pw"); code != 0 {
			t.Fatalf("user:create %s = %d: %s", e, code, stderr)
		}
	}
	raw := deletionTestDB(t, dbPath)
	id := userIDByEmail(t, raw, email)

	code, stdout, stderr := runApp(t, "user:mark-deleted", email)
	if code != 0 || !strings.Contains(stdout, "User "+email+" marked for deletion") {
		t.Fatalf("user:mark-deleted = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, _, _ := runApp(t, "user:mark-deleted", email); code != 1 {
		t.Fatalf("second user:mark-deleted = %d, want 1", code)
	}

	code, stdout, stderr = runApp(t, "user:list-pending-deletion")
	if code != 0 {
		t.Fatalf("user:list-pending-deletion = %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("list output = %q, want a header and one row", stdout)
	}
	if got := strings.Fields(lines[0]); !slices.Equal(got, []string{"ID", "EMAIL", "REASON", "MARKED", "AUTO-WIPE"}) {
		t.Errorf("header = %q", lines[0])
	}
	row := strings.Fields(lines[1])
	if len(row) != 6 || row[0] != id || row[1] != email || row[2] != "cli" || row[5] != "manual" {
		t.Errorf("row = %q, want id, email, cli, the mark time and manual", lines[1])
	}

	t.Setenv("ECONUMO_USER_DELETION_DAYS", "30")
	_, stdout, _ = runApp(t, "user:list-pending-deletion")
	lines = strings.Split(strings.TrimSpace(stdout), "\n")
	row = strings.Fields(lines[len(lines)-1])
	if len(row) != 7 {
		t.Fatalf("row with a deletion period = %q", lines[len(lines)-1])
	}
	marked, err := time.Parse(datetime.Layout, row[3]+" "+row[4])
	if err != nil {
		t.Fatalf("marked column: %v", err)
	}
	if want := marked.AddDate(0, 0, 30).Format(datetime.Layout); row[5]+" "+row[6] != want {
		t.Errorf("auto-wipe = %s %s, want %s", row[5], row[6], want)
	}

	code, stdout, stderr = runApp(t, "user:delete", email)
	if code != 0 || !strings.Contains(stdout, "User "+email+" deleted") {
		t.Fatalf("user:delete = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if userExists(t, raw, id) {
		t.Error("the deleted user's row is still there")
	}

	code, _, stderr = runApp(t, "user:delete", unmarked)
	if code != 1 || !strings.Contains(stderr, "not marked") {
		t.Fatalf("user:delete on an unmarked user = %d, stderr %q", code, stderr)
	}
	if code, _, _ := runApp(t, "user:delete", "nobody@example.test"); code != 1 {
		t.Fatalf("user:delete on an unknown user = %d, want 1", code)
	}
}

const releasedMsg = "not deleted: the inactivity mark no longer applies (a connected user is active, or inactivity marking is off); the mark was removed"

func TestUserDelete_InactiveMarkIsReleased(t *testing.T) {
	cases := []struct {
		name         string
		inactiveDays string
		activePeer   bool
		cause        string
	}{
		{"active connection", "30", true, "active_connection"},
		{"inactivity marking off", "", false, "inactivity_off"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := cliEnvDB(t)
			t.Setenv("ECONUMO_INACTIVE_USER_DAYS", tc.inactiveDays)
			raw := deletionTestDB(t, dbPath)
			f := fixture.New(t, &dbtest.DB{Raw: raw, Engine: "sqlite"})
			const email = "del-inactive@example.test"
			id := f.User(fixture.User{Email: email})
			if tc.activePeer {
				partner := f.User(fixture.User{Email: "del-partner@example.test"})
				f.Connect(id, partner)
				if _, err := raw.Exec(`UPDATE users SET last_active_at = ? WHERE id = ?`, time.Now().UTC().Format(datetime.Layout), partner); err != nil {
					t.Fatalf("stamp the partner active: %v", err)
				}
			}
			markUser(t, raw, id, "inactive", time.Now().AddDate(0, 0, -1))

			code, _, stderr := runApp(t, "user:delete", email)
			if code != 1 || !strings.Contains(stderr, releasedMsg) {
				t.Fatalf("user:delete = %d, stderr %q", code, stderr)
			}
			if !userExists(t, raw, id) {
				t.Fatal("a released user was deleted")
			}
			var markedAt sql.NullString
			if err := raw.QueryRow(`SELECT deletion_marked_at FROM users WHERE id = ?`, id).Scan(&markedAt); err != nil {
				t.Fatal(err)
			}
			if markedAt.Valid {
				t.Errorf("deletion_marked_at = %q, want the mark removed", markedAt.String)
			}
			var details string
			if err := raw.QueryRow(`SELECT details FROM users_audit WHERE user_id = ? AND event = 'deletion_cancelled'`, id).Scan(&details); err != nil {
				t.Fatalf("cancel audit row: %v", err)
			}
			if !strings.Contains(details, `"`+tc.cause+`"`) {
				t.Errorf("cancel audit details = %s, want the %s cause", details, tc.cause)
			}
		})
	}
}

func TestUserPurgeDeleted(t *testing.T) {
	dbPath := cliEnvDB(t)
	t.Setenv("ECONUMO_USER_DELETION_DAYS", "30")
	raw := deletionTestDB(t, dbPath)
	f := fixture.New(t, &dbtest.DB{Raw: raw, Engine: "sqlite"})
	due := f.User(fixture.User{Email: "purge-due@example.test"})
	recent := f.User(fixture.User{Email: "purge-recent@example.test"})
	markUser(t, raw, due, "cli", time.Now().AddDate(0, 0, -40))
	markUser(t, raw, recent, "cli", time.Now().AddDate(0, 0, -1))

	code, stdout, stderr := runApp(t, "user:purge-deleted")
	if code != 0 || !strings.Contains(stdout, "released 0, deleted 1") {
		t.Fatalf("user:purge-deleted = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if userExists(t, raw, due) || !userExists(t, raw, recent) {
		t.Fatal("want the due user deleted and the recent one kept")
	}

	failing := f.User(fixture.User{Email: "purge-failing@example.test"})
	markUser(t, raw, failing, "cli", time.Now().AddDate(0, 0, -40))
	if _, err := raw.Exec(`CREATE TRIGGER refuse_user_delete BEFORE DELETE ON users BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	code, stdout, stderr = runApp(t, "user:purge-deleted")
	if code != 1 {
		t.Fatalf("user:purge-deleted with a failing wipe = %d, want 1", code)
	}
	if !strings.Contains(stdout, "released 0, deleted 0") {
		t.Errorf("stdout = %q, want the summary", stdout)
	}
	if !regexp.MustCompile(`(?m)^failed ` + failing + `: \S+$`).MatchString(stderr) {
		t.Errorf("stderr = %q, want one failure line with the user id and the error type", stderr)
	}
	if strings.Contains(stderr, "refused") || strings.Contains(stderr, "@") {
		t.Errorf("stderr = %q carries the error text or an email", stderr)
	}
}

func TestReportPurge_PassError(t *testing.T) {
	var out, errOut bytes.Buffer
	err := reportPurge(&out, &errOut, appuser.DeletionPassResult{Err: errors.New("listing failed")})
	if err == nil {
		t.Fatal("a pass that errored must fail the command")
	}
}
