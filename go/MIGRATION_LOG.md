# Migration Log — PHP/Symfony → Go backend

A chronological record of how the Go backend (`go/`) was built as a **drop-in
replacement** for the Symfony PHP API, and how each step was verified. It is the
"why and how we got here" companion to:

- [`COMPATIBILITY.md`](./COMPATIBILITY.md) — the frozen wire/JWT/DB/crypto contracts the Go code must honor.
- [`TESTING.md`](./TESTING.md) — how to run the test tiers (smoke / regression) today.
- `deployment/compare/README.md` (local-only) — how to diff Go against the **real PHP** backend.

> Goal of the whole effort: swap the Symfony backend for Go **without touching the
> frontend, the database, already-issued JWTs, stored password hashes, or
> AES-encrypted data**. Three contracts were frozen and had to stay byte-exact:
> the JSON response envelope, RS256 JWTs (claims/TTL/keypair), and the DB schema +
> stored hashes + encrypted emails. See `COMPATIBILITY.md`.

---

## Snapshot (current state)

- **9 resource modules** ported as full vertical slices: `account` (+folders),
  `budget` (+envelopes), `category`, `connection`, `currency`, `payee`, `tag`,
  `transaction`, `user` (+auth).
- **~218 Go source files**, **~85 test files**.
- **Two databases** supported from one binary (`DATABASE_DRIVER`): SQLite
  (default) and PostgreSQL, via per-engine sqlc adapters.
- **Test tiers:** smoke (unit + sqlite, no deps) and regression (smoke +
  sqlite-vs-PostgreSQL parity, incl. a full HTTP API-parity suite). See
  `TESTING.md`.
- **Architecture:** idiomatic hexagonal — `internal/{domain,app,infra,ui}` —
  stdlib-first (net/http router, hand-written validation, `flag` CLI), with a
  deliberately small dependency set.

---

## Phase 1 — The initial drop-in rewrite

**Commit:** `5d505bc` *feat: Add drop-in Go backend rewrite of the Symfony API*

The foundational port: the `go/` skeleton, config loading, the `Backend`
interface + registry (SQLite + PostgreSQL linked into one binary, chosen at
runtime), sqlc-generated queries per engine, the custom migration runner over the
frozen baseline schema, the full middleware chain (requestid → recover → cors →
timezone → jwt), the `net/http.ServeMux` router, the SPA file server, and the
auth crypto (Symfony-compatible sha512/500-round password hasher, the
md5-identifier + AES-128-CBC `EncodeService`, RS256 JWT sign/verify). All ~84
`/api/v1/...` endpoints across the 9 modules were implemented as vertical slices
(domain → repo → app service → handler), following the `user` module as the
reference pattern.

The non-negotiable here was **byte-compatibility**, captured in
`COMPATIBILITY.md`: the response envelope shape, the JWT claim set + 30-day TTL,
and the existing keypair/hashes/encrypted emails all had to keep working.

---

## Phase 2 — Side-by-side parity bug hunt (vs the real PHP backend)

Once the Go backend ran, it was diffed against the **live PHP backend** serving an
identical seed DB. A local-only comparison harness was built for this (kept out of
git — see `deployment/compare/README.md`):

- `apicompare` — walks every **read-only** GET and diffs the full response body.
- `mutatecompare` + `mutate_compare.sh` — for **write** endpoints: per case, give
  both backends a fresh copy of the seed, POST an identical body, diff the
  response, then diff the resulting **state** via a follow-up read.

This surfaced a long tail of real divergences, each fixed and (where applicable)
locked with a regression test:

| Commits | What was fixed |
|---------|----------------|
| `1810bfe` | First batch of parity bugs found by the side-by-side comparison |
| `4c78708` | **Login envelope** returned raw `{token,user}` (PHP/SPA expect it un-wrapped); **budget empty `children`** emitted `null` instead of `[]` |
| `b4adafa`, `6979392`, `a3a2a2a`, `40b2e47`, `404a61e` | **Budget builder** parity: account set, rate period selection, average-rate conversion, element `budgeted` (limits), spending float formatting, first-of-month date-boundary drop, rate-month snapping, summarized-limit precision |
| `1d431ea` | Mutation-compare harness + **category** write endpoints (notably: create must mint a server-side id and use the request id only as the idempotency key) |
| `a2eba91` | **tag** & **payee** write endpoints |
| `ac1ec19` | **account** & **folder** write endpoints (all 10 byte-match) |
| `2e68867` | **user-settings** & **transaction** write endpoints |
| `ea90a93` | **budget** write endpoints; added budget + connection mutation cases |

