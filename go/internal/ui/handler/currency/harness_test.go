package currency_test

// HTTP test harness for the currency module: open a fresh in-memory sqlite DB
// per test, run the real migrations, seed a user + a handful of currencies and
// rates, build the REAL router (global middleware + the currency RegisterAPI
// with real JWT middleware), and exercise it through an httptest.Server with the
// production envelope on the wire.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	appcurrency "github.com/econumo/econumo/internal/app/currency"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	handlercurrency "github.com/econumo/econumo/internal/ui/handler/currency"
	"github.com/econumo/econumo/internal/ui/router"
)

const (
	testDataSalt   = "0123456789abcdef"
	testPassphrase = "d78eedcb16c13bd949ede5d1b8b910cd"
	testPrivateKey = "../../../infra/auth/testdata/private.pem"
	testPublicKey  = "../../../infra/auth/testdata/public.pem"

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
	jwt   *auth.JWT
	clock fixedClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if err := migrate.Run(ctx, db, toMigrations(migrations.SQLite())); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encode := auth.NewEncodeService(testDataSalt)
	hasher := auth.NewPasswordHasher()
	jwt, err := auth.NewJWT(testPrivateKey, testPublicKey, testPassphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	seedUser(t, ctx, db, encode, hasher)

	txm := backend.NewTxManager(db)
	readRepo := currencyrepo.NewReadRepo("sqlite", txm)

	cfg := config.Config{AppEnv: "test", CORSAllowOrigin: "*"}
	readSvc := appcurrency.NewReadService(readRepo)
	handlers := handlercurrency.NewHandlers(readSvc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handlercurrency.RegisterAPI(handlers, jwt, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, jwt: jwt, clock: clk}
}

func seedUser(t *testing.T, ctx context.Context, db *sql.DB, encode *auth.EncodeService, hasher *auth.PasswordHasher) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	identifier := encode.Hash(strings.ToLower(seedEmail))
	encEmail, err := encode.Encode(seedEmail)
	if err != nil {
		t.Fatalf("encode email: %v", err)
	}
	passwordHash := hasher.Hash("pw", seedSalt)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		seedUserID, identifier, encEmail, seedName, seedAvatarURL, passwordHash, seedSalt, now, now,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
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
// name is always resolved from the Intl table by code).
func (h *harness) seedCurrency(t *testing.T, id, code, symbol string, fractionDigits int) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO currencies (id, code, symbol, name, fraction_digits, created_at)
		 VALUES (?, ?, ?, NULL, ?, ?)`,
		id, code, symbol, fractionDigits, now,
	); err != nil {
		t.Fatalf("seed currency %s: %v", code, err)
	}
}

// seedRate inserts a currency_rates row published on the given date (YYYY-MM-DD).
func (h *harness) seedRate(t *testing.T, id, currencyID, baseID, publishedAt, rate string) {
	t.Helper()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES (?, ?, ?, ?, ?)`,
		id, currencyID, baseID, publishedAt, rate,
	); err != nil {
		t.Fatalf("seed rate %s: %v", id, err)
	}
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
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Code    int                 `json:"code"`
	Data    json.RawMessage     `json:"data"`
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
// leaves the returned map empty. Added because the access-denied envelope's
// errors is [] (PHP shape), which won't unmarshal into a map.
func (e envelope) errorsMap() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) > 0 {
		_ = json.Unmarshal(e.Errors, &m)
	}
	return m
}
