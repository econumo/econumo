# Handoff: the Econumo billing portal

**For:** a new session starting the billing portal as a **separate project** (private repository).
**Date:** 2026-07-19
**Status:** nothing built yet. Econumo's side is partially built — read "What already exists" carefully.

---

## 1. What this project is

A small web service that sells access to Econumo Cloud and tells Econumo who may use it.

The split is deliberate and load-bearing:

- **Econumo (the product)** knows *what level of access a user has and until when*. It does not know what a plan is, what a trial is, or that Stripe exists.
- **The portal (this project)** owns everything about money: Stripe checkout and webhooks, prices, promo codes, its own database, its own cron, and all billing email.

Econumo does not know the portal's URL, and the portal never touches Econumo's database. They meet at exactly one interface, described in §4.

The reason for this split: Econumo ships as a self-hostable open-source binary. It must not acquire a paywall, a trial, or any notion of money. A single "cloud" edition is shipped from the same source — the difference is configuration, not code.

---

## 2. Product model (decisions already made)

| concept | how Econumo stores it |
|---|---|
| trial | `access_level = full`, `access_until = start of the month after next` |
| paid core | `access_level = full`, `access_until = NULL` (currently lifetime) |
| lapsed | `access_until` in the past → effectively read-only |
| manual restriction | `access_level = readonly` |
| self-hosted | `access_level = full`, `access_until = NULL` |
| banned | `is_active = false` — a separate dimension, not your concern |

**Core is a one-off purchase.** The trial runs until the end of the *next* calendar month (29–62 days) because the product's moment of value is a closed budget month — plan against actual. A fixed 7- or 30-day trial delivers that to nobody who registers early in a month.

**Term-based pricing needs zero Econumo changes.** If core later becomes a 2-year or 5-year purchase, the portal simply writes a different `until`. This was explicitly considered and deferred; the current recommendation is to keep core one-off and build recurring revenue on future AI features instead, which have genuine per-use cost (tokens). Do not build term logic into the portal until that decision is actually made.

**Pay-for-partner.** The pricing model is "I pay for myself; if I see the value for my partner, I pay for them too." Econumo implements none of this — enforcement is per caller. The portal charges one Stripe customer and issues two independent `set-access` calls.

**Two axes, never one ladder.** `access_level` is binary (`full` / `readonly`) and must never grow tiers (`pro`, `premium`). Future per-feature entitlements (e.g. AI) are a separate axis, a separate table, and a separate endpoint. A user with lifetime core and a cancelled AI subscription is not "below" a regular user — they are `core, no ai`.

---

## 3. What already exists on the Econumo side

