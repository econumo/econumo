//go:build enginecompare

package enginecompare

// API-level engine-parity harness. Unlike the repo-level scenarios in
// scenarios_test.go (which compare a single repository call's output), this
// harness stands up the REAL production HTTP handler (internal/server.BuildAPI —
// the identical router cmd/econumo serves) over a given engine's database, seeds
// an identical fixture, and lets a scenario replay a catalogue of HTTP requests.
// runAPIOnBoth runs the same scenario on SQLite and PostgreSQL and asserts every
// response is byte-identical, with SQLite as the reference (the target engine).
//
// Why this is the strongest parity contract we have: it exercises the entire
// stack — middleware, JWT, the per-engine sqlc query adapters, decimal/datetime
// handling, and the envelope serialization — and compares the actual wire bytes
// a client would receive. Any divergence between the two engine adapters that is
// observable through the API surfaces here.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/testkeys"
	"github.com/econumo/econumo/pkg/jwt"
)

// Fixed crypto material shared by both engines so seeded users + minted tokens
// are identical. The data salt is exactly 16 bytes (AES-128). The JWT keypair
// comes from the shared testkeys helper (embedded, written to a temp file) so no
// fragile relative path to it is needed.
const (
	apiDataSalt     = "0123456789abcdef"
	apiSeedPassword = "secret-pw"
)

// apiFixedClock pins issuance + persistence time so tokens and any created-row
// timestamps are deterministic and identical across engines.
type apiFixedClock struct{ t time.Time }

func (c apiFixedClock) Now() time.Time { return c.t }

// apiHarness bundles the running production handler over one engine plus the
// collaborators a scenario needs to craft authenticated requests.
type apiHarness struct {
	srv    *httptest.Server
	engine string
	jwt    *jwt.JWT
	clock  apiFixedClock
}

