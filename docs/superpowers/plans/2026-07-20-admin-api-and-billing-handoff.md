# Admin API and Billing Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the private admin listener (`set-access`, `user-context`), the billing-handoff token, and `POST /api/v1/user/create-billing-link`, so the payment portal can read and write access state.

**Architecture:** A new feature package `internal/admin/` holds the two admin use cases and its HTTP edge, served by a **second `http.Server`** that opens only when `ECONUMO_ADMIN_PORT` and `ECONUMO_ADMIN_TOKEN` are both set. It never imports `user` or `connection`; `internal/server` wires adapters over them. The HMAC signer lives in `internal/infra/handoff/` because its only caller is the `user` feature.

**Tech Stack:** Go stdlib only — `crypto/hmac`, `crypto/sha256`, `crypto/subtle`, `encoding/base64`, `net/url`, `net/http.ServeMux`. No new dependencies, no new SQL, no migration.

**Spec:** `docs/superpowers/specs/2026-07-19-admin-api-and-billing-handoff-design.md`

## Global Constraints

- **Comments:** only exceptional scenarios and non-obvious rationale. No godoc restating a signature, no section dividers, no references to the removed PHP implementation. (`CLAUDE.md`)
- **Envelope frozen:** success `{"success":true,"message":"","data":…}`; handled error `{"success":false,"message":…,"code":…,"errors":{}}`. Use `httpx.OK` / `httpx.WriteError`; never hand-roll JSON.
- **Datetimes:** `datetime.Layout` = `"2006-01-02 15:04:05"` — space separator, no zone, UTC, no fractional seconds.
- **`accessUntil` is `""` when NULL**, never `null` and never absent.
- **Features never import features.** `internal/admin` must not import `internal/user` or `internal/connection`; enforced by `internal/test/archtest`.
- **Kernel rule:** `internal/model` may import only `internal/shared`.
- **DTOs live in `internal/model/<feature>_dto.go`** with a `Validate()` method.
- **Errors:** return `errs.NewValidation` / `errs.NewNotFound` from services; the edge maps them.
- **Gates before done:** `make go-test` (includes gofmt, vet, OpenAPI freshness, and the `GO_COVER_MIN=72` floor).

---

### Task 1: Configuration

**Files:**
- Modify: `internal/config/config.go` (struct near line 60, `Load` near line 118)
- Test: `internal/config/config_test.go`
- Modify: `.env.example`

**Interfaces:**
- Consumes: nothing.
- Produces: `config.Config` fields `AdminPort string`, `AdminToken string`, `BillingURL string`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`. Follow the existing tests' style for setting env (they use `t.Setenv`); `PORT` and `DATABASE_URL` are required, so set whatever minimum the neighbouring tests set.

```go
func TestAdminConfigRequiresBothOrNeither(t *testing.T) {
	baseEnv(t) // whatever the existing helper/minimum in this file is
	t.Setenv("ECONUMO_ADMIN_PORT", "9090")
	if _, err := Load(); err == nil {
		t.Fatal("port without token must fail at boot")
	}
}

func TestAdminTokenMinimumLength(t *testing.T) {
	baseEnv(t)
	t.Setenv("ECONUMO_ADMIN_PORT", "9090")
	t.Setenv("ECONUMO_ADMIN_TOKEN", "tooshort")
	if _, err := Load(); err == nil {
		t.Fatal("a token under 32 chars must fail at boot")
	}
}

func TestAdminConfigBothSet(t *testing.T) {
	baseEnv(t)
	t.Setenv("ECONUMO_ADMIN_PORT", "9090")
	t.Setenv("ECONUMO_ADMIN_TOKEN", strings.Repeat("k", 32))
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminPort != "9090" || len(c.AdminToken) != 32 {
		t.Fatalf("AdminPort=%q AdminToken len=%d", c.AdminPort, len(c.AdminToken))
	}
}

func TestBillingURLRequiresAdminToken(t *testing.T) {
	baseEnv(t)
	t.Setenv("ECONUMO_BILLING_URL", "https://pay.example.test/cloud/")
	if _, err := Load(); err == nil {
		t.Fatal("billing URL without an admin token (the HMAC key) must fail at boot")
	}
}

func TestBillingURLMustBeAbsolute(t *testing.T) {
	baseEnv(t)
	t.Setenv("ECONUMO_ADMIN_PORT", "9090")
	t.Setenv("ECONUMO_ADMIN_TOKEN", strings.Repeat("k", 32))
	t.Setenv("ECONUMO_BILLING_URL", "/cloud")
	if _, err := Load(); err == nil {
		t.Fatal("a non-absolute billing URL must fail at boot")
	}
}
```

If no `baseEnv` helper exists, inline the same `t.Setenv` calls the neighbouring tests use and drop the helper.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ -run 'Admin|Billing' -v`
Expected: FAIL — `c.AdminPort` undefined (compile error).

- [ ] **Step 3: Add the config fields**

In the `Config` struct, after the `Trial` field:

```go
	// Admin listener (see the 2026-07-19 admin-API spec). Both empty on a
	// self-hosted instance, so the listener never opens and its routes exist on
	// no mux at all.
	AdminPort  string // ECONUMO_ADMIN_PORT
	AdminToken string // ECONUMO_ADMIN_TOKEN: bearer credential AND handoff HMAC key
	BillingURL string // ECONUMO_BILLING_URL: payment portal; empty disables billing
```

- [ ] **Step 4: Add the validation to `Load`**

After the `c.Trial` block (near line 121):

```go
	c.AdminPort = getEnv("ECONUMO_ADMIN_PORT", "")
	c.AdminToken = getEnv("ECONUMO_ADMIN_TOKEN", "")
	// Half-configured is operator error. Silently not opening the listener is
	// the failure mode that costs an afternoon to diagnose, so fail loudly.
	if (c.AdminPort == "") != (c.AdminToken == "") {
		return Config{}, fmt.Errorf("ECONUMO_ADMIN_PORT and ECONUMO_ADMIN_TOKEN must be set together")
	}
	// The token is an HMAC key as well as a bearer credential.
	if c.AdminToken != "" && len(c.AdminToken) < 32 {
		return Config{}, fmt.Errorf("ECONUMO_ADMIN_TOKEN: must be at least 32 characters")
	}

	if v := os.Getenv("ECONUMO_BILLING_URL"); v != "" {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, fmt.Errorf("ECONUMO_BILLING_URL: not an absolute http(s) URL: %q", v)
		}
		if c.AdminToken == "" {
			return Config{}, fmt.Errorf("ECONUMO_BILLING_URL requires ECONUMO_ADMIN_TOKEN (the handoff signing key)")
		}
		c.BillingURL = v
	}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 6: Document in `.env.example`**

Append:

```
# Admin listener for the payment portal. Both must be set together, or neither.
# Leave empty when self-hosting: the listener never opens.
# ECONUMO_ADMIN_PORT=9090
# ECONUMO_ADMIN_TOKEN=  # >= 32 chars; also the handoff signing key
# Payment portal URL. Empty = no billing endpoint and no billing UI.
# ECONUMO_BILLING_URL=
```

- [ ] **Step 7: Commit**

```bash
git add internal/config/ .env.example
git commit -m "feat(config): admin listener and billing portal variables"
```

---

### Task 2: The handoff token

**Files:**
- Create: `internal/infra/handoff/handoff.go`
- Test: `internal/infra/handoff/handoff_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `handoff.TTL` (`time.Duration`, 10 minutes)
  - `handoff.NewSigner(key string) *Signer`
  - `(*Signer).Sign(uid string, now time.Time) (string, error)`
  - `handoff.Verify(token, key string, now time.Time) (string, error)` — returns the uid
  - `handoff.ErrInvalid`, `handoff.ErrExpired`

