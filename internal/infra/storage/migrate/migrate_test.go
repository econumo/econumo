package migrate

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// openMemory opens a fresh in-memory SQLite database. Each call to this DSN
// shares one connection pool but a distinct in-memory database per test.
func openMemory(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// cache=shared keeps the in-memory DB alive as long as a connection is
	// open; pin to a single connection so the schema persists across queries.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func appliedSet(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	got, err := appliedVersions(context.Background(), db)
	if err != nil {
		t.Fatalf("read applied versions: %v", err)
	}
	return got
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("check table %q: %v", name, err)
	}
	return n > 0
}

func TestRun_FreshDatabase_AppliesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	migs := []Migration{
		{Version: "0001_init", SQL: `CREATE TABLE widget (id TEXT PRIMARY KEY, name TEXT NOT NULL);`},
		{Version: "0002_add_gadget", SQL: `
			CREATE TABLE gadget (id TEXT PRIMARY KEY);
			INSERT INTO gadget (id) VALUES ('a');
		`},
	}

	if err := Run(ctx, db, migs); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	applied := appliedSet(t, db)
	if !applied["0001_init"] || !applied["0002_add_gadget"] {
		t.Fatalf("expected both migrations applied, got %v", applied)
	}
	if !tableExists(t, db, "widget") || !tableExists(t, db, "gadget") {
		t.Fatal("expected widget and gadget tables to exist")
	}

	// Idempotent: a second Run must not error and must not re-execute SQL
	// (re-running CREATE TABLE would fail; INSERT would duplicate).
	if err := Run(ctx, db, migs); err != nil {
		t.Fatalf("second Run (idempotency): %v", err)
	}

	var gadgetRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gadget`).Scan(&gadgetRows); err != nil {
		t.Fatalf("count gadget: %v", err)
	}
	if gadgetRows != 1 {
		t.Fatalf("expected gadget to have 1 row after idempotent re-run, got %d", gadgetRows)
	}
}

func TestRun_UnorderedMigrationsAppliedInOrder(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	// 0002 depends on the table created by 0001; supplying them out of order
	// must still apply 0001 first.
	migs := []Migration{
		{Version: "0002_seed", SQL: `INSERT INTO thing (id) VALUES ('x');`},
		{Version: "0001_create", SQL: `CREATE TABLE thing (id TEXT PRIMARY KEY);`},
	}

	if err := Run(ctx, db, migs); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM thing`).Scan(&n); err != nil {
		t.Fatalf("count thing: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 seeded row, got %d", n)
	}
}

func TestRun_FailedMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	migs := []Migration{
		{Version: "0001_bad", SQL: `
			CREATE TABLE good (id TEXT PRIMARY KEY);
			THIS IS NOT VALID SQL;
		`},
	}

	if err := Run(ctx, db, migs); err == nil {
		t.Fatal("expected error from invalid migration SQL")
	}
	// The transaction must have rolled back: neither the table nor the
	// bookkeeping row should survive.
	if tableExists(t, db, "good") {
		t.Fatal("expected 'good' table to be rolled back")
	}
	if appliedSet(t, db)["0001_bad"] {
		t.Fatal("failed migration must not be recorded as applied")
	}
}

