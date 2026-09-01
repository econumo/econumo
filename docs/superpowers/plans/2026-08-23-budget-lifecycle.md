# Budget Lifecycle (end date, archive, clone) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Budgets gain an end month (`ended_at`), an archived state (hidden + read-only), and an owner-only deep clone — so users can complete a budget and continue in a fresh copy without rebuilding structure, sharing, membership, or plans.

**Architecture:** Two nullable-ish columns on `budgets` (`ended_at DATETIME NULL`, `is_archived BOOLEAN NOT NULL DEFAULT 0`) flow through the model's invariant-preserving mutators (`EndAt`/`Archive`/`Unarchive`) into the shared `MetaResult` wire shape. An `requireNotArchived` guard runs after `loadAggregate` in every budget write except a five-route allowlist. End-date semantics clamp the period readers and the set-limit/removal windows. Clone is a single-transaction use case driving the existing store Save methods with an old→new id map (envelope-type elements' `external_id` remapped through it). REST + MCP edges follow; the SPA adds Duplicate/Complete/Archive flows.

**Tech Stack:** Go 1.x (stdlib `net/http`), sqlc (sqlite + pgsql), modernc sqlite / pgx, React 19 + Vite + vitest, react-i18next.

**Spec:** `docs/superpowers/specs/2026-08-17-budget-lifecycle-design.md` (binding; builds on `docs/superpowers/specs/2026-08-16-budget-account-membership-design.md`)

## Global Constraints

- Branch: `feature/budget-lifecycle` (based on `bug/budget-account-membership`; commit here).
- Two HTTP methods only: `GET` reads, `POST` writes; routes `/api/v1/budget/<action>-<subject>`.
- Frozen wire conventions: datetimes `"2006-01-02 15:04:05"`; empty string for NULL datetimes on the wire; int `0`/`1` for booleans on the wire (`isArchived`); JSON arrays `[]` never `null`; envelope shapes unchanged.
- Features never import features; kernel/infra never import a feature (archtest).
- Every coded error is registered in `errs.AllCodes` AND translated in all 11 catalogues (`locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`); every new frontend `t()` key exists in all 11. New non-error UI copy: the plan gives exact `en` strings; translate the other ten in each catalogue's established style (i18ntest enforces key/placeholder parity).
- Goldens (`internal/test/apiparity`, `internal/test/mcpparity`) regenerated with `UPDATE_GOLDEN=1` and the diff INSPECTED — never hand-edited; numeric values in pre-existing scenarios must not drift.
- After changing sqlc queries: `cd internal/infra/storage/sqlc && go generate ./...`. After changing handlers/DTOs: `make swagger`.
- Comments only for non-obvious "why"; no PHP references. Commit after every task with the trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Gates: `make go-lint` and `go test ./...` pass at the end of every task unless the task text scopes them; `make test` at the end of Task 11.

---

## File Map

| Area | Files |
|---|---|
| Migration + model | `internal/infra/storage/migrations/{sqlite,pgsql}/20260818000000.sql` (new); `internal/model/budget.go`; sqlc `query/{sqlite,pgsql}/budgets.sql` + regenerated `gen/`; `internal/budget/repo/{adapters.go,engine.go,repo.go}` |
| Wire meta + endDate | `internal/model/budget_dto.go`; `internal/budget/builder.go` (`buildMeta`); `internal/budget/crud.go` (`UpdateBudget`); `internal/shared/errs/codes.go`; `locales/*.json` |
| Archive | `internal/budget/lifecycle.go` (new), `internal/budget/api/{budget_more.go,routes.go}`, every write use case (guard call), `internal/budget/api/archived_test.go` (new) |
| End-date semantics | `internal/budget/read.go`, `builder_plan.go` or `plan.go`, `txlist.go`, `accounts.go` (`SetLimit`, `removableAccounts`) |
| Clone | `internal/budget/clone.go` (new); one new sqlc query `ListBudgetLimitsFrom` (both engines) + `internal/budget/repo/*`, `repository.go`; `internal/budget/api/*`; `internal/model/budget_dto.go` |
| MCP | `internal/budget/mcp/mcp.go`; `internal/test/mcpparity/catalogue.go` + goldens |
| apiparity | `internal/test/apiparity/catalogue_budget_lifecycle.go` (new) + goldens; `catalogue_test.go` floor |
| SPA | `web/src/api/{budget.ts,dto/budget.ts}`; `web/src/features/budgets/{queries.ts,BudgetsPage.tsx,BudgetUpdateDialog.tsx,BudgetPage.tsx,PeriodStrip.tsx,budgetMath.ts,planMath.ts,PlanSheet.tsx}`; new `DuplicateBudgetDialog.tsx`, `CompleteBudgetDialog.tsx`; `web/src/lib/metrics.ts` |
| Docs | `CLAUDE.md` |

---

### Task 1: Migration, model fields + mutators, sqlc columns, hydration

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260818000000.sql`, `internal/infra/storage/migrations/pgsql/20260818000000.sql`
- Modify: `internal/model/budget.go`, `internal/model/budget_test.go` (or the file holding budget model tests)
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql` (+ regenerate)
- Modify: `internal/budget/repo/adapters.go` (`hydrateBudget`), `internal/budget/repo/repo.go` (budget Save/Upsert param mapping), `internal/budget/repo/engine.go` (pgsql conversion if the row structs stop being type-aliasable)
- Test: `internal/budget/repo/lifecycle_repo_test.go` (new)

**Interfaces:**
- Produces: `model.Budget.EndedAt *time.Time`, `model.Budget.IsArchived bool`; `model.ErrBudgetEndBeforeStart` (sentinel); mutators `(*Budget).EndAt(month *time.Time, now time.Time) error` (snaps to `FirstOfMonth`; `nil` clears; returns the sentinel when `month < FirstOfMonth(StartedAt)`; bumps `UpdatedAt` only on change), `(*Budget).Archive(now time.Time)`, `(*Budget).Unarchive(now time.Time)` (both idempotent — no `UpdatedAt` bump when already in the target state).
- Persistence: `budgets` rows round-trip `EndedAt` (`*time.Time`, NULL ↔ nil — same pattern as `users.access_until`) and `IsArchived` (`bool` on both engines, like `budgets_access.is_accepted`).

- [ ] **Step 1: Migration (both engines)**

`sqlite/20260818000000.sql`:

```sql
-- Budget lifecycle: ended_at is the LAST month the budget covers (inclusive,
-- first-of-month, NULL = open-ended); is_archived hides the budget and makes
-- it read-only. Independent flags: archiving does not set an end date.
ALTER TABLE budgets ADD COLUMN ended_at DATETIME NULL;
ALTER TABLE budgets ADD COLUMN is_archived BOOLEAN DEFAULT '0' NOT NULL;
```

`pgsql/20260818000000.sql` — copy the pgsql conventions from the columns `users.access_until` (nullable timestamp) and `accounts_access.is_accepted` (boolean) use in their own migrations (open `pgsql/20260719000000.sql` and `pgsql/20260714000000.sql` and mirror the exact type spellings):

```sql
ALTER TABLE budgets ADD COLUMN ended_at TIMESTAMP(0) WITHOUT TIME ZONE NULL;
ALTER TABLE budgets ADD COLUMN is_archived BOOLEAN DEFAULT FALSE NOT NULL;
```

- [ ] **Step 2: Failing model tests**

Append to the budget model test file (find it: `grep -rn "func TestBudget" internal/model/`):

