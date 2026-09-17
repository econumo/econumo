package migrations_test

// Verifies the 20260915000000 migration: currencies_rates.published_at written
// as a bound time.Time ("2026-09-14 00:00:00 +0000 UTC") is rewritten to the
// 'Y-m-d' form date() can read, collapsing same-day duplicates of legacy rows.

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

const rateDateFormatVersion = "20260915000000"

const rateXTS = "0a000000-0000-0000-0000-000000000001"

type rateRow struct {
	id          string
	publishedAt string
	rate        string
}

func seedRateCurrency(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO currencies (id, code, symbol, fraction_digits, created_at)
		VALUES (?, 'XTS', 'x', 2, '2026-01-01 00:00:00')`, rateXTS); err != nil {
		t.Fatalf("seed currency: %v", err)
	}
}

func seedRawRate(t *testing.T, db *sql.DB, id, publishedAt, rate string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		VALUES (?, ?, ?, ?, ?)`, id, rateXTS, usdSeed, publishedAt, rate); err != nil {
		t.Fatalf("seed rate %s: %v", id, err)
	}
}

func readRates(t *testing.T, db *sql.DB) []rateRow {
	t.Helper()
	// CAST to TEXT: the driver parses DATE-declared columns into time.Time on
	// read-back, which would hide the stored form this test inspects.
	rows, err := db.QueryContext(context.Background(),
		`SELECT id, CAST(published_at AS TEXT), CAST(rate AS TEXT) FROM currencies_rates WHERE currency_id = ? ORDER BY published_at`, rateXTS)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []rateRow
	for rows.Next() {
		var r rateRow
		if err := rows.Scan(&r.id, &r.publishedAt, &r.rate); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMigration20260915_CanonicalizesRateDates(t *testing.T) {
	db := runUpTo(t, "rate_date_format", rateDateFormatVersion)
	seedRateCurrency(t, db)

	// Legacy row with a same-day Go-written twin: the Go row is the later write.
	seedRawRate(t, db, "0b000000-0000-0000-0000-000000000001", "2026-09-13", "11")
	seedRawRate(t, db, "0b000000-0000-0000-0000-000000000002", "2026-09-13 00:00:00 +0000 UTC", "12")
	// Lone Go-written row: rewrite only.
	seedRawRate(t, db, "0b000000-0000-0000-0000-000000000003", "2026-09-14 00:00:00 +0000 UTC", "13")
	// Lone legacy row: untouched.
	seedRawRate(t, db, "0b000000-0000-0000-0000-000000000004", "2026-08-31", "10")

	runAll(t, db)

	got := readRates(t, db)
	want := []rateRow{
		{"0b000000-0000-0000-0000-000000000004", "2026-08-31", "10"},
		{"0b000000-0000-0000-0000-000000000002", "2026-09-13", "12"},
		{"0b000000-0000-0000-0000-000000000003", "2026-09-14", "13"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	var visible int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM currencies_rates WHERE currency_id = ? AND date(published_at) IS NOT NULL`, rateXTS).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != len(want) {
		t.Errorf("%d rows readable by date(), want %d", visible, len(want))
	}
}
