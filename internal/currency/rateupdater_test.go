package currency

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
)

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeLoader struct {
	calls   int
	lastDay time.Time
	rates   []model.RateInput
}

func (l *fakeLoader) Load(_ context.Context, date time.Time, _ string, _ []string) ([]model.RateInput, error) {
	l.calls++
	l.lastDay = date
	return l.rates, nil
}

type fakeStore struct {
	latest      time.Time
	latestOK    bool
	codes       []string
	updated     int
	updateCalls int
}

func (s *fakeStore) LatestRateDate(context.Context) (time.Time, bool, error) {
	return s.latest, s.latestOK, nil
}
func (s *fakeStore) AvailableCodes(context.Context) ([]string, error) { return s.codes, nil }
func (s *fakeStore) UpdateRates(_ context.Context, rates []model.RateInput) (int, error) {
	s.updateCalls++
	s.updated = len(rates)
	return len(rates), nil
}

var today = time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)

func TestRateUpdater_DisabledNeverFetches(t *testing.T) {
	loader := &fakeLoader{}
	store := &fakeStore{}
	u := NewRateUpdater(false, 1, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 0 || store.updateCalls != 0 {
		t.Fatalf("disabled updater fetched: load=%d update=%d", loader.calls, store.updateCalls)
	}
}

func TestRateUpdater_FreshSkips(t *testing.T) {
	loader := &fakeLoader{}
	// Newest rate is today; interval 1 day -> fresh -> skip.
	store := &fakeStore{latest: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), latestOK: true}
	u := NewRateUpdater(true, 1, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 0 || store.updateCalls != 0 {
		t.Fatalf("fresh rates should skip: load=%d update=%d", loader.calls, store.updateCalls)
	}
}

func TestRateUpdater_StaleFetches(t *testing.T) {
	loader := &fakeLoader{rates: []model.RateInput{{Code: "EUR", Base: "USD", Rate: "0.9", Date: today}}}
	// Newest rate is 3 days old; interval 1 day -> stale -> fetch.
	store := &fakeStore{latest: time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC), latestOK: true, codes: []string{"EUR"}}
	u := NewRateUpdater(true, 1, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 1 || store.updateCalls != 1 {
		t.Fatalf("stale rates should fetch: load=%d update=%d", loader.calls, store.updateCalls)
	}
	if loader.lastDay.Format("2006-01-02") != "2026-07-22" {
		t.Errorf("loader called for %s, want 2026-07-22", loader.lastDay.Format("2006-01-02"))
	}
	if store.updated != 1 {
		t.Errorf("updated %d rates, want 1", store.updated)
	}
}

func TestRateUpdater_NoRatesFetches(t *testing.T) {
	loader := &fakeLoader{rates: []model.RateInput{{Code: "EUR", Base: "USD", Rate: "0.9", Date: today}}}
	store := &fakeStore{latestOK: false, codes: []string{"EUR"}} // empty DB
	u := NewRateUpdater(true, 7, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 1 || store.updateCalls != 1 {
		t.Fatalf("empty DB should fetch: load=%d update=%d", loader.calls, store.updateCalls)
	}
}
