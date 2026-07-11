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
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	apptag "github.com/econumo/econumo/internal/tag"
	handlertag "github.com/econumo/econumo/internal/tag/api"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/web/router"
)

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

type harness struct {
	srv   *httptest.Server
	db    *sql.DB
	tdb   *dbtest.DB
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

	clk := fixedClock{t: time.Now().Truncate(time.Second)}

	txm := backend.NewTxManager(db)
	tdb := &dbtest.DB{Raw: db, TX: txm, Engine: "sqlite"}

	seedUsers(t, tdb)

	repo := tagrepo.NewSQLiteRepo(txm)
	readRepo := tagrepo.NewReadRepo("sqlite", txm)
	opGuard := operationrepo.NewGuard("sqlite", txm)

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	accountAccess := connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm))
	svc := apptag.NewService(repo, txm, opGuard, clk, readRepo, accountAccess)
	readSvc := apptag.NewReadService(readRepo)
	handlers := handlertag.NewHandlers(svc, readSvc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handlertag.RegisterAPI(handlers, authstub.Authenticator{}, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, tdb: tdb, clock: clk}
}

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

// seedTag inserts a tag row directly (bypassing the API) for the given owner,
// for tests that need a pre-existing tag.
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

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	raw     []byte
}

// tagItem is the wire shape of a tag result. Tests assert key presence and
// types, including isArchived as a number, the "2006-01-02 15:04:05" timestamp
// format, and the absence of type/icon.
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
// Access-denied / exception responses emit an empty array ([]) instead of an
// object, which won't unmarshal into a map and leaves the result empty.
func (e envelope) errorsMap() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) > 0 {
		_ = json.Unmarshal(e.Errors, &m)
	}
	return m
}
