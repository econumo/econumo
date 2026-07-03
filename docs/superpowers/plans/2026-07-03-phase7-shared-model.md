# Phase 7: Shared Model Package Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks 2–8 follow **The Model-Move Procedure** below.

**Goal:** One flat `internal/model` leaf package holding every feature's entities, value objects, and DTOs (~274 types); twins merged into single definitions; features reduced to behavior (use-cases, ports, repository interfaces, repo/, api/).

**Architecture:** Per feature, leaf-most first: `git mv` the type files into `internal/model` (renamed `<feature>.go` / `<feature>_dto.go` etc.), package `model`; apply the collision map (merge twins / feature-prefix renames); qualify references compile-driven (`Category` → `model.Category`) in the feature, its repo/, api/, server glue, and tests. Twins' conversion glue that becomes identity/pass-through is deleted per the Phase 4 rule. `model` joins the archtest kernel.

**Spec:** the "Post-restructure evolution" section of `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md`.

## Global Constraints

- **Behavior-identical**: type relocation + qualification + twin merges (shape-identical only, verified) + wire-invisible renames. Mutator/Validate bodies byte-equivalent. Goldens byte-identical ALWAYS (JSON tags travel verbatim; a golden diff = drift = STOP).
- **OpenAPI**: definition keys become `model.X` per task — the committed docs CHANGE each task; regenerate + verify semantic identity (canonical `$ref`-remap compare, Phase 2 method) + commit. `SWAG_INIT` gains `,../../model` in Task 2 and keeps the feature dirs (api annotations live there).
- `make test` green per commit (gate 72, guard 84, archtest); tagged tail green; gofmt clean. `make regression` at phase end.
- Never rename anything wire-visible (JSON tags, aliases, validation strings, routes). Type renames are Go-identifier-only.
- Merging a twin REQUIRES a recorded shape proof (field names+tags+types+order identical) in the task report; if shapes differ, rename instead — never "fix" a shape to force a merge.
- Branch model: base `refactor/feature-packages`, branch per task (`p7/task-NN-…`); merge to `golang` at phase end after the final review.

## The Collision Map (from the 2026-07-03 inventory; Task 1 verifies shapes and finalizes)

**MERGE (expected twins — one survivor in model):** `AccountResult` family (account survives; transaction's copies + `toTransactionAccountResults` glue die), `ConvertItem` + `FullRate` (currency survives; budget's copies + `BudgetConvertor`/`BudgetAverageRateLookup` conversion bodies collapse — delete glue if it becomes identity), `CurrencyResult` (account vs currency — verify), `PositionChange` (category/payee/tag ×3 → one), `OwnerView` ×3 (account/budget/connection — verify shapes), `UserResult` ×2 (budget/connection — verify).

**RENAME (different types, same name):** `Type` → `AccountType` / `CategoryType` / `TransactionType`; `FolderResult` → `AccountFolderResult` / `BudgetFolderResult`; `GetTransactionListRequest` → `TransactionListRequest` (transaction's) / `BudgetTransactionListRequest` (budget's); `UpdateBudgetRequest`/`UpdateBudgetResult` → user's become `UpdateActiveBudgetRequest`/`Result` (user's endpoint sets the active budget — name by meaning), budget's keep the name. Task 1 may extend this map; record every addition.

## The Model-Move Procedure (Tasks 2–8; X = the feature)

**M1 — File inventory.** X's type files: the entity file(s) (`<X>.go`, plus folder.go/invite.go/password_request.go/valueobject.go where present), `dto.go`, view-row/readmodel type files. Behavior files (usecase, per-verb, ports, repository, convertor) STAY. Entity/DTO unit tests move with their types (become `package model` tests, renamed `<X>_test.go` etc. under model/).

**M2 — Move + rename.** `git mv internal/X/<file>.go internal/model/<X>[_dto].go`; package decl → `model`; apply the collision map for X (merges: delete the twin, keep survivor; renames: feature-prefix). Files keep per-feature naming inside model/.

**M3 — Qualify references compile-driven.** Repo-wide `go build ./...`; in X's remaining files + repo/ + api/ + server glue + tests, add the `model` import and qualify (`Category` → `model.Category`, using sed per known type name within the affected dirs, then compiler for residuals). NEVER edit logic while qualifying.

**M4 — Twin fallout (only when the map says so for X).** Delete the dead twin's conversion functions/glue; if a glue adapter becomes a pure pass-through (identity conversion), delete it and wire the concrete directly (Phase 4 rule; verify same instance). Ports that referenced the dead twin now reference the model survivor.

