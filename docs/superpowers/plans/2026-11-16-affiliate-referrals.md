# Affiliate Referrals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Credit Affonso partners for sign-ups and payments: the app stores the partner click id
on new users, billing reads it and reports sign-ups, payments (pre-tax) and refunds to Affonso.

**Architecture:** econumo.com's pixel sets `affonso_referral=<click id>` on `.econumo.com`. The
app copies that cookie (name from config, vendor-neutral) into a write-once user option
`affiliate` at account creation and serves it on the private admin listener: a polled list
(`GET /admin/affiliate-users`) and `GET /admin/user-context`. Billing records it from both, and
an outbox worker (like `internal/grants`) calls Affonso.

**Tech Stack:** Go (both repos), sqlc, SQLite+PostgreSQL (app), PostgreSQL (billing), Astro (website).

**Spec:** `docs/superpowers/specs/2026-11-16-affiliate-referrals-design.md`

**Repos and branches:**
- App: this worktree, branch `feature/affiliate-referrals` (Tasks 1-3).
- Billing: `~/dev/econumo/billing`, new worktree on branch `feature/affiliate-reports` from
  `origin/main` (Tasks 4-7). Read its `README.md` and `CLAUDE.md` if present; `make db` for tests.
- Website: `~/dev/econumo/website`, existing PR #92 branch `claude/affonso-pixel-tracking-10e7f9`
  (Task 8).

## Global Constraints

- The app never names the vendor: no "affonso" in `econumo/econumo` code, config names or docs
  except as the example value `ECONUMO_AFFILIATES=affonso:affonso_referral` in `docs/full.env.example`.
- App capture runs only when BOTH `ECONUMO_AFFILIATES` and `ECONUMO_BILLING_URL` are set.
- Options `affiliate` (program) and `affiliate_code`; admin JSON fields `affiliate` and `affiliateCode` (`""` when none); list endpoint takes a required `program`.
- Valid value: `^[A-Za-z0-9_-]{1,64}$`; anything else is dropped silently, never an error.
- Referral handling never fails or changes a sign-up, a portal page, a checkout, a webhook ack
  or a grant.
- Billing's Affonso integration is on only when `AFFONSO_API_KEY` is set
  (`AFFONSO_API_URL` default `https://api.affonso.io/v1`); off = no rows, no calls.
- Affonso identifies users by `external_user_id` = Econumo user id. No email is sent.
- `sale_amount` is pre-tax: `amount_net_cents / 100`, currency from the purchase.
- Comments: only the non-obvious why (app CLAUDE.md rule).

## Review Focus

- A cookie present on a LOGIN or a Settings link-a-provider flow must not write the option to an
  existing user (only new accounts) → Task 2 test `TestAffiliateNotWrittenOnExistingUserSignIn`.
- A malformed / oversized cookie (`a b`, 65 chars, `<x>`) is ignored and the sign-up still
  succeeds → Task 1 `TestNormalizeAffiliate`, Task 2 `TestRegisterWithMalformedAffiliateCookie`.
- The option must not leak to the SPA through `get-user-data`, `get-option-list` or the
  register/login user payload → Task 1 `TestServerOnlyOptionHidden` (all three paths).
- The app's list endpoint down or slow: the worker logs it and still drains queued reports;
  a later pull catches up through the cursor overlap → Task 7 `TestPullFailureStillDrains`.
- Affonso down / 5xx during a portal load or webhook: page and webhook behave exactly as
  without Affonso → Task 5 `TestContextSurvivesRecordFailure`, Task 7 retry tests.
- A refund arriving before its conversion was sent, or after the sign-up failed for good →
  Task 7 `TestRefundWaitsForConversion`, `TestEventsDroppedWhenSignupFailed`.

---

### Task 1: App model — the `affiliate` option, validation, hidden from client payloads

**Files:**
- Modify: `internal/model/user.go` (constants near line 24; setter next to `SetAI`)
- Create: `internal/model/affiliate.go`, `internal/model/affiliate_test.go`
- Modify: `internal/user/read.go` (`GetOptionList` ~line 60, get-user-data options ~line 113),
  `internal/user/usecase.go` (`toCurrentUser` options loop ~line 234)
