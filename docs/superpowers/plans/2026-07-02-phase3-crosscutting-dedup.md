# Phase 3: Cross-Cutting Dedup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One canonical `Clock`, `TxRunner`, and `OperationGuard` interface in the shared kernel (replacing 23 identical per-feature declarations), and one shared `RequireUser` helper (replacing 9 identical per-handler copies) — all behavior-identical.

**Architecture:** A new kernel package `internal/shared/port` holds the three seam interfaces (all kernel-compatible: stdlib + `shared/vo` only). Features reference `port.X` directly — local declarations are deleted, not aliased, so each contract exists exactly once. The `requireUser` helper moves to the package that owns the JWT context key (import-graph-decided: `ui/middleware` or `ui/httpx`), exported as `RequireUser`.

**Tech Stack:** Pure Go refactor; the Phase-0 safety net (goldens/parity/guard) plus archtest verify.

**Spec:** `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md` (Phase 3 section).

## Global Constraints

- **Behavior-identical**: interface consolidation and helper extraction only; method sets, semantics, messages, and the 401 envelope are byte-stable. Goldens byte-identical (`git status --short internal/test/apiparity/testdata/` empty; never UPDATE_GOLDEN).
- `make test` green after every commit (gate 72, guard 84, archtest, swagger-check — the OpenAPI docs must NOT change at all this phase: no package moves, no DTO changes).
- Tagged suite green: `CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3`.
- Kernel rule: `internal/shared/port` may import ONLY stdlib + `internal/shared/*` (archtest enforces).
- Declaration inventory (measured 2026-07-02, all shape-identical):
  - `Clock { Now() time.Time }` ×10: `{budget,user,category,transaction,payee,account,connection,tag}/usecase.go`, `user/api/handler.go`, `server/server.go:189`
  - `TxRunner { WithTx(ctx, func(ctx) error) error }` ×8: the eight feature `usecase.go` files (not currency)
  - `OperationGuard { Claim(ctx, vo.Id, time.Time) (bool, error); MarkHandled(ctx, vo.Id, time.Time) error }` ×5: `{category,transaction,account,payee,tag}/usecase.go`
  - `requireUser(w, r) (vo.Id, bool)` ×9: every `internal/<feature>/api/handler.go`
- Branch model: base `refactor/feature-packages`, branch per task (`p3/task-NN-…`), merge to `golang` at phase end after `make regression`.

---

### Task 1: Opening chores (deferred Minors from the Phase 2 final review)

**Files:**
- Modify: `Makefile` (~line 80: the test-cover comment says "internal + pkg packages" — `pkg/` died in Phase 1; reword to "internal packages")
- Modify: `internal/test/archtest/archtest_test.go` (`isLeaf`: add `"config"` and `"logging"` so a future `config→feature` import fails; update the function comment)

- [ ] **Step 1: Make both edits.** For archtest, the function becomes:

```go
// leaves may be imported by features but must never import one.
// server is deliberately absent: it is the composition root and imports everything.
// config and logging import nothing internal today; listing them keeps that true.
func isLeaf(top string) bool {
	switch top {
	case "shared", "reqctx", "ui", "infra", "config", "logging":
		return true
	}
	return false
}
```

- [ ] **Step 2: Verify + commit**

Run: `go test ./internal/test/archtest/ -v && make test` → PASS.

```bash
git add Makefile internal/test/archtest
git commit -m "chore: archtest leaf set gains config/logging; drop stale pkg comment"
```

---

### Task 2: `internal/shared/port` — consolidate Clock, TxRunner, OperationGuard

**Files:**
- Create: `internal/shared/port/port.go`
- Modify: the 10+8+5 declaring files (delete local decls; requalify references to `port.X`), plus any `var _` assertions naming the old locals (grep for them)

**Interfaces (Produces):**

