# OAuth Login Review Round 8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the round-8 security findings on PR #238 (head 36de1af): a revoke that a concurrent touch can undo, an email-drift write that crosses the reclaim fence, a claimable custom URL scheme for the app return, unvalidated discovery endpoints, and the parked double-unlink lockout.

**Architecture:** Token lifecycle writes become monotonic single statements (touch only while unrevoked; revoke by id / by set — never a whole-row write from memory). The drift mirror becomes one conditional UPDATE fenced on generation AND passwordless status. Discovery endpoints are validated as absolute HTTPS (loopback http allowed). The app return moves to verified Universal/App Links served from `ECONUMO_URL` (`/.well-known/…` generated from config) with the private scheme renamed to reverse-domain `com.econumo.app` as the documented fallback. `oauth.Service` gets a transaction runner so unlink can lock the user row.

**Tech Stack:** Go 1.27 (`/usr/local/go/bin/go`, `GOTOOLCHAIN=go1.27.1`), sqlc v1.30 (`~/go/bin/sqlc`), React 19 + vitest in `web/` (pnpm; `web/node_modules` is installed), Capacitor manifests in `mobile/`.

**Spec:** `docs/superpowers/specs/2026-09-07-oauth-login-design.md` — this plan amends §6.3 (app return), §11 (fence), §5 (discovery); Task 6 edits the spec.

## Global Constraints