```go
func TestBudgetEndAt(t *testing.T) {
	now := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	b := model.NewBudget(vo.NewId(), vo.NewId(), "B", vo.NewId(), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), now)

	mid := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // snaps to 2026-06-01
	if err := b.EndAt(&mid, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b.EndedAt == nil || !b.EndedAt.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("EndedAt = %v, want 2026-06-01", b.EndedAt)
	}
	if !b.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Fatal("UpdatedAt not bumped")
	}
	before := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := b.EndAt(&before, now); !errors.Is(err, model.ErrBudgetEndBeforeStart) {
		t.Fatalf("err = %v, want ErrBudgetEndBeforeStart", err)
	}
	// equal to the start month is allowed (a one-month budget)
	start := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	if err := b.EndAt(&start, now); err != nil {
		t.Fatal(err)
	}
	if err := b.EndAt(nil, now.Add(2*time.Hour)); err != nil || b.EndedAt != nil {
		t.Fatalf("clear: err=%v EndedAt=%v", err, b.EndedAt)
	}
}

func TestBudgetArchiveUnarchive_Idempotent(t *testing.T) {
	now := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	b := model.NewBudget(vo.NewId(), vo.NewId(), "B", vo.NewId(), now, now)
	b.Archive(now.Add(time.Hour))
	if !b.IsArchived || !b.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("archive: %+v", b)
	}
	b.Archive(now.Add(2 * time.Hour)) // idempotent: no bump
	if !b.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Fatal("second Archive bumped UpdatedAt")
	}
	b.Unarchive(now.Add(3 * time.Hour))
	if b.IsArchived || !b.UpdatedAt.Equal(now.Add(3*time.Hour)) {
		t.Fatalf("unarchive: %+v", b)
	}
}
```

Add `"errors"` to imports as needed.

- [ ] **Step 3: Run to verify failure** — `go test ./internal/model/ -run 'BudgetEndAt|BudgetArchive' -v` → compile FAIL (fields/mutators undefined).

- [ ] **Step 4: Implement the model**

In `internal/model/budget.go`: add `EndedAt *time.Time` and `IsArchived bool` to `Budget` (after `StartedAt`); near the mutators:

```go
// ErrBudgetEndBeforeStart is returned by EndAt when the requested end month
// precedes the budget's start month. The use case maps it to the coded
// validation error budget.end_before_start.
var ErrBudgetEndBeforeStart = errors.New("budget end month is before its start")

// EndAt sets the last covered month (snapped to first-of-month; nil clears).
func (b *Budget) EndAt(month *time.Time, now time.Time) error {
	if month == nil {
		if b.EndedAt != nil {
			b.EndedAt = nil
			b.UpdatedAt = now
		}
		return nil
	}
	m := FirstOfMonth(*month)
	if m.Before(FirstOfMonth(b.StartedAt)) {
		return ErrBudgetEndBeforeStart
	}
	if b.EndedAt == nil || !b.EndedAt.Equal(m) {
		b.EndedAt = &m
		b.UpdatedAt = now
	}
	return nil
}

func (b *Budget) Archive(now time.Time) {
	if !b.IsArchived {
		b.IsArchived = true
		b.UpdatedAt = now
	}
}

func (b *Budget) Unarchive(now time.Time) {
	if b.IsArchived {
		b.IsArchived = false
		b.UpdatedAt = now
	}
}
```

- [ ] **Step 5: sqlc + repo**

In both `query/{sqlite,pgsql}/budgets.sql`: add `ended_at, is_archived` to the column lists of `GetBudgetByID`, `ListBudgetsForUser`, and `UpsertBudget` (INSERT columns AND the `ON CONFLICT` SET list — `ended_at = excluded.ended_at, is_archived = excluded.is_archived`; pgsql `EXCLUDED.`). Regenerate. Fix `internal/budget/repo`: `hydrateBudget` copies `EndedAt` (pointer straight through, like the access-token repo does with `ExpiresAt`) and `IsArchived`; the budget upsert param mapping in `repo.go` carries both; if the sqlite/pgsql generated row structs still have identical field sets the existing type-alias/conversion in `engine.go` needs only the regenerated types — follow whatever the compiler demands, converting field-by-field like the neighboring rows if aliasing breaks.

- [ ] **Step 6: Failing repo round-trip test**

