package repo_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestWriteRepo_LatestRateDate(t *testing.T) {
	db := dbtest.New(t)
	w := currencyrepo.NewWriteRepo(db.Engine, db.TX)
	ctx := context.Background()

	// No rates yet -> ok=false.
	if _, ok, err := w.LatestRateDate(ctx); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	// Seed two rates on different days; the newest wins.
	for _, d := range []time.Time{
		time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
	} {
		if err := w.UpsertRate(ctx, model.RateRow{
			ID:             vo.NewId().String(),
			CurrencyID:     seededUSD,
			BaseCurrencyID: seededUSD,
			Date:           d,
			Rate:           "1.00000000",
		}); err != nil {
			t.Fatalf("UpsertRate: %v", err)
		}
	}

	got, ok, err := w.LatestRateDate(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestRateDate: ok=%v err=%v", ok, err)
	}
	if got.Format("2006-01-02") != "2026-07-22" {
		t.Errorf("LatestRateDate = %s, want 2026-07-22", got.Format("2006-01-02"))
	}
}

// Rates written by the rate updater must be visible to the convertor. On
// SQLite a bound time.Time is stored as "2026-09-14 00:00:00 +0000 UTC", which
// date()/datetime() read as NULL, so every updater-written rate was skipped and
// budgets converted foreign-currency amounts 1:1 (issue #257).
func TestWriteRepo_UpsertedRateIsUsedByRateProvider(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	xts := f.Currency(fixture.Currency{Code: "XTS", Symbol: "x"})

	w := currencyrepo.NewWriteRepo(db.Engine, db.TX)
	for _, rr := range []struct {
		day  int
		rate string
	}{{1, "10.00000000"}, {14, "11.00000000"}, {14, "12.00000000"}} {
		if err := w.UpsertRate(ctx, model.RateRow{
			ID:             vo.NewId().String(),
			CurrencyID:     xts,
			BaseCurrencyID: seededUSD,
			Date:           time.Date(2026, 9, rr.day, 9, 30, 0, 0, time.UTC),
			Rate:           rr.rate,
		}); err != nil {
			t.Fatalf("UpsertRate: %v", err)
		}
	}

	p := currencyrepo.NewRateProvider(db.Engine, db.TX, currencyrepo.New(db.Engine, db.TX), seededUSD)
	// A quarter-long request: the period must snap to September, the month of
	// the latest stored rate, rather than fall back to the raw request.
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	rs, re, err := p.SnappedRatePeriod(ctx, start, end)
	if err != nil {
		t.Fatalf("SnappedRatePeriod: %v", err)
	}
	if rs.Format("2006-01-02") != "2026-09-01" || re.Format("2006-01-02") != "2026-10-01" {
		t.Errorf("snapped period = %s..%s, want 2026-09-01..2026-10-01", rs.Format("2006-01-02"), re.Format("2006-01-02"))
	}

	rates, err := p.AverageRates(ctx, start, end)
	if err != nil {
		t.Fatalf("AverageRates: %v", err)
	}
	var got string
	for _, r := range rates {
		if r.CurrencyID.String() == xts {
			got = r.Rate.String()
		}
	}
	// The second write on the 14th replaces the first: AVG(10, 12) = 11.
	if got != "11" {
		t.Fatalf("XTS average rate = %q, want 11; rates=%+v", got, rates)
	}
}
