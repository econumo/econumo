# Brute-force protection for auth endpoints — design

**Date:** 2026-07-09
**Status:** Approved

## Problem

`POST /api/v1/user/login-user`, `reset-password`, `remind-password`, and
`register-user` accept unlimited attempts. Combined with the fast frozen
password hash (sha512 x500), this makes online password guessing and
reset-code guessing cheap, and lets an attacker mail-bomb arbitrary addresses
via remind-password or mass-create accounts via register-user.

## Decision summary

Per-username attempt limiting with a per-endpoint global backstop, enforced in
the user use cases via a new in-memory `internal/infra/ratelimit` service.
Over-limit requests get HTTP 429 with the standard handled-error envelope.
Limits are configurable via `ECONUMO_RATE_LIMIT_*` env vars. No IP-based
keying (spoofable/misconfigurable behind reverse proxies), no fixed sleep, no
DB persistence (single-binary deployment; state resets on restart by design).

## Architecture

- **New package `internal/infra/ratelimit`** — in-memory limiter: mutex + map
  of sliding-window attempt counters, constructed with `port.Clock` (tests
  drive time). No engine or feature dependencies.
- **Consumer-side interface** in `internal/user/ports.go` (dependency rule —
  the feature never imports infra concretions directly beyond existing
  precedent; wiring happens in `internal/server`):

  ```go
  type AttemptLimiter interface {
      Allow(scope, key string) error // *errs.TooManyRequestsError when over limit
      Fail(scope, key string)        // record a failed attempt
      Clear(scope, key string)       // wipe the key after success
  }
  ```

- **Enforcement points** — top of each use case (`Login`, `ResetPassword`,
  `RemindPassword`, `Register`): `Allow` first; `Fail` after a failed
  verify/lookup; `Clear` after successful login/reset. Keys are the
  lowercased+trimmed submitted username/email. Scopes: `login`, `reset`,
  `remind`, `register`, plus internal global scopes per endpoint.
- **Wiring** — `server.BuildAPI` constructs the limiter from config values and
  passes it to `user.NewService`. The apiparity harness constructs its own
  (generous limits so existing goldens are unaffected; a dedicated scenario
  uses the real path to freeze the 429).

## Policy

| Scope | Counted event | Default limit | On success |
|---|---|---|---|
| login | failed verify (unknown user or bad password) | 5 / window / username | `Clear` |
| reset | failed attempt (unknown user, bad code, expired code) | 5 / window / username | `Clear` |
| remind | every request (each sends an email) | 3 / window / username | — |
| register | every attempt | 5 / window / email | — |
| global (each endpoint) | every request to that endpoint | 60 / minute | — |

- Window is shared and configurable (default 15 minutes), sliding.
- Anti-enumeration is preserved: `Allow`/`Fail` behave identically whether the
  username exists or not, so a 429 leaks nothing a 401/400 didn't.
- The global cap catches username-spray attacks that per-key limits miss.

## Configuration

Parsed in `config.Load`; a bad value fails at boot (same posture as
`MAILER_DSN`). `0` disables that particular check.

| Variable | Default | Meaning |
|---|---|---|
| `ECONUMO_RATE_LIMIT_LOGIN` | `5` | failed logins per username per window |
| `ECONUMO_RATE_LIMIT_RESET` | `5` | failed reset attempts per username per window |
| `ECONUMO_RATE_LIMIT_REMIND` | `3` | remind requests per username per window |
| `ECONUMO_RATE_LIMIT_REGISTER` | `5` | register attempts per email per window |
| `ECONUMO_RATE_LIMIT_WINDOW` | `15m` | shared sliding-window size (Go duration syntax) |
| `ECONUMO_RATE_LIMIT_GLOBAL` | `60` | per-endpoint global cap per minute (`0` disables) |

Documented in `.env.example` and CLAUDE.md's configuration list.

## Wire contract

New code path only; every existing response stays byte-identical.

- Over-limit: HTTP **429** with the standard handled-error envelope:
  `{"success":false,"message":"Too many attempts. Try again later.","code":429,"errors":{}}`
- New error type `errs.NewTooManyRequests` in `internal/shared/errs`, mapped
  to 429 in `httpx.WriteError`. The message string above is frozen once
  shipped.
- Handlers gain `@Failure 429` swagger annotations; docs regenerated with
  `make swagger`.

## Memory hygiene

Expired windows are pruned lazily on write, plus a hard size cap (10k keys):
when exceeded, evict expired entries first, then oldest. A spray of unique
usernames therefore cannot grow the map unbounded.

## Observability

The existing AccessLog operation line already logs 4xx at WARN with
`err`/`err_type` — 429s are visible with no new logging code. The limiter
never logs keys (usernames/emails are PII).

## Testing

- **Unit** (`internal/infra/ratelimit`): window slide, clear, global cap,
  disable-on-zero, eviction/size cap — all on a fake clock.
- **Use case** (`internal/user`): 6th bad login → 429; success clears the
  counter; equivalent paths for reset/remind/register; disabled limiter
  (0) passes everything.
- **apiparity**: harness wires generous limits so all existing scenarios and
  goldens are unchanged; one new scenario drives a real over-limit sequence
  and freezes the 429 envelope as a golden. Scenario/route guard counts only
  grow.
- **enginecompare**: unaffected — the limiter is engine-agnostic and
  deterministic per test server.

## Out of scope

- IP/proxy-aware limiting (`X-Forwarded-For` trust) — revisit only if
  per-username + global proves insufficient.
- Persistent lockout state across restarts.
- Retry-After header (can be added later without breaking anything).
