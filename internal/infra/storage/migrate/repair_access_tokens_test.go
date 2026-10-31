package migrate_test

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/infra/storage/sqlite"
)

// The two migrations whose release order inverted their version order: the
// oauth columns shipped in v1.5.0, the access_tokens rebuild that drops them
// in v1.6.0 with an earlier version.
const (
	oauthColumnsMigration = "20260907000000"
	tableRebuildMigration = "20260901000000"
)

// repairCommands dispatches the repair step the way serve does, so the boot
// runner's own ordering decides when it runs.
func repairCommands(t *testing.T, db *sql.DB) migrate.CommandRunner {
	t.Helper()
	return func(ctx context.Context, name string) error {
		if name != "migration:repair-access-tokens" {
			return nil
		}
		_, err := sqlite.RepairAccessTokensOAuthColumns(ctx, db)
		return err
	}
}

func oauthColumnCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('access_tokens')
		WHERE name IN ('provider', 'id_token')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestUpgradeFromV15KeepsOAuthSessionColumns replays the upgrade that broke
// login on issue #410: a v1.5.x database recorded 20260907000000 but never saw
// the lower-numbered 20260901000000, so the rebuild runs afterwards and drops
// access_tokens.provider / id_token. The v1.5.x set is reconstructed by
// omitting exactly the migration that had not shipped yet — filtering by
// version instead would include it and hide the bug.
func TestUpgradeFromV15KeepsOAuthSessionColumns(t *testing.T) {
	db, err := sql.Open("sqlite", "file:repair410?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	var asShippedInV15 []migrate.Migration
	for _, f := range migrations.SQLite() {
		if f.Version > oauthColumnsMigration || f.Version == tableRebuildMigration {
			continue
		}
		asShippedInV15 = append(asShippedInV15, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(ctx, db, asShippedInV15, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("v1.5.x migrate: %v", err)
	}
	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("v1.5.x should already have both columns, got %d/2", got)
	}

	var current []migrate.Migration
	for _, f := range migrations.SQLite() {
		current = append(current, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(ctx, db, current, migrate.WithCommandRunner(repairCommands(t, db))); err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}

	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("access_tokens lost the oauth session columns: got %d/2", got)
	}
	// The failure users saw: minting a session writes both columns.
	if _, err := db.ExecContext(ctx, `SELECT provider, id_token FROM access_tokens LIMIT 1`); err != nil {
		t.Fatalf("select provider/id_token: %v", err)
	}
}

// A fresh database runs both migrations in version order, so the columns are
// present and the repair must leave them alone rather than fail on a duplicate.
func TestRepairIsNoOpOnFreshDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", "file:repair410fresh?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	var current []migrate.Migration
	for _, f := range migrations.SQLite() {
		current = append(current, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(ctx, db, current, migrate.WithCommandRunner(repairCommands(t, db))); err != nil {
		t.Fatalf("fresh migrate: %v", err)
	}

	added, err := sqlite.RepairAccessTokensOAuthColumns(ctx, db)
	if err != nil {
		t.Fatalf("repair on a healthy database: %v", err)
	}
	if added != 0 {
		t.Fatalf("repair added %d column(s) to a healthy database, want 0", added)
	}
	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("columns after repair: got %d/2", got)
	}
}

// captureWarnings installs a slog handler that records migrate's out-of-order
// warnings for the duration of the test.
func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(&recordHandler{msgs: &got}))
	return &got
}

type recordHandler struct{ msgs *[]string }

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		var version string
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "version" {
				version = a.Value.String()
			}
			return true
		})
		*h.msgs = append(*h.msgs, r.Message+" "+version)
	}
	return nil
}
func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func TestRunWarnsOnOutOfOrderMigration(t *testing.T) {
	db, err := sql.Open("sqlite", "file:outoforder?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	first := []migrate.Migration{{Version: "20260102000000", SQL: `CREATE TABLE a (id TEXT)`}}
	if err := migrate.Run(ctx, db, first, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}

	warnings := captureWarnings(t)
	// 20260101000000 is numbered below the recorded 20260102000000: exactly the
	// shape that dropped the columns in issue #410.
	late := append(first, migrate.Migration{Version: "20260101000000", SQL: `CREATE TABLE b (id TEXT)`})
	if err := migrate.Run(ctx, db, late, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}
	if len(*warnings) != 1 || !strings.Contains((*warnings)[0], "20260101000000") {
		t.Fatalf("want one warning naming 20260101000000, got %v", *warnings)
	}

	// In order from here on: nothing more to warn about.
	*warnings = nil
	inOrder := append(late, migrate.Migration{Version: "20260103000000", SQL: `CREATE TABLE c (id TEXT)`})
	if err := migrate.Run(ctx, db, inOrder, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}
	if len(*warnings) != 0 {
		t.Fatalf("in-order migration warned: %v", *warnings)
	}
}

// A fresh database applies everything in one pass, so the real migration set
// must not warn — the warning has to mean "this instance upgraded oddly".
func TestFreshInstallDoesNotWarn(t *testing.T) {
	db, err := sql.Open("sqlite", "file:outoforderfresh?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	warnings := captureWarnings(t)
	var current []migrate.Migration
	for _, f := range migrations.SQLite() {
		current = append(current, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(context.Background(), db, current, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}
	if len(*warnings) != 0 {
		t.Fatalf("fresh install warned: %v", *warnings)
	}
}