**M5 — Swagger.** `make swagger`; committed docs change (X's keys → `model.*`); verify semantic identity via canonical compare; commit docs with the task.

**M6 — Verify.** `make test`; tagged tail; `git status --short internal/test/apiparity/testdata` EMPTY; gofmt clean; archtest green.

**M7 — Commit** `refactor(model): move X types into internal/model` (+ separate commit for big twin-fallout if clearer).

---

### Task 1: Foundations — model package, archtest, shape audit

**Files:** Create `internal/model/doc.go`; modify `internal/test/archtest/archtest_test.go`; produce the FINAL collision map (report + plan-amendment if it changes).

- [ ] `internal/model/doc.go`:

```go
// Package model is the application's shared type universe: every feature's
// entities (with their invariant-preserving mutators), value objects, and
// request/result DTOs live here, named per feature file (account.go,
// account_dto.go, ...). Behavior stays in the feature packages — they import
// model; model imports only the shared kernel. One definition per concept:
// cross-feature reads use these types directly instead of structural copies.
package model
```

- [ ] Archtest: `isKernel` → `top == "shared" || top == "model"`; add `"model": true` to the infrastructure map and to `isLeaf`; update comments. Probe: temp import of `internal/config` from model/doc.go must FAIL the kernel rule; revert.
- [ ] Shape audit: for each MERGE candidate pair in the map, diff field lists (names+tags+types+order) via `git show`/reads; produce the verdict table (merge vs rename) in the report. Controller folds verdicts into the plan.
- [ ] `make test` green (model compiles empty); commit `feat(model): package skeleton + kernel rules + collision audit`.

### Task 2: user types (+ SWAG_INIT `,../../model`)
M-procedure, X = user (user.go, password_request.go, dto.go view rows in read.go? — M1 inventories; only TYPE declarations move, functions stay). Collision-map items: none expected for user beyond `UpdateActiveBudgetRequest`/`Result` rename. First swagger-key migration — establish the canonical-compare evidence format for later tasks.

### Task 3: currency types
X = currency (dto.go, ConvertItem/FullRate/RateInput/RateRow/FullRate etc. from convertor.go/admin.go — TYPES only; convertor behavior + names.go stay). `CurrencyResult` merge verdict applied.

### Task 4: account types (+ Folder)
X = account (account.go, folder.go, dto.go). Renames: `AccountType`, `AccountFolderResult`. Merges landing here: `CurrencyResult` (if account's dies), `OwnerView` survivor decision per audit.

### Task 5: category + tag + payee types
Three small moves, one commit each or one combined (implementer judgment). `CategoryType` rename; `PositionChange` ×3 → one model.PositionChange.

### Task 6: transaction types
X = transaction (transaction.go incl. NewState family, dto.go). `TransactionType`, `TransactionListRequest` renames; **AccountResult twin family dies** — DTOs point at model's; `toTransactionAccountResults` + the conversion in `glue_transaction_adapters.go` deleted if identity (M4; check whether `TransactionAccountResolver` becomes pass-through → Phase 4 rule).

### Task 7: connection types
X = connection (connection.go, invite.go, dto.go). `OwnerView`/`UserResult` merge verdicts applied.

### Task 8: budget types
X = budget (budget.go, valueobject.go, readmodel types, dto.go — the largest). `BudgetFolderResult`, `BudgetTransactionListRequest` renames; **ConvertItem/FullRate twins die** — `BudgetConvertor`/`BudgetAverageRateLookup` glue becomes identity → delete + wire currency's concretes directly if signatures align (M4 verify same instances); budget ports retype to model.

### Task 9: Teardown + checkpoint
- Sweep: zero type declarations left in feature roots except interfaces (ports/repository) and service structs (`grep -rE '^type [A-Z].*(struct|int16)' internal/{account,budget,category,connection,currency,payee,tag,transaction,user}/*.go` — survivors must be Service/ReadService structs only); no residual twins; glue count re-measured (expect meaningful shrink from M4).
- CLAUDE.md: architecture tree + feature-shape section rewritten for model/ (verify paths); dependency-rule paragraph gains model-as-kernel.
- `make test` + `make regression` PASS; final whole-branch review; controller merges to `golang`.

## Verification checklist (end of phase)
- [ ] `internal/model` holds all entities/VOs/DTOs; features are behavior-only; twins gone; renames per map.
- [ ] Goldens byte-identical across the phase; OpenAPI semantically identical (keys `model.*`); regression green; archtest green with model in the kernel.
- [ ] CLAUDE.md matches; branch merged into `golang`.
