# data:import-sqlite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a CLI command `data:import-sqlite <sqlite-path>` that copies all data from an existing SQLite database into the configured PostgreSQL database.

**Architecture:** A new engine-agnostic copy package (`internal/infra/storage/sqliteimport`) introspects the **target** PostgreSQL `information_schema` for the table list, column types, and foreign-key topological order (nothing hardcoded, so it never drifts), then copies every app table from the source SQLite `*sql.DB` into the target `*sql.DB` inside one transaction with native type handling. A thin CLI command (`internal/cli/data_commands.go`) opens the source, runs the PostgreSQL migrations on the target (so a bare `createdb` works in one step), then calls the package and prints a per-table report.

**Tech Stack:** Go stdlib `database/sql`, the existing `modernc.org/sqlite` + `pgx` (simple protocol) drivers already linked in the binary, the project's `migrate` runner and embedded `migrations`, and the `dbtest` test harness.

## Global Constraints

- **Source SQLite is opened via the registered backend** `backend.Get("sqlite").Open(ctx, path)` — the DSN accepts a plain file path; this applies `PRAGMA foreign_keys = ON` and registers the `modernc.org/sqlite` driver. Copied for reads only.
- **Target driver name is `"postgresql"`** (value of `cfg.DatabaseDriver` for a `postgres://`/`postgresql://` URL; the pgsql backend's `Name` const is `"postgresql"`).
- **Introspection filters on `current_schema()`**, never a literal `public` — production uses `public`, tests use a private per-test schema on the search_path.
- **Excluded tables (never copied):** `messenger_messages` (dead Symfony leftover, only `BIGSERIAL` table), `schema_migrations` (owned by the migrate runner), `migration_versions` (legacy external bookkeeping).
- **PostgreSQL uses `$N` placeholders**, not `?`.
- **Comments are sparse** — only non-obvious *why* (per CLAUDE.md). No godoc that restates a signature; no references to the former PHP implementation.
- **The copy runs in one transaction** on the target; any failure rolls back so a partial import never persists.

---

### Task 1: `sqliteimport` package skeleton + pure topological sort

**Files:**
- Create: `internal/infra/storage/sqliteimport/sqliteimport.go`
- Test: `internal/infra/storage/sqliteimport/topo_test.go`

**Interfaces:**
- Consumes: nothing (leaf package; stdlib only in this task).
- Produces:
  - `var ErrTargetNotEmpty error`
  - `type TableCount struct { Name string; Rows int64 }`
  - `type Report struct { Tables []TableCount; Total int64 }`
  - `func topoSort(nodes []string, deps map[string][]string) ([]string, error)` — unexported; returns nodes ordered parents-before-children; deterministic (sorted); errors on a cycle. `deps[child]` lists the tables `child` references.

- [ ] **Step 1: Write the failing test**

```go
package sqliteimport

import (
	"reflect"
	"strings"
	"testing"
)

func TestTopoSort_ParentsBeforeChildren(t *testing.T) {
	// transactions -> accounts -> currencies; accounts -> users; transactions -> users
	nodes := []string{"transactions", "accounts", "currencies", "users"}
	deps := map[string][]string{
		"accounts":     {"currencies", "users"},
		"transactions": {"accounts", "users"},
	}
	got, err := topoSort(nodes, deps)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	pos := map[string]int{}
	for i, n := range got {
		pos[n] = i
	}
	if len(got) != len(nodes) {
		t.Fatalf("expected %d nodes, got %d (%v)", len(nodes), len(got), got)
	}
	for child, parents := range deps {
		for _, p := range parents {
			if pos[p] > pos[child] {
				t.Errorf("parent %q (%d) must precede child %q (%d): %v", p, pos[p], child, pos[child], got)
			}
		}
	}
}

func TestTopoSort_Deterministic(t *testing.T) {
	nodes := []string{"b", "a", "c"}
	got1, _ := topoSort(nodes, nil)
	got2, _ := topoSort(nodes, nil)
	if !reflect.DeepEqual(got1, got2) {
		t.Fatalf("not deterministic: %v vs %v", got1, got2)
	}
	if !reflect.DeepEqual(got1, []string{"a", "b", "c"}) {
		t.Fatalf("expected sorted order with no deps, got %v", got1)
	}
}

func TestTopoSort_CycleErrors(t *testing.T) {
	nodes := []string{"a", "b"}
	deps := map[string][]string{"a": {"b"}, "b": {"a"}}
	_, err := topoSort(nodes, deps)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestTopoSort_EdgeToUnknownNodeIgnored(t *testing.T) {
	// A parent that is not in the node set (e.g. an excluded table) is skipped.
	nodes := []string{"a"}
	deps := map[string][]string{"a": {"excluded"}}
	got, err := topoSort(nodes, deps)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("expected [a], got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/storage/sqliteimport/ -run TestTopoSort -v`
Expected: FAIL — `undefined: topoSort` (package won't compile).

- [ ] **Step 3: Write minimal implementation**

```go
// Package sqliteimport copies an Econumo SQLite database into an
// already-migrated PostgreSQL database. It introspects the target schema for the
// table list, column types, and foreign-key order, so it carries no hardcoded
// schema and does not drift as migrations evolve.
package sqliteimport

import (
	"errors"
	"fmt"
	"sort"
)

// ErrTargetNotEmpty is returned by Import when the target already holds user
// data and force is false; nothing is copied in that case.
var ErrTargetNotEmpty = errors.New("target database already contains data")

type TableCount struct {
	Name string
	Rows int64
}

type Report struct {
	Tables []TableCount
	Total  int64
}

// topoSort orders nodes so every table precedes the tables that reference it.
// deps[child] lists child's referenced (parent) tables; edges to names outside
// the node set are ignored (e.g. FKs into excluded tables). Order is
// deterministic. A foreign-key cycle is an error.
func topoSort(nodes []string, deps map[string][]string) ([]string, error) {
	inSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		inSet[n] = true
	}
	sorted := append([]string(nil), nodes...)
	sort.Strings(sorted)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(nodes))
	var order []string

	var visit func(n string) error
	visit = func(n string) error {
		switch color[n] {
		case black:
			return nil
		case gray:
			return fmt.Errorf("sqliteimport: foreign-key cycle at table %q", n)
		}
		color[n] = gray
		parents := append([]string(nil), deps[n]...)
		sort.Strings(parents)
		for _, p := range parents {
			if p == n || !inSet[p] {
				continue
			}
			if err := visit(p); err != nil {
				return err
			}
		}
		color[n] = black
		order = append(order, n)
		return nil
	}
	for _, n := range sorted {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return order, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infra/storage/sqliteimport/ -run TestTopoSort -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add internal/infra/storage/sqliteimport/
git commit -m "feat(sqliteimport): package skeleton + topological sort"
```

---

### Task 2: Introspection + copy engine (`Import`)

**Files:**
- Modify: `internal/infra/storage/sqliteimport/sqliteimport.go`
- Test: `internal/infra/storage/sqliteimport/import_test.go` (build tag `enginecompare`)

**Interfaces:**
- Consumes: `topoSort`, `ErrTargetNotEmpty`, `Report`, `TableCount` from Task 1.
- Produces:
  - `func Import(ctx context.Context, src, dst *sql.DB, force bool) (Report, error)` — copies every non-excluded base table from `src` (migrated SQLite) into `dst` (migrated PostgreSQL) in one `dst` transaction. If `dst` already has rows in `users` and `force` is false, returns `ErrTargetNotEmpty` and copies nothing. Otherwise truncates the copied tables (removing migration seed rows) and copies parents-before-children.

- [ ] **Step 1: Write the failing test**

```go
//go:build enginecompare

package sqliteimport_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/sqliteimport"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// seedSource inserts one user, one currency, one account and one transaction into
// a migrated SQLite DB, covering a boolean (is_deleted), a NUMERIC amount, a
// timestamp, and a NULL FK (category_id). Returns the user id.
func seedSource(t *testing.T, src *dbtest.DB) string {
	t.Helper()
	const uid = "0190a0aa-0000-7000-8000-000000000001"
	const cid = "0190a0aa-0000-7000-8000-0000000000c1"
	const aid = "0190a0aa-0000-7000-8000-0000000000a1"
	const tid = "0190a0aa-0000-7000-8000-0000000000t1"
	ts := "2026-07-23 10:00:00"
	src.Exec(t, `INSERT INTO currencies (id, code, symbol, created_at) VALUES (?,?,?,?)`,
		cid, "EUR", "€", ts)
	src.Exec(t, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, language, is_active, email_verified, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, uid, "a@b.test", "A", "diamond:sky", "x", "", "argon2id", "en", 1, 1, ts, ts)
	src.Exec(t, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		aid, cid, uid, "Cash", 1, "wallet", 1, ts, ts)
	src.Exec(t, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		tid, uid, aid, 1, "42.50", "lunch", ts, ts, ts)
	return uid
}

func TestImport_CopiesAllTablesWithTypes(t *testing.T) {
	ctx := context.Background()
	src := dbtest.NewSQLite(t)
	dst := dbtest.NewPostgres(t) // migrated; seeds the default USD currency

	seedSource(t, src)

	report, err := sqliteimport.Import(ctx, src.Raw, dst.Raw, false)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Total == 0 {
		t.Fatal("expected rows copied, got 0")
	}

	// Row counts: the source's currency replaced the seeded USD row.
	var accounts, currencies, txns int
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&accounts)
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM currencies`).Scan(&currencies)
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM transactions`).Scan(&txns)
	if accounts != 1 || txns != 1 {
		t.Fatalf("expected 1 account and 1 transaction, got %d / %d", accounts, txns)
	}
	if currencies != 1 {
		t.Fatalf("expected only the source currency (seed replaced), got %d", currencies)
	}

	// Boolean round-trip: is_deleted was 1 in SQLite -> true in Postgres.
	var isDeleted bool
	if err := dst.Raw.QueryRowContext(ctx, `SELECT is_deleted FROM accounts`).Scan(&isDeleted); err != nil {
		t.Fatalf("scan is_deleted: %v", err)
	}
	if !isDeleted {
		t.Fatal("expected is_deleted = true")
	}

	// NUMERIC round-trip: 42.50.
	var amount string
	if err := dst.Raw.QueryRowContext(ctx, `SELECT amount::text FROM transactions`).Scan(&amount); err != nil {
		t.Fatalf("scan amount: %v", err)
	}
	if amount != "42.50" {
		t.Fatalf("expected amount 42.50, got %q", amount)
	}

	// NULL FK preserved.
	var categoryNull bool
	dst.Raw.QueryRowContext(ctx, `SELECT category_id IS NULL FROM transactions`).Scan(&categoryNull)
	if !categoryNull {
		t.Fatal("expected category_id to be NULL")
	}
}

func TestImport_AbortsWhenTargetHasUsersWithoutForce(t *testing.T) {
	ctx := context.Background()
	src := dbtest.NewSQLite(t)
	dst := dbtest.NewPostgres(t)
	seedSource(t, src)

	// Put a user into the target so the guard trips.
	dst.Exec(t, dst.Rebind(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, language, is_active, email_verified, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		"0190a0aa-0000-7000-8000-0000000000e1", "0190a0aa-0000-7000-8000-0000000000e1", "x@y.test", "X", "diamond:sky", "x", "",
		"argon2id", "en", true, true, "2026-07-23 10:00:00", "2026-07-23 10:00:00")

	if _, err := sqliteimport.Import(ctx, src.Raw, dst.Raw, false); !errors.Is(err, sqliteimport.ErrTargetNotEmpty) {
		t.Fatalf("expected ErrTargetNotEmpty, got %v", err)
	}

	// With force it replaces cleanly.
	if _, err := sqliteimport.Import(ctx, src.Raw, dst.Raw, true); err != nil {
		t.Fatalf("Import(force): %v", err)
	}
	var users int
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&users)
	if users != 1 {
		t.Fatalf("expected 1 user after forced replace, got %d", users)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags enginecompare ./internal/infra/storage/sqliteimport/ -run TestImport -v`
Expected: FAIL — `undefined: sqliteimport.Import` (or SKIP if `DATABASE_TEST_PGSQL_URL` is unset; if skipped, provision Postgres with `make test-repo-pgsql`'s compose stack or set the env var, then re-run — the test must actually execute to validate this task).

- [ ] **Step 3: Write minimal implementation**

Append to `internal/infra/storage/sqliteimport/sqliteimport.go` (add `context`, `database/sql`, `strings` to the import block):

```go
var excludedTables = map[string]bool{
	"messenger_messages": true,
	"schema_migrations":  true,
	"migration_versions": true,
}

type column struct {
	name   string
	isBool bool
}

func Import(ctx context.Context, src, dst *sql.DB, force bool) (Report, error) {
	tables, err := listTables(ctx, dst)
	if err != nil {
		return Report{}, err
	}
	deps, err := fkEdges(ctx, dst, tables)
	if err != nil {
		return Report{}, err
	}
	ordered, err := topoSort(tables, deps)
	if err != nil {
		return Report{}, err
	}

	if !force {
		var users int64
		if err := dst.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil {
			return Report{}, fmt.Errorf("sqliteimport: count users: %w", err)
		}
		if users > 0 {
			return Report{}, ErrTargetNotEmpty
		}
	}

	tx, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Clear the copied tables (including migration seed rows) so the result is a
	// faithful replica; CASCADE + the FK-safe insert order below keep it consistent.
	quoted := make([]string, len(ordered))
	for i, t := range ordered {
		quoted[i] = pgIdent(t)
	}
	if _, err := tx.ExecContext(ctx, "TRUNCATE TABLE "+strings.Join(quoted, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		return Report{}, fmt.Errorf("sqliteimport: truncate: %w", err)
	}

	report := Report{}
	for _, table := range ordered {
		cols, err := tableColumns(ctx, dst, table)
		if err != nil {
			return Report{}, err
		}
		n, err := copyTable(ctx, src, tx, table, cols)
		if err != nil {
			return Report{}, fmt.Errorf("sqliteimport: copy %q: %w", table, err)
		}
		report.Tables = append(report.Tables, TableCount{Name: table, Rows: n})
		report.Total += n
	}

	if err := tx.Commit(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func listTables(ctx context.Context, dst *sql.DB) ([]string, error) {
	rows, err := dst.QueryContext(ctx, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`)
	if err != nil {
		return nil, fmt.Errorf("sqliteimport: list tables: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if excludedTables[name] {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func tableColumns(ctx context.Context, dst *sql.DB, table string) ([]column, error) {
	rows, err := dst.QueryContext(ctx, `
		SELECT column_name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1
		ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("sqliteimport: columns of %q: %w", table, err)
	}
	defer rows.Close()
	var out []column
	for rows.Next() {
		var name, dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			return nil, err
		}
		out = append(out, column{name: name, isBool: dataType == "boolean"})
	}
	return out, rows.Err()
}

func fkEdges(ctx context.Context, dst *sql.DB, tables []string) (map[string][]string, error) {
	inSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		inSet[t] = true
	}
	rows, err := dst.QueryContext(ctx, `
		SELECT tc.table_name AS child, ccu.table_name AS parent
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = current_schema()`)
	if err != nil {
		return nil, fmt.Errorf("sqliteimport: fk edges: %w", err)
	}
	defer rows.Close()
	deps := map[string][]string{}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			return nil, err
		}
		if inSet[child] && inSet[parent] {
			deps[child] = append(deps[child], parent)
		}
	}
	return deps, rows.Err()
}

func copyTable(ctx context.Context, src *sql.DB, dstTx *sql.Tx, table string, cols []column) (int64, error) {
	names := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, c := range cols {
		names[i] = pgIdent(c.name)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	selectSQL := "SELECT " + strings.Join(names, ", ") + " FROM " + pgIdent(table)
	insertSQL := "INSERT INTO " + pgIdent(table) + " (" + strings.Join(names, ", ") +
		") VALUES (" + strings.Join(placeholders, ", ") + ")"

	rows, err := src.QueryContext(ctx, selectSQL)
	if err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	defer rows.Close()

	stmt, err := dstTx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	var count int64
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return 0, fmt.Errorf("scan: %w", err)
		}
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				vals[i] = string(v) // uuid/numeric/text columns reject bytea in simple protocol
			case int64:
				if c.isBool {
					vals[i] = v != 0
				}
			}
		}
		if _, err := stmt.ExecContext(ctx, vals...); err != nil {
			return 0, fmt.Errorf("insert: %w", err)
		}
		count++
	}
	return count, rows.Err()
}

// pgIdent double-quotes a PostgreSQL identifier. Table/column names here come
// from information_schema (the target's own catalog), not user input.
func pgIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags enginecompare ./internal/infra/storage/sqliteimport/ -run TestImport -v`
Expected: PASS (both `TestImport_*`). Also run the pure suite to confirm no regression: `go test ./internal/infra/storage/sqliteimport/ -v`.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/storage/sqliteimport/
git commit -m "feat(sqliteimport): schema-driven copy engine with FK order + type coercion"
```

---

### Task 3: `data:import-sqlite` CLI command + registration + docs

**Files:**
- Create: `internal/cli/data_commands.go`
- Modify: `internal/cli/cli.go` (add `dataCommands()` to `commandList()`)
- Test: `internal/cli/data_commands_test.go`
- Modify: `CLAUDE.md` (add the command to the CLI list)

**Interfaces:**
- Consumes: `sqliteimport.Import`, `sqliteimport.ErrTargetNotEmpty`, `sqliteimport.Report` (Task 2); `container.db` (`*sql.DB`, the target) and `container.cfg` (`config.Config`) from `internal/cli/container.go`; `migrate.Run`, `migrations.Pgsql`, `backend.Get` from infra.
- Produces:
  - `func dataCommands() []command`
  - `func parseImportArgs(args []string) (path string, force bool, err error)` — unexported, pure, unit-testable.

- [ ] **Step 1: Write the failing test**

```go
package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/config"
)

func TestParseImportArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		path    string
		force   bool
		wantErr bool
	}{
		{"path only", []string{"db.sqlite"}, "db.sqlite", false, false},
		{"force before path", []string{"--force", "db.sqlite"}, "db.sqlite", true, false},
		{"force after path", []string{"db.sqlite", "--force"}, "db.sqlite", true, false},
		{"missing path", []string{}, "", false, true},
		{"missing path with force", []string{"--force"}, "", false, true},
		{"two paths", []string{"a.sqlite", "b.sqlite"}, "", false, true},
		{"unknown flag", []string{"--nope", "db.sqlite"}, "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, force, err := parseImportArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got path=%q force=%v", path, force)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if path != tc.path || force != tc.force {
				t.Fatalf("got (%q,%v), want (%q,%v)", path, force, tc.path, tc.force)
			}
		})
	}
}