`internal/budget/repo/lifecycle_repo_test.go`:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestBudgetRepo_EndedAtAndArchivedRoundTrip(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId().String()
	f.User(fixture.User{ID: u, Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	id := vo.NewId()
	f.Budget(fixture.Budget{ID: id.String(), UserID: u})

	r := budgetrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	b, err := r.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if b.EndedAt != nil || b.IsArchived {
		t.Fatalf("fresh budget: EndedAt=%v IsArchived=%v", b.EndedAt, b.IsArchived)
	}
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := b.EndAt(&end, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	b.Archive(time.Now().UTC())
	if err := r.Save(ctx, b); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.EndedAt == nil || !got.EndedAt.Equal(end) || !got.IsArchived {
		t.Fatalf("round-trip: EndedAt=%v IsArchived=%v", got.EndedAt, got.IsArchived)
	}
	if err := got.EndAt(nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got.Unarchive(time.Now().UTC())
	if err := r.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	back, _ := r.GetByID(ctx, id)
	if back.EndedAt != nil || back.IsArchived {
		t.Fatalf("clear round-trip: EndedAt=%v IsArchived=%v", back.EndedAt, back.IsArchived)
	}
}
```

(If `NewRepo`'s signature differs, mirror `accounts_test.go` in the same package. NOTE the modernc driver normalizes DATETIME values — compare with `.Equal`, never string equality.)

- [ ] **Step 7: Run** — `go test ./internal/model/ ./internal/budget/... ./internal/infra/storage/...` → PASS; then `go build ./... && go vet ./... && go test ./...` → PASS (nothing reads the new fields yet).

- [ ] **Step 8: Commit** — `git commit -am "feat(budget): ended_at + is_archived columns, model mutators"` (with trailer).

---

### Task 2: Wire meta (`endedAt`/`isArchived`), `update-budget.endDate`, error codes + catalogues

**Files:**
- Modify: `internal/model/budget_dto.go` (`MetaResult`, `UpdateBudgetRequest`), `internal/budget/builder.go` (`buildMeta`), `internal/budget/crud.go` (`UpdateBudget`)
- Modify: `internal/shared/errs/codes.go`, all 11 `locales/<lang>.json`
- Test: `internal/budget/api/lifecycle_test.go` (new)
- Run: `make swagger`

**Interfaces:**
- Produces: `MetaResult.EndedAt string \`json:"endedAt"\`` (datetime layout, `""` when unset — placed right after `StartedAt`) and `MetaResult.IsArchived int \`json:"isArchived"\`` (0/1, placed after `CurrencyId`); `UpdateBudgetRequest.EndDate *string \`json:"endDate"\`` (nil absent → untouched; `""` → clear; `"2006-01-02"` → set); `errs.CodeBudgetArchived = "budget.archived"`, `errs.CodeBudgetEndBeforeStart = "budget.end_before_start"` in `AllCodes`.
- NOTE: every apiparity/mcpparity golden containing budget meta will change shape (two new keys). Regeneration happens in Tasks 6-7; until then the parity suites are red — this task's gate excludes them.

- [ ] **Step 1: Failing api tests**

`internal/budget/api/lifecycle_test.go`, in the existing harness style (grep `newHarness`, `createBudgetReq`, `mustUnmarshal`, `containsField` in this package and reuse):

```go
package api_test

import (
	"net/http"
	"testing"
)

// metaItemOut matches {item: MetaResult} responses (update-budget,
// archive-budget, unarchive-budget). Clone returns {item: BudgetResult},
// whose meta nests one level deeper — see metaOut in clone_test.go.
type metaItemOut struct {
	Item struct {
		EndedAt    string `json:"endedAt"`
		IsArchived int    `json:"isArchived"`
	} `json:"item"`
}

func TestUpdateBudget_EndDate(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Budget")))

	// absent endDate → untouched (still "")
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID})
	if st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	res := mustUnmarshal[metaItemOut](t, env.Data)
	if res.Item.EndedAt != "" || res.Item.IsArchived != 0 {
		t.Fatalf("meta=%+v", res.Item)
	}

	// set (mid-month input snaps to first-of-month, full datetime on the wire)
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "2030-06-15"})
	res = mustUnmarshal[metaItemOut](t, env.Data)
	if st != http.StatusOK || res.Item.EndedAt != "2030-06-01 00:00:00" {
		t.Fatalf("set: status=%d endedAt=%q", st, res.Item.EndedAt)
	}

	// before start → coded 400 on endDate
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "1999-01-01"})
	if st != http.StatusBadRequest || !containsField(env, "endDate", "end month") {
		t.Fatalf("before-start: status=%d body=%s", st, env.raw)
	}

	// garbage → blank-validation on endDate
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "not-a-date"})
	if st != http.StatusBadRequest {
		t.Fatalf("garbage accepted: %s", env.raw)
	}

	// "" clears
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": ""})
	res = mustUnmarshal[metaItemOut](t, env.Data)
	if st != http.StatusOK || res.Item.EndedAt != "" {
		t.Fatalf("clear: status=%d endedAt=%q", st, res.Item.EndedAt)
	}
}
```

(`mustOK`/`containsField` exist from the membership work — grep; if `budgetID1`/`usdID`/`createBudgetReq` differ, adapt names, keep substance. The start date of `createBudgetReq`'s budget is the harness clock's month — `"1999-01-01"` is safely before it.)

- [ ] **Step 2: Run** — `go test ./internal/budget/api/ -run UpdateBudget_EndDate -v` → FAIL.

- [ ] **Step 3: Implement**

`budget_dto.go`: add the two `MetaResult` fields exactly as the Interfaces block places them; add `EndDate *string \`json:"endDate"\`` to `UpdateBudgetRequest` (no `Validate()` change — semantics live in the use case).

`builder.go` `buildMeta`: populate

```go
	endedAt := ""
	if b.budget.EndedAt != nil {
		endedAt = b.budget.EndedAt.Format(datetime.Layout)
	}
	isArchived := 0
	if b.budget.IsArchived {
		isArchived = 1
	}
```

`errs/codes.go`: both constants next to the other `CodeBudget*` + both in `AllCodes`.

`crud.go` `UpdateBudget`, inside the existing tx after the name/currency mutations:

```go
		if req.EndDate != nil {
			if *req.EndDate == "" {
				if err := b.budget.EndAt(nil, now); err != nil {
					return err
				}
			} else {
				d, perr := time.Parse(datetime.DateLayout, *req.EndDate)
				if perr != nil {
					return model.ValidateBlank(map[string]string{"endDate": ""})
				}
				if eerr := b.budget.EndAt(&d, now); eerr != nil {
					if errors.Is(eerr, model.ErrBudgetEndBeforeStart) {
						return errs.NewValidation("Validation failed", errs.FieldError{
							Key: "endDate", Message: "The end month is before the budget start",
							Code: errs.CodeBudgetEndBeforeStart,
						})
					}
					return eerr
				}
			}
		}
```

(`ValidateBlank` is the name used by the membership work — grep and reuse the real helper. Ensure `UpdateBudget` saves the budget row after the mutation — it already does for name/currency.)

Catalogue rows (add to **all 11**; `en` values below are exact, translate the rest in-style):

| key | en |
|---|---|
| `errors.budget.archived` | This budget is archived |
| `errors.budget.end_before_start` | The end month is before the budget start |

Run `make swagger`.

- [ ] **Step 4: Run** — `go test ./internal/budget/... ./internal/test/i18ntest/ ./internal/shared/...` → PASS. Parity suites are now red (meta shape) — expected until Tasks 6-7; run `go test $(go list ./... | grep -v '/test/apiparity' | grep -v '/test/mcpparity')` → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(budget): endedAt/isArchived on the wire; update-budget endDate"` (with trailer).

---

### Task 3: Archived guard, archive/unarchive endpoints, write matrix test

**Files:**
- Create: `internal/budget/lifecycle.go`
- Modify: every budget write use case (guard call — list below), `internal/model/budget_dto.go` (Archive/Unarchive DTOs), `internal/budget/api/budget_more.go` (handlers), `internal/budget/api/routes.go`
- Test: `internal/budget/api/archived_test.go` (new)
- Run: `make swagger`

**Interfaces:**
- Produces: `(*Service).ArchiveBudget(ctx, userID, model.ArchiveBudgetRequest) (*model.ArchiveBudgetResult, error)` and `UnarchiveBudget(...)` — auth `canDelete` (owner|admin), idempotent, result `{Item MetaResult}` via the existing `reloadMeta` helper; `model.ArchiveBudgetRequest{Id string \`json:"id"\`}` (Validate: Id NotBlank) + `ArchiveBudgetResult{Item MetaResult \`json:"item"\`}`, same pair for Unarchive; unexported `func (s *Service) requireNotArchived(b *budgetAggregate) error` returning `&errs.AccessDeniedError{Msg: "This budget is archived", Code: errs.CodeBudgetArchived}` when `b.budget.IsArchived` (the coded-403 pattern `internal/user/verify_email.go:29` uses).
- Guard applied (right after `loadAggregate`/`requireBudget` + role check) in: `UpdateBudget`, `ResetBudget`, `CreateFolder`, `UpdateFolder`, `DeleteFolder`, `MoveFolder`, `CreateEnvelope`, `UpdateEnvelope`, `DeleteEnvelope`, `GrantAccess`, `AddAccount`+`RemoveAccount` (in `membershipPrelude`), `ChangeElementCurrency`, `SetLimit`, `MoveElement`.
- Guard NOT applied (allowlist): `DeleteBudget`, `RevokeAccess`, `DeclineAccess`, `AcceptAccess`, `UnarchiveBudget`, and `ArchiveBudget` itself (idempotent no-op on an archived budget). `CreateBudget` has no aggregate — untouched.
- Routes: `POST /api/v1/budget/archive-budget`, `POST /api/v1/budget/unarchive-budget` via `endpoint.Handle` with swag blocks mirroring the neighboring handlers.

- [ ] **Step 1: Failing matrix test**

`internal/budget/api/archived_test.go` — seed a full budget (folder, envelope, element via the harness fixture or API calls), grant+accept a second user, archive it directly (`UPDATE budgets SET is_archived = 1 WHERE id = ?` through `h.db` — the endpoint exists only after Step 3), then table-drive EVERY write route:

```go
func TestArchivedBudget_WriteMatrix(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Budget")))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{"budgetId": budgetID1, "id": folderID1, "name": "F"}))
	if _, err := h.db.Exec(`UPDATE budgets SET is_archived = 1 WHERE id = ?`, budgetID1); err != nil {
		t.Fatal(err)
	}

	blocked := []struct {
		label, path string
		body        map[string]any
	}{
		{"update-budget", "/api/v1/budget/update-budget", map[string]any{"id": budgetID1, "name": "X", "currencyId": usdID}},
		{"reset-budget", "/api/v1/budget/reset-budget", map[string]any{"id": budgetID1, "startDate": "2026-01-01"}},
		{"create-folder", "/api/v1/budget/create-folder", map[string]any{"budgetId": budgetID1, "id": folderID2, "name": "G"}},
		{"update-folder", "/api/v1/budget/update-folder", map[string]any{"budgetId": budgetID1, "id": folderID1, "name": "G"}},
		{"delete-folder", "/api/v1/budget/delete-folder", map[string]any{"budgetId": budgetID1, "id": folderID1}},
		{"move-folder", "/api/v1/budget/move-folder", map[string]any{"budgetId": budgetID1, "id": folderID1, "after": ""}},
		{"create-envelope", "/api/v1/budget/create-envelope", map[string]any{"budgetId": budgetID1, "id": envelopeID1, "name": "E", "currencyId": usdID, "categories": []string{}}},
		{"grant-access", "/api/v1/budget/grant-access", map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"}},
		{"add-account", "/api/v1/budget/add-account", map[string]any{"id": budgetID1, "accountId": accountID2}},
		{"remove-account", "/api/v1/budget/remove-account", map[string]any{"id": budgetID1, "accountId": accountID}},
		{"set-limit", "/api/v1/budget/set-limit", map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-08-01", "amount": "10"}},
		{"move-element", "/api/v1/budget/move-element", map[string]any{"budgetId": budgetID1, "id": catID, "folderId": nil, "after": ""}},
		{"change-element-currency", "/api/v1/budget/change-element-currency", map[string]any{"budgetId": budgetID1, "id": catID, "currencyId": usdID}},
	}
	for _, c := range blocked {
		st, env := h.do(t, http.MethodPost, c.path, tok, c.body)
		if st != http.StatusForbidden || !strings.Contains(string(env.raw), "archived") {
			t.Fatalf("%s: status=%d body=%s (want 403 budget-archived)", c.label, st, env.raw)
		}
	}

	// allowlist still works: unarchive (added in this task), then re-archive over the API,
	// then delete succeeds on the archived budget.
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/unarchive-budget", tok, map[string]any{"id": budgetID1}))
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": budgetID1})
	res := mustUnmarshal[metaItemOut](t, env.Data)
	if st != http.StatusOK || res.Item.IsArchived != 1 {
		t.Fatalf("archive: status=%d meta=%+v", st, res.Item)
	}
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": budgetID1})) // idempotent
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/delete-budget", tok, map[string]any{"id": budgetID1}))
}
```

Also add: `TestArchiveBudget_RequiresOwnerOrAdmin` (a `user`-role caller gets 403 on archive/unarchive — reuse the grant/accept flow from `membership_test.go`), and an accept-on-archived case: owner grants access BEFORE archiving, archives, the invitee's `accept-access` still succeeds, and `revoke-access`/`decline-access` succeed on the archived budget. Adapt constants (`folderID1`, `envelopeID1`, `otherUserID`, `accountID2`, `catID`) to what the harness declares — grep first; the update-budget body uses valid current name/currency so ONLY the archived guard can 403 it. UUID constants that don't exist yet get added next to the existing ones.

- [ ] **Step 2: Run** — → FAIL (routes missing; writes succeed on archived).

- [ ] **Step 3: Implement**

`internal/budget/lifecycle.go`:

```go
// Lifecycle use cases: archive-budget / unarchive-budget. Archived = hidden +
// read-only: requireNotArchived blocks every budget write except the
// allowlist (unarchive, delete, revoke, decline, accept — and archive itself,
// which is an idempotent no-op on an archived budget).
package budget

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) requireNotArchived(b *budgetAggregate) error {
	if b.budget.IsArchived {
		return &errs.AccessDeniedError{Msg: "This budget is archived", Code: errs.CodeBudgetArchived}
	}
	return nil
}

func (s *Service) ArchiveBudget(ctx context.Context, userID vo.Id, req model.ArchiveBudgetRequest) (*model.ArchiveBudgetResult, error) {
	meta, err := s.setArchived(ctx, userID, req.Id, true)
	if err != nil {
		return nil, err
	}
	return &model.ArchiveBudgetResult{Item: meta}, nil
}

func (s *Service) UnarchiveBudget(ctx context.Context, userID vo.Id, req model.UnarchiveBudgetRequest) (*model.UnarchiveBudgetResult, error) {
	meta, err := s.setArchived(ctx, userID, req.Id, false)
	if err != nil {
		return nil, err
	}
	return &model.UnarchiveBudgetResult{Item: meta}, nil
}

func (s *Service) setArchived(ctx context.Context, userID vo.Id, rawID string, archived bool) (model.MetaResult, error) {
	budgetID, err := vo.ParseId(rawID)
	if err != nil {
		return model.MetaResult{}, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return model.MetaResult{}, err
	}
	if !s.canDelete(b, userID) {
		return model.MetaResult{}, accessDenied()
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if archived {
			b.budget.Archive(s.clock.Now())
		} else {
			b.budget.Unarchive(s.clock.Now())
		}
		return s.budgets.Save(txCtx, b.budget)
	}); err != nil {
		return model.MetaResult{}, err
	}
	return s.reloadMeta(ctx, budgetID)
}
```

(`canDelete`/`accessDenied`/`reloadMeta` are existing helpers — grep for exact receivers/signatures and adapt; `canDelete` may be a free function over the aggregate.)

DTOs in `budget_dto.go` (next to the other request/result pairs, `Validate()` NotBlank on `Id` in the file's style):

```go
type ArchiveBudgetRequest struct{ Id string `json:"id"` }
type ArchiveBudgetResult struct{ Item MetaResult `json:"item"` }
type UnarchiveBudgetRequest struct{ Id string `json:"id"` }
type UnarchiveBudgetResult struct{ Item MetaResult `json:"item"` }
```

Handlers + routes: mirror the `AddAccount` handler's `endpoint.Handle` shape and swag block (`@Summary Archive a budget` / `Unarchive a budget`, `@Router /api/v1/budget/archive-budget [post]` / `unarchive-budget`). Register both in `routes.go`.

Guard calls: in each use case from the Interfaces list, immediately after the aggregate is loaded and the role check passes, add `if err := s.requireNotArchived(b); err != nil { return nil, err }` (adjust returns to each signature; in `membershipPrelude` it covers add/remove-account in one place). ResetBudget/DeleteEnvelope/GrantAccess have their own guards — same placement rule.

Run `make swagger`.

- [ ] **Step 4: Run** — `go test ./internal/budget/api/ -run 'Archived|Archive' -v` then `go test $(go list ./... | grep -v '/test/apiparity' | grep -v '/test/mcpparity')` → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(budget): archived state — archive/unarchive endpoints, write guard with allowlist"` (with trailer).

---

### Task 4: End-date semantics — period clamps, set-limit guard, removal boundary

**Files:**
- Modify: `internal/budget/read.go` (`GetBudget`), `internal/budget/plan.go` + `builder_plan.go` (window), `internal/budget/txlist.go` (`GetTransactionList`), `internal/budget/accounts.go` (`SetLimit`, `removableAccounts`)
- Test: extend `internal/budget/api/lifecycle_test.go`

**Interfaces:**
- Produces: unexported helper on the aggregate — `func (b *budgetAggregate) endMonth() *time.Time` returning `FirstOfMonth(*b.budget.EndedAt)` or nil. Clamp rules (spec §2): get-budget and get-transaction-list snap a requested period **later** than the end month down to it; get-budget-plan clamps `from` the same way and caps `windowEnd` at `endMonth + 1 month`; set-limit refuses `period > endMonth` with blank-validation on `period` (mirror of the started_at guard); `removableAccounts`' window end becomes `min(localMonth(now, loc), endMonth + 1 month)`.

- [ ] **Step 1: Failing tests** — append to `lifecycle_test.go`:

```go
func TestEndedBudget_Clamps(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust()) // the fixed-clock helper from membership_test.go
	tok := h.token(t)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "2026-05-01"}))

	// get-budget for August clamps to May
	_, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-15", tok, nil)
	fl := mustUnmarshal[periodsOut](t, env.Data) // {Item{Filters{PeriodStart, PeriodEnd string}}} — declare locally
	if fl.Item.Filters.PeriodStart != "2026-05-01 00:00:00" {
		t.Fatalf("periodStart=%q want clamped to 2026-05-01", fl.Item.Filters.PeriodStart)
	}
	// within range is untouched
	_, env = h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-03-15", tok, nil)
	if fl = mustUnmarshal[periodsOut](t, env.Data); fl.Item.Filters.PeriodStart != "2026-03-01 00:00:00" {
		t.Fatalf("in-range periodStart=%q", fl.Item.Filters.PeriodStart)
	}

	// set-limit after the end month → 400 on period; at the end month → 200
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-06-01", "amount": "10"})
	if st != http.StatusBadRequest {
		t.Fatalf("set-limit past end accepted: %s", env.raw)
	}
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-05-01", "amount": "10"}))

	// plan window ends at the end month: from=2026-04, months=12 → periods 04 and 05 only
	_, env = h.do(t, http.MethodGet, "/api/v1/budget/get-budget-plan?id="+budgetID1+"&from=2026-04-01&months=12", tok, nil)
	pl := mustUnmarshal[planPeriodsOut](t, env.Data) // {Item{Periods []string}} — adapt to the real plan DTO (grep GetBudgetPlanResult)
	if len(pl.Item.Periods) != 2 {
		t.Fatalf("plan periods=%v want [2026-04, 2026-05]", pl.Item.Periods)
	}

	// get-transaction-list for August clamps to May (the response echoes/uses the period —
	// assert via the period-scoped items: seed a May transaction and an August transaction
	// on the member account; the clamped call returns the May one)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "7", SpentAt: "2026-05-10 12:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "9", SpentAt: "2026-08-10 12:00:00"})
	_, env = h.do(t, http.MethodGet, "/api/v1/budget/get-transaction-list?budgetId="+budgetID1+"&periodStart=2026-08-01", tok, nil)
	if !strings.Contains(string(env.Data), `"7"`) || strings.Contains(string(env.Data), `"9"`) {
		t.Fatalf("txlist not clamped: %s", env.Data)
	}
}

func TestEndedBudget_RemovalBoundaryHonoursEnd(t *testing.T) {
	// A transaction AFTER the end month must not lock the account: window end is
	// min(current month, end+1). Budget ends 2026-05; a July transaction exists;
	// with now=August the account stays removable.
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "endDate": "2026-05-01"}))
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2026-07-10 12:00:00"})
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/remove-account", tok, map[string]any{"id": budgetID1, "accountId": accountID}))
	// but a transaction INSIDE [start, end] still locks
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/add-account", tok, map[string]any{"id": budgetID1, "accountId": accountID}))
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2026-04-10 12:00:00"})
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/remove-account", tok, map[string]any{"id": budgetID1, "accountId": accountID})
	if st != http.StatusBadRequest {
		t.Fatalf("in-range history did not lock: %s", env.raw)
	}
}
```

Declare the small local unmarshal structs (`periodsOut`, `planPeriodsOut`) against the REAL DTO field names — open `model.GetBudgetPlanResult`/`FiltersResult` first and adapt; the assertions' substance is binding, the struct spellings are not.

- [ ] **Step 2: Run** — → FAIL.

- [ ] **Step 3: Implement**

Add to `usecase.go` (near `budgetAggregate`):

```go
// endMonth is the budget's last covered month (first-of-month), nil when open-ended.
func (b *budgetAggregate) endMonth() *time.Time {
	if b.budget.EndedAt == nil {
		return nil
	}
	m := model.FirstOfMonth(*b.budget.EndedAt)
	return &m
}
```

- `read.go` `GetBudget`: after the aggregate is loaded, `if em := b.endMonth(); em != nil && period.After(*em) { period = *em }` (place it where `period` and `b` are both in scope; the clamped period flows into the builder).
- `plan.go`/`builder_plan.go`: clamp `from` the same way; in `BuildBudgetPlan` cap `windowEnd`: `if em := b.endMonth(); em != nil { if cap := em.AddDate(0, 1, 0); cap.Before(windowEnd) { windowEnd = cap } }` (adapt to where `windowEnd` is computed — `builder_plan.go:22`).
- `txlist.go` `GetTransactionList`: after `periodStart = model.FirstOfMonth(periodStart)` (line ~60), clamp against the loaded budget's end month (the use case loads the budget for access checks — reuse that; if it only has the row not the aggregate, compare against `budget.EndedAt` directly with `FirstOfMonth`).
- `accounts.go` `SetLimit`: after the started_at guard (lines ~225-227): `if em := b.endMonth(); em != nil && period.After(*em) { return nil, model.ValidateBlank(map[string]string{"period": ""}) }`.
- `accounts.go` `removableAccounts` (line ~28): `end := localMonth(now, reqctx.Location(ctx)); if em := b.endMonth(); em != nil { if cap := em.AddDate(0, 1, 0); cap.Before(end) { end = cap } }`.

- [ ] **Step 4: Run** — `go test ./internal/budget/... -run 'EndedBudget|Membership|Budget'` then the non-parity suite → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(budget): ended_at semantics — period clamps, set-limit ceiling, removal boundary"` (with trailer).

---

### Task 5: Clone

**Files:**
- Create: `internal/budget/clone.go`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql` (`ListBudgetLimitsFrom`) + regenerate; `internal/budget/repo/{engine.go,repo.go}`, `internal/budget/repository.go` (`LimitStore` gains the method)
- Modify: `internal/model/budget_dto.go` (Clone DTOs), `internal/budget/api/{budget_more.go,routes.go}`
- Test: `internal/budget/api/clone_test.go` (new)
- Run: `make swagger`

**Interfaces:**
- Produces: `(*Service).CloneBudget(ctx, userID, model.CloneBudgetRequest) (*model.CloneBudgetResult, error)`; `model.CloneBudgetRequest{Id, NewId, Name, StartDate string; WithLimits bool}` with JSON tags `id,newId,name,startDate,withLimits` (Validate: Id + NewId NotBlank, Name via the same rule create-budget uses); `CloneBudgetResult{Item BudgetResult \`json:"item"\`}` — built for the caller's current month exactly like `CreateBudget`'s result (mirror its tail); `LimitStore.ListLimitsFrom(ctx, budgetID vo.Id, from time.Time) ([]*model.BudgetLimit, error)` (adapt the model type name to what `hydrateLimit` returns).
- Rules (spec §4, binding): caller must be the source's **owner** (`budgetRole == BudgetRoleOwner`; admin/user/guest → 403). Source may be archived or ended — NO `requireNotArchived` on the source. `startDate`: empty → source `StartedAt`; else `datetime.DateLayout`, snapped `FirstOfMonth`, must satisfy `source.StartedAt ≤ sd ≤ localMonth(now)` else blank-validation on `startDate` (the source's `ended_at` does NOT constrain it). One `WithTx`, fresh `vo.NewId()` for every row, old→new id map. Copy: budget row (`ended_at = NULL`, `is_archived = false`, `user_id = caller`, timestamps `now`); every `budgets_access` row (same user/role/is_accepted, timestamps `now`); every `budgets_accounts` row (`created_at = now`); folders (name, sort key); envelopes + their category links; elements — same `type`, folder remapped, currency, sort key, and `external_id` KEPT for category/tag elements but **REMAPPED through the envelope id map for envelope-type elements** (their external_id references the budget-scoped `budgets_envelopes.id`); limits only when `WithLimits`, only `period ≥ startDate`, element remapped, same period/amount, timestamps `now`.

- [ ] **Step 1: sqlc query (both engines)**

Append to `query/sqlite/budgets.sql` (pgsql `$1/$2`):

```sql
-- name: ListBudgetLimitsFrom :many
SELECT l.id, l.element_id, l.period, l.created_at, l.updated_at, l.amount
FROM budgets_elements_limits l
JOIN budgets_elements e ON e.id = l.element_id
WHERE e.budget_id = ? AND l.period >= ?
ORDER BY l.period, l.id;
```

Regenerate. Repo: add to the `querier` + both adapters + `Repo` + `LimitStore` interface. CRITICAL: mirror EXACTLY how the existing limit queries bind/compare `period` on sqlite (the repo hand-binds it as a `'Y-m-d H:i:s'` string in `engine.go` ~lines 147-162 because `datetime()` text comparison is layout-sensitive) — open `ListBudgetLimitsForPeriod`'s engine handling and copy its treatment; if that one goes through raw SQL, this one does too.

- [ ] **Step 2: Failing clone tests**

`internal/budget/api/clone_test.go` — the assertions are binding; adapt DTO-shape structs by opening `model.BudgetResult` first:

```go
func TestCloneBudget_FullCopyEquivalence(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	// source: budget from 2026-01, folder, envelope over catID, a limit in March and one in July
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Src", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{"budgetId": budgetID1, "id": folderID1, "name": "F"}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{"budgetId": budgetID1, "id": envelopeID1, "name": "E", "currencyId": usdID, "categories": []string{catID}}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-03-01", "amount": "100"}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-07-01", "amount": "50"}))
	// share it (accepted user) so access copying is observable
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: "s"})
	h.f.Connect(seedUserID, otherUserID)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1}))

	// full backup: startDate absent, withLimits true
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok, map[string]any{"id": budgetID1, "newId": budgetID2, "name": "Copy", "withLimits": true})
	if st != http.StatusOK {
		t.Fatalf("clone: status=%d body=%s", st, env.raw)
	}
	// the result is the create-budget shape for the current month; the copy's meta
	// shows name, startedAt = source start, endedAt "", isArchived 0. metaOut is
	// local to this file: {item: {meta: {endedAt, isArchived}}} (BudgetResult).
	meta := mustUnmarshal[metaOut](t, env.Data)
	if meta.Item.Meta.EndedAt != "" || meta.Item.Meta.IsArchived != 0 {
		t.Fatalf("copy meta: %+v", meta.Item.Meta)
	}
	// structural equivalence via raw rows: counts match, ids all fresh, envelope
	// element external_id remapped to the NEW envelope id
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_access WHERE budget_id = ?`, budgetID2).Scan(&n)
	if n != 1 {
		t.Fatalf("access rows=%d want 1", n)
	}
	var accepted bool
	h.db.QueryRow(`SELECT is_accepted FROM budgets_access WHERE budget_id = ? AND user_id = ?`, budgetID2, otherUserID).Scan(&accepted)
	if !accepted {
		t.Fatal("accepted state not copied")
	}
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, budgetID2, accountID).Scan(&n)
	if n != 1 {
		t.Fatal("membership not copied")
	}
	var newEnvID string
	h.db.QueryRow(`SELECT id FROM budgets_envelopes WHERE budget_id = ?`, budgetID2).Scan(&newEnvID)
	if newEnvID == "" || newEnvID == envelopeID1 {
		t.Fatalf("envelope id not fresh: %q", newEnvID)
	}
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND type = 2 AND external_id = ?`, budgetID2, newEnvID).Scan(&n)
	if n != 1 {
		t.Fatal("envelope element external_id not remapped to the new envelope id")
	} // adapt type value: open the element-type constants (grep ElementType) — the envelope type's stored int is what matters
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ?`, budgetID2).Scan(&n)
	if n != 2 {
		t.Fatalf("limits=%d want 2 (full backup copies all)", n)
	}
}

func TestCloneBudget_StartDateAndLimitsFilters(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Src", "currencyId": usdID, "startDate": "2026-01-01", "accountIds": []string{accountID}}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-03-01", "amount": "100"}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-08-01", "amount": "50"}))

	// continuation: startDate = this month → only the August limit copies
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok, map[string]any{"id": budgetID1, "newId": budgetID2, "name": "Cont", "startDate": "2026-08-01", "withLimits": true}))
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ?`, budgetID2).Scan(&n)
	if n != 1 {
		t.Fatalf("limits=%d want 1 (period >= startDate only)", n)
	}
	// withLimits=false → none
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok, map[string]any{"id": budgetID1, "newId": budgetID3, "name": "Bare", "withLimits": false}))
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ?`, budgetID3).Scan(&n)
	if n != 0 {
		t.Fatalf("limits=%d want 0", n)
	}
	// startDate before source start → 400; after this month → 400
	st, _ := h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok, map[string]any{"id": budgetID1, "newId": vo.NewId().String(), "name": "X", "startDate": "2025-12-01", "withLimits": false})
	if st != http.StatusBadRequest {
		t.Fatal("pre-start startDate accepted")
	}
	st, _ = h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok, map[string]any{"id": budgetID1, "newId": vo.NewId().String(), "name": "X", "startDate": "2026-09-01", "withLimits": false})
	if st != http.StatusBadRequest {
		t.Fatal("future startDate accepted")
	}
}

