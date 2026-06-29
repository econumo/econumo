// Package openexchangerates is the infra adapter that loads exchange rates from
// the Open Exchange Rates API for the app:update-currency-rates CLI command: it
// picks the latest or historical endpoint by date, requests the configured base +
// symbols, and maps the response into app/currency.RateInput values keyed by ISO
// code.
package openexchangerates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appcurrency "github.com/econumo/econumo/internal/app/currency"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
)

// apiBaseURL is the Open Exchange Rates API root. The latest/historical endpoints
// hang off it; it is a Loader field (defaulted here) only so tests can point the
// loader at an httptest server.
const apiBaseURL = "https://openexchangerates.org/api"

// Loader fetches and parses Open Exchange Rates responses.
type Loader struct {
	token   string
	client  *http.Client
	now     func() time.Time
	baseURL string
}

// NewLoader wires the loader. token is OPEN_EXCHANGE_RATES_TOKEN; now supplies
// the current time (used only to decide latest-vs-historical for a given date).
func NewLoader(token string, now func() time.Time) *Loader {
	return &Loader{
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
		now:     now,
		baseURL: apiBaseURL,
	}
}

// apiResponse is the subset of the Open Exchange Rates JSON we consume.
type apiResponse struct {
	Timestamp int64              `json:"timestamp"`
	Base      string             `json:"base"`
	Rates     map[string]float64 `json:"rates"`
}

// Load fetches rates for the given date and base currency, limited to symbols
// (when non-empty). It uses the latest endpoint when date is today, else the
// historical endpoint. The returned rates carry the published date derived from
// the response timestamp (at midnight UTC).
func (l *Loader) Load(ctx context.Context, date time.Time, base string, symbols []string) ([]appcurrency.RateInput, error) {
	if l.token == "" {
		return nil, fmt.Errorf("OPEN_EXCHANGE_RATES_TOKEN is not set")
	}

	endpoint := fmt.Sprintf("%s/historical/%s.json", l.baseURL, date.Format(datetime.DateLayout))
	if date.Format(datetime.DateLayout) == l.now().Format(datetime.DateLayout) {
		endpoint = l.baseURL + "/latest.json"
	}

	q := url.Values{}
	q.Set("app_id", l.token)
	if base != "" {
		q.Set("base", base)
	}
	if len(symbols) > 0 {
		q.Set("symbols", strings.Join(symbols, ","))
	}

	// Log the endpoint + non-secret params (NOT the full URL — that carries the
	// app_id token in its query). Visible at -vv/-vvv; the full code list is shown
	// (the operator running -vvv wants the detail), with a count for convenience.
	slog.Debug("openexchangerates: requesting rates",
		"endpoint", endpoint, "base", base, "count", len(symbols), "symbols", strings.Join(symbols, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open exchange rates request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data apiResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("decode open exchange rates response: %w", err)
	}

	published := time.Unix(data.Timestamp, 0).UTC()
	pubDate := time.Date(published.Year(), published.Month(), published.Day(), 0, 0, 0, 0, time.UTC)

	out := make([]appcurrency.RateInput, 0, len(data.Rates))
	for code, rate := range data.Rates {
		out = append(out, appcurrency.RateInput{
			Code: code,
			Base: data.Base,
			// NUMERIC(19,8): fixed 8 fractional digits, matching the rate column
			// scale and the read side's vo.NewDecimal normalization.
			Rate: strconv.FormatFloat(rate, 'f', 8, 64),
			Date: pubDate,
		})
	}
	return out, nil
}
