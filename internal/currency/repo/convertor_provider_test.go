package repo_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID = "a98daee8-1740-4514-a33e-dccaa90fc07b"
	eurID = "1ae5bfd5-03e8-412b-80d2-c0ecf3ce32fe"
)

func setup(t *testing.T) (*dbtest.DB, *backend.TxManager) {
	t.Helper()
	db := dbtest.NewSQLite(t)
	// The baseline migration seeds a USD currency; this suite reseeds its own
	// USD/EUR with distinct ids, so wipe the baseline rows first (DELETEs are
	// allowed; only the INSERTs move to the fixture builder).
	db.Exec(t, `DELETE FROM currencies_rates`)
	db.Exec(t, `DELETE FROM currencies`)
	// Two currencies (USD base + EUR) and two rate dates in Jan 2026 plus one in
	// Dec 2025. getLatestDate(end) should snap to Jan 2026 -> AVG over Jan only.
	f := fixture.New(t, db)
	f.Currency(fixture.Currency{ID: usdID, Code: "USD", Symbol: "$"})
	f.Currency(fixture.Currency{ID: eurID, Code: "EUR", Symbol: "E"})
	rows := []struct {
		id, date, rate string
	}{
		{"10000000-0000-7000-8000-000000000001", "2026-01-10", "0.90"},
		{"10000000-0000-7000-8000-000000000002", "2026-01-20", "0.92"},
		{"10000000-0000-7000-8000-000000000003", "2025-12-15", "0.85"}, // earlier month, excluded
	}
	for _, r := range rows {
		f.Rate(fixture.Rate{ID: r.id, CurrencyID: eurID, BaseCurrencyID: usdID, Rate: r.rate, PublishedAt: r.date})
	}
	return db, db.TX
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

// TestRateProvider_AverageRates_IncludesFirstOfMonth: a rate published on the
// FIRST day of the snapped month (stored date-only, e.g. "2026-01-01") must be
// included in the average. A time.Time lower bound renders as
// "2026-01-01 00:00:00", which lexically EXCLUDES the date-only row; the query
// binds via date()+"Y-m-d" to include it. Also checks the AVG is rounded to 8
// decimals ("%.8f"), not truncated.
func TestRateProvider_AverageRates_IncludesFirstOfMonth(t *testing.T) {
	ctx := context.Background()
	db, txm := setup(t)
	// Replace the seed with three rates IN January, including the 1st-of-month.
	db.Exec(t, `DELETE FROM currencies_rates`)
	f := fixture.New(t, db)
	for _, r := range []struct{ id, date, rate string }{
		{"20000000-0000-7000-8000-000000000001", "2026-01-01", "0.90"}, // first of month
		{"20000000-0000-7000-8000-000000000002", "2026-01-15", "0.93"},
		{"20000000-0000-7000-8000-000000000003", "2026-01-25", "0.96"},
	} {
		f.Rate(fixture.Rate{ID: r.id, CurrencyID: eurID, BaseCurrencyID: usdID, Rate: r.rate, PublishedAt: r.date})
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
// the latest-rate month, not the requested period.
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