func TestCloneBudget_OwnerOnly_ArchivedSourceAllowed(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Src")))
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: "s"})
	h.f.Connect(seedUserID, otherUserID)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "admin"}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1}))
	// admin cannot clone
	st, _ := h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", otherUserID, map[string]any{"id": budgetID1, "newId": budgetID2, "name": "X", "withLimits": false})
	if st != http.StatusForbidden {
		t.Fatalf("admin clone: status=%d", st)
	}
	// archived source clones fine
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/archive-budget", tok, map[string]any{"id": budgetID1}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/clone-budget", tok, map[string]any{"id": budgetID1, "newId": budgetID2, "name": "Copy", "withLimits": false}))
}
```

Add `budgetID3` next to the harness UUID constants if absent.

- [ ] **Step 3: Run** — → FAIL (route missing).

- [ ] **Step 4: Implement**

`internal/budget/clone.go` — the use case: parse ids; `loadAggregate(source)`; `if role, err := s.budgetRole(b, userID); err != nil || role != model.BudgetRoleOwner { return nil, accessDenied() }` (adapt to the real `budgetRole` shape); resolve `startDate` per the rules; inside one `WithTx`:

1. `now := s.clock.Now()`; `nb := model.NewBudget(newID, userID, req.Name, b.budget.CurrencyID, startDate, now)`; `s.budgets.Save(txCtx, nb)`.
2. Access: for each `b.access` row build a fresh `model.BudgetAccess` for the new budget (same UserID/Role/IsAccepted, timestamps now) and save via the store method the grant/accept flow uses (grep `AccessStore` — `Save`/`Upsert`).
3. Accounts: `for _, m := range b.accounts { s.budgets.AddAccount(txCtx, newID, m.AccountID, now) }`.
4. Folders: id map `folderMap[old]=new`; construct via the model constructor `CreateFolder` uses, same Name/SortKey, save.
5. Envelopes: id map `envMap[old]=new`; same fields (Name/Icon/IsArchived); save; then copy each envelope's category links (`ListEnvelopeCategoryIDs` on the old id → `AddEnvelopeCategory` on the new).
6. Elements: fresh ids; FolderID remapped through `folderMap` (nil stays nil); `ExternalID`: if the element's type is the envelope type → `envMap[old external]`; else unchanged. Same Type/CurrencyID/SortKey.
7. Limits (only `req.WithLimits`): `s.limits.ListLimitsFrom(txCtx, sourceID, startDate)` → for each, fresh id, `ElementID` remapped through the element id map, same Period/Amount, timestamps now, save via the store method `SetLimit` uses.

Return: mirror `CreateBudget`'s tail — reload the new aggregate and build `BudgetResult` for the caller's current month (`localMonth(s.clock.Now(), reqctx.Location(ctx))`), wrap in `CloneBudgetResult`.

DTOs + `Validate()` (Id/NewId NotBlank; Name via the same `ValidateName` create-budget uses). Handler + route `POST /api/v1/budget/clone-budget` with swag block (`@Summary Clone a budget`). `make swagger`.

- [ ] **Step 5: Run** — `go test ./internal/budget/... -run Clone -v` then the non-parity suite → PASS.

- [ ] **Step 6: Commit** — `git commit -am "feat(budget): clone-budget — owner-only deep copy with id remap and optional plans"` (with trailer).

---

### Task 6: MCP tools + mcpparity goldens

**Files:**
- Modify: `internal/budget/mcp/mcp.go`, `internal/test/mcpparity/catalogue.go`, goldens under `internal/test/mcpparity/testdata/golden/`

- [ ] **Step 1: Tools**

Follow the `add_budget_account` registration shape exactly (`mcp.go:368-380` pattern: `reqctx.AddLogAttr`, `webmcp.UserID`, service call, `webmcp.MapErr`):

- `updateBudgetInput` gains `EndDate *string \`json:"end_date,omitempty" jsonschema:"end month YYYY-MM-DD; omit to leave unchanged, empty string to clear"\`` — passed straight into `model.UpdateBudgetRequest.EndDate`.
- `clone_budget`: input `{BudgetID string \`json:"budget_id" ...\`; NewID string \`json:"new_id" jsonschema:"client-supplied id (UUID) for the copy"\`; Name string \`json:"name"\`; StartDate string \`json:"start_date,omitempty" jsonschema:"first month YYYY-MM-DD; omit for a full copy from the source's start"\`; WithLimits bool \`json:"with_limits" jsonschema:"copy the plans (limits) too"\`}` → `svc.CloneBudget` → returns `model.CloneBudgetResult`. Description: "Clone a budget you own: structure, sharing and accounts are copied; optionally the plans. Start from the source's start (full copy) or a later month (continuation)."
- `archive_budget` / `unarchive_budget`: input `{BudgetID string \`json:"budget_id" ...\`}` → `svc.ArchiveBudget`/`UnarchiveBudget` → `model.ArchiveBudgetResult`/`UnarchiveBudgetResult`. Descriptions: "Archive a budget: hidden and read-only until unarchived." / "Unarchive a budget."

