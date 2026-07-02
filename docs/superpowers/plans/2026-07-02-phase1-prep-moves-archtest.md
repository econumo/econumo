# Phase 1: Prep Moves + Dependency-Rule Archtest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Relocate the shared kernel packages to their target homes (`internal/shared/{vo,errs,datetime,jwt}`, `internal/reqctx`, `internal/infra/operation`), delete `pkg/`, and add the architecture test that turns the feature-dependency rule into a CI failure — all behavior-identical.

**Architecture:** Pure package moves with mechanical import rewrites, verified by the Phase-0 safety net (goldens must not change at all — these are import-path edits, not behavior edits). The archtest lands last, locking the end state and auto-detecting feature packages as Phase 2 creates them.

**Tech Stack:** `git mv` + `sed` import rewrites, `go list` (exec'd from the archtest), the Phase-0 apiparity suite as the verifier.

**Spec:** `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md` (Phase 1 section).

## Global Constraints

- **Behavior-identical by construction**: file moves, package renames, import rewrites ONLY. Any real code change is out of scope.
- **Wire contract frozen**; goldens must be byte-identical after every task — `git status internal/test/apiparity/testdata/` stays clean; NEVER run UPDATE_GOLDEN in this phase (a golden diff means a mistake, not a regeneration).
- `make test` green after every commit (gate 72). The tagged suite must also build+pass on sqlite: `CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3` — the sed sweeps must cover build-tagged files too (plain `grep -rl` does; build/vet alone would miss them).
- Existing tests never deleted or weakened. Package names do NOT change (only their directories/import paths): `vo` stays `vo`, `jwt` stays `jwt`, `reqctx` stays `reqctx`, `operation` stays `operation`.
- Baseline (measured 2026-07-02): importer files — `domain/shared` 176, `pkg/jwt` 17, `app/reqctx` 11, `repo/operation` 6 (incl. its own test). `pkg/**` appears in `Makefile:91` (coverpkg) and `.github/workflows/go-tests.yml:12,21` (path filters).
- Branch model: base `refactor/feature-packages` (recreate from `golang` if deleted), one feature branch per task (`p1/task-NN-…`), merged back after review. Merge to `golang` at the phase boundary after `make regression`.

---

### Task 1: Move `internal/domain/shared` → `internal/shared`

The kernel packages (`vo`, `errs`, `datetime`) move to their target home. Biggest mechanical sweep of the phase (176 importing files).

**Files:**
- Move: `internal/domain/shared/{vo,errs,datetime}` → `internal/shared/{vo,errs,datetime}` (incl. `vo/testdata`)
- Modify: every file importing `github.com/econumo/econumo/internal/domain/shared/...` (176 files)

**Interfaces:**
- Produces: import paths `github.com/econumo/econumo/internal/shared/{vo,errs,datetime}` — Tasks 2–6 rely on `internal/shared/` existing.

- [ ] **Step 1: Move the directory**

```bash
git mv internal/domain/shared internal/shared
```

- [ ] **Step 2: Rewrite imports repo-wide**

```bash
grep -rl 'github.com/econumo/econumo/internal/domain/shared' --include='*.go' . \
  | xargs sed -i 's|github.com/econumo/econumo/internal/domain/shared|github.com/econumo/econumo/internal/shared|g'
```

Then check nothing referencing the old path remains anywhere (including non-Go files):

```bash
grep -rn 'internal/domain/shared' --include='*.go' . ; echo "---"; grep -rn 'domain/shared' Makefile .github/ docs/superpowers/plans/2026-07-02-phase1-prep-moves-archtest.md 2>/dev/null | grep -v 'phase1-prep'
```

Expected: no Go hits. (CLAUDE.md and the spec still mention the old path — they are updated in Task 6, not here.)

- [ ] **Step 3: Verify — build, both test tiers, goldens untouched**

```bash
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...
make test
CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3
git status --short internal/test/apiparity/testdata/
```

Expected: all green; the last command prints NOTHING (goldens untouched).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: move internal/domain/shared to internal/shared"
```

---

### Task 2: Move `pkg/jwt` → `internal/shared/jwt`; delete `pkg/`

**Files:**
- Move: `pkg/jwt` → `internal/shared/jwt` (incl. `testdata/`)
- Modify: the 17 importing files (`cmd/econumo/main.go`, `internal/app/user/{service.go,migrate_test.go}`, `internal/cli/setup_commands.go`, `internal/server/server.go`, `internal/test/apiparity/harness.go`, all 9 `internal/ui/handler/*/harness_test.go`, `internal/ui/middleware/{auth.go,middleware_test.go}`)
- Modify: `Makefile:91` (drop `./pkg/...` from coverpkg), `.github/workflows/go-tests.yml:12,21` (drop the `pkg/**` path filters)
- Modify: `CLAUDE.md` — the jwt-specific references only (the `pkg/ → jwt/` lines in the architecture tree, and the Authentication section's "in `pkg/jwt`" wording → `internal/shared/jwt`). The broader architecture-section rewrite happens in Task 6; the spec requires the jwt doc references to move in THIS commit.

**Interfaces:**
- Produces: import path `github.com/econumo/econumo/internal/shared/jwt` (package name stays `jwt`; `jwt.New`, `jwt.JWT`, `EnsureKeypair` signatures unchanged).

- [ ] **Step 1: Move and rewrite**

```bash
git mv pkg/jwt internal/shared/jwt
rmdir pkg
grep -rl 'github.com/econumo/econumo/pkg/jwt' --include='*.go' . \
  | xargs sed -i 's|github.com/econumo/econumo/pkg/jwt|github.com/econumo/econumo/internal/shared/jwt|g'
```

- [ ] **Step 2: Update build files**

In `Makefile` line 91: `-coverpkg=./internal/...,./pkg/...` → `-coverpkg=./internal/...`.
In `.github/workflows/go-tests.yml`: delete the two `- "pkg/**"` path-filter lines (12 and 21).
In `CLAUDE.md`: update the jwt references as listed above (tree line + Authentication/`pkg/jwt` mentions; `%kernel.project_dir%`/EnsureKeypair prose is content, not path — leave it).

- [ ] **Step 3: Verify**

```bash
grep -rn 'econumo/pkg' --include='*.go' . ; ls pkg 2>&1
CGO_ENABLED=0 go build ./... && make test
CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3
git status --short internal/test/apiparity/testdata/
```

Expected: no `econumo/pkg` hits; `ls pkg` errors (gone); suites green (total coverage may shift a few tenths since the coverpkg denominator changed — the 72 gate must still pass; if it doesn't, STOP and report the number, don't touch the gate); goldens untouched.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: move pkg/jwt to internal/shared/jwt; drop pkg/"
```

---

### Task 3: Move `internal/app/reqctx` → `internal/reqctx`

**Files:**
- Move: `internal/app/reqctx` → `internal/reqctx`
- Modify: the 11 importing files

- [ ] **Step 1: Move and rewrite**

```bash
git mv internal/app/reqctx internal/reqctx
grep -rl 'github.com/econumo/econumo/internal/app/reqctx' --include='*.go' . \
  | xargs sed -i 's|github.com/econumo/econumo/internal/app/reqctx|github.com/econumo/econumo/internal/reqctx|g'
```

- [ ] **Step 2: Verify**

```bash
grep -rn 'app/reqctx' --include='*.go' .
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...
make test
CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3
git status --short internal/test/apiparity/testdata/
```

Expected: no old-path hits; suites green; the goldens status prints nothing.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor: move internal/app/reqctx to internal/reqctx"
```

---

### Task 4: Move `internal/infra/repo/operation` → `internal/infra/operation`

**Files:**
- Move: `internal/infra/repo/operation` → `internal/infra/operation`
- Modify: the 5 external importing files (`internal/server/server.go`, 4 handler harness tests) + the package's own integration test moves with it

- [ ] **Step 1: Move and rewrite**

```bash
git mv internal/infra/repo/operation internal/infra/operation
grep -rl 'github.com/econumo/econumo/internal/infra/repo/operation' --include='*.go' . \
  | xargs sed -i 's|github.com/econumo/econumo/internal/infra/repo/operation|github.com/econumo/econumo/internal/infra/operation|g'
```

- [ ] **Step 2: Verify**

```bash
grep -rn 'infra/repo/operation' --include='*.go' .
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...
make test
CGO_ENABLED=0 go test -count=1 -tags enginecompare ./internal/test/enginecompare/ 2>&1 | tail -3
git status --short internal/test/apiparity/testdata/
```

Expected: no old-path hits; suites green; the goldens status prints nothing.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor: move the operation idempotency guard to internal/infra/operation"
```

---

### Task 5: Architecture test enforcing the dependency rule

A test that fails when: a feature package imports another feature; a shared leaf imports a feature; or a kernel package (`internal/shared/...`, `internal/reqctx`) imports anything internal outside the kernel. Features are auto-detected (any `internal/<top>` not in the known-infrastructure set), so Phase 2's moved features come under enforcement automatically with zero edits here. Legacy `internal/{domain,app}` are exempt until they disappear.

**Files:**
- Create: `internal/test/archtest/archtest_test.go`

**Interfaces:**
- Consumes: the module's package graph via `go list` (production imports only — `.Imports`, not test imports).
- Produces: the enforcement Phase 2 relies on; when route/feature dirs appear under `internal/`, they are treated as features automatically.

- [ ] **Step 1: Write the test**

```go
// Package archtest enforces the restructure's dependency rule (see
// docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md):
// feature packages never import each other; shared leaves never import
// features; the kernel (internal/shared, internal/reqctx) imports nothing
// internal outside itself. Features are auto-detected as any internal/<top>
// directory not in the infrastructure set, so newly moved features come under
// enforcement without edits here.
package archtest

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const module = "github.com/econumo/econumo"

// infrastructure lists the internal/<top> dirs that are NOT feature packages.
// "domain" and "app" are the legacy layered packages, exempt from the
// feature rules until Phase 2 dissolves them (they are still covered by the
// kernel rules below when they import shared/reqctx — allowed direction).
var infrastructure = map[string]bool{
	"shared": true, "reqctx": true, "ui": true, "infra": true,
	"server": true, "cli": true, "config": true, "logging": true,
	"test": true, "domain": true, "app": true,
}

// kernel packages may import internal code only from inside the kernel.
func isKernel(top string) bool { return top == "shared" || top == "reqctx" }

// leaves may be imported by features but must never import one.
// server is deliberately absent: it is the composition root and imports everything.
func isLeaf(top string) bool {
	return top == "shared" || top == "reqctx" || top == "ui" || top == "infra"
}

// topOf extracts the first path segment under internal/ ("" if not internal).
func topOf(pkg string) string {
	rest, ok := strings.CutPrefix(pkg, module+"/internal/")
	if !ok {
		return ""
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

// listImports returns production (non-test) imports for every package in the
// module, via the go tool so build constraints resolve exactly as `go build`.
func listImports(t *testing.T) map[string][]string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	cmd := exec.Command("go", "list", "-f",
		"{{.ImportPath}}|{{range .Imports}}{{.}} {{end}}", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	imports := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, deps, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		imports[pkg] = strings.Fields(deps)
	}
	if len(imports) < 30 {
		t.Fatalf("go list scanned only %d packages — the scan is broken, not the architecture", len(imports))
	}
	return imports
}

func TestDependencyRule(t *testing.T) {
	for pkg, deps := range listImports(t) {
		top := topOf(pkg)
		if top == "" {
			continue // cmd/ and the module root are unconstrained
		}
		feature := !infrastructure[top]
		for _, dep := range deps {
			dtop := topOf(dep)
			if dtop == "" {
				continue
			}
			depFeature := !infrastructure[dtop]
			switch {
			case feature && depFeature && dtop != top:
				t.Errorf("feature %s imports feature %s — features stay decoupled via consumer-side ports wired in internal/server", pkg, dep)
			case feature && (dtop == "domain" || dtop == "app"):
				t.Errorf("feature %s imports legacy layer %s — moved features must not depend on the packages being dissolved", pkg, dep)
			case !feature && isLeaf(top) && depFeature:
				t.Errorf("leaf %s imports feature %s — shared leaves must not depend on features", pkg, dep)
			case isKernel(top) && !isKernel(dtop):
				t.Errorf("kernel %s imports %s — internal/shared and internal/reqctx import nothing internal outside the kernel", pkg, dep)
			}
		}
	}
}
```

- [ ] **Step 2: Run it — must pass on the current tree**

Run: `go test ./internal/test/archtest/ -v`
Expected: PASS. If it FAILS, the violation is real (e.g. a kernel package importing outward) — report it verbatim, do not loosen the rule. Known-good facts: `vo` imports `errs` (kernel→kernel, allowed); `shared/jwt` imports no internal packages; `reqctx` imports nothing internal.

- [ ] **Step 3: Prove it can fail.** Temporarily add `_ "github.com/econumo/econumo/internal/logging"` to the imports of `internal/reqctx/reqctx.go` (a kernel package importing outside the kernel; note: errs→httpx would be a real Go import cycle since httpx imports errs, failing in go list before the rule fires), run `go test ./internal/test/archtest/ -v`, confirm it FAILS with the "kernel … imports …" message, then revert the probe (`git checkout -- internal/reqctx/reqctx.go`). Do not commit the probe. Record the failing output in your report.

- [ ] **Step 4: Full suites**

```bash
make test
git status --short   # only archtest_test.go added
```

- [ ] **Step 5: Commit**

```bash
git add internal/test/archtest
git commit -m "test(archtest): enforce the feature dependency rule from the spec"
```

---

### Task 6: CLAUDE.md architecture section + phase checkpoint

**Files:**
- Modify: `CLAUDE.md` — the architecture tree and prose:
  - In the repo tree: remove the `pkg/` block (jwt now lives under `internal/shared/jwt` — Task 2 already reworded the jwt lines; make the tree consistent), move the `shared` kernel line out of `domain/` (now `internal/shared/` with `vo`, `errs`, `datetime`, `jwt`), show `internal/reqctx/` at top level (remove the `app/reqctx` line), and `infra/operation` (remove it from the `repo/` listing).
  - Add one short paragraph under Architecture: the dependency rule (features never import features; leaves never import features; kernel imports only kernel) is enforced by `internal/test/archtest`; legacy `internal/{domain,app}` are exempt until Phase 2 dissolves them.
- No code changes.

- [ ] **Step 1: Make the CLAUDE.md edits** per the list above. Cross-check every path claim against the actual tree (`ls internal/shared internal/reqctx internal/infra/operation`) — CLAUDE.md must not describe a layout that doesn't exist.

- [ ] **Step 2: Verify + phase checkpoint**

```bash
make test
make regression
```

Expected: both PASS (regression needs the compose PostgreSQL; Docker is available).

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): reflect Phase 1 layout (shared kernel, reqctx, operation) and the archtest rule"
```

- [ ] **Step 4: Phase boundary.** Per the spec's merge cadence: the controller merges `refactor/feature-packages` → `golang` after the final review (not part of this task's commits).

---

## Verification checklist (end of phase)

- [ ] `internal/domain/shared`, `internal/app/reqctx`, `internal/infra/repo/operation`, and `pkg/` no longer exist; `internal/shared/{vo,errs,datetime,jwt}`, `internal/reqctx`, `internal/infra/operation` do.
- [ ] Zero golden-file changes across the entire phase (`git log --stat` shows no `testdata/golden` touches).
- [ ] `make test` (gate 72) and `make regression` green; archtest passing and demonstrably able to fail.
- [ ] CLAUDE.md matches the actual tree.
- [ ] Branch merged into `golang`.
