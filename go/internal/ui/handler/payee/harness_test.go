package payee_test

// HTTP test harness for the payee module: open a fresh in-memory sqlite DB per
// test, run the real migrations, seed a user, build the REAL router (global
// middleware + the payee RegisterAPI with real JWT middleware), and exercise it
// through an httptest.Server with the production envelope on the wire.
//
// Fresh-DB-per-test isolation (same rationale as the tag/category harnesses):
// the request flows through net/http, which builds its own per-request context,
// so the savepoint-rollback-per-test optimization is not wired here. Fresh
// :memory: is correct and cheap.

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

	apppayee "github.com/econumo/econumo/internal/app/payee"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	operationrepo "github.com/econumo/econumo/internal/infra/repo/operation"
	payeerepo "github.com/econumo/econumo/internal/infra/repo/payee"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	handlerpayee "github.com/econumo/econumo/internal/ui/handler/payee"
	"github.com/econumo/econumo/internal/ui/router"
)

// Fixed test data. The JWT keypair is the repo dev keypair vendored into
// infra/auth/testdata; referenced by a relative path that resolves from this
// package directory.
const (
	testDataSalt   = "0123456789abcdef" // 16 bytes -> AES-128
	testPassphrase = "d78eedcb16c13bd949ede5d1b8b910cd"
	testPrivateKey = "../../../infra/auth/testdata/private.pem"
	testPublicKey  = "../../../infra/auth/testdata/public.pem"

	seedUserID    = "11111111-1111-1111-1111-111111111111"
	otherUserID   = "22222222-2222-2222-2222-222222222222"
	seedEmail     = "user@example.test"
	seedName      = "Seed User"
	seedSalt      = "0000000000000000000000000000000000000001"
	seedAvatarURL = "https://avatar.test/x"
)

// fixedClock pins issuance time so login tokens are deterministic.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// harness bundles the running server and the collaborators a test needs.
type harness struct {
	srv   *httptest.Server
	db    *sql.DB
	jwt   *auth.JWT
	clock fixedClock
}

// newHarness builds a fully-wired payee module over a fresh in-memory sqlite DB
// with one seeded user (and a second user, to test ownership boundaries).
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

	seedUsers(t, ctx, db, encode, hasher)

	txm := backend.NewTxManager(db)
	repo := payeerepo.NewSQLiteRepo(txm)
	readRepo := payeerepo.NewReadRepo("sqlite", txm)
	opGuard := operationrepo.NewGuard("sqlite", txm)

	cfg := config.Config{AppEnv: "test", CORSAllowOrigin: "*"}
	svc := apppayee.NewService(repo, txm, opGuard, clk, readRepo)
	readSvc := apppayee.NewReadService(readRepo)
	handlers := handlerpayee.NewHandlers(svc, readSvc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handlerpayee.RegisterAPI(handlers, jwt, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, jwt: jwt, clock: clk}
}

// seedUsers inserts the seeded user (the JWT subject) and a second user used to
// test ownership boundaries. Payees are created via the API in each test.
func seedUsers(t *testing.T, ctx context.Context, db *sql.DB, encode *auth.EncodeService, hasher *auth.PasswordHasher) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()

	for _, u := range []struct{ id, email string }{
		{seedUserID, seedEmail},
		{otherUserID, "other@example.test"},
	} {
		identifier := encode.Hash(strings.ToLower(u.email))
		encEmail, err := encode.Encode(u.email)
		if err != nil {
			t.Fatalf("encode email: %v", err)
		}
		passwordHash := hasher.Hash("pw", seedSalt)
		if _, err := db.ExecContext(ctx,
			`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			u.id, identifier, encEmail, seedName, seedAvatarURL, passwordHash, seedSalt, now, now,
		); err != nil {
			t.Fatalf("seed user %s: %v", u.id, err)
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

// seedPayee inserts a payee row directly (bypassing the API) for the given owner
// — handy for tests that need a pre-existing payee (e.g. ownership setups). The
// payees table has no type/icon columns.
func (h *harness) seedPayee(t *testing.T, id, ownerID, name string, position int, archived bool) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	arch := 0
	if archived {
		arch = 1
	}
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO payees (id, user_id, name, position, is_archived, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, ownerID, name, position, arch, now, now,
	); err != nil {
		t.Fatalf("seed payee %s: %v", id, err)
	}
}

// ---- request helpers ----

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

// envelope is the decoded response envelope (success or error).
type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	raw     []byte
}

// payeeItem is the wire shape of a PayeeResult, with exact JSON keys (the tests
// assert key presence + types, including isArchived as a number, the
// "Y-m-d H:i:s" timestamp format, and the ABSENCE of type/icon).
type payeeItem struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"ownerUserId"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	IsArchived  int    `json:"isArchived"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
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
