package currency_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	appcurrency "github.com/econumo/econumo/internal/app/currency"
	"github.com/econumo/econumo/internal/config"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	"github.com/econumo/econumo/internal/shared/jwt"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	handlercurrency "github.com/econumo/econumo/internal/ui/handler/currency"
	"github.com/econumo/econumo/internal/ui/router"
)

const (
	testDataSalt = "0123456789abcdef"

	seedUserID    = "11111111-1111-1111-1111-111111111111"
	seedEmail     = "user@example.test"
	seedName      = "Seed User"
	seedSalt      = "0000000000000000000000000000000000000001"
	seedAvatarURL = "https://avatar.test/x"

	usdID = "cccccccc-0000-0000-0000-0000000000us"
	eurID = "cccccccc-0000-0000-0000-0000000000eu"
	rubID = "cccccccc-0000-0000-0000-0000000000ru"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type harness struct {
	srv   *httptest.Server
	db    *sql.DB
	jwt   *jwt.JWT
	clock fixedClock
	f     *fixture.Builder
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	tdb := dbtest.NewSQLite(t)
	db := tdb.Raw

	priv, pub := testkeys.Paths(t)
	jwtSvc, err := jwt.New(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	f := fixture.New(t, tdb).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: seedUserID, Email: seedEmail, Name: seedName, Avatar: seedAvatarURL, Password: "pw", Salt: seedSalt})

	readRepo := currencyrepo.NewReadRepo("sqlite", tdb.TX)

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	readSvc := appcurrency.NewReadService(readRepo)
	handlers := handlercurrency.NewHandlers(readSvc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handlercurrency.RegisterAPI(handlers, jwtSvc, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, jwt: jwtSvc, clock: clk, f: f}
}

// resetCurrencies clears the currencies + rates tables so a test controls the
// exact dataset. The baseline migration seeds one USD currency
// (dffc2a06-...); tests that assert exact counts/ordering clear it first.
func (h *harness) resetCurrencies(t *testing.T) {
	t.Helper()
	for _, q := range []string{"DELETE FROM currencies_rates", "DELETE FROM currencies"} {
		if _, err := h.db.ExecContext(context.Background(), q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
}

// seedCurrency inserts a currency row with a NULL name (matching prod, where the
// name is always resolved from the Intl table by code). fractionDigits is passed
// through verbatim (the builder's *int field honors an explicit 0 — the
// unknown-code fallback case).
func (h *harness) seedCurrency(t *testing.T, id, code, symbol string, fractionDigits int) {
	t.Helper()
	fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"}).Currency(fixture.Currency{
		ID: id, Code: code, Symbol: symbol, FractionDigits: &fractionDigits,
	})
}

// seedRate inserts a currency_rates row published on the given date (YYYY-MM-DD).
func (h *harness) seedRate(t *testing.T, id, currencyID, baseID, publishedAt, rate string) {
	t.Helper()
	h.f.Rate(fixture.Rate{ID: id, CurrencyID: currencyID, BaseCurrencyID: baseID, PublishedAt: publishedAt, Rate: rate})
}

func (h *harness) do(t *testing.T, method, path, token string) (int, envelope) {
	t.Helper()
	req, err := http.NewRequest(method, h.srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var env envelope
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode envelope (status %d): %v\nbody: %s", resp.StatusCode, err, raw)
		}
	}
	env.raw = raw
	return resp.StatusCode, env
}

func (h *harness) issueToken(t *testing.T) string {
	t.Helper()
	tok, err := h.jwt.Issue(seedUserID, seedEmail, h.clock.Now())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	raw     []byte
}

type currencyItem struct {
	ID             string `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Symbol         string `json:"symbol"`
	FractionDigits int    `json:"fractionDigits"`
}

type rateItem struct {
	CurrencyID     string `json:"currencyId"`
	BaseCurrencyID string `json:"baseCurrencyId"`
	Rate           string `json:"rate"`
	UpdatedAt      string `json:"updatedAt"`
}

func mustUnmarshal[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal %T: %v\nraw: %s", v, err, raw)
	}
	return v
}

// errorsMap decodes the validation-form errors object (field -> messages).
// Access-denied / exception responses emit an empty array ([]) instead, which
// won't unmarshal into a map and leaves the returned map empty.
func (e envelope) errorsMap() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) > 0 {
		_ = json.Unmarshal(e.Errors, &m)
	}
	return m
}
