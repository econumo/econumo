// Package migrate is Econumo's small, dependency-free database migration
// runner. It deliberately avoids golang-migrate (and any third-party tool):
// migrations are an ordered, embedded []Migration supplied by each storage
// backend, applied inside transactions on boot.
//
// Drop-in safety is the central concern. The service runs against an existing
// production database whose schema must not be re-created. The migrations are
// keyed by the SAME version strings the legacy schema used (e.g.
// "20210812210548"), so the runner can import the versions already recorded in
// the legacy migration table (migration_versions) into its own bookkeeping as
// already-applied — without executing their SQL. An existing DB therefore skips
// the schema-creating migrations and applies only genuinely new migrations; a
// fresh DB runs everything normally.
//
// The runner keeps its own bookkeeping table (schema_migrations) side by side
// with the legacy migration table; the two never interfere.
//
// Only database/sql and the standard library are used. The same SQL set is also
// consumed by sqlc as its schema input, so schema and queries cannot drift.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Migration is a single ordered schema change.
//
// Version is an opaque, sortable identifier (e.g. "0001_baseline"). Migrations
// are applied in ascending Version order. SQL may contain multiple statements
// separated by ';' — the runner splits and executes them individually within
// one transaction so it works on drivers (such as lib/pq) that reject
// multi-statement Exec.
//
// Migration is defined in this package (not in the backend package) so that the
// backend package can import migrate and expose Migrations() []migrate.Migration
// without creating an import cycle.
type Migration struct {
	Version string
	SQL     string
}

// legacyMigrationTable is the legacy migration bookkeeping table. It is a fixed
// name in the existing databases, so it is a constant rather than a configurable
// option.
const legacyMigrationTable = "migration_versions"

// Run ensures the schema_migrations bookkeeping table exists, imports any
// versions already recorded by the legacy migration system, then applies every
// migration in migs whose Version has not already been recorded, in ascending
// Version order. Each migration's statements run inside a single transaction
// together with the row that records its version; a failure rolls the whole
// migration back so partial application never occurs.
//
// Drop-in behavior: because the migrations reuse the legacy version strings
// (e.g. "20210812210548"), Run copies the versions found in the legacy
// migration_versions table into schema_migrations as already-applied WITHOUT
// executing their SQL. An existing production DB therefore skips the
// schema-creating migrations and only applies genuinely new ones; a fresh DB
// (no/empty legacy table) runs everything. Run is idempotent.
func Run(ctx context.Context, db *sql.DB, migs []Migration) error {
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	// Import legacy versions (drop-in): copy any version recorded in the legacy
	// table into schema_migrations as already-applied, without running SQL.
	legacyVersions, err := readLegacyVersions(ctx, db, legacyMigrationTable)
	if err != nil {
		return err
	}
	for _, v := range legacyVersions {
		if applied[v] {
			continue
		}
		if err := recordVersion(ctx, db, v); err != nil {
			return fmt.Errorf("migrate: import legacy version %q: %w", v, err)
		}
		applied[v] = true
	}

	// Apply in ascending version order; do not mutate the caller's slice.
	ordered := make([]Migration, len(migs))
	copy(ordered, migs)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Version < ordered[j].Version
	})

	for _, m := range ordered {
		if applied[m.Version] {
			continue
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("migrate: apply %q: %w", m.Version, err)
		}
	}

	return nil
}

