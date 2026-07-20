# Cloud monetization: trial, access levels, and the payment contract

**Date:** 2026-07-19
**Status:** approved (design)

Supersedes an earlier unreleased draft (2026-07-11) that proposed a fixed
seven-day trial. That draft was never committed or implemented; the trial
boundary and the pricing unit have both changed since — see "Why not a
seven-day trial".

## Problem

The cloud edition runs two separate properties:

- a **demo** instance where anyone can register and use the product for free,
- a **production** instance gated behind a one-off Stripe payment (the SPA hides
  the sign-up form via `PAYWALL_ENABLED`, the user pays on an external page,
  Stripe calls a webhook, n8n runs `user:create` and mails a generated password).

The demo carries the users; production carries the ability to pay. They never
meet. Production registrations are near zero.

This is the expected outcome of a paywall placed before value. Econumo's value
is not visible until a user has entered accounts and transactions — asking for
payment first asks someone to pay for something they have never seen.

We want a single production property: **registration open to everyone, a trial
long enough to reach the product's moment of value, then a one-off payment for
core access.**

That requires the product to answer three questions it cannot answer today:

1. When does a given user's free access end?
2. What happens when it ends — how is the account restricted?
3. How does a payment lift the restriction?

The self-hosted edition must not acquire a paywall, a trial, or any notion of
money.

## Why not a seven-day trial

The product's moment of value is **closing the first calendar month** — seeing
plan against actual. A trial must therefore let the user live through at least
one complete budget month. Fixed day-counts do not:

| registration | first complete calendar month ends | days from registration |
|---|---|---|
| 31 July | 31 August | 31 |
| 1 July | 31 July | 30 |
| **2 July** | 31 August (July is incomplete) | **60** |

A seven-day trial delivers the moment of value to nobody. Thirty days fails
everyone who registers between roughly the 2nd and the 5th: they see a stub of a
partial month and hit the wall immediately before the month that would have shown
them the point. That is the worst possible ordering — maximum data-entry effort,
zero reward.

**Decision: the trial runs until the end of the next calendar month.** Every user
gets at least one fully closed month (29–62 days depending on registration date).

## Non-goals

- **Account deletion.** A "delete my user and all related data" endpoint does not
  exist in the product today (no endpoint, no cascades). It is **the planned next
  iteration** and gets its own design. It does not block this work — a restricted
  user can still log in and export via CSV — but shipping a paywall without an
  exit is only acceptable as a temporary state. See below for the direction
  already agreed and the one constraint it places on *this* spec.
- **The payment portal itself.** It lives in a private repository. This spec
  fixes only the contract between it and the product.
- **Trial abuse.** A new email buys a new trial. Accepted; the portal can address
  it from the Stripe side later.
- **Subscriptions and per-feature entitlements.** No code is written for them
  now. The model below states the invariant that keeps them additive later.
- **Household/seat pricing in the product.** "Pay for your partner" is entirely a
  portal concern — see "Paying for a partner".

### Account deletion: recorded direction

Not designed here, but agreed, so the next iteration starts from it rather than
re-litigating:

- **Delete the data, keep a tombstone row in `users`.** The reason is not foreign
  keys in the abstract — it is *other people's data*: transactions the departing
  user authored in a partner's shared account stay with the partner, and dropping
  the row would leave that history with an unresolvable author. The tombstone
  renders as "Deleted user".
- **The tombstone is `is_active = false` plus a new `deleted_at` column.** Reusing
  `is_active` keeps the login path unchanged (it already refuses inactive users);
  `deleted_at` is what distinguishes a deletion from a ban. `user:activate` must
  refuse a row with `deleted_at` set, or a ban-reversal command becomes an
  undelete. In practice the scrubbed email already makes the row unreachable by
  the email-keyed commands, but the guard should be explicit rather than
  incidental.
- **Accounts the departing user owned and had shared are deleted**, not
  transferred. They are the owner's records and go with the owner.
- **The identifying fields must be cleared, not retained** — email, name, avatar,
  password hash. Keeping the email would both defeat the point of deletion and
  permanently block that address from re-registering, since the user identifier is
  `md5(lower(email))`.
