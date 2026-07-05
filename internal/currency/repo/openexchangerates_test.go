package repo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fixedNow returns a now func pinned to d (for latest-vs-historical selection).
func fixedNow(d time.Time) func() time.Time { return func() time.Time { return d } }

// TestLoad_ParsesAndSelectsEndpoint verifies request construction (endpoint by
// date, app_id/base/symbols query) and response mapping (rates -> RateInput with
// 8-digit decimal strings and the timestamp's date at midnight).
func TestLoad_ParsesAndSelectsEndpoint(t *testing.T) {
	today := time.Date(2025, 4, 10, 9, 0, 0, 0, time.UTC)
	// 2025-04-09 12:00:00 UTC -> the published date must collapse to 2025-04-09.
	ts := time.Date(2025, 4, 9, 12, 0, 0, 0, time.UTC).Unix()

	var gotPath, gotAppID, gotBase, gotSymbols string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAppID = r.URL.Query().Get("app_id")
		gotBase = r.URL.Query().Get("base")
		gotSymbols = r.URL.Query().Get("symbols")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"timestamp":` + strconv.FormatInt(ts, 10) + `,"base":"USD","rates":{"EUR":0.92,"GBP":0.8}}`))
	}))
	defer srv.Close()

	l := NewLoader("test-token", fixedNow(today))
	l.baseURL = srv.URL

	// Historical date (not today) -> /historical/<date>.json.
	histDate := time.Date(2025, 4, 9, 0, 0, 0, 0, time.UTC)
	rates, err := l.Load(context.Background(), histDate, "USD", []string{"EUR", "GBP"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if gotPath != "/historical/2025-04-09.json" {
		t.Errorf("endpoint path = %q, want /historical/2025-04-09.json", gotPath)
	}
	if gotAppID != "test-token" || gotBase != "USD" {
		t.Errorf("query app_id=%q base=%q", gotAppID, gotBase)
	}
	if gotSymbols != "EUR,GBP" {
		t.Errorf("symbols = %q, want EUR,GBP", gotSymbols)
	}

	byCode := map[string]string{}
	for _, r := range rates {
		if r.Base != "USD" {
			t.Errorf("rate %s base = %q, want USD", r.Code, r.Base)
		}
		if !r.Date.Equal(time.Date(2025, 4, 9, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("rate %s date = %v, want 2025-04-09 midnight", r.Code, r.Date)
		}
		byCode[r.Code] = r.Rate
	}
	if byCode["EUR"] != "0.92000000" {
		t.Errorf("EUR rate = %q, want 0.92000000", byCode["EUR"])
	}
	if byCode["GBP"] != "0.80000000" {
		t.Errorf("GBP rate = %q, want 0.80000000", byCode["GBP"])
	}

	// today's date -> /latest.json.
	if _, err := l.Load(context.Background(), today, "USD", nil); err != nil {
		t.Fatalf("Load(latest): %v", err)
	}
	if gotPath != "/latest.json" {
		t.Errorf("endpoint path = %q, want /latest.json", gotPath)
	}
}

// TestLoad_MissingToken returns a clear error and makes no request.
func TestLoad_MissingToken(t *testing.T) {
	l := NewLoader("", fixedNow(time.Now()))
	_, err := l.Load(context.Background(), time.Now(), "USD", nil)
	if err == nil || !strings.Contains(err.Error(), "OPEN_EXCHANGE_RATES_TOKEN") {
		t.Fatalf("want OPEN_EXCHANGE_RATES_TOKEN error, got %v", err)
	}
}

// TestLoad_NonOKError surfaces the upstream status + body.
func TestLoad_NonOKError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":true,"message":"invalid_app_id"}`))
	}))
	defer srv.Close()

	l := NewLoader("bad", fixedNow(time.Now()))
	l.baseURL = srv.URL
	_, err := l.Load(context.Background(), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), "USD", nil)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid_app_id") {
		t.Fatalf("want 401 invalid_app_id error, got %v", err)
	}
}
