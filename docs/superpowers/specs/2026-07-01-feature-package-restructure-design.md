# Feature-Package Restructure & Post-Migration Cleanup

**Date:** 2026-07-01
**Status:** Approved design, pending implementation plan
**Branch:** `refactor/feature-packages` (from `golang`)

## Problem

The Go backend was ported from a PHP/Symfony hexagonal application, largely
preserving the PHP architecture. Now that the migration bugs are fixed and the
test suite is trusted, the structure should be reorganized for long-term
maintainability as idiomatic Go. A survey of the codebase identified the
inherited pain points:

1. **Interface + adapter proliferation** — 135 interface declarations for ~73
   constructors, most with exactly one implementation. `Clock` is declared 8
   times, `TxRunner` 8 times, `OperationGuard` 5 times (once per app package).
   A layer of pass-through adapter structs (`infra/repo/connection/adapters.go`,
   `infra/repo/account/lookups.go`) mostly forwards calls verbatim.
2. **Handler boilerplate** — ~90 handlers repeat the same ~17-line
   decode/validate/call/respond ritual; `requireUser` is copy-pasted into all
   9 handler packages.
3. **PHP-style entities** — private fields plus a wall of one-line getters on
   every aggregate; positional `FromState(...)` constructors with 8–10 args
   (except `transaction`, which already uses the better struct-based `NewState`).
4. **God repository interfaces** — `domain/budget` has 33 methods,
   `domain/account` 21 (+ an 11-method `FolderRepository`).
5. **Horizontal slicing** — one feature is spread across four packages
   (`domain/X`, `app/X`, `infra/repo/X`, `ui/handler/X`), so routine changes
   touch four directories.

Worth keeping, explicitly: the sqlc engine-adapter pattern, the `httpx`
envelope/decode helpers, `server.BuildAPI` as the single composition root used
by both production and the enginecompare suite, and the shared kernel
(`vo`/`errs`/`datetime`).

## Decision summary

- **Restructure to feature packages**: dissolve the horizontal `domain`/`app`/
  `infra/repo`/`ui/handler` layers into one directory per feature.
- **Shape**: each feature is a root package (entity + use-cases + DTOs +
  interfaces) with two sub-packages, `repo/` (persistence) and `api/` (HTTP).
