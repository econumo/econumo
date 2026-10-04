package migrations_test

import (
	"context"
	"testing"
)

func TestMigration20261004_BackfillsBudgetFolderSide(t *testing.T) {
	db := runUpTo(t, "folder_side", "20261004000000")
	ctx := context.Background()
	seedUser(t, db, "u1")
	const ts = "'2026-01-01 00:00:00'"
	if _, err := db.ExecContext(ctx, `INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		VALUES ('b1', ?, 'u1', 'B', `+ts+`, `+ts+`, `+ts+`)`, usdSeed); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"f_income_cat", "f_income_env", "f_expense", "f_empty"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO budgets_folders (id, budget_id, name, created_at, updated_at)
			VALUES (?, 'b1', 'F', `+ts+`, `+ts+`)`, f); err != nil {
			t.Fatalf("seed folder %s: %v", f, err)
		}
	}
	// Types: 1 category, 2 tag, 3 income category, 4 income envelope.
	elems := []struct {
		id, folder string
		typ        int
	}{
		{"e1", "f_income_cat", 3}, {"e2", "f_income_env", 4}, {"e3", "f_expense", 1}, {"e4", "f_expense", 2}, {"e5", "", 3},
	}
	for _, e := range elems {
		var folder any
		if e.folder != "" {
			folder = e.folder
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO budgets_elements (id, budget_id, folder_id, external_id, type, created_at, updated_at)
			VALUES (?, 'b1', ?, ?, ?, `+ts+`, `+ts+`)`, e.id, folder, e.id, e.typ); err != nil {
			t.Fatalf("seed element %s: %v", e.id, err)
		}
	}

	runAll(t, db)

	want := map[string]string{"f_income_cat": "income", "f_income_env": "income", "f_expense": "expense", "f_empty": "expense"}
	for id, side := range want {
		var got string
		if err := db.QueryRowContext(ctx, `SELECT side FROM budgets_folders WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != side {
			t.Errorf("folder %s: side = %q, want %q", id, got, side)
		}
	}
}
