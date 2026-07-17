package api_test

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

	"github.com/econumo/econumo/internal/config"
	appcurrency "github.com/econumo/econumo/internal/currency"
	handlercurrency "github.com/econumo/econumo/internal/currency/api"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/web/router"
)

const (
	testDataSalt = "0123456789abcdef"

	seedUserID  = "11111111-1111-1111-1111-111111111111"
	otherUserID = "22222222-2222-2222-2222-222222222222"
	seedEmail   = "user@example.test"
	seedName    = "Seed User"
	seedSalt    = "0000000000000000000000000000000000000001"
	seedAvatar  = "https://avatar.test/x"

	usdID = "cccccccc-0000-0000-0000-0000000000us"
	eurID = "cccccccc-0000-0000-0000-0000000000eu"
	rubID = "cccccccc-0000-0000-0000-0000000000ru"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// fakeProfileCurrency stubs the currency feature's ProfileCurrency port
// (normally satisfied by a glue adapter over the user feature). Mutable so a
// test can point the caller's profile currency away from the default before
// exercising the hide-currency guard.
type fakeProfileCurrency struct{ code string }

func (f *fakeProfileCurrency) CurrencyCode(ctx context.Context, userID string) (string, error) {
	return f.code, nil
}

var _ appcurrency.ProfileCurrency = (*fakeProfileCurrency)(nil)

type harness struct {
	srv     *httptest.Server
	db      *sql.DB
	clock   fixedClock
	f       *fixture.Builder
	profile *fakeProfileCurrency
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	tdb := dbtest.NewSQLite(t)
	db := tdb.Raw

	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	f := fixture.New(t, tdb).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: seedUserID, Email: seedEmail, Name: seedName, Avatar: seedAvatar, Password: "pw", Salt: seedSalt})
	f.User(fixture.User{ID: otherUserID, Email: "other@example.test", Name: "Other User", Avatar: seedAvatar, Password: "pw", Salt: seedSalt})

	readRepo := currencyrepo.NewReadRepo("sqlite", tdb.TX)
	manageRepo := currencyrepo.NewManageRepo("sqlite", tdb.TX)
	opGuard := operationrepo.NewGuard("sqlite", tdb.TX)
	profile := &fakeProfileCurrency{code: "EUR"}

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}, CurrencyBase: "USD"}
	readSvc := appcurrency.NewReadService(readRepo)
	manageSvc := appcurrency.NewManageService(manageRepo, tdb.TX, opGuard, clk, profile, cfg.CurrencyBase)
	handlers := handlercurrency.NewHandlers(readSvc, manageSvc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handlercurrency.RegisterAPI(handlers, authstub.Authenticator{}, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, clock: clk, f: f, profile: profile}
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

// seedCustomCurrency inserts a currency owned by userID (a per-user custom
// currency, as opposed to seedCurrency's global row).
func (h *harness) seedCustomCurrency(t *testing.T, id, userID, code, symbol string, fractionDigits int) {
	t.Helper()
	fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"}).Currency(fixture.Currency{
		ID: id, Code: code, Symbol: symbol, FractionDigits: &fractionDigits, UserID: userID,
	})
}

func (h *harness) do(t *testing.T, method, path, token string) (int, envelope) {
	t.Helper()
	return h.doJSON(t, method, path, token, nil)
}

func (h *harness) doJSON(t *testing.T, method, path, token string, body any) (int, envelope) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
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
	// authstub: the bearer token IS the user id string.
	return seedUserID
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
