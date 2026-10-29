package currency_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// rateGapWorld seeds EUR on an account, GBP as a user's display currency, an
// unused global XTS and a custom currency on an account. Only EUR and GBP need
// rates: XTS converts nothing and a custom currency carries its own rate.
func rateGapWorld(t *testing.T) (*dbtest.DB, *fixture.Builder, map[string]string) {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	ids := map[string]string{
		"EUR": f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"}),
		"GBP": f.Currency(fixture.Currency{Code: "GBP", Symbol: "G"}),
		"XTS": f.Currency(fixture.Currency{Code: "XTS", Symbol: "X"}),
	}
	user := f.User(fixture.User{})
	custom := f.Currency(fixture.Currency{Code: "BTC", Symbol: "B", UserID: user, Rate: "0.00001000"})
	eurAccount := f.Account(fixture.Account{UserID: user, CurrencyID: ids["EUR"]})
	f.Account(fixture.Account{UserID: user, CurrencyID: custom})
	gbp := ids["GBP"]
	f.Option(user, "currency", &gbp)
	f.Transaction(fixture.Transaction{UserID: user, AccountID: eurAccount, Amount: "5", SpentAt: "2026-01-10 12:00:00"})
	return db, f, ids
}

func newGapSvc(db *dbtest.DB, now string) *appcurrency.WriteService {
	return appcurrency.NewWriteService(currencyrepo.NewWriteRepo(db.Engine, db.TX), db.TX, fixedClock{t: day(now).Add(15 * time.Hour)})
}

