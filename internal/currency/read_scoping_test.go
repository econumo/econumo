package currency_test

// Service-level test for ReadService.GetCurrencyList/GetCurrencyRateList,
// against an in-package fake ReadModel (no DB). Covers the scope/isArchived/
// isHidden mapping and the rate-list visibility filter; the DB-backed scoping
// itself is covered by internal/currency/repo/lookup_read_integration_test.go.

import (
	"context"
	"testing"

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
	svc := appcurrency.NewReadService(fake)

	res, err := svc.GetCurrencyList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]model.CurrencyResult, len(res.Items))
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
			{CurrencyID: "visible-usd", BaseCurrencyID: "base", Rate: "1.00", UpdatedAt: "2026-07-15 00:00:00"},
			{CurrencyID: "invisible-eur", BaseCurrencyID: "base", Rate: "0.90", UpdatedAt: "2026-07-15 00:00:00"},
		},
	}
	svc := appcurrency.NewReadService(fake)

	res, err := svc.GetCurrencyRateList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].CurrencyId != "visible-usd" {
		t.Fatalf("want only the visible currency's rate, got %+v", res.Items)
	}
}