func TestRun_LegacyDatabase_PortedMigrationsImportedNotExecuted(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	// Simulate an existing legacy-provisioned database: the legacy
	// migration_versions table records the schema-creating migration, and the
	// table it would create already exists with data.
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE migration_versions (version VARCHAR(191) PRIMARY KEY, executed_at TIMESTAMP);
		INSERT INTO migration_versions (version, executed_at) VALUES ('20210812210548', CURRENT_TIMESTAMP);
		CREATE TABLE account (id TEXT PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO account (id, name) VALUES ('acc-1', 'Wallet');
	`); err != nil {
		t.Fatalf("seed legacy database: %v", err)
	}

	// The ported migration's SQL recreates the schema. If executed against the
	// legacy DB it would fail (table exists). Run must import + mark it
	// applied without running it; a genuinely new migration still runs.
	migs := []Migration{
		{Version: "20210812210548", SQL: `CREATE TABLE account (id TEXT PRIMARY KEY, name TEXT NOT NULL);`},
		{Version: "20260101000000", SQL: `CREATE TABLE budget (id TEXT PRIMARY KEY);`},
	}

	if err := Run(ctx, db, migs); err != nil {
		t.Fatalf("Run on legacy DB: %v", err)
	}

	applied := appliedSet(t, db)
	// Legacy version imported as applied (not executed).
	if !applied["20210812210548"] {
		t.Fatal("expected legacy version to be imported as applied")
	}
	// New Go-era migration executed.
	if !applied["20260101000000"] {
		t.Fatal("expected new migration to be applied")
	}
	if !tableExists(t, db, "budget") {
		t.Fatal("expected new migration to create 'budget'")
	}

	// Existing data untouched: the ported migration's SQL was NOT executed.
	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM account WHERE id = 'acc-1'`).Scan(&name); err != nil {
		t.Fatalf("query existing account: %v", err)
	}
	if name != "Wallet" {
		t.Fatalf("expected existing data preserved, got name=%q", name)
	}

	// Re-running stays a no-op.
	if err := Run(ctx, db, migs); err != nil {
		t.Fatalf("second Run on legacy DB: %v", err)
	}
}

func TestRun_FreshDatabase_AllMigrationsRunWhenNoLegacyTable(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	// No legacy table present: nothing to import, so every
	// migration runs normally, creating the schema.
	migs := []Migration{
		{Version: "20210812210548", SQL: `CREATE TABLE account (id TEXT PRIMARY KEY, name TEXT NOT NULL);`},
		{Version: "20260101000000", SQL: `CREATE TABLE budget (id TEXT PRIMARY KEY);`},
	}

	if err := Run(ctx, db, migs); err != nil {
		t.Fatalf("Run on fresh DB: %v", err)
	}
	if !tableExists(t, db, "account") || !tableExists(t, db, "budget") {
		t.Fatal("expected all migrations to create their tables on a fresh DB")
	}
	applied := appliedSet(t, db)
	if !applied["20210812210548"] || !applied["20260101000000"] {
		t.Fatal("expected all migrations recorded as applied")
	}
}

func TestRun_LegacyTableEmpty_AllMigrationsRun(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	// Legacy table exists but is EMPTY -> nothing to import; all migrations run.
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE migration_versions (version VARCHAR(191) PRIMARY KEY);`,
	); err != nil {
		t.Fatalf("create empty legacy table: %v", err)
	}

	migs := []Migration{
		{Version: "20210812210548", SQL: `CREATE TABLE account (id TEXT PRIMARY KEY);`},
	}
	if err := Run(ctx, db, migs); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !tableExists(t, db, "account") {
		t.Fatal("expected migration to run when legacy table is empty")
	}
}

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single", `SELECT 1`, []string{"SELECT 1"}},
		{"trailing semicolon", `SELECT 1;`, []string{"SELECT 1"}},
		{"two", `SELECT 1; SELECT 2;`, []string{"SELECT 1", "SELECT 2"}},
		{"semicolon in single quote", `INSERT INTO t VALUES ('a;b'); SELECT 2`, []string{`INSERT INTO t VALUES ('a;b')`, "SELECT 2"}},
		{"escaped quote", `INSERT INTO t VALUES ('it''s; ok')`, []string{`INSERT INTO t VALUES ('it''s; ok')`}},
		{"line comment", "SELECT 1; -- a; b\nSELECT 2", []string{"SELECT 1", "-- a; b\nSELECT 2"}},
		{"block comment", `SELECT 1 /* a;b */; SELECT 2`, []string{"SELECT 1 /* a;b */", "SELECT 2"}},
		{"empty", `   ;  ;`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitStatements(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d statements %q, want %d %q", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("statement %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestValidIdentifier(t *testing.T) {
	ok := []string{"migration_versions", "_x", "Abc123"}
	bad := []string{"", "1abc", "a b", "a;b", "tbl-name", "tbl.name"}
	for _, s := range ok {
		if !validIdentifier(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range bad {
		if validIdentifier(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}