- **All sessions and PATs are revoked** as part of the same transaction.
- **A CLI counterpart** (`user:delete <email>`) alongside the API endpoint, as with
  every other user operation. Being irreversible, it should require an explicit
  confirmation argument rather than deleting on sight.

One consequence of deleting shared accounts needs handling in that design rather
than discovering it in production: **the other participants' own entries go too.**
If Alex owns "Family budget" and Sam has been recording transactions in it for a
year, Alex's deletion destroys Sam's year of work, and Sam finds out by the
account vanishing. The decision stands — the account is Alex's record — but it
cannot be silent. At minimum the deletion confirmation must name the collateral
("3 people share 2 of these accounts and will lose them"), and notifying the
participants is worth considering.

**The constraint on this spec:** deletion will be a `POST`, and a lapsed user is
exactly who most wants to leave. When it lands it **must** be added to the
read-only allowlist below — otherwise the 402 rule turns "your access expired"
into "and you may not leave", which is the trap this spec's non-goal note warns
about.

## Core idea: the product knows about access, not about money

The product stores **what level of access a user has and until when**. It does not
know what a plan is, what a trial is, or that Stripe exists. Every billing concept
is a projection onto two columns:

| billing concept | representation |
|---|---|
| trial | `access_level = full`, `access_until = end of next calendar month` |
| paid (one-off / lifetime core) | `access_level = full`, `access_until = NULL` |
| lapsed / unpaid | `access_until` in the past → effectively read-only |
| manual restriction | `access_level = readonly` |
| self-hosted | `access_level = full`, `access_until = NULL` (the default) |
| banned | `is_active = false` — unchanged, a separate dimension |

`is_active` keeps its current meaning (cannot log in at all; `user:deactivate`
revokes sessions and PATs). It is deliberately **not** overloaded with a "blocked
reason": a lapsed user must be able to log in — otherwise they see neither the
notice, nor the payment link, nor their own data. Why a user is restricted is the
portal's business; the product does not store it.

### Naming follows the same rule

Identifiers in the product describe **access**, never money. `full_access`, not
`paid`; `appAccessCtaClick`, not `appBillingCtaClick`; an "Access" settings entry,
not "Subscription". The same binary serves a self-hosted instance where nobody
pays for anything, and money-flavoured names would be false there.

Two deliberate exceptions: `errs.PaymentRequired` (the RFC 7231 name for HTTP 402
— renaming it would obscure where the status comes from) and `BILLING_URL` (SPA
runtime config naming an external address, not product logic).

## Data model

Two columns on `users`, migrated for both engines (`sqlite` + `pgsql`):

| column | type | notes |
|---|---|---|
| `access_level` | TEXT NOT NULL DEFAULT `'full'` | `full` \| `readonly` |
| `access_until` | DATETIME NULL | NULL = no expiry |

Datetimes follow the frozen layout (`2006-01-02 15:04:05`, UTC, no zone).

**Backfill:** the migration sets every existing row to `full` / NULL. Existing
one-off payers are grandfathered into lifetime access; self-hosted instances see
no behavior change. Only users registered *after* the migration can get a trial.

**Effective level** is a pure function on the `User` entity — `(access_level,
access_until, now) → full | readonly`:

- `access_until` is NULL → `access_level`
- `access_until` is in the future → `access_level`
- `access_until` has passed → `readonly`

There is no background job that "moves users to expired". State is derived from
the clock on every request, so nothing can fail to run and no row is left stale.

### The two-axis invariant

`access_level` is binary and **must never grow tiers** (`pro`, `premium`,
`plus`). There are two independent axes, not one ladder:

- **access** — is this user allowed to write at all? (this spec)
- **entitlements** — which optional features has this user paid for? (future)

A user with lifetime core and a cancelled AI subscription is not "below" a
regular user; they are `core, no ai`. The moment `pro` enters the `access_level`
enum that state becomes inexpressible.

When per-feature subscriptions arrive they land as a separate table
`user_entitlements(user_id, feature, until, …)`, a separate `route → feature` map
in the same middleware, and a `features` key on `CurrentUserResult`. **No
foundation code is written now** — every one of those steps is additive in this
codebase:

