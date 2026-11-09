package currency_test

import (
	"context"
	"sort"
	"testing"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const historyUser = "10000000-0000-7000-8000-000000000001"

func dayRate(cur, day string) model.CurrencyRateViewRow {
	return model.CurrencyRateViewRow{
		CurrencyID: cur, BaseCurrencyID: "usd-id", Rate: "1.5",
		UpdatedAt: day + " 00:00:00", Date: day,
	}
}

func historyDates(t *testing.T, fake *fakeReadModel, from, to string) map[string][]string {
	t.Helper()
	uid, err := vo.ParseId(historyUser)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := time.Parse("2006-01-02", from)
	e, _ := time.Parse("2006-01-02", to)
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "usd-id", Code: "USD"})
	res, err := svc.GetCurrencyRateHistory(context.Background(), uid, f, e)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, it := range res.Items {
		out[it.CurrencyId] = append(out[it.CurrencyId], it.Date)
	}
	for _, d := range out {
		sort.Strings(d)
	}
	return out
}

func visibleRows(ids ...string) []model.CurrencyViewRow {
	rows := []model.CurrencyViewRow{{ID: "usd-id", Code: "USD", Symbol: "$"}}
	for _, id := range ids {
		rows = append(rows, model.CurrencyViewRow{ID: id, Code: id, Symbol: id})
	}
	return rows
}

func TestGetCurrencyRateHistory_IncludesLatestBeforeFrom(t *testing.T) {
	fake := &fakeReadModel{
		rows:  visibleRows("eur-id"),
		rates: []model.CurrencyRateViewRow{dayRate("eur-id", "2026-02-20"), dayRate("eur-id", "2026-02-27"), dayRate("eur-id", "2026-03-03")},
	}
	got := historyDates(t, fake, "2026-03-01", "2026-03-31")["eur-id"]
	if len(got) != 2 || got[0] != "2026-02-27" || got[1] != "2026-03-03" {
		t.Fatalf("eur dates = %v, want [2026-02-27 2026-03-03]", got)
	}
}

func TestGetCurrencyRateHistory_OnlyOlderRates(t *testing.T) {
	fake := &fakeReadModel{
		rows:  visibleRows("gbp-id"),
		rates: []model.CurrencyRateViewRow{dayRate("gbp-id", "2026-01-10")},
	}
	got := historyDates(t, fake, "2026-03-01", "2026-03-31")["gbp-id"]
	if len(got) != 1 || got[0] != "2026-01-10" {
		t.Fatalf("gbp dates = %v, want [2026-01-10]", got)
	}
}

func TestGetCurrencyRateHistory_RateOnFrom(t *testing.T) {
	fake := &fakeReadModel{
		rows:  visibleRows("eur-id"),
		rates: []model.CurrencyRateViewRow{dayRate("eur-id", "2026-02-27"), dayRate("eur-id", "2026-03-01")},
	}
	got := historyDates(t, fake, "2026-03-01", "2026-03-31")["eur-id"]
	if len(got) != 2 || got[0] != "2026-02-27" || got[1] != "2026-03-01" {
		t.Fatalf("eur dates = %v, want both", got)
	}
}

func TestGetCurrencyRateHistory_FiltersInvisible(t *testing.T) {
	fake := &fakeReadModel{
		rows:  visibleRows("eur-id"),
		rates: []model.CurrencyRateViewRow{dayRate("eur-id", "2026-03-02"), dayRate("hidden-id", "2026-03-02")},
	}
	got := historyDates(t, fake, "2026-03-01", "2026-03-31")
	if _, ok := got["hidden-id"]; ok {
		t.Fatal("invisible currency leaked")
	}
	if len(got["eur-id"]) != 1 {
		t.Fatalf("eur dates = %v", got["eur-id"])
	}
}

func TestGetCurrencyRateHistory_CustomAndBaseDatedFrom(t *testing.T) {
	created := time.Date(2025, 7, 15, 10, 30, 0, 0, time.UTC)
	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "usd-id", Code: "USD", Symbol: "$"},
			{ID: "pts-id", Code: "PTS", Symbol: "p", UserID: strPtr(historyUser), Rate: strPtr("2.5"), CreatedAt: created},
		},
	}
	uid, _ := vo.ParseId(historyUser)
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "usd-id", Code: "USD"})
	res, err := svc.GetCurrencyRateHistory(context.Background(), uid, from, from.AddDate(0, 0, 30))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %+v, want custom + synthetic base", res.Items)
	}
	byID := map[string]model.CurrencyRateResult{}
	for _, it := range res.Items {
		byID[it.CurrencyId] = it
	}
	if p := byID["pts-id"]; p.Date != "2026-03-01" || p.Rate != "2.5" || p.UpdatedAt != "2026-03-01 00:00:00" {
		t.Errorf("custom = %+v", p)
	}
	if b := byID["usd-id"]; b.Date != "2026-03-01" || b.Rate != "1" {
		t.Errorf("base = %+v", b)
	}
}

func TestGetCurrencyRateList_DateField(t *testing.T) {
	created := time.Date(2026, 7, 15, 10, 30, 0, 0, time.UTC)
	fake := &fakeReadModel{
		rows: []model.CurrencyViewRow{
			{ID: "usd-id", Code: "USD", Symbol: "$"},
			{ID: "eur-id", Code: "EUR", Symbol: "e"},
			{ID: "pts-id", Code: "PTS", Symbol: "p", UserID: strPtr(historyUser), Rate: strPtr("10"), CreatedAt: created},
		},
		rates: []model.CurrencyRateViewRow{dayRate("eur-id", "2026-07-30")},
	}
	uid, _ := vo.ParseId(historyUser)
	svc := appcurrency.NewReadService(fake, model.BaseCurrency{ID: "usd-id", Code: "USD"})
	res, err := svc.GetCurrencyRateList(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if it.Date != it.UpdatedAt[:10] {
			t.Errorf("%s: date %q != updatedAt day %q", it.CurrencyId, it.Date, it.UpdatedAt[:10])
		}
	}
	for _, it := range res.Items {
		if it.CurrencyId == "pts-id" && it.Date != "2026-07-15" {
			t.Errorf("custom date = %q, want creation day", it.Date)
		}
		if it.CurrencyId == "usd-id" && it.Date != "2026-07-30" {
			t.Errorf("synthetic base date = %q, want newest rate day", it.Date)
		}
	}
}
