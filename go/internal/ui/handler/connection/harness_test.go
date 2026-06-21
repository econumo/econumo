package connection_test

// HTTP test harness for the connection module: fresh in-memory sqlite, real
// migrations, two seeded users (owner + guest) with a symmetric connection, the
// owner holding one account. The REAL router with the connection RegisterAPI
// behind real JWT. The connection service depends on the account folder/options
// repos (side effects) and the user repo (embed), all wired here.

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

	appconnection "github.com/econumo/econumo/internal/app/connection"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	handlerconnection "github.com/econumo/econumo/internal/ui/handler/connection"
	"github.com/econumo/econumo/internal/ui/router"
)

const (
	testDataSalt   = "0123456789abcdef"
	testPassphrase = "d78eedcb16c13bd949ede5d1b8b910cd"
	testPrivateKey = "../../../infra/auth/testdata/private.pem"
	testPublicKey  = "../../../infra/auth/testdata/public.pem"

	ownerUserID = "11111111-1111-1111-1111-111111111111"
	ownerEmail  = "owner@example.test"
	ownerName   = "Owner User"
	ownerAvatar = "https://avatar.test/owner"

	guestUserID = "22222222-2222-2222-2222-222222222222"
	guestEmail  = "guest@example.test"
	guestName   = "Guest User"
	guestAvatar = "https://avatar.test/guest"

	usdID         = "dffc2a06-6f29-4704-8575-31709adee926"
	ownerFolderID = "ffffffff-0000-0000-0000-00000000f01d"
	guestFolderID = "ffffffff-0000-0000-0000-00000000f02d"
	ownerAccount  = "aaaa1111-0000-0000-0000-0000000000a1"
)

type harness struct {
	srv *httptest.Server
	db  *sql.DB
	jwt *auth.JWT
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
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
	now := time.Unix(1690000000, 0).UTC()

	seedUser := func(id, email, name, avatar string) {
		identifier := encode.Hash(strings.ToLower(email))
		encEmail, _ := encode.Encode(email)
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			id, identifier, encEmail, name, avatar, hasher.Hash("pw", "0000000000000000000000000000000000000001"), "0000000000000000000000000000000000000001", now, now); err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
	}
	seedUser(ownerUserID, ownerEmail, ownerName, ownerAvatar)
	seedUser(guestUserID, guestEmail, guestName, guestAvatar)

	// Symmetric connection between owner and guest.
	db.ExecContext(ctx, `INSERT INTO users_connections (user_id, connected_user_id) VALUES (?, ?)`, ownerUserID, guestUserID)
	db.ExecContext(ctx, `INSERT INTO users_connections (user_id, connected_user_id) VALUES (?, ?)`, guestUserID, ownerUserID)

	// Each user has one folder; owner owns one account.
	db.ExecContext(ctx, `INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at) VALUES (?, ?, 'Main', 0, 1, ?, ?)`, ownerFolderID, ownerUserID, now, now)
	db.ExecContext(ctx, `INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at) VALUES (?, ?, 'Main', 0, 1, ?, ?)`, guestFolderID, guestUserID, now, now)
	db.ExecContext(ctx, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'Cash', 2, 'wallet', 0, ?, ?)`, ownerAccount, usdID, ownerUserID, now, now)
	db.ExecContext(ctx, `INSERT INTO accounts_folders (folder_id, account_id) VALUES (?, ?)`, ownerFolderID, ownerAccount)
	db.ExecContext(ctx, `INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at) VALUES (?, ?, 0, ?, ?)`, ownerAccount, ownerUserID, now, now)

	txm := backend.NewTxManager(db)
	folderRepo := accountrepo.NewFolderRepo("sqlite", txm)
	accountRepo := accountrepo.NewRepo("sqlite", txm)
	svc := appconnection.NewService(
		connectionrepo.NewRepo("sqlite", txm),
		connectionrepo.NewFolderPort(folderRepo),
		connectionrepo.NewOptionPort(accountRepo),
		connectionrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm)),
		txm, clock.New(),
	)
	cfg := config.Config{AppEnv: "test", CORSAllowOrigin: "*"}
	handlers := handlerconnection.NewHandlers(svc, cfg.IsDev())
	h := router.New(router.Deps{Cfg: cfg, DB: nil, RegisterAPI: handlerconnection.RegisterAPI(handlers, jwt, cfg.IsDev())})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &harness{srv: srv, db: db, jwt: jwt}
}

func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

func (h *harness) token(t *testing.T, userID, email string) string {
	t.Helper()
	tok, err := h.jwt.Issue(userID, email, time.Now())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return tok
}

// doRaw issues a request and returns the raw body bytes (used for 501 stubs
// whose errors field is [] not {}, which the standard envelope can't decode).
func (h *harness) doRaw(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (h *harness) do(t *testing.T, method, path, token string, body any) (int, envelope) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode (status %d): %v\nbody: %s", resp.StatusCode, err, raw)
		}
	}
	env.raw = raw
	return resp.StatusCode, env
}

type envelope struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Code    int                 `json:"code"`
	Data    json.RawMessage     `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	raw     []byte
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