- Test: `internal/user/*_test.go` (wherever get-user-data / option list / register are tested)

**Interfaces:**
- Produces: `model.OptionAffiliate = "affiliate"`; `model.NormalizeAffiliate(raw string) string`
  (raw if valid, else `""`); `func (u *User) SetAffiliate(value string, id vo.Id, now time.Time)`
  (no-op when value is `""` or the option already exists — write-once);
  `func (u *User) Affiliate() string` (program), `AffiliateCode()`; (superseded by the
  program/code split in the spec: `NormalizeAffiliateCode`, `SetAffiliate(program, code, newID, now)`); `model.IsServerOnlyOption(name string) bool` (true for
  `OptionAffiliate`).

- [ ] **Step 1: Write failing tests** `TestNormalizeAffiliate` (table: `"cmwirmwzi0031h9pc8rhijzge"`
  kept, `"a_b-C9"` kept, 64×`a` kept, 65×`a` → `""`, `"a b"` → `""`, `"<x>"` → `""`, `""` → `""`),
  `TestSetAffiliateWriteOnce` (second call with another value keeps the first; empty value adds
  nothing), and `TestServerOnlyOptionHidden`: a user with `affiliate` set shows no option named
  `affiliate` in `GetOptionList`, get-user-data and the `RegisterResult.User.Options`.
- [ ] **Step 2: Run** `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/model/ ./internal/user/ -run 'Affiliate|ServerOnly'` — FAIL (undefined).
- [ ] **Step 3: Implement** the produced API; skip `IsServerOnlyOption` names in the three option
  loops. Do NOT add `affiliate` to `persistedOptions` (it is not a seeded default).
- [ ] **Step 4: Run** the same command — PASS; then `go test ./internal/model/ ./internal/user/...`.
- [ ] **Step 5: Commit** `feat: Add the server-only affiliate user option`.

### Task 2: App capture — config, request context, middleware, write at creation

**Files:**
- Modify: `internal/config/config.go` (field next to `BillingURL` ~line 67; parse after the
  `ECONUMO_BILLING_URL` block ~line 349), `internal/config/config_test.go`
- Modify: `internal/shared/reqctx/reqctx.go` (+ test): new key `affiliateKey`
- Create: `internal/web/middleware/affiliate.go`, `internal/web/middleware/affiliate_test.go`
- Modify: `internal/web/router/router.go:170` — `root.Handle("/api/", global(apiMux))` wraps
  `apiMux` with `middleware.Affiliate(programs)` only when `len(deps.Cfg.Affiliates) > 0 &&
  deps.Cfg.BillingURL != ""`
- Modify: `internal/user/register.go` (`persistNewUser`, before `s.tx.WithTx`)
- Modify: `docs/full.env.example` (next to `ECONUMO_BILLING_URL`, line ~155)
- Test: `internal/user/register_test.go` (or the existing register/external test file),
  `internal/oauth/service_test.go` or `internal/oauth/api/*_test.go` for the provider path

**Interfaces:**
- Consumes: Task 1 `model.NormalizeAffiliate`, `(*User).SetAffiliate`.
- Produces: `config.Config.Affiliates []model.AffiliateCookie` (`ECONUMO_AFFILIATES`, `program:cookie` pairs; any non-empty
  value that is a valid cookie name per `net/http` token rules, else boot error);
  `reqctx.WithAffiliate(ctx, v string) context.Context`, `reqctx.Affiliate(ctx) string`;
  `middleware.Affiliate(cookieName string) func(http.Handler) http.Handler` — reads the cookie,
  stores `model.NormalizeAffiliate(value)` in the context when non-empty, otherwise passes
  through unchanged.

