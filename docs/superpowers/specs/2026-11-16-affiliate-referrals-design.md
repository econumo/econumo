# Affiliate referrals (Affonso, reported by billing)

Date: 2026-11-16. Repos: `econumo/website`, `econumo/econumo` (this one), `econumo/billing`.

## Intent

Run a partner program on Affonso: credit a partner for the sign-ups and payments they send.
Econumo Cloud only. The app (app.econumo.com) stays free of third-party scripts, and the
app's code does not name the vendor: billing is the only part that knows about Affonso, the
same way it is the only part that knows about money.

## Goals

- A visitor who arrives at econumo.com through a partner link and then creates an account
  (password OR Google/Apple/OIDC) is attributed to that partner.
- Payments and refunds of that user's purchases are reported to Affonso so commissions follow.
- No email address goes to Affonso: the user is identified by the Econumo user id.

## Non-goals

- Tracking partner links that point at app.econumo.com directly. Partners link to
  econumo.com.
- Attributing existing users who click a partner link later: only accounts created while the
  referral is present count.
- Self-hosted instances: they never see the cookie, and without a billing URL nothing is
  captured.
- Any UI in the app.

## Anti-goals

- Any Affonso script, request or vendor name in app.econumo.com or in `econumo/econumo`.
- A sign-up or a billing page failing, or slowing down noticeably, because of referral
  handling.
- Billing requiring Affonso: a test environment runs without it and reports nothing.

## How it works

```
econumo.com?via=partner ──pixel──▶ Affonso creates a click, cookie affonso_referral=<click id>
                                   on the root domain (.econumo.com), SameSite=None; Secure
app.econumo.com sign-up ──────────▶ the app reads the cookie, stores user option `affiliate`
user opens billing ───────────────▶ billing reads `affiliate` from /admin/user-context
                                   ──▶ POST /v1/signups {click_id, external_user_id}
user pays / is refunded ──────────▶ billing ──▶ POST /v1/conversions / .../refund
```

Verified in the pixel source (`cdn.affonso.io/js/pixel.min.js`): the cookie value is the
click id (`affonso_referral = e.cid`) and it is written for the registrable domain, so every
request to app.econumo.com carries it, including Apple's cross-site `form_post` callback
(SameSite=None).

### 1. Website (`econumo/website`, PR #92)

- Keep the econumo.com pixel snippet in `src/layouts/Layout.astro` as it is.
- Drop both LilTag tags (`affonso`, `affonso-signup`) from `public/apps/liltag-app.json`.
- Privacy policy: Affonso's script and 30-day cookie run on econumo.com only; when a referred
  user opens billing or pays, we share an internal account identifier and the purchase amount
  (and refunds) with Affonso. No email. Legal basis unchanged (legitimate interest).

### 2. App (`econumo/econumo`)

- **Config**: `ECONUMO_REFERRAL_COOKIE` (cookie name, e.g. `affonso_referral`). Capture runs
  only when BOTH it and `ECONUMO_BILLING_URL` are set. Documented in `docs/full.env.example`.
- **Capture**: at account creation only, on both paths:
  - `POST /api/v1/user/register-user`: the cookie on that request.
  - OAuth callback when it provisions a new user (`internal/oauth`): the cookie on the
    callback request. A sign-in into an existing account or a link from Settings records
    nothing.
  The value is validated (`^[A-Za-z0-9_-]{1,64}$`); anything else is dropped silently. A
  missing, malformed or unreadable cookie never fails or changes the sign-up.
- **Storage**: user option `affiliate` (`model.OptionAffiliate`), written with the new user in
  the same transaction, write-once (never overwritten or cleared by any flow).
- **Exposure**: only through the private admin listener. `GET /admin/user-context` adds an
  additive `affiliate` field to the user view (`""` when none; connections carry it
  too, unused). It is filtered out of `get-user-data` and `get-option-list`, which today
  return every stored option, and not exposed on `/mcp`.
- The name is deliberately not `referral`: the parked `feature/referral-program` branch uses
  that for a different thing (the share report a user signed up from).
- No HTTP client, no outbound call, no retries in the app.

### 3. Billing (`econumo/billing`)

