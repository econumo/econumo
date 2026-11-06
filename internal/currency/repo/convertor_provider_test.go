package repo_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/model"
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
	p := currencyrepo.NewRateProvider("sqlite", txm, lookup, usdID)

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
	p := currencyrepo.NewRateProvider("sqlite", txm, currencyrepo.New("sqlite", txm), usdID)

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
	p := currencyrepo.NewRateProvider("sqlite", txm, currencyrepo.New("sqlite", txm), usdID)

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

// A period before the first stored rate snaps FORWARD to the earliest rate
// month: with no rates at all, every foreign amount would count at face value.
func TestRateProvider_BeforeFirstRate_UsesEarliestMonth(t *testing.T) {
	ctx := context.Background()
	_, txm := setup(t) // earliest rate 2025-12-15
	p := currencyrepo.NewRateProvider("sqlite", txm, currencyrepo.New("sqlite", txm), usdID)

	start := time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC)
	rs, re, err := p.SnappedRatePeriod(ctx, start, end)
	if err != nil {
		t.Fatalf("SnappedRatePeriod: %v", err)
	}
	if rs.Format("2006-01-02") != "2025-12-01" || re.Format("2006-01-02") != "2026-01-01" {
		t.Fatalf("snapped period = %s..%s, want 2025-12-01..2026-01-01", rs.Format("2006-01-02"), re.Format("2006-01-02"))
	}
	rates, err := p.AverageRates(ctx, start, end)
	if err != nil {
		t.Fatalf("AverageRates: %v", err)
	}
	if len(rates) != 1 || rates[0].Rate.String() != "0.85" {
		t.Fatalf("rates = %+v, want EUR 0.85 (December 2025's)", rates)
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

// A custom currency's FIXED rate applies to any requested period — including
// historical months long before any stored rate row exists. Dated custom
// rates could never serve this case (the period snap found nothing).
func TestRateProvider_FixedCustomRate_AppliesToAnyPeriod(t *testing.T) {
	ctx := context.Background()
	db, txm := setup(t)
	f := fixture.New(t, db)
	uid := f.User(fixture.User{Name: "A"})
	pts := f.Currency(fixture.Currency{Code: "PTS", Symbol: "p", UserID: uid, Rate: "10.00000000"})

	lookup := currencyrepo.New("sqlite", txm)
	p := currencyrepo.NewRateProvider("sqlite", txm, lookup, usdID)

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rates, err := p.AverageRates(ctx, start, end)
	if err != nil {
		t.Fatalf("AverageRates: %v", err)
	}
	var got string
	for _, r := range rates {
		if r.CurrencyID.String() == pts {
			got = r.Rate.String()
		}
	}
	if got != "10" {
		t.Fatalf("fixed PTS rate for a historical period = %q, want 10; rates=%+v", got, rates)
	}
}

// shiftedRates seeds EUR rates on both sides of a day-5 budget period
// ("October" = 5 Oct - 4 Nov) on whichever engine dbtest selects, plus a
// custom currency with a fixed rate.
func shiftedRates(t *testing.T, rates map[string]string) (*currencyrepo.RateProvider, string) {
	t.Helper()
	db := dbtest.New(t)
	db.Exec(t, `DELETE FROM currencies_rates`)
	db.Exec(t, `DELETE FROM currencies`)
	f := fixture.New(t, db)
	f.Currency(fixture.Currency{ID: usdID, Code: "USD", Symbol: "$"})
	f.Currency(fixture.Currency{ID: eurID, Code: "EUR", Symbol: "E"})
	uid := f.User(fixture.User{Name: "A"})
	pts := f.Currency(fixture.Currency{Code: "PTS", Symbol: "p", UserID: uid, Rate: "10.00000000"})
	for date, rate := range rates {
		f.Rate(fixture.Rate{CurrencyID: eurID, BaseCurrencyID: usdID, Rate: rate, PublishedAt: date})
	}
	return currencyrepo.NewRateProvider(db.Engine, db.TX, currencyrepo.New(db.Engine, db.TX), usdID), pts
}

func rateOf(t *testing.T, rates []model.FullRate, id string) string {
	t.Helper()
	for _, r := range rates {
		if r.CurrencyID.String() == id {
			return r.Rate.String()
		}
	}
	t.Fatalf("no rate for %s in %+v", id, rates)
	return ""
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func checkRatePeriod(t *testing.T, p *currencyrepo.RateProvider, start, end time.Time, wantStart, wantEnd, wantEUR string, pts string) {
	t.Helper()
	ctx := context.Background()
	rates, err := p.AverageRates(ctx, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if got := rateOf(t, rates, eurID); got != wantEUR {
		t.Errorf("EUR = %s, want %s", got, wantEUR)
	}
	if got := rateOf(t, rates, pts); got != "10" {
		t.Errorf("custom fixed rate = %s, want 10", got)
	}
	rs, re, err := p.SnappedRatePeriod(ctx, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if rs.Format("2006-01-02") != wantStart || re.Format("2006-01-02") != wantEnd {
		t.Errorf("rate period = %s..%s, want %s..%s", rs.Format("2006-01-02"), re.Format("2006-01-02"), wantStart, wantEnd)
	}
}

var octNovRates = map[string]string{
	"2026-10-01": "0.82", // before the shifted window
	"2026-10-05": "0.90", // first day of the window, included
	"2026-10-20": "0.92",
	"2026-11-04": "0.94", // last day of the window, included
	"2026-11-05": "1.50", // the next window's first day
	"2026-11-20": "1.61",
}

// On the Windowed provider (shifted budget periods) a day-5 "October" averages
// the rates inside 5 Oct - 4 Nov, not November's (the month of the latest rate
// before its end) nor October's. The default provider keeps the snap.
func TestRateProvider_ShiftedWindowAveragesItsOwnRates(t *testing.T) {
	p, pts := shiftedRates(t, octNovRates)
	checkRatePeriod(t, p.Windowed(), day(2026, 10, 5), day(2026, 11, 5), "2026-10-05", "2026-11-05", "0.92", pts)
	checkRatePeriod(t, p, day(2026, 10, 5), day(2026, 11, 5), "2026-11-01", "2026-12-01", "1.35", pts)
}

// A calendar month gets its own month's average on both providers.
func TestRateProvider_CalendarMonthUnchanged(t *testing.T) {
	p, pts := shiftedRates(t, octNovRates)
	for _, provider := range []*currencyrepo.RateProvider{p, p.Windowed()} {
		checkRatePeriod(t, provider, day(2026, 10, 1), day(2026, 11, 1), "2026-10-01", "2026-11-01", "0.88", pts)
		checkRatePeriod(t, provider, day(2026, 11, 1), day(2026, 12, 1), "2026-11-01", "2026-12-01", "1.35", pts)
	}
	// A single day (an import's conversion) keeps its month's average.
	checkRatePeriod(t, p, day(2026, 10, 20), day(2026, 10, 21), "2026-10-01", "2026-11-01", "0.88", pts)
	// A legacy day-1 budget's mid-month window [15 Oct, 15 Nov) keeps the snap
	// to the latest rate's month, as before shifted periods existed.
	checkRatePeriod(t, p, day(2026, 10, 15), day(2026, 11, 15), "2026-11-01", "2026-12-01", "1.35", pts)
}

// A shifted window holding no rate falls back to the latest rate's month.
func TestRateProvider_ShiftedWindowWithoutRatesSnaps(t *testing.T) {
	p, pts := shiftedRates(t, map[string]string{"2026-08-10": "0.70", "2026-08-20": "0.72"})
	checkRatePeriod(t, p.Windowed(), day(2026, 10, 5), day(2026, 11, 5), "2026-08-01", "2026-09-01", "0.71", pts)
}
