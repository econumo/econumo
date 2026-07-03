# Phase 4: Interface Pruning + ports.go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the three true pass-through adapters, prune ~24 intra-feature single-implementation interfaces to concrete types, and materialize each feature's `ports.go` — leaving only load-bearing interfaces: cross-feature ports, sqlc querier seams, genuine polymorphism, and the one real test seam.

**Architecture:** Directly from the Phase-4 audit (2026-07-02, embedded below). Pruning = replace an interface-typed field/param with the concrete pointer type, delete the interface + its `var _` assertion; consumers keep calling the same methods. Pass-through deletion = wire the wrapped concrete directly (structural typing). `ports.go` = pure intra-package file moves of the surviving cross-feature port declarations.

**Spec:** `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md` (Phase 4). Exemptions honored: cross-feature ports, sqlc queriers, `currency.RateProvider` (real test seam: `fakeRates`/`flexRates`), `httpx.Validator`, `httpx.errorRecorder` (cycle-breaker), `mailer.Mailer`, `backend.{DBTX,Backend}`, `router.Pinger` (method rename), god repos (`account.Repository`+`FolderRepository`, `budget.Repository` — Phase 6).

## Global Constraints

- **Behavior-identical**: wiring and type-level changes only; same methods called on the same instances. Goldens byte-identical; OpenAPI byte-identical (`git status --short internal/test/apiparity/testdata internal/ui/apidoc/docs` empty after every task); never UPDATE_GOLDEN.
- `make test` green per commit (gate 72, guard 84, archtest); tagged suite tail green; gofmt clean.
- NEVER prune: anything in the exemption list above. If a prune candidate turns out to have a hidden consumer or fake, KEEP it and report — don't force.
- Branch model: base `refactor/feature-packages`, branch per task (`p4/task-NN-…`); merge to `golang` at phase end after `make regression`.

---

### Task 1: Delete the three pass-through adapters + the anonymous clock

**Files (from the audit):**
- `internal/transaction/repo/adapters.go` — delete `AccountGrants`/`NewAccountGrants`/`accountWriteGranter` (lines ~26-45) and `VisibleAccounts`/`NewVisibleAccounts`/`visibleAccountsPort` (~20, 48-57). Signatures verified identical: `HasWriteGrant(ctx, accountID, userID vo.Id) (bool, error)` ≡ `connectionrepo.AccountAccessResolver.HasWriteGrant`; `VisibleAccountIDs(ctx, userID vo.Id) ([]vo.Id, error)` ≡ `(*account.Service).VisibleAccountIDs`.
- `internal/connection/repo/adapters.go` — delete `OptionPort`/`NewOptionPort`/`optionRepo` (~79-100). `MaxPosition`/`SavePosition` ≡ `*accountrepo.Repo`'s methods.
- `internal/server/server.go` — rewire: pass `accountAccessResolver` directly where `NewAccountGrants(...)` was (~:134), `accountSvc` directly where `NewVisibleAccounts(...)` was (~:135), `accountRepo` directly where `connectionrepo.NewOptionPort(accountRepo)` was (~:117). Harness tests that wired these adapters need the same rewire (grep for the constructors).
- `internal/server/glue_budget_userlookup.go` — replace the anonymous `clock interface{ Now() time.Time }` (lines ~27, 33) with `port.Clock` (import `internal/shared/port`).

- [ ] **Step 1:** Make the deletions + rewires; `grep -rn 'NewAccountGrants\|NewVisibleAccounts\|NewOptionPort' internal --include='*.go'` → empty.
- [ ] **Step 2:** `CGO_ENABLED=0 go build ./... && go vet ./...`; `make test`; tagged tail; goldens/docs status empty.
- [ ] **Step 3:** Commit `refactor: delete pass-through adapters; wire concretes directly`.

---

### Task 2: Prune single-impl interfaces — currency, category, tag, payee, user

For each: replace the `Service`/`ReadService`/`WriteService` struct field (and `New*` constructor param) with the concrete pointer type; delete the interface decl + any `var _` assertion; requalify references. Constructors' call sites (server.go, harness tests, cli/container.go where applicable) already pass the concretes — only signatures change.

**Prune list (audit category C — verified no test fakes):**
- `currency.WriteModel` (admin.go:20) → `*currencyrepo.Repo`; `currency.ReadModel` (read.go:15) → `*currencyrepo.ReadRepo`. KEEP `currency.RateProvider` (test seam).
- `category.ReadModel` (read.go:18) → `*categoryrepo.ReadRepo`; `category.Repository` (repository.go:12 — delete the file if empty after) → `*categoryrepo.Repo`.
- `tag.ReadModel` (read.go:10) → `*tagrepo.ReadRepo`; `tag.Repository` (repository.go:12) → `*tagrepo.Repo`.
- `payee.ReadModel` (read.go:11) → `*payeerepo.ReadRepo`; `payee.Repository` (repository.go:12) → `*payeerepo.Repo`.
- `user.Repository` (repository.go:13) → `*userrepo.Repo`; `user.ReadModel` (read.go:24) → `*userrepo.ReadRepo`; `user.PasswordRequests` (usecase.go:63) → `*userrepo.PasswordRequestRepo`; `user.passwordHasher` (usecase.go:45) → `*auth.PasswordHasher`; `user.encoder` (usecase.go:50) → `*auth.EncodeService`; `user.jwtIssuer` (usecase.go:57) → `*jwt.JWT`; `user.ResetMailer` (usecase.go:76) → `*mailer.ResetSender`.

