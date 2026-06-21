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
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// Fixed crypto material shared by both engines so seeded users + minted tokens
// are identical. The data salt is exactly 16 bytes (AES-128). The JWT keypair is
// the repo dev keypair vendored under infra/auth/testdata; the relative path
// resolves from THIS package directory (internal/test/enginecompare).
const (
	apiDataSalt     = "0123456789abcdef"
	apiPassphrase   = "d78eedcb16c13bd949ede5d1b8b910cd"
	apiPrivateKey   = "../../infra/auth/testdata/private.pem"
	apiPublicKey    = "../../infra/auth/testdata/public.pem"
	apiSeedPassword = "secret-pw"
	apiSeedSalt     = "0000000000000000000000000000000000000001" // 40-char sha1-shaped salt
)

// apiFixedClock pins issuance + persistence time so tokens and any created-row
// timestamps are deterministic and identical across engines.
type apiFixedClock struct{ t time.Time }

func (c apiFixedClock) Now() time.Time { return c.t }

// apiHarness bundles the running production handler over one engine plus the
// collaborators a scenario needs to craft authenticated requests.
type apiHarness struct {
	srv    *httptest.Server
	db     *sql.DB
	engine string
	jwt    *auth.JWT
	clock  apiFixedClock
}

// newAPIHarness builds the full production API over the given (already-migrated)
// engine DB, seeds the shared fixture, and returns a harness with a live server.
func newAPIHarness(t *testing.T, db *dbtest.DB) *apiHarness {
	t.Helper()

	jwt, err := auth.NewJWT(apiPrivateKey, apiPublicKey, apiPassphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	// Near-now issuance so tokens verify against the real wall clock; truncated to
	// the second to match the integer-timestamp JWT claims. The SAME instant is
	// used on both engines (passed in by runAPIOnBoth) so created-row timestamps
	// match too.
	clk := apiFixedClock{t: apiClockTime}

	cfg := config.Config{
		AppEnv:            "test",
		DatabaseDriver:    db.Engine, // "sqlite" | "postgresql" — selects sqlc adapters
		CurrencyBase:      "USD",
		AllowRegistration: true,
		ConnectUsers:      false,
		DataSalt:          apiDataSalt,
		CORSAllowOrigin:   "*",
	}

	encode := auth.NewEncodeService(apiDataSalt)
	seedAPIFixture(t, db, encode)

	handler := server.BuildAPI(cfg, db.Raw, jwt, clk)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &apiHarness{srv: srv, db: db.Raw, engine: db.Engine, jwt: jwt, clock: clk}
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

// apiSeedTime is the fixed created/updated timestamp for every seeded row. Bound
// as a "Y-m-d H:i:s" string by the seed helper's time formatting (see seed()).
var apiSeedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

// seedAPIFixture inserts an identical, cross-module fixture into the given engine
// using engine-portable statements (the rebind + TRUE/FALSE helpers from
// harness_test.go). It seeds: two connected users (with hashed password +
// encrypted email so login works), their default options, folders, an owned
// account and a guest-owned account shared to the owner, categories, a tag, a
// payee, two transactions, and a budget — so every read endpoint returns
// non-empty data on both engines.
func seedAPIFixture(t *testing.T, db *dbtest.DB, encode *auth.EncodeService) {
	t.Helper()
	hasher := auth.NewPasswordHasher()

	for _, u := range []struct{ id, email string }{
		{apiOwnerID, apiOwnerEmail},
		{apiGuestID, apiGuestEmail},
	} {
		identifier := encode.Hash(lower(u.email))
		encEmail, err := encode.Encode(u.email)
		if err != nil {
			t.Fatalf("encode email: %v", err)
		}
		pwHash := hasher.Hash(apiSeedPassword, apiSeedSalt)
		seedTS(t, db, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
			VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, TRUE)`,
			u.id, identifier, encEmail, "User "+u.id[:4], pwHash, apiSeedSalt, apiSeedTime, apiSeedTime)

		// Four default options per user (budget value NULL — matches production).
		// All four share an IDENTICAL created_at, exactly as production does (one
		// clock.Now() per registration seeds every option). This deliberately
		// exercises the options query's secondary "ORDER BY ..., id" tiebreak: with
		// equal created_at the order would otherwise be engine-specific
		// (SQLite=insertion, PostgreSQL=unspecified). The ids below are fixed and
		// strictly increasing so both engines agree on the resulting order.
		for _, o := range []struct {
			id, name, value string
			null            bool
		}{
			{u.id[:8] + "-0000-0000-0000-000000000001", "currency", "USD", false},
			{u.id[:8] + "-0000-0000-0000-000000000002", "report_period", "monthly", false},
			{u.id[:8] + "-0000-0000-0000-000000000003", "onboarding", "started", false},
			{u.id[:8] + "-0000-0000-0000-000000000004", "budget", "", true},
		} {
			if o.null {
				seedTS(t, db, `INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?)`,
					o.id, u.id, o.name, apiSeedTime, apiSeedTime)
			} else {
				seedTS(t, db, `INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
					o.id, u.id, o.name, o.value, apiSeedTime, apiSeedTime)
			}
		}
	}

	// Bidirectional connection between owner and guest.
	seed(t, db, `INSERT INTO users_connections (user_id, connected_user_id) VALUES (?, ?)`, apiOwnerID, apiGuestID)
	seed(t, db, `INSERT INTO users_connections (user_id, connected_user_id) VALUES (?, ?)`, apiGuestID, apiOwnerID)

	// Folders.
	seedTS(t, db, `INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at) VALUES (?, ?, 'Main', 0, TRUE, ?, ?)`,
		apiOwnerFolder, apiOwnerID, apiSeedTime, apiSeedTime)
	seedTS(t, db, `INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at) VALUES (?, ?, 'Main', 0, TRUE, ?, ?)`,
		apiGuestFolder, apiGuestID, apiSeedTime, apiSeedTime)

	// Owner's own account.
	seedTS(t, db, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'Cash', 2, 'wallet', FALSE, ?, ?)`,
		apiOwnerAccount, apiUSD, apiOwnerID, apiSeedTime, apiSeedTime)
	seed(t, db, `INSERT INTO accounts_folders (folder_id, account_id) VALUES (?, ?)`, apiOwnerFolder, apiOwnerAccount)
	seedTS(t, db, `INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at) VALUES (?, ?, 0, ?, ?)`,
		apiOwnerAccount, apiOwnerID, apiSeedTime, apiSeedTime)

	// Guest-owned account, SHARED to the owner (accounts_access grant) so the
	// owner's get-account-list / sharedAccess[] / connection list are non-empty.
	seedTS(t, db, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'Shared', 2, 'wallet', FALSE, ?, ?)`,
		apiSharedAccount, apiUSD, apiGuestID, apiSeedTime, apiSeedTime)
	seedTS(t, db, `INSERT INTO accounts_folders (folder_id, account_id) VALUES (?, ?)`, apiGuestFolder, apiSharedAccount)
	seedTS(t, db, `INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at) VALUES (?, ?, 0, ?, ?)`,
		apiSharedAccount, apiGuestID, apiSeedTime, apiSeedTime)
	seedTS(t, db, `INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`,
		apiSharedAccount, apiOwnerID, apiSeedTime, apiSeedTime)

	// Categories (owner): one expense, one income.
	seedTS(t, db, `INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at) VALUES (?, ?, 'Food', 0, 0, 'i', FALSE, ?, ?)`,
		apiCatFood, apiOwnerID, apiSeedTime, apiSeedTime)
	seedTS(t, db, `INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at) VALUES (?, ?, 'Salary', 1, 1, 'i', FALSE, ?, ?)`,
		apiCatSalary, apiOwnerID, apiSeedTime, apiSeedTime)

	// Tag + payee (owner).
	seedTS(t, db, `INSERT INTO tags (id, user_id, name, position, is_archived, created_at, updated_at) VALUES (?, ?, 'Work', 0, FALSE, ?, ?)`,
		apiTagWork, apiOwnerID, apiSeedTime, apiSeedTime)
	seedTS(t, db, `INSERT INTO payees (id, user_id, name, position, is_archived, created_at, updated_at) VALUES (?, ?, 'Shop', 0, FALSE, ?, ?)`,
		apiPayeeShop, apiOwnerID, apiSeedTime, apiSeedTime)

	// Transactions on the owner's account (one expense, one income).
	seedTS(t, db, `INSERT INTO transactions (id, user_id, account_id, category_id, payee_id, type, amount, description, spent_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, 'lunch', ?, ?, ?)`,
		apiTxn1, apiOwnerID, apiOwnerAccount, apiCatFood, apiPayeeShop, "12.50000000", apiSeedTime, apiSeedTime, apiSeedTime)
	seedTS(t, db, `INSERT INTO transactions (id, user_id, account_id, category_id, type, amount, description, spent_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, 'pay', ?, ?, ?)`,
		apiTxn2, apiOwnerID, apiOwnerAccount, apiCatSalary, "1000.00000000", apiSeedTime, apiSeedTime, apiSeedTime)

	// A budget owned by the owner.
	seedTS(t, db, `INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at) VALUES (?, ?, ?, 'Budget', ?, ?, ?)`,
		apiBudget, apiUSD, apiOwnerID, apiSeedTime, apiSeedTime, apiSeedTime)
}

// seedTS is seed() but formats any time.Time arg as the bare "Y-m-d H:i:s" string
// the production code stores (modernc serializes time.Time to RFC3339, which
// SQLite datetime() can't parse — so timestamp columns must be bound as strings).
func seedTS(t *testing.T, db *dbtest.DB, query string, args ...any) {
	t.Helper()
	out := make([]any, len(args))
	for i, a := range args {
		if tm, ok := a.(time.Time); ok {
			out[i] = tm.Format("2006-01-02 15:04:05")
		} else {
			out[i] = a
		}
	}
	if _, err := db.Raw.ExecContext(context.Background(), rebind(db.Engine, query), out...); err != nil {
		t.Fatalf("seedTS (%s) %q: %v", db.Engine, query, err)
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