func TestImportSQLite_RejectsNonPostgresTarget(t *testing.T) {
	c := &container{cfg: config.Config{DatabaseDriver: "sqlite"}}
	err := runImportSQLite(context.Background(), c, []string{"db.sqlite"})
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL") {
		t.Fatalf("expected a PostgreSQL-required error, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run 'TestParseImportArgs|TestImportSQLite' -v`
Expected: FAIL — `undefined: parseImportArgs` / `undefined: runImportSQLite`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/data_commands.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/infra/storage/sqliteimport"
)

func dataCommands() []command {
	return []command{
		{
			name:    "data:import-sqlite",
			summary: "Copy all data from a SQLite DB into the configured PostgreSQL: data:import-sqlite [--force] <sqlite-path>",
			run:     runImportSQLite,
		},
	}
}

func parseImportArgs(args []string) (path string, force bool, err error) {
	const usage = "data:import-sqlite [--force] <sqlite-path>"
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "-"):
			return "", false, usageErr(usage)
		default:
			if path != "" {
				return "", false, usageErr(usage)
			}
			path = a
		}
	}
	if path == "" {
		return "", false, usageErr(usage)
	}
	return path, force, nil
}

func runImportSQLite(ctx context.Context, c *container, args []string) error {
	path, force, err := parseImportArgs(args)
	if err != nil {
		return err
	}

	if c.cfg.DatabaseDriver != "postgresql" {
		return fmt.Errorf("data:import-sqlite requires DATABASE_URL to point at PostgreSQL; current engine is %q", c.cfg.DatabaseDriver)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("source sqlite file: %w", err)
	}

	be, ok := backend.Get("sqlite")
	if !ok {
		return errors.New("sqlite backend not registered")
	}
	src, err := be.Open(ctx, abs)
	if err != nil {
		return fmt.Errorf("open source sqlite: %w", err)
	}
	defer func() { _ = src.Close() }()

	// Bring the target to the current schema (schema + schema_migrations), so a
	// bare createdb'd Postgres works in one command; migrate.Run is idempotent.
	if err := migrate.Run(ctx, c.db, toMigratePg(migrations.Pgsql())); err != nil {
		return fmt.Errorf("migrate target: %w", err)
	}

	start := time.Now()
	report, err := sqliteimport.Import(ctx, src, c.db, force)
	if err != nil {
		if errors.Is(err, sqliteimport.ErrTargetNotEmpty) {
			return errors.New("the target PostgreSQL already contains data; re-run with --force to truncate and replace it")
		}
		return err
	}

	for _, t := range report.Tables {
		fmt.Printf("  %-32s %d\n", t.Name, t.Rows)
	}
	fmt.Printf("Imported %d row(s) across %d table(s) in %s.\n",
		report.Total, len(report.Tables), time.Since(start).Round(time.Millisecond))
	return nil
}

