package currency_test

// Service-level test for ReadService.GetCurrencyList/GetCurrencyRateList,
// against an in-package fake ReadModel (no DB). Covers the scope/isArchived/
// isHidden mapping and the rate-list visibility filter; the DB-backed scoping
// itself is covered by internal/currency/repo/lookup_read_integration_test.go.

import (
	"context"
	"testing"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type fakeReadModel struct {
	rows    []model.CurrencyViewRow
	hidden  []string
	rates   []model.CurrencyRateViewRow
	rateErr error
}

func (f *fakeReadModel) UserCurrencyListView(ctx context.Context, userID string) ([]model.CurrencyViewRow, error) {
	return f.rows, nil
}

func (f *fakeReadModel) HiddenCurrencyIDs(ctx context.Context, userID string) ([]string, error) {
	return f.hidden, nil
}

func (f *fakeReadModel) LatestCurrencyRateListView(ctx context.Context) ([]model.CurrencyRateViewRow, error) {
	if f.rateErr != nil {
		return nil, f.rateErr
	}
	return f.rates, nil
}

func strPtr(s string) *string { return &s }

func TestReadService_GetCurrencyList_ScopeAndFlags(t *testing.T) {
	const meID = "10000000-0000-7000-8000-000000000001"
	const otherID = "20000000-0000-7000-8000-000000000002"
	uid, err := vo.ParseId(meID)
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "global-usd", Code: "USD", Symbol: "$", UserID: nil, IsArchived: false},
			{ID: "global-hidden", Code: "HHH", Symbol: "H", UserID: nil, IsArchived: false},
			{ID: "own-archived", Code: "PTS", Symbol: "p", UserID: strPtr(meID), IsArchived: true},
			{ID: "foreign-custom", Code: "GEM", Symbol: "g", UserID: strPtr(otherID), IsArchived: false},
		},
		hidden: []string{"global-hidden"},
	}
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "global-usd", Code: "USD"})

	res, err := svc.GetCurrencyList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]model.CurrencyListItem, len(res.Items))
	for _, item := range res.Items {
		byID[item.Id] = item
	}

	if got := byID["global-usd"]; got.Scope != appcurrency.ScopeGlobal || got.IsHidden != 0 {
		t.Errorf("global-usd: scope=%q isHidden=%d, want global/0", got.Scope, got.IsHidden)
	}
	if got := byID["global-hidden"]; got.Scope != appcurrency.ScopeGlobal || got.IsHidden != 1 {
		t.Errorf("global-hidden: scope=%q isHidden=%d, want global/1", got.Scope, got.IsHidden)
	}
	if got := byID["own-archived"]; got.Scope != appcurrency.ScopeOwn || got.IsArchived != 1 {
		t.Errorf("own-archived: scope=%q isArchived=%d, want own/1", got.Scope, got.IsArchived)
	}
	if got := byID["foreign-custom"]; got.Scope != appcurrency.ScopeShared {
		t.Errorf("foreign-custom: scope=%q, want shared", got.Scope)
	}
}

func TestReadService_GetCurrencyRateList_FiltersToVisible(t *testing.T) {
	const meID = "10000000-0000-7000-8000-000000000001"
	uid, err := vo.ParseId(meID)
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "visible-usd", Code: "USD", Symbol: "$"},
		},
		rates: []model.CurrencyRateViewRow{
			{CurrencyID: "visible-usd", BaseCurrencyID: "visible-usd", Rate: "1.00", UpdatedAt: "2026-07-15 00:00:00"},
			{CurrencyID: "invisible-eur", BaseCurrencyID: "visible-usd", Rate: "0.90", UpdatedAt: "2026-07-15 00:00:00"},
		},
	}
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "visible-usd", Code: "USD"})

	res, err := svc.GetCurrencyRateList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].CurrencyId != "visible-usd" {
		t.Fatalf("want only the visible currency's rate, got %+v", res.Items)
	}
}

