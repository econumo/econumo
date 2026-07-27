// Package sqliteimport copies an Econumo SQLite database into an
// already-migrated PostgreSQL database. It introspects the target schema for the
// table list, column types, and foreign-key order, so it carries no hardcoded
// schema and does not drift as migrations evolve.
package sqliteimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrTargetNotEmpty is returned by Import when the target already holds user
// data and force is false; nothing is copied in that case.
var ErrTargetNotEmpty = errors.New("target database already contains data")

// ErrSchemaMismatch is returned when the source and target databases are at
// different migration versions. The copier selects the target's columns out of
// the source, so a version skew would silently drop columns/tables the two do
// not share; refusing is safer than a partial import that reports success.
var ErrSchemaMismatch = errors.New("source and target schema versions differ")

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

func schemaVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		set[v] = true
	}
	return set, rows.Err()
}

func sameVersionSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

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
	srcVers, err := schemaVersions(ctx, src)
	if err != nil {
		return Report{}, fmt.Errorf("sqliteimport: read source schema_migrations (was the SQLite database migrated by econumo?): %w", err)
	}
	dstVers, err := schemaVersions(ctx, dst)
	if err != nil {
		return Report{}, fmt.Errorf("sqliteimport: read target schema_migrations: %w", err)
	}
	if !sameVersionSet(srcVers, dstVers) {
		return Report{}, fmt.Errorf("%w (source has %d applied migration(s), target has %d); boot the current econumo binary against the source SQLite once so it is on the latest schema, then retry", ErrSchemaMismatch, len(srcVers), len(dstVers))
	}

	tables, err := listTables(ctx, dst)
	if err != nil {
		return Report{}, err
	}
	deps, err := fkEdges(ctx, dst, tables)
	if err != nil {
		return Report{}, err
	}
	// The schema has no self-referential foreign keys, so rows within a table
	// carry no intra-table ordering constraint; only cross-table order matters.
	ordered, err := topoSort(tables, deps)
	if err != nil {
		return Report{}, err
	}

	// Introspect every table's columns BEFORE opening the transaction: the copy
	// runs inside a tx, and a constrained pool (tests pin MaxOpenConns=1) would
	// deadlock if a metadata query needed a second connection mid-transaction.
	cols := make(map[string][]column, len(ordered))
	for _, table := range ordered {
		c, err := tableColumns(ctx, dst, table)
		if err != nil {
			return Report{}, err
		}
		cols[table] = c
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

	quoted := make([]string, len(ordered))
	for i, t := range ordered {
		quoted[i] = pgIdent(t)
	}
	if _, err := tx.ExecContext(ctx, "TRUNCATE TABLE "+strings.Join(quoted, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		return Report{}, fmt.Errorf("sqliteimport: truncate: %w", err)
	}

	report := Report{}
	for _, table := range ordered {
		n, err := copyTable(ctx, src, tx, table, cols[table])
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