Merged to `main` as commit `8ab5fa8` (PR #119). **Read `docs/superpowers/specs/2026-07-19-cloud-monetization-trial-access-design.md` in the econumo repo — it is the authoritative design.**

Shipped and working:

- `users.access_level` (`TEXT NOT NULL DEFAULT 'full'`) and `users.access_until` (nullable timestamp), on both SQLite and PostgreSQL.
- Effective level is a pure function of `(access_level, access_until, now)` evaluated on every request. There is no background expiry job — an elapsed `access_until` *is* read-only, so nothing can fail to run.
- `ECONUMO_TRIAL` = `none` (default) | `end-of-next-month`. Currently `none` in production; the trial is not switched on yet.
- Enforcement: `POST` + not-full + path off a five-entry allowlist → **HTTP 402**, envelope:
  ```json
  {"success":false,"message":"Read-only access. Write operations are disabled.","code":402,"errors":{}}
  ```
- `accessLevel` / `accessUntil` on `CurrentUserResult` (login, get-user-data) and on `ConnectionResult` (so the connections page can show a lapsed partner).
- CLI: `user:set-access <email> <full|readonly> [YYYY-MM-DD]` and `user:show <email>`.

**NOT built — this is your blocker.** Everything in §4 below (the admin listener, the three endpoints, the handoff token, and `POST /api/v1/user/create-billing-link`) is "Plan B" and **has not been implemented**. The portal cannot integrate until someone builds it in the econumo repo. Plan B is fully specified in the design doc; it needs its own implementation plan and PR.

Decide early whether you build Plan B first in econumo, or develop the portal against a stub.

---

## 4. The contract with Econumo

### 4.1 The admin listener

A **second** `http.Server` inside the Econumo binary, started by `serve` **only when both** `ECONUMO_ADMIN_PORT` and `ECONUMO_ADMIN_TOKEN` are set. Self-hosted instances set neither, so the routes do not exist at all. The port is never published to the internet — the portal reaches it over the internal network.

This is fail-safe by construction: a misconfigured reverse proxy cannot expose a route that is not on the public mux.

Auth: `Authorization: Bearer <ECONUMO_ADMIN_TOKEN>`, compared in constant time. Responses use Econumo's standard envelope.

| endpoint | body / query | effect |
|---|---|---|
| `POST /admin/set-access` | `{"userId": "…"}` **or** `{"email": "…"}`, plus `"level": "full\|readonly"` and `"until": "2027-01-01 00:00:00" \| null` | sets the two columns |
| `GET /admin/expiring-users?days=3` | — | `[{"id","email","name","accessUntil"}]` — who to email |
| `GET /admin/user-context?userId=…` | — | `{user: {id,name,email,accessLevel,accessUntil}, connections: [{id,name,email,accessLevel,accessUntil}, …]}` |

Supplying both `userId` and `email`, or neither, is a validation error. Use `email` for self-purchases (you have the Stripe customer's address) and `userId` when one user pays for another.

`expiring-users` is how you know whom to email; you decide what to send and remember what you already sent, in **your** database. Econumo sends no billing mail and runs no billing cron.

### 4.2 The handoff token — how you know who the visitor is

**Do not trust anything in the URL query except a signed assertion.** Anything the SPA puts in a link is forgeable by whoever operates the browser. A bare `uid` was the original design and was rejected precisely because the portal now *displays* status, and a forged id would expose another person's payment state.

Econumo mints a short-lived token:

```
payload = {uid, exp}                     // exp = now + 10 minutes
sig     = HMAC-SHA256("billing-handoff:v1" || payload, ECONUMO_ADMIN_TOKEN)
link    = ${BILLING_URL}?t=<base64url(payload)>.<base64url(sig)>[&for=<user id>]
```

**No new secret.** You already hold `ECONUMO_ADMIN_TOKEN` to authenticate against the admin listener. The `billing-handoff:v1` prefix gives domain separation so a handoff signature cannot be replayed as anything else.

Verification on your side: recompute the HMAC, compare in constant time, reject if `exp` has passed. Then call `GET /admin/user-context?userId=<uid>` to fetch names, emails and access states server-side. **No personal data ever travels through the browser.**

Hygiene: the token is read-only, lives 10 minutes, and you should exchange it for your own session and strip it from the URL so it does not persist in history or `Referer`.

### 4.3 `for` is a preselection hint, nothing more

It authorizes nothing — you already know the visitor from the signature and already have their full context.

| link | show |
|---|---|
| `?t=…` | self preselected |
| `?t=…&for=<own id>` | identical — a legal value, not a special case |
| `?t=…&for=<partner id>` | that partner preselected |

Validate `for` against the `user-context` you fetched (self or one of the connections); anything else is ignored and falls back to self. A forged `for` at worst pays for someone the user is already connected to.

### 4.4 Stripe specifics

- **The checkout email must be non-editable and preloaded** from the account. If a user pays from a different address, the payment cannot be matched to the account.
- When paying *for a partner*, the Stripe customer is the **payer**; the beneficiary is identified by `userId` in the `set-access` call. The payer's own email goes on the checkout.
- Webhook → verify → `POST /admin/set-access`. Be idempotent: Stripe retries.

---

## 5. What to build (suggested first cut)

1. **Verify handoff + fetch context** — the entry point. Everything else depends on knowing who arrived.
2. **A page listing the visitor and their connections**, each with access state and a "pay for this person" action.
3. **Stripe Checkout** with a non-editable prefilled email, and a webhook that calls `set-access` idempotently.
4. **Your own database**: Stripe objects, which reminder emails were sent, payment→beneficiary mapping.
5. **A cron** polling `GET /admin/expiring-users?days=N` and sending reminders.

Keep the portal's own notion of "plan" internal. Econumo must keep receiving nothing but `(level, until)`.

---

## 6. Open questions not yet decided

- **Price and currency.** No paying users exist in the new funnel yet; there is no willingness-to-pay data.
- **Whether core stays lifetime.** See §2 — deferred deliberately, and changing it costs nothing on Econumo's side.
- **Grandfathering.** The migration set every pre-existing user to `access_until = NULL` (permanent). If core ever becomes term-based, those users stay permanent forever. That is fine, but it means two classes exist.
- **Trial abuse.** A new email buys a new trial. Accepted; address it from the Stripe side if it becomes real.
- **Refunds and chargebacks** — no policy defined. A chargeback presumably means `set-access readonly`.

---

## 7. Things that will bite you

- **`ECONUMO_ADMIN_TOKEN` serves two roles**: bearer credential and HMAC key. Domain separation handles it, but if you ever need to rotate one without the other, you must split the secrets on both sides at once.
- **The 402 envelope is frozen.** Live clients parse it byte-for-byte. If you propose changing it, that is an Econumo wire-contract change with golden-file consequences.
- **Datetimes are `"2006-01-02 15:04:05"`** — space separator, no zone, UTC, no fractional seconds. Frozen.
- **A lapsed user must still be able to log in.** Do not ask Econumo to deactivate anyone: `is_active = false` revokes all tokens, and a user who cannot log in can neither see your notice nor reach your payment page.
- **Account deletion does not exist in Econumo yet.** A restricted user can export via CSV but cannot delete their account. It is the planned next iteration; direction is recorded in the design doc.

---

## 8. Reference

In the econumo repository (`github.com/econumo/econumo`, public):

- `docs/superpowers/specs/2026-07-19-cloud-monetization-trial-access-design.md` — the authoritative design, including everything in §4 above
- `docs/superpowers/plans/2026-07-19-access-model-and-enforcement.md` — what was implemented (Plan A)
- `CLAUDE.md` — repository conventions, the frozen wire contract, configuration reference
- Issue #120 — deferred follow-ups from Plan A
- PR #119 — the merged implementation
