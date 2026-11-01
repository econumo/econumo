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
	// The repair's first step; the copy command and the swap follow it.
	repairMigration = "20261031000000"
)

// repairCommands dispatches the copy step the way serve does, so the boot
// runner's own ordering decides when it runs.
func repairCommands(db *sql.DB) migrate.CommandRunner {
	return func(ctx context.Context, name string) error {
		if name != "migration:copy-access-tokens" {
			return nil
		}
		_, err := sqlite.CopyAccessTokens(ctx, db)
		return err
	}
}

// beforeRepair is every SQLite migration numbered below the repair's first step.
func beforeRepair() []migrate.Migration {
	var out []migrate.Migration
	for _, m := range sqliteMigrations("") {
		if m.Version < repairMigration {
			out = append(out, m)
		}
	}
	return out
}

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

// seedToken writes a session row so the upgrade is exercised with data to carry
// over, not just an empty table. withScope is false before
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

	if err := migrate.Run(ctx, db, sqliteMigrations(""), migrate.WithCommandRunner(repairCommands(db))); err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}

	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("access_tokens lost the oauth session columns: got %d/2", got)
	}
	// The failure users saw: minting a session writes both columns.
	if _, err := db.ExecContext(ctx, `SELECT provider, id_token FROM access_tokens LIMIT 1`); err != nil {
		t.Fatalf("select provider/id_token: %v", err)
	}
	// Existing sessions must keep authenticating across the upgrade.
	var scope string
	if err := db.QueryRow(`SELECT scope FROM access_tokens WHERE token_hash = ?`,
		"hash-01a00000-0000-7000-8000-000000000001").Scan(&scope); err != nil {
		t.Fatalf("token lost in the rebuild: %v", err)
	}
	if scope != "full" {
		t.Fatalf("scope = %q, want full", scope)
	}
}

// A fresh install, or one upgraded from v1.6.x, has both columns when the repair
// runs: the copy must carry the values they hold across. Clearing them
// would drop every OIDC session's RP-initiated logout on the common upgrade
// path to fix the rare one.
func TestRepairKeepsValuesOnHealthyDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", "file:repair410fresh?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	if err := migrate.Run(ctx, db, beforeRepair(), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("pre-repair migrate: %v", err)
	}
	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("healthy database should have both columns, got %d/2", got)
	}
	id := "01a00000-0000-7000-8000-000000000002"
	seedToken(t, db, id, true)
	if _, err := db.Exec(`UPDATE access_tokens SET provider = 'oidc', id_token = 'tok' WHERE id = ?`, id); err != nil {
		t.Fatalf("set provider: %v", err)
	}

	if err := migrate.Run(ctx, db, sqliteMigrations(""), migrate.WithCommandRunner(repairCommands(db))); err != nil {
		t.Fatalf("repair migrate: %v", err)
	}

	var provider, idToken sql.NullString
	if err := db.QueryRow(`SELECT provider, id_token FROM access_tokens WHERE id = ?`, id).Scan(&provider, &idToken); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if provider.String != "oidc" || idToken.String != "tok" {
		t.Fatalf("provider/id_token = %v/%v, want oidc/tok", provider, idToken)
	}
	if got := oauthColumnCount(t, db); got != 2 {
		t.Fatalf("columns after repair: got %d/2", got)
	}
}

// The copy rebuilds every row, so it must carry each token's scope across
// verbatim. An ingest PAT may only call /api/v1/import/ingest-*; silently
// widening it to full would be a privilege escalation.
func TestRepairPreservesIngestScope(t *testing.T) {
	db, err := sql.Open("sqlite", "file:scopepreserve?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	if err := migrate.Run(ctx, db, beforeRepair(), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}
	u := "01a00000-0000-7000-8000-00000000000c"
	seedToken(t, db, u, true)
	if _, err := db.Exec(`INSERT INTO access_tokens (id, user_id, kind, token_hash, scope, created_at, last_used_at)
		VALUES ('tok-ingest', ?, 'personal', 'h-ingest', 'ingest', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, u); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Run(ctx, db, sqliteMigrations(""), migrate.WithCommandRunner(repairCommands(db))); err != nil {
		t.Fatalf("repair: %v", err)
	}

	for id, want := range map[string]string{"tok-ingest": "ingest", u: "full"} {
		var got string
		if err := db.QueryRow(`SELECT scope FROM access_tokens WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got != want {
			t.Errorf("%s scope = %q, want %q", id, got, want)
		}
	}
}

// The swap drops the old table, so it must refuse to run on an incomplete
// copy. A runner that records the copy command without running it
// (migrate.NoCommands) would otherwise leave access_tokens empty and sign
// everyone out, PATs included.
func TestRepairSwapRefusesAnIncompleteCopy(t *testing.T) {
	db, err := sql.Open("sqlite", "file:swapguard?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	if err := migrate.Run(ctx, db, beforeRepair(), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatal(err)
	}
	id := "01a00000-0000-7000-8000-00000000000d"
	seedToken(t, db, id, true)

	if err := migrate.Run(ctx, db, sqliteMigrations(""), migrate.WithCommandRunner(migrate.NoCommands)); err == nil {
		t.Fatal("the swap ran over an empty copy")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM access_tokens WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("old table gone: %v", err)
	}
	if n != 1 {
		t.Fatalf("token count after the refused swap = %d, want 1", n)
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