- [ ] **Step 2: Catalogue + goldens**

Add a new scenario `budget_lifecycle` to `internal/test/mcpparity/catalogue.go` (REST-seed a budget exactly like the `budget_write` scenario does, then): `update_budget` with `end_date` → `archive_budget` → an expected-error write on the archived budget (e.g. `set_limit` — asserts the coded 403 rendering through MapErr) → `unarchive_budget` → `clone_budget` (capture the new id) → `get_budget` of the copy. Then:

```bash
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/ && git diff --stat internal/test/mcpparity/testdata
```

INSPECT: expected diffs are `tools/list` (three new tools + `end_date` schema), `endedAt`/`isArchived` appearing in every budget-meta block of existing goldens, and the new `budget_lifecycle.golden`. Any numeric drift in pre-existing scenarios is a bug — stop and investigate.

- [ ] **Step 3: Run** — `go test ./internal/test/mcpparity/` → PASS; `go build ./... && go vet ./...` clean.

- [ ] **Step 4: Commit** — `git commit -am "feat(mcp): clone/archive/unarchive budget tools, update_budget end_date; goldens"` (with trailer).

---

### Task 7: apiparity catalogue + goldens

**Files:**
- Create: `internal/test/apiparity/catalogue_budget_lifecycle.go`
- Modify: `internal/test/apiparity/catalogue_test.go` (raise the floor), goldens

