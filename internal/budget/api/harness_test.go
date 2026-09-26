package api_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	appbudget "github.com/econumo/econumo/internal/budget"
	handlerbudget "github.com/econumo/econumo/internal/budget/api"
	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/config"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	domcurrency "github.com/econumo/econumo/internal/currency"
	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/clock"
	operationrepo "github.com/econumo/econumo/internal/infra/operation"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/port"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
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

	usdID     = "dffc2a06-6f29-4704-8575-31709adee926" // seeded by baseline
	folderID  = "ffffffff-0000-0000-0000-00000000f01d"
	accountID = "aaaa1111-0000-7000-8000-000000000001"
	catID     = "cccc1111-0000-7000-8000-000000000001"
	tagID     = "dddd1111-0000-7000-8000-000000000001"

	// Ids tests seed themselves (membership scenarios): a second owned account,
	// a soft-deleted one, and an account belonging to otherUserID.
	accountID2     = "aaaa1111-0000-7000-8000-000000000002"
	deadAccountID  = "aaaa1111-0000-7000-8000-000000000003"
	otherAccountID = "aaaa2222-0000-7000-8000-000000000001"
)

type harness struct {
	srv *httptest.Server
	db  *sql.DB
	tdb *dbtest.DB
	f   *fixture.Builder
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWithClock(t, clock.New())
}

// newHarnessWithClock injects the budget-service clock so tests can fix "now"
// (e.g. around a month boundary for timezone-sensitive behaviour).
func newHarnessWithClock(t *testing.T, clk port.Clock) *harness {
	t.Helper()
	return newHarnessWithTx(t, clk, nil)
}

// newHarnessWithTx lets a test wrap the budget service's transaction runner,
// e.g. to run another request between a use case's reads and its transaction.
func newHarnessWithTx(t *testing.T, clk port.Clock, wrapTx func(port.TxRunner) port.TxRunner) *harness {
	t.Helper()
	// The shared opener: production DSN settings (frozen datetime layout,
	// foreign keys, single connection) and every migration.
	tdb := dbtest.NewSQLite(t)
	db := tdb.Raw

	txm := tdb.TX

	// Seed via the shared fixture builder over the same DB handle.
	f := fixture.New(t, tdb).WithCrypto(testDataSalt)
	f.User(fixture.User{ID: seedUserID, Email: seedEmail, Name: seedName, Avatar: seedAvatar, Password: "pw", Salt: seedSalt})
	// A 'budget' users_options row so SetActiveBudget persists (real registered
	// users have it; seed-cmd users don't).
	f.Option(seedUserID, "budget", nil)
	// account + a non-income category + a tag so create-budget seeds elements.
	f.Folder(fixture.Folder{ID: folderID, UserID: seedUserID, Name: "Main"})
	f.Account(fixture.Account{ID: accountID, UserID: seedUserID, CurrencyID: usdID, Name: "Cash"})
	f.AccountInFolder(folderID, accountID)
	f.AccountOption(accountID, seedUserID, 0)
	f.Category(fixture.Category{ID: catID, UserID: seedUserID, Name: "Food", Type: 0, Icon: "local_offer"})
	f.Tag(fixture.Tag{ID: tagID, UserID: seedUserID, Name: "Trip"})

	userRepo := userrepo.NewRepo("sqlite", txm)
	accountRepo := accountrepo.NewRepo("sqlite", txm)
	categoryRepo := categoryrepo.NewRepo("sqlite", txm)
	tagRepo := tagrepo.NewRepo("sqlite", txm)
	payeeRepo := payeerepo.NewRepo("sqlite", txm)
	currencyLookup := currencyrepo.New("sqlite", txm)

	var svcTx port.TxRunner = txm
	if wrapTx != nil {
		svcTx = wrapTx(txm)
	}
	budgetRepo := budgetrepo.NewRepo("sqlite", txm)
	budgetReadRepo := budgetrepo.NewReadRepo("sqlite", txm)
	rateProvider := currencyrepo.NewRateProvider("sqlite", txm, currencyLookup, usdID)
	convertor := domcurrency.NewConvertor(rateProvider)
	svc := appbudget.NewService(
		budgetRepo, budgetReadRepo, convertor, rateProvider,
		server.NewBudgetUserLookup(userRepo, clk),
		server.NewBudgetAccountLookup(accountRepo),
		server.NewBudgetCurrencyLookup(currencyLookup),
		budgetrepo.NewMetadataLookup(server.NewBudgetCategoryMetadataLookup(categoryRepo), server.NewBudgetTagMetadataLookup(tagRepo), server.NewBudgetPayeeMetadataLookup(payeeRepo)),
		connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo("sqlite", txm)),
		operationrepo.NewGuard("sqlite", txm),
		svcTx, clk,
	)

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	handlers := handlerbudget.NewHandlers(svc)
	h := router.New(router.Deps{Cfg: cfg, DB: nil, RegisterAPI: handlerbudget.RegisterAPI(handlers, authstub.Authenticator{})})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &harness{srv: srv, db: db, tdb: tdb, f: f}
}

func (h *harness) token(t *testing.T) string {
	t.Helper()
	// authstub: the bearer token IS the user id string.
	return seedUserID
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
	req, _ := http.NewRequest(method, h.srv.URL+path, rdr)
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
// won't unmarshal into a map and leaves the returned map empty.
func (e envelope) errorsMap() map[string][]string {
	m := map[string][]string{}
	if len(e.Errors) > 0 {
		_ = json.Unmarshal(e.Errors, &m)
	}
	return m
}
