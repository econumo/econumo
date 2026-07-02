# Phase 2: Feature-Package Moves Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every feature task (2–10) follows **The Feature-Move Procedure** below — the dispatcher hands implementers both their task section AND the procedure section.

**Goal:** Dissolve the four horizontal layers into nine flat-ish feature packages (`internal/<feature>` root + `repo/` + `api/`), one behavior-identical task per feature, leaf-most first, with cross-feature glue relocating to `internal/server`.

**Architecture:** Each feature move merges `domain/X` + `app/X` into the root package `internal/X`, renames `infra/repo/X` (package `xrepo`) to `internal/X/repo` (package `repo`), and `ui/handler/X` (package `x`) to `internal/X/api` (package `api`). Files whose imports reach into ANOTHER feature are cross-feature glue and move to `internal/server` instead. The Phase-0 safety net (goldens, parity, route guard) and the Phase-1 archtest verify every step; goldens must never change.

**Tech Stack:** `git mv`, `sed`, gofmt, the apiparity suite, archtest.

**Spec:** `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md` (Phase 2 + "Feature package shape" + "The dependency rule").

## Global Constraints

- **Behavior-identical**: moves, package renames, import rewrites, alias de-qualification, and collision renames of NON-CONTRACT symbols only. Never rename an entity, a JSON-tagged field, a validation string, or anything on the wire.
- **Goldens byte-identical every task** (`git status --short internal/test/apiparity/testdata/` empty; never UPDATE_GOLDEN). Route guard floor (84) and catalogue floor (33) must stay green — routes.go files keep their literal `"METHOD /path"` strings; the guard's globs already include `internal/*/api/routes.go`.
- `make test` green after every commit (gate 72), including swagger-check (the committed OpenAPI doc must stay identical), archtest, and the smoke suite. Tagged suite green on sqlite: `CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3`.
- Move order (leaf-most first, one task per feature): `user → currency → account → category → tag → payee → transaction → connection → budget`.
- `passwordrequest` moves with `user`; `userbudget` moves with `budget` (its `exists.go` imports the user feature → it is glue, see procedure).
- Existing tests move with their code; never deleted or weakened.
- Branch model: base `refactor/feature-packages`, feature branch per task (`p2/task-NN-…`), merged back after review; merge to `golang` at phase end after `make regression`.
- Deviation from the spec's target shape, recorded here: `ports.go` is NOT materialized during the move (consumer-side interface declarations stay in the files they're in). Phase 4's interface audit creates it. Everything else matches the spec shape (`entity.go`, `usecase.go` from `service.go`, per-verb files, `dto.go`, `repository.go`, `repo/`, `api/`).

---

## The Feature-Move Procedure (applies to Tasks 2–10; X = the feature)

**P0 — Inventory (before touching anything).** List the four slices and detect collisions:

```bash
ls internal/domain/X internal/app/X internal/infra/repo/X internal/ui/handler/X
# exported symbols that would collide when domain/X and app/X merge:
go doc ./internal/domain/X 2>/dev/null | grep -oE '^(func|type|const|var) [A-Z][A-Za-z0-9]*' | sort > /tmp/dom.txt
go doc ./internal/app/X    2>/dev/null | grep -oE '^(func|type|const|var) [A-Z][A-Za-z0-9]*' | sort > /tmp/app.txt
comm -12 /tmp/dom.txt /tmp/app.txt
```