- **Config**: optional group `AFFONSO_API_KEY` (+ `AFFONSO_API_URL`, default
  `https://api.affonso.io/v1`, for tests). Unset = Affonso off: billing ignores `referral`
  entirely, stores and sends nothing.
- **Reading**: billing already calls `GET /admin/user-context` server to server with
  `ECONUMO_ADMIN_URL` + `ECONUMO_ADMIN_TOKEN` (`internal/econumo/http.go`) every time the
  portal loads a user (`Service.Context`, `CreateCheckout`); the short-lived handoff token
  only tells billing WHICH user. `internal/econumo` parses the new `affiliate` field.
- **Recording the sign-up**: whenever billing loads the user's context (opening the portal,
  starting a checkout) and the user has a referral not yet recorded, it upserts a row in a new
  table `affiliate_referrals (user_id PK, click_id, signup_status, signup_attempts,
  next_attempt_at, signup_reported_at, created_at)`. The request itself never calls Affonso.
- **Outbox worker** (same pattern as `internal/grants`): reports due sign-ups with
  `POST /v1/signups {click_id, external_user_id: <econumo user id>}` (idempotent on
  `click_id`; a 200 repeat is success; 404 `CLICK_NOT_FOUND` is terminal and logged).
- **Payments**: when a purchase is marked paid and its PAYER has an `affiliate_referrals`
  row, enqueue `POST /v1/conversions {external_user_id, sale_amount, sale_amount_currency,
  external_event_id: <purchase id>}` with `sale_amount` = `purchases.amount_cents` / 100
  (the pre-tax amount, see below) and its currency; store the returned conversion id. A purchase
  with amount 0 (100% discount) is not reported. The worker sends a
  user's sign-up before any of their conversions.
- **Refunds**: when a referred payer's purchase is refunded, enqueue
  `POST /v1/conversions/{conversion id}/refund` (full refund, `external_event_id:
  refund-<purchase id>`), matching billing's rule that any refund revokes access.
- Failures retry with backoff and give up after a fixed number of attempts with an ERROR log;
  they never affect a checkout, a webhook acknowledgement or a grant.
- If a sign-up fails for good, its conversions and refunds are dropped with it (Affonso
  rejects identifiers that resolve to no referral). A refund whose conversion is not yet sent
  waits behind it; a refund whose conversion was dropped is dropped.
- No backfill: purchases made before this ships are not reported.

## Pre-tax amounts (decided)

Commission is on the amount before tax. Billing records it when a purchase is paid, in a new
nullable column `purchases.amount_net_cents` (migration), filled by both webhook handlers:
Stripe from the session's total minus its tax, Paddle from the transaction totals' subtotal
after discount, before tax. A conversion is sent with `amount_net_cents`; a purchase paid
before the column existed has NULL and is not reported (there is no backfill anyway).

## Consequence of reporting at billing-open (accepted)

Affonso learns of a sign-up only when the user first opens billing (or pays), not at
registration. A referred user who never opens billing never appears as a lead. Payments and
refunds are unaffected. Fine while commissions are paid on payments; revisit if lead-only
commissions are wanted.

## Approaches considered

- **Browser signup event in the app + pixel in the app** (`Affonso.signup(email)`): rejected;
  puts a third-party script in the app and sends emails to Affonso.
- **App pushes each sign-up to billing**: rejected; the first app-to-billing call, needing a
  client, a contract and retries.
- **Billing polls a referred-users list**: workable, reports leads within a minute, but adds a
  list endpoint and cursor.
- **Chosen: user option, passed in `/admin/user-context` when the user opens billing**:
  smallest app change, one call direction (billing to app) as today. Picked by Dmitry.
- Affonso's native Stripe/Paddle integrations could report payments and refunds instead of
  billing; not used, to keep billing the single reporter and the integration optional.

## Rules touched

- Env vars: `ECONUMO_REFERRAL_COOKIE` in `docs/full.env.example` (not `.env.example`).
- No user-observable change in the app, so no regression-plan, analytics or i18n items.
- Admin user-context response gains a field; update its tests (and goldens if covered).

## Left to the builder

- Exact table/column names and worker cadence in billing (mirror `grants`).
- Where in the OAuth provisioning path the cookie is read (the callback handler has the
  request; the parked `feature/referral-program` branch shows the provisioning seam, but no
  OAuth-state column is needed since the cookie is on the callback itself).
