package account_test

// HTTP test harness for the account+folder module: fresh in-memory sqlite per
// test, real migrations, a seeded user + currency, the REAL router with the
// account RegisterAPI behind real JWT, exercised through httptest.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	appaccount "github.com/econumo/econumo/internal/app/account"
	appconnection "github.com/econumo/econumo/internal/app/connection"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/clock"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	budgetrepo "github.com/econumo/econumo/internal/infra/repo/budget"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	operationrepo "github.com/econumo/econumo/internal/infra/repo/operation"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	handleraccount "github.com/econumo/econumo/internal/ui/handler/account"
	"github.com/econumo/econumo/internal/ui/router"
	"github.com/econumo/econumo/pkg/jwt"
)

const (
	testDataSalt = "0123456789abcdef"

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
	jwt *jwt.JWT
	f   *fixture.Builder
}

func newHarness(t *testing.T) *harness {
	return newHarnessWithClock(t, clock.New())
}

// newHarnessWithClock is newHarness with an injectable account-service clock, for
// tests that need a deterministic "now" (the balance day boundary depends on it).
func newHarnessWithClock(t *testing.T, clk appaccount.Clock) *harness {
	t.Helper()

	tdb := dbtest.NewSQLite(t)
	db := tdb.Raw

	priv, pub := testkeys.Paths(t)
	jwtSvc, err := jwt.New(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}

	f := fixture.New(t, tdb).WithCrypto(testDataSalt)
	seedUsers(t, f)
	f.Folder(fixture.Folder{ID: seedFolderID, UserID: seedUserID, Name: "Main", Position: 0})

	txm := tdb.TX
	repo := accountrepo.NewRepo("sqlite", txm)
	folderRepo := accountrepo.NewFolderRepo("sqlite", txm)
	curLookup := currencyrepo.New("sqlite", txm)
	accCur := accountrepo.NewCurrencyLookup(curLookup)
	accUser := accountrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm))
	opGuard := operationrepo.NewGuard("sqlite", txm)

	cfg := config.Config{AppEnv: "test", CORSAllowedOrigins: []string{"*"}}
	// Wire the real connection module so sharedAccess[] + the delete-account
	// non-owner revoke branch are exercised against actual accounts_access rows.
	connRepo := connectionrepo.NewRepo("sqlite", txm)
	connSvc := appconnection.NewService(
		connRepo, connectionrepo.NewInviteRepo("sqlite", txm),
		connectionrepo.NewFolderPort(folderRepo), connectionrepo.NewOptionPort(repo),
		connectionrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm)),
		connectionrepo.NewBudgetAccessRevoker(budgetrepo.NewRepo("sqlite", txm)), txm, clock.New(),
	)
	sharedLookup := connectionrepo.NewSharedAccessLookup(connRepo)
	revoker := connectionrepo.NewAccessRevoker(connRepo, connSvc)
	svc := appaccount.NewService(repo, folderRepo, accCur, accUser, sharedLookup, revoker, txm, opGuard, clk)
	handlers := handleraccount.NewHandlers(svc, cfg.IsDev())

	h := router.New(router.Deps{
		Cfg:         cfg,
		DB:          nil,
		RegisterAPI: handleraccount.RegisterAPI(handlers, jwtSvc, cfg.IsDev()),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &harness{srv: srv, db: db, jwt: jwtSvc, f: f}
}

func seedUsers(t *testing.T, f *fixture.Builder) {
	t.Helper()
	for _, u := range []struct{ id, email string }{
		{seedUserID, seedEmail},
		{otherUserID, "other@example.test"},
	} {
		f.User(fixture.User{ID: u.id, Email: u.email, Name: seedName, Avatar: seedAvatar, Password: "pw", Salt: seedSalt})
	}
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
	return h.doH(t, method, path, token, body, nil)
}

// doH is do with optional extra request headers (e.g. X-Timezone).
func (h *harness) doH(t *testing.T, method, path, token string, body any, headers map[string]string) (int, envelope) {
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
	for k, v := range headers {
		req.Header.Set(k, v)
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
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
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
	h.f.Account(fixture.Account{ID: id, UserID: ownerID, CurrencyID: usdID, Name: name, Type: 2, Icon: "wallet"})
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