- [ ] **Step 1: Write failing tests:**
  - `TestAffiliateMiddleware`: request with cookie `affonso_referral=abc123` → handler sees
    `reqctx.Affiliate == "abc123"`; malformed value → `""`; no cookie → `""`.
  - `TestRegisterStoresAffiliate`: ctx with `reqctx.WithAffiliate(ctx, "abc123")` → registered
    user's `Affiliate() == "abc123"`.
  - `TestRegisterWithMalformedAffiliateCookie`: through the router with the cookie
    `affonso_referral=a b` → 200, user has no `affiliate` option.
  - `TestProvisionExternalUserStoresAffiliate`: same via `ProvisionExternalUser`.
  - `TestAdminCreateUserIgnoresAffiliate`: CLI path (`selfService=false`) stores nothing.
  - `TestAffiliateNotWrittenOnExistingUserSignIn`: OAuth callback for an existing user with the
    cookie set leaves the user without the option.
  - `TestRouterAffiliateGate`: with only one of `Affiliates` / `BillingURL` set, a
    register request carrying the cookie stores nothing.
  - Config test: `ECONUMO_AFFILIATES=affonso:affonso_referral` parses; `bad name` errors.
- [ ] **Step 2: Run** `go test ./internal/web/middleware/ ./internal/user/ ./internal/oauth/... ./internal/config/ ./internal/web/router/ -run 'Affiliate'` — FAIL.
- [ ] **Step 3: Implement.** In `persistNewUser`: when `selfService` and `reqctx.Affiliate(ctx)`
  is non-empty, `u.SetAffiliate(v, s.repo.NextIdentity(), now)`. The OAuth callback needs no
  change: provisioning runs on the callback request's ctx (Apple's POST callback carries the
  SameSite=None cookie too). Document the variable in `docs/full.env.example` as cloud-only,
  "captured only together with ECONUMO_BILLING_URL".
- [ ] **Step 4: Run** the same command — PASS; then `make go-lint`.
- [ ] **Step 5: Commit** `feat: Capture the affiliate cookie on new accounts`.

### Task 3: App admin — `affiliate` on `/admin/user-context` and `GET /admin/affiliate-users`

**Files:**
- Modify: `internal/admin/ports.go` (`UserRecord` + `Affiliate string`),
  `internal/admin/service.go` (`view`), `internal/model/admin_dto.go` (`AdminUserView`
  + `Affiliate string \`json:"affiliate"\``), `internal/server/glue_admin.go` (`record`:
  `Affiliate: u.Affiliate()`)
- Create: queries `ListAffiliateUsers` in `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql`
  (`SELECT user_id, value, created_at FROM users_options WHERE name = 'affiliate' AND created_at >= ?
  ORDER BY created_at, user_id LIMIT ?` — check the real option table/column names), `sqlc generate`
- Modify: `internal/admin/ports.go` (new port), `internal/admin/` (new `affiliates.go` use case),
  `internal/admin/api/routes.go` + handler, `internal/model/admin_dto.go`, `internal/server/glue_admin.go`,
  the user repo/read side that runs the query
