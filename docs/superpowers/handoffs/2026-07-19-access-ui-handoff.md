# Handoff: making access state visible in the Econumo SPA

**For:** a new session doing the frontend work for the access/trial feature ("Plan C").
**Repository:** `econumo/econumo`, work in `web/`.
**Date:** 2026-07-19
**Status:** backend shipped and inert. No frontend work started.

---

## 1. Where things stand

The access model was merged to `main` in `8ab5fa8` (PR #119) and is **switched off**: `ECONUMO_TRIAL=none`, every user has `access_level = full` and `access_until = NULL`. Nothing changed for anyone.

The backend can already:

- grant a trial at registration (until the start of the month after next) when `ECONUMO_TRIAL=end-of-next-month`
- return `accessLevel` and `accessUntil` on `CurrentUserResult` (login, `get-user-data`) and on each `ConnectionResult`
- reject writes from a restricted caller with **HTTP 402**

**The SPA does none of it.** `grep -rn "accessLevel\|402" web/src/` returns nothing. Today a restricted user would hit a wall with no banner, no explanation, and no way to pay.

**This is the blocker for launching the trial.** Do not set `ECONUMO_TRIAL=end-of-next-month` in production until this work ships.

Authoritative design: `docs/superpowers/specs/2026-07-19-cloud-monetization-trial-access-design.md`. Read §"What the SPA sees". Two claims in it are outdated — see §5 below.

---

## 2. What to build

### 2.1 Consume the new fields

`accessLevel` is `"full"` or `"readonly"`. `accessUntil` is `"2006-01-02 15:04:05"` or the **empty string** when there is no expiry — not `null`, not absent. Both arrive on the current user and on every connection.

Derive one client-side state, because the raw pair is not what the UI needs:

| state | condition |
|---|---|
| `trial` | `accessLevel === 'full'` and `accessUntil !== ''` |
| `full_access` | `accessLevel === 'full'` and `accessUntil === ''` |
| `readonly` | `accessLevel === 'readonly'` |

Vocabulary rule, enforced throughout this codebase: **identifiers describe access, never money.** `full_access`, never `paid`. The same binary serves self-hosted instances where nobody pays for anything.

### 2.2 The banner

Shown when `accessUntil` is within 3 days, or when the state is `readonly`. Generic copy ("access ends in N days" / "read-only"), with a CTA to the portal.

### 2.3 Settings entry

Label it **"Access"**, not "Subscription". Core is a one-off purchase; the word "Subscription" promises recurring charges that do not exist and will cost conversion at the exact moment it matters.

### 2.4 Per-connection CTA

On the connections page, show "Pay for <name>" when that connection's `accessLevel` is read-only or its `accessUntil` is near. This is the pricing model — *I pay for myself; if I see the value for my partner, I pay for them too* — and it is the reason `ConnectionResult` carries the fields at all.

### 2.5 Handle 402

A 402 response means the user is read-only. Flip the client into read-only, show the banner, and surface a toast. Hide the primary create action (the FAB) while read-only so a restricted user does not walk into a wall.

**Do not hide every per-screen write button** across ten features. The persistent banner carries the message; that churn is not worth it.

### 2.6 Remove the old paywall

The previous funnel hid registration behind a paywall. That is gone — registration is open to everyone now. Delete:

- `PAYWALL_ENABLED` in `web/public/econumo-config.js:6` and `web/src/lib/config.ts:12,141-148`
- `paywallUrl` / `isPaywallEnabled` in `web/src/lib/package.ts:8,14,19-20` (currently hardcodes `https://pay.econumo.com/cloud/`)
- the paywall block in `web/src/features/auth/RegistrationPage.tsx:83` and its i18n keys (`auth.page.sign_up.paywall.*`)
- the corresponding assertions in `RegistrationPage.test.tsx`

### 2.7 `BILLING_URL`

New runtime config in `econumo-config.js`, empty by default. **Empty means no banner, no settings entry, no per-connection CTA, no link** — a self-hosted build shows no trace of billing. Follow how the existing keys are read in `web/src/lib/config.ts`.

---

## 3. The dependency you need to know about

The billing link is **not** assembled by the SPA. It calls a backend endpoint that mints a signed, short-lived handoff token:

```
POST /api/v1/user/create-billing-link   →   {"url": "…"}
```

with an optional `for` (a user id) to preselect a partner.

**That endpoint does not exist yet.** It is part of "Plan B" along with the admin listener, and has not been implemented. Consequences:

- §2.1, §2.2 (banner), §2.5 (402 handling), §2.6 (cleanup) and the analytics in §4 can all ship **now**.
- §2.3 and §2.4 (anything that links out to the portal) need Plan B first.

Split the work accordingly rather than blocking the whole thing. A banner that explains the situation is worth shipping even before there is somewhere to pay.

Why a per-click endpoint rather than a field on the user: the token lives 10 minutes and `get-user-data` is cached by TanStack Query, so a user opening Settings half an hour after login would otherwise arrive at the portal with an expired assertion.

---

## 4. Analytics — read this, the spec is wrong about it

The design spec says eight `METRICS` keys need adding and describes registering `accessState` as a PostHog "super property". Both statements are inaccurate against the code that actually exists.

### 4.1 Most of the catalogue already exists

`web/src/lib/metrics.ts` already defines ~80 keys including `USER_REGISTRATION`, `ACCOUNT_CREATE`, `TRANSACTION_CREATE`, `TRANSACTION_IMPORT`, the whole budget/category/connection surface, and the UI modal events.

An earlier analysis in this project claimed "there is no registration event". That was reading PostHog's *received* event taxonomy — which showed only four custom events in its first week — not the catalogue. The instrumentation is broadly there; it simply had not fired much.

**Genuinely new keys for this feature:**

| key | fired when |
|---|---|
| `appAccessBannerShow` | the trial/read-only banner renders |
| `appAccessCtaClick` | the portal CTA is clicked |
| `appAccessPartnerCtaClick` | the "pay for partner" CTA is clicked |
| `appAccessReadonlyBlocked` | a 402 is received |

Names are `app`-prefixed camelCase and frozen once shipped; the PostHog snake_case name derives automatically. `web/src/lib/metrics-coverage.test.ts` fails the suite if a key is never fired, so wire each one at its action's success point as you add it.

### 4.2 There are no super properties — and no person profiles at all

`web/src/lib/analytics.ts` is a hand-rolled transport, not `posthog-js`. It is **anonymous by construction**:

```ts
const distinctId = crypto.randomUUID()          // per page load, never persisted
properties: { ...properties, $process_person_profile: false }
```

So there is no SDK to register a super property with, and no person profile to set properties on. To attach `accessState` to every event, add it inside `capture()` from a module-level variable updated on login and whenever `get-user-data` returns. That gives per-event accuracy, which is what you want — the whole point is to see the moment the state changes.

### 4.3 The limitation worth escalating before you build funnels

Because `distinct_id` is random per page load and person profiles are disabled, **events cannot be linked to a person across sessions**. You can count how many registrations happened and how many 402s happened; you cannot ask "what fraction of trial users who created a transaction went on to pay", because there is no stable identity joining those events.

That is a deliberate privacy stance (see `docs/superpowers/specs/2026-07-17-posthog-analytics-design.md`), not an oversight. But it means the trial-length hypothesis this whole feature rests on — *value arrives at the first closed month* — **cannot be validated with the current analytics**.

Someone has to decide whether that stance changes for cloud. Do not quietly work around it; raise it.

---

## 5. Where the spec is out of date

Two claims in `2026-07-19-cloud-monetization-trial-access-design.md` no longer hold:

1. It lists eight new `METRICS` keys. Only four are new (§4.1).
2. It describes `accessState` as a PostHog super property. There is no SDK and no person profile; implement it inside `capture()` (§4.2).

Everything else in that document is accurate and was verified against the merged implementation.

---

## 6. Frozen contracts — do not "clean these up"

- **`accessUntil` is `""` for NULL**, and datetimes are `"2006-01-02 15:04:05"` (space separator, no zone, UTC).
- **The 402 envelope is byte-frozen:**
  ```json
  {"success":false,"message":"Read-only access. Write operations are disabled.","code":402,"errors":{}}
  ```
- **Render your own copy for 402.** The server message is deliberately product-neutral. There is currently **no `messageCode`** on the 402 (unlike the sibling 429), so no `locales/errors.*` key exists and `apiErrorMessage` cannot translate it. Adding one is additive and golden-safe — tracked in issue #120, worth doing alongside this work if you want the message localized.
- **i18n:** all strings go in `locales/{en,ru}.json` under the feature's namespace; both catalogues must stay key- and placeholder-identical or `internal/test/i18ntest` fails the Go suite.

---

## 7. Gates before you claim done

```bash
make web-test     # vitest
make web-lint     # oxlint
cd web && pnpm exec tsc -b     # NEITHER of the above type-checks
make go-test      # i18n parity guards live in the Go suite
```

`pnpm exec tsc -b` is not optional. Neither vitest nor oxlint type-checks this project, and a `TS2322` has previously shipped past both gates.

If you touch anything the API returns, regenerate goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and **inspect the diff** — a golden change means observable behavior changed.

---

## 8. Reference

- `docs/superpowers/specs/2026-07-19-cloud-monetization-trial-access-design.md` — the design (with §5's caveats)
- `docs/superpowers/plans/2026-07-19-access-model-and-enforcement.md` — what the backend does
- `docs/superpowers/specs/2026-07-17-posthog-analytics-design.md` — why analytics is anonymous
- `docs/superpowers/handoffs/2026-07-19-billing-portal-handoff.md` — the portal side, including the handoff-token scheme
- `CLAUDE.md` — conventions, the frozen wire contract, the product-analytics rule
- Issue #120 — deferred follow-ups, including the missing 402 `messageCode`
