package currencyrepo_test

// Integration test for the currency RateProvider against a seeded in-memory
// sqlite: verifies AverageRates snaps the period to the latest rate month and
// averages each currency's rate, matching a direct SQL AVG.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

const (
	usdID = "a98daee8-1740-4514-a33e-dccaa90fc07b"
	eurID = "1ae5bfd5-03e8-412b-80d2-c0ecf3ce32fe"
)

func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

func setup(t *testing.T) (*sql.DB, *backend.TxManager) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := migrate.Run(ctx, db, toMigrations(migrations.SQLite())); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Two currencies (USD base + EUR) and two rate dates in Jan 2026 plus one in
	// Dec 2025. getLatestDate(end) should snap to Jan 2026 -> AVG over Jan only.
	now := time.Unix(1690000000, 0).UTC()
	if _, err := db.ExecContext(ctx, `DELETE FROM currencies_rates`); err != nil {
		t.Fatalf("clear rates: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM currencies`); err != nil {
		t.Fatalf("clear currencies: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, fraction_digits, created_at) VALUES (?, 'USD', '$', 2, ?), (?, 'EUR', 'E', 2, ?)`, usdID, now, eurID, now); err != nil {
		t.Fatalf("seed currencies: %v", err)
	}
	rows := []struct {
		id, date, rate string
	}{
		{"10000000-0000-7000-8000-000000000001", "2026-01-10", "0.90"},
		{"10000000-0000-7000-8000-000000000002", "2026-01-20", "0.92"},
		{"10000000-0000-7000-8000-000000000003", "2025-12-15", "0.85"}, // earlier month, excluded
	}
	for _, r := range rows {
		db.ExecContext(ctx, `INSERT INTO currencies_rates (id, currency_id, base_currency_id, rate, published_at) VALUES (?, ?, ?, ?, ?)`,
			r.id, eurID, usdID, r.rate, r.date)
	}
	return db, backend.NewTxManager(db)
}

func TestRateProvider_AverageRates_SnapsToLatestMonth(t *testing.T) {
	ctx := context.Background()
	_, txm := setup(t)
	lookup := currencyrepo.New("sqlite", txm)
	p := currencyrepo.NewRateProvider("sqlite", txm, lookup, "USD")

	// Period covering all of 2026; latest rate date is 2026-01-20, so the period
	// snaps to Jan 2026 and AVG(EUR) = (0.90 + 0.92)/2 = 0.91. The Dec row is
	// excluded.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	rates, err := p.AverageRates(ctx, start, end)
	if err != nil {
		t.Fatalf("AverageRates: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("rates = %+v, want 1 (EUR)", rates)
	}
	if rates[0].CurrencyID.String() != eurID {
		t.Fatalf("currency = %s, want EUR", rates[0].CurrencyID.String())
	}
	if got := rates[0].Rate.String(); got != "0.91" {
		t.Fatalf("avg rate = %s, want 0.91", got)
	}

	base, err := p.BaseCurrencyID(ctx)
	if err != nil || base.String() != usdID {
		t.Fatalf("base = %v err=%v, want USD", base, err)
	}
	if d, _ := p.FractionDigits(ctx, mustParse(t, eurID)); d != 2 {
		t.Fatalf("EUR digits = %d, want 2", d)
	}
}

// TestRateProvider_AverageRates_IncludesFirstOfMonth is the regression for the
// api-compare finding: a rate published on the FIRST day of the snapped month
// (stored date-only, e.g. "2026-01-01") must be included in the average. A
// time.Time lower bound renders as "2026-01-01 00:00:00", which lexically
// EXCLUDES the date-only row; the query/binding uses date()+'Y-m-d' to include
// it. Also checks the AVG is rounded to 8 decimals (%.8f), not truncated.
func TestRateProvider_AverageRates_IncludesFirstOfMonth(t *testing.T) {
	ctx := context.Background()
	db, txm := setup(t)
	// Replace the seed with three rates IN January, including the 1st-of-month.
	if _, err := db.ExecContext(ctx, `DELETE FROM currencies_rates`); err != nil {
		t.Fatalf("clear: %v", err)
	}
	for _, r := range []struct{ id, date, rate string }{
		{"20000000-0000-7000-8000-000000000001", "2026-01-01", "0.90"}, // first of month
		{"20000000-0000-7000-8000-000000000002", "2026-01-15", "0.93"},
		{"20000000-0000-7000-8000-000000000003", "2026-01-25", "0.96"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO currencies_rates (id, currency_id, base_currency_id, rate, published_at) VALUES (?, ?, ?, ?, ?)`,
			r.id, eurID, usdID, r.rate, r.date); err != nil {
			t.Fatalf("seed rate: %v", err)
		}
	}
	p := currencyrepo.NewRateProvider("sqlite", txm, currencyrepo.New("sqlite", txm), "USD")

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	rates, err := p.AverageRates(ctx, start, end)
	if err != nil {
		t.Fatalf("AverageRates: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("rates = %+v, want 1", rates)
	}
	// AVG of (0.90, 0.93, 0.96) = 0.93 exactly (all three rows included). If the
	// 1st-of-month row were dropped, AVG(0.93,0.96)=0.945.
	if got := rates[0].Rate.String(); got != "0.93" {
		t.Fatalf("avg = %s, want 0.93 (first-of-month row must be included)", got)
	}
}

// TestRateProvider_SnappedRatePeriod verifies the reported budget rate period is
// the latest-rate month, not the requested period (PHP getLatestDate snap).
func TestRateProvider_SnappedRatePeriod(t *testing.T) {
	ctx := context.Background()
	_, txm := setup(t) // seed has latest rate 2026-01-20
	p := currencyrepo.NewRateProvider("sqlite", txm, currencyrepo.New("sqlite", txm), "USD")

	// Request a far-future period; it must snap back to Jan 2026.
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	rs, re, err := p.SnappedRatePeriod(ctx, start, end)
	if err != nil {
		t.Fatalf("SnappedRatePeriod: %v", err)
	}
	if rs.Format("2006-01-02") != "2026-01-01" || re.Format("2006-01-02") != "2026-02-01" {
		t.Fatalf("snapped period = %s..%s, want 2026-01-01..2026-02-01", rs.Format("2006-01-02"), re.Format("2006-01-02"))
	}
}

func mustParse(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return id
}