// newAPIHarness builds the full production API over the given (already-migrated)
// engine DB, seeds the shared fixture, and returns a harness with a live server.
func newAPIHarness(t *testing.T, db *dbtest.DB) *apiHarness {
	t.Helper()

	priv, pub := testkeys.Paths(t)
	jwtSvc, err := jwt.New(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	// Near-now issuance so tokens verify against the real wall clock; truncated to
	// the second to match the integer-timestamp JWT claims. The SAME instant is
	// used on both engines (passed in by runAPIOnBoth) so created-row timestamps
	// match too.
	clk := apiFixedClock{t: apiClockTime}

	cfg := config.Config{
		DatabaseDriver:     db.Engine, // "sqlite" | "postgresql" — selects sqlc adapters
		CurrencyBase:       "USD",
		AllowRegistration:  true,
		DataSalt:           apiDataSalt,
		CORSAllowedOrigins: []string{"*"},
	}

	seedAPIFixture(t, db)

	handler := server.BuildAPI(cfg, db.Raw, jwtSvc, clk)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &apiHarness{srv: srv, engine: db.Engine, jwt: jwtSvc, clock: clk}
}

// apiClockTime is the fixed instant used for token issuance + any created rows.
// Truncated to the second; near "now" so the JWT exp (iat + 30d) is still valid
// when the verifier checks against the real wall clock during the test run.
var apiClockTime = time.Now().UTC().Truncate(time.Second)

// token mints a valid JWT for one of the seeded users via the real signer.
func (h *apiHarness) token(t *testing.T, userID, email string) string {
	t.Helper()
	tok, err := h.jwt.Issue(userID, email, h.clock.Now())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// call issues an HTTP request to the harness server and returns the status code
// and the RAW response body bytes (not decoded), which is what the parity
// comparison diffs. token may be "" for public endpoints. body may be nil.
func (h *apiHarness) call(t *testing.T, method, path, token string, body any) (int, []byte) {
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
	return resp.StatusCode, raw
}

// ---- shared fixture ----

// Identity + entity ids used across the catalogue. UUIDs are literal so both
// engines store the same keys; the comparison is over the API RESPONSES.
const (
	apiOwnerID    = "11111111-1111-1111-1111-111111111111"
	apiOwnerEmail = "owner@example.test"
	apiGuestID    = "22222222-2222-2222-2222-222222222222"
	apiGuestEmail = "guest@example.test"

	apiUSD = "dffc2a06-6f29-4704-8575-31709adee926" // seeded by the baseline migration

	apiOwnerFolder   = "f0000000-0000-0000-0000-000000000001"
	apiGuestFolder   = "f0000000-0000-0000-0000-000000000002"
	apiOwnerAccount  = "a0000000-0000-0000-0000-000000000001"
	apiSharedAccount = "a0000000-0000-0000-0000-000000000002" // owned by guest, shared to owner

	apiCatFood   = "c0000000-0000-0000-0000-000000000001"
	apiCatSalary = "c0000000-0000-0000-0000-000000000002"
	apiTagWork   = "10000000-0000-0000-0000-000000000001"
	apiPayeeShop = "20000000-0000-0000-0000-000000000001"

	apiTxn1 = "d0000000-0000-0000-0000-000000000001"
	apiTxn2 = "d0000000-0000-0000-0000-000000000002"

	apiBudget = "b0000000-0000-0000-0000-000000000001"
)

// seedAPIFixture seeds an identical, cross-module fixture into the given engine
// via the typed fixture builder. It seeds: two connected users (with hashed
// password + encrypted email so login works), their default options, folders, an
// owned account and a guest-owned account shared to the owner, categories, a tag,
// a payee, two transactions, and a budget — so every read endpoint returns
// non-empty data on both engines. Fixed ids are used (the scenarios reference
// them in request bodies); the comparison is over the API RESPONSES.
func seedAPIFixture(t *testing.T, db *dbtest.DB) {
	t.Helper()
	f := fixture.New(t, db).WithCrypto(apiDataSalt)

	f.User(fixture.User{ID: apiOwnerID, Email: apiOwnerEmail, Name: "User " + apiOwnerID[:4], Password: apiSeedPassword})
	f.DefaultOptions(apiOwnerID)
	f.User(fixture.User{ID: apiGuestID, Email: apiGuestEmail, Name: "User " + apiGuestID[:4], Password: apiSeedPassword})
	f.DefaultOptions(apiGuestID)
	f.Connect(apiOwnerID, apiGuestID)

	// Folders.
	f.Folder(fixture.Folder{ID: apiOwnerFolder, UserID: apiOwnerID, Name: "Main"})
	f.Folder(fixture.Folder{ID: apiGuestFolder, UserID: apiGuestID, Name: "Main"})

	// Owner's own account.
	f.Account(fixture.Account{ID: apiOwnerAccount, UserID: apiOwnerID, CurrencyID: apiUSD, Name: "Cash"})
	f.AccountInFolder(apiOwnerFolder, apiOwnerAccount)
	f.AccountOption(apiOwnerAccount, apiOwnerID, 0)

	// Guest-owned account, SHARED to the owner (accounts_access grant) so the
	// owner's get-account-list / sharedAccess[] / connection list are non-empty.
	f.Account(fixture.Account{ID: apiSharedAccount, UserID: apiGuestID, CurrencyID: apiUSD, Name: "Shared"})
	f.AccountInFolder(apiGuestFolder, apiSharedAccount)
	f.AccountOption(apiSharedAccount, apiGuestID, 0)
	f.AccountAccess(apiSharedAccount, apiOwnerID, 1)

	// Categories (owner): one expense, one income.
	f.Category(fixture.Category{ID: apiCatFood, UserID: apiOwnerID, Name: "Food", Position: 0, Type: 0})
	f.Category(fixture.Category{ID: apiCatSalary, UserID: apiOwnerID, Name: "Salary", Position: 1, Type: 1})

	// Tag + payee (owner).
	f.Tag(fixture.Tag{ID: apiTagWork, UserID: apiOwnerID, Name: "Work"})
	f.Payee(fixture.Payee{ID: apiPayeeShop, UserID: apiOwnerID, Name: "Shop"})

	// Transactions on the owner's account (one expense, one income).
	f.Transaction(fixture.Transaction{ID: apiTxn1, UserID: apiOwnerID, AccountID: apiOwnerAccount, CategoryID: apiCatFood, PayeeID: apiPayeeShop, Type: 1, Amount: "12.50000000", Description: "lunch"})
	f.Transaction(fixture.Transaction{ID: apiTxn2, UserID: apiOwnerID, AccountID: apiOwnerAccount, CategoryID: apiCatSalary, Type: 0, Amount: "1000.00000000", Description: "pay"})

	// A budget owned by the owner.
	f.Budget(fixture.Budget{ID: apiBudget, UserID: apiOwnerID, CurrencyID: apiUSD, Name: "Budget"})
}
