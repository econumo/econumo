# Access-state UI ("Plan C") — design

**Date:** 2026-07-20
**Scope:** `web/` only (plus `locales/{en,ru}.json`). No Go API changes.
**Prerequisites:** all merged — the access model (PR #119) and the admin API /
billing handoff (PR #122). `POST /api/v1/user/create-billing-link` exists and
`BILLING_URL` is server-merged into the served `econumo-config.js`.
**Supersedes/complements:** the SPA sections of
`2026-07-19-cloud-monetization-trial-access-design.md` and the handoff
`2026-07-19-access-ui-handoff.md` (whose "Plan B not built" claim is now stale).

## Goal

Make access state visible in the SPA so the trial can be switched on: a user
whose trial is ending sees a banner and can open the billing portal and pay;
a read-only user understands why writes fail and how to restore access. This
is the launch blocker for `ECONUMO_TRIAL=end-of-next-month` in production.

## Decisions made during brainstorming

- **Scope: everything in one branch** — banner, Settings entry, per-connection
  CTA, 402 handling, paywall removal, analytics.
- **Trial banner is dismissible per session** (component state in the
  persistent layout — reappears on next page load/login). The read-only banner
  is never dismissible.
- **Analytics stays anonymous.** The four new events are aggregate-only;
  conversion truth (trial → paid, per user) comes from the billing portal's
  own database, not PostHog. No stable `distinct_id` is introduced.
- **State architecture: TanStack Query is the single source of truth** plus a
  thin derivation hook. No Zustand access store — the 402 interceptor
  invalidates the user query instead of holding state, so the UI can never
  disagree with the server for longer than one refetch.
- **Self-hosted (`BILLING_URL` empty) shows no trial countdown.** The
  read-only banner still renders (it explains why writes fail) but without a
  pay CTA. No Settings "Access" group, no partner CTA, no billing link
  anywhere.
- **One "Billing" group on the main settings page, for everyone.** Rendered
  whenever `billingEnabled`, regardless of state — a trial user can upgrade
  from day one without waiting for the 3-day banner, and a paying customer
  always has a path back to the portal (receipts, paying for a partner). The
  label is "Billing" by explicit product decision, deviating from the
  handoff's "Access" suggestion; the original concern was "Subscription"
  promising recurring charges, which "Billing" does not.
- **The 402 `messageCode` backend addition (issue #120) stays out of scope** —
  the SPA renders its own localized copy, so a `messageCode` would only
  benefit non-SPA clients.

## Wire contract consumed (frozen — do not "clean up")

- `CurrentUserResult` (login, `get-user-data`) and each `ConnectionResult`
  (top level, sibling of `user`) carry `accessLevel: "full" | "readonly"` and
  `accessUntil`.
- `accessUntil` is `"2006-01-02 15:04:05"` (space separator, UTC, no zone, no
  fractional seconds) or the **empty string** for no expiry — not null, not
  absent.
- The 402 envelope is byte-frozen and carries no `messageCode`:
  `{"success":false,"message":"Read-only access. Write operations are disabled.","code":402,"errors":{}}`.
- Vocabulary rule: identifiers describe **access, never money** —
  `full_access`, never `paid`.

## 1. Access-state layer

### DTOs (`web/src/api/dto/`)

- `CurrentUserDto` (`user.ts`): add `accessLevel: 'full' | 'readonly'` and
  `accessUntil: string`.
- `ConnectionDto` (`connection.ts`): add the same two fields at the top level
  (siblings of `user`), matching `internal/model/connection_dto.go`.

### Derivation (`web/src/lib/access.ts`, new)

Pure functions, unit-testable without React:

- `deriveAccessState(level, until)` →
  `'trial' | 'full_access' | 'readonly'`:

  | state | condition |
  |---|---|
  | `trial` | `level === 'full'` and `until !== ''` |
  | `full_access` | `level === 'full'` and `until === ''` |
  | `readonly` | `level === 'readonly'` |

  Note the server already reports an elapsed `accessUntil` as
  `accessLevel: "readonly"` (effective level is computed per request), so the
  client does not need its own expiry comparison for the state itself.
- `accessDaysLeft(until, now)` → whole days until expiry (ceiling), parsing
  the frozen UTC datetime format. Banner threshold: `daysLeft <= 3`.

### Hook (`useAccessState()` in `web/src/features/user/queries.ts`)

Wraps `useUserData()`; returns
`{ state, accessUntil, daysLeft, billingEnabled }`.
`billingEnabled = getBillingUrl() !== ''` is the master switch for every
billing surface.

### Config (`web/src/lib/config.ts`, `web/public/econumo-config.js`)

- `getBillingUrl(): string` following the `getWebsiteUrl()` pattern; add
  `BILLING_URL` to the `EconumoConfig` interface.
- `BILLING_URL: ''` in the dist `econumo-config.js` (the server overwrites it
  unconditionally — server truth, already implemented in
  `internal/web/router/router.go`).

### Billing link (`web/src/api/user.ts` + one shared hook)

- `createBillingLink(forUserId?: string)` → POST
  `/api/v1/user/create-billing-link` with body `{"for": "<id>"}` (or `{}`),
  returns `{url}`.
- `useOpenBillingPortal(forUserId?)` — the single implementation of
  mint-then-open: call the endpoint **per click** (the handoff token lives 10
  minutes; pre-fetching is wrong by design), `window.open(url, '_blank')`,
  fire the appropriate CTA metric, surface failures via `apiErrorMessage`
  toast. All CTAs (banner, Settings, connections) use this hook.

### Analytics plumbing

`metrics.ts` gains a module-level `accessState` variable plus an exported
setter; `capture()`'s PostHog properties gain `access_state`. The setter is
called where user data lands (`getUserData()` and the login response
handling), so it is fresh on every fetch without touching React. The
anonymous transport is unchanged (random per-page-load `distinct_id`, no
person profiles).

## 2. Surfaces

### Banner (`AccessBanner`, new component)

Mounted once in `ApplicationLayout`, inside `<main>` above `<Outlet/>` — spans
every authenticated page, survives navigation. Variants from
`useAccessState()`:

- **Trial ending** — shown when `state === 'trial'` && `daysLeft <= 3` &&
  `billingEnabled`. Styling follows the update-card pattern
  (`bg-primary/10 text-primary`). Copy: "Your access ends in {N} days"
  (pipe-plural via `pluralPick`), a "Manage access" button
  (`useOpenBillingPortal()`), and an × dismissing it for the session
  (component state in the persistent layout).
- **Read-only** — shown when `state === 'readonly'`, regardless of billing.
  Not dismissible; muted/destructive styling. Copy: "Your account is
  read-only — you can view and export, but not add or change data." The
  "Manage access" button renders only when `billingEnabled`.

`METRICS.ACCESS_BANNER_SHOW` fires once per mount **per variant** (effect
keyed on the variant, so trial → readonly fires again).

### Settings (`SettingsPage.tsx`)

No sub-page, no profile changes. A single new `MenuGroup` labeled
**"Billing"** on the main settings page, rendered whenever `billingEnabled`
(all states — this is every customer's standing path to the portal), placed
after the existing groups. It contains one `MenuRow`-style action, "Open
billing portal", that mints the link per click (`useOpenBillingPortal()`,
firing `METRICS.ACCESS_CTA_CLICK`). For `trial` and `readonly` states the row
carries a status hint ("Trial — access ends {date}" / "Read-only") so a trial
user sees the upgrade path from day one; for `full_access` it is just the
plain row. Copy keys shared with the banner where possible. Self-hosted
(`BILLING_URL` empty): the group does not exist.

### Connections (`ConnectionsPage.tsx`)

Per row:

- a subtle status line when the connection is not `full_access`
  ("read-only" / "access ends in {N} days") — rendered regardless of billing,
  informational;
- a **"Pay for {name}"** button when `billingEnabled` and the connection's
  state is `readonly` or within the 3-day window. Click →
  `useOpenBillingPortal(connection.user.id)` (the backend's `for`
  preselection hint), firing `METRICS.ACCESS_PARTNER_CTA_CLICK`.

### 402 handling (`web/src/api/client.ts`)

In the axios response interceptor, beside the existing 401 branch,
`status === 402`:

1. fire `METRICS.ACCESS_READONLY_BLOCKED`;
2. show a sonner toast with client-rendered localized copy and a **fixed
   toast id** so repeated 402s do not stack;
3. `queryClient.invalidateQueries({ queryKey: queryKeys.user })` — the
   server's own answer flips the UI to read-only within one refetch;
4. reject the promise as before, so per-call-site error handling keeps
   working.

Enabler refactor: `queryClient` moves from a local in `main.tsx` to an
exported module `web/src/app/queryClient.ts` (created via the existing
`createAppQueryClient()`), imported by `main.tsx` and the interceptor.

### Primary create action

Only the primary action hides while read-only: `AccountPage.tsx`'s
"Add transaction" footer button and toolbar add button render `null` when
`state === 'readonly'`. Deliberately **no sweep** across other features'
write buttons — the persistent banner carries the message (handoff §2.5).

## 3. Paywall removal

Registration is open; the old funnel dies:

- `PAYWALL_ENABLED` from `web/public/econumo-config.js`; `isPaywallEnabled()`
  from `config.ts` and its `config.test.ts` case.
- `paywallUrl` / `isPaywallEnabled` from `web/src/lib/package.ts` (the
  hardcoded `https://pay.econumo.com/cloud/` dies here).
- The paywall early-return block in
  `web/src/features/auth/RegistrationPage.tsx` (~lines 79–93) and its test in
  `RegistrationPage.test.tsx` (~lines 60–64).
- `LoginLayout.tsx` simplifies to `registerEnabled = isRegistrationAllowed()`.
- The four `auth.page.sign_up.paywall.*` keys from **both** `locales/en.json`
  and `locales/ru.json` (parity guard enforces both-or-neither).

## 4. Analytics events

Four new `METRICS` keys (frozen once shipped; PostHog snake_case derives
automatically), each with a real call site — `metrics-coverage.test.ts`
enforces this:

| key | name | fires |
|---|---|---|
| `ACCESS_BANNER_SHOW` | `appAccessBannerShow` | banner mount, per variant |
| `ACCESS_CTA_CLICK` | `appAccessCtaClick` | self portal-open click (banner CTA or Settings billing row) |
| `ACCESS_PARTNER_CTA_CLICK` | `appAccessPartnerCtaClick` | "Pay for {name}" click |
| `ACCESS_READONLY_BLOCKED` | `appAccessReadonlyBlocked` | 402 received in the interceptor |

Plus `access_state` on every `capture()` (see §1). Known, accepted
limitation: without a stable identity, per-user conversion funnels are not
measurable in PostHog; the portal's database is the source of conversion
truth.

## 5. i18n

New top-level `access` namespace in `locales/{en,ru}.json`: banner copy for
both variants, the days-left pipe-plural (`en` two variants; `ru` three:
`one | few | many`), Settings billing copy ("Billing", "Open billing
portal", status hints), connection status/"Pay for {name}" copy, and the 402
toast text.
Key- and placeholder-parity across catalogues is enforced by
`internal/test/i18ntest` inside `make go-test`.

## 6. Testing

- **Unit (`lib/access.test.ts`):** the derivation table; day math around
  month/DST boundaries; the `""`-means-no-expiry contract; threshold edges
  (3 days, 0 days, past).
- **Component:** banner variant matrix (state × billingEnabled) + per-session
  dismissal + metric firing; Settings Billing group visibility (billing
  on/off) and status hints per state;
  connections CTA/status visibility matrix; 402 interceptor (toast, user-query
  invalidation, metric, promise still rejects); rewritten RegistrationPage and
  config tests after paywall removal.
- **Gates:** `make web-test`, `make web-lint`,
  `cd web && pnpm exec tsc -b` (vitest and oxlint do **not** type-check),
  `make go-test` (i18n parity guards). No Go API changes, so no golden
  regeneration is expected; if one shows up, something is wrong — inspect it.

## Out of scope

- The 402 `messageCode` catalogue code (issue #120) — additive backend change,
  only benefits non-SPA clients.
- Any identity/`distinct_id` change to analytics.
- Hiding write buttons beyond AccountPage's add-transaction actions.
- Everything portal-side (Stripe, reminder email, webhooks) — separate
  private repository, see the billing-portal handoff.