func TestFindRateGaps_Monthly(t *testing.T) {
	db, f, ids := rateGapWorld(t)
	f.Rate(fixture.Rate{CurrencyID: ids["EUR"], PublishedAt: "2026-01-15"})
	f.Rate(fixture.Rate{CurrencyID: ids["GBP"], PublishedAt: "2026-01-31"})
	f.Rate(fixture.Rate{CurrencyID: ids["EUR"], PublishedAt: "2026-03-01"})
	// A rate against another base does not count for USD conversions.
	f.Rate(fixture.Rate{CurrencyID: ids["GBP"], BaseCurrencyID: ids["EUR"], PublishedAt: "2026-03-02"})

	svc := newGapSvc(db, "2026-03-20")
	got, err := svc.FindRateGaps(context.Background(), "usd", model.RatePeriodMonth, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FindRateGaps: %v", err)
	}
	want := model.RateGapReport{
		Base:       "USD",
		Period:     model.RatePeriodMonth,
		From:       day("2026-01-01"),
		To:         day("2026-03-01"),
		Currencies: []string{"EUR", "GBP"},
		Gaps: []model.RateGap{
			{Start: day("2026-02-01"), Missing: []string{"EUR", "GBP"}},
			{Start: day("2026-03-01"), Missing: []string{"GBP"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("report =\n%+v\nwant\n%+v", got, want)
	}
}

func TestFindRateGaps_Daily(t *testing.T) {
	db, f, ids := rateGapWorld(t)
	f.Rate(fixture.Rate{CurrencyID: ids["EUR"], PublishedAt: "2026-03-01"})
	f.Rate(fixture.Rate{CurrencyID: ids["GBP"], PublishedAt: "2026-03-01"})
	f.Rate(fixture.Rate{CurrencyID: ids["EUR"], PublishedAt: "2026-03-03"})
	f.Rate(fixture.Rate{CurrencyID: ids["EUR"], PublishedAt: "2026-03-04"})

	svc := newGapSvc(db, "2026-03-20")
	got, err := svc.FindRateGaps(context.Background(), "USD", model.RatePeriodDay, day("2026-03-01"), day("2026-03-03"))
	if err != nil {
		t.Fatalf("FindRateGaps: %v", err)
	}
	want := []model.RateGap{
		{Start: day("2026-03-02"), Missing: []string{"EUR", "GBP"}},
		{Start: day("2026-03-03"), Missing: []string{"GBP"}},
	}
	if !reflect.DeepEqual(got.Gaps, want) {
		t.Errorf("gaps = %+v, want %+v", got.Gaps, want)
	}
}

func TestFindRateGaps_Range(t *testing.T) {
	db, _, _ := rateGapWorld(t)
	svc := newGapSvc(db, "2026-03-20")
	ctx := context.Background()

	got, err := svc.FindRateGaps(ctx, "USD", model.RatePeriodDay, day("2026-03-19"), day("2027-01-01"))
	if err != nil {
		t.Fatalf("FindRateGaps: %v", err)
	}
	if !got.To.Equal(day("2026-03-20")) || len(got.Gaps) != 2 {
		t.Errorf("an end past today must stop at today: to=%s gaps=%d", got.To.Format("2006-01-02"), len(got.Gaps))
	}

	if _, err := svc.FindRateGaps(ctx, "USD", model.RatePeriodMonth, day("2026-04-01"), time.Time{}); err == nil {
		t.Error("a start after today must be refused")
	}
	if _, err := svc.FindRateGaps(ctx, "ZZZ", model.RatePeriodMonth, time.Time{}, time.Time{}); err == nil {
		t.Error("an unknown base must be refused")
	}
}

func TestFindRateGaps_FutureOnlyLedgerStartsToday(t *testing.T) {
	db, _, _ := rateGapWorld(t)
	svc := newGapSvc(db, "2025-12-20")
	got, err := svc.FindRateGaps(context.Background(), "USD", model.RatePeriodMonth, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FindRateGaps: %v", err)
	}
	if !got.From.Equal(day("2025-12-01")) || !got.To.Equal(day("2025-12-01")) {
		t.Errorf("range = %s..%s, want 2025-12 only", got.From.Format("2006-01-02"), got.To.Format("2006-01-02"))
	}
}

func TestFindRateGaps_NothingInUse(t *testing.T) {
	db := dbtest.New(t)
	svc := newGapSvc(db, "2026-03-20")
	got, err := svc.FindRateGaps(context.Background(), "USD", model.RatePeriodMonth, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FindRateGaps: %v", err)
	}
	if len(got.Currencies) != 0 || len(got.Gaps) != 0 || !got.From.Equal(day("2026-03-01")) {
		t.Errorf("empty database: %+v, want no currencies, no gaps, from this month", got)
	}
}

type recordingLoader struct {
	calls []string
	fail  error
}

func (l *recordingLoader) Load(_ context.Context, date time.Time, base string, symbols []string) ([]model.RateInput, error) {
	l.calls = append(l.calls, date.Format("2006-01-02")+" "+base+" "+strings.Join(symbols, ","))
	if l.fail != nil {
		return nil, l.fail
	}
	var out []model.RateInput
	for _, s := range symbols {
		if s == "GBP" {
			continue
		}
		// Stamped a day late, as a response timestamp past midnight UTC would be.
		out = append(out, model.RateInput{Code: s, Base: base, Rate: "0.90000000", Date: date.AddDate(0, 0, 1)})
	}
	return out, nil
}

func TestBackfillRates(t *testing.T) {
	db, f, ids := rateGapWorld(t)
	f.Rate(fixture.Rate{CurrencyID: ids["GBP"], PublishedAt: "2026-02-10"})
	svc := newGapSvc(db, "2026-02-20")
	ctx := context.Background()

	report, err := svc.FindRateGaps(ctx, "USD", model.RatePeriodMonth, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FindRateGaps: %v", err)
	}
	loader := &recordingLoader{}
	var results []model.RateBackfill
	if err := svc.BackfillRates(ctx, loader, report, func(r model.RateBackfill) { results = append(results, r) }); err != nil {
		t.Fatalf("BackfillRates: %v", err)
	}
	wantCalls := []string{"2026-01-01 USD EUR,GBP", "2026-02-01 USD EUR"}
	if !reflect.DeepEqual(loader.calls, wantCalls) {
		t.Errorf("loader calls = %v, want %v", loader.calls, wantCalls)
	}
	wantResults := []model.RateBackfill{
		{Date: day("2026-01-01"), Requested: []string{"EUR", "GBP"}, Stored: 1, Unavailable: []string{"GBP"}},
		{Date: day("2026-02-01"), Requested: []string{"EUR"}, Stored: 1},
	}
	if !reflect.DeepEqual(results, wantResults) {
		t.Errorf("results = %+v, want %+v", results, wantResults)
	}

	after, err := svc.FindRateGaps(ctx, "USD", model.RatePeriodDay, day("2026-01-01"), day("2026-01-01"))
	if err != nil {
		t.Fatalf("FindRateGaps after: %v", err)
	}
	wantGaps := []model.RateGap{{Start: day("2026-01-01"), Missing: []string{"GBP"}}}
	if !reflect.DeepEqual(after.Gaps, wantGaps) {
		t.Errorf("after backfill the 1st must hold EUR, only GBP missing: %+v", after.Gaps)
	}
}

func TestBackfillRates_StopsAtFirstFailure(t *testing.T) {
	db, _, _ := rateGapWorld(t)
	svc := newGapSvc(db, "2026-03-20")
	ctx := context.Background()
	report, err := svc.FindRateGaps(ctx, "USD", model.RatePeriodMonth, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("FindRateGaps: %v", err)
	}
	boom := errors.New("quota exceeded")
	loader := &recordingLoader{fail: boom}
	err = svc.BackfillRates(ctx, loader, report, nil)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "2026-01-01") {
		t.Errorf("err = %v, want the loader error naming the date", err)
	}
	if len(loader.calls) != 1 {
		t.Errorf("loader called %d times, want 1", len(loader.calls))
	}
}
