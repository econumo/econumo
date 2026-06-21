package transaction_test

// HTTP test harness for the transaction module: fresh in-memory sqlite, real
// migrations, seeded user + currency + folder + account, the REAL router with
// the transaction RegisterAPI behind real JWT. The transaction service depends
// on the account service (for the embed + access), so both are wired here.

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

	appaccount "github.com/econumo/econumo/internal/app/account"
	appcategory "github.com/econumo/econumo/internal/app/category"
	apppayee "github.com/econumo/econumo/internal/app/payee"
	apptag "github.com/econumo/econumo/internal/app/tag"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	categoryrepo "github.com/econumo/econumo/internal/infra/repo/category"
	currencyrepo "github.com/econumo/econumo/internal/infra/repo/currency"
	operationrepo "github.com/econumo/econumo/internal/infra/repo/operation"
	payeerepo "github.com/econumo/econumo/internal/infra/repo/payee"
	tagrepo "github.com/econumo/econumo/internal/infra/repo/tag"
	transactionrepo "github.com/econumo/econumo/internal/infra/repo/transaction"
	userrepo "github.com/econumo/econumo/internal/infra/repo/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	handlertransaction "github.com/econumo/econumo/internal/ui/handler/transaction"
	"github.com/econumo/econumo/internal/ui/router"
)

const (
	testDataSalt = "0123456789abcdef"

	seedUserID = "11111111-1111-1111-1111-111111111111"
	seedEmail  = "user@example.test"
	seedName   = "Seed User"
	seedSalt   = "0000000000000000000000000000000000000001"
	seedAvatar = "https://avatar.test/x"
	usdID      = "dffc2a06-6f29-4704-8575-31709adee926"
	folderID   = "ffffffff-0000-0000-0000-00000000f01d"
	accountID  = "aaaa1111-0000-0000-0000-0000000000a1"
	catID      = "cccc2222-0000-0000-0000-0000000000c1"
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

	priv, pub := testkeys.Paths(t)
	jwt, err := auth.NewJWT(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}

	txm := backend.NewTxManager(db)

	// Seed via the shared fixture builder over the same DB handle.
	f := fixture.New(t, &dbtest.DB{Raw: db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: seedUserID, Email: seedEmail, Name: seedName, Avatar: seedAvatar, Password: "pw", Salt: seedSalt})
	// folder + account + a category (for non-transfer transactions).
	f.Folder(fixture.Folder{ID: folderID, UserID: seedUserID, Name: "Main"})
	f.Account(fixture.Account{ID: accountID, UserID: seedUserID, CurrencyID: usdID, Name: "Cash"})
	f.AccountInFolder(folderID, accountID)
	f.AccountOption(accountID, seedUserID, 0)
	f.Category(fixture.Category{ID: catID, UserID: seedUserID, Name: "Food", Type: 0, Icon: "local_offer"})

	curLookup := currencyrepo.New("sqlite", txm)
	accSvc := appaccount.NewService(
		accountrepo.NewRepo("sqlite", txm), accountrepo.NewFolderRepo("sqlite", txm),
		accountrepo.NewCurrencyLookup(curLookup), accountrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm)),
		nil, nil, txm, operationrepo.NewGuard("sqlite", txm), clock.New(),
	)
	txRepo := transactionrepo.NewRepo("sqlite", txm)
	catRepo := categoryrepo.NewRepo("sqlite", txm)
	tgRepo := tagrepo.NewRepo("sqlite", txm)
	pyRepo := payeerepo.NewRepo("sqlite", txm)
	txExport := transactionrepo.NewExportLookup(txRepo, catRepo, tgRepo, pyRepo)
	catSvc := appcategory.NewService(catRepo, txm, catRepo, clock.New(), categoryrepo.NewReadRepo("sqlite", txm))
	tgSvc := apptag.NewService(tgRepo, txm, operationrepo.NewGuard("sqlite", txm), clock.New(), tagrepo.NewReadRepo("sqlite", txm))
	pySvc := apppayee.NewService(pyRepo, txm, operationrepo.NewGuard("sqlite", txm), clock.New(), payeerepo.NewReadRepo("sqlite", txm))
	txImport := transactionrepo.NewImportLookup(
		accSvc, accountrepo.NewRepo("sqlite", txm), accountrepo.NewFolderRepo("sqlite", txm),
		catSvc, pySvc, tgSvc, catRepo, tgRepo, pyRepo, curLookup, txRepo, "USD",
	)
	svc := apptransaction.NewService(
		txRepo, transactionrepo.NewAccountResolver(accSvc), transactionrepo.NewVisibleAccounts(accSvc),
		transactionrepo.NewUserLookup(userrepo.NewRepo("sqlite", txm)), txExport, txImport, txm, operationrepo.NewGuard("sqlite", txm), clock.New(),
	)
	cfg := config.Config{AppEnv: "test", CORSAllowOrigin: "*"}
	handlers := handlertransaction.NewHandlers(svc, cfg.IsDev())
	h := router.New(router.Deps{Cfg: cfg, DB: nil, RegisterAPI: handlertransaction.RegisterAPI(handlers, jwt, cfg.IsDev())})
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

func (h *harness) token(t *testing.T) string {
	t.Helper()
	tok, err := h.jwt.Issue(seedUserID, seedEmail, time.Now())
	if err != nil {
		t.Fatalf("token: %v", err)
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
