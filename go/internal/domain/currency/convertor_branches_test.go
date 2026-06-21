package currency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// flexRates lets each method be overridden per-test so we can exercise the
// error branches of the convertor.
type flexRates struct {
	avg    func(start, end time.Time) ([]FullRate, error)
	base   func() (vo.Id, error)
	digits func(id vo.Id) (int, error)
}

func (f flexRates) AverageRates(_ context.Context, start, end time.Time) ([]FullRate, error) {
	return f.avg(start, end)
}
func (f flexRates) BaseCurrencyID(_ context.Context) (vo.Id, error) { return f.base() }
func (f flexRates) FractionDigits(_ context.Context, id vo.Id) (int, error) {
	return f.digits(id)
}

func ids(t *testing.T) (usd, eur vo.Id) {
	return mustID(t, "00000000-0000-7000-8000-000000000001"),
		mustID(t, "00000000-0000-7000-8000-000000000002")
}

// FractionDigits returning NotFound must NOT propagate: the convertor swallows
// it and defaults to 2 digits ("currency vanished").
func TestConvert_FractionDigitsNotFound_DefaultsToTwo(t *testing.T) {
	usd, eur := ids(t)
	f := flexRates{
		avg: func(_, _ time.Time) ([]FullRate, error) {
			return []FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("2")}}, nil
		},
		base: func() (vo.Id, error) { return usd, nil },
		digits: func(id vo.Id) (int, error) {
			return 0, errs.NewNotFound("currency gone")
		},
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	// 10 USD -> EUR at rate 2 = 20, rounded to default 2 digits.
	got, err := c.Convert(context.Background(), ps, ps.AddDate(0, 1, 0), usd, eur, vo.NewDecimal("10.005"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "20.01" {
		t.Errorf("got %s want 20.01 (rounded to 2 default digits)", got.String())
	}
}

// A non-NotFound FractionDigits error must propagate.
func TestConvert_FractionDigitsOtherError_Propagates(t *testing.T) {
	usd, eur := ids(t)
	sentinel := errors.New("db down")
	f := flexRates{
		avg: func(_, _ time.Time) ([]FullRate, error) {
			return []FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("2")}}, nil
		},
		base:   func() (vo.Id, error) { return usd, nil },
		digits: func(id vo.Id) (int, error) { return 0, sentinel },
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	_, err := c.Convert(context.Background(), ps, ps.AddDate(0, 1, 0), usd, eur, vo.NewDecimal("10"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v want sentinel propagated", err)
	}
}

// When the from/to currency is absent from the rates slice, that hop is skipped
// (no division/multiplication), matching the PHP loop-with-no-match behavior.
func TestConvert_MissingRate_SkipsHop(t *testing.T) {
	usd, eur := ids(t)
	f := flexRates{
		avg:    func(_, _ time.Time) ([]FullRate, error) { return nil, nil }, // no rates at all
		base:   func() (vo.Id, error) { return usd, nil },
		digits: func(id vo.Id) (int, error) { return 2, nil },
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	got, err := c.Convert(context.Background(), ps, ps.AddDate(0, 1, 0), usd, eur, vo.NewDecimal("123.45"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No EUR rate found -> the base->foreign hop is a no-op -> amount unchanged.
	if got.String() != "123.45" {
		t.Errorf("got %s want 123.45 (unchanged when rate missing)", got.String())
	}
}

func TestConvert_AverageRatesError_Propagates(t *testing.T) {
	usd, eur := ids(t)
	sentinel := errors.New("rate fetch failed")
	f := flexRates{
		avg:    func(_, _ time.Time) ([]FullRate, error) { return nil, sentinel },
		base:   func() (vo.Id, error) { return usd, nil },
		digits: func(id vo.Id) (int, error) { return 2, nil },
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	_, err := c.Convert(context.Background(), ps, ps.AddDate(0, 1, 0), usd, eur, vo.NewDecimal("10"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v want AverageRates error propagated", err)
	}
}

func TestConvert_BaseCurrencyError_Propagates(t *testing.T) {
	usd, eur := ids(t)
	sentinel := errors.New("no base")
	f := flexRates{
		avg:    func(_, _ time.Time) ([]FullRate, error) { return nil, nil },
		base:   func() (vo.Id, error) { return usd, sentinel },
		digits: func(id vo.Id) (int, error) { return 2, nil },
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	_, err := c.Convert(context.Background(), ps, ps.AddDate(0, 1, 0), eur, usd, vo.NewDecimal("10"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v want BaseCurrencyID error propagated", err)
	}
}

// Convert short-circuits when from == to, so it never touches the provider.
func TestConvert_SameCurrency_NoProviderCalls(t *testing.T) {
	usd, _ := ids(t)
	called := false
	f := flexRates{
		avg:    func(_, _ time.Time) ([]FullRate, error) { called = true; return nil, nil },
		base:   func() (vo.Id, error) { called = true; return usd, nil },
		digits: func(id vo.Id) (int, error) { called = true; return 2, nil },
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	got, err := c.Convert(context.Background(), ps, ps.AddDate(0, 1, 0), usd, usd, vo.NewDecimal("99.99"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "99.99" || called {
		t.Fatalf("same-currency convert should pass through untouched (got %s, providerCalled=%v)", got.String(), called)
	}
}

// BulkConvert with an empty item list still returns a map (and loads the
// top-level period only).
func TestBulkConvert_Empty(t *testing.T) {
	usd, _ := ids(t)
	f := flexRates{
		avg:    func(_, _ time.Time) ([]FullRate, error) { return nil, nil },
		base:   func() (vo.Id, error) { return usd, nil },
		digits: func(id vo.Id) (int, error) { return 2, nil },
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	res, err := c.BulkConvert(context.Background(), ps, ps.AddDate(0, 1, 0), map[string][]ConvertItem{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("empty bulk result len=%d want 0", len(res))
	}
}

func TestBulkConvert_AverageRatesError_Propagates(t *testing.T) {
	usd, eur := ids(t)
	sentinel := errors.New("boom")
	f := flexRates{
		avg:    func(_, _ time.Time) ([]FullRate, error) { return nil, sentinel },
		base:   func() (vo.Id, error) { return usd, nil },
		digits: func(id vo.Id) (int, error) { return 2, nil },
	}
	c := NewConvertor(f)
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	items := map[string][]ConvertItem{
		"a": {{PeriodStart: ps, PeriodEnd: ps.AddDate(0, 1, 0), From: eur, To: usd, Amount: vo.NewDecimal("10")}},
	}
	_, err := c.BulkConvert(context.Background(), ps, ps.AddDate(0, 1, 0), items)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v want propagated", err)
	}
}

// A bulk item whose PeriodStart is in a DIFFERENT month than the top-level
// period forces a second rate-period to be loaded and keyed by that month
// (the cross-month fan-out branch). Each month returns a distinct rate so we
// can assert the correct period's rate was applied.
func TestBulkConvert_CrossMonthRatePeriods(t *testing.T) {
	usd, eur := ids(t)
	top := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)   // March (top-level)
	other := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC) // May (sub-item)
	f := flexRates{
		avg: func(start, _ time.Time) ([]FullRate, error) {
			// March rate 0.5, May rate 0.25 (keyed by the start month).
			if start.Month() == time.May {
				return []FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("0.25")}}, nil
			}
			return []FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("0.5")}}, nil
		},
		base:   func() (vo.Id, error) { return usd, nil },
		digits: func(id vo.Id) (int, error) { return 2, nil },
	}
	c := NewConvertor(f)
	items := map[string][]ConvertItem{
		// March item: 10 EUR / 0.5 = 20 USD.
		"march": {{PeriodStart: top, PeriodEnd: top.AddDate(0, 1, 0), From: eur, To: usd, Amount: vo.NewDecimal("10")}},
		// May item: 10 EUR / 0.25 = 40 USD (must use the May-keyed rates).
		"may": {{PeriodStart: other, PeriodEnd: other.AddDate(0, 1, 0), From: eur, To: usd, Amount: vo.NewDecimal("10")}},
	}
	res, err := c.BulkConvert(context.Background(), top, top.AddDate(0, 1, 0), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := res["march"].String(); got != "20" {
		t.Errorf("march = %s want 20", got)
	}
	if got := res["may"].String(); got != "40" {
		t.Errorf("may = %s want 40 (must use May rate period)", got)
	}
}