Recurring root causes worth remembering (all now fixed): create endpoints were
echoing the request id as the entity id instead of generating a fresh one; nil
slices marshaled to `null` where PHP emits `[]`; timestamps were persisted in
local time instead of UTC; and SQLite's `datetime()` cannot parse the RFC3339
string modernc serializes `time.Time` to (so timestamp columns must be bound as
`Y-m-d H:i:s` strings).

---

## Phase 3 — EconumoCloudBundle connection routes

**Commits:** `9f35130`, `853f5c8`

The open-source PHP `EconumoBundle` stubs four connection controllers (invites +
delete-connection) as `501`; the proprietary `EconumoCloudBundle` overrides them
with real implementations. Those four routes were ported to Go (generate/delete
invite, accept invite, delete connection — with cascade revocation of shared
budget access), and added to the mutation-comparison loop so they're checked
against the live PHP behavior like everything else.

---

## Phase 4 — A real, committed test suite

The comparison harness proved parity but is local-only and needs the PHP backend
running. To let the codebase be refactored fearlessly, a proper Go test suite was
built (this is what `TESTING.md` documents):

| Commits | What was added |
|---------|----------------|
| `7a01ef3` | Shared `testutil` DB harness + domain & repository test suites; fixed a **set-limit period bug** the new repo tests caught (a `time.Time` bound as RFC3339 read back as `budgeted=0`) |
| `d6fdab4` | +91 edge-case tests across app / handler / middleware / router / httpx |
| `7417262` | **sqlite-vs-pgsql engine-comparison suite** + Makefile coverage gates + GitHub Actions CI |
| `a179879` | Split the suites into **smoke** (unit + sqlite, no deps) and **regression** (+ engine comparison) tiers |
| `701de10` | Renamed `go-smoke` → `go-test` (bare run became `go-test-fast`) |

---

## Phase 5 — Housekeeping: structure, secrets, local-only tooling

| Commits | What changed |
|---------|--------------|
| `82e429a` | Consolidated standalone test code into dedicated trees |
| `65cb720` | **Removed personal-DB credentials** from tracked test/dev files (synthetic values substituted) |
| `c110fc4` | **Untracked the PHP↔Go comparison tooling** (`go/cmd/devtools/`, `deployment/compare/`) — kept local-only, gitignored. The committed equivalent is the engine-comparison suite. |

> Note: the credentials were scrubbed from the working tree, but two earlier
> commits (`1d431ea`, `1810bfe`) still contain them in **history**. Rotating the
> AES salt is the safe follow-up if that history is sensitive.

---

## Phase 6 — Comprehensive API-level engine parity

**Commit:** `356f2fd` *test(go): comprehensive sqlite-vs-pgsql API-parity suite*

The original engine comparison had only 3 repo-level scenarios. This phase made it
comprehensive at the **HTTP wire level**: the production module wiring was
extracted into `internal/server.BuildAPI` (so production and tests build the
**identical** router), and an `enginecompare` harness now stands that real handler
up over **both** SQLite and PostgreSQL from an identical seed, replays every read
endpoint + a write→read sequence per module, and asserts **byte-identical**
responses (SQLite is the reference).

It immediately earned its keep — it caught a real bug: `users_options` came back
in **different orders** on the two engines because the query ordered only by
`created_at`, which is identical across all four options at registration. Fixed by
adding a deterministic `ORDER BY created_at, id` tiebreak in both engines'
queries.

---

## Phase 7 — Test-infrastructure cleanup

**Commit:** `34be838` *test(go): centralize JWT keypair + replace raw-SQL seeding
with a fixture builder*

Two test-only quality improvements (no production code changed):