- Test: `internal/admin/admin_test.go`, the admin API tests; regenerate goldens if any suite
  covers the admin listener (`UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect diff)

**Interfaces:**
- Consumes: Task 1 `(*User).Affiliate()`.
- Produces: wire field `user.affiliate` (and on each connection) in `GET /admin/user-context`;
  `""` when none. `POST /admin/set-access` returns the same view, so it carries it too.
- Produces: `GET /admin/affiliate-users?since=<"2006-01-02 15:04:05" UTC>&limit=<1..500, default 500>`
  → envelope `data: [{"userId","affiliate","createdAt"}]`, ordered by option `created_at`, then
  user id; `since` required. Port: `AffiliateLister.ListAffiliateUsers(ctx, since time.Time, limit int)
  ([]model.AdminAffiliateUser, error)`.

- [ ] **Step 1: Write failing tests** `TestUserContextIncludesAffiliate`: user with option →
  JSON `"affiliate":"abc123"`; user without → `"affiliate":""`. `TestListAffiliateUsers`: three
  referred users + one without → three rows in creation order; `since` filters inclusively;
  `limit=2` → first two; missing/garbled `since`, `limit=0`, `limit=501` → 400; the route is on
  the admin listener only (404 on the public mux). Run on both engines (`DBTEST_ENGINE`).
- [ ] **Step 2: Run** `go test ./internal/admin/... -run Affiliate` — FAIL.
- [ ] **Step 3: Implement** the field through record → view.
- [ ] **Step 4: Run** `make go-test` (smoke) — PASS.
- [ ] **Step 5: Commit** `feat: Serve the affiliate on the admin user context`; push the app
  branch, open a draft PR (body mentions @dmtrkzntsv), wait for CI.

### Task 4: Billing — config and Affonso client

**Files:**
- Modify: `internal/config/config.go` (+ test): optional group `AFFONSO_API_KEY`, `AFFONSO_API_URL`
- Create: `internal/affonso/client.go`, `internal/affonso/client_test.go` (httptest server)
- Modify: `.env.example`, `deploy/ship/billing.env.example`, `README.md` (one section)

**Interfaces:**
- Produces: `config.Config.AffonsoAPIKey, AffonsoAPIURL string`, `(*Config).AffonsoConfigured() bool`;
  package `affonso`:
  ```go
  type Client interface {
      Signup(ctx context.Context, clickID, userID string) error
      Conversion(ctx context.Context, c Conversion) (conversionID string, err error)
      Refund(ctx context.Context, conversionID, eventID string) error
  }
  type Conversion struct { UserID, EventID, Currency string; AmountCents int64 }
  var ErrPermanent = errors.New("affonso: permanent failure")
  func NewHTTP(baseURL, apiKey string, hc *http.Client) *HTTP
  ```
  Requests: `POST {base}/signups {"click_id","external_user_id"}`;
  `POST {base}/conversions {"external_user_id","sale_amount" (cents/100 as JSON number),
  "sale_amount_currency" (upper-case),"external_event_id"}` → `data.id`;
  `POST {base}/conversions/{id}/refund {"external_event_id"}`. `Authorization: Bearer <key>`.
  2xx (200 or 201) = success; 4xx except 408/429 wraps `ErrPermanent`; everything else retryable.
  Never log the key or a response body.

- [ ] **Step 1: Failing tests** `TestSignupRequest`, `TestConversionRequestAndID` (1999 cents EUR →
  `"sale_amount":19.99`, `"sale_amount_currency":"EUR"`, returns `com_1`), `TestRefundRequest`,
  `TestErrorClassification` (404 → `ErrPermanent`; 429, 500 → not), config tests (key alone on,
  URL defaulted; nothing set → off).
- [ ] **Step 2: Run** `go test ./internal/affonso/ ./internal/config/` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: Add the optional Affonso client`.

### Task 5: Billing — read `affiliate`, record referred users

**Files:**
- Create: `internal/infra/storage/migrate/migrations/pgsql/20261116000000.sql`; update
  `internal/infra/storage/sqlc/schema.sql`, `internal/infra/storage/sqlc/query/pgsql/billing.sql`,
  then `make sqlc`
- Modify: `internal/econumo/econumo.go` (`User.Affiliate string`; `Client` gains
  `AffiliateUsers(ctx, since time.Time, afterUserID string, limit int) ([]AffiliateUser, error)` with
  `type AffiliateUser struct { UserID, Affiliate string; CreatedAt time.Time }`),
  `internal/econumo/http.go` (`wireUser` + the new call), `internal/test/econumofake/fake.go` (settable), `cmd/econumo-stub` (one seeded
  user with an affiliate)
- Modify: `internal/billing/service.go` (`Service.Context`), `internal/billing/checkout.go`
  (`CreateCheckout`), `internal/billing/repo/` (new `affiliates.go` + test)

**Interfaces:**
- Produces: tables
  ```sql
  CREATE TABLE affiliate_referrals (
    user_id text PRIMARY KEY, click_id text NOT NULL,
    status text NOT NULL,            -- pending | reported | failed
    attempts int NOT NULL DEFAULT 0, next_attempt_at timestamptz,
    reported_at timestamptz, created_at timestamptz NOT NULL);
  CREATE TABLE affiliate_events (
    id uuid PRIMARY KEY, user_id text NOT NULL, purchase_id uuid NOT NULL REFERENCES purchases(id),
    kind text NOT NULL,              -- conversion | refund
    status text NOT NULL,            -- pending | sent | failed
    attempts int NOT NULL DEFAULT 0, next_attempt_at timestamptz,
    conversion_id text, created_at timestamptz NOT NULL,
    UNIQUE (purchase_id, kind));
  ALTER TABLE purchases ADD COLUMN amount_net_cents bigint;
  ```
  (`amount_net_cents` is filled by Task 6; it lives in this migration so there is one.)
  `repo.RecordAffiliate(ctx, userID, clickID string, now time.Time) error` — insert
  `ON CONFLICT (user_id) DO NOTHING`, status `pending`, `next_attempt_at = now`.
  `billing.Service` gets an `affiliatesOn bool` set by the constructor from config.

- [ ] **Step 1: Failing tests:** `TestContextRecordsAffiliate` (fake user with `Affiliate: "abc"`,
  affiliates on → one row; second load → still one row, click id unchanged),
  `TestContextIgnoresAffiliateWhenOff` (no row), `TestCheckoutRecordsAffiliate` (records the
  PAYER's own affiliate), `TestContextSurvivesRecordFailure` (repo error → `Context` returns
  normally, error logged), http client parse test for `"affiliate"`.
- [ ] **Step 2: Run** `make db` then `go test ./internal/billing/... ./internal/econumo/` — FAIL.
- [ ] **Step 3: Implement.** Record only `uc.User.Affiliate` (the caller), never connections'.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: Record referred users from the econumo context`.

### Task 6: Billing — pre-tax amounts and in-transaction event enqueue

**Files:**
- Modify: `internal/billing/repo/repo.go` (`RecordCompletedEvent` gains `amountNetCents *int64`;
  inside its tx, when the purchase becomes paid and `affiliate_referrals` has a row for its
  `payer_user_id` and `amountNetCents` is non-nil and > 0, insert an `affiliate_events`
  `conversion` row; `RecordRefundedEvent`: when it marks a purchase refunded and a `conversion`
  event exists for it, insert a `refund` row; both `ON CONFLICT (purchase_id, kind) DO NOTHING`)
- Modify: `internal/billing/webhook.go:97` (Stripe: `net := cs.AmountTotal - cs.TotalDetails.AmountTax`,
  nil when `TotalDetails` is nil), `internal/billing/webhook_paddle.go:76` (Paddle: parse
  `Totals.Total` and `Totals.Tax`, `net = total - tax`; a parse failure of these two leaves net
  nil rather than failing the webhook)
- Test: `internal/billing/repo/repo_test.go`, `webhook_test.go`, `webhook_paddle_test.go`

**Interfaces:**
- Consumes: Task 5 tables.
- Produces: `purchases.amount_net_cents` populated for new payments; `affiliate_events` rows
  that Task 7 drains.

- [ ] **Step 1: Failing tests:** Stripe completed with total 2380, tax 380 → `amount_net_cents`
  2000; Paddle total `"2380"` tax `"380"` → 2000; referred payer → one conversion event;
  non-referred payer → none; net 0 (100% off) → none; redelivered webhook → still one event;
  refund of a referred purchase with a conversion event → one refund event; refund without →
  none; a Paddle payload with an unparsable `tax` still grants (net nil, no event).
- [ ] **Step 2: Run** `go test ./internal/billing/...` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: Store pre-tax amounts and queue affiliate events`.

### Task 7: Billing — affiliate outbox worker and wiring

**Files:**
- Create: `internal/affiliates/worker.go`, `internal/affiliates/worker_test.go` (mirror
  `internal/grants/worker.go`: 30 s tick, `FOR UPDATE SKIP LOCKED`, backoff `30s * 2^attempts`,
  `maxAttempts = 10`)
- Modify: `internal/server/server.go` (build the worker only when `cfg.AffonsoConfigured()`),
  `cmd/billing/main.go:83` (`go affWorker.Run(ctx)` when non-nil)

**Interfaces:**
- Consumes: Task 4 `affonso.Client`, Task 5/6 tables.
- Produces: `affiliates.NewWorker(pool *pgxpool.Pool, ec econumo.Client, c affonso.Client, logger *slog.Logger) *Worker`,
  `(*Worker).Run(ctx)`, `(*Worker).Sweep(ctx) error`; table
  `affiliate_sync (id int PRIMARY KEY CHECK (id = 1), cursor timestamptz)` (add to Task 5's
  migration if Task 5 is not merged yet, else a new migration).

Sweep order per tick: (0) pull — first page `ec.AffiliateUsers(since = cursor - 10 min, or zero time,
afterUserID = "", 500)`; while a page is full, the next is `since = last.CreatedAt,
afterUserID = last.UserID` (the endpoint's keyset continuation; `since` alone repeats
same-second ties forever). `RecordAffiliate` each row, then store the newest
`CreatedAt` as the cursor; a pull failure is logged and the tick continues with (1). (1) pending sign-ups → `Signup`; success = `reported`. (2) pending
`conversion` events whose user's referral is `reported` → `Conversion` with
`EventID = purchase id`, store `conversion_id`, `sent`. (3) pending `refund` events whose
conversion event is `sent` → `Refund(conversion_id, "refund-"+purchase id)`. An event whose
referral is `failed`, or a refund whose conversion is `failed`, becomes `failed` without a call.
`ErrPermanent` or attempts reaching `maxAttempts` → `failed` + one ERROR log with ids only.

- [ ] **Step 1: Failing tests** (fake `affonso.Client`, `econumofake`): `TestPullRecordsAndAdvancesCursor`,
  `TestPullOverlapDoesNotDuplicate`, `TestPullPagesUntilShortPage`, `TestPullFailureStillDrains`,
  `TestSignupReported`,
  `TestConversionWaitsForSignup`, `TestRefundWaitsForConversion`,
  `TestEventsDroppedWhenSignupFailed`, `TestRetryBackoffThenFail` (500 ×10 → failed),
  `TestPermanentErrorFailsAtOnce`, `TestServerWithoutAffonsoHasNoWorker`.
- [ ] **Step 2: Run** `go test ./internal/affiliates/ ./internal/server/` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** full billing tests (`make test` or CI) — PASS.
- [ ] **Step 5: Commit** `feat: Report referred sign-ups, payments and refunds to Affonso`; push,
  open a draft PR in `econumo/billing` (mention @dmtrkzntsv), wait for CI.

### Task 8: Website — pixel on econumo.com only, privacy policy

**Files:**
- Modify (PR #92 branch): `public/apps/liltag-app.json` (remove tags `affonso` and
  `affonso-signup`), `src/content/docs/en/legal/privacy-policy.md`, the PR description

- [ ] **Step 1:** Remove both LilTag tags; confirm `liltag-app.json` parses (`node -e
  "JSON.parse(require('fs').readFileSync('public/apps/liltag-app.json'))"`).
- [ ] **Step 2:** Rewrite the policy entries: Section 1 — the Affonso script and its 30-day
  cookie run on econumo.com only, for visitors arriving through a partner link. Section 3
  *Affiliate Program* — for an account created within 30 days of such a visit, we share an
  internal account identifier, and later the pre-tax amount of its purchases and any refunds,
  with Affonso so it can credit the partner; no email address; legitimate interest; link to
  Affonso's policy. Section 4 cookie note unchanged in substance. Effective date = merge day.
- [ ] **Step 3:** `npm run build` passes.
- [ ] **Step 4:** Commit `feat: Keep the Affonso pixel on econumo.com only`, push, update the PR
  body (drop the XHR hook sections; say signups/payments are reported server-side by billing).

## Rollout (after all PRs merge)

Set on production: app `ECONUMO_AFFILIATES=affonso:affonso_referral`; billing `AFFONSO_API_KEY`.
Test environment sets neither.
