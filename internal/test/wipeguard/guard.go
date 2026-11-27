// Package wipeguard fails when the schema would let a hard delete of a user
// leave rows behind or be refused: every foreign key to users must cascade,
// nothing the cascade reaches may be blocked by an unhandled foreign key, and
// every id-like column without a foreign key must be one the wipe handles.
package wipeguard

import (
	"database/sql"
	"sort"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

var handledBlockers = map[string]string{
	"budgets.currency_id": "wipe step 7 moves a shared currency; step 9 deletes own budgets first",
	"debts.currency_id":   "wipe step 7 moves a shared currency; own debts go with own budgets",
}

var handledLooseIDs = map[string]string{
	"users_audit.user_id":                              "kept on purpose",
	"users_audit.ref_id":                               "names another users_audit row, which is never deleted",
	"idempotency_keys.user_id":                         "wipe step 8",
	"idempotency_keys.resource_id":                     "wipe step 8",
	"oauth_states.link_user_id":                        "wipe step 8",
	"budgets_elements.external_id":                     "wipe step 3",
	"debts.element_external_id":                        "wipe step 4",
	"import_transaction_links.applied_category_id":     "wipe step 8",
	"import_transaction_links.applied_payee_id":        "wipe step 8",
	"import_transaction_links.applied_tag_id":          "wipe step 8",
	"import_transaction_links.applied_rule_id":         "wipe step 8",
	"import_suggestions.category_id":                   "the owner's own entity; the row cascades with its owner",
	"import_suggestions.payee_id":                      "the owner's own entity; the row cascades with its owner",
	"access_tokens.grant_id":                           "the token cascades with its user",
	"import_account_links.external_account_id":         "provider id, not an Econumo id",
	"import_transaction_links.external_transaction_id": "provider id, not an Econumo id",
	"import_transaction_links.external_account_id":     "provider id, not an Econumo id",
}

// looseUserColumns name a user without a foreign key; the wipe clears them by
// hand (or, for users_audit, keeps them on purpose).
var looseUserColumns = []Column{
	{Table: "idempotency_keys", Name: "user_id"},
	{Table: "oauth_states", Name: "link_user_id"},
	{Table: "users_audit", Name: "user_id"},
}

// Column names one column of one table.
type Column struct{ Table, Name string }

func (c Column) String() string { return c.Table + "." + c.Name }

type foreignKey struct {
	Column
	RefTable  string
	RefColumn string
	OnDelete  string
}

type schema struct {
	columns map[Column]bool // value: NOT NULL
	fks     []foreignKey
}

func (s schema) hasFK(c Column) bool {
	for _, fk := range s.fks {
		if fk.Column == c {
			return true
		}
	}
	return false
}

// Check fails the test for every foreign key or column that would escape or
// block a hard delete of a user, and for every stale entry of the handled lists.
func Check(t *testing.T, db *dbtest.DB) {
	t.Helper()
	s := readSchema(t, db)

	reachable := map[string]bool{"users": true}
	for grown := true; grown; {
		grown = false
		for _, fk := range s.fks {
			if fk.OnDelete == "CASCADE" && reachable[fk.RefTable] && !reachable[fk.Table] {
				reachable[fk.Table] = true
				grown = true
			}
		}
	}

	blockers := map[string]bool{}
	for _, fk := range s.fks {
		if fk.RefTable == "users" && fk.RefColumn == "id" {
			if fk.OnDelete != "CASCADE" {
				t.Errorf("%s references users(id) ON DELETE %s; a user delete must cascade to it", fk, fk.OnDelete)
			}
			continue
		}
		if !reachable[fk.RefTable] || fk.OnDelete == "CASCADE" {
			continue
		}
		if fk.OnDelete == "SET NULL" && !s.columns[fk.Column] {
			continue
		}
		blockers[fk.String()] = true
		if _, ok := handledBlockers[fk.String()]; !ok {
			t.Errorf("%s references %s(%s) ON DELETE %s, which a user delete reaches; it would block the delete: handle it in the wipe and list it in handledBlockers", fk, fk.RefTable, fk.RefColumn, fk.OnDelete)
		}
	}

	for c := range s.columns {
		if !strings.HasSuffix(c.Name, "_id") || s.hasFK(c) {
			continue
		}
		if _, ok := handledLooseIDs[c.String()]; !ok {
			t.Errorf("%s looks like an id but has no foreign key; decide what the wipe does with it and list it in handledLooseIDs", c)
		}
	}

	for key := range handledBlockers {
		if !blockers[key] {
			t.Errorf("handledBlockers lists %s, which is no longer a blocking foreign key", key)
		}
	}
	for key := range handledLooseIDs {
		c, ok := parseColumn(key)
		if !ok {
			t.Errorf("handledLooseIDs key %q is not table.column", key)
			continue
		}
		if _, exists := s.columns[c]; !exists {
			t.Errorf("handledLooseIDs lists %s, which is not a column", key)
		} else if s.hasFK(c) {
			t.Errorf("handledLooseIDs lists %s, which now has a foreign key", key)
		}
	}
}

// UserColumns returns every column that names a user: each foreign key to
// users(id), plus the loose user columns without one, sorted.
func UserColumns(t *testing.T, db *dbtest.DB) []Column {
	t.Helper()
	s := readSchema(t, db)
	seen := map[Column]bool{}
	var out []Column
	add := func(c Column) {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, fk := range s.fks {
		if fk.RefTable == "users" && fk.RefColumn == "id" {
			add(fk.Column)
		}
	}
	for _, c := range looseUserColumns {
		if _, ok := s.columns[c]; !ok {
			t.Fatalf("wipeguard: loose user column %s is not in the schema", c)
		}
		add(c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func parseColumn(key string) (Column, bool) {
	table, name, ok := strings.Cut(key, ".")
	return Column{Table: table, Name: name}, ok && table != "" && name != ""
}

func readSchema(t *testing.T, db *dbtest.DB) schema {
	t.Helper()
	s := schema{columns: map[Column]bool{}}
	var colQuery, fkQuery string
	switch db.Engine {
	case "sqlite":
		colQuery = `SELECT m.name, c.name, c."notnull" FROM sqlite_master m, pragma_table_info(m.name) c
			WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'`
		fkQuery = `SELECT m.name, f."from", f."table", f."to", upper(f.on_delete) FROM sqlite_master m, pragma_foreign_key_list(m.name) f
			WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'`
	case "postgresql":
		colQuery = `SELECT table_name, column_name, CASE WHEN is_nullable = 'NO' THEN 1 ELSE 0 END
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name IN (SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE')`
		fkQuery = `SELECT src.relname, sa.attname, ref.relname, ra.attname,
				CASE con.confdeltype WHEN 'a' THEN 'NO ACTION' WHEN 'r' THEN 'RESTRICT' WHEN 'c' THEN 'CASCADE'
					WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' END
			FROM pg_constraint con
			JOIN pg_class src ON src.oid = con.conrelid
			JOIN pg_class ref ON ref.oid = con.confrelid
			JOIN pg_namespace ns ON ns.oid = con.connamespace
			CROSS JOIN LATERAL unnest(con.conkey, con.confkey) AS k(src_att, ref_att)
			JOIN pg_attribute sa ON sa.attrelid = con.conrelid AND sa.attnum = k.src_att
			JOIN pg_attribute ra ON ra.attrelid = con.confrelid AND ra.attnum = k.ref_att
			WHERE con.contype = 'f' AND ns.nspname = current_schema()`
	default:
		t.Fatalf("wipeguard: unknown engine %q", db.Engine)
	}

	scan(t, db.Raw, colQuery, func(rows *sql.Rows) error {
		var c Column
		var notNull int
		if err := rows.Scan(&c.Table, &c.Name, &notNull); err != nil {
			return err
		}
		s.columns[c] = notNull != 0
		return nil
	})
	scan(t, db.Raw, fkQuery, func(rows *sql.Rows) error {
		var fk foreignKey
		if err := rows.Scan(&fk.Table, &fk.Name, &fk.RefTable, &fk.RefColumn, &fk.OnDelete); err != nil {
			return err
		}
		s.fks = append(s.fks, fk)
		return nil
	})
	if len(s.columns) == 0 {
		t.Fatalf("wipeguard: read no columns from the %s schema", db.Engine)
	}
	return s
}

func scan(t *testing.T, raw *sql.DB, query string, row func(*sql.Rows) error) {
	t.Helper()
	rows, err := raw.Query(query)
	if err != nil {
		t.Fatalf("wipeguard: query schema: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := row(rows); err != nil {
			t.Fatalf("wipeguard: scan schema: %v", err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("wipeguard: read schema: %v", err)
	}
}
