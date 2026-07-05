# Phase 6: Entity Idioms + God-Repo Splits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks 2–6 follow **The Entity-Idiom Procedure** below.

**Goal:** Idiomatic Go entities across all aggregates — exported fields, no getter walls, no positional `FromState` — plus role-based splits of the two god repository interfaces. The final phase of the restructure.

**Architecture:** Per aggregate: export the fields (idiomatic names: `ID`, `UserID`, …), delete bare getters, KEEP every mutator and every logic-bearing method verbatim (change-tracking `updatedAt`-bumps are behavior pinned by goldens), delete `FromState` (repo hydration becomes a struct literal). Compile-driven reference migration (the compiler enumerates every `x.Name()` → `x.Name` site; no cross-type sed). God repos: role interfaces + a composite embedding them (wiring stays one param; the Service holds per-role fields assigned from it).

**Recorded decisions (spec's "decide at Phase 6 planning"):**
- Use-case struct names: **KEEP `Service`/`ReadService`/`WriteService`** — renaming churns every wiring site for zero behavioral value.
- One-file-per-verb consolidation: **SKIP** — files are cohesive post-restructure; churn outweighs benefit.
- `transaction.Importer` breadth (11 methods): **accept with a doc comment** (it fronts the import pipeline's find-or-create surface; slimming = redesign, out of scope).

**Spec:** Phase 6 of `docs/superpowers/specs/2026-07-01-feature-package-restructure-design.md`.

## Global Constraints

- **Behavior-identical**: field access replaces method calls 1:1; mutator semantics (change-tracking, `updatedAt` bump only-on-real-change) preserved EXACTLY; creation defaults preserved. Goldens byte-identical; OpenAPI byte-identical; never UPDATE_GOLDEN.
- `make test` green per commit (gate 72, guard 84, archtest); tagged tail green; gofmt clean. `make regression` at phase end.
- Entities' constructors (`New<X>`) stay — they encode creation defaults. Methods with ANY logic beyond `return x.field` stay as methods (if a kept method's name would clash with an exported field, keep the method and name the field to avoid the clash — report each case).
- One aggregate (family) per commit.
- Branch model: base `refactor/feature-packages`, branch per task (`p6/task-NN-…`); merge to `golang` at phase end.

---

## The Entity-Idiom Procedure (Tasks 1–6; A = the aggregate)

**E1 — Inventory.** For A's entity file(s): classify every method — bare getter (`return x.field` only) → DELETE+export; logic-bearing accessor → KEEP as method; mutator/constructor → KEEP verbatim (bodies switch to exported field names only). List `FromState` call sites (`grep -rn 'A.FromState\|FromState(' internal/<feature>`) — typically exactly one, in `repo/`.

**E2 — Rewrite the entity file.** Exported fields with idiomatic names (`id`→`ID`, `userID`→`UserID`, `isArchived`→`IsArchived`, `typ`→`Type` unless a kept method claims the name). Field docs absorb any getter doc worth keeping. Delete bare getters and `FromState`. Constructors/mutators keep signatures and semantics; only internal field references change.

**E3 — Migrate references compile-driven.** `CGO_ENABLED=0 go build ./... 2>&1 | head -40`, fix each reported site `x.Name()` → `x.Name` VERBATIM in place (no logic edits, no simplifications), repeat until clean. Then the same for `go vet ./...` and the test files (`go test ./internal/<feature>/... -count=1`). The `FromState` site in repo/ becomes a struct literal:

```go
return &tag.Tag{ID: id, UserID: userID, Name: row.Name, Position: row.Position,
	IsArchived: archived, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
```

**E4 — Verify.** `make test` (goldens are the behavior pin — any golden diff means a mutator/default drifted: STOP); tagged tail; `git status --short internal/test/apiparity/testdata internal/ui/apidoc/docs` empty; gofmt clean.

**E5 — Commit** `refactor(<feature>): idiomatic entity — exported fields, no getter wall`.

Worked example — the complete target `internal/tag/entity.go` (Task 1 transcribes this):

```go
// Package tag is the tag aggregate's domain layer: the Tag entity and the
// repository interface.
//
// Unlike a category, a tag has no type and no persisted icon: its icon is a
// fixed "tag" and is not stored or returned on the wire (the TagResult DTO has
// no icon field).
package tag

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Tag is the tag aggregate root. The name is validated on the way in by the
// application layer; the entity holds already-valid state and its mutators each
// bump UpdatedAt only on a real change. Fields are exported for direct read
// access; all writes after construction go through the mutators.
type Tag struct {
	ID         vo.Id
	UserID     vo.Id
	Name       string
	Position   int16
	IsArchived bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewTag constructs a freshly-created tag. Position defaults to 0 and is set by
// the service via SetPosition before the first save.
func NewTag(id, userID vo.Id, name string, now time.Time) *Tag {
	return &Tag{ID: id, UserID: userID, Name: name, CreatedAt: now, UpdatedAt: now}
}

// SetPosition sets the initial position at creation. It does not bump UpdatedAt
// — it is part of construction.
func (t *Tag) SetPosition(position int16) { t.Position = position }

func (t *Tag) UpdateName(name string, now time.Time) {
	if t.Name != name {
		t.Name = name
		t.UpdatedAt = now
	}
}

func (t *Tag) UpdatePosition(position int16, now time.Time) {
	if t.Position != position {
		t.Position = position
		t.UpdatedAt = now
	}
}

func (t *Tag) Archive(now time.Time) {
	if !t.IsArchived {
		t.IsArchived = true
		t.UpdatedAt = now
	}
}

func (t *Tag) Unarchive(now time.Time) {
	if t.IsArchived {
		t.IsArchived = false
		t.UpdatedAt = now
	}
}
```

(Note `NewTag` drops the explicit zero-value assignments — identical semantics; keep that style.)

---

### Task 1: Template — tag

Procedure E1–E5 with A = tag; entity target is the worked example above verbatim. Reference sites: `internal/tag/{usecase,create,update,archive,order,read? (read uses view rows, likely none),dto}.go`, `internal/tag/repo/repo.go` (FromState site), `internal/tag/entity...tag_test.go`. Report the exact site count for calibration.

### Task 2: category + payee

Procedure per aggregate, one commit each. category adds `type`/`icon` fields (getter `Type()`/`Icon()`; watch: `Type` field vs any kept method — category's type alias helpers live on the `CategoryType`-ish value object, check E1). payee mirrors tag.

### Task 3: user (+ PasswordRequest) + connection (+ Invite)

- user: entity.go (18 getters) + password_request.go (6 getters, no FromState — check construction path). SEED: move the `PasswordRequests` port declaration from `usecase.go` into `repository.go` beside `Repository` (file-placement consistency; pure move).
- connection: entity.go (AccountAccess; its `Role()` may be logic-bearing — alias mapping — E1 decides) + invite.go (Invite; `GenerateNewCode` mutator stays; code field exported only if no logic getter).

### Task 4: account (+ Folder) + account god-repo split

- Entities: entity.go (10 getters) + folder.go (7).
- God split: `internal/account/repository.go` — `Repository` (11 methods) and `FolderRepository` (10). Split by role, e.g.:

```go
// AccountStore / PositionStore / BalanceReader for Repository;
// FolderStore / FolderMembership for FolderRepository — derive the actual
// grouping from the method semantics at E1-time (names above are the shape,
// not a mandate); each role gets a doc comment saying which use-cases consume it.
type Repository interface {       // composite for wiring; roles for consumption
	AccountStore
	PositionStore
	BalanceReader
}
```

The Service's struct fields split per role (assigned from the one composite constructor param — wiring in server.go unchanged); each use-case file references the narrowest field. Same for FolderRepository. `var _` assertions in repo/ updated to the composite.

### Task 5: transaction

Entity already has `NewState`/`New`/`FromState(NewState)`/`Update` — KEEP all four (Update enforces the transfer type rules; NewState stays the constructor bundle). Work: export the fields, delete the 18 bare getters, migrate references. `FromState(NewState)` stays (it's already struct-based; deleting it would push `updatedAt := s.CreatedAt`-vs-`s.UpdatedAt` subtlety to the repo — not worth it). SEED: add the accept-with-comment doc on `transaction.Importer` (ports.go) noting the 11-method breadth fronts the import find-or-create pipeline by design.

### Task 6: budget family + budget god-repo split

- Entities: entity.go (Budget + BudgetAccess/BudgetFolder/BudgetEnvelope/BudgetElement/BudgetElementLimit — 43 getters, 6 FromStates) + valueobject.go (ElementType/UserRole keep their `Alias()`/parse methods — logic, not getters). One commit may cover the family; if the diff balloons, split entity-family/repo-split into two commits.
- God split: `internal/budget/repository.go` (33 methods) → roles (shape suggestion: `BudgetStore`, `AccessStore`, `FolderStore`, `EnvelopeStore`, `ElementStore`, `LimitStore`; derive the real grouping from method prefixes at E1-time) + composite `Repository` embedding them; Service fields per role.

### Task 7: Checkpoint + docs (controller-assisted)

- CLAUDE.md: the architecture section's entity-shape wording (exported fields + mutators; no getters), god-repo note removed from "Key design decisions" if present; verify paths.
- `make test` + `make regression` PASS; final whole-branch review; merge to `golang`. Record in the ledger: Phase 6 = restructure COMPLETE.

---

## Verification checklist (end of phase)

- [ ] Zero bare-getter walls in `internal/*/entity*.go` (+folder/invite/password_request/valueobject files); zero positional `FromState` (transaction's struct-based one excepted).
- [ ] account + budget repositories split into role interfaces with composites; wiring unchanged.
- [ ] Goldens + OpenAPI byte-identical across the phase; regression green.
- [ ] Branch merged into `golang`. Restructure complete.
