package server

import (
	"context"
	"testing"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

type stubAccounts struct {
	owner    vo.Id
	deleted  bool
	currency vo.Id
	err      error
}

func (s stubAccounts) AccountOwner(context.Context, vo.Id) (vo.Id, error)  { return s.owner, s.err }
func (s stubAccounts) AccountDeleted(context.Context, vo.Id) (bool, error) { return s.deleted, s.err }
func (s stubAccounts) AccountCurrency(context.Context, vo.Id) (vo.Id, error) {
	return s.currency, s.err
}

type stubCurrencyByID struct {
	views map[string]currencyrepo.CurrencyView
}

func (s stubCurrencyByID) GetByID(_ context.Context, id string) (currencyrepo.CurrencyView, error) {
	v, ok := s.views[id]
	if !ok {
		return currencyrepo.CurrencyView{}, errs.NewNotFound("Currency not found")
	}
	return v, nil
}

type stubCodeLookup struct{ ids map[string]string }

func (s stubCodeLookup) GetIDByCodeForUser(_ context.Context, _ string, code string) (string, error) {
	id, ok := s.ids[code]
	if !ok {
		return "", errs.NewNotFound("Currency not found")
	}
	return id, nil
}

type stubRates struct {
	base      vo.Id
	rates     []model.FullRate
	snapStart time.Time
	snapEnd   time.Time
}

func (s stubRates) BaseCurrencyID(context.Context) (vo.Id, error) { return s.base, nil }
func (s stubRates) AverageRates(context.Context, time.Time, time.Time) ([]model.FullRate, error) {
	return s.rates, nil
}

// SnappedRatePeriod defaults to echoing the requested period (same month as
// the caller's `at`) unless a test overrides it to simulate a stale rate.
func (s stubRates) SnappedRatePeriod(_ context.Context, start, end time.Time) (time.Time, time.Time, error) {
	if s.snapStart.IsZero() {
		return start, end, nil
	}
	return s.snapStart, s.snapEnd, nil
}

type stubConvertor struct{ factor string }

func (s stubConvertor) Convert(_ context.Context, _, _ time.Time, _, _ vo.Id, sum vo.DecimalNumber) (vo.DecimalNumber, error) {
	return sum.Mul(vo.NewDecimal(s.factor)), nil
}

func TestImportsAccountReader_CurrencyCode(t *testing.T) {
	usd := vo.NewId()
	r := NewImportsAccountReader(stubAccounts{currency: usd}, stubCurrencyByID{views: map[string]currencyrepo.CurrencyView{usd.String(): {ID: usd.String(), Code: "USD"}}})
	code, err := r.AccountCurrencyCode(context.Background(), vo.NewId())
	if err != nil || code != "USD" {
		t.Fatalf("code = %q, %v", code, err)
	}
}

func TestImportsCurrencyConverter(t *testing.T) {
	usd, eur, gbp := vo.NewId(), vo.NewId(), vo.NewId()
	lookup := stubCodeLookup{ids: map[string]string{"USD": usd.String(), "EUR": eur.String(), "GBP": gbp.String()}}
	rates := stubRates{base: usd, rates: []model.FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("0.9")}}}
	c := NewImportsCurrencyConverter(lookup, rates, stubConvertor{factor: "1.10"})
	at := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	got, ok, err := c.Convert(context.Background(), vo.NewId(), "EUR", "USD", "10", at)
	if err != nil || !ok || !vo.NewDecimal(got).Equals(vo.NewDecimal("11")) {
		t.Fatalf("EUR->USD = %q, %v, %v", got, ok, err)
	}
	// the base currency needs no rate row
	if _, ok, err := c.Convert(context.Background(), vo.NewId(), "USD", "EUR", "10", at); err != nil || !ok {
		t.Errorf("USD->EUR must be ok: %v, %v", ok, err)
	}
	// GBP has no rate: the shared convertor would silently go 1:1, so the adapter must say no
	if _, ok, err := c.Convert(context.Background(), vo.NewId(), "GBP", "USD", "10", at); err != nil || ok {
		t.Errorf("GBP without a rate must be ok=false: %v, %v", ok, err)
	}
	// unknown code
	if _, ok, err := c.Convert(context.Background(), vo.NewId(), "XXX", "USD", "10", at); err != nil || ok {
		t.Errorf("unknown code must be ok=false: %v, %v", ok, err)
	}
}

func TestImportsCurrencyConverter_StaleSnappedMonth(t *testing.T) {
	usd, eur := vo.NewId(), vo.NewId()
	lookup := stubCodeLookup{ids: map[string]string{"USD": usd.String(), "EUR": eur.String()}}
	at := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	// the only rate on file snaps to July: it must not be used to convert an August tap
	prevMonth := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	rates := stubRates{
		base: usd, rates: []model.FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("0.9")}},
		snapStart: prevMonth, snapEnd: prevMonth.AddDate(0, 1, 0),
	}
	c := NewImportsCurrencyConverter(lookup, rates, stubConvertor{factor: "1.10"})
	if _, ok, err := c.Convert(context.Background(), vo.NewId(), "EUR", "USD", "10", at); err != nil || ok {
		t.Errorf("a rate snapped to a month other than the event's must be ok=false: %v, %v", ok, err)
	}
}

// TestImportsCurrencyConverter_RealProvider wires the real currency/repo
// provider (the stub above hides bugs in its own snap logic) to prove the
// month-scoped no-rate guard holds end to end: a rate published in the
// event's month converts, but one whose only rate is two months stale must
// queue as no-rate rather than convert at that stale average.
func TestImportsCurrencyConverter_RealProvider(t *testing.T) {
	ctx := context.Background()
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	eurID := f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	f.Rate(fixture.Rate{CurrencyID: eurID, BaseCurrencyID: fixture.USD, Rate: "0.90", PublishedAt: "2026-08-10"})

	lookup := currencyrepo.New("sqlite", db.TX)
	rateProvider := currencyrepo.NewRateProvider("sqlite", db.TX, lookup, fixture.USD)
	convertor := appcurrency.NewConvertor(rateProvider)
	c := NewImportsCurrencyConverter(lookup, rateProvider, convertor)
	userID := vo.NewId()

	// same month as the only rate row -> converts
	inMonth := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	got, ok, err := c.Convert(ctx, userID, "EUR", "USD", "10", inMonth)
	if err != nil || !ok {
		t.Fatalf("EUR->USD in the rate's month must convert: ok=%v err=%v", ok, err)
	}
	if want := vo.NewDecimal("10").Div(vo.NewDecimal("0.90")).Round(2); !vo.NewDecimal(got).Equals(want) {
		t.Errorf("converted = %q, want %s", got, want.String())
	}

	// two months after the only rate row -> no rate for THIS event's month
	twoMonthsLater := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	if _, ok, err := c.Convert(ctx, userID, "EUR", "USD", "10", twoMonthsLater); err != nil || ok {
		t.Errorf("a rate stale by two months must be ok=false: ok=%v err=%v", ok, err)
	}
}