1. **JWT keypair** moved out of `infra/auth/testdata` into a dedicated
   `internal/test/testkeys` package, exposed via an embed-backed helper
   (`testkeys.Paths(t)`), so no test reaches it through fragile
   `../../../infra/auth/testdata/*.pem` relative paths. The auth package's own
   golden vectors stayed put (Go's `testdata` convention).
2. **Typed fixture builder** (`internal/test/fixture`) replaced the hand-written
   `INSERT INTO ...` statements that were duplicated across ~33 test files. It
   owns every insert in one place with the engine-portability gotchas baked in
   (`?`→`$N` rebinding, `TRUE`/`FALSE` booleans, `time.Time`→`Y-m-d H:i:s`), so
   tests read as intent: `f.User(...).Account(...).Transaction(...)`.

The test-support packages now live under `internal/test/`: `dbtest` (migrated DB
harness), `fixture` (the builder), `testkeys` (the shared keypair), and
`enginecompare` (the sqlite-vs-pgsql suite, build-tagged).

---

## Phase 8 — Dev-environment portability

**Commit:** `ae3aef5` *Run Makefile docker commands as host user via compose v2
plugin*

Moving development from macOS (Docker Desktop auto-remaps uids) to a native-Linux
homelab surfaced a bind-mount uid mismatch: the container's `www-data` (uid 82)
couldn't write the host-owned tree, breaking `composer install` and tripping
git's "dubious ownership" check. Fixed host-side in the Makefile — `DC_EXEC` runs
container commands as `$(id -u):$(id -g)` with `HOME=/tmp` — so the bind-mount
stays writable by the container **and** editable by the host user, with no change
to the (production-shared) Docker image.

---

## Phase 9 — Deterministic budget structure ordering

**Commit:** `82bb388` *fix(go): make budget structure ordering deterministic*

A re-run of the local `apicompare` harness against the live PHP backend surfaced
a **flaky** `get-budget` diff: the same budget returned its `structure.elements`
in a different order on every request. Root cause was the same class of bug as
Phase 6's `users_options`, but one layer up in the builder: the structure
elements are accumulated by iterating Go **maps** (tags, standalone categories,
and tag children — `builder_structure_build.go`), whose iteration order Go
randomizes, and the final sort keyed on `position` **only**. Many elements share
a position, so ties kept the random map order and the response varied run-to-run.

Fixed by sorting parents and folders by **position then id**, and children by
**id** (children carry no position). This is determinism, not PHP-order parity:
PHP breaks position ties by a different key (e.g. `name`), so the Go order is now
stable but intentionally need not match PHP — the frontend reorders lists when it
needs a specific presentation order. Locked with a `sortByPositionThenID` unit
test.

> Known, accepted divergence: the read-only list endpoints (`category`, `tag`,
> `payee`, `account`-folders, `user`-options, `connection`-sharedAccounts) are
> each **deterministic** on Go but ordered by a different tiebreak than PHP. They
> show as stable `apicompare` diffs and are left as-is for the same reason — the
> frontend owns presentation order. They are *not* flakiness; only the budget
> structure was non-deterministic, and that is now fixed.

---

## How verification works today

- **Everyday:** `make go-test` — smoke tier (build + vet + gofmt + unit + sqlite
  tests + coverage gate). No external dependencies.
- **Before merge/release:** `make go-regression` — smoke + the sqlite-vs-PostgreSQL
  engine-parity suite (repo-level + full HTTP API parity) against a real Postgres.
- **Against the real PHP backend (optional, local-only):** the `apicompare` /
  `mutatecompare` harness — see `deployment/compare/README.md`.

CI (`.github/workflows/go-tests.yml`) runs smoke on every push/PR and regression
against a `postgres:17` service container.

See `TESTING.md` for the full command reference.

---

## Key decisions captured along the way

- **One binary, two engines, runtime-selected** — no Go plugins (the toolchain
  lock-step makes them illusory); both DB backends are linked and chosen by
  `DATABASE_DRIVER`.
- **sqlc for compile-checked SQL** — a wrong column/arg fails `go build`; per-engine
  query variants only where the dialects genuinely diverge.
- **stdlib-first** — `net/http.ServeMux` routing, plain `func(http.Handler)
  http.Handler` middleware, hand-written `Validate()` (no tag DSL), `flag` CLI.
  Third-party deps only where stdlib can't deliver (decimal, JWT, DB drivers,
  uuid, sqlc).
- **No assembler layer** — the app service builds and returns the result DTO
  directly; any entity→DTO mapping is trivial field-copy.
- **SQLite is the reference engine** in every parity comparison (it's the default
  and the migration target); PostgreSQL must match it byte-for-byte.
```
