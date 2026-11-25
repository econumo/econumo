package migrations_test

import (
	"context"
	"database/sql"
	"testing"
)

// A link a rule classified before the column existed reads as 'rule'; a skip
// row (a rule id, no transaction) and an unruled row stay NULL.
func TestMigration20261122000000BackfillsAssignedBy(t *testing.T) {
	ctx := context.Background()
	db := runUpTo(t, "assigned_by_backfill", "20261122000000")
	insert := `INSERT INTO import_transaction_links (id, source_id, external_account_id, external_transaction_id, transaction_id, status,
		external_payee, external_description, external_amount, external_posted_at, applied_rule_id, imported_at)
		VALUES (?, 's1', 'card', ?, ?, ?, 'Shop', '', 1, '2026-11-20 00:00:00', ?, '2026-11-20 00:00:00')`
	for _, row := range [][]any{
		{"ruled", "e1", "t1", "linked", "r1"},
		{"skipped", "e2", nil, "skipped", "r1"},
		{"plain", "e3", "t3", "linked", nil},
		{"tombstone", "e4", nil, "linked", "r1"},
	} {
		if _, err := db.ExecContext(ctx, insert, row...); err != nil {
			t.Fatal(err)
		}
	}
	runAll(t, db)
	want := map[string]sql.NullString{
		"ruled":     {String: "rule", Valid: true},
		"skipped":   {},
		"plain":     {},
		"tombstone": {},
	}
	for id, w := range want {
		var got sql.NullString
		if err := db.QueryRow(`SELECT assigned_by FROM import_transaction_links WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Fatalf("%s: assigned_by = %+v, want %+v", id, got, w)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'import_transaction_suggestions'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("import_transaction_suggestions: count %d, err %v", n, err)
	}
}