- Branch: `feature/oauth-login-fixes` (worktree `.claude/worktrees/bridge-cse_01Qx2CadepXjbi8Cte53yHaC`, HEAD 36de1af, already pushed as `feature/oauth-login`). One commit per task; conventional prefix; every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`. Stage with explicit paths; never commit `.remember/`.
- Go: `PATH=/usr/local/go/bin:$HOME/go/bin:$PATH GOTOOLCHAIN=go1.27.1 CGO_ENABLED=0 go …` from the worktree root. After editing any `internal/infra/storage/sqlc/query/{sqlite,pgsql}/*.sql`: `cd internal/infra/storage/sqlc && ~/go/bin/sqlc generate && cd -`; commit `gen/` with the query; never hand-edit `gen/`; same `-- name:` and column list on both engines (`?` vs `$N`); the statement-ending `;` on the last line; `.sql` files ASCII-only including comments.
- Frozen wire contract: no response shape/message/route changes; 401 texts stay `"Invalid access token"` / `"Invalid credentials."`. Goldens: only Task 3 changes goldens (`http://idp.example.test` → `https://idp.example.test`); regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`, inspect, never hand-edit.
- TDD: failing test first (a race reproducer where the finding is a race), watch it fail, then implement; RED/GREEN evidence in the report.
- Web: from `web/`, foreground with timeouts: `timeout 600 pnpm test -- <file>`; `pnpm lint && npx tsc -b` clean before committing (known `transaction.test.ts` Blob failure excepted).
- Docs travel with behavior (CLAUDE.md, `docs/oidc-setup.md`, `mobile/README.md`, `docs/regression-test-plan.md`, the spec) in the same task. Comments only for the non-obvious why. Never log emails/tokens/hashes.

---

### Task 1: Token touch and revoke are monotonic SQL statements

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/access_tokens.sql` (replace `UpdateAccessToken` with `TouchAccessToken`, `RevokeAccessToken`, `RevokeUserAccessTokens`)
- Regenerate: `gen/`
- Modify: `internal/user/repository.go` (`AccessTokens`: delete `Update`; add `Touch`, `Revoke`, `RevokeAll`), `internal/user/repo/accesstoken.go` + `_sqlite.go` + `_pgsql.go`
- Modify: `internal/user/authenticate.go:24-30`, `internal/user/session.go:97-99,124-135`, `internal/user/pat.go:102-105`, `internal/user/usecase.go:117-119`
- Test: `internal/user/repo/accesstoken_integration_test.go`, `internal/user/authenticate_test.go`

**Interfaces:**
- Produces: `AccessTokens.Touch(ctx, id vo.Id, lastUsedAt time.Time, expiresAt *time.Time) (int64, error)` — `UPDATE … SET last_used_at, expires_at WHERE id = ? AND revoked_at IS NULL`; `AccessTokens.Revoke(ctx, id vo.Id, now time.Time) error` — `SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`; `AccessTokens.RevokeAll(ctx, userID vo.Id, kind string, exceptID vo.Id, now time.Time) error` — `SET revoked_at = ? WHERE user_id = ? AND kind = ? AND revoked_at IS NULL AND id <> ?` (pass the zero id's string for "no exception"; it matches no row).
- `model.AccessToken.Touch`/`Revoke` stay as in-memory helpers for callers that still need the struct.

- [ ] **Step 1: Failing repo test — a touch cannot un-revoke**

In `internal/user/repo/accesstoken_integration_test.go`:

```go
func TestTouch_DoesNotResurrectARevokedToken(t *testing.T) {
	db := dbtest.New(t)
	r := NewAccessTokenRepo(db.Engine, backend.NewTxManager(db.Raw))
	u := seedUser(t, db) // the file's existing user helper
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	exp := now.Add(time.Hour)
	tok := &model.AccessToken{ID: vo.NewId(), UserID: u.ID, Kind: model.TokenKindSession, TokenHash: "h-touch",
		CreatedAt: now, LastUsedAt: now, ExpiresAt: &exp}
	if n, err := r.InsertIfGeneration(ctx, tok, 0); err != nil || n != 1 {
		t.Fatalf("insert: %d %v", n, err)
	}
	// The request read the row (revoked_at NULL) …
	loaded, _, _, err := r.GetByHash(ctx, "h-touch")
	if err != nil {
		t.Fatal(err)
	}
	// … the reclaim revokes it …
	if err := r.Revoke(ctx, tok.ID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// … and the request's touch lands afterwards with the stale snapshot.
	later := now.Add(10 * time.Minute)
	loaded.Touch(later, 30*24*time.Hour)
	n, err := r.Touch(ctx, loaded.ID, loaded.LastUsedAt, loaded.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("touch wrote %d rows on a revoked token", n)
	}
	after, err := r.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.RevokedAt == nil {
		t.Fatal("the touch resurrected the revoked token")
	}
}
```

- [ ] **Step 2: Run — FAIL to compile (`Revoke`/`Touch` undefined)**

Run: `go test ./internal/user/repo/ -run TestTouch_DoesNotResurrectARevokedToken`

- [ ] **Step 3: Replace the whole-row UPDATE with three monotonic statements**

`query/sqlite/access_tokens.sql` (delete `UpdateAccessToken`):

```sql
-- name: TouchAccessToken :execrows
-- The sliding-expiry touch never writes revoked_at and never touches a row a
-- reclaim has revoked: a request that read the row before the revoke must not
-- be able to write a stale NULL back.
UPDATE access_tokens SET last_used_at = ?, expires_at = ? WHERE id = ? AND revoked_at IS NULL;

-- name: RevokeAccessToken :exec
UPDATE access_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL;

-- name: RevokeUserAccessTokens :exec
-- Set-based, so a revoke sweep is one statement and cannot race a concurrent
-- touch row by row. exceptID is the presenting token (or an id that matches
-- nothing when everything must go).
UPDATE access_tokens SET revoked_at = ? WHERE user_id = ? AND kind = ? AND revoked_at IS NULL AND id <> ?;
```

pgsql: `$1..$3` / `$1..$2` / `$1..$4`. `sqlc generate`. Repository interface: delete `Update`; add the three methods with the doc lines above (shortened). Implement in `accesstoken.go` + both adapters (the `Touch` param struct carries `LastUsedAt`, `ExpiresAt`, `ID`).

Callers:
- `authenticate.go`: keep `t.NeedsTouch`; replace the `Update` with `n, err := s.tokens.Touch(ctx, t.ID, now, expiresAfterTouch)` where `expiresAfterTouch` is `now.Add(SessionTTL)` for sessions and `t.ExpiresAt` for PATs (compute via `t.Touch(now, SessionTTL)` then pass `t.LastUsedAt, t.ExpiresAt`); `if n == 0 { return … errs.NewUnauthorized("Invalid access token") }` — a row that vanished or got revoked between read and touch is not authenticated.
- `session.go` `RevokeSession`, `pat.go` `RevokePersonalToken`, `usecase.go` `Logout`: replace `t.Revoke(now); s.tokens.Update(ctx, t)` with `s.tokens.Revoke(ctx, t.ID, s.clock.Now())` (keep the preceding ownership/kind checks and the provider/id_token reads in Logout).
- `session.go` `revokeTokens`: replace the list-and-update loop with one `RevokeAll` per kind.

- [ ] **Step 4: Failing service test — Authenticate racing a reclaim fails closed**

In `internal/user/authenticate_test.go` (reuse `newAuthEnv` and the decorator pattern from `login_test.go` — here decorate `AccessTokens.GetByHash` so that, once, right after returning the live row, it calls the real `Service.ResetPassword`/`reclaimCredentials` path or simply `tokens.RevokeAll(ctx, userID, kind, vo.Id{}, now)`):

```go
func TestAuthenticate_RevokeBetweenReadAndTouchFailsClosed(t *testing.T) {
	env := newAuthEnv(t)
	raw := env.mintSession(t) // whatever helper mints a session and returns the raw token
	env.clock.advance(10 * time.Minute) // past touchInterval so a touch is due
	env.tokens.afterGetByHash = func(tok *model.AccessToken) { _ = env.rawTokens.RevokeAll(context.Background(), tok.UserID, tok.Kind, vo.Id{}, env.clock.Now()) }
	_, _, _, err := env.svc.Authenticate(context.Background(), raw)
	var unauthorized *errs.UnauthorizedError
	if !errors.As(err, &unauthorized) || unauthorized.Msg != "Invalid access token" {
		t.Fatalf("want 401 Invalid access token, got %v", err)
	}
	if _, _, _, err := env.svc.Authenticate(context.Background(), raw); err == nil {
		t.Fatal("the token authenticates again: the touch resurrected it")
	}
}
```

Run: FAIL before the callers change (the old code wrote `revoked_at = NULL` back). After Step 3: PASS.

- [ ] **Step 5: Whole tier, commit**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./internal/user/... ./internal/server/... ./internal/cli/... ./internal/test/apiparity/ ./internal/test/mcpparity/` — PASS, no golden diffs (a touch on a live token still slides exactly as before).

```bash
git commit -m "fix(user): make token touch and revoke monotonic so a touch can never un-revoke

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: The email-drift mirror is fenced on generation and passwordless status

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql` (add `UpdateUserEmailIfPasswordlessAndGeneration`)
- Regenerate: `gen/`
- Modify: `internal/user/repository.go` (add `ReplaceEmailIfPasswordless`), `internal/user/repo/repo.go` + adapters, `internal/user/external.go:58-69` (`ReplaceVerifiedEmail` gains `generation int64`, returns `(int64, error)`)
- Modify: `internal/oauth/ports.go` (`Users.ReplaceVerifiedEmail(ctx, userID, email, generation) (int64, error)`), `internal/server/glue_oauth_users.go`, `internal/oauth/callback.go:303-329` (`mirrorEmailDrift` takes the generation; 0 rows → skip with a log line)
- Test: `internal/oauth/service_test.go` (+ its `fakeUsers`), `internal/oauth/api/harness_test.go` fake, `internal/user/external_test.go`

**Interfaces:**
- Produces: `Repository.ReplaceEmailIfPasswordless(ctx, userID vo.Id, encryptedEmail string, now time.Time, generation int64) (int64, error)` — `UPDATE users SET email = ?, email_verified = 1, updated_at = ? WHERE id = ? AND credentials_generation = ? AND algorithm = 'none'` (pgsql: `email_verified = TRUE`, `$1..$4`).

- [ ] **Step 1: Failing service test — a reset between the drift checks and the write is refused**

Using the existing harness in `internal/oauth/service_test.go` (real sqlite repos; `fakeUsers` with the `beforeFindByID`-style hook — add a `beforeReplaceEmail func()` hook):

```go
func TestCallback_EmailDrift_ResetBetweenChecksAndWriteIsRefused(t *testing.T) {
	h := newHarness(t)
	u := h.users.seed(t, "victim@x.test", model.AlgorithmNone)
	h.identities.add(model.NewIdentity(vo.NewId(), u.ID, "oidc", h.idp.IssuerURL(), h.idp.Subject, "victim@x.test", h.clock.Now()))
	h.idp.Email = "attacker@x.test" // the provider now reports a drifted address
	h.users.beforeReplaceEmail = func() { h.users.reclaim(u.ID) } // reset commits: bump + password set
	_ = h.completeLogin(t)
	after, _ := h.users.FindByID(context.Background(), u.ID)
	if after.Email != "victim@x.test" {
		t.Fatalf("drift mirror crossed the reclaim: email is now %q", after.Email)
	}
}
```

(`reclaim` must also set a password/`Algorithm = argon2id` on the stored user, as a real reset does.) Run: FAIL (email replaced).

- [ ] **Step 2: Implement the conditional write**

Add the query (both engines), regenerate, implement `ReplaceEmailIfPasswordless` in the repo. `external.go`:

```go
// ReplaceVerifiedEmail mirrors an IdP-side email change onto the primary email
// ONLY while the account is still passwordless and still at the generation the
// callback resolved it under; the database decides, so a reset committing after
// the eligibility checks leaves the recovered account's address alone.
func (s *Service) ReplaceVerifiedEmail(ctx context.Context, userID vo.Id, email string, generation int64) (int64, error) {
	encrypted, err := s.encode.Encode(strings.TrimSpace(email))
	if err != nil {
		return 0, err
	}
	return s.repo.ReplaceEmailIfPasswordless(ctx, userID, encrypted, s.clock.Now(), generation)
}
```

`callback.go`: `mirrorEmailDrift(ctx, u, email, provider)` uses `u.CredentialsGeneration`; on `n == 0` log `"oauth email drift: skipped, account reclaimed or now has a password"` at WARN with `user_id`. Update the port, the glue, both fakes, and `internal/user/external_test.go`'s `TestReplaceVerifiedEmail` (add a case: wrong generation → 0 rows; password account → 0 rows).

- [ ] **Step 3: Run, commit**

Run: `go test ./internal/user/... ./internal/oauth/... ./internal/server/... ./internal/test/apiparity/` — PASS.

```bash
git commit -m "fix(oauth): fence the email-drift mirror on generation and passwordless status

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Discovery endpoints are validated as absolute HTTPS URLs

**Files:**
- Modify: `internal/infra/oidc/client.go:95-115` (`Discover`)
- Modify: `internal/test/apiparity/harness.go` (`PublicURL = "https://idp.example.test"`), regenerate goldens (3 lines)
- Modify: `internal/infra/oidc/oidctest/fake.go` `Transport()` (already forces scheme `http` onto the fake — confirm)
- Test: `internal/infra/oidc/client_errors_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestDiscover_RejectsNonHTTPSEndpoints(t *testing.T) {
	cases := map[string]func(*oidctest.Fake){
		"http token endpoint":       func(f *oidctest.Fake) { f.TokenEndpointOverride = "http://idp.example.test/token" },
		"javascript authorization": func(f *oidctest.Fake) { f.AuthorizationEndpointOverride = "javascript:alert(1)" },
		"relative jwks":             func(f *oidctest.Fake) { f.JWKSOverride = "/jwks" },
		"http userinfo":             func(f *oidctest.Fake) { f.UserInfoOverride = "http://idp.example.test/userinfo" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := oidctest.New(t)
			f.PublicURL = "https://idp.example.test"
			mutate(f)
			c := oidc.NewClient(f.Issuer("oidc", false), &http.Client{Transport: f.Transport()})
			if _, err := c.Discover(context.Background()); err == nil {
				t.Fatal("discovery accepted an insecure endpoint")
			}
		})
	}
}

func TestDiscover_AllowsLoopbackHTTP(t *testing.T) {
	f := oidctest.New(t) // PublicURL unset: http://127.0.0.1:<port>
	if _, err := oidc.NewClient(f.Issuer("oidc", false), f.Server.Client()).Discover(context.Background()); err != nil {
		t.Fatalf("loopback http must stay allowed for development: %v", err)
	}
}
```

Add the four `*Override string` fields to `oidctest.Fake` (used by `discovery()` when non-empty). Run: FAIL (compile, then acceptance).

- [ ] **Step 2: Validate**

In `client.go`:

```go
// validateEndpoint enforces the OpenID Connect Discovery rule that provider
// endpoints are absolute HTTPS URLs: an http token/JWKS/userinfo endpoint
// would hand the client secret, the signing keys or the access token to
// anyone on the path, and a non-http(s) authorization endpoint would be
// handed to the browser verbatim. Plain http is allowed for loopback hosts
// only (local development issuers).
func validateEndpoint(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("oidc: discovery %s is not an absolute URL", name)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("oidc: discovery %s must use https", name)
}
```

with `isLoopbackHost` (net.ParseIP → IsLoopback, or `localhost`) — copy the shape from `internal/config/config.go:isLoopbackHost` rather than importing config into infra. In `Discover`, after the non-empty check: validate `AuthorizationEndpoint`, `TokenEndpoint`, `JWKSURI` always, `UserinfoEndpoint` and `EndSessionEndpoint` when non-empty. Error messages carry the endpoint NAME, never the URL value.

- [ ] **Step 3: Harness to https, goldens, commit**

`harness.go`: `fakeIDP.PublicURL = "https://idp.example.test"` (the `Transport()` rewrites the scheme to http for the in-process server — verify in `fake.go` and fix if it only rewrites the host). Regenerate goldens: exactly the 3 lines in `oauth_flows.golden` change `http://idp.example.test` → `https://idp.example.test`. Run: `go test ./internal/infra/oidc/... ./internal/oauth/... ./internal/server/... ./internal/test/apiparity/ ./internal/test/mcpparity/ && go vet -tags enginecompare ./internal/test/enginecompare/`. CLAUDE.md: in the OAuth bullet add "discovered endpoints must be absolute https URLs (plain http only for loopback issuers); a document that violates that is rejected".

```bash
git commit -m "fix(oidc): reject discovery documents whose endpoints are not absolute HTTPS URLs

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Unlink holds the user row so two concurrent unlinks cannot strand a passwordless account

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql` (add `LockUserRow`)
- Regenerate: `gen/`
- Modify: `internal/user/repository.go` (add `LockRow`), `internal/user/repo/repo.go` + adapters; `internal/oauth/ports.go` (`Users.LockRow(ctx, userID) error`), `internal/server/glue_oauth_users.go`
- Modify: `internal/oauth/service.go` (`NewService` gains `tx port.TxRunner` after `handoffs`), `internal/oauth/identities.go` (`UnlinkIdentity`)
- Modify every `appoauth.NewService(` caller (`internal/server/server.go`, `internal/cli/container.go`, the oauth/api/server tests) — pass the real `txm` (`backend.NewTxManager(db.Raw)` in tests)
- Test: `internal/oauth/service_test.go`

**Interfaces:**
- Produces: `Repository.LockRow(ctx, userID vo.Id) error` — `UPDATE users SET updated_at = updated_at WHERE id = ?` (same text both engines; on PostgreSQL the row lock is held to commit, on SQLite the write serializes on the single writer).

- [ ] **Step 1: Failing test — two concurrent unlinks leave one identity**

```go
func TestUnlinkIdentity_ConcurrentUnlinksKeepOneSignInMethod(t *testing.T) {
	h := newHarness(t) // real sqlite repos + real tx runner
	u := h.users.seed(t, "p@x.test", model.AlgorithmNone)
	h.identities.add(model.NewIdentity(vo.NewId(), u.ID, "google", "https://accounts.google.com", "g", "p@x.test", h.clock.Now()))
	h.identities.add(model.NewIdentity(vo.NewId(), u.ID, "apple", "https://appleid.apple.com", "a", "p@x.test", h.clock.Now()))
	// Both requests pass the count check before either deletes.
	h.identities.afterCount = func() { <-h.gate } // a channel the test closes after the second call has counted
	// … run two UnlinkIdentity calls in goroutines (google, apple), release the gate, wait …
	n, _ := h.identities.CountByUser(context.Background(), u.ID)
	if n != 1 {
		t.Fatalf("passwordless user left with %d identities", n)
	}
}
```

Shape the synchronisation so it demonstrates the lost-update on the OLD code deterministically (a hook after `CountByUser` that blocks until both goroutines have counted). With the row lock, the second transaction cannot count until the first commits, so the hook ordering changes: adapt the test so it asserts the outcome (exactly one identity remains and one call returned `CodeOAuthLastIdentity`) rather than the interleaving. Run: FAIL on the old code (0 identities).

- [ ] **Step 2: Implement**

`UnlinkIdentity` body runs inside `s.tx.WithTx(ctx, func(ctx) error { s.users.LockRow(ctx, userID); … existing checks and delete … })`; `LockRow` first, then `GetByUserProvider`, `FindByID`, `CountByUser`, `DeleteByUserProvider` all inside. The `Users` port gains `LockRow`; the glue forwards to `userSvc.LockRow` (new one-liner on the user service over the repo).

- [ ] **Step 3: Run, docs, commit**

Run: `go test ./internal/oauth/... ./internal/server/... ./internal/cli/... ./internal/test/apiparity/` — PASS. Update the regression plan's "Unlink is refused for a passwordless user's last remaining identity" item: "…including two unlink requests sent at the same time — one succeeds, the other is refused."

```bash
git commit -m "fix(oauth): lock the user row while unlinking so concurrent unlinks cannot strand a passwordless account

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: App return via verified Universal/App Links (server side)

**Files:**
- Modify: `internal/config/config.go` (`AppLinksIOSAppIDs []string`, `AppLinksAndroid []AndroidAppLink{Package, Fingerprints []string}`, `AppLinksEnabled()`), `internal/config/config_test.go`
- Create: `internal/web/applinks/applinks.go` (+ `_test.go`): handlers for `/.well-known/apple-app-site-association` and `/.well-known/assetlinks.json`
- Modify: `internal/web/router/router.go` (mount both on the root mux next to `/health`, only when enabled), `internal/server/server.go`
- Modify: `internal/oauth/service.go` (`NewService` gains `appLinks bool`; `successURL`/`linkPendingURL`/`errorURLFor`/`errorURL` app branches), `internal/oauth/callback.go:clientFromState` unchanged
- Modify: `.env.example`, `CLAUDE.md` (Configuration + the OAuth bullet), `docs/oidc-setup.md` (new "Mobile app return" section)
- Test: `internal/oauth/service_test.go`, `internal/web/router/router_test.go`

**Interfaces:**
- Produces: `config.Config.AppLinksIOSAppIDs`, `config.Config.AppLinksAndroid`, `config.Config.AppLinksEnabled() bool`.
- Produces: app-flow redirect targets when enabled: success `<appURL>/oauth/app-return#handoff=<code>`, link pending `<appURL>/oauth/app-return#linkHandoff=<code>`, link error `<appURL>/oauth/app-return?linkError=<code>`, error `<appURL>/oauth/app-return?error=<code>`. When disabled: the private scheme, renamed to `com.econumo.app://oauth?…` (same parameters as today).
- Env: `ECONUMO_APP_LINKS_IOS` — comma-separated `TEAMID.bundle.id` entries (`^[A-Z0-9]{10}\.[A-Za-z0-9.-]+$`); `ECONUMO_APP_LINKS_ANDROID` — comma-separated `package.name=AA:BB:…` entries (fingerprint `^([0-9A-F]{2}:){31}[0-9A-F]{2}$`, upper-cased on load). Either set enables app links; a malformed entry fails boot; either requires `ECONUMO_URL` to be `https://` (a non-https app URL cannot host verified links).

- [ ] **Step 1: Failing config + handler tests**

`config_test.go`: valid iOS + Android parse; malformed team id fails; malformed fingerprint fails; `http://` `ECONUMO_URL` with app links fails; unset → `AppLinksEnabled() == false`.

`internal/web/applinks/applinks_test.go`:

```go
func TestAASA(t *testing.T) {
	h := Handler(config.Config{AppLinksIOSAppIDs: []string{"ABCDE12345.com.econumo.app"}})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil))
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	want := `{"applinks":{"details":[{"appIDs":["ABCDE12345.com.econumo.app"],"components":[{"/":"/oauth/app-return","comment":"OAuth return to the app"}]}]}}`
	if strings.TrimSpace(rr.Body.String()) != want {
		t.Fatalf("got %s", rr.Body.String())
	}
}

func TestAssetLinks(t *testing.T) {
	h := Handler(config.Config{AppLinksAndroid: []config.AndroidAppLink{{Package: "com.econumo.app", Fingerprints: []string{"AA:BB"}}}})
	// GET /.well-known/assetlinks.json ->
	// [{"relation":["delegate_permission/common.handle_all_urls"],"target":{"namespace":"android_app","package_name":"com.econumo.app","sha256_cert_fingerprints":["AA:BB"]}}]
}
```

(Serve each document only when its platform list is non-empty; the other returns 404.) `router_test.go`: with app links enabled the two paths are served on the root mux; disabled → 404. `service_test.go`: with `appLinks: true` the four app redirect targets are the https `app-return` URLs; with `false` they are `com.econumo.app://oauth?…`.

- [ ] **Step 2: Implement**

`applinks.go` builds both JSON documents with `encoding/json` (deterministic field order via structs), `Content-Type: application/json`, `Cache-Control: public, max-age=3600`. Router: `if cfg.AppLinksEnabled() { mux.Handle("GET /.well-known/apple-app-site-association", h); mux.Handle("GET /.well-known/assetlinks.json", h) }` on the ROOT mux (outside `/api`, like `/health` and `/mcp`, so the apiparity route scanner never sees them). `server.go` passes `cfg.AppLinksEnabled()` into `appoauth.NewService`. `service.go`:

```go
// appReturnURL is where an app flow lands. With verified app links configured
// the redirect is an https URL on ECONUMO_URL that only the associated app
// (Apple: Team ID + bundle; Android: package + signing certificate) may claim,
// so a look-alike app cannot receive the handoff. Without them the reverse-
// domain private scheme is the only option, and any app that registers it on
// the device receives the redirect (documented residual risk for self-hosted
// backends used from the store app).
func (s *Service) appReturnURL(fragment, query string) string {
	if s.appLinks {
		return s.appURL + "/oauth/app-return" + query + fragment
	}
	return "com.econumo.app://oauth" + query + fragment
}
```

and the four builders call it (`fragment = "#handoff=" + …`, `query = "?error=" + …`).

- [ ] **Step 3: Docs, run, commit**

`.env.example`: the two variables with one-line comments. CLAUDE.md Configuration: a bullet `ECONUMO_APP_LINKS_IOS` / `ECONUMO_APP_LINKS_ANDROID` (what they are, https requirement, that they only make sense on the domain the store app is associated with — `app.econumo.com` — and that a self-hosted backend used from the store app keeps the private scheme). `docs/oidc-setup.md`: "Mobile app return" section (how to obtain Team ID / signing fingerprint: `keytool -list -v -keystore …` / Play Console → App signing). Run: `go build ./... && go vet ./... && gofmt -l . && go test ./internal/config/... ./internal/web/... ./internal/oauth/... ./internal/server/... ./internal/test/apiparity/ ./internal/test/mcpparity/` — PASS; goldens unchanged (the harness has no app links).

```bash
git commit -m "feat(oauth): app return via verified Universal/App Links; reverse-domain private scheme as the fallback

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: App return — SPA route, deep-link dispatcher, mobile manifests, docs

**Files:**
- Modify: `web/src/lib/deepLinks.ts` (accept `https://<any host>/oauth/app-return` AND `com.econumo.app://oauth`; drop `econumo:`), `web/src/lib/deepLinks.test.ts`
- Create: `web/src/features/auth/AppReturnPage.tsx` (+ test), route `RouterPage.OAUTH_APP_RETURN = '/oauth/app-return'` in `web/src/app/router-pages.ts` + `routes.tsx`
- Modify: `mobile/ios/App/App/Info.plist` (scheme `com.econumo.app`), `mobile/android/app/src/main/AndroidManifest.xml` (scheme `com.econumo.app`; a second `VIEW` intent-filter with `android:autoVerify="true"` for `https` host `app.econumo.com` path `/oauth/app-return`)
- Modify: `mobile/README.md` (Associated Domains entitlement step for Xcode: `applinks:app.econumo.com`; Android autoVerify; how the two well-known files are served), `docs/regression-test-plan.md`, the spec §6.3, `locales/en.json` + the other 10 catalogues (two new keys: `auth.oauth.app_return.title`, `auth.oauth.app_return.open_app`), `web/src/lib/metrics.ts` (no new event — the page is a transit page; note it in the PR)
- Test: `web/src/features/auth/AppReturnPage.test.tsx`

- [ ] **Step 1: Failing tests**

`deepLinks.test.ts`: `handleAppUrl('https://app.econumo.com/oauth/app-return#handoff=abc')` → `navigateTo('/oauth/callback#handoff=abc')`; `handleAppUrl('com.econumo.app://oauth?error=denied')` → `/login?oauthError=denied`; `handleAppUrl('econumo://oauth?handoff=abc')` → ignored (no navigation). `AppReturnPage.test.tsx`: on the web (not native) with `#handoff=abc` it renders the title and a button whose `href` is `com.econumo.app://oauth?handoff=abc`; with `?error=denied` the button href is `com.econumo.app://oauth?error=denied`; the handoff is NOT put into the DOM anywhere except the href (no text).

- [ ] **Step 2: Implement**

`deepLinks.ts`:

```ts
// Two shapes reach the app: the verified https app link the backend prefers
// (only the associated app can claim it) and the reverse-domain private scheme
// it falls back to. Both carry the same parameters.
function isAppReturn(url: URL): boolean {
  if (url.protocol === 'com.econumo.app:' && url.host === 'oauth') return true
  return (url.protocol === 'https:') && url.pathname === '/oauth/app-return'
}
```

Parameters come from `url.searchParams` for `error`/`linkError` and from the fragment (`new URLSearchParams(url.hash.slice(1))`) for `handoff`/`linkHandoff`. `AppReturnPage.tsx`: reads the same, renders `t('auth.oauth.app_return.title')` and an `<a href={schemeUrl}>` button `t('auth.oauth.app_return.open_app')`; in the native app it renders nothing (the `appUrlOpen` handler already dispatched). The 10 non-English catalogues get the two keys translated (short strings: "Sign-in finished — return to the Econumo app." / "Open the app"); `internal/test/i18ntest` enforces parity.

Manifests: rename the scheme in both; Android adds:

```xml
<intent-filter android:autoVerify="true">
    <action android:name="android.intent.action.VIEW" />
    <category android:name="android.intent.category.DEFAULT" />
    <category android:name="android.intent.category.BROWSABLE" />
    <data android:scheme="https" android:host="app.econumo.com" android:pathPrefix="/oauth/app-return" />
</intent-filter>
```

`mobile/README.md`: Xcode step (Signing & Capabilities → Associated Domains → `applinks:app.econumo.com`; the AASA is served by the backend when `ECONUMO_APP_LINKS_IOS` is set), Android (autoVerify + `assetlinks.json` from `ECONUMO_APP_LINKS_ANDROID`; the signing fingerprint from Play App Signing), and the residual-risk note for self-hosted backends (private scheme).

- [ ] **Step 3: Run, docs, commit**

`cd web && timeout 600 pnpm test -- deepLinks AppReturnPage OAuthCallbackPage && pnpm lint && npx tsc -b`; `go test ./internal/test/i18ntest/`. Regression plan: replace the `econumo://` mentions with the app link + scheme fallback and add "📱 With app links configured, the return from the provider opens the app directly; opening the return URL in a browser with the app not installed shows the 'return to the app' page." Spec §6.3 updated accordingly.

```bash
git commit -m "feat(web,mobile): app return over verified app links with the reverse-domain scheme fallback

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Final gate

`make go-test`; `cd web && pnpm test && pnpm lint && npx tsc -b`; PostgreSQL: enginecompare + `DBTEST_ENGINE=pgsql` for `./internal/user/... ./internal/oauth/... ./internal/server/... ./internal/cli/...` (throwaway container recipe in memory `oauth-login-status.md`).

## Rulings

- The private scheme is kept as the fallback (renamed to reverse-domain `com.econumo.app`): a self-hosted backend used from the store app cannot be a verified app-link domain, so without the scheme those users could not sign in at all. Cost if wrong: the hijack remains possible for self-hosted backends; documented.
- The iOS Associated Domains entitlement is a documented Xcode step, not a `project.pbxproj` edit (hand-editing pbxproj is error-prone and cannot be verified here). Cost if wrong: iOS app links stay inactive until the owner does the step; the scheme fallback keeps sign-in working.
