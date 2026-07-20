# Admin API and billing handoff

**Date:** 2026-07-19
**Status:** design approved, not implemented
**Depends on:** `2026-07-19-cloud-monetization-trial-access-design.md` (merged as PR #119)

## Problem

The access model shipped and is inert. `users.access_level` and
`users.access_until` exist, the 402 middleware enforces them, and the CLI can set
them by hand. Nothing else can.

The payment portal — a separate private service that owns Stripe, prices, and all
billing mail — has no way to talk to the product. It cannot write access after a
payment, and it cannot learn who arrived on a billing link. The SPA cannot send a
user to the portal, because the link must carry a signed identity assertion that
only the server can mint.

This document specifies that interface: a private admin listener with two
endpoints, a handoff-token scheme, and one new public route.

## Scope

In scope: the admin listener (`set-access`, `user-context`), the handoff token,
`POST /api/v1/user/create-billing-link`, and the configuration and guards they
need.

Out of scope: `GET /admin/expiring-users`. It serves only the reminder cron, which
is meaningless until the portal has its own record of what it already sent. The
parent spec also notes that per-feature entitlements will want their own expiry
endpoint, so its shape is likely to change. Deferring it avoids freezing a query
we would rewrite — and, because it was the only endpoint needing a windowed
lookup, deferring it removes all new SQL from this change.

## Two defects in the parent spec

**1. A read-only user cannot reach the payment link.** `ReadonlyAllowedPaths`
(`internal/web/middleware/auth.go:45`) holds five entries.
`create-billing-link` is a `POST` and is not among them, so the middleware would
return 402 to precisely the user trying to pay. It joins the allowlist. The
list's comment widens accordingly: the endpoint is read-only and mints no
write-capable credentials, unlike `create-personal-token`.

**2. `BILLING_URL` is specified in two contradictory places.** The parent spec
says the product knows nothing about the portal, "not even its URL (that lives in
the SPA's runtime config)", while also requiring `create-billing-link` to return
an *assembled* URL and to 400 "when `BILLING_URL` is unset" — which the backend
can only do if it holds the value.

Resolved with a single `ECONUMO_BILLING_URL`, read by the backend and merged into
`window.econumoConfig` by the SPA handler through the existing `Object.assign`
path (as `ANALYTICS` and `ALLOW_REGISTRATION` already are). One variable, one
source of truth; the two halves cannot disagree.

## Addressing: user ids only

Both endpoints address users by `userId`. The parent spec allowed `email` as an
alternative and defined a both-or-neither validation rule; that rule disappears.

Every purchase originates from a handoff link, so the portal knows the user id
before checkout and stores the `userId`→Stripe-customer mapping itself. By
webhook time it always has the id. Nobody pays before creating an account.

`user-context` still *returns* `email` — the portal needs it to prefill the
non-editable Stripe checkout address — but never accepts one.

The CLI keeps its email-keyed commands: an operator has an address, not a UUID.

## Architecture

A new feature package, `internal/admin/`:

```
internal/admin/
  ports.go        consumer-side interfaces: UserLookup, ConnectionLookup
  context.go      the user-context use case
  access.go       the set-access use case
  api/
    routes.go     RegisterAdmin(mux) — never on the public mux
    handler.go    thin handlers over httpx
```

It never imports `user` or `connection`. It declares what it needs;
`internal/server` wires adapters in `glue_admin_userlookup.go` and
`glue_admin_connections.go` over those features' public APIs, exactly as
`glue_budget_userlookup.go` does today.

Placing it in `internal/user/api` was rejected: it would mix a privileged
internal surface into the public-API package and make "is this route public?" a
per-file question. Placing it in `internal/web` was rejected: leaves may not
import features, so every dependency would become an interface defined in a
package documented as holding no feature logic.

### Two servers

`serve` gains a second `http.Server` on `ECONUMO_ADMIN_PORT`, built by
`server.BuildAdmin(cfg, db)`, started only when both admin variables are set.
Self-hosted instances set neither, so the routes do not exist at all — a
misconfigured reverse proxy cannot expose a route that is not on any mux it
serves.

`cmd/econumo/main.go:223` currently blocks on a single `ListenAndServe`. It
becomes a concurrent wait over both servers with graceful shutdown on signal; a
failure in either brings the process down rather than leaving a half-serving
binary.

### Middleware

```
requestid -> accesslog -> recover -> adminauth
```

Deliberately shorter than the public chain (`requestid -> accesslog -> recover ->
cors -> timezone -> language -> auth`). No CORS: the listener is never reached by
a browser. No timezone or language: nothing here is user-facing, and datetimes
are frozen UTC.

`adminauth` compares the bearer against `ECONUMO_ADMIN_TOKEN` with
`subtle.ConstantTimeCompare` and emits the standard 401 envelope. Responses use
the same `httpx` envelope as the public API, so the portal parses one format.

## Endpoints

### `POST /admin/set-access`

```json
{"userId": "0198f3c1-…", "level": "full|readonly", "until": "2027-01-01 00:00:00"}
```

`until` null or omitted sets `access_until = NULL`. Parsed with the frozen
`datetime` layout, UTC. Unknown `userId` returns 404 in the standard envelope;
an invalid `level` or unparseable `until` returns the validation envelope.

**It is already idempotent.** The portal's handoff warns "be idempotent, Stripe
retries" — this is a set, not an increment, so writing the same `(level, until)`
twice is indistinguishable from writing it once. No `operation_requests_ids`
guard is needed; that mechanism exists for client-supplied operation ids on
*create* endpoints.

Implementation: `AdminSetAccessByID` alongside today's email-keyed
`AdminSetAccess` (`internal/user/admin.go:107`), both delegating to one core
taking a `*model.User`.

### `GET /admin/user-context?userId=…`

```json
{"user":        {"id","name","email","accessLevel","accessUntil","effectiveAccessLevel"},
 "connections": [{"id","name","email","accessLevel","accessUntil","effectiveAccessLevel"}]}
```

Unknown `userId` returns 404.

**Three access fields, not two**, for the reason `user:show` prints both the raw
column and the effective value: `accessUntil` is the date to display,
`effectiveAccessLevel` answers "can this person write right now", and raw
`accessLevel` distinguishes a **lapsed** user (offer a purchase) from a
**manually restricted** one (do not). Returning only the raw pair would force the
portal to re-implement the collapse rule — the duplication the parent spec
avoided by keeping the connection graph in the product.

`email` on connections lets the portal notify a beneficiary that someone paid for
them. It is the one field here not strictly required by checkout, and is the
first thing to drop if that notification is never built.

Keeping the connection graph server-side means the rule for "who may see whom"
lives in one system, not two.

### The N+1, accepted

`GetConnectionListResult` embeds `model.UserResult`, which carries only `id`,
`avatar`, and `name` — deliberately no email. So `user-context` resolves
connection emails with one `GetByID` per connection.

Chosen knowingly: connections are partners, typically 0–3, and the alternative is
a new cross-engine query to save a handful of indexed primary-key lookups. Revisit
if anyone accumulates dozens of connections.

## The handoff token

Lives in **`internal/infra/handoff/`**, not in `internal/admin/`. The only minter
is `create-billing-link`, which belongs to the `user` feature — and a feature may
never import another feature, so a signer under `internal/admin/` would be
unreachable from its sole caller. `infra` is the shared leaf that already holds
the other cryptographic primitives (`infra/auth`: password hashing, AES, the
identifier hash), which is exactly what this is.

```
payload = base64url(JSON{uid, exp})            exp = now + 10 minutes
sig     = base64url(HMAC-SHA256(key=ECONUMO_ADMIN_TOKEN, msg="billing-handoff:v1" || payload))
token   = payload "." sig
```

The HMAC covers the **encoded** payload, not the raw struct: signing
pre-serialization invites a verify-side mismatch when JSON key order or escaping
differs between implementations.

No new secret. `ECONUMO_ADMIN_TOKEN` is already shared with the portal; the
`billing-handoff:v1` prefix gives domain separation, so a handoff signature
cannot be replayed as anything else. The token is stateless, read-only, and lives
10 minutes.

### `POST /api/v1/user/create-billing-link`

Body: optional `for` (a user id). Returns `{"url": "…"}`; 400 when
`ECONUMO_BILLING_URL` is unset.

The assembled URL carries `lang`, so the portal renders in the language the user
is already reading:

```
${ECONUMO_BILLING_URL}?t=<token>[&for=<user id>]&lang=<en|ru>
```

Sourced from `reqctx.Language(ctx)`, not from `users.language`. The SPA sends
`Accept-Language: locale()` on every request (`web/src/api/client.ts:14`), so the
`Language` middleware has already resolved the user's *current* UI choice, and it
defaults to `en` when absent or unsupported. `users.language` is written at login
(`internal/user/login.go:59`) and documented as write-only; it is a lagging copy,
and reading it here would be both staler and a new dependency on a column
deliberately kept write-only.

Named `lang` rather than `locale` because the value is a two-letter language tag
from `i18n.Supported`, not a region-qualified locale. It is an unsigned query
parameter, like `for`: a display preference the user already controls needs no
tamper protection, and the portal falls back to its own default for a language it
does not have.

Minted per click rather than carried on `CurrentUserResult`: the token lives 10
minutes and `get-user-data` is cached by TanStack Query, so a user opening
Settings half an hour after login would otherwise arrive with an expired
assertion.

Three requirements:

- **It joins `ReadonlyAllowedPaths`** (see defect 1).
- **`for` is validated as a well-formed id** before reaching the URL. The product
  does not check it names a real connection — the portal does that against the
  `user-context` it fetches — but an unvalidated string concatenated into a query
  is parameter injection into the portal.
- **The URL is assembled with `net/url`**, setting `url.Values`, never string
  concatenation. `ECONUMO_BILLING_URL` is validated at boot as an absolute
  `http`/`https` URL.

## Configuration

| variable | default | meaning |
|---|---|---|
| `ECONUMO_ADMIN_PORT` | empty | admin listener port; with the token, gates the listener |
| `ECONUMO_ADMIN_TOKEN` | empty | bearer credential **and** handoff HMAC key; minimum 32 characters |
| `ECONUMO_BILLING_URL` | empty | portal URL; empty → `create-billing-link` 400s and the SPA shows no billing |

Boot-time validation fails loudly rather than degrading:

- exactly one of `ECONUMO_ADMIN_PORT` / `ECONUMO_ADMIN_TOKEN` set is an error.
  Half-configured is operator error, and silently not opening the listener is the
  failure mode that costs an afternoon to diagnose.
- `ECONUMO_BILLING_URL` set without `ECONUMO_ADMIN_TOKEN` is an error: the token
  is the signing key.
- a token shorter than 32 characters is an error: it is both a bearer credential
  and an HMAC key.

All three default empty, so a self-hosted binary has no admin listener, no
billing endpoint, and no billing UI.

## Guards

The apiparity guard globs `internal/*/api/routes.go`
(`internal/test/apiparity/guard_test.go:24`), so it would auto-discover
`internal/admin/api/routes.go`, demand a scenario and golden per admin route, and
fail when replaying them against the public mux returns 404.

The scan therefore **explicitly excludes** `internal/admin`, with a comment
stating why. Naming the file to dodge the glob was rejected: it evades a guard by
filename coincidence, and the next person to move the file back re-breaks the
suite with no explanation anywhere.

Paired with that exclusion, a **new reachability guard** asserts no `/admin/`
path is served by `server.BuildAPI`. This turns the design's central fail-safe
claim — that a misconfigured proxy cannot expose an admin route — into an
enforced test rather than a property maintained by hand.

## Persistence

None. `GetByID` and `Save` exist on the user repo; `GetConnectionList` exists on
the connection service. With `expiring-users` deferred, this change adds no sqlc
query, no migration, and no new engine-comparison surface.

## Testing

- **Handoff:** mint→verify roundtrip; expired `exp`; tampered payload; signature
  under a different key; signature computed *without* the `billing-handoff:v1`
  prefix (domain separation holds).
- **Admin auth:** neither variable set → listener not started; wrong bearer →
  401; valid bearer → 200.
- **`set-access`:** by id roundtrips; unknown id → 404; invalid level;
  unparseable `until`; `null` → NULL; applying the same call twice is identical.
- **`user-context`:** returns the caller plus exactly their connections; unknown
  id → 404; `effectiveAccessLevel` diverges from `accessLevel` once `accessUntil`
  has passed.
- **`create-billing-link`:** a read-only caller gets 200, not 402 — the
  regression test for defect 1; 400 when `ECONUMO_BILLING_URL` is unset;
  malformed `for` rejected; the assembled URL parses and carries every parameter;
  `lang` follows `Accept-Language` and falls back to `en` when the header is
  absent or names an unsupported language.
- **Config:** half-configured admin variables fail at boot; a short token fails;
  a non-absolute `ECONUMO_BILLING_URL` fails.
- **Guards:** the reachability guard proves no `/admin/` path is on
  `server.BuildAPI`; `minRoutes` rises 87→88; `create-billing-link` gets a
  scenario and golden.

Gates: `make go-test` (including the 72% coverage floor), then
`UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` — one new route and the
`create-billing-link` golden should be the only diff.

## Risks accepted

1. **A privileged surface exists in the product.** Mitigated by making it
   unreachable unless deliberately configured, and by the reachability guard.
2. **`ECONUMO_ADMIN_TOKEN` serves two roles** — bearer credential and HMAC key.
   Domain separation prevents cross-purpose reuse, but rotating one requires
   rotating both, on both sides at once.
3. **A handoff token can leak via history or `Referer`.** Bounded by the
   10-minute TTL and by granting read-only context, never writes; the portal
   strips it from the URL on arrival.
4. **A forged `for`** at worst pays for someone the user is already connected to.
   Identity comes from the signature, not the query.
5. **Connection emails cross to the portal.** Deliberate, and narrower than the
   access-state disclosure the parent spec already accepted between connected
   users.

## Open questions

- **Reminder emails** need `expiring-users`, deferred above. The portal must
  build its own sent-log first, and the window semantics (future-only versus
  symmetric around now, and whether a missed cron run is self-healing) should be
  decided when that work starts.
- **Chargebacks** presumably mean `set-access readonly`, but no policy exists.
- **Token rotation** has no procedure; see risk 2.