| future step | why it is additive |
|---|---|
| `user_entitlements` table | an ordinary migration on both engines |
| `features` on `CurrentUserResult` | a new JSON key; no new routes, only goldens change |
| per-feature gate | a second rule beside the read-only rule, same middleware |
| `POST /admin/set-entitlement` | the admin listener is private, unversioned, single-consumer |
| usage quotas instead of binary flags | columns on a table that does not exist yet |

The real risk is not a missing table; it is a missing written decision. This
section is the foundation.

## Granting the trial

At registration, when trials are enabled, `access_until` is set to the **start of
the month after next**, UTC:

- registered 2 July 2026 → `2026-09-01 00:00:00`
- registered 31 July 2026 → `2026-09-01 00:00:00`

Using the first instant of the following month rather than `23:59:59` on the last
day avoids end-of-day arithmetic and leaves timezone slack — a user in UTC+13
does not lose their final evening.

Users created by the CLI (`user:create`) get `access_until = NULL`: an
operator-created account is a deliberate grant, not a trial.

Config: `ECONUMO_TRIAL` = `none` (default — self-hosted gets no trial and no
trace of billing) | `end-of-next-month` (cloud).

## Enforcement: one middleware rule

`middleware.Auth` already resolves the bearer token against the DB on every
request. `TokenAuthenticator.Authenticate` is extended to return the caller's
effective access level alongside the user id and token id:

```go
Authenticate(ctx context.Context, token string) (userID vo.Id, tokenID vo.Id, level model.AccessLevel, err error)
```

**This costs a join on the hot path, and that is unavoidable.** Today
`Authenticate` (`internal/user/authenticate.go:12`) reads `access_tokens` only and
never touches `users` — a deliberate minimalism documented at
`internal/user/admin.go:90-93`: per-request auth needs no `is_active` join because
deactivating a user revokes their tokens. That trick does not extend to access
level: an expired trial must **not** revoke sessions, since a lapsed user has to
keep logging in to see the notice and pay. So `GetAccessTokenByHash` grows a join
onto `users` for `access_level` and `access_until`. It stays a single round trip,
but it is a new join where there was none, on the most frequently executed query
in the product.

The rule itself lives **inside `middleware.Auth`**, matching on `r.Method` and
`r.URL.Path`. `Auth` is applied per route by each feature's `RegisterAPI`, but it
is one implementation, so the rule is written once and no feature package is
edited.

The middleware then applies a single rule:

> `POST` + effective level `readonly` + path not on the allowlist → **402 Payment
> Required**, in the standard handled-error envelope.

This works because of the frozen API convention: **GET reads, POST writes — there
is no PUT/PATCH/DELETE**. One rule covers every write in the product, and none of
the ten feature packages is touched. CSV export (`GET`) keeps working for a
restricted user with no special case; CSV import (`POST`) is blocked, correctly.

**Allowlist (POST endpoints permitted while read-only):**

- `logout-user`, `revoke-session`, `revoke-other-sessions`, `revoke-personal-token`
- `update-password`

Password change is allowed on purpose: it is a security operation, not data.
Locking a user out of rotating a compromised password would be indefensible.
`create-personal-token` is *not* allowed — it grants new write-capable credentials.

