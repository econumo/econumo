package user_test

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

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/mailer"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	passwordrequestrepo "github.com/econumo/econumo/internal/infra/repo/passwordrequest"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	userbudgetrepo "github.com/econumo/econumo/internal/infra/repo/userbudget"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/shared/jwt"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	handleruser "github.com/econumo/econumo/internal/ui/handler/user"
	"github.com/econumo/econumo/internal/ui/router"
)

const (
	testDataSalt = "0123456789abcdef" // 16 bytes -> AES-128 requires exactly 16 key bytes

	seedUserID   = "11111111-1111-1111-1111-111111111111"
	seedEmail    = "user@example.test"
	seedPassword = "secret-pw"
	seedName     = "Seed User"
	seedSalt     = "0000000000000000000000000000000000000001" // 40-char sha1-shaped salt

	// USD is inserted by the baseline migration (20210812210548) with this id;
	// the harness reuses it rather than seeding its own row.
	usdCurrencyID = "dffc2a06-6f29-4704-8575-31709adee926"

	// A budget owned by the seed user, used by the update-budget edge test.
	seedBudgetID = "44444444-4444-4444-4444-444444444441"
)

// fixedClock pins issuance time so login tokens are deterministic.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type harness struct {
	srv    *httptest.Server
	db     *sql.DB
	tdb    *dbtest.DB
	encode *auth.EncodeService
	hasher *auth.PasswordHasher
	jwt    *jwt.JWT
	clock  fixedClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	// Pinned to a single connection so the schema and data survive across queries
	// (cache=shared + 1 conn keeps the :memory: db alive).
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
	priv, pub := testkeys.Paths(t)
	jwtSvc, err := jwt.New(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	// Use a near-now issuance time so tokens verify (the JWT verifier checks exp
	// against the real wall clock). Truncated to the second to match the
	// integer-timestamp JWT claims.
	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	txm := backend.NewTxManager(db)
	tdb := &dbtest.DB{Raw: db, TX: txm, Engine: "sqlite"}

	seed(t, tdb)

	repo := userrepo.NewSQLiteRepo(txm)
	readRepo := userrepo.NewReadRepo("sqlite", txm)
	currency := currencyrepo.New("sqlite", txm)
	budgets := userbudgetrepo.New("sqlite", txm)
	passwordReqs := passwordrequestrepo.New("sqlite", txm)
	// Discard mailer — the reset test reads the code from the DB, so email output
	// is irrelevant here (and we keep it off stdout, unlike the console default).
	resetMailer := mailer.NewResetSender(discardMailer{}, "", "")

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}, AllowRegistration: true}
	svc := appuser.NewService(repo, txm, encode, hasher, jwtSvc, currency, budgets, passwordReqs, resetMailer, clk, cfg.AllowRegistration)
	readSvc := appuser.NewReadService(readRepo, encode)
	handlers := handleruser.NewHandlers(svc, readSvc, cfg.IsDev(), clk)

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handleruser.RegisterAPI(handlers, jwtSvc, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, tdb: tdb, encode: encode, hasher: hasher, jwt: jwtSvc, clock: clk}
}

// discardMailer drops every message; it keeps the reset test silent (the console
// default would print to stdout) without re-exposing a no-op transport in prod.
type discardMailer struct{}

func (discardMailer) Send(context.Context, mailer.Message) error { return nil }

// seed inserts a known user (with hashed password and encrypted email) plus the
// four default user options so login and get-user-data work. The budget option
// is seeded with a NULL value (matching the production seed for a user with no
// default budget); it must be PRESENT so UpdateBudget — which only sets an
// existing option — can write to it.
func seed(t *testing.T, tdb *dbtest.DB) {
	t.Helper()
	f := fixture.New(t, tdb).WithCrypto(testDataSalt)
	f.User(fixture.User{
		ID:       seedUserID,
		Email:    seedEmail,
		Name:     seedName,
		Avatar:   "https://avatar.test/x",
		Password: seedPassword,
		Salt:     seedSalt,
	})
	f.DefaultOptions(seedUserID)
}

// seedBudget inserts a budget row owned by the seed user (id seedBudgetID) so
// the update-budget success path has an existing budget to point at. Kept out of
// the default seed so budget-less tests exercise the empty-option path.
func (h *harness) seedBudget(t *testing.T) {
	t.Helper()
	f := fixture.New(t, h.tdb)
	f.Budget(fixture.Budget{
		ID:         seedBudgetID,
		UserID:     seedUserID,
		CurrencyID: usdCurrencyID,
		Name:       "Test Budget",
	})
}

func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

// do issues a request to the harness server. token may be "" for public calls.
func (h *harness) do(t *testing.T, method, path, token string, body any) (int, envelope) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
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

// currentUser is the subset of the current-user result the tests assert on.
type currentUser struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Avatar       string `json:"avatar"`
	Currency     string `json:"currency"`
	ReportPeriod string `json:"reportPeriod"`
	Options      []struct {
		Name  string  `json:"name"`
		Value *string `json:"value"`
	} `json:"options"`
}

func (c currentUser) optionValue(name string) (*string, bool) {
	for _, o := range c.Options {
		if o.Name == name {
			return o.Value, true
		}
	}
	return nil, false
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
// Access-denied / exception responses emit an empty array ([]) instead of an
// object, which won't unmarshal into a map and leaves the result empty.
func (e envelope) errorsMap() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) > 0 {
		_ = json.Unmarshal(e.Errors, &m)
	}
	return m
}
