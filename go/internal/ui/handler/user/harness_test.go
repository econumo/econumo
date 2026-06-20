package user_test

// HTTP test harness for the user module. This is the reference pattern every
// other module's edge tests will copy: open a real sqlite DB, run the real
// migrations, seed a known fixture, build the REAL router (global middleware +
// the user RegisterAPI with real JWT middleware), and exercise it through an
// httptest.Server with the production envelope on the wire.
//
// Isolation strategy (TODO, flagged): each test gets a FRESH in-memory database
// (fresh schema + reseed). The savepoint-rollback-per-test optimization the
// plan describes (open an outer tx, ContextWithTx, roll back at the end) is not
// wired here because the request flows through net/http, which builds its own
// per-request context — there is no seam to inject the harness's outer tx into
// the request context without a custom middleware. Fresh-DB-per-test is correct
// and cheap for sqlite :memory:; switching to the savepoint design is a future
// optimization once a context-injection middleware exists.

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

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	handleruser "github.com/econumo/econumo/internal/ui/handler/user"
	"github.com/econumo/econumo/internal/ui/router"
)

// Fixed test data. The data salt is a 16-byte string (AES-128 requires exactly
// 16 key bytes — see auth.EncodeService). The JWT keys are the repo dev keypair
// vendored into infra/auth/testdata; we reference them by a relative path that
// resolves from this package directory.
const (
	testDataSalt   = "0123456789abcdef" // 16 bytes -> AES-128
	testPassphrase = "d78eedcb16c13bd949ede5d1b8b910cd"
	testPrivateKey = "../../../infra/auth/testdata/private.pem"
	testPublicKey  = "../../../infra/auth/testdata/public.pem"

	seedUserID   = "11111111-1111-1111-1111-111111111111"
	seedEmail    = "user@example.test"
	seedPassword = "secret-pw"
	seedName     = "Seed User"
	seedSalt     = "0000000000000000000000000000000000000001" // 40-char sha1-shaped salt

	// USD is inserted by the baseline migration (20210812210548) with this id;
	// the harness reuses it rather than seeding its own row.
	usdCurrencyID = "dffc2a06-6f29-4704-8575-31709adee926"
)

// fixedClock pins issuance time so login tokens are deterministic.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// harness bundles the running server and the collaborators a test needs to
// craft requests and assert state.
type harness struct {
	srv    *httptest.Server
	db     *sql.DB
	encode *auth.EncodeService
	hasher *auth.PasswordHasher
	jwt    *auth.JWT
	clock  fixedClock
}

// newHarness builds a fully-wired user module over a fresh in-memory sqlite DB
// with one seeded user and a USD currency.
func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	// Fresh in-memory DB, pinned to a single connection so the schema and data
	// survive across queries (cache=shared + 1 conn keeps the :memory: db alive).
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// Run the real sqlite migrations.
	if err := migrate.Run(ctx, db, toMigrations(migrations.SQLite())); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	encode := auth.NewEncodeService(testDataSalt)
	hasher := auth.NewPasswordHasher()
	jwt, err := auth.NewJWT(testPrivateKey, testPublicKey, testPassphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	// Use a near-now issuance time so tokens verify (the JWT verifier checks exp
	// against the real wall clock). Truncated to the second to match the
	// integer-timestamp JWT claims.
	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	seed(t, ctx, db, encode, hasher)

	txm := backend.NewTxManager(db)
	repo := userrepo.NewSQLiteRepo(txm)
	readRepo := userrepo.NewReadRepo("sqlite", txm)
	currency := currencyrepo.New("sqlite", txm)

	cfg := config.Config{AppEnv: "test", CORSAllowOrigin: "*", AllowRegistration: true}
	svc := appuser.NewService(repo, txm, encode, hasher, jwt, currency, clk, cfg.AllowRegistration, cfg.ConnectUsers)
	readSvc := appuser.NewReadService(readRepo, encode)
	handlers := handleruser.NewHandlers(svc, readSvc, cfg.IsDev(), clk)

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handleruser.RegisterAPI(handlers, jwt, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, encode: encode, hasher: hasher, jwt: jwt, clock: clk}
}

// seed inserts the USD currency and a known user (with hashed password and
// encrypted email) so login and get-user-data work.
func seed(t *testing.T, ctx context.Context, db *sql.DB, encode *auth.EncodeService, hasher *auth.PasswordHasher) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()

	// USD currency is provided by the baseline migration; no need to seed it.

	identifier := encode.Hash(strings.ToLower(seedEmail))
	encEmail, err := encode.Encode(seedEmail)
	if err != nil {
		t.Fatalf("encode email: %v", err)
	}
	passwordHash := hasher.Hash(seedPassword, seedSalt)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		seedUserID, identifier, encEmail, seedName, "https://avatar.test/x", passwordHash, seedSalt, now, now,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Seed the default user options (currency + report_period).
	for _, o := range []struct {
		id, name, value string
	}{
		{"33333333-3333-3333-3333-333333333331", "currency", "USD"},
		{"33333333-3333-3333-3333-333333333332", "report_period", "monthly"},
		{"33333333-3333-3333-3333-333333333333", "onboarding", "started"},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			o.id, seedUserID, o.name, o.value, now, now,
		); err != nil {
			t.Fatalf("seed option %s: %v", o.name, err)
		}
	}
}

func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

// ---- request helpers ----

// do issues a request to the harness server. token may be "" for public calls.
// It returns the HTTP status and the decoded envelope.
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

// issueToken mints a valid JWT for the seeded user via the real signer.
func (h *harness) issueToken(t *testing.T) string {
	t.Helper()
	tok, err := h.jwt.Issue(seedUserID, seedEmail, h.clock.Now())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// envelope is the decoded response envelope (success or error). data is left as
// raw JSON so tests pick out the shape they care about.
type envelope struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Code    int                 `json:"code"`
	Data    json.RawMessage     `json:"data"`
	Errors  map[string][]string `json:"errors"`
	raw     []byte
}

// currentUser is the subset of CurrentUserResult the tests assert on.
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

// optionValue returns the value of the named option and whether it was present.
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
