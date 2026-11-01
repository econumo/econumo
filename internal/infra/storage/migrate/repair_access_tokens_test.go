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
)

// The two migrations whose release order inverted their version order: the
// oauth columns shipped in v1.5.0, the access_tokens rebuild that drops them
// in v1.6.0 with an earlier version.
const (
	oauthColumnsMigration = "20260907000000"
	tableRebuildMigration = "20260901000000"
	repairMigration       = "20261031000000"
)

func sqliteMigrations(upTo string, skip ...string) []migrate.Migration {
	skipped := make(map[string]bool, len(skip))
	for _, s := range skip {
		skipped[s] = true
	}
	var out []migrate.Migration
	for _, f := range migrations.SQLite() {
		if skipped[f.Version] || (upTo != "" && f.Version > upTo) {
			continue
		}
		out = append(out, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	return out
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

// seedToken writes a session row so the repair's rebuild is exercised with data
// to carry over, not just an empty table. withScope is false before
// 20260901000000 has run, where the column does not exist yet.
func seedToken(t *testing.T, db *sql.DB, id string, withScope bool) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`,
		id, id, id+"@x.test"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	stmt := `INSERT INTO access_tokens (id, user_id, kind, token_hash, created_at, last_used_at)
		VALUES (?, ?, 'session', ?, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`
	if withScope {
		stmt = `INSERT INTO access_tokens (id, user_id, kind, token_hash, scope, created_at, last_used_at)
		VALUES (?, ?, 'session', ?, 'full', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`
	}
	if _, err := db.Exec(stmt, id, id, "hash-"+id); err != nil {
		t.Fatalf("seed token: %v", err)
	}
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

	asShippedInV15 := sqliteMigrations(oauthColumnsMigration, tableRebuildMigration)
	if err := migrate.Run(ctx, db, asShippedInV15, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("v1.5.x migrate: %v", err)
	}
	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("v1.5.x should already have both columns, got %d/2", got)
	}
	seedToken(t, db, "01a00000-0000-7000-8000-000000000001", false)

	if err := migrate.Run(ctx, db, sqliteMigrations(""), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}

	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("access_tokens lost the oauth session columns: got %d/2", got)
	}
	// The failure users saw: minting a session writes both columns.
	if _, err := db.ExecContext(ctx, `SELECT provider, id_token FROM access_tokens LIMIT 1`); err != nil {
		t.Fatalf("select provider/id_token: %v", err)
	}
	// The repair rebuilds the table, so the rows must survive it.
	var scope string
	if err := db.QueryRow(`SELECT scope FROM access_tokens WHERE token_hash = ?`,
		"hash-01a00000-0000-7000-8000-000000000001").Scan(&scope); err != nil {
		t.Fatalf("token lost in the rebuild: %v", err)
	}
	if scope != "full" {
		t.Fatalf("scope = %q, want full", scope)
	}
}

// A fresh database runs every migration in version order, so the columns are
// already present when the repair rebuilds the table: it must preserve both the
// shape and the existing rows rather than fail or drop anything.
func TestRepairIsHarmlessOnHealthyDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", "file:repair410fresh?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	// Everything up to just before the repair, then seed, then the repair.
	beforeRepair := sqliteMigrations("20261024000001")
	if err := migrate.Run(ctx, db, beforeRepair, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("pre-repair migrate: %v", err)
	}
	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("healthy database should have both columns, got %d/2", got)
	}
	id := "01a00000-0000-7000-8000-000000000002"
	seedToken(t, db, id, true)
	if _, err := db.Exec(`UPDATE access_tokens SET provider = 'google', id_token = 'tok' WHERE id = ?`, id); err != nil {
		t.Fatalf("set provider: %v", err)
	}

	if err := migrate.Run(ctx, db, sqliteMigrations(""), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("repair migrate: %v", err)
	}

	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("columns after repair: got %d/2", got)
	}
	// provider/id_token are rebuilt as NULL for everyone: the damaged databases
	// this repairs have no values to read, so the column cannot be copied.
	var provider, idToken sql.NullString
	if err := db.QueryRow(`SELECT provider, id_token FROM access_tokens WHERE id = ?`, id).Scan(&provider, &idToken); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if provider.Valid || idToken.Valid {
		t.Fatalf("provider/id_token = %v/%v, want NULL on both", provider, idToken)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM access_tokens`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("token count after rebuild = %d, want 1", n)
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
	if err := migrate.Run(context.Background(), db, sqliteMigrations(""), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}
	if len(*warnings) != 0 {
		t.Fatalf("fresh install warned: %v", *warnings)
	}
}

// The repair rebuilds access_tokens, so it must carry each row's scope across
// verbatim. An ingest PAT may only call /api/v1/import/ingest-*; silently
// widening it to full would be a privilege escalation.
func TestRepairPreservesIngestScope(t *testing.T) {
	db, err := sql.Open("sqlite", "file:scopepreserve?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	// Everything up to just before the repair.
	var before []migrate.Migration
	for _, f := range migrations.SQLite() {
		if f.Version >= "20261031000000" {
			continue
		}
		before = append(before, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(ctx, db, before, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}
	u := "01a00000-0000-7000-8000-00000000000c"
	if _, err := db.Exec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?,?,?,'U','face:sky','x','s','argon2id','2026-01-01 00:00:00','2026-01-01 00:00:00',1)`, u, u, u+"@x.test"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, scope string }{
		{"tok-ingest", "ingest"},
		{"tok-full", "full"},
	} {
		if _, err := db.Exec(`INSERT INTO access_tokens (id,user_id,kind,token_hash,scope,created_at,last_used_at)
			VALUES (?,?,'personal',?,?,'2026-01-01 00:00:00','2026-01-01 00:00:00')`,
			tc.id, u, "h-"+tc.id, tc.scope); err != nil {
			t.Fatal(err)
		}
	}

	// Now the repair.
	var all []migrate.Migration
	for _, f := range migrations.SQLite() {
		all = append(all, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(ctx, db, all, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("repair: %v", err)
	}

	for _, tc := range []struct{ id, want string }{
		{"tok-ingest", "ingest"},
		{"tok-full", "full"},
	} {
		var got string
		if err := db.QueryRow(`SELECT scope FROM access_tokens WHERE id = ?`, tc.id).Scan(&got); err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if got != tc.want {
			t.Errorf("%s scope = %q, want %q (the rebuild must not widen a token's scope)", tc.id, got, tc.want)
		}
		t.Logf("%-12s scope preserved: %q", tc.id, got)
	}
}
