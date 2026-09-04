package cli

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite" // register the pure-Go sqlite driver for tests

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"

	// Blank-import the sqlite backend so its init() registers it with
	// backend.Get, the same way cmd/econumo does. Without this, newContainer's
	// backend.Get(cfg.DatabaseDriver) fails even though the migrated file DB
	// this package's tests build is fine.
	_ "github.com/econumo/econumo/internal/infra/storage/sqlite"

	"github.com/econumo/econumo/internal/shared/vo"
)

// cliEnv points the container at an isolated sqlite DB + JWT dir. Unlike the
// server, the CLI container does NOT migrate on open (see container.go's
// doc comment: "it assumes an already-migrated database"), so this helper
// migrates the fresh file itself before any command touches it — the same way
// dbtest does for repo/app tests, just against a file DB instead of in-memory.
func cliEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.sqlite")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("cliEnv: open sqlite: %v", err)
	}
	if _, err := raw.ExecContext(context.Background(), "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("cliEnv: pragma foreign_keys: %v", err)
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
