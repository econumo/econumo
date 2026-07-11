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

	appcategory "github.com/econumo/econumo/internal/category"
	handlercategory "github.com/econumo/econumo/internal/category/api"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/config"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/web/router"
)

// Fixed test data.
const (
	testDataSalt = "0123456789abcdef" // 16 bytes -> AES-128

	seedUserID  = "11111111-1111-1111-1111-111111111111"
	otherUserID = "22222222-2222-2222-2222-222222222222"
	seedEmail   = "user@example.test"
	seedName    = "Seed User"
	seedSalt    = "0000000000000000000000000000000000000001"
	seedAvatar  = "https://avatar.test/x"
)

// fixedClock pins issuance time so login tokens are deterministic.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// harness bundles the running server and the collaborators a test needs.
type harness struct {
	srv   *httptest.Server
	db    *sql.DB
	tdb   *dbtest.DB
	clock fixedClock
}

// newHarness builds a fully-wired category module over a fresh in-memory sqlite
// DB with one seeded user (and a second user, to test ownership boundaries).
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

	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	txm := backend.NewTxManager(db)
	tdb := &dbtest.DB{Raw: db, TX: txm, Engine: "sqlite"}

	seedUsers(t, tdb)

	repo := categoryrepo.NewSQLiteRepo(txm)
	readRepo := categoryrepo.NewReadRepo("sqlite", txm)

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	accountAccess := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm))
	svc := appcategory.NewService(repo, txm, repo, clk, readRepo, accountAccess)
	readSvc := appcategory.NewReadService(readRepo)
	handlers := handlercategory.NewHandlers(svc, readSvc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handlercategory.RegisterAPI(handlers, authstub.Authenticator{}, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, tdb: tdb, clock: clk}
}

// seedUsers inserts the seeded user (the JWT subject) and a second user used to
// test ownership boundaries. The USD currency is provided by the baseline
// migration; categories are created via the API in each test.
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
			Avatar:   seedAvatar,
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

// seedCategory inserts a category row directly (bypassing the API) for the given
// owner — handy for tests that need a pre-existing category (e.g. ownership or
// delete-replace setups).
func (h *harness) seedCategory(t *testing.T, id, ownerID, name string, position int, typ int, archived bool) {
	t.Helper()
	fixture.New(t, h.tdb).Category(fixture.Category{
		ID:       id,
		UserID:   ownerID,
		Name:     name,
		Position: position,
		Type:     typ,
		Icon:     "local_offer",
		Archived: archived,
	})
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

func (h *harness) issueToken(t *testing.T) string {
	t.Helper()
	// authstub: the bearer token IS the user id string.
	return seedUserID
}

// envelope is the decoded response envelope (success or error). Errors is raw
// because the wire shape varies: the validation path emits an OBJECT
// (field -> []messages) while access-denied/exception paths emit an empty ARRAY
// ([]). Use errorsMap() to read the validation-object form.
type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	raw     []byte
}

// errorsMap decodes the validation-form errors object (field -> messages). It
// returns an empty map when errors is absent or the empty-array ([]) form.
func (e envelope) errorsMap() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) == 0 {
		return m
	}
	_ = json.Unmarshal(e.Errors, &m) // ignores the [] form, leaving m empty
	return m
}

// categoryItem is the wire shape of a CategoryResult, with exact JSON keys (the
// tests assert key presence + types, including isArchived as a number and the
// "Y-m-d H:i:s" timestamp format).
type categoryItem struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"ownerUserId"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Type        string `json:"type"`
	Icon        string `json:"icon"`
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
