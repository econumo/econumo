package connection_test

// HTTP test harness for the connection module: fresh in-memory sqlite, real
// migrations, two seeded users (owner + guest) with a symmetric connection, the
// owner holding one account. The REAL router with the connection RegisterAPI
// behind real JWT. The connection service depends on the account folder/options
// repos (side effects) and the user repo (embed), all wired here.

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

	appconnection "github.com/econumo/econumo/internal/app/connection"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/clock"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	budgetrepo "github.com/econumo/econumo/internal/infra/repo/budget"
	connectionrepo "github.com/econumo/econumo/internal/infra/repo/connection"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	handlerconnection "github.com/econumo/econumo/internal/ui/handler/connection"
	"github.com/econumo/econumo/internal/ui/router"
	"github.com/econumo/econumo/pkg/jwt"
)

const (
	testDataSalt = "0123456789abcdef"

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

	// A third user NOT connected to owner/guest, for the invite happy-path.
	thirdUserID   = "33333333-3333-3333-3333-333333333333"
	thirdEmail    = "third@example.test"
	thirdName     = "Third User"
	thirdAvatar   = "https://avatar.test/third"
	thirdFolderID = "ffffffff-0000-0000-0000-00000000f03d"
)

type harness struct {
	srv *httptest.Server
	db  *sql.DB
	jwt *jwt.JWT
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	tdb := dbtest.NewSQLite(t)
	db := tdb.Raw

	priv, pub := testkeys.Paths(t)
	jwtSvc, err := jwt.New(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}

	const seedSalt = "0000000000000000000000000000000000000001"
	f := fixture.New(t, tdb).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: ownerUserID, Email: ownerEmail, Name: ownerName, Avatar: ownerAvatar, Password: "pw", Salt: seedSalt})
	f.User(fixture.User{ID: guestUserID, Email: guestEmail, Name: guestName, Avatar: guestAvatar, Password: "pw", Salt: seedSalt})
	f.User(fixture.User{ID: thirdUserID, Email: thirdEmail, Name: thirdName, Avatar: thirdAvatar, Password: "pw", Salt: seedSalt})

	// Symmetric connection between owner and guest.
	f.Connect(ownerUserID, guestUserID)

	// Each user has one folder; owner owns one account.
	f.Folder(fixture.Folder{ID: ownerFolderID, UserID: ownerUserID, Name: "Main", Position: 0})
	f.Folder(fixture.Folder{ID: guestFolderID, UserID: guestUserID, Name: "Main", Position: 0})
	f.Folder(fixture.Folder{ID: thirdFolderID, UserID: thirdUserID, Name: "Main", Position: 0})
	f.Account(fixture.Account{ID: ownerAccount, UserID: ownerUserID, CurrencyID: usdID, Name: "Cash", Type: 2, Icon: "wallet"})
	f.AccountInFolder(ownerFolderID, ownerAccount)
	f.AccountOption(ownerAccount, ownerUserID, 0)

	txm := tdb.TX
	folderRepo := accountrepo.NewFolderRepo("sqlite", txm)
	accountRepo := accountrepo.NewRepo("sqlite", txm)
	svc := appconnection.NewService(
		connectionrepo.NewRepo("sqlite", txm),
		connectionrepo.NewInviteRepo("sqlite", txm),
		connectionrepo.NewFolderPort(folderRepo),
		connectionrepo.NewOptionPort(accountRepo),
		connectionrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm)),
		connectionrepo.NewBudgetAccessRevoker(budgetrepo.NewRepo("sqlite", txm)),
		txm, clock.New(),
	)
	cfg := config.Config{AppEnv: "test", CORSAllowedOrigins: []string{"*"}}
	handlers := handlerconnection.NewHandlers(svc, cfg.IsDev())
	h := router.New(router.Deps{Cfg: cfg, DB: nil, RegisterAPI: handlerconnection.RegisterAPI(handlers, jwtSvc, cfg.IsDev())})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &harness{srv: srv, db: db, jwt: jwtSvc}
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
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
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
