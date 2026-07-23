package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/econumo/econumo/internal/config"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/handoff"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	handleruser "github.com/econumo/econumo/internal/user/api"
	userrepo "github.com/econumo/econumo/internal/user/repo"
	"github.com/econumo/econumo/internal/web/router"
)

const (
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
	tokens *userrepo.AccessTokenRepo
	clock  fixedClock
	mail   *recordingMailer
}

func newHarness(t *testing.T) *harness { return newHarnessWithLimiter(t, nil) }

// newHarnessWithLimiter lets rate-limit tests inject a tight limiter; every
// other test keeps the nil (disabled) default.
func newHarnessWithLimiter(t *testing.T, limiter appuser.AttemptLimiter) *harness {
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

	encode := auth.NewEncodeService("") // salt-free, matching server.BuildAPI
	hasher := auth.NewPasswordHasher()
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
	budgets := server.NewUserBudgetAccess("sqlite", txm)
	passwordReqs := userrepo.NewPasswordRequestRepo("sqlite", txm)
	// Recording mailer — reset codes are hashed at rest, so the plaintext is only
	// available from the email. The reset test reads it from here.
	rec := &recordingMailer{}
	resetMailer := mailer.NewResetSender(rec, "", "")

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}, AllowRegistration: true}
	tokens := userrepo.NewAccessTokenRepo("sqlite", txm)
	svc := appuser.NewService(repo, txm, encode, hasher, tokens, currency, budgets, passwordReqs, resetMailer,
		userrepo.NewEmailVerificationRepo("sqlite", txm), nil,
		userrepo.NewEmailChangeRequestRepo("sqlite", txm), nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clk, limiter, cfg.AllowRegistration, "", false)
	readSvc := appuser.NewReadService(readRepo, encode, clk)
	billing := appuser.NewBillingService(
		"https://pay.example.test/cloud/",
		handoff.NewSigner("0123456789abcdef0123456789abcdef"),
		clk,
	)
	handlers := handleruser.NewHandlers(svc, readSvc, clk, billing)

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handleruser.RegisterAPI(handlers, svc),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, tdb: tdb, encode: encode, hasher: hasher, tokens: tokens, clock: clk, mail: rec}
}

// recordingMailer captures every sent message so tests can recover the emitted
// reset code (which is hashed at rest and no longer readable from the DB).
type recordingMailer struct{ last mailer.Message }

func (m *recordingMailer) Send(_ context.Context, msg mailer.Message) error {
	m.last = msg
	return nil
}

// resetCodeRe matches the 6-digit reset code in the rendered email body. It is
// anchored on the body's marker text so a digit-bearing user name can never be
// mistaken for the code.
var resetCodeRe = regexp.MustCompile(`code is: (\d{6})`)

// lastResetCode extracts the reset code from the most recently sent email.
func (m *recordingMailer) lastResetCode(t *testing.T) string {
	t.Helper()
	match := resetCodeRe.FindStringSubmatch(m.last.Text)
	if match == nil {
		t.Fatalf("no reset code found in email body: %q", m.last.Text)
	}
	return match[1]
}

// seed inserts a known user (with hashed password and encrypted email) plus the
// four default user options so login and get-user-data work. The budget option
// is seeded with a NULL value (matching the production seed for a user with no
// default budget); it must be PRESENT so UpdateBudget — which only sets an
// existing option — can write to it.
func seed(t *testing.T, tdb *dbtest.DB) {
	t.Helper()
	f := fixture.New(t, tdb).WithCrypto("")
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
// doHeader is do() for callers that need a response header rather than the body.
func (h *harness) doHeader(t *testing.T, method, path string, body any, header string) (int, string) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get(header)
}

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

// issueToken seeds a live session row for the seeded user and returns its raw
// bearer token (unique per call; the hash is what lands in access_tokens).
func (h *harness) issueToken(t *testing.T) string {
	t.Helper()
	return h.issueTokenFor(t, seedUserID)
}

func (h *harness) issueTokenFor(t *testing.T, userID string) string {
	t.Helper()
	raw := "eco_ses_" + vo.NewId().String()
	now := h.clock.Now()
	exp := now.Add(appuser.SessionTTL)
	tok := &model.AccessToken{
		ID: vo.NewId(), UserID: vo.MustParseId(userID), Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(raw), CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp,
	}
	if err := h.tokens.Insert(context.Background(), tok); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return raw
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
	AccessLevel  string `json:"accessLevel"`
	AccessUntil  string `json:"accessUntil"`
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