The account-deletion endpoint joins this list the moment it exists (see "Account
deletion: recorded direction"). The principle behind the allowlist is that a
restricted user may always secure their account and leave it — they may not add
data.

**Enforcement is per caller.** A restricted user cannot write anywhere, including
into accounts shared with them. A paying user keeps writing into accounts shared
by a restricted user. One person's lapsed payment does not freeze a household.

**Error envelope (new):** an `errs.PaymentRequired` type mapping to HTTP 402:

```json
{"success": false, "message": "Read-only access. Write operations are disabled.", "code": 402, "errors": {}}
```

402 lets the SPA distinguish this from 400 (validation) and 401 (auth) with a
single status comparison. The message is deliberately product-neutral — the SPA
renders its own copy and the portal owns all billing wording.

## Paying for a partner

The pricing model is: *I pay for myself; if I see the value for my partner, I pay
for them too.*

The product implements **no** household, seat, or discount concept. Enforcement
stays per caller; the portal charges one Stripe customer and issues independent
`set-access` calls for each beneficiary.

`POST /admin/set-access` accepts **either** `userId` or `email`: the portal uses
`email` for self-purchases (it has the Stripe customer's address) and `userId`
when one user pays for another. `model.UserResult` — the person embed in
`GetConnectionListResult` — carries only `id`, `avatar`, and `name`, so a partner
is only ever addressable by id from the client side.

### Seeing who needs paying for

`ConnectionResult` gains the connected user's effective access state, so the
connections page can show "your partner is read-only" and offer to pay for them:

```go
type ConnectionResult struct {
    User           UserResult            `json:"user"`
    SharedAccounts []AccountAccessResult `json:"sharedAccounts"`
    AccessLevel    string                `json:"accessLevel"`
    AccessUntil    string                `json:"accessUntil"` // "" when NULL
}
```

**Deliberately on `ConnectionResult`, not on the shared `UserResult` embed.**
`UserResult` is embedded in six places — including transaction author
(`internal/model/transaction_dto.go:20`, `internal/model/account_dto.go:129`) —
and transaction lists are the heaviest responses in the product. Putting two
fields on every author of every row costs real bytes for data that is needed on
exactly one page, and would broadcast payment status far wider than the
connections view.

Exposing a connection's access state to the people they share finances with is an
intentional disclosure: they already see each other's accounts and transactions,
and the whole point is to let one of them fix the other's lapsed access.

### Handing off to the portal

The link carries a **signed identity assertion**, not data. Anything the SPA puts
in a URL is forgeable by whoever operates the browser, so a bare `uid` cannot be
trusted the moment the portal starts *displaying* status rather than just taking
money — a forged id would expose another person's payment state.

The product mints a short-lived handoff token:

```
payload = {uid, exp}                     // exp = now + 10 minutes
sig     = HMAC-SHA256("billing-handoff:v1" || payload, ECONUMO_ADMIN_TOKEN)
link    = ${BILLING_URL}?t=<base64url(payload)>.<base64url(sig)>[&for=<user id>]
```

**No new secret.** The portal already holds `ECONUMO_ADMIN_TOKEN` to authenticate
against the admin listener; the `billing-handoff:v1` prefix gives domain
separation so a handoff signature cannot be replayed as anything else. Stateless,
no storage, verification is a few lines on the portal side.

The SPA never assembles this link itself — it calls
`POST /api/v1/user/create-billing-link` and follows the returned `url`, so the
signing key never leaves the server and the token is always freshly minted.

The portal then fetches everything it needs server-side (see
`GET /admin/user-context`). **No email, name, or access state ever travels through
the browser.**

`for` is a **preselection hint only** — it authorizes nothing, since the portal
already knows the visitor from the signature and already has their full context:

| link | portal shows |
|---|---|
| `?t=…` | self preselected |
| `?t=…&for=<own id>` | identical — a legal value, not a special case |
| `?t=…&for=<partner id>` | that partner preselected |

The portal validates `for` against the `user-context` it fetched (self or one of
the connections); anything else is ignored and falls back to self. A forged `for`
at worst pays for someone the user is already connected to.

Hygiene: the token is read-only (it grants no writes), lives 10 minutes, and the
portal exchanges it for its own session and strips it from the URL so it does not
persist in history or `Referer`.

## Contract with the payment portal

The portal (private repository) owns everything the product must not know: Stripe
checkout and webhooks, prices, promo codes, its own database (Stripe objects,
which reminder emails were already sent), its own cron, and all billing email. The
product sends **no** money-related mail and runs **no** billing cron; its
`MAILER_DSN` stays dedicated to password reset.

The portal never touches the product's database. It talks to the product over a
**dedicated admin listener**, and the product knows nothing about the portal in
return — not even its URL (that lives in the SPA's runtime config).

### Admin listener

A second `http.Server`, started by `serve` **only when both** `ECONUMO_ADMIN_PORT`
and `ECONUMO_ADMIN_TOKEN` are set. Self-hosted instances set neither, so the
listener never opens and the routes do not exist. The port is not published to the
internet — the portal reaches it over the internal network. This is fail-safe by
construction: a misconfigured reverse proxy cannot expose an admin route that is
not on the public mux at all.

Auth: `Authorization: Bearer <ECONUMO_ADMIN_TOKEN>`, compared in constant time.
Responses use the same `httpx` envelope as the public API.

| endpoint | body / query | effect |
|---|---|---|
| `POST /admin/set-access` | `{"userId": "…"}` **or** `{"email": "…"}`, plus `"level": "full\|readonly"` and `"until": "2027-01-01 00:00:00" \| null` | sets the two columns |
| `GET /admin/expiring-users?days=3` | — | `[{"id","email","name","accessUntil"}]` — everyone whose access expires within N days |
| `GET /admin/user-context?userId=…` | — | `{user: {id,name,email,accessLevel,accessUntil}, connections: [{id,name,email,accessLevel,accessUntil}, …]}` |

Supplying both `userId` and `email`, or neither, is a validation error.

`user-context` is what the portal calls after verifying a handoff token. It keeps
the connection graph — and the authorization rules behind it — in the product,
rather than duplicating "who may see whom" in a second system. The portal renders
the visitor plus their connections with each one's access state, so a user can
arrive on a bare link and choose whom to pay for on the spot.

`expiring-users` is how the portal knows whom to email; it decides what to send
and remembers what it already sent, in its own database. When per-feature
entitlements arrive they get their own expiry endpoint rather than extending this
one.

### CLI (kept, for humans)

The same use cases stay reachable from the CLI for operations, alongside the
existing `user:*` commands — both edges call the same service methods:

```
user:set-access <email> <full|readonly> [YYYY-MM-DD]    # date omitted = no expiry
user:show <email>
```

`user:set-access` takes the expiry as an **optional positional**, matching the
existing commands (`token:purge [days]`, `currency:add <code> [name]
[fraction-digits]`) rather than introducing a `never` sentinel word. An omitted
date means `access_until = NULL`, exactly as `"until": null` does on the admin
endpoint. The date format is `datetime.DateLayout` (`YYYY-MM-DD`), as in
`currency:update-rates`.

`user:show` is new and closes a real gap: every existing CLI command is a
mutation, so today an operator can change state but cannot read it back without
opening the database directly. That is tolerable while the only fields are name
and email; it stops being tolerable once access level and expiry are the things
being set. It prints id, name, email, active flag, access level, effective access
level, and expiry:

```
$ econumo user:show user@example.test
Id:              0198f3c1-...
Name:            Alex
Email:           user@example.test
Active:          yes
Access level:    full
Access until:    2026-09-01 00:00:00
Effective:       full
```

`Effective` is printed alongside the raw column on purpose: the whole model turns
on `(access_level, access_until, now)` collapsing to one value, and an operator
debugging "why can this user not write" needs to see the result, not re-derive it.
Read-only by construction — it takes no other arguments and changes nothing.

## What the SPA sees

Two new fields on `CurrentUserResult` (returned by both `login-user` and
`get-user-data`): `accessLevel` and `accessUntil`.

One new route: **`POST /api/v1/user/create-billing-link`** → `{"url": "…"}`,
which mints a fresh handoff token and returns the assembled portal URL. It takes
an optional `for` (a user id) and returns 400 when `BILLING_URL` is unset.

The token is minted per click rather than carried on `CurrentUserResult`: it
lives 10 minutes, `get-user-data` is cached by TanStack Query, and a user opening
Settings half an hour after login would otherwise arrive at the portal with an
expired assertion. "The payment link is broken" is the worst possible place for
that failure.

This means the apiparity guards *do* move — the scenario catalogue and the
scanned-route count both grow by one, and a scenario plus golden must be added
for the new route. Every other change here is additive to existing payloads and
touches only golden content.

- **Banner** when `accessUntil` is within 3 days or has passed. Generic copy
  ("access ends in N days" / "read-only"), CTA to the portal.
- **Settings entry** "Access" → same link. Not "Subscription": core is a one-off
  purchase, and that word would promise recurring charges that do not exist.
- **Per-connection CTA** "Pay for <name>" in the connections view, shown when a
  connection's `accessLevel` is read-only or its `accessUntil` is near, using the
  `for=<partner user id>` link above.
- **Handoff token** minted server-side and appended to every billing link.
- **`BILLING_URL`** is read from `econumo-config.js` (empty by default). Empty →
  no banner, no settings entry, no per-connection CTA, no link. A self-hosted
  build shows no trace of billing.
- **402 from the API** → read-only toast + banner. The primary create action (FAB)
  is hidden while read-only, so a restricted user does not walk into a wall.
  Individual per-screen write buttons are left alone (the persistent banner
  carries the message); hiding all of them across ten features is not worth the
  churn.
- **Cleanup:** `PAYWALL_ENABLED`, the paywall block on `RegistrationPage.tsx`,
  `paywallUrl` in `web/src/lib/package.ts`, and the associated i18n keys are
  removed — registration is open to everyone now.

## Funnel analytics (required, not optional)

Product analytics went live on 12 July 2026 (0 events before; 757 events / 248
unique users in the first week). Only two actions are instrumented:
`appAccountFolderCollapse`/`Expand` and `appUIModalTransactionOpen`/`Close`. There
is no registration event, no host property, and therefore no funnel.

Shipping the trial without instrumentation means knowing revenue in two months
and not knowing **where** the funnel breaks — and the next iteration would be
equally blind. The following `METRICS` keys ship with this work
(`web/src/lib/metrics.ts`, `app`-prefixed camelCase, wired at each action's
success point per the project analytics rule):

| key | fired when |
|---|---|
| `appUserRegister` | registration succeeds |
| `appAccountCreate` | an account is created |
| `appTransactionCreate` | a transaction is created |
| `appBudgetView` | the budget page is opened |
| `appAccessBannerShow` | the trial/read-only banner renders |
| `appAccessCtaClick` | the portal CTA is clicked |
| `appAccessPartnerCtaClick` | the "pay for partner" CTA is clicked |
| `appAccessReadonlyBlocked` | a 402 is received |

"First account" and "first transaction" need no separate events — they are the
first occurrence per person in PostHog. `appBudgetView` after a month boundary is
the proxy for reaching the moment of value, which is the hypothesis this trial
boundary rests on.

### Access state on every event

Every product event carries the caller's access state, so any funnel can be split
by it without a join. This is registered once as a PostHog **super property**
rather than added at each call site — a per-call-site attribute would be forgotten
on the next new event, and the metrics-coverage test cannot catch a missing
property.

The property is `accessState`, with three values rather than the raw
`access_level`:

| value | condition |
|---|---|
| `trial` | effective `full` and `accessUntil` is set |
| `full_access` | effective `full` and `accessUntil` is NULL |
| `readonly` | effective `readonly` |

The raw column cannot distinguish time-limited access from permanent access, and
that is precisely the distinction the funnel is about.

`full_access` rather than `paid`: the product describes access, never money. A
self-hosted instance emits `full_access` for every user, where `paid` would be a
lie — and the same code path serves both editions.

It is registered as a super property (not a person property) so each event records
the state at the time it fired: with person-on-events, person properties reflect
ingestion-time values, which would blur the `trial` → `full_access` transition
this analysis exists to measure. It is re-registered on login and whenever
`get-user-data` returns.

`web/src/lib/metrics-coverage.test.ts` enforces that every key is actually fired.

## Retiring the demo instance

The demo runs a **separate database** and already expires accounts after seven
days. Its data is transient by design, so there is nothing to migrate and no
retention obligation.

The wind-down needs no notice campaign: set `ECONUMO_ALLOW_REGISTRATION=false` on
the demo so no new accounts are created, let the existing ones age out on the
seven-day limit they already carry, then shut the instance down. New arrivals go
to production and get the real trial.

## Configuration

| variable | default | meaning |
|---|---|---|
| `ECONUMO_TRIAL` | `none` | `none` \| `end-of-next-month`. Cloud sets `end-of-next-month`. |
| `ECONUMO_ADMIN_PORT` | empty | admin listener port; empty → no listener |
| `ECONUMO_ADMIN_TOKEN` | empty | admin bearer token; empty → no listener |
| `BILLING_URL` (SPA, `econumo-config.js`) | empty | payment portal URL; empty → no billing UI |

The same binary serves a self-hosted user (all defaults) and the cloud.

## Risks accepted

1. **A privileged admin surface exists in the product.** Mitigated by making it
   impossible to reach unless deliberately configured: no token or no port → no
   listener. Nothing about it is cloud-specific — `set-access` is as
   business-neutral as the existing `user:deactivate`.
2. **Trial abuse** — a new email buys a new trial. Not addressed.
3. **A forged `for` in the billing link** at worst lets someone pay for a person
   they are already connected to. Identity itself is not forgeable — it comes from
   the signed handoff token, not the URL parameters.
4. **`ECONUMO_ADMIN_TOKEN` serves two roles** — bearer credential for the admin
   listener and HMAC key for handoff tokens. Domain separation
   (`billing-handoff:v1`) prevents cross-purpose signature reuse. The alternative
   is a separate `ECONUMO_BILLING_SECRET`, rejected to avoid a fourth billing
   variable; revisit if the portal ever needs to rotate one without the other.
5. **A handoff token can leak via history or `Referer`.** Bounded by a 10-minute
   TTL and by granting read-only billing context, never writes; the portal strips
   it from the URL on arrival.
6. **The month-boundary hypothesis is unproven.** The trial length rests on
   "value arrives at the first closed month", which is currently a belief, not a
   measurement. The analytics above exist to test it; expect to revisit the
   boundary once there is data.
7. **Demo shutdown loses users.** Some of the ~248 weekly actives will not
   re-register in production. The cost is low — demo accounts already expire
   after seven days, so nobody loses durable data — and a split funnel costs more
   than the churn.
8. **Golden files and route guards change** — two new fields in `login-user` /
   `get-user-data`, two more in `get-connection-list` / `accept-invite`, and one
   new route (`create-billing-link`) that grows the apiparity scenario and
   scanned-route counts. Regenerate with `UPDATE_GOLDEN=1` and read the diff; a
   golden change means observable behavior changed.

## Testing

- Unit: the effective-level function — NULL `until`, `until` exactly now, past,
  future; `readonly` with a future `until` stays `readonly`.
- Unit: the trial-boundary function — registration on the 1st, the 2nd, the last
  day of a 31-day month, the last day of February, and across a year boundary
  (December → 1 February).
- Middleware: GET passes while read-only; POST returns 402; every allowlisted POST
  passes; a `full` user is unaffected; the 402 envelope matches the frozen shape.
- Integration: registration with `ECONUMO_TRIAL=end-of-next-month` sets
  `access_until`; with `none` leaves it NULL; `user:create` always leaves it NULL.
  The migration backfills existing rows to `full` / NULL.
- Admin listener: no token or no port → not started; wrong bearer → 401;
  `set-access` by `userId` and by `email` both round-trip; both-or-neither is a
  validation error; `expiring-users` returns the right window; `user-context`
  returns the caller plus exactly their connections, with each one's effective
  level, and 404s on an unknown user id.
- Handoff token: a freshly minted token verifies; a token past `exp` is rejected;
  a tampered payload is rejected; a signature made with a different key is
  rejected; a signature made over the same payload *without* the
  `billing-handoff:v1` prefix is rejected (domain separation holds).
- Connections: `get-connection-list` and `accept-invite` carry `accessLevel` /
  `accessUntil` per connection, and `accessUntil` is `""` when NULL.
- CLI: `user:set-access` with and without the date positional, an invalid level, an
  invalid date, and an unknown email; `user:show` renders every field and prints an
  `Effective` value that differs from `Access level` once the expiry has passed.
  Both sit alongside the existing command tests in `internal/cli/commands_test.go`.
- Frontend (vitest): banner appears within the window and while read-only, hidden
  when `BILLING_URL` is empty; a 402 response flips the client into read-only; the
  per-connection CTA appears only for a read-only or near-expiry connection and
  builds the `for=<user id>` link; `accessState` is registered as a super property
  and resolves to `trial` / `full_access` / `readonly` correctly.
- Regenerate the apiparity goldens and inspect the diff.