Any collision: rename the APP-side symbol (DTOs/services are internal; the domain entity's name and anything JSON-tagged is untouchable). Also check unexported top-level collisions:

```bash
grep -hoE '^(func|type|const|var) [a-z][A-Za-z0-9]*' internal/domain/X/*.go internal/app/X/*.go | sort | uniq -d
```

(non-empty output = collision; note that methods `func (r *T) name` don't match this pattern and can't collide across types — only true top-level dupes matter). Record every rename in the report.

**P1 — Identify glue.** For each non-test file in `internal/infra/repo/X`: if it imports `internal/app/Y`, `internal/domain/Y`, or `internal/Y` for any OTHER feature Y, it is cross-feature glue → destination `internal/server/glue_X_<origname>.go` (package `server`), NOT `internal/X/repo`. Same-feature imports (e.g. `passwordrequest/repo.go` → `domain/user` while moving INTO the user feature) are not glue. Glue symbol collisions against existing `package server` symbols (several features contribute `UserLookup`-style adapters): prefix with the source feature (`accountUserLookup`, `NewAccountUserLookup`), updating the only call sites (server.go wiring).

**P2 — Move the four slices** (git mv preserves history):

```bash
mkdir -p internal/X
git mv internal/domain/X/<each file> internal/X/
git mv internal/app/X/<each file> internal/X/          # service.go arrives as usecase.go:
git mv internal/X/service.go internal/X/usecase.go 2>/dev/null || true
git mv internal/infra/repo/X internal/X/repo            # glue files then move on to server:
git mv internal/X/repo/<glue file> internal/server/glue_X_<origname>.go
git mv internal/ui/handler/X internal/X/api
rmdir internal/domain/X internal/app/X 2>/dev/null || true
```

If a domain file and an app file share a NAME (e.g. both have `dto.go` — rare), rename the app one during the mv (report it).

**P3 — Package renames and import rewrites.**

```bash
# package statements:
sed -i 's/^package Xrepo$/package repo/' internal/X/repo/*.go
sed -i 's/^package X$/package api/' internal/X/api/*.go          # handler package was named X
# glue files become package server:
sed -i 's/^package Xrepo$/package server/' internal/server/glue_X_*.go
# import paths, repo-wide (covers tagged files):
grep -rl 'econumo/internal/domain/X"' --include='*.go' . | xargs sed -i 's|econumo/internal/domain/X"|econumo/internal/X"|g'
grep -rl 'econumo/internal/app/X"' --include='*.go' . | xargs sed -i 's|econumo/internal/app/X"|econumo/internal/X"|g'
grep -rl 'econumo/internal/infra/repo/X"' --include='*.go' . | xargs sed -i 's|econumo/internal/infra/repo/X"|econumo/internal/X/repo"|g'
grep -rl 'econumo/internal/ui/handler/X"' --include='*.go' . | xargs sed -i 's|econumo/internal/ui/handler/X"|econumo/internal/X/api"|g'
```

Because the repo package is now named `repo` (was `Xrepo`) and the api package `api` (was `X`), every importer needs an alias preserving its existing identifier — add `Xrepo` / `Xapi` aliases on those import lines (e.g. `userrepo "github.com/econumo/econumo/internal/user/repo"`), so code references (`userrepo.NewRepo`, wiring calls) stay textually unchanged. Do the same inside moved test files.

**P4 — De-qualify domain references in the merged root.** App files referenced domain types via an import alias (commonly `domX`). In each `internal/X/*.go` file that imports `econumo/internal/X"` (now a self-import): note its alias, delete the import line, strip the `alias.` qualifier:

```bash
grep -l 'econumo/internal/X"' internal/X/*.go            # self-imports to fix
# per file: remove the import line, then e.g.
sed -i 's/\bdomX\.//g' internal/X/<file>.go
```

Watch for qualifier collisions the P0 inventory predicted; apply the recorded renames.

**P5 — Format and verify.**

```bash
gofmt -l . | grep -v '/gen/' | xargs -r gofmt -w
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...
go test ./internal/test/archtest/ -v          # the new feature is now auto-detected & checked
make test                                      # incl. swagger-check, guard, smoke, gate 72
CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3
git status --short internal/test/apiparity/testdata/    # MUST print nothing
grep -rn "internal/domain/X\|internal/app/X\|infra/repo/X\|ui/handler/X" --include='*.go' .   # MUST be empty
```

If archtest fails, the failure is REAL (the feature or a leaf imports something forbidden) — investigate whether a glue file was mis-classified in P1; never loosen the rule. If swagger-check fails, a moved handler's annotations left the scan set — that means Task 1's scan-dir fix is wrong; STOP and report.

**P6 — Commit** (one commit per feature): `refactor(X): move X into internal/X feature package`, with the standard trailer. Report: inventory findings, glue files moved, every symbol rename, verification output tails.

---

### Task 1: Prep — archtest polish + swag scan roots

**Files:**
- Modify: `internal/test/archtest/archtest_test.go` (two deferred Minors from the Phase-1 final review)
- Modify: `Makefile` (the `SWAG_INIT` line)

- [ ] **Step 1: Archtest polish.** (a) Move the kernel case ABOVE the leaf case in `TestDependencyRule`'s switch so a kernel→feature import reports the kernel-specific message (kernel ⊆ leaf, so today the generic leaf message wins). (b) In `listImports`, print stderr on failure: replace `t.Fatalf("go list: %v", err)` with

```go
if ee, ok := err.(*exec.ExitError); ok {
    t.Fatalf("go list: %v\n%s", err, ee.Stderr)
}
t.Fatalf("go list: %v", err)
```

Re-run the reqctx→logging probe from the Phase-1 procedure to confirm the kernel message still fires; revert the probe.

- [ ] **Step 2: Swagger scan roots.** `Makefile` line 115 currently scans `-d .,../handler,../../app` (from `internal/ui/apidoc`) — both dirs empty out during this phase. Change to `-d .,../../../internal` (apidoc dir + all of internal). Verify the generated spec is IDENTICAL: `make swagger && git status --short internal/ui/apidoc/docs` must be clean (same annotations scanned, wider net). If the docs differ, STOP and report the diff.

- [ ] **Step 3: Verify + commit**

Run: `make test` → PASS (incl. swagger-check with the new scan roots).

```bash
git add internal/test/archtest Makefile
git commit -m "chore: archtest diagnostics polish; widen swag scan to all of internal/"
```

---

### Task 2: Move the user feature (+ passwordrequest)

Follow **The Feature-Move Procedure** with X = `user`, plus:

- `internal/infra/repo/passwordrequest` merges into `internal/user/repo` (its `repo.go` imports `domain/user` — same feature, NOT glue). Package `passwordrequestrepo` → `repo`. KNOWN COLLISION: both `userrepo` and `passwordrequestrepo` likely export `NewRepo`/`Repo` — rename the passwordrequest side (`PasswordRequestRepo`, `NewPasswordRequestRepo`), updating its references (server.go wiring, its own integration test).
- Consumers to watch: `internal/cli` imports `app/user` (cli→feature is allowed by archtest); `internal/server`; `internal/ui/middleware` does NOT import app/user (the JWT middleware is generic) — verify.
- The user api package also registers the PUBLIC routes (login/register/remind/reset) — routes.go moves intact; guard floor stays 84.

**Files:** the four user slices + `internal/infra/repo/passwordrequest` → `internal/user/{...,repo/,api/}`; importers repo-wide.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(user): move user into internal/user feature package`.

---

### Task 3: Move the currency feature

Procedure with X = `currency`, plus:

- KNOWN GLUE: `internal/infra/repo/currency/lookup.go` imports the user feature → `internal/server/glue_currency_lookup.go`.
- `internal/cli` imports `app/currency` (allowed direction).
- domain/currency has `convertor.go` (a domain service) — moves to the root like any domain file.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(currency): move currency into internal/currency feature package`.

---

### Task 4: Move the account feature

Procedure with X = `account`, plus:

- KNOWN GLUE: `internal/infra/repo/account/lookups.go` imports the user feature → `internal/server/glue_account_lookups.go`. Watch for `UserLookup`/`CurrencyLookup` name collisions in package server as later tasks add more glue — feature-prefix now (`accountUserLookup` etc.) per P1.
- domain/account has TWO aggregates (`entity.go`, `folder.go`, `repository.go` with two interfaces) — all move to the root.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(account): move account into internal/account feature package`.

---

### Task 5: Move the category feature

Procedure with X = `category`. No known glue (its repo imports only its own feature + shared). The enginecompare `scenarios_test.go` imports `appcategory` + `categoryrepo` — the P3 sed + aliasing covers tagged files; verify with the tagged run.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(category): move category into internal/category feature package`.

---

### Task 6: Move the tag feature

Procedure with X = `tag`. No known glue.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(tag): move tag into internal/tag feature package`.

---

### Task 7: Move the payee feature

Procedure with X = `payee`. No known glue.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(payee): move payee into internal/payee feature package`.

---

### Task 8: Move the transaction feature

Procedure with X = `transaction`, plus:

- KNOWN GLUE: `adapters.go` (imports account/category/connection/payee/tag/user) and `import_adapter.go` (imports account/category/connection/payee/tag) → `internal/server/glue_transaction_adapters.go` / `glue_transaction_import_adapter.go`. These are the biggest glue files — expect several feature-prefix renames against symbols already in package server.
- The api package has three files (`transaction.go`-style endpoints, `export.go`, `import.go`) — all to `internal/transaction/api`.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(transaction): move transaction into internal/transaction feature package`.

---

### Task 9: Move the connection feature

Procedure with X = `connection`, plus:

- KNOWN GLUE: `adapters.go` (imports account/user) and `budget_revoker.go` (imports budget) → `internal/server/glue_connection_adapters.go` / `glue_connection_budget_revoker.go`. Note `budget_revoker_integration_test.go` tests glue — it moves with its glue file into `internal/server` (package `server` test) and must still pass; if it needs unexported access it keeps working in-package.

- [ ] Steps: Procedure P0–P6. Commit: `refactor(connection): move connection into internal/connection feature package`.

---

### Task 10: Move the budget feature (+ userbudget)

Procedure with X = `budget`, plus:

- KNOWN GLUE: `internal/infra/repo/budget/adapters.go` (imports account/category/payee/tag/user) → `internal/server/glue_budget_adapters.go`. `internal/infra/repo/userbudget/exists.go` implements the USER feature's budget-existence port (imports user) → glue: `internal/server/glue_userbudget_exists.go`; its integration test moves alongside. `read.go` (the hand-built dynamic SQL) is NOT glue (own feature only) → `internal/budget/repo/read.go`.
- This is the largest feature (root ~19 files + repo + api). The P0 inventory matters most here (domain/budget's `valueobject.go` symbols vs app/budget's DTO names).
- After this task, `internal/domain`, `internal/app`, `internal/infra/repo`, and `internal/ui/handler` must ALL be empty and deleted (`git mv` leaves no residue; `find internal/domain internal/app internal/infra/repo internal/ui/handler -type f 2>/dev/null` → nothing, dirs gone).

- [ ] Steps: Procedure P0–P6 + the emptiness check above. Commit: `refactor(budget): move budget into internal/budget feature package`.

---

### Task 11: Teardown — archtest legacy exemption, CLAUDE.md, phase checkpoint

**Files:**
- Modify: `internal/test/archtest/archtest_test.go` — remove `"domain": true, "app": true` from the `infrastructure` map (the dirs no longer exist; keeping them would silently exempt a future dir with those names) and update the comment.
- Modify: `CLAUDE.md` — rewrite the architecture section for the new reality: the feature-package tree (root + `repo/` + `api/` shape, the naming conventions: `entity.go`, `usecase.go`, `dto.go`, `repository.go`), glue in `internal/server`, the engine-adapter pattern's new home (`internal/<feature>/repo`), reference-repo pointers updated (e.g. `internal/{tag,user,currency}` + `internal/user/repo`), the API-handler-pattern paragraph's paths (`internal/<feature>/api/`), and the dependency-rule paragraph updated (legacy exemption gone). Verify every stated path with `ls`.

- [ ] **Step 1: Archtest map cleanup**; run `go test ./internal/test/archtest/ -v` → PASS.
- [ ] **Step 2: CLAUDE.md rewrite** per the list; cross-check paths.
- [ ] **Step 3: Phase checkpoint**

```bash
make test
make regression
```

Both PASS; capture tails.

- [ ] **Step 4: Commit** `docs: reflect Phase 2 feature-package layout; archtest drops legacy exemption`. The controller merges to `golang` after the final whole-branch review.

---

## Verification checklist (end of phase)

- [ ] Nine `internal/<feature>` packages with root+`repo/`+`api/`; `internal/{domain,app,infra/repo,ui/handler}` gone.
- [ ] Zero golden changes across the phase; guard floor 84 and catalogue floor 33 intact; OpenAPI doc byte-identical.
- [ ] archtest green with legacy exemption removed; `make regression` green.
- [ ] CLAUDE.md matches the tree.
- [ ] Branch merged into `golang`.