// Rates are stored as X-per-base, so the base currency has no stored row
// against itself unless a rate feed happens to write one. Clients convert
// through the base and treat a missing rate row as "fall back to 1:1", which
// silently breaks every base<->custom conversion. The read side therefore
// synthesizes a base/base = 1 row whenever rates exist but the base has none.
func TestReadService_GetCurrencyRateList_SynthesizesBaseRate(t *testing.T) {
	const meID = "10000000-0000-7000-8000-000000000001"
	uid, err := vo.ParseId(meID)
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "usd-id", Code: "USD", Symbol: "$"},
			{ID: "pts-id", Code: "PTS", Symbol: "p", UserID: strPtr(meID)},
		},
		rates: []model.CurrencyRateViewRow{
			{CurrencyID: "pts-id", BaseCurrencyID: "usd-id", Rate: "10", UpdatedAt: "2026-07-30 00:00:00"},
		},
	}
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "usd-id", Code: "USD"})

	res, err := svc.GetCurrencyRateList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("want PTS rate + synthetic base rate, got %+v", res.Items)
	}
	synthetic := res.Items[1]
	if synthetic.CurrencyId != "usd-id" || synthetic.BaseCurrencyId != "usd-id" {
		t.Errorf("synthetic row ids = %s/%s, want usd-id/usd-id", synthetic.CurrencyId, synthetic.BaseCurrencyId)
	}
	if synthetic.Rate != "1" {
		t.Errorf("synthetic rate = %q, want \"1\"", synthetic.Rate)
	}
	if synthetic.UpdatedAt != "2026-07-30 00:00:00" {
		t.Errorf("synthetic updatedAt = %q, want the latest item date", synthetic.UpdatedAt)
	}
}

// A stored base rate row (e.g. written by the OXR feed, which returns the base
// at 1.0) wins: no synthetic duplicate is added.
func TestReadService_GetCurrencyRateList_NoSyntheticWhenBaseRateStored(t *testing.T) {
	const meID = "10000000-0000-7000-8000-000000000001"
	uid, err := vo.ParseId(meID)
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "usd-id", Code: "USD", Symbol: "$"},
			{ID: "eur-id", Code: "EUR", Symbol: "E"},
		},
		rates: []model.CurrencyRateViewRow{
			{CurrencyID: "eur-id", BaseCurrencyID: "usd-id", Rate: "0.90", UpdatedAt: "2026-07-30 00:00:00"},
			{CurrencyID: "usd-id", BaseCurrencyID: "usd-id", Rate: "1", UpdatedAt: "2026-07-30 00:00:00"},
		},
	}
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "usd-id", Code: "USD"})

	res, err := svc.GetCurrencyRateList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("stored base rate must not be duplicated, got %+v", res.Items)
	}
}

// Custom currencies carry a FIXED rate on the currency row; the rate list
// serves it as a synthesized row so every client keeps one conversion source.
// NULL-rate legacy customs yield nothing.
func TestReadService_GetCurrencyRateList_ServesFixedCustomRates(t *testing.T) {
	const meID = "10000000-0000-7000-8000-000000000001"
	const otherID = "20000000-0000-7000-8000-000000000002"
	uid, err := vo.ParseId(meID)
	if err != nil {
		t.Fatal(err)
	}

	created := time.Date(2026, 7, 15, 10, 30, 0, 0, time.UTC)
	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "usd-id", Code: "USD", Symbol: "$"},
			{ID: "pts-id", Code: "PTS", Symbol: "p", UserID: strPtr(meID), Rate: strPtr("10.00000000"), CreatedAt: created},
			{ID: "gem-id", Code: "GEM", Symbol: "g", UserID: strPtr(otherID), Rate: strPtr("2.5"), CreatedAt: created},
			{ID: "nul-id", Code: "NUL", Symbol: "n", UserID: strPtr(meID)},
		},
		rates: []model.CurrencyRateViewRow{
			{CurrencyID: "usd-id", BaseCurrencyID: "usd-id", Rate: "1", UpdatedAt: "2026-07-30 00:00:00"},
		},
	}
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "usd-id", Code: "USD"})

	res, err := svc.GetCurrencyRateList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("want USD row + 2 fixed custom rows, got %+v", res.Items)
	}
	byID := map[string]model.CurrencyRateResult{}
	for _, it := range res.Items {
		byID[it.CurrencyId] = it
	}
	pts := byID["pts-id"]
	if pts.BaseCurrencyId != "usd-id" || pts.Rate != "10" || pts.UpdatedAt != "2026-07-15 10:30:00" {
		t.Fatalf("pts synthesized row = %+v, want base usd-id / rate 10 / created-at datetime", pts)
	}
	if _, ok := byID["gem-id"]; !ok {
		t.Fatal("shared-visible custom's fixed rate missing")
	}
	if _, ok := byID["nul-id"]; ok {
		t.Fatal("NULL-rate legacy custom must yield no row")
	}
}

// With no rate rows at all there is nothing to convert against, so no
// synthetic row is fabricated either.
func TestReadService_GetCurrencyRateList_NoSyntheticWithoutAnyRates(t *testing.T) {
	const meID = "10000000-0000-7000-8000-000000000001"
	uid, err := vo.ParseId(meID)
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "usd-id", Code: "USD", Symbol: "$"},
		},
	}
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "usd-id", Code: "USD"})

	res, err := svc.GetCurrencyRateList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("want empty rate list, got %+v", res.Items)
	}
}