- **Dependency rule**: feature packages never import each other ("ports
  everywhere"); all cross-feature needs remain consumer-side interfaces wired
  in `internal/server`.
- **Sequencing**: move first (behavior-identical relocation), clean later
  (idiomatic cleanups as follow-up phases in this same spec).
- **`pkg/jwt`** moves to `internal/shared/jwt`; the `pkg/` directory disappears.
- **sqlc, storage, and migrations do not move** and their configuration is not
  touched anywhere in this effort.

## Target layout

```
internal/
├── account/           ─┐
├── budget/             │
├── category/           │
├── connection/         │  9 feature packages
├── currency/           ├─ (shape below)
├── payee/              │
├── tag/                │
├── transaction/        │
├── user/              ─┘
├── shared/            leaf kernel packages: vo/, errs/, datetime/, jwt/
├── reqctx/            request-scoped context values (from app/reqctx)
├── ui/                HTTP edge machinery only: httpx, middleware, router, spa, apidoc
├── infra/             technical adapters only: auth, mailer, clock, storage,
│                      operation (idempotency guard), openexchangerates
├── server/            composition root + cross-feature glue adapters
├── cli/               unchanged
├── config/            unchanged
├── logging/           unchanged
└── test/              unchanged (dbtest, fixture, testkeys, enginecompare)
```

`internal/domain`, `internal/app`, and `pkg/` disappear entirely.
`internal/infra/repo` disappears (its contents move into the features).

### Feature homes for orphan repos

- `infra/repo/passwordrequest` → `internal/user/repo/` (part of the user feature).
- `infra/repo/userbudget` → `internal/budget/repo/` (part of the budget feature).
- `infra/repo/operation` → `internal/infra/operation/` — it is a deliberately
  shared idempotency guard over `operation_requests_ids`, used by many
  features; it stays shared infrastructure, not a feature.

### Feature package shape

```
internal/category/                  package category — the feature's core
├── entity.go                       entity + invariants (moves unchanged from domain/)
├── usecase.go                      use-case struct + deps (was app/…/service.go)
├── create.go / update.go / …       per-verb use-cases (unchanged granularity)
├── dto.go                          request/result DTOs
├── repository.go                   the persistence interface the use-cases consume
├── ports.go                        consumer-side cross-feature ports
├── entity_test.go, …               entity/use-case tests
│
├── repo/                           package repo — persistence implementation
│   ├── repo.go                     querier iface + method bodies
│   │                               (implements category.Repository)
│   ├── read.go                     (where the feature has one today)
│   ├── sqlite.go                   engine adapter (native passthrough)
│   ├── pgsql.go                    engine adapter (conversion shim)
│   └── *_integration_test.go
│
└── api/                            package api — HTTP edge
    ├── handler.go                  Handlers struct
    ├── routes.go                   RegisterAPI
    ├── <endpoint files>            (current per-endpoint files, unchanged)
    └── endpoint/harness tests
```

Naming conventions:

- `entity.go` (singular; moves as-is — the domain files already use this name).
  Features with extra domain files keep them (e.g. budget's `valueobject.go`).
- `usecase.go` is the entrypoint file holding the use-case struct, constructor,
  and shared helpers. Struct names (`Service`, `ReadService`, …) are NOT renamed
  at move time; renaming is a Phase 6 decision.
- `repository.go` (root) is the interface; `repo/` is the implementation. This
  replaces today's confusing `domain/X/repository.go` vs `infra/repo/X/repo.go`
  split-by-layer with a split-by-role that the compiler enforces.
- Use-cases live in the root package WITH the entity, not in a `usecases/`
  sub-package — a separate package would resurrect the domain/app split and
  force the entity's internals to be exported across a boundary. Entity +
  business logic together is the point of the merge.
- Every feature's sub-packages are named `repo` and `api`. The composition
  root aliases them on import (`categoryrepo "…/category/repo"`); that is the
  only place that imports many at once.

Import direction within a feature, compiler-enforced:
`repo` → root ← `api`; the root imports neither.

## The dependency rule

**Feature packages never import each other.** The merged post-move dependency
graph would contain real cycles (`connection → budget` for access revocation
while `budget → category/payee/tag → connection` for access checks), so
cross-feature imports are banned uniformly rather than judged case-by-case:

- Every cross-feature need is a small consumer-side interface in the consuming
  feature's `ports.go` (this is already the app layer's style today).
- Implementations are wired in `internal/server`. Existing cross-feature
  adapter structs (e.g. `infra/repo/connection/adapters.go`, which wrap another
  feature's repo and return the consumer's view types) move to
  `internal/server/adapters.go` — the composition root already imports
  everything one-way, so no cycle is possible there.
- Shared leaf packages (`shared/*`, `reqctx`, `ui/httpx`, `infra/*`) may be
  imported by any feature and import no features.

## Phases

Phases 1–2 are behavior-identical by construction (file moves, package renames,
import rewrites, and mechanical symbol renames only). Any real code change
belongs to Phases 3–6.

### Phase 1 — Prep (mechanical)

- `internal/domain/shared/{vo,errs,datetime}` → `internal/shared/{vo,errs,datetime}`.
- `pkg/jwt` → `internal/shared/jwt` (importers: `cmd/econumo/main.go`,
  `internal/app/user`, `internal/cli`, `internal/server`, `internal/ui/middleware`).
- `internal/app/reqctx` → `internal/reqctx` (used by features and middleware).
- `infra/repo/operation` → `internal/infra/operation`.
- Update CLAUDE.md's architecture section and jwt references in the same commit.

### Phase 2 — The move (mechanical, one commit per feature)

Merge each feature's four slices into `internal/<feature>` with the shape
above, leaf-most first:

`user` → `currency` → `account` → `category` → `tag` → `payee` →
`transaction` → `connection` → `budget`

Per feature commit:

- `domain/X/entity.go` → `X/entity.go`; `domain/X/repository.go` → `X/repository.go`.
- `app/X/*.go` → `X/*.go` (`service.go` renamed `usecase.go`; per-verb files keep
  their names; consumer-side port declarations consolidate into `ports.go`).
- `infra/repo/X/*` → `X/repo/*`.
- `ui/handler/X/*` → `X/api/*`.
- Cross-feature adapters found in the feature's old infra package →
  `internal/server/adapters.go`.
- `passwordrequest` moves with `user`; `userbudget` moves with `budget`.
- Tests move alongside their code in the same commit.
- Symbol collisions from the merge are resolved by renaming the DTO or other
  internal symbol — never the entity, and never anything on the wire.
- `make test` green after every commit; `make regression` at phase end.

### Phase 3 — Cross-cutting dedup

- One `Clock`, one `TxRunner`, one `OperationGuard` interface in
  `internal/shared` (removes ~25 duplicate declarations; structural typing
  means features stay decoupled).
- Shared `requireUser` helper in `httpx` (or middleware) replacing 9 copies.

### Phase 4 — Interface pruning (within features only)

- Delete pass-through adapters whose wrapped method already matches the port
  signature (Go structural typing needs no adapter there).
- Prune single-implementation interfaces *within* a feature; depend on the
  concrete type unless a real test seam needs the interface.
- Cross-feature ports are exempt — they are load-bearing under the dependency
  rule. The sqlc `querier` seam is exempt — it is the deliberate engine-adapter
  pattern.

### Phase 5 — Handler de-boilerplating

- Generic `httpx.Handle[Req, Res]` collapsing the decode/validate/call/respond
  ritual (~17 lines × ~90 handlers) to ~1 line per route.
- Swag `@` annotations stay attached to small named wrappers; `make swagger`
  must keep producing an identical committed OpenAPI document.

### Phase 6 — Entity & repository idioms (one aggregate per commit)

- Exported fields + struct-based state constructors (extend the
  `transaction.NewState` pattern to all aggregates); delete getter walls;
  replace positional `FromState(...)` reconstructors.
- Split the god repository interfaces (`budget` 33 methods, `account` 21+11)
  into role-based interfaces declared at their consumers.
- Optional, opportunistic: consolidate one-file-per-verb splinters where files
  are trivially small; decide whether the use-case structs keep the `Service` /
  `ReadService` names or adopt a single consistent name (decision made at
  Phase 6 planning, applied uniformly across all features).

## Frozen guardrails (every phase)

- **Wire contract untouched**: response envelope, exact route paths/methods,
  datetime formats, validation strings, JWT claims. The `enginecompare` suite
  runs the real `server.BuildAPI` on both engines and must stay byte-identical —
  it is the primary safety net.
- **No SQL, sqlc config, generated-code, or migration changes.**
  `internal/infra/storage/**` (sqlc.yaml, `query/`, `gen/`, migrations,
  `backend`) does not move and is not edited.
- **Coverage gate holds** (`GO_COVER_MIN`); tests move with their code in the
  same commit, never dropped or skipped.
- `server.BuildAPI` remains the single composition root used by production and
  tests.

## Verification

- Every commit: `make test` (build + vet + gofmt + OpenAPI-fresh + sqlite
  tests + coverage gate).
- Every phase boundary: `make regression` (adds the sqlite-vs-PostgreSQL
  engine-comparison suite).
- Phase 5 additionally: committed OpenAPI document diff must be empty.

## Risks

- **Symbol collisions on merge** (entity vs DTO vs handler names now share a
  package). Mitigation: per-feature collision inventory before each move;
  rename only non-contract symbols.
- **Swagger generation**: swag scans source paths; its invocation must be
  checked against the new layout and the committed OpenAPI doc must stay
  identical (already enforced by `make test`'s docs-fresh check).
- **Import-path churn**: every file's imports change; anything in flight on
  other branches will conflict. Mitigation: dedicated branch
  (`refactor/feature-packages`), continuous push, no parallel feature work on
  `golang` during Phase 2.
- **Budget feature size**: even split root/`repo/`/`api/`, budget's root is
  ~19 files. Acceptable; Phase 6 consolidation shrinks it further.

## Out of scope

- Any behavior, SQL, or wire-format change.
- The web SPA (`web/`), deployment, CI workflows (beyond path updates if any
  reference moved packages).
- Renaming HTTP routes or reshaping DTO JSON.
