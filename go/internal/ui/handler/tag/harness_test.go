package tag_test

// HTTP test harness for the tag module: open a fresh in-memory sqlite DB per
// test, run the real migrations, seed a user, build the REAL router (global
// middleware + the tag RegisterAPI with real JWT middleware), and exercise it
// through an httptest.Server with the production envelope on the wire.
//
// Fresh-DB-per-test isolation (same rationale as the category harness): the
// request flows through net/http, which builds its own per-request context, so
// the savepoint-rollback-per-test optimization is not wired here. Fresh
// :memory: is correct and cheap.

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

	apptag "github.com/econumo/econumo/internal/app/tag"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	operationrepo "github.com/econumo/econumo/internal/infra/repo/operation"
	tagrepo "github.com/econumo/econumo/internal/infra/repo/tag"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	handlertag "github.com/econumo/econumo/internal/ui/handler/tag"
	"github.com/econumo/econumo/internal/ui/router"
)

// Fixed test data. The JWT keypair comes from the shared testkeys package
// (testkeys.Paths + testkeys.Passphrase).
const (
	testDataSalt = "0123456789abcdef" // 16 bytes -> AES-128

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
	tdb   *dbtest.DB
	jwt   *auth.JWT
	clock fixedClock
}

// newHarness builds a fully-wired tag module over a fresh in-memory sqlite DB
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

	priv, pub := testkeys.Paths(t)
	jwt, err := auth.NewJWT(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	txm := backend.NewTxManager(db)
	tdb := &dbtest.DB{Raw: db, TX: txm, Engine: "sqlite"}

	seedUsers(t, tdb)

	repo := tagrepo.NewSQLiteRepo(txm)
	readRepo := tagrepo.NewReadRepo("sqlite", txm)
	opGuard := operationrepo.NewGuard("sqlite", txm)

	cfg := config.Config{AppEnv: "test", CORSAllowOrigin: "*"}
	svc := apptag.NewService(repo, txm, opGuard, clk, readRepo)
	readSvc := apptag.NewReadService(readRepo)
	handlers := handlertag.NewHandlers(svc, readSvc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handlertag.RegisterAPI(handlers, jwt, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, tdb: tdb, jwt: jwt, clock: clk}
}

// seedUsers inserts the seeded user (the JWT subject) and a second user used to
// test ownership boundaries. Tags are created via the API in each test.
func seedUsers(t *testing.T, tdb *dbtest.DB) {
	t.Helper()
	f := fixture.New(t, tdb).WithCrypto(testDataSalt)
	for _, u := range []struct{ id, email string }{
		{seedUserID, seedEmail},
		{otherUserID, "other@example.test"},
	} {
		f.User(fixture.User{
			ID:       u.id,
			Email:    u.email,
			Name:     seedName,
			Avatar:   seedAvatarURL,
			Password: "pw",
			Salt:     seedSalt,
		})
	}
}

func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

// seedTag inserts a tag row directly (bypassing the API) for the given owner —
// handy for tests that need a pre-existing tag (e.g. ownership setups). The tags
// table has no type/icon columns.
func (h *harness) seedTag(t *testing.T, id, ownerID, name string, position int, archived bool) {
	t.Helper()
	fixture.New(t, h.tdb).Tag(fixture.Tag{
		ID:       id,
		UserID:   ownerID,
		Name:     name,
		Position: position,
		Archived: archived,
	})
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

// tagItem is the wire shape of a TagResult, with exact JSON keys (the tests
// assert key presence + types, including isArchived as a number, the
// "Y-m-d H:i:s" timestamp format, and the ABSENCE of type/icon).
type tagItem struct {
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