func toMigratePg(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}
```

Then register it in `internal/cli/cli.go` — add to `commandList()`:

```go
func commandList() []command {
	var cs []command
	cs = append(cs, userCommands()...)
	cs = append(cs, currencyCommands()...)
	cs = append(cs, tokenCommands()...)
	cs = append(cs, dataCommands()...)
	return cs
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run 'TestParseImportArgs|TestImportSQLite' -v`
Expected: PASS. Then `go build ./...` and `go vet ./...` to confirm the new imports wire up.

- [ ] **Step 5: Update CLAUDE.md**

In the "CLI / management commands" list, add under the existing `data:remove-salt` grouping:

```
data:import-sqlite [--force] <sqlite-path>
```

And add a sentence after the `data:remove-salt` paragraph:

> `data:import-sqlite` copies every table from an existing SQLite database into
> the configured PostgreSQL (`DATABASE_URL` must be a `postgres://` URL). It runs
> the PostgreSQL migrations on the target first (so a bare `createdb` works in one
> step), then copies all app data in one transaction — `access_tokens` and
> `currencies` included, the dead `messenger_messages` and the migration
> bookkeeping tables excluded. It aborts if the target already holds data unless
> `--force` is given, which truncates and replaces.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/data_commands.go internal/cli/cli.go internal/cli/data_commands_test.go CLAUDE.md
git commit -m "feat(cli): data:import-sqlite command for SQLite->PostgreSQL migration"
```

---

### Task 4: Full smoke + gofmt/vet gate

**Files:** none (verification task).

- [ ] **Step 1: Run gofmt check**

Run: `gofmt -l internal/infra/storage/sqliteimport/ internal/cli/`
Expected: no output (all files formatted).

- [ ] **Step 2: Run the smoke suite**

Run: `make go-test`
Expected: PASS, coverage gate (`GO_COVER_MIN=78`) satisfied. `sqliteimport`'s pure `topoSort` tests and the CLI unit tests run here; the `enginecompare` integration test does not (correct — it's build-tagged).

- [ ] **Step 3: Run the engine-comparison / pgsql suite (needs Postgres)**

Run: `go test -tags enginecompare ./internal/infra/storage/sqliteimport/ -v`
Expected: PASS (or SKIP only if `DATABASE_TEST_PGSQL_URL` is unset — in CI it runs; locally provision via the compose stack). If it SKIPs, note that the copy path is unverified until Postgres is available.

- [ ] **Step 4: Manual end-to-end verification (documented, optional but recommended)**

```bash
# Build, seed a scratch SQLite instance, then import into a fresh Postgres.
go build -o /tmp/econumo ./cmd/econumo
# (Assuming a populated sqlite at /tmp/src.sqlite and a running empty Postgres.)
DATABASE_URL='postgres://econumo:econumo@localhost:5432/econumo_new?sslmode=disable' \
  /tmp/econumo data:import-sqlite /tmp/src.sqlite
# Expect a per-table count report ending in "Imported N row(s) across M table(s)".
# Then point the server at the same DATABASE_URL and confirm it boots (migrations
# already recorded, so it runs none) and a known user can log in.
```

- [ ] **Step 5: Commit (if any formatting fixes were needed)**

```bash
git add -A
git commit -m "chore(sqliteimport): gofmt + verification" || echo "nothing to commit"
```

---

## Self-Review

**Spec coverage:**
- Command `data:import-sqlite <sqlite-path>` → Task 3.
- Target-must-be-Postgres guard → Task 3 (`TestImportSQLite_RejectsNonPostgresTarget`).
- Source opened read-only via registered sqlite backend → Task 3.
- Command runs migrations on the target → Task 3 (`migrate.Run` before `Import`).
- Overwrite policy: abort if target has data, `--force` truncates+replaces → Task 2 (`ErrTargetNotEmpty` + `TestImport_AbortsWhenTargetHasUsersWithoutForce`), surfaced in Task 3.
- One transaction, FK-topological order, schema-derived from `information_schema` → Task 2 (`Import`, `listTables`, `tableColumns`, `fkEdges`, `topoSort`).
- Boolean/NUMERIC/timestamp/UUID/NULL handling → Task 2 (`copyTable` + `TestImport_CopiesAllTablesWithTypes`).
- Excluded tables → Task 1/2 (`excludedTables`).
- Included: access_tokens, currencies (seed replaced) → Task 2 (truncate-then-copy; currency count assertion).
- Testing: integration under enginecompare + pure unit tests → Tasks 1, 2, 3, 4.
- Docs (CLAUDE.md) → Task 3 Step 5.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every command has an expected result.

**Type consistency:** `Import`, `Report`, `TableCount`, `ErrTargetNotEmpty`, `topoSort`, `column`, `parseImportArgs`, `runImportSQLite`, `toMigratePg` are defined once and referenced with matching signatures across tasks. Driver name `"postgresql"` and backend key `"sqlite"` used consistently. `current_schema()` used in every introspection query.

## Out of scope
- PostgreSQL → SQLite (reverse).
- Live/incremental sync (this is one-shot, offline).
- Any schema change to either engine.
