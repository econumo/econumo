package server

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
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
	base  vo.Id
	rates []model.FullRate
}

func (s stubRates) BaseCurrencyID(context.Context) (vo.Id, error) { return s.base, nil }
func (s stubRates) AverageRates(context.Context, time.Time, time.Time) ([]model.FullRate, error) {
	return s.rates, nil
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