// ensureSchemaMigrations creates the bookkeeping table if it does not yet exist.
// The column types (VARCHAR/TIMESTAMP) are understood by both the sqlite
// (modernc.org/sqlite) and postgresql (lib/pq) drivers used by Econumo.
func ensureSchemaMigrations(ctx context.Context, db *sql.DB) error {
	const stmt = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version VARCHAR(191) NOT NULL PRIMARY KEY,
	applied_at TIMESTAMP NOT NULL
)`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("migrate: create schema_migrations: %w", err)
	}
	return nil
}

// appliedVersions returns the set of versions already recorded as applied.
func appliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrate: scan version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: iterate versions: %w", err)
	}
	return applied, nil
}

// readLegacyVersions returns the versions recorded in a pre-existing legacy
// migration table. A missing/unqueryable table yields no versions (treated as a
// fresh database) rather than an error. The version column is assumed to be
// named "version".
func readLegacyVersions(ctx context.Context, db *sql.DB, legacyTable string) ([]string, error) {
	if !validIdentifier(legacyTable) {
		return nil, fmt.Errorf("migrate: invalid legacy table name %q", legacyTable)
	}
	// Table name cannot be a bound parameter; validated above.
	q := fmt.Sprintf(`SELECT version FROM %s`, legacyTable)
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		// Table absent: fresh database, nothing to import.
		return nil, nil
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrate: scan legacy version: %w", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: iterate legacy versions: %w", err)
	}
	return versions, nil
}

// applyMigration runs all statements of a single migration plus its bookkeeping
// row inside one transaction.
func applyMigration(ctx context.Context, db *sql.DB, m Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	// Rollback is a no-op after a successful Commit.
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range splitStatements(m.SQL) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec statement: %w\n--- statement ---\n%s", err, stmt)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (`+placeholdersFor(db.Driver(), 2)+`)`,
		m.Version, nowUTC(),
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// recordVersion inserts a bookkeeping row without running any migration SQL.
// Used for the baseline-on-legacy case.
func recordVersion(ctx context.Context, db *sql.DB, version string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (`+placeholdersFor(db.Driver(), 2)+`)`,
		version, nowUTC(),
	)
	return err
}

// splitStatements splits a SQL blob into individual statements on top-level
// semicolons. lib/pq rejects multi-statement Exec, so statements must be
// executed one at a time; modernc.org/sqlite accepts either form, so splitting
// is safe for both. Semicolons inside single/double-quoted string literals and
// inside line ('--') or block ('/* */') comments are ignored. Empty trailing
// statements are dropped.
func splitStatements(sqlText string) []string {
	var (
		stmts   []string
		b       strings.Builder
		runes   = []rune(sqlText)
		n       = len(runes)
		inSQ    bool // inside '...'
		inDQ    bool // inside "..."
		inLine  bool // inside -- comment
		inBlock bool // inside /* */ comment
	)

	flush := func() {
		s := strings.TrimSpace(b.String())
		if s != "" {
			stmts = append(stmts, s)
		}
		b.Reset()
	}

	for i := 0; i < n; i++ {
		c := runes[i]

		switch {
		case inLine:
			b.WriteRune(c)
			if c == '\n' {
				inLine = false
			}
			continue
		case inBlock:
			b.WriteRune(c)
			if c == '*' && i+1 < n && runes[i+1] == '/' {
				b.WriteRune(runes[i+1])
				i++
				inBlock = false
			}
			continue
		case inSQ:
			b.WriteRune(c)
			if c == '\'' {
				// '' is an escaped quote inside a single-quoted literal.
				if i+1 < n && runes[i+1] == '\'' {
					b.WriteRune(runes[i+1])
					i++
				} else {
					inSQ = false
				}
			}
			continue
		case inDQ:
			b.WriteRune(c)
			if c == '"' {
				if i+1 < n && runes[i+1] == '"' {
					b.WriteRune(runes[i+1])
					i++
				} else {
					inDQ = false
				}
			}
			continue
		}

		// Not inside any literal or comment.
		switch c {
		case '-':
			if i+1 < n && runes[i+1] == '-' {
				inLine = true
				b.WriteRune(c)
				continue
			}
			b.WriteRune(c)
		case '/':
			if i+1 < n && runes[i+1] == '*' {
				inBlock = true
				b.WriteRune(c)
				b.WriteRune(runes[i+1])
				i++
				continue
			}
			b.WriteRune(c)
		case '\'':
			inSQ = true
			b.WriteRune(c)
		case '"':
			inDQ = true
			b.WriteRune(c)
		case ';':
			flush()
		default:
			b.WriteRune(c)
		}
	}
	flush()
	return stmts
}

// validIdentifier reports whether s is a safe SQL identifier (letters, digits,
// underscore; not starting with a digit). Used to guard the legacy table name,
// which cannot be passed as a bound parameter.
func validIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
		isDigit := r >= '0' && r <= '9'
		if i == 0 {
			if !isLetter {
				return false
			}
			continue
		}
		if !isLetter && !isDigit {
			return false
		}
	}
	return true
}