- [ ] **Step 1: Scenario**

Register `budget_lifecycle` (Call/Scenario shapes per `catalogue_budget_access.go`): against the fixture's `Budget2` (so the heavily-asserted `Budget` scenarios stay untouched):

1. `update-budget` setting `endDate` (a month ≥ Budget2's start) → 200;
2. `err:update-budget-end-before-start` with `endDate` before the start → 400;
3. `archive-budget` → 200; 4. `err:set-limit-archived` (a write on the archived Budget2) → 403; 5. `unarchive-budget` → 200;
6. `clone-budget` `{id: Budget2, newId: <a fixed new UUID constant>, name: "Copy", withLimits: true}` → 200; 7. `get-budget` of the clone (fixed id, so no capture machinery needed) → 200;
8. `err:clone-budget-not-owner` — `Auth: "guest"` cloning `Budget2` → 403;
9. `update-budget` clearing `endDate` (`""`) → 200.

Add the new-clone UUID as a fixture-style constant in the scenario file. Raise `const min` in `catalogue_test.go` by 1 (with an accurate comment). The route-scan guard picks the three new routes up automatically — `minRoutes` (112) still holds as a floor.

- [ ] **Step 2: Regenerate + inspect**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/golden
```

Expected: every budget-meta-bearing golden gains `endedAt`/`isArchived` keys (shape only), plus the new `budget_lifecycle.golden`. NUMBERS in pre-existing scenarios must not change — if any amount moves, stop and investigate before committing.

- [ ] **Step 3: Run** — `go test ./internal/test/apiparity/` → PASS; `make go-lint` → PASS; full `go test ./...` → PASS (everything is green again from here).

- [ ] **Step 4: Commit** — `git commit -am "test(apiparity): budget lifecycle scenario; meta goldens gain endedAt/isArchived"` (with trailer).

---

### Task 8: SPA — DTOs, api, hooks, METRICS, BudgetsPage actions + archived group, i18n keys

**Files:**
- Modify: `web/src/api/dto/budget.ts`, `web/src/api/budget.ts`, `web/src/features/budgets/queries.ts`, `web/src/lib/metrics.ts`, `web/src/features/budgets/BudgetsPage.tsx`
- Modify: all 11 `locales/<lang>.json` (every new UI key below)
- Test: `web/src/features/budgets/BudgetsPage.test.tsx`, `queries.test.tsx`

**Interfaces:**
- Produces: `BudgetMetaDto` gains `endedAt: string` and `isArchived: 0 | 1`; api functions `cloneBudget(form: CloneBudgetForm)` (`CloneBudgetForm = { id: Id; newId: Id; name: string; startDate: string | null; withLimits: boolean }`, POST `/api/v1/budget/clone-budget`, body sends `startDate` only when non-null), `archiveBudget(id: Id)`, `unarchiveBudget(id: Id)`; `UpdateBudgetForm` gains `endDate?: string` (absent = untouched; `""` = clear); hooks `useCloneBudget()`, `useArchiveBudget()`, `useUnarchiveBudget()` (each invalidates `queryKeys.budgets` + `queryKeys.budget` and fires its metric in `onSuccess`, mirroring `useDeleteBudget`); `useUpdateBudgetDetail` additionally fires `METRICS.BUDGET_SET_END_DATE` when `variables.endDate !== undefined`.
- METRICS additions (`web/src/lib/metrics.ts`, exact values are the spec's): `BUDGET_CLONE: 'appBudgetCloned'`, `BUDGET_COMPLETE: 'appBudgetCompleted'`, `BUDGET_ARCHIVE: 'appBudgetArchived'`, `BUDGET_UNARCHIVE: 'appBudgetUnarchived'`, `BUDGET_SET_END_DATE: 'appBudgetEndDateSet'`. `BUDGET_COMPLETE` fires from the Complete flow (Task 9) — reference it there; the coverage test requires every key referenced, so add `BUDGET_COMPLETE` in Task 9's commit OR add all five now and wire `BUDGET_COMPLETE` provisionally into the (not-yet-shown) complete handler stub — simplest: add the four wired keys now, add `BUDGET_COMPLETE` in Task 9 together with its call site.
- BudgetsPage: the per-row `DropdownMenu` (lines ~111-139) gains, for `hasBudgetAdminAccess` rows: **Duplicate…** (owner only — `budget.ownerUserId === myId`), **Complete…** (owner|admin, hidden when `isArchived === 1`), **Archive** (`isArchived === 0`) / **Unarchive** (`isArchived === 1`); Duplicate/Complete only OPEN dialogs here — the dialogs land in Task 9; wire Archive/Unarchive to the hooks now. The flat list splits: active budgets first (current sort), then a collapsed **Archived** group (`isArchived === 1`) with a toggle header showing the count — collapsed by default, plain `useState`.
- i18n — add in ALL 11 catalogues (exact `en` below; translate the rest in-style; key placement follows the existing `budgets.page.settings.list_actions.*` / `budgets.modal.*_form.*` conventions):

| key | en |
|---|---|
| `budgets.page.settings.list_actions.duplicate` | Duplicate… |
| `budgets.page.settings.list_actions.complete` | Complete… |
| `budgets.page.settings.list_actions.archive` | Archive |
| `budgets.page.settings.list_actions.unarchive` | Unarchive |
| `budgets.page.settings.archived_group.header` | Archived |
| `budgets.page.budget.archived_banner` | This budget is archived and read-only |
| `budgets.modal.duplicate_budget_form.header` | Duplicate budget |
| `budgets.modal.duplicate_budget_form.copy_plans` | Copy plans |
| `budgets.modal.duplicate_budget_form.start_this_month` | Start from this month |
| `budgets.modal.complete_budget_form.header` | Complete budget |
| `budgets.modal.complete_budget_form.end_month` | Last month |
| `budgets.modal.complete_budget_form.continue_copy` | Continue in a copy starting this month |
| `budgets.modal.complete_budget_form.copy_plans` | Copy plans |
| `budgets.form.budget.end_month.label` | End month |
| `budgets.form.budget.end_month.none` | No end date |

(Task 9/10 consume the modal/banner keys; adding them here keeps the catalogue work in one commit. The i18ntest frontend guard fails only on used-but-missing keys, so unused-until-Task-9 is safe.)

- [ ] **Step 1: Failing tests** — `queries.test.tsx`: `useArchiveBudget` posts to `/api/v1/budget/archive-budget` with `{id}` and invalidates budgets; `useCloneBudget` posts the full form (`startDate` omitted when null). `BudgetsPage.test.tsx`: a meta list containing one archived budget renders it under a collapsed "Archived" header (not in the main list until expanded); the actions menu shows Unarchive for it and Archive for a live one; clicking Archive calls the mutation. Follow the existing test-file rendering/msw patterns.
- [ ] **Step 2: Run** — `cd web && pnpm vitest run budgets` → FAIL.
- [ ] **Step 3: Implement** per the Interfaces block. Dto int-bool convention: `isArchived: 0 | 1` exactly like `BudgetChildElementDto`.
- [ ] **Step 4: Run** — `cd web && pnpm vitest run budgets && pnpm lint` → PASS; `go test ./internal/test/i18ntest/` → PASS.
- [ ] **Step 5: Commit** — `git commit -am "feat(web): budget archive/unarchive + archived group; lifecycle api, hooks, metrics, i18n"` (with trailer).

---

### Task 9: SPA — Duplicate and Complete dialogs, End-month field

**Files:**
- Create: `web/src/features/budgets/DuplicateBudgetDialog.tsx`, `web/src/features/budgets/CompleteBudgetDialog.tsx` (+ tests for both)
- Modify: `web/src/features/budgets/BudgetsPage.tsx` (mount + wire the dialogs), `web/src/features/budgets/BudgetUpdateDialog.tsx` (+ test), `web/src/lib/metrics.ts` (`BUDGET_COMPLETE`)

**Interfaces:**
- `DuplicateBudgetDialog` props `{ budget: BudgetMetaDto | null; onClose: () => void }`: name Input (prefilled `budget.name`), "Copy plans" Switch (default on), "Start from this month" Switch (default off); submit → `useCloneBudget().mutate({ id, newId: crypto.randomUUID(), name, startDate: startThisMonth ? currentMonth() + '-01' : null, withLimits: copyPlans })` (reuse `currentMonth()` from `planMath.ts`), close on success. `ResponsiveDialog` + `CardField` + footer per `BudgetDialog`'s conventions; Switch import per `ClassificationList`.
- `CompleteBudgetDialog` props `{ budget: BudgetMetaDto | null; onClose: () => void }`: **End month** `Select` (options: every month from `budget.startedAt`'s month through the current month, newest first, default = last month — clamp to the start month when the budget started this month; label months with the same formatter `PeriodStrip` uses); "Continue in a copy starting this month" Switch (default on); when on: "Copy plans" Switch (default on) + name Input (prefilled). Submit runs SEQUENTIALLY with `mutateAsync`, stopping on the first failure (each step is a valid end state; failures surface the standard toast the hooks already produce): `updateBudgetDetail({ id, name: budget.name, currencyId: budget.currencyId, endDate })` → `archiveBudget(id)` → if continuing: `cloneBudget({...startDate: thisMonth, withLimits: copyPlans, name, newId})` → `updateDefaultBudget(newId)` → `navigate(RouterPage.BUDGET)`; fire `trackEvent(METRICS.BUDGET_COMPLETE)` after the whole sequence succeeds. Add `BUDGET_COMPLETE: 'appBudgetCompleted'` to METRICS in this commit (coverage test needs the reference to exist together with the key).
- `BudgetUpdateDialog`: an **End month** `CardField` — a `Select` with a `budgets.form.budget.end_month.none` option ("" value) plus the months from the start month through the current month; initial value from `budget.filters`-independent `meta.endedAt` (slice to month); include `endDate` in the mutation body ONLY when changed (absent otherwise — the wire treats absent as untouched).
- Tests (vitest + msw): Duplicate sends the right body for both toggle states; Complete full flow issues the four calls in order and stops after a failed `archive-budget` (assert `clone-budget` never fired); update dialog sends `endDate` only when the field changed, and `""` when cleared.

- [ ] **Step 1: Failing tests** (as above; follow `BudgetUpdateDialog.test.tsx` msw body-capture style)
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — `cd web && pnpm vitest run budgets && pnpm lint` → PASS.
- [ ] **Step 5: Commit** — `git commit -am "feat(web): duplicate and complete budget flows; end-month field"` (with trailer).

---

### Task 10: SPA — archived read-only BudgetPage, period upper bounds

**Files:**
- Modify: `web/src/features/budgets/BudgetPage.tsx`, `PeriodStrip.tsx`, `budgetMath.ts` (`periodRange`), `planMath.ts`, `PlanSheet.tsx`
- Test: `BudgetPage.test.tsx`, `PeriodStrip.test.tsx`, `budgetMath.test.ts`, `planMath.test.ts`

**Interfaces:**
- `periodRange` gains an `endMonth: string | null` parameter (from `meta.endedAt.slice(0, 7)` or null): `outsideBudget` becomes true when the month is before the start OR after the end (same greyed styling both directions).
- `planMath`: `clampFirstMonth(firstMonth, startedAt, endedAt?)` also clamps down so the visible window never starts past the end month; `PlanSheet` caps its visible months so no column exceeds the end month, and the `nav.next` control disables at the boundary (mirror how the start boundary behaves).
- `BudgetPage`: when `meta.isArchived === 1`, render an `InfoBox` (the `BudgetsPage` InfoBox pattern) with `t('budgets.page.budget.archived_banner')` above the table, and force the read-only path: reuse the existing role-gating — compute `readOnly = archived || !canEditBudget(...)` and thread it exactly where `editDetails`/`configure`/`limitsEditable` gate menus and editors (archived wins over role).

- [ ] **Step 1: Failing tests** — `budgetMath.test.ts`: `periodRange` marks months after the end as `outsideBudget`; `planMath.test.ts`: clamp cases; `BudgetPage.test.tsx`: archived meta renders the banner and no limit editor opens (assert the budgeted cell is not editable — mirror how the guest-role read-only case is asserted).
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — `cd web && pnpm test && pnpm lint && pnpm build` → PASS (modulo the known pre-existing `transaction.test.ts` Blob failure); `go test ./internal/test/i18ntest/` → PASS.
- [ ] **Step 5: Commit** — `git commit -am "feat(web): archived budget read-only view; period bounds honour the end month"` (with trailer).

---

### Task 11: Docs, full verification, PR

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: CLAUDE.md** — under **Notable behaviours**, append to the budget-membership bullet block a new bullet:
  "**Budget lifecycle**: a budget may carry an end month (`ended_at`, inclusive first-of-month; `endedAt` on the wire, `\"\"` when unset) and an archived flag (`isArchived` 0/1). Archived = hidden + read-only: every budget write returns a coded 403 `budget.archived` except unarchive/delete/revoke/decline/accept (and archive itself, idempotent). Readers clamp periods to the end month; `set-limit` refuses periods past it; the membership removal window ends at `min(current month, end month + 1)`. `clone-budget` is an owner-only single-transaction deep copy (structure, sharing, membership, optionally plans from a chosen start month; envelope elements' `external_id` remapped) — the copy starts open-ended and unarchived."
- [ ] **Step 2: Full suite** — `make go-test` and `make test` (Postgres via compose or `DATABASE_TEST_PGSQL_URL`; the pre-existing `web/src/api/transaction.test.ts` Blob failure is the only accepted web-tier failure — anything else is real). Expected: PASS.
- [ ] **Step 3: Commit** — `git commit -am "docs: budget lifecycle"` (with trailer).
- [ ] **Step 4: Handoff** — push `feature/budget-lifecycle` and open a PR **against `bug/budget-account-membership`** (it builds on the membership branch; re-target to `main` after #218 merges) titled "feat: budget lifecycle — end date, archive, clone", body summarizing the spec's Design summary + the golden diff areas.

---

## Self-review notes (spec coverage)

- Spec §1 data model → Task 1. §2 end-date semantics → Task 4 (all four consumers). §2 archived semantics → Task 3 (guard + allowlist + banner note; SPA read-only in Task 10). §3 wire contract → Tasks 2-3 (meta fields, endDate, archive routes; coded errors). §4 clone → Task 5 (incl. the envelope `external_id` remap the spec now records). §5 frontend → Tasks 8-10. §6 MCP → Task 6. §7 i18n → Tasks 2 (errors) + 8 (UI copy). §8 testing → distributed: clone equivalence (T5), archived matrix (T3), end-date clamps + removal boundary (T4), goldens (T6-7), SPA (T8-10), engine parity (T11).
- Out of scope per spec §9: start-date changes, non-owner clone, auto-archive on end — no task touches them.
