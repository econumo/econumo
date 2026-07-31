package apiparity

// API-level engine-parity harness. Unlike the repo-level scenarios in
// enginecompare's scenarios_test.go (which compare a single repository call's
// output), this harness stands up the REAL production HTTP handler
// (internal/server.BuildAPI — the identical router cmd/econumo serves) over a
// given engine's database, seeds an identical fixture, and lets a scenario
// replay a catalogue of HTTP requests.
//
// Why this is the strongest parity contract available: it exercises the entire
// stack — middleware, token authentication, the per-engine sqlc query adapters,
// decimal/datetime handling, and the envelope serialization — and compares the
// actual wire bytes a client would receive. Any divergence between two engine adapters that is
// observable through the API surfaces here.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

// recordingMailer captures sent messages so a scenario can recover the emitted
// reset code (hashed at rest, so no longer readable from the DB).
type recordingMailer struct{ last mailer.Message }

func (m *recordingMailer) Send(_ context.Context, msg mailer.Message) error {
	m.last = msg
	return nil
}

// Anchored on the body's marker text so a digit-bearing user name can never be
// mistaken for the 6-digit code.
var resetCodeRe = regexp.MustCompile(`code is: (\d{6})`)

// ignoredDataSalt is set on cfg.DataSalt but the seeded fixture is plaintext
// (WithCrypto("")). The login + parity scenarios still pass, which asserts that
// server.BuildAPI ignores ECONUMO_DATA_SALT and always runs salt-free.
const ignoredDataSalt = "0123456789abcdef" // 16 bytes; deliberately ignored by the API

// fixedClock pins issuance + persistence time so tokens and any created-row
// timestamps are deterministic and identical across engines.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// ClockTime is the fixed instant used for seeded sessions + any created rows.
// Truncated to the second so datetime columns round-trip identically on both
// engines; the seeded sessions expire at ClockTime + SessionTTL, checked
// against the same fixed clock the server runs on.
var ClockTime = time.Now().UTC().Truncate(time.Second)

// Harness bundles the running production handler over one engine plus the
// collaborators a scenario needs to craft authenticated requests.
type Harness struct {
	srv    *httptest.Server
	engine string
	clock  fixedClock
	db     *dbtest.DB
	mail   *recordingMailer
	minted map[string]string // userID -> raw session token minted by Token()
}

// NewHarness builds the full production API over the given (already-migrated)
// engine DB, seeds the shared fixture, and returns a harness with a live server.
func NewHarness(t *testing.T, db *dbtest.DB) *Harness {
	t.Helper()

	// The SAME instant is used on both engines so created-row timestamps match.
	clk := fixedClock{t: ClockTime}

	cfg := config.Config{
		DatabaseDriver:     db.Engine, // "sqlite" | "postgresql" — selects sqlc adapters
		CurrencyBase:       "USD",
		AllowRegistration:  true,
		DataSalt:           ignoredDataSalt, // set on purpose; the API must ignore it
		CORSAllowedOrigins: []string{"*"},
		// Production-default auth rate limits: existing auth scenarios stay far
		// under them (1 bad login / 1 remind / 1 bad reset per fresh-DB scenario),
		// and the auth_rate_limit scenario deliberately exceeds them to freeze the
		// 429 envelope.
		RateLimitLogin:    5,
		RateLimitReset:    5,
		RateLimitRemind:   3,
		RateLimitRegister: 5,
		RateLimitWindow:   15 * time.Minute,
		RateLimitGlobal:   60,
		// Billing configured so create-billing-link pins its SUCCESS shape. The
		// admin token is the handoff signing key; the assertion it produces is
		// redacted by handoffRe (its exp is clock-derived).
		AdminToken: "0123456789abcdef0123456789abcdef",
		BillingURL: "https://pay.example.test/cloud/",
	}

	Seed(t, db)

	rec := &recordingMailer{}
	handler := server.BuildAPI(cfg, db.Raw, server.Seams{
		Clock:   clk,
		Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar),
		Mailer:  rec,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Harness{srv: srv, engine: db.Engine, clock: clk, db: db, mail: rec, minted: map[string]string{}}
}

// Engine reports which engine ("sqlite" | "postgresql") this harness runs over.
func (h *Harness) Engine() string { return h.engine }

// LastResetCode returns the reset code from the most recently emailed message.
// Reset codes are hashed at rest, so the plaintext is only recoverable here.
func (h *Harness) LastResetCode(t *testing.T) string {
	t.Helper()
	m := resetCodeRe.FindStringSubmatch(h.mail.last.Text)
	if m == nil {
		t.Fatalf("no reset code found in email body: %q", h.mail.last.Text)
	}
	return m[1]
}

// URL returns the running server's base URL, for callers (e.g. mcpparity)
// that need to issue requests outside the REST Call/Replay helpers.
func (h *Harness) URL() string { return h.srv.URL }

// Token returns a live session token for a seeded user: the fixed fixture
// tokens for owner/guest, or (for any other user a test seeds itself) a
// deterministic session minted on first use. Deterministic raw tokens keep
// both engines' rows identical; the minted map keeps re-requests from
// violating the token_hash unique index.
func (h *Harness) Token(t *testing.T, userID, email string) string {
	t.Helper()
	switch userID {
	case OwnerID:
		return OwnerToken
	case GuestID:
		return GuestToken
	case ReadonlyID:
		return ReadonlyToken
	}
	if raw, ok := h.minted[userID]; ok {
		return raw
	}
	// Payload must be exactly 43 chars to match the production token shape:
	// "u" + 36-char uuid + 6 zeros.
	raw := "eco_ses_u" + userID + "000000"
	exp := ClockTime.Add(appuser.SessionTTL)
	fixture.New(t, h.db).AccessToken(fixture.AccessToken{
		UserID: userID, Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(raw), UserAgent: "apiparity", ExpiresAt: &exp,
	})
	h.minted[userID] = raw
	return raw
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
// the seeded users and deterministic tokens are identical across engines).
func (h *Harness) Replay(t *testing.T, calls []Call) ([]int, [][]byte) {
	t.Helper()
	ownerTok := h.Token(t, OwnerID, OwnerEmail)
	guestTok := h.Token(t, GuestID, GuestEmail)
	readonlyTok := h.Token(t, ReadonlyID, ReadonlyEmail)

	statuses := make([]int, len(calls))
	bodies := make([][]byte, len(calls))
	for i, c := range calls {
		var tok string
		switch c.Auth {
		case "owner":
			tok = ownerTok
		case "guest":
			tok = guestTok
		case "readonly":
			tok = readonlyTok
		case "":
			tok = ""
		default:
			t.Fatalf("[%s] unknown auth %q", c.Label, c.Auth)
		}
		statuses[i], bodies[i] = h.do(t, c.Method, c.Path, tok, c.Body, c.RawBody, c.ContentType)
		if c.CaptureIDInto != nil {
			*c.CaptureIDInto = extractItemID(bodies[i])
		}
	}
	return statuses, bodies
}

// extractItemID pulls "data.item.id" out of a create-endpoint response —
// every CREATE result wraps the new entity as {item: {id, ...}}. Returns ""
// (a deliberately invalid id for any later call that dereferences it) if the
// call failed or the shape doesn't match, so a wiring mistake surfaces as a
// downstream "not found" rather than silently reusing a stale id.
func extractItemID(body []byte) string {
	var env struct {
		Data struct {
			Item struct {
				Id string `json:"id"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return env.Data.Item.Id
}
