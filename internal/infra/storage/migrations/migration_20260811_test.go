package migrations_test

// Verifies the 20260811000000 migration: budgets_elements_limits.period is
// collapsed to one row per (element, month) and rewritten to the canonical
// 'Y-m-d H:i:s' form.

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

const periodFormatVersion = "20260811000000"

type limitRow struct {
	id     string
	period string
	amount string
}

func seedLimitBudget(t *testing.T, db *sql.DB, elementID string) {
	t.Helper()
	ctx := context.Background()
	seedUser(t, db, "u1")
	if _, err := db.ExecContext(ctx, `INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		VALUES ('b1', ?, 'u1', 'B', '2026-01-01 00:00:00', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, usdSeed); err != nil {
		t.Fatalf("seed budget: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO budgets_elements (id, budget_id, external_id, type, sort_key, created_at, updated_at)
		VALUES (?, 'b1', 'ext-1', 1, 'c0', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, elementID); err != nil {
		t.Fatalf("seed element: %v", err)
	}
}

// seedRawLimit writes period verbatim, which is the whole point: the legacy
// RFC3339 form cannot be produced through the repo any more.
func seedRawLimit(t *testing.T, db *sql.DB, id, elementID, period, amount string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO budgets_elements_limits (id, element_id, period, created_at, updated_at, amount)
		VALUES (?, ?, ?, '2026-01-01 00:00:00', '2026-01-01 00:00:00', ?)`, id, elementID, period, amount); err != nil {
		t.Fatalf("seed limit %s: %v", id, err)
	}
}

func readLimits(t *testing.T, db *sql.DB, elementID string) []limitRow {
	t.Helper()
	// CAST to TEXT deliberately: the driver converts DATETIME-declared columns to
	// time.Time and re-renders them as RFC3339 on read-back, which would hide the
	// very thing this test inspects (and is how the legacy rows arose).
	rows, err := db.QueryContext(context.Background(),
		`SELECT id, CAST(period AS TEXT), amount FROM budgets_elements_limits WHERE element_id = ? ORDER BY period`, elementID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []limitRow
	for rows.Next() {
		var r limitRow
		if err := rows.Scan(&r.id, &r.period, &r.amount); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestMigration20260811_CollapsesDuplicateFormatsAndCanonicalizes covers the
// pair that double-counts today: two rows for one month whose only difference
// is the stored format, which the textual unique index cannot see.
func TestMigration20260811_CollapsesDuplicateFormatsAndCanonicalizes(t *testing.T) {
	db := runUpTo(t, "period_format", periodFormatVersion)
	seedLimitBudget(t, db, "el-1")

	seedRawLimit(t, db, "01000000-0000-0000-0000-000000000001", "el-1", "2026-08-01T00:00:00Z", "100")
	seedRawLimit(t, db, "01000000-0000-0000-0000-000000000002", "el-1", "2026-08-01 00:00:00", "300")
	// a lone legacy row: nothing to collapse, only the rewrite
	seedRawLimit(t, db, "01000000-0000-0000-0000-000000000003", "el-1", "2026-09-01T00:00:00Z", "500")

	runAll(t, db)

	got := readLimits(t, db, "el-1")
	if len(got) != 2 {
		t.Fatalf("got %d limits, want 2 (August collapsed, September kept): %+v", len(got), got)
	}
	byPeriod := map[string]limitRow{}
	for _, r := range got {
		byPeriod[r.period] = r
	}
	aug, ok := byPeriod["2026-08-01 00:00:00"]
	if !ok {
		t.Fatalf("no canonical August row: %+v", got)
	}
	// MAX(id) wins and amounts are NOT summed: a duplicate pair is one limit
	// recorded twice, so summing would inflate the user's budget.
	if aug.amount != "300" {
		t.Errorf("August amount = %q, want 300 (the MAX(id) row, not the sum)", aug.amount)
	}
	sep, ok := byPeriod["2026-09-01 00:00:00"]
	if !ok {
		t.Fatalf("September row was not canonicalized: %+v", got)
	}
	if sep.amount != "500" {
		t.Errorf("September amount = %q, want 500", sep.amount)
	}
}

// TestMigration20260811_LeavesCanonicalRowsAlone proves the sweep is a no-op on
// an already-clean database, so re-running it (or running it on a fresh
// install) changes nothing.
func TestMigration20260811_LeavesCanonicalRowsAlone(t *testing.T) {
	db := runUpTo(t, "period_format_clean", periodFormatVersion)
	seedLimitBudget(t, db, "el-2")
	seedRawLimit(t, db, "02000000-0000-0000-0000-000000000001", "el-2", "2026-08-01 00:00:00", "100")
	seedRawLimit(t, db, "02000000-0000-0000-0000-000000000002", "el-2", "2026-09-01 00:00:00", "200")

	runAll(t, db)

	got := readLimits(t, db, "el-2")
	if len(got) != 2 {
		t.Fatalf("got %d limits, want 2: %+v", len(got), got)
	}
	for _, r := range got {
		if r.amount != "100" && r.amount != "200" {
			t.Errorf("amount %q was altered", r.amount)
		}
	}
}

// TestMigration20260811_EveryPeriodIsCanonical is the invariant the merge code
// then relies on: no stored period differs from its datetime() normalization.
func TestMigration20260811_EveryPeriodIsCanonical(t *testing.T) {
	db := runUpTo(t, "period_format_invariant", periodFormatVersion)
	seedLimitBudget(t, db, "el-3")
	seedRawLimit(t, db, "03000000-0000-0000-0000-000000000001", "el-3", "2026-08-01T00:00:00Z", "100")
	seedRawLimit(t, db, "03000000-0000-0000-0000-000000000002", "el-3", "2026-10-01T00:00:00Z", "200")

	runAll(t, db)

	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM budgets_elements_limits WHERE period <> datetime(period)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d period(s) still non-canonical", n)
	}
}