`Verify` has no production caller in this repo (the portal verifies). It exists so the scheme is executable and testable on both sides from one definition — without it the tests could only assert that `Sign` is self-consistent, which would not catch a spec-level mistake.

- [ ] **Step 1: Write the failing tests**

```go
package handoff

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

const testKey = "0123456789abcdef0123456789abcdef"

var now = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func TestSignVerifyRoundTrip(t *testing.T) {
	tok, err := NewSigner(testKey).Sign("user-1", now)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := Verify(tok, testKey, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if uid != "user-1" {
		t.Fatalf("uid = %q, want user-1", uid)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	if _, err := Verify(tok, testKey, now.Add(TTL)); !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired at exactly exp", err)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	_, sig, _ := strings.Cut(tok, ".")
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":"user-2","exp":9999999999}`))
	if _, err := Verify(forged+"."+sig, testKey, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestVerifyRejectsForeignKey(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	if _, err := Verify(tok, "ffffffffffffffffffffffffffffffff", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

// Domain separation: the same payload signed WITHOUT the billing-handoff:v1
// prefix must not verify, or a signature minted for another purpose under the
// same key could be replayed as a handoff.
func TestVerifyRejectsUndomainedSignature(t *testing.T) {
	tok, _ := NewSigner(testKey).Sign("user-1", now)
	encPayload, _, _ := strings.Cut(tok, ".")
	m := hmac.New(sha256.New, []byte(testKey))
	m.Write([]byte(encPayload))
	bare := encPayload + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	if _, err := Verify(bare, testKey, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, tok := range []string{"", "nodot", "!!!.!!!", "a.b.c"} {
		if _, err := Verify(tok, testKey, now); err == nil {
			t.Fatalf("token %q verified", tok)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/infra/handoff/ -v`
Expected: FAIL — no non-test Go files / undefined symbols.

- [ ] **Step 3: Implement**

```go
// Package handoff mints and verifies the short-lived identity assertion the SPA
// carries to the payment portal. It lives in infra rather than in a feature
// because its only minter is the user feature, and features may not import each
// other.
package handoff

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// domain separates these signatures from any other HMAC taken under the same
// key, so a handoff signature cannot be replayed as something else.
const domain = "billing-handoff:v1"

const TTL = 10 * time.Minute

var (
	ErrInvalid = errors.New("handoff: invalid token")
	ErrExpired = errors.New("handoff: token expired")
)

type payload struct {
	UID string `json:"uid"`
	Exp int64  `json:"exp"`
}

type Signer struct{ key []byte }

func NewSigner(key string) *Signer { return &Signer{key: []byte(key)} }

func (s *Signer) Sign(uid string, now time.Time) (string, error) {
	body, err := json.Marshal(payload{UID: uid, Exp: now.Add(TTL).Unix()})
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(body)
	return enc + "." + base64.RawURLEncoding.EncodeToString(mac(s.key, enc)), nil
}

func Verify(token, key string, now time.Time) (string, error) {
	enc, encSig, ok := strings.Cut(token, ".")
	if !ok || strings.Contains(encSig, ".") {
		return "", ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return "", ErrInvalid
	}
	if !hmac.Equal(sig, mac([]byte(key), enc)) {
		return "", ErrInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", ErrInvalid
	}
	var p payload
	if err := json.Unmarshal(body, &p); err != nil || p.UID == "" {
		return "", ErrInvalid
	}
	if !now.Before(time.Unix(p.Exp, 0)) {
		return "", ErrExpired
	}
	return p.UID, nil
}

// mac signs the ENCODED payload, not the struct: signing before serialization
// invites a verify-side mismatch when JSON key order or escaping differs
// between implementations (the portal is a separate codebase).
func mac(key []byte, encPayload string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(domain))
	m.Write([]byte(encPayload))
	return m.Sum(nil)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/infra/handoff/ -v`
Expected: PASS, all six tests.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/handoff/
git commit -m "feat(handoff): billing-handoff:v1 signed identity assertion"
```

---

### Task 3: `POST /api/v1/user/create-billing-link`

**Files:**
- Create: `internal/user/billing.go`
- Create: `internal/user/billing_test.go`
- Modify: `internal/model/user_dto.go` (append DTOs)
- Modify: `internal/user/api/handler.go` (add `billing` field + constructor param)
- Create: `internal/user/api/billing.go` (the handler)
- Modify: `internal/user/api/routes.go`
- Modify: `internal/web/middleware/auth.go:45` (allowlist)
- Modify: `internal/server/server.go` (wire the service)
- Test: `internal/web/middleware/middleware_test.go`

**Interfaces:**
- Consumes: `handoff.NewSigner`, `(*Signer).Sign` (Task 2); `cfg.BillingURL`, `cfg.AdminToken` (Task 1).
- Produces:
  - `model.CreateBillingLinkRequest{For string}` with `Validate()`
  - `model.CreateBillingLinkResult{URL string}`
  - `user.NewBillingService(baseURL string, signer BillingLinkSigner, clk port.Clock) *BillingService`
  - `(*BillingService).CreateBillingLink(ctx, userID vo.Id, req model.CreateBillingLinkRequest) (*model.CreateBillingLinkResult, error)`

- [ ] **Step 1: Write the failing service tests**

`internal/user/billing_test.go`:

```go
package user

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type stubSigner struct{}

func (stubSigner) Sign(uid string, _ time.Time) (string, error) { return "tok-" + uid, nil }

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newBillingSvc(t *testing.T, base string) *BillingService {
	t.Helper()
	return NewBillingService(base, stubSigner{}, fixedClock{t: time.Unix(0, 0).UTC()})
}

func TestCreateBillingLinkCarriesTokenAndLang(t *testing.T) {
	svc := newBillingSvc(t, "https://pay.example.test/cloud/")
	ctx := reqctx.WithLanguage(context.Background(), "ru")
	id := vo.NewId()

	res, err := svc.CreateBillingLink(ctx, id, model.CreateBillingLinkRequest{})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(res.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("t"); got != "tok-"+id.String() {
		t.Fatalf("t = %q", got)
	}
	if got := u.Query().Get("lang"); got != "ru" {
		t.Fatalf("lang = %q, want ru", got)
	}
	if u.Query().Has("for") {
		t.Fatal("for must be absent when not requested")
	}
}

func TestCreateBillingLinkDefaultsLangToEnglish(t *testing.T) {
	svc := newBillingSvc(t, "https://pay.example.test/cloud/")
	res, err := svc.CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	if got := u.Query().Get("lang"); got != "en" {
		t.Fatalf("lang = %q, want en", got)
	}
}

func TestCreateBillingLinkPreservesExistingQuery(t *testing.T) {
	svc := newBillingSvc(t, "https://pay.example.test/cloud/?src=app")
	res, err := svc.CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	if u.Query().Get("src") != "app" {
		t.Fatalf("existing query lost: %s", res.URL)
	}
}

func TestCreateBillingLinkPassesFor(t *testing.T) {
	svc := newBillingSvc(t, "https://pay.example.test/cloud/")
	partner := vo.NewId()
	res, err := svc.CreateBillingLink(context.Background(), vo.NewId(),
		model.CreateBillingLinkRequest{For: partner.String()})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	if got := u.Query().Get("for"); got != partner.String() {
		t.Fatalf("for = %q", got)
	}
}

// for is concatenated into a URL, so a malformed value is rejected before it
// can inject parameters into the portal link.
func TestCreateBillingLinkRejectsMalformedFor(t *testing.T) {
	svc := newBillingSvc(t, "https://pay.example.test/cloud/")
	_, err := svc.CreateBillingLink(context.Background(), vo.NewId(),
		model.CreateBillingLinkRequest{For: "not-a-uuid&evil=1"})
	if err == nil {
		t.Fatal("malformed for accepted")
	}
}

func TestCreateBillingLinkDisabledWithoutURL(t *testing.T) {
	svc := newBillingSvc(t, "")
	if _, err := svc.CreateBillingLink(context.Background(), vo.NewId(), model.CreateBillingLinkRequest{}); err == nil {
		t.Fatal("want an error when ECONUMO_BILLING_URL is unset")
	}
}
```

If `vo.NewId` does not exist under that name, use whatever the codebase's id constructor is (check `internal/shared/vo`); the tests only need a valid id.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/user/ -run Billing -v`
Expected: FAIL — `NewBillingService` undefined.

- [ ] **Step 3: Add the DTOs**

Append to `internal/model/user_dto.go`:

```go
// CreateBillingLinkRequest optionally preselects a beneficiary. For is a
// preselection hint only — the portal authorizes it against the connection
// list it fetches server-side.
type CreateBillingLinkRequest struct {
	For string `json:"for"`
}

func (r CreateBillingLinkRequest) Validate() error { return nil }

type CreateBillingLinkResult struct {
	URL string `json:"url"`
}
```

`Validate()` is a no-op because `For` needs id parsing, which the service does — duplicating it here would report the same failure twice with different wording.

- [ ] **Step 4: Implement the service**

`internal/user/billing.go`:

```go
package user

import (
	"context"
	"net/url"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type BillingLinkSigner interface {
	Sign(uid string, now time.Time) (string, error)
}

type BillingService struct {
	baseURL string
	signer  BillingLinkSigner
	clock   port.Clock
}

func NewBillingService(baseURL string, signer BillingLinkSigner, clk port.Clock) *BillingService {
	return &BillingService{baseURL: baseURL, signer: signer, clock: clk}
}

// CreateBillingLink mints a fresh assertion per click rather than carrying one
// on the user payload: the token lives 10 minutes and get-user-data is cached
// by the SPA, so a link built at login would be stale by the time it is used.
func (s *BillingService) CreateBillingLink(ctx context.Context, userID vo.Id, req model.CreateBillingLinkRequest) (*model.CreateBillingLinkResult, error) {
	if s.baseURL == "" {
		return nil, errs.NewValidation("Billing is not configured")
	}
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, err
	}

	token, err := s.signer.Sign(userID.String(), s.clock.Now())
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("t", token)
	if req.For != "" {
		forID, perr := vo.ParseId(req.For)
		if perr != nil {
			return nil, errs.NewValidation("Invalid user id",
				errs.FieldError{Key: "for", Message: "Invalid user id"})
		}
		q.Set("for", forID.String())
	}
	// The SPA sends Accept-Language on every request, so this is the language
	// the user is reading right now — fresher than the users.language column,
	// which is written only at login.
	q.Set("lang", reqctx.Language(ctx))
	u.RawQuery = q.Encode()

	return &model.CreateBillingLinkResult{URL: u.String()}, nil
}
```

Check `errs.NewValidation`'s signature (`internal/shared/errs/errs.go:49`) and `errs.FieldError`'s fields before writing — match them exactly. If `port.Clock`'s method is not `Now()`, adapt.

- [ ] **Step 5: Run the service tests**

Run: `go test ./internal/user/ -run Billing -v`
Expected: PASS, all seven tests.

- [ ] **Step 6: Add the handler**

`internal/user/api/billing.go`:

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// CreateBillingLink godoc
//
//	@Summary	Create a payment portal link
//	@Tags		user
//	@Accept		json
//	@Produce	json
//	@Param		request	body		model.CreateBillingLinkRequest	true	"optional beneficiary"
//	@Success	200		{object}	model.CreateBillingLinkResult
//	@Router		/api/v1/user/create-billing-link [post]
func (h *Handlers) CreateBillingLink(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.billing.CreateBillingLink)
}
```

Match the surrounding handlers' swag annotation style exactly — copy the shape from a neighbouring POST handler in `internal/user/api/`, including its security/`@Param` conventions. The OpenAPI-freshness check in `make go-lint` will fail on a malformed block.

- [ ] **Step 7: Wire the handler struct and route**

In `internal/user/api/handler.go`, add a `billing *appuser.BillingService` field to `Handlers` and a parameter to `NewHandlers` (match the existing import alias for the user package).

In `internal/user/api/routes.go`, alongside the other authenticated user POSTs:

```go
		mux.Handle("POST /api/v1/user/create-billing-link", auth(h.CreateBillingLink))
```

- [ ] **Step 8: Add to the read-only allowlist**

In `internal/web/middleware/auth.go`, add the entry and extend the comment. The full block becomes:

```go
// ReadonlyAllowedPaths are the POST endpoints a restricted caller may still
// reach. The principle: a restricted user may always secure their account,
// leave it, or pay to restore it, but may not add data. update-password is a
// security operation, so locking someone out of rotating a compromised password
// would be indefensible; create-billing-link is how a lapsed user reaches the
// payment portal, so blocking it would 402 exactly the person trying to fix
// their access; create-personal-token is excluded because it mints new
// write-capable credentials. Account deletion joins this list when it exists.
//
// Exported so a guard test (internal/test/apiparity) can assert every path
// here is still a real registered route, catching a route rename that would
// otherwise leave a lapsed user unable to log out or rotate a password.
var ReadonlyAllowedPaths = map[string]bool{
	"/api/v1/user/logout-user":           true,
	"/api/v1/user/revoke-session":        true,
	"/api/v1/user/revoke-other-sessions": true,
	"/api/v1/user/revoke-personal-token": true,
	"/api/v1/user/update-password":       true,
	"/api/v1/user/create-billing-link":   true,
}
```

- [ ] **Step 9: Add the middleware regression test**

In `internal/web/middleware/middleware_test.go`, next to the existing read-only tests (around line 516):

```go
// A read-only user is exactly the person who needs the payment link; 402 here
// would be a dead end with no way out.
func TestReadonlyReachesBillingLink(t *testing.T) {
	rec, ran := authRequest(t, http.MethodPost, "/api/v1/user/create-billing-link", readonlyStub())
	if !ran || rec.Code == http.StatusPaymentRequired {
		t.Fatalf("billing link blocked for a read-only user: status %d ran %v", rec.Code, ran)
	}
}
```

- [ ] **Step 10: Wire in the composition root**

In `internal/server/server.go`, after `userReadSvc` is built:

```go
	billingSvc := appuser.NewBillingService(cfg.BillingURL, handoff.NewSigner(cfg.AdminToken), clk)
```

Pass `billingSvc` into `handleruser.NewHandlers(...)` and add the `handoff` import.

- [ ] **Step 11: Run the suite and regenerate goldens**

```bash
go test ./internal/user/... ./internal/web/... ./internal/server/... -v
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
git diff --stat internal/test/apiparity/testdata/golden/
```

Expected: one new golden for `create-billing-link` and **no other golden changed**. If an existing golden moved, stop and investigate — this change is additive and must not alter any existing response.

The apiparity guard requires a scenario per route. Add one to the catalogue following the neighbouring user scenarios; the run above will fail with a clear message naming the missing scenario if you skip it.

- [ ] **Step 12: Raise the route floor**

In `internal/test/apiparity/guard_test.go:51`, `const minRoutes = 87` → `88`.

- [ ] **Step 13: Full gate**

Run: `make go-test`
Expected: PASS.

- [ ] **Step 14: Commit**

```bash
git add -A
git commit -m "feat(user): create-billing-link mints a signed portal handoff"
```

---

### Task 4: The admin feature — ports, DTOs, use cases

**Files:**
- Create: `internal/admin/ports.go`
- Create: `internal/admin/service.go`
- Create: `internal/admin/access.go`
- Create: `internal/admin/context.go`
- Create: `internal/model/admin_dto.go`
- Test: `internal/admin/admin_test.go`

**Interfaces:**
- Consumes: `model.AccessLevel`, `model.ParseAccessLevel`, `model.EffectiveAccessLevel`, `datetime.Layout`, `port.Clock`.
- Produces:
  - `admin.UserRecord{ID, Name, Email string; AccessLevel model.AccessLevel; AccessUntil *time.Time}`
  - `admin.UserLookup` / `admin.ConnectionLookup` (interfaces below)
  - `admin.NewService(users UserLookup, conns ConnectionLookup, clk port.Clock) *Service`
  - `(*Service).SetAccess(ctx, req model.AdminSetAccessRequest) (*model.AdminUserView, error)`
  - `(*Service).UserContext(ctx, userID vo.Id) (*model.AdminUserContextResult, error)`
  - `model.AdminSetAccessRequest`, `model.AdminUserView`, `model.AdminUserContextResult`

- [ ] **Step 1: Write the DTOs**

`internal/model/admin_dto.go`:

```go
// This file holds the admin listener's request/result DTOs. The listener is
// private and single-consumer (the payment portal), so these are not part of
// the frozen public wire contract.
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
)

// AdminSetAccessRequest addresses a user by id only. Until is a pointer so an
// explicit JSON null (meaning "no expiry") is distinguishable from an absent
// field; both clear the column.
type AdminSetAccessRequest struct {
	UserId string  `json:"userId"`
	Level  string  `json:"level"`
	Until  *string `json:"until"`
}

func (r AdminSetAccessRequest) Validate() error {
	var fields []errs.FieldError
	if r.UserId == "" {
		fields = append(fields, errs.FieldError{Key: "userId", Message: "This value should not be blank."})
	}
	if _, err := ParseAccessLevel(r.Level); err != nil {
		fields = append(fields, errs.FieldError{Key: "level", Message: "Level must be full or readonly"})
	}
	if r.Until != nil && *r.Until != "" {
		if _, err := time.Parse(datetime.Layout, *r.Until); err != nil {
			fields = append(fields, errs.FieldError{Key: "until", Message: "Until must be formatted as " + datetime.Layout})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// AdminUserView carries both the stored access columns and the effective level.
// The portal needs the raw level to tell a LAPSED user (offer a purchase) from a
// MANUALLY RESTRICTED one (do not), and the effective level to answer "can this
// person write right now" without re-implementing the collapse rule.
type AdminUserView struct {
	Id                   string `json:"id"`
	Name                 string `json:"name"`
	Email                string `json:"email"`
	AccessLevel          string `json:"accessLevel"`
	AccessUntil          string `json:"accessUntil"`
	EffectiveAccessLevel string `json:"effectiveAccessLevel"`
}

type AdminUserContextResult struct {
	User        AdminUserView   `json:"user"`
	Connections []AdminUserView `json:"connections"`
}
```

Confirm `errs.FieldError`'s field names against `internal/shared/errs/errs.go` before writing; add `Code:` values only if the neighbouring DTOs set them.

- [ ] **Step 2: Write the ports**

`internal/admin/ports.go`:

```go
// Package admin serves the private listener the payment portal talks to. It
// never imports another feature: the user and connection capabilities it needs
// are declared here and wired by internal/server.
package admin

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserRecord is the neutral shape the admin package works in — raw columns, not
// collapsed, so this package owns the one place the effective level is derived.
type UserRecord struct {
	ID          string
	Name        string
	Email       string
	AccessLevel model.AccessLevel
	AccessUntil *time.Time
}

type UserLookup interface {
	GetUser(ctx context.Context, id vo.Id) (UserRecord, error)
	SetAccess(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) error
}

type ConnectionLookup interface {
	ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}
```

- [ ] **Step 3: Write the failing tests**

`internal/admin/admin_test.go`:

```go
package admin

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

var now = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

type clk struct{}

func (clk) Now() time.Time { return now }

type stubUsers struct {
	byID  map[string]UserRecord
	saved map[string]model.AccessLevel
	until map[string]*time.Time
}

func (s *stubUsers) GetUser(_ context.Context, id vo.Id) (UserRecord, error) {
	u, ok := s.byID[id.String()]
	if !ok {
		return UserRecord{}, errs.NewNotFound("User not found")
	}
	return u, nil
}

func (s *stubUsers) SetAccess(_ context.Context, id vo.Id, level model.AccessLevel, until *time.Time) error {
	if _, ok := s.byID[id.String()]; !ok {
		return errs.NewNotFound("User not found")
	}
	s.saved[id.String()] = level
	s.until[id.String()] = until
	rec := s.byID[id.String()]
	rec.AccessLevel, rec.AccessUntil = level, until
	s.byID[id.String()] = rec
	return nil
}

type stubConns struct{ ids map[string][]vo.Id }

func (s *stubConns) ConnectedUserIDs(_ context.Context, id vo.Id) ([]vo.Id, error) {
	return s.ids[id.String()], nil
}

func newFixture(t *testing.T) (*Service, *stubUsers, *stubConns, vo.Id, vo.Id) {
	t.Helper()
	self, partner := vo.NewId(), vo.NewId()
	users := &stubUsers{
		byID: map[string]UserRecord{
			self.String():    {ID: self.String(), Name: "Alex", Email: "alex@example.test", AccessLevel: model.AccessLevelFull},
			partner.String(): {ID: partner.String(), Name: "Sam", Email: "sam@example.test", AccessLevel: model.AccessLevelFull},
		},
		saved: map[string]model.AccessLevel{},
		until: map[string]*time.Time{},
	}
	conns := &stubConns{ids: map[string][]vo.Id{self.String(): {partner}}}
	return NewService(users, conns, clk{}), users, conns, self, partner
}

func TestSetAccessWritesLevelAndExpiry(t *testing.T) {
	svc, users, _, self, _ := newFixture(t)
	until := "2027-01-01 00:00:00"
	view, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: self.String(), Level: "full", Until: &until,
	})
	if err != nil {
		t.Fatal(err)
	}
	if users.saved[self.String()] != model.AccessLevelFull {
		t.Fatalf("level = %q", users.saved[self.String()])
	}
	if view.AccessUntil != until {
		t.Fatalf("accessUntil = %q, want %q", view.AccessUntil, until)
	}
}

func TestSetAccessNilUntilClearsExpiry(t *testing.T) {
	svc, users, _, self, _ := newFixture(t)
	view, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: self.String(), Level: "readonly", Until: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if users.until[self.String()] != nil {
		t.Fatal("until must be NULL")
	}
	if view.AccessUntil != "" {
		t.Fatalf("accessUntil = %q, want empty string for NULL", view.AccessUntil)
	}
}

// Stripe retries webhooks; applying the same call twice must be indistinguishable
// from applying it once.
func TestSetAccessIsIdempotent(t *testing.T) {
	svc, _, _, self, _ := newFixture(t)
	until := "2027-01-01 00:00:00"
	req := model.AdminSetAccessRequest{UserId: self.String(), Level: "full", Until: &until}
	first, err := svc.SetAccess(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.SetAccess(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if *first != *second {
		t.Fatalf("not idempotent: %+v vs %+v", *first, *second)
	}
}

func TestSetAccessUnknownUser(t *testing.T) {
	svc, _, _, _, _ := newFixture(t)
	_, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: vo.NewId().String(), Level: "full",
	})
	var nf *errs.NotFoundError
	if !errorsAs(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestUserContextReturnsSelfAndConnections(t *testing.T) {
	svc, _, _, self, partner := newFixture(t)
	res, err := svc.UserContext(context.Background(), self)
	if err != nil {
		t.Fatal(err)
	}
	if res.User.Id != self.String() || res.User.Email != "alex@example.test" {
		t.Fatalf("user = %+v", res.User)
	}
	if len(res.Connections) != 1 || res.Connections[0].Id != partner.String() {
		t.Fatalf("connections = %+v", res.Connections)
	}
}

// The whole model turns on (level, until, now) collapsing to one value; the
// portal must not have to re-derive it.
func TestUserContextEffectiveDivergesAfterExpiry(t *testing.T) {
	svc, users, _, self, _ := newFixture(t)
	past := now.Add(-time.Hour)
	rec := users.byID[self.String()]
	rec.AccessUntil = &past
	users.byID[self.String()] = rec

	res, err := svc.UserContext(context.Background(), self)
	if err != nil {
		t.Fatal(err)
	}
	if res.User.AccessLevel != "full" {
		t.Fatalf("raw accessLevel = %q, want full", res.User.AccessLevel)
	}
	if res.User.EffectiveAccessLevel != "readonly" {
		t.Fatalf("effective = %q, want readonly", res.User.EffectiveAccessLevel)
	}
}

func TestUserContextUnknownUser(t *testing.T) {
	svc, _, _, _, _ := newFixture(t)
	if _, err := svc.UserContext(context.Background(), vo.NewId()); err == nil {
		t.Fatal("want NotFound for an unknown user")
	}
}
```

Replace `errorsAs` with a direct `errors.As` call and add the `errors` import — it is written that way here only to keep the snippet self-contained.

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/admin/ -v`
Expected: FAIL — `NewService` undefined.

- [ ] **Step 5: Implement the service**

`internal/admin/service.go`:

```go
package admin

import (
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/port"
)

type Service struct {
	users UserLookup
	conns ConnectionLookup
	clock port.Clock
}

func NewService(users UserLookup, conns ConnectionLookup, clk port.Clock) *Service {
	return &Service{users: users, conns: conns, clock: clk}
}

func (s *Service) view(r UserRecord) model.AdminUserView {
	until := ""
	if r.AccessUntil != nil {
		until = r.AccessUntil.UTC().Format(datetime.Layout)
	}
	return model.AdminUserView{
		Id:                   r.ID,
		Name:                 r.Name,
		Email:                r.Email,
		AccessLevel:          string(r.AccessLevel),
		AccessUntil:          until,
		EffectiveAccessLevel: string(model.EffectiveAccessLevel(r.AccessLevel, r.AccessUntil, s.clock.Now())),
	}
}
```

`internal/admin/access.go`:

```go
package admin

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SetAccess is naturally idempotent — it assigns rather than accumulates — so
// the portal's retrying webhook needs no operation guard.
func (s *Service) SetAccess(ctx context.Context, req model.AdminSetAccessRequest) (*model.AdminUserView, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	id, err := vo.ParseId(req.UserId)
	if err != nil {
		return nil, err
	}
	level, err := model.ParseAccessLevel(req.Level)
	if err != nil {
		return nil, err
	}
	var until *time.Time
	if req.Until != nil && *req.Until != "" {
		t, perr := time.ParseInLocation(datetime.Layout, *req.Until, time.UTC)
		if perr != nil {
			return nil, perr
		}
		until = &t
	}
	if err := s.users.SetAccess(ctx, id, level, until); err != nil {
		return nil, err
	}
	rec, err := s.users.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	v := s.view(rec)
	return &v, nil
}
```

`internal/admin/context.go`:

```go
package admin

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UserContext keeps the connection graph — and the rules behind it — in the
// product, so the portal never duplicates "who may see whom".
func (s *Service) UserContext(ctx context.Context, userID vo.Id) (*model.AdminUserContextResult, error) {
	self, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids, err := s.conns.ConnectedUserIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	// One lookup per connection: connections are partners (typically 0-3), so a
	// dedicated cross-engine query would buy little over indexed primary-key reads.
	conns := make([]model.AdminUserView, 0, len(ids))
	for _, id := range ids {
		rec, cerr := s.users.GetUser(ctx, id)
		if cerr != nil {
			return nil, cerr
		}
		conns = append(conns, s.view(rec))
	}
	return &model.AdminUserContextResult{User: s.view(self), Connections: conns}, nil
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/admin/ -v`
Expected: PASS, all seven tests.

- [ ] **Step 7: Verify the architecture rule**

Run: `go test ./internal/test/archtest/ -v`
Expected: PASS — `internal/admin` imports no other feature.

- [ ] **Step 8: Commit**

```bash
git add internal/admin/ internal/model/admin_dto.go
git commit -m "feat(admin): set-access and user-context use cases"
```

---

### Task 5: Admin HTTP edge and bearer middleware

**Files:**
- Create: `internal/web/middleware/adminauth.go`
- Test: `internal/web/middleware/adminauth_test.go`
- Create: `internal/admin/api/handler.go`
- Create: `internal/admin/api/routes.go`
- Test: `internal/admin/api/handler_test.go`

**Interfaces:**
- Consumes: `admin.Service` (Task 4), `httpx`, `errs`.
- Produces:
  - `middleware.AdminAuth(token string) Middleware`
  - `api.NewHandlers(svc *admin.Service, dev bool) *Handlers`
  - `api.RegisterAdmin(h *Handlers) func(mux *http.ServeMux)`

- [ ] **Step 1: Write the failing middleware test**

`internal/web/middleware/adminauth_test.go`:

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func adminRequest(t *testing.T, header string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	ran := false
	h := AdminAuth(strings.Repeat("k", 32))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin/user-context", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, ran
}

func TestAdminAuthAcceptsValidToken(t *testing.T) {
	rec, ran := adminRequest(t, "Bearer "+strings.Repeat("k", 32))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("status %d ran %v", rec.Code, ran)
	}
}

func TestAdminAuthRejects(t *testing.T) {
	for name, header := range map[string]string{
		"missing":      "",
		"wrong token":  "Bearer " + strings.Repeat("x", 32),
		"wrong scheme": "Basic " + strings.Repeat("k", 32),
		"empty bearer": "Bearer ",
	} {
		t.Run(name, func(t *testing.T) {
			rec, ran := adminRequest(t, header)
			if ran || rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d ran %v", rec.Code, ran)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/web/middleware/ -run AdminAuth -v`
Expected: FAIL — `AdminAuth` undefined.

- [ ] **Step 3: Implement the middleware**

`internal/web/middleware/adminauth.go`:

```go
package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/web/httpx"
)

// AdminAuth guards the private admin listener with a single shared bearer
// token. Unlike the public Auth middleware there is no user, no session, and no
// access-level gate: the caller is a service, not a person.
func AdminAuth(token string) Middleware {
	want := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok := bearerToken(r)
			if !ok || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
				httpx.WriteError(w, errs.NewUnauthorized("Invalid access token"), false)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

`subtle.ConstantTimeCompare` returns 0 for unequal lengths, so a short guess leaks nothing beyond length — acceptable for a 32-byte minimum token. `dev` is hard-coded `false`: this surface never returns stack traces.

- [ ] **Step 4: Run the middleware tests**

Run: `go test ./internal/web/middleware/ -run AdminAuth -v`
Expected: PASS.

- [ ] **Step 5: Write the failing handler tests**

`internal/admin/api/handler_test.go` — build a `Handlers` over an `admin.Service` backed by the same stubs from Task 4 (export a small test helper from `internal/admin` or redeclare the stubs here), then assert:

```go
func TestSetAccessHandlerRejectsBadLevel(t *testing.T) {
	rec := post(t, "/admin/set-access", `{"userId":"`+selfID+`","level":"premium"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserContextHandlerRequiresUserId(t *testing.T) {
	rec := get(t, "/admin/user-context")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUserContextHandlerUnknownUser(t *testing.T) {
	rec := get(t, "/admin/user-context?userId="+vo.NewId().String())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUserContextHandlerReturnsEnvelope(t *testing.T) {
	rec := get(t, "/admin/user-context?userId="+selfID)
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			User        map[string]string   `json:"user"`
			Connections []map[string]string `json:"connections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Success || env.Data.User["email"] == "" {
		t.Fatalf("envelope = %s", rec.Body.String())
	}
}
```

Write `post`/`get` helpers that build the mux via `RegisterAdmin` and serve an `httptest` request.

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/admin/api/ -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 7: Implement the handlers**

`internal/admin/api/handler.go`:

```go
// Package api is the admin listener's HTTP edge. These routes are registered on
// a separate mux served by a separate http.Server; they are never reachable on
// the public API mux (asserted by internal/test/apiparity).
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/admin"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/httpx"
)

type Handlers struct {
	svc *admin.Service
	dev bool
}

func NewHandlers(svc *admin.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}

func (h *Handlers) SetAccess(w http.ResponseWriter, r *http.Request) {
	var req model.AdminSetAccessRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	res, err := h.svc.SetAccess(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}

func (h *Handlers) UserContext(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("userId")
	if raw == "" {
		httpx.WriteError(w, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "userId", Message: "This value should not be blank."}), h.dev)
		return
	}
	id, err := vo.ParseId(raw)
	if err != nil {
		httpx.WriteError(w, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "userId", Message: "Invalid user id"}), h.dev)
		return
	}
	res, err := h.svc.UserContext(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err, h.dev)
		return
	}
	httpx.OK(w, res)
}
```

`httpx.Decode` (not `DecodeValidate`) because `SetAccess` calls `Validate()` itself — validating twice would be the same check in two places.

- [ ] **Step 8: Implement the routes**

`internal/admin/api/routes.go`:

```go
package api

import "net/http"

// RegisterAdmin registers the private admin routes. Deliberately NOT a
// router.RegisterAPI: that type feeds the public mux, and these routes must
// never be mounted there.
func RegisterAdmin(h *Handlers) func(mux *http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("POST /admin/set-access", h.SetAccess)
		mux.HandleFunc("GET /admin/user-context", h.UserContext)
	}
}
```

- [ ] **Step 9: Run tests**

Run: `go test ./internal/admin/... ./internal/web/middleware/ -v`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/admin/api/ internal/web/middleware/adminauth.go internal/web/middleware/adminauth_test.go
git commit -m "feat(admin): HTTP edge and bearer-token middleware"
```

---

### Task 6: Compose the admin server

**Files:**
- Create: `internal/server/glue_admin.go`
- Create: `internal/server/admin.go`
- Modify: `internal/user/admin.go` (add two by-id methods)
- Test: `internal/server/glue_admin_test.go`

**Interfaces:**
- Consumes: everything from Tasks 4–5.
- Produces:
  - `server.BuildAdmin(cfg config.Config, db *sql.DB, seams Seams) http.Handler`
  - `appuser.(*Service).AdminUserByID(ctx, id vo.Id) (*model.User, string, error)` — user plus decrypted email
  - `appuser.(*Service).AdminSetAccessByID(ctx, id vo.Id, level model.AccessLevel, until *time.Time) error`

- [ ] **Step 1: Add the by-id user service methods**

In `internal/user/admin.go`, alongside `AdminSetAccess`:

```go
// AdminUserByID loads a user by id with the email decrypted, for the admin
// listener (which addresses users by id, never by email).
func (s *Service) AdminUserByID(ctx context.Context, id vo.Id) (*model.User, string, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	email, err := s.encode.Decode(u.Email)
	if err != nil {
		return nil, "", err
	}
	return u, email, nil
}

// AdminSetAccessByID is AdminSetAccess keyed by id: the CLI has an operator's
// email address, the payment portal has a user id.
func (s *Service) AdminSetAccessByID(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		u.SetAccess(level, until, s.clock.Now())
		return s.repo.Save(ctx, u)
	})
}
```

Check `s.encode.Decode`'s exact signature in `internal/infra/auth` first (the CLI's `user:show` already decodes an email — copy that call).

- [ ] **Step 2: Write the glue**

`internal/server/glue_admin.go`:

```go
// Adapters bridging the admin feature to the user and connection features. They
// live here because features must not import each other; only the composition
// root may join them.
package server

import (
	"context"
	"time"

	appadmin "github.com/econumo/econumo/internal/admin"
	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

type AdminUserAccess struct{ users *appuser.Service }

var _ appadmin.UserLookup = (*AdminUserAccess)(nil)

func NewAdminUserAccess(users *appuser.Service) *AdminUserAccess {
	return &AdminUserAccess{users: users}
}

func (a *AdminUserAccess) GetUser(ctx context.Context, id vo.Id) (appadmin.UserRecord, error) {
	u, email, err := a.users.AdminUserByID(ctx, id)
	if err != nil {
		return appadmin.UserRecord{}, err
	}
	return appadmin.UserRecord{
		ID:          u.ID.String(),
		Name:        u.Name,
		Email:       email,
		AccessLevel: u.AccessLevel,
		AccessUntil: u.AccessUntil,
	}, nil
}

func (a *AdminUserAccess) SetAccess(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) error {
	return a.users.AdminSetAccessByID(ctx, id, level, until)
}

type AdminConnections struct{ conns *appconnection.Service }

var _ appadmin.ConnectionLookup = (*AdminConnections)(nil)

func NewAdminConnections(conns *appconnection.Service) *AdminConnections {
	return &AdminConnections{conns: conns}
}

func (a *AdminConnections) ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	list, err := a.conns.GetConnectionList(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]vo.Id, 0, len(list.Items))
	for _, item := range list.Items {
		id, perr := vo.ParseId(item.User.Id)
		if perr != nil {
			return nil, perr
		}
		ids = append(ids, id)
	}
	return ids, nil
}
```

- [ ] **Step 3: Write `BuildAdmin`**

`internal/server/admin.go`. Mirror the relevant wiring from `BuildAPI` — read `internal/server/server.go:77-140` and reuse the same constructors for `txm`, `encodeSvc`, `hasher`, `userRepo`, `accessTokens`, `currencyLookup`, `budgetAccess`, and the connection service. `BuildAdmin` needs a full `appuser.Service` (for the two by-id methods) and an `appconnection.Service`.

```go
package server

import (
	"database/sql"
	"net/http"

	appadmin "github.com/econumo/econumo/internal/admin"
	handleradmin "github.com/econumo/econumo/internal/admin/api"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/web/middleware"
)

// BuildAdmin returns the private admin handler. The caller (serve) starts it on
// its own http.Server and only when both admin variables are configured, so
// these routes exist on no mux at all for a self-hosted instance.
func BuildAdmin(cfg config.Config, db *sql.DB, seams Seams) http.Handler {
	// ... build clk, txm, encodeSvc, hasher, repos, userSvc, connSvc exactly as
	// BuildAPI does (see internal/server/server.go) ...

	adminSvc := appadmin.NewService(
		NewAdminUserAccess(userSvc),
		NewAdminConnections(connSvc),
		clk,
	)
	handlers := handleradmin.NewHandlers(adminSvc, cfg.IsDev())

	mux := http.NewServeMux()
	handleradmin.RegisterAdmin(handlers)(mux)

	// No CORS (never browser-reached) and no timezone/language (nothing here is
	// user-facing; datetimes are frozen UTC).
	chain := middleware.Chain(
		middleware.RequestID,
		middleware.AccessLog,
		middleware.Recover(cfg.IsDev()),
		middleware.AdminAuth(cfg.AdminToken),
	)
	return chain(mux)
}
```

If the shared wiring from `BuildAPI` is long enough to duplicate awkwardly, extract the common prefix into an unexported helper in `server.go` returning the built services, and call it from both. Do not copy-paste two divergent copies.

- [ ] **Step 4: Write the integration test**

`internal/server/glue_admin_test.go` — using `dbtest` and `fixture` as the other server tests do:

```go
func TestAdminSetAccessRoundTrip(t *testing.T) {
	// build a user via fixture, then:
	// POST /admin/set-access {"userId":…,"level":"readonly","until":null}
	// with a valid bearer, assert 200 and that a follow-up
	// GET /admin/user-context?userId=… reports accessLevel "readonly".
}

func TestAdminUserContextIncludesConnections(t *testing.T) {
	// build two connected users via fixture, assert the caller plus exactly
	// one connection, each with a non-empty email.
}

func TestAdminRejectsWrongBearer(t *testing.T) {
	// assert 401 and that no state changed.
}
```

Fill these in against the existing `internal/server/glue_*_test.go` helpers — copy their `dbtest.New(t)` + `fixture` setup verbatim rather than inventing new scaffolding.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/... ./internal/admin/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/ internal/user/admin.go
git commit -m "feat(server): compose the admin listener"
```

---

### Task 7: Start the second listener

**Files:**
- Modify: `cmd/econumo/main.go:205-230`

**Interfaces:**
- Consumes: `server.BuildAdmin` (Task 6), `cfg.AdminPort` / `cfg.AdminToken` (Task 1).
- Produces: nothing importable.

- [ ] **Step 1: Replace the single blocking listen**

`cmd/econumo/main.go` currently ends `serve` with a bare `srv.ListenAndServe()`. Replace that tail with a two-server run that shuts both down on signal and fails the process if either dies:

```go
	srv := &http.Server{
		Addr:              addr(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	servers := []*http.Server{srv}
	// The admin listener opens only when both variables are configured, so a
	// self-hosted instance never serves these routes at all.
	if cfg.AdminPort != "" && cfg.AdminToken != "" {
		servers = append(servers, &http.Server{
			Addr:              addr(cfg.AdminPort),
			Handler:           server.BuildAdmin(cfg, db, server.Seams{}),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		})
		slog.Info("admin listener enabled", "addr", addr(cfg.AdminPort))
	}

	errCh := make(chan error, len(servers))
	for _, s := range servers {
		go func(s *http.Server) {
			slog.Info("listening", "addr", s.Addr)
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
			errCh <- nil
		}(s)
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errCh:
		// One listener failing leaves a half-serving binary, which is worse than
		// exiting: shut the other down too.
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(shutdownCtx)
	}
	return runErr
```

Check how `ctx` is derived in `main.go` — if `serve` does not already receive a signal-cancelled context, wrap with `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` at the top of the function and add the `os`/`os/signal`/`syscall` imports.

- [ ] **Step 2: Verify it builds and the binary runs**

```bash
go build ./... && go vet ./...
```

- [ ] **Step 3: Verify by hand that the listener is conditional**

```bash
# Without the admin variables: only the main port listens.
PORT=8181 DATABASE_URL=sqlite:///tmp/admincheck.sqlite go run ./cmd/econumo serve &
sleep 2
curl -s -o /dev/null -w '%{http_code}\n' localhost:9090/admin/user-context || echo "refused (expected)"
kill %1
```

Expected: connection refused on 9090.

```bash
# With them: 401 without a bearer, 400 with one but no userId.
PORT=8181 ECONUMO_ADMIN_PORT=9090 ECONUMO_ADMIN_TOKEN=$(head -c 32 /dev/zero | tr '\0' 'k') \
  DATABASE_URL=sqlite:///tmp/admincheck.sqlite go run ./cmd/econumo serve &
sleep 2
curl -s localhost:9090/admin/user-context
curl -s -H "Authorization: Bearer kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk" localhost:9090/admin/user-context
kill %1
```

Expected: the first prints the 401 envelope, the second the 400 "should not be blank" envelope.

- [ ] **Step 4: Commit**

```bash
git add cmd/econumo/main.go
git commit -m "feat(serve): run the admin listener alongside the API"
```

---

### Task 8: Guards

**Files:**
- Modify: `internal/test/apiparity/guard_test.go:20-53`
- Create: `internal/test/apiparity/admin_reachability_test.go`

**Interfaces:**
- Consumes: `server.BuildAPI`.
- Produces: nothing importable.

- [ ] **Step 1: Write the failing reachability guard**

`internal/test/apiparity/admin_reachability_test.go`:

```go
package apiparity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The whole design rests on admin routes being unreachable from the public
// mux: a misconfigured reverse proxy must not be able to expose them, because
// they are not registered there at all. This asserts that property rather than
// leaving it to be maintained by hand.
func TestAdminRoutesAreNotOnThePublicMux(t *testing.T) {
	handler := buildTestAPI(t) // reuse this file's existing harness constructor
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/admin/set-access"},
		{http.MethodGet, "/admin/user-context"},
		{http.MethodGet, "/admin/user-context?userId=x"},
		{http.MethodGet, "/api/v1/admin/user-context"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		// The SPA catch-all answers unknown paths, so anything other than a 2xx
		// admin response is fine; a 200 carrying admin JSON is not.
		if rec.Code == http.StatusOK && rec.Header().Get("Content-Type") == "application/json" {
			t.Fatalf("%s %s is reachable on the public mux: %s", tc.method, tc.path, rec.Body.String())
		}
	}
}
```

Replace `buildTestAPI(t)` with whatever this package already uses to construct the production handler (grep the package for `server.BuildAPI`).

- [ ] **Step 2: Run it**

Run: `go test ./internal/test/apiparity/ -run AdminRoutes -v`
Expected: PASS immediately — admin routes were never registered on the public mux. This guard is a regression net, so a first-run pass is correct; if it FAILS, something is wired wrong in Task 6.

- [ ] **Step 3: Exclude `internal/admin` from the route scan**

In `internal/test/apiparity/guard_test.go`, inside `registeredRoutes`, skip the admin package and explain why:

```go
	handlerGlobs := []string{"internal/*/api/routes.go"}
	// internal/admin registers the PRIVATE admin listener, which is served by a
	// separate http.Server and never mounted on the public mux — so it has no
	// parity scenario and no golden. Excluded explicitly rather than by naming
	// the file outside the glob, so the reason survives the next refactor.
	// TestAdminRoutesAreNotOnThePublicMux asserts the separation holds.
	const excluded = "internal/admin/api/routes.go"
	routes := map[string]bool{}
	for _, g := range handlerGlobs {
		files, err := filepath.Glob(filepath.Join(repoRoot, g))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if rel, rerr := filepath.Rel(repoRoot, f); rerr == nil && filepath.ToSlash(rel) == excluded {
				continue
			}
			// ... existing body unchanged ...
		}
	}
```

- [ ] **Step 4: Run the full guard suite**

Run: `go test ./internal/test/apiparity/ -v`
Expected: PASS. `minRoutes` is 88 from Task 3 and the scan still finds every public route.

- [ ] **Step 5: Commit**

```bash
git add internal/test/apiparity/
git commit -m "test(apiparity): guard that admin routes stay off the public mux"
```

---

### Task 9: Documentation and final verification

**Files:**
- Modify: `CLAUDE.md` (Configuration section; Architecture feature list)

- [ ] **Step 1: Document the configuration**

In `CLAUDE.md`'s configuration list, after the `ECONUMO_TRIAL` entry:

```markdown
- `ECONUMO_ADMIN_PORT` / `ECONUMO_ADMIN_TOKEN` — the private admin listener the payment
  portal talks to (`POST /admin/set-access`, `GET /admin/user-context`). A **second**
  `http.Server`, started by `serve` only when BOTH are set, so a self-hosted instance
  never serves these routes. Auth is `Authorization: Bearer <ECONUMO_ADMIN_TOKEN>`
  compared in constant time; the token is also the HMAC key for billing-handoff tokens
  (minimum 32 characters). Half-configured fails at boot.
- `ECONUMO_BILLING_URL` — payment portal URL. Empty (default) means
  `POST /api/v1/user/create-billing-link` returns 400 and the SPA shows no billing UI.
  Merged into the served `econumo-config.js` as `BILLING_URL`, so one variable drives
  both halves. Requires `ECONUMO_ADMIN_TOKEN` (the signing key).
```

- [ ] **Step 2: Add the feature to the architecture list**

In the feature-package paragraph, change "ten features" to "eleven features" and add `admin` to the alphabetical list in both places it appears. Then add under the tree:

```markdown
The `admin` feature is the private listener for the payment portal; it is the one
feature whose HTTP edge is NOT mounted on the public mux (see `internal/server/admin.go`
and the reachability guard in `internal/test/apiparity`).
```

- [ ] **Step 3: Verify the count claim**

Run: `ls -d internal/*/ | wc -l` and confirm the feature count in `CLAUDE.md` matches reality after adding `admin`.

- [ ] **Step 4: Full gate**

```bash
make go-test
```

Expected: PASS, including gofmt, vet, OpenAPI freshness, the i18n parity guards, and the `GO_COVER_MIN=72` coverage floor.

If coverage dropped below the floor, add tests to the thinnest new package (usually `internal/server/admin.go`) rather than lowering the gate.

- [ ] **Step 5: Confirm no golden drifted**

```bash
git diff --stat main -- internal/test/apiparity/testdata/golden/
```

Expected: exactly one added golden (`create-billing-link`), zero modified. A modified golden means an existing response changed — investigate before committing.

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: admin listener and billing configuration"
```

---

## Self-Review

**Spec coverage:**

| spec section | task |
|---|---|
| Defect 1 — allowlist | 3 (steps 8–9) |
| Defect 2 — one `BILLING_URL` | 1, 3, 9 |
| Ids-only addressing | 4 (DTO has no email field) |
| `internal/admin/` package + ports + glue | 4, 6 |
| Two servers, conditional | 1, 6, 7 |
| Middleware chain | 5, 6 |
| `set-access` incl. idempotence | 4, 6 |
| `user-context` incl. three access fields | 4, 6 |
| N+1 accepted | 4 (`context.go`) |
| Handoff token + domain separation | 2 |
| `create-billing-link` + `lang` + `for` validation | 3 |
| Boot validation, token length | 1 |
| apiparity exclusion + reachability guard | 8 |
| No new SQL | — (nothing to do; asserted by there being no migration task) |
| Testing section | distributed; every bullet has a step |

**Deferred deliberately:** `GET /admin/expiring-users` is out of scope per the spec.

**Known soft spots for the implementer:**
- Task 6 Step 3 does not spell out `BuildAdmin`'s shared wiring verbatim, because it must mirror `BuildAPI` as it exists at implementation time. Read `internal/server/server.go` first; extract a shared helper rather than duplicating.
- Task 5 Step 5 and Task 6 Step 4 describe test bodies rather than giving them in full, because they depend on this repo's existing `dbtest`/`fixture` helper signatures. Copy the setup from the neighbouring `glue_*_test.go` files.
- Several snippets reference `vo.NewId`, `port.Clock.Now`, and `errs.FieldError` field names from memory. Verify each against the source on first use; they are used consistently across tasks, so one correction propagates predictably.
