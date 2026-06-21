package account_test

// HTTP test harness for the account+folder module: fresh in-memory sqlite per
// test, real migrations, a seeded user + currency, the REAL router with the
// account RegisterAPI behind real JWT, exercised through httptest.

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

	appaccount "github.com/econumo/econumo/internal/app/account"
	appconnection "github.com/econumo/econumo/internal/app/connection"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	operationrepo "github.com/econumo/econumo/internal/infra/repo/operation"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	handleraccount "github.com/econumo/econumo/internal/ui/handler/account"
	"github.com/econumo/econumo/internal/ui/router"
)

const (
	testDataSalt   = "0123456789abcdef"
	testPassphrase = "d78eedcb16c13bd949ede5d1b8b910cd"
	testPrivateKey = "../../../infra/auth/testdata/private.pem"
	testPublicKey  = "../../../infra/auth/testdata/public.pem"

	seedUserID  = "11111111-1111-1111-1111-111111111111"
	otherUserID = "22222222-2222-2222-2222-222222222222"
	seedEmail   = "user@example.test"
	seedName    = "Seed User"
	seedSalt    = "0000000000000000000000000000000000000001"
	seedAvatar  = "https://avatar.test/x"

	// USD is seeded by the baseline migration (dffc2a06-...).
	usdID = "dffc2a06-6f29-4704-8575-31709adee926"

	seedFolderID = "ffffffff-0000-0000-0000-00000000f01d"
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

	seedUsers(t, ctx, db, encode, hasher)
	seedFolder(t, ctx, db, seedFolderID, seedUserID, "Main", 0)

	txm := backend.NewTxManager(db)
	repo := accountrepo.NewRepo("sqlite", txm)
	folderRepo := accountrepo.NewFolderRepo("sqlite", txm)
	curLookup := currencyrepo.New("sqlite", txm)
	accCur := accountrepo.NewCurrencyLookup(curLookup)
	accUser := accountrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm))
	opGuard := operationrepo.NewGuard("sqlite", txm)

	cfg := config.Config{AppEnv: "test", CORSAllowOrigin: "*"}
	// Wire the real connection module so sharedAccess[] + the delete-account
	// non-owner revoke branch are exercised against actual accounts_access rows.
	connRepo := connectionrepo.NewRepo("sqlite", txm)
	connSvc := appconnection.NewService(
		connRepo, connectionrepo.NewFolderPort(folderRepo), connectionrepo.NewOptionPort(repo),
		connectionrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm)), txm, clock.New(),
	)
	sharedLookup := connectionrepo.NewSharedAccessLookup(connRepo)
	revoker := connectionrepo.NewAccessRevoker(connRepo, connSvc)
	svc := appaccount.NewService(repo, folderRepo, accCur, accUser, sharedLookup, revoker, txm, opGuard, clock.New())
	handlers := handleraccount.NewHandlers(svc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handleraccount.RegisterAPI(handlers, jwt, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, jwt: jwt}
}

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
		if _, err := db.ExecContext(ctx,
			`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			u.id, identifier, encEmail, seedName, seedAvatar, hasher.Hash("pw", seedSalt), seedSalt, now, now,
		); err != nil {
			t.Fatalf("seed user %s: %v", u.id, err)
		}
	}
}

func seedFolder(t *testing.T, ctx context.Context, db *sql.DB, id, userID, name string, position int) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?)`,
		id, userID, name, position, now, now,
	); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
}

func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

func (h *harness) token(t *testing.T) string {
	t.Helper()
	tok, err := h.jwt.Issue(seedUserID, seedEmail, time.Now())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (h *harness) do(t *testing.T, method, path, token string, body any) (int, envelope) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
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

type envelope struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Code    int                 `json:"code"`
	Data    json.RawMessage     `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	raw     []byte
}

// accountItem mirrors AccountResult's wire shape for assertions.
type accountItem struct {
	ID    string `json:"id"`
	Owner struct {
		ID     string `json:"id"`
		Avatar string `json:"avatar"`
		Name   string `json:"name"`
	} `json:"owner"`
	FolderID *string `json:"folderId"`
	Name     string  `json:"name"`
	Position int     `json:"position"`
	Currency struct {
		ID             string `json:"id"`
		Code           string `json:"code"`
		Name           string `json:"name"`
		Symbol         string `json:"symbol"`
		FractionDigits int    `json:"fractionDigits"`
	} `json:"currency"`
	Balance      string `json:"balance"`
	Type         int    `json:"type"`
	Icon         string `json:"icon"`
	SharedAccess []any  `json:"sharedAccess"`
}

type folderItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	IsVisible int    `json:"isVisible"`
}

func mustUnmarshal[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal %T: %v\nraw: %s", v, err, raw)
	}
	return v
}

func mustDecode(t *testing.T, raw json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode %T: %v\nraw: %s", v, err, raw)
	}
}

// seedAccount inserts an account row directly (bypassing the API), for ownership
// tests. Always CREDIT_CARD, USD, not deleted.
func (h *harness) seedAccount(t *testing.T, id, ownerID, name string) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 2, 'wallet', 0, ?, ?)`,
		id, usdID, ownerID, name, now, now,
	); err != nil {
		t.Fatalf("seed account %s: %v", id, err)
	}
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
