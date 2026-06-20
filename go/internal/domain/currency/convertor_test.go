package currency

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// fakeRates is an in-memory RateProvider for the convertor math tests. Rates are
// "units of currency per one base unit" (base = USD here). digits maps a
// currency id to its fraction digits.
type fakeRates struct {
	base   vo.Id
	rates  []FullRate
	digits map[string]int
}

func (f fakeRates) AverageRates(_ context.Context, _, _ time.Time) ([]FullRate, error) {
	return f.rates, nil
}
func (f fakeRates) BaseCurrencyID(_ context.Context) (vo.Id, error) { return f.base, nil }
func (f fakeRates) FractionDigits(_ context.Context, id vo.Id) (int, error) {
	return f.digits[id.String()], nil
}

func mustID(t *testing.T, s string) vo.Id {
	t.Helper()
	id, err := vo.ParseId(s)
	if err != nil {
		t.Fatalf("parse id %s: %v", s, err)
	}
	return id
}

func TestConvertor_TwoHop(t *testing.T) {
	usd := mustID(t, "00000000-0000-7000-8000-000000000001") // base
	eur := mustID(t, "00000000-0000-7000-8000-000000000002") // rate 0.9 per USD
	gbp := mustID(t, "00000000-0000-7000-8000-000000000003") // rate 0.8 per USD
	f := fakeRates{
		base: usd,
		rates: []FullRate{
			{CurrencyID: eur, Rate: vo.NewDecimal("0.9")},
			{CurrencyID: gbp, Rate: vo.NewDecimal("0.8")},
		},
		digits: map[string]int{usd.String(): 2, eur.String(): 2, gbp.String(): 2},
	}
	c := NewConvertor(f)
	ctx := context.Background()
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	pe := ps.AddDate(0, 1, 0)

	cases := []struct {
		name     string
		from, to vo.Id
		amount   string
		want     string
	}{
		// same currency -> passthrough
		{"usd->usd", usd, usd, "100", "100"},
		{"eur->eur", eur, eur, "42.5", "42.5"},
		// base -> foreign: 100 USD * 0.9 = 90 EUR
		{"usd->eur", usd, eur, "100", "90"},
		// foreign -> base: 90 EUR / 0.9 = 100 USD
		{"eur->usd", eur, usd, "90", "100"},
		// foreign -> foreign through base: 90 EUR /0.9 =100 USD *0.8 =80 GBP
		{"eur->gbp", eur, gbp, "90", "80"},
		// rounding to 2 digits: 10 USD * 0.9 with extra precision still 9
		{"usd->eur round", usd, eur, "9.999", "9"},
	}
	for _, tc := range cases {
		got, err := c.Convert(ctx, ps, pe, tc.from, tc.to, vo.NewDecimal(tc.amount))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.String() != tc.want {
			t.Errorf("%s: convert(%s) = %s, want %s", tc.name, tc.amount, got.String(), tc.want)
		}
	}
}

func TestConvertor_BulkSummarizesAndSums(t *testing.T) {
	usd := mustID(t, "00000000-0000-7000-8000-000000000001")
	eur := mustID(t, "00000000-0000-7000-8000-000000000002")
	f := fakeRates{
		base:   usd,
		rates:  []FullRate{{CurrencyID: eur, Rate: vo.NewDecimal("0.5")}},
		digits: map[string]int{usd.String(): 2, eur.String(): 2},
	}
	c := NewConvertor(f)
	ctx := context.Background()
	ps := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	pe := ps.AddDate(0, 1, 0)

	// key "a": two EUR->USD items in the same period summarize (40+60=100 EUR)
	// then convert: 100 / 0.5 = 200 USD. key "b": single passthrough 25 USD.
	items := map[string][]ConvertItem{
		"a": {
			{PeriodStart: ps, PeriodEnd: pe, From: eur, To: usd, Amount: vo.NewDecimal("40")},
			{PeriodStart: ps, PeriodEnd: pe, From: eur, To: usd, Amount: vo.NewDecimal("60")},
		},
		"b": {
			{PeriodStart: ps, PeriodEnd: pe, From: usd, To: usd, Amount: vo.NewDecimal("25")},
		},
	}
	res, err := c.BulkConvert(ctx, ps, pe, items)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if got := res["a"].String(); got != "200" {
		t.Errorf("a = %s, want 200", got)
	}
	if got := res["b"].String(); got != "25" {
		t.Errorf("b = %s, want 25", got)
	}
}