```go
// Package port holds the cross-cutting seam interfaces every feature consumes:
// the clock, the transaction runner, and the create-idempotency guard. One
// canonical declaration each — features used to re-declare these locally, and
// Go's structural typing means consolidating them changes nothing at runtime.
package port

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Clock supplies the current instant; the infra implementation wraps time.Now,
// tests pin a fixed instant.
type Clock interface{ Now() time.Time }

// TxRunner runs fn inside one database transaction.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// OperationGuard is the row-based idempotency seam for create endpoints that
// take a client-supplied operation id (backed by operation_requests_ids).
type OperationGuard interface {
	// Claim inserts the id. already=true means a row existed (duplicate).
	// Runs inside the caller's tx.
	Claim(ctx context.Context, id vo.Id, now time.Time) (already bool, err error)
	// MarkHandled flips is_handled after the operation succeeds.
	MarkHandled(ctx context.Context, id vo.Id, now time.Time) error
}
```

- [ ] **Step 1: Create the package** exactly as above. `go build ./internal/shared/port/` compiles; `go test ./internal/test/archtest/` stays green (kernel imports kernel only).

- [ ] **Step 2: Replace the declarations, one interface at a time.** For each declaring file in the inventory: delete the local `type X interface{...}` (WITH its doc comment — the canonical comment now lives in port), add the `port` import, and requalify every reference in that package (`Clock` → `port.Clock` in struct fields, constructor params, vars). Preserve any comment content that is feature-specific (e.g. category's Claim comment mentioning create-category) by folding genuinely feature-specific notes into the USING site, not port. Compile after each interface class:

```bash
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...
```

- [ ] **Step 3: Sweep for stragglers.**

```bash
grep -rn 'type Clock interface\|type TxRunner interface\|type OperationGuard interface' internal --include='*.go' | grep -v shared/port
```

Expected: empty. Also `gofmt -l . | grep -v '/gen/'` clean.

- [ ] **Step 4: Full verify** — `make test`; tagged suite tail; `git status --short internal/test/apiparity/testdata/` prints nothing; `git status --short internal/ui/apidoc/docs/` prints nothing (OpenAPI untouched).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: consolidate Clock/TxRunner/OperationGuard into internal/shared/port"
```

---

### Task 3: Shared `RequireUser`

**Files:**
- Create or modify: the owning package — decide by import graph: `requireUser` reads the JWT user id from the request context (the key/accessor lives where the auth middleware put it — check `internal/ui/middleware/auth.go` and what `internal/<feature>/api/handler.go` currently calls) and writes the 401 envelope via `httpx`. Place `RequireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool)` in the package that owns the context accessor IF that creates no import cycle with httpx; otherwise in `httpx` taking the accessor's result. Inspect first, decide, record the reasoning in the report.
- Modify: all 9 `internal/<feature>/api/handler.go` — delete the local method; call sites `h.requireUser(w, r)` → `<pkg>.RequireUser(w, r)`.

- [ ] **Step 1: Read one copy fully** (e.g. `internal/category/api/handler.go`) plus `internal/ui/middleware/auth.go`; confirm all 9 copies are byte-identical in behavior (diff them); pick the home per the rule above.

- [ ] **Step 2: Extract.** The shared helper's body must be byte-equivalent to the copies (same envelope, same message, same status). Delete all 9 local methods; update call sites mechanically.

- [ ] **Step 3: Sweep + verify.** `grep -rn 'func.*requireUser' internal --include='*.go'` → empty. `make test`; tagged tail; goldens + OpenAPI status clean (the 401 envelope is pinned by `error_paths`/`negative_paths` goldens — if any golden changes, the extraction drifted: STOP).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: single RequireUser helper replaces nine handler copies"
```

---

### Task 4: Phase checkpoint

- [ ] **Step 1:** `make test` AND `make regression` → both PASS (capture tails in the report). No commit unless something needed fixing (report if so).
- [ ] **Step 2:** The controller merges to `golang` after the final review.

---

## Verification checklist (end of phase)

- [ ] Exactly one declaration each of Clock/TxRunner/OperationGuard (in `internal/shared/port`); zero `requireUser` methods; ~23+9 duplicate declarations gone.
- [ ] Goldens and committed OpenAPI docs byte-identical across the phase.
- [ ] `make test` + `make regression` green; archtest green (port passes the kernel rule).
- [ ] Branch merged into `golang`.
