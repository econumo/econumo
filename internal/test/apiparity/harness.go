package apiparity

// API-level engine-parity harness. Unlike the repo-level scenarios in
// enginecompare's scenarios_test.go (which compare a single repository call's
// output), this harness stands up the REAL production HTTP handler
// (internal/server.BuildAPI — the identical router cmd/econumo serves) over a
// given engine's database, seeds an identical fixture, and lets a scenario
// replay a catalogue of HTTP requests.
//
// Why this is the strongest parity contract available: it exercises the entire
// stack — middleware, JWT, the per-engine sqlc query adapters, decimal/datetime
// handling, and the envelope serialization — and compares the actual wire bytes
// a client would receive. Any divergence between two engine adapters that is
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
	"github.com/econumo/econumo/internal/test/testkeys"
	"github.com/econumo/econumo/pkg/jwt"
)

// ignoredDataSalt is set on cfg.DataSalt but the seeded fixture is plaintext
// (WithCrypto("")). The login + parity scenarios still pass, which asserts that
// server.BuildAPI ignores ECONUMO_DATA_SALT and always runs salt-free.
const ignoredDataSalt = "0123456789abcdef" // 16 bytes; deliberately ignored by the API

// fixedClock pins issuance + persistence time so tokens and any created-row
// timestamps are deterministic and identical across engines.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// ClockTime is the fixed instant used for token issuance + any created rows.
// Truncated to the second; near "now" so the JWT exp (iat + 30d) is still valid
// when the verifier checks against the real wall clock during the test run.
var ClockTime = time.Now().UTC().Truncate(time.Second)

// Harness bundles the running production handler over one engine plus the
// collaborators a scenario needs to craft authenticated requests.
type Harness struct {
	srv    *httptest.Server
	engine string
	jwt    *jwt.JWT
	clock  fixedClock
}

// NewHarness builds the full production API over the given (already-migrated)
// engine DB, seeds the shared fixture, and returns a harness with a live server.
func NewHarness(t *testing.T, db *dbtest.DB) *Harness {
	t.Helper()

	priv, pub := testkeys.Paths(t)
	jwtSvc, err := jwt.New(priv, pub, testkeys.Passphrase)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	// Near-now issuance so tokens verify against the real wall clock; truncated to
	// the second to match the integer-timestamp JWT claims. The SAME instant is
	// used on both engines so created-row timestamps match too.
	clk := fixedClock{t: ClockTime}

	cfg := config.Config{
		DatabaseDriver:     db.Engine, // "sqlite" | "postgresql" — selects sqlc adapters
		CurrencyBase:       "USD",
		AllowRegistration:  true,
		DataSalt:           ignoredDataSalt, // set on purpose; the API must ignore it
		CORSAllowedOrigins: []string{"*"},
	}

	Seed(t, db)

	handler := server.BuildAPI(cfg, db.Raw, jwtSvc, clk)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Harness{srv: srv, engine: db.Engine, jwt: jwtSvc, clock: clk}
}

// Engine reports which engine ("sqlite" | "postgresql") this harness runs over.
func (h *Harness) Engine() string { return h.engine }

// Token mints a valid JWT for one of the seeded users via the real signer.
func (h *Harness) Token(t *testing.T, userID, email string) string {
	t.Helper()
	tok, err := h.jwt.Issue(userID, email, h.clock.Now())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// do issues an HTTP request to the harness server and returns the status code
// and the RAW response body bytes (not decoded), which is what the parity
// comparison diffs. token may be "" for public endpoints. rawBody wins over body
// when both are non-nil (multipart-style requests supply rawBody directly).
func (h *Harness) do(t *testing.T, method, path, token string, body any, rawBody []byte, contentType string) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if rawBody != nil {
		rdr = bytes.NewReader(rawBody)
	} else if body != nil {
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
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else if body != nil {
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

// Call issues a single ad-hoc request outside the Call/Replay catalogue
// sequencing — used by scenarios that need per-request control (e.g. asserting
// an intermediate status before issuing the next call).
func (h *Harness) Call(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	return h.do(t, method, path, token, body, nil, "")
}

// Replay issues each call against the harness, returning per-call statuses and
// raw bodies. Owner/guest tokens are minted once per run (engine-independent:
// the JWT signer + the seeded users are identical across engines).
func (h *Harness) Replay(t *testing.T, calls []Call) ([]int, [][]byte) {
	t.Helper()
	ownerTok := h.Token(t, OwnerID, OwnerEmail)
	guestTok := h.Token(t, GuestID, GuestEmail)

	statuses := make([]int, len(calls))
	bodies := make([][]byte, len(calls))
	for i, c := range calls {
		var tok string
		switch c.Auth {
		case "owner":
			tok = ownerTok
		case "guest":
			tok = guestTok
		case "":
			tok = ""
		default:
			t.Fatalf("[%s] unknown auth %q", c.Label, c.Auth)
		}
		statuses[i], bodies[i] = h.do(t, c.Method, c.Path, tok, c.Body, c.RawBody, c.ContentType)
	}
	return statuses, bodies
}
