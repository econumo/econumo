//go:build upgrade

package upgradetest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/infra/storage/pgsql"
	"github.com/econumo/econumo/internal/infra/storage/sqlite"
)

// engine is a database flavour the upgrade runs on: it hands out empty
// databases and reads one back as a schema description.
type engine interface {
	name() string
	// newDatabase returns the DATABASE_URL of a new, empty database.
	newDatabase(ctx context.Context, label string) (string, error)
	open(ctx context.Context, url string) (*sql.DB, error)
	schema(ctx context.Context, db *sql.DB) ([]string, error)
}

// appliedVersions reads the migration bookkeeping the runner keeps.
func appliedVersions(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func queryLines(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		parts := make([]string, len(cols))
		for i, v := range vals {
			if v.Valid {
				parts[i] = cols[i] + "=" + v.String
			} else {
				parts[i] = cols[i] + "=NULL"
			}
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out, rows.Err()
}

type sqliteEngine struct{ dir string }

func (sqliteEngine) name() string { return "sqlite" }

func (e sqliteEngine) newDatabase(_ context.Context, label string) (string, error) {
	return "sqlite:///" + strings.TrimPrefix(e.dir, "/") + "/" + label + ".sqlite", nil
}

func (sqliteEngine) open(ctx context.Context, url string) (*sql.DB, error) {
	return sqlite.New().Open(ctx, url)
}

var spaces = regexp.MustCompile(`\s+`)

// schema lists every table's columns, indexes and foreign keys in a form two
// databases can be diffed by. Columns are compared by name, not position:
// every query and data:import-sqlite name their columns, and a column added by
// a migration that ran in a different order on an older database lands in a
// different position without being different. For the same reason table SQL
// text is not compared.
func (sqliteEngine) schema(ctx context.Context, db *sql.DB) ([]string, error) {
	tables, err := queryLines(ctx, db, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range tables {
		table := strings.TrimPrefix(line, "name=")
		cols, err := queryLines(ctx, db, `SELECT name, type, "notnull", dflt_value, pk FROM pragma_table_info(?) ORDER BY name`, table)
		if err != nil {
			return nil, err
		}
		for _, c := range cols {
			out = append(out, "table "+table+" column "+c)
		}
		fks, err := queryLines(ctx, db, `SELECT "table", "from", "to", on_update, on_delete FROM pragma_foreign_key_list(?) ORDER BY "table", "from"`, table)
		if err != nil {
			return nil, err
		}
		for _, fk := range fks {
			out = append(out, "table "+table+" fk "+fk)
		}
		idx, err := queryLines(ctx, db, `SELECT name, "unique", origin, partial FROM pragma_index_list(?) ORDER BY name`, table)
		if err != nil {
			return nil, err
		}
		for _, ix := range idx {
			name := strings.Fields(ix)[0][len("name="):]
			icols, err := queryLines(ctx, db, `SELECT seqno, name FROM pragma_index_info(?) ORDER BY seqno`, name)
			if err != nil {
				return nil, err
			}
			out = append(out, fmt.Sprintf("table %s index %s columns [%s]", table, ix, strings.Join(icols, "; ")))
		}
	}
	// Index, trigger and view text is written once by its CREATE, so it must
	// match exactly (this also covers a partial index's WHERE).
	defs, err := queryLines(ctx, db, `SELECT type, name, sql FROM sqlite_master WHERE type IN ('index', 'trigger', 'view') AND sql IS NOT NULL ORDER BY type, name`)
	if err != nil {
		return nil, err
	}
	for _, d := range defs {
		out = append(out, "def "+spaces.ReplaceAllString(d, " "))
	}
	return out, nil
}

type pgsqlEngine struct {
	admin *sql.DB
	url   string
	dbs   []string
}

func (*pgsqlEngine) name() string { return "pgsql" }

func (e *pgsqlEngine) newDatabase(ctx context.Context, label string) (string, error) {
	name := "econumo_upgrade_" + strings.NewReplacer(".", "_", "-", "_").Replace(label)
	dsn, err := withDatabase(e.url, name)
	if err != nil {
		return "", err
	}
	if _, err := e.admin.ExecContext(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`); err != nil {
		return "", err
	}
	if _, err := e.admin.ExecContext(ctx, `CREATE DATABASE `+name); err != nil {
		return "", err
	}
	e.dbs = append(e.dbs, name)
	return dsn, nil
}

func (e *pgsqlEngine) cleanup(ctx context.Context) {
	for _, name := range e.dbs {
		_, _ = e.admin.ExecContext(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
	}
	_ = e.admin.Close()
}

// withDatabase swaps the database name in a postgres:// URL.
func withDatabase(raw, name string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("DATABASE_TEST_PGSQL_URL must be a postgres://host/db URL")
	}
	u.Path = "/" + name
	return u.String(), nil
}

func (*pgsqlEngine) open(ctx context.Context, url string) (*sql.DB, error) {
	return pgsql.New().Open(ctx, url)
}

func (*pgsqlEngine) schema(ctx context.Context, db *sql.DB) ([]string, error) {
	var out []string
	cols, err := queryLines(ctx, db, `
		SELECT table_name, column_name, data_type, character_maximum_length,
		       numeric_precision, numeric_scale, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		ORDER BY table_name, column_name`)
	if err != nil {
		return nil, err
	}
	for _, c := range cols {
		out = append(out, "column "+c)
	}
	idx, err := queryLines(ctx, db, `SELECT tablename, indexname, indexdef FROM pg_indexes WHERE schemaname = current_schema() ORDER BY tablename, indexname`)
	if err != nil {
		return nil, err
	}
	for _, i := range idx {
		out = append(out, "index "+i)
	}
	cons, err := queryLines(ctx, db, `
		SELECT c.conrelid::regclass::text AS table_name, c.conname, c.contype, pg_get_constraintdef(c.oid) AS def
		FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE n.nspname = current_schema()
		ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	for _, c := range cons {
		out = append(out, "constraint "+c)
	}
	return out, nil
}

// diffLines lists the lines only one side has.
func diffLines(fresh, upgraded []string) []string {
	in := func(list []string) map[string]bool {
		m := make(map[string]bool, len(list))
		for _, l := range list {
			m[l] = true
		}
		return m
	}
	f, u := in(fresh), in(upgraded)
	var out []string
	for _, l := range fresh {
		if !u[l] {
			out = append(out, "only in a fresh install: "+l)
		}
	}
	for _, l := range upgraded {
		if !f[l] {
			out = append(out, "only after the upgrade:  "+l)
		}
	}
	sort.Strings(out)
	return out
}