Note on archtest: features importing `internal/infra/auth`, `internal/infra/mailer`, `internal/shared/jwt` is feature→leaf/kernel — allowed. Verify archtest stays green.

- [ ] **Step 1:** Prune per list, feature by feature, compiling between features.
- [ ] **Step 2:** Sweep: `grep -rn 'type \(WriteModel\|ReadModel\|Repository\|PasswordRequests\|passwordHasher\|encoder\|jwtIssuer\|ResetMailer\) interface' internal/currency internal/category internal/tag internal/payee internal/user --include='*.go'` → empty. Empty `repository.go` files deleted.
- [ ] **Step 3:** `make test`; tagged tail; goldens/docs clean; archtest green.
- [ ] **Step 4:** Commit `refactor: prune single-impl interfaces (currency/category/tag/payee/user)`.

---

### Task 3: Remaining prunes — connection/repo.accountAccessFull + middleware.TokenVerifier

> **Amended after Task 2:** the audit's root-declared repo-interface prune
> candidates (`<feature>.Repository`/`ReadModel`/`PasswordRequests`/`WriteModel`,
> `budget.ReadModel`, `connection.*Repository`, `transaction.Repository`/
> `ExportLookup`/`Importer`) are IMPOSSIBLE to prune: each feature's `repo/`
> subpackage imports the feature root for entity types, so the root holding a
> `*<feature>repo.X` concretely is a Go import cycle (proven empirically in
> Task 2). These interfaces are the spec's DESIGNED intra-feature seam — the
> target shape's `repository.go` — and are hereby reclassified KEEP-by-design
> (spec amended to say so). Phase 4 pruning applies only to seams not serving
> the root←repo direction.

**Prune list (what actually remains):**
- `connection/repo.accountAccessFull` (repo/adapters.go:21) → fold `AccountAccessResolver` to hold `*Repo` directly (same package — no cycle).
- ~~middleware.TokenVerifier~~ — KEPT: `stubVerifier` in middleware_test.go is a real structural test seam (injects claims/errors in 6 JWT tests); the audit missed it (fake never names the interface). Spec rule honored.
- (original text follows, superseded:) `middleware.TokenVerifier` (ui/middleware/auth.go:18) → `*jwt.JWT` — touches `middleware.JWT(...)` and all 9 `internal/<feature>/api/routes.go` `RegisterAPI` signatures (they thread the verifier). Verify no fake implements TokenVerifier anywhere in tests first (audit says none; re-verify: `grep -rn 'TokenVerifier' internal --include='*_test.go'`). middleware (ui leaf) importing shared/jwt (kernel) — allowed. If a routes.go signature turns out not to thread it (wired otherwise), adapt minimally and report.

- [ ] **Step 1:** Prune per list, compiling per feature.
- [ ] **Step 2:** Sweep greps analogous to Task 2 (+ `TokenVerifier` gone); empty `repository.go` files deleted.
- [ ] **Step 3:** `make test`; tagged tail; goldens/docs clean; archtest green.
- [ ] **Step 4:** Commit `refactor: prune single-impl interfaces (budget/connection/transaction) + TokenVerifier`.

---

### Task 4: Materialize ports.go per feature

Pure intra-package file moves: relocate each feature's surviving cross-feature port declarations (audit category A that live in the feature root) into `internal/<feature>/ports.go` with a standard header comment:

```go
// Ports: the consumer-side interfaces this feature declares for capabilities
// other features provide. Implementations are wired in internal/server —
// features never import each other (enforced by internal/test/archtest).
```

**Moves (post-prune inventory):**
- account: `CurrencyLookup`, `UserLookup`, `SharedAccessLookup`, `AccessRevoker` (from usecase.go)
- budget: `UserLookup`, `AccountLookup`, `CurrencyLookup` (usecase.go), `MetadataLookup`, `Convertor`, `AverageRateLookup` (builder.go)
- category: `AccountAccess` (usecase.go); tag: `AccountAccess`; payee: `AccountAccess`
- connection: `BudgetAccessRevoker` (invite_usecase.go), `UserLookup`, `FolderPort`, `OptionPort` (usecase.go)
- transaction: `UserLookup`, `AccountResolver`, `VisibleAccounts`, `AccountGrants` (usecase.go)
- user: `CurrencyLookup`, `BudgetExistence` (usecase.go)
- currency: none (it's a producer) — no ports.go.

Move each decl WITH its doc comment verbatim; no signature changes. Update CLAUDE.md's feature-shape line to include `ports.go` (the spec's target shape, now real).

- [ ] **Step 1:** Move per list; `go build ./...` after each feature.
- [ ] **Step 2:** Verify: every `internal/<feature>/ports.go` exists per the list; `make test`; goldens/docs clean.
- [ ] **Step 3:** Commit `refactor: materialize ports.go in each feature package`.

---

### Task 5: Phase checkpoint (controller)

- [ ] `make test` + `make regression` both PASS; final review; merge to `golang`.

---

## Verification checklist (end of phase)

- [ ] Zero pass-through adapters (the 3 named gone); ~24 pruned interfaces gone; ports.go in 8 features; RateProvider + exemptions intact.
- [ ] Goldens + OpenAPI byte-identical across the phase; `make regression` green.
- [ ] Branch merged into `golang`.
