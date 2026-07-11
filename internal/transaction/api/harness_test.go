package api_test

// The transaction service depends on the account service (for the embed + access
// checks), so both are wired here.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	appaccount "github.com/econumo/econumo/internal/account"
	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appcategory "github.com/econumo/econumo/internal/category"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/config"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	apppayee "github.com/econumo/econumo/internal/payee"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	apptag "github.com/econumo/econumo/internal/tag"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	handlertransaction "github.com/econumo/econumo/internal/transaction/api"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
	userrepo "github.com/econumo/econumo/internal/user/repo"
	"github.com/econumo/econumo/internal/web/router"
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

	txm := backend.NewTxManager(db)

	f := fixture.New(t, &dbtest.DB{Raw: db, Engine: "sqlite", TX: txm}).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: seedUserID, Email: seedEmail, Name: seedName, Avatar: seedAvatar, Password: "pw", Salt: seedSalt})
	// A category is needed for non-transfer transactions.
	f.Folder(fixture.Folder{ID: folderID, UserID: seedUserID, Name: "Main"})
	f.Account(fixture.Account{ID: accountID, UserID: seedUserID, CurrencyID: usdID, Name: "Cash"})
	f.AccountInFolder(folderID, accountID)
	f.AccountOption(accountID, seedUserID, 0)
	f.Category(fixture.Category{ID: catID, UserID: seedUserID, Name: "Food", Type: 0, Icon: "local_offer"})

	curLookup := currencyrepo.New("sqlite", txm)
	accSvc := appaccount.NewService(
		accountrepo.NewRepo("sqlite", txm), accountrepo.NewFolderRepo("sqlite", txm),
		server.NewAccountCurrencyLookup(curLookup), server.NewUserOwnerLookup(userrepo.NewRepo("sqlite", txm)),
		nil, nil, txm, operationrepo.NewGuard("sqlite", txm), clock.New(),
	)
	txRepo := transactionrepo.NewRepo("sqlite", txm)
	catRepo := categoryrepo.NewRepo("sqlite", txm)
	tgRepo := tagrepo.NewRepo("sqlite", txm)
	pyRepo := payeerepo.NewRepo("sqlite", txm)
	txExport := transactionrepo.NewExportLookup(txRepo, server.NewTransactionCategoryNameLookup(catRepo), server.NewTransactionTagNameLookup(tgRepo), server.NewTransactionPayeeNameLookup(pyRepo))
	catSvc := appcategory.NewService(catRepo, txm, catRepo, clock.New(), categoryrepo.NewReadRepo("sqlite", txm), connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm)))
	tgSvc := apptag.NewService(tgRepo, txm, operationrepo.NewGuard("sqlite", txm), clock.New(), tagrepo.NewReadRepo("sqlite", txm), connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm)))
	pySvc := apppayee.NewService(pyRepo, txm, operationrepo.NewGuard("sqlite", txm), clock.New(), payeerepo.NewReadRepo("sqlite", txm), connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm)))
	txImportAccounts := server.NewTransactionImportAccounts(
		accSvc, accountrepo.NewRepo("sqlite", txm), accountrepo.NewFolderRepo("sqlite", txm), curLookup, "USD",
	)
	txImportCategories := server.NewTransactionImportCategories(catSvc, catRepo)
	txImportTags := server.NewTransactionImportTags(tgSvc, tgRepo)
	txImportPayees := server.NewTransactionImportPayees(pySvc, pyRepo)
	txImport := transactionrepo.NewImportLookup(
		txImportAccounts, connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm)),
		txImportCategories, txImportPayees, txImportTags, txRepo,
	)
	svc := apptransaction.NewService(
		txRepo, accSvc,
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm)),
		accSvc,
		server.NewUserOwnerLookup(userrepo.NewRepo("sqlite", txm)), txExport, txImport, txm, operationrepo.NewGuard("sqlite", txm), clock.New(),
	)
	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	handlers := handlertransaction.NewHandlers(svc, cfg.IsDev())
	h := router.New(router.Deps{Cfg: cfg, DB: nil, RegisterAPI: handlertransaction.RegisterAPI(handlers, authstub.Authenticator{}, cfg.IsDev())})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &harness{srv: srv, db: db}
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
	// authstub: the bearer token IS the user id string.
	return seedUserID
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
// Access-denied / exception responses emit an empty array ([]) instead of an
// object, which won't unmarshal into a map and leaves the result empty.
func (e envelope) errorsMap() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) > 0 {
		_ = json.Unmarshal(e.Errors, &m)
	}
	return m
}
