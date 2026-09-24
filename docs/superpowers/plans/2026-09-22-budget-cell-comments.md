# Budget Cell Comments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every budget cell (one budget element x one month) carries a flat, shared, plain-text comment thread, readable and writable from both the plan view and the monthly budget view.

**Architecture:** A new `budgets_elements_comments` table cascading from `budgets_elements`, reached through a `CommentStore` role interface on the existing budget repository (sqlc, engine-adapter pattern). Four new REST routes and four MCP tools sit on four new use cases in `internal/budget/comments.go`; `get-budget` and `get-budget-plan` are untouched, so the SPA fetches threads through one windowed list query and renders them in a shared `CommentThread` component mounted from the existing `LimitEditor` popover and `SetLimitDialog`.

**Tech Stack:** Go 1.27 (stdlib `net/http`, sqlc, modernc sqlite + pgx), React 19 + TanStack Query + Zustand + vitest, react-i18next over the shared `locales/*.json`.

**Spec:** `docs/superpowers/specs/2026-09-12-budget-cell-comments-design.md` (read it before Task 1; it carries the permission matrix, the lifecycle table and the Revisions log)

## Global Constraints

- **Go is not on `PATH` in this environment.** Every Go command needs `export PATH=/usr/local/go/bin:$PATH` first. Use `GOTOOLCHAIN=go1.27.1` if the toolchain complains.
- Run every command from the repo root (the Go module root); never `cd` out of the worktree.
- Branch: `feature/budget-cell-comments`. Commit after every task. Never force-push.
- `.sql` files (migrations and sqlc queries) are **ASCII-only, including comments** — a multibyte character silently truncates the generated SQL constant.
- In sqlc query files a `;` must end the last SQL line, never sit on its own line — a lone `;` truncates the generated constant.
- Both engines get every migration file, same version, same column order. A no-op for one engine is still a file (`SELECT 1;`).
- SQLite datetime columns hold bare `Y-m-d H:i:s` UTC text. Period comparisons wrap both sides in `datetime()`; period writes bind `period.Format(datetime.Layout)`, never a raw `time.Time`.
- Wire datetimes are `"2006-01-02 15:04:05"`; wire periods are `"2006-01-02"`. Both are frozen.
- Every new `errs` code goes in `codes.go` **and** `AllCodes` **and** all 11 `locales/*.json` (`de en es fr it nl pl pt ru uk zh`) — `internal/test/i18ntest` asserts a two-way match.
- Every `t('some.key')` literal added to `web/src` must exist in `locales/en.json` or `TestFrontendKeysExist` fails.
- Every new `METRICS.*` key must be referenced from non-test source or `web/src/lib/metrics-coverage.test.ts` fails.
- Coverage gate: `make go-test` enforces `GO_COVER_MIN=80` across packages.
- Never hand-edit a golden file. Regenerate, then read the diff.

## Review Focus

Five input classes the spec implies but no task's happy path exercises. Each has a test pinned to the task that owns the code.

1. **Multi-byte text at the length boundary** — a 500-emoji comment is 500 runes and 2000 bytes; it must be accepted, and 501 runes rejected. (Task 1)
2. **A comment id belonging to a different budget** — update/delete must answer `budget.comment_not_found`, never "forbidden", so existence is not disclosed. (Task 4)
3. **Malformed window parameters** — `months=0`, `months=25`, `months=abc`, `from=not-a-date`, `from` mid-month: reject or snap, never return an unbounded window. (Task 3)
4. **A double-tapped create** — the same client id posted twice must leave exactly one row and return the same comment both times, not a duplicate and not a 500. (Task 4)
5. **An author who is no longer a participant** — a revoked member's comments on surviving elements keep rendering their name and avatar rather than blanking or erroring. (Task 3)

---
## File Structure

**Backend — new files**
- `internal/infra/storage/migrations/sqlite/20260922000000.sql` — the table, sqlite dialect, carries the prose
- `internal/infra/storage/migrations/pgsql/20260922000000.sql` — same table, pgsql dialect
- `internal/budget/comments.go` — the four use cases, nothing else
- `internal/budget/comments_test.go` — use-case tests
- `internal/budget/api/comments.go` — the four HTTP handlers
- `internal/test/apiparity/catalogue_budget_comments.go` — one REST scenario
- `web/src/features/budgets/CommentThread.tsx` — the shared thread UI
- `web/src/features/budgets/CommentThread.test.tsx`
- `web/src/features/budgets/CommentsDialog.tsx` — compact/read-only entry point
- `web/src/features/budgets/comments.test.tsx` — grid + monthly integration tests

**Backend — modified**
- `internal/model/budget.go` — `BudgetElementComment` entity + `BudgetCommentRow` read row
- `internal/model/budget_dto.go` — 4 request DTOs, `CommentResult`, `GetCommentListResult`
- `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql` — 7 queries each
- `internal/budget/repo/{engine.go,repo.go}` — querier methods, both adapters, hydration
- `internal/budget/repository.go` — `CommentStore` + add to `Repository`
- `internal/budget/usecase.go` — `comments CommentStore` + `ops port.OperationGuard` fields
- `internal/budget/{crud.go,clone.go,merge.go}` — reset / clone / merge lifecycle
- `internal/budget/api/routes.go` — 4 routes
- `internal/budget/mcp/mcp.go` — 4 tools
- `internal/server/server.go` — pass `opGuard` and the comment store
- `internal/shared/errs/codes.go`, `locales/*.json` (11)
- `internal/test/apiparity/{guard_test.go,catalogue_test.go}` — raise the floors
- `internal/test/mcpparity/catalogue.go` — one MCP scenario

**Frontend — modified**
- `web/src/api/budget.ts`, `web/src/api/dto/budget.ts`
- `web/src/features/budgets/queries.ts` — one query hook, three mutation hooks
- `web/src/features/budgets/{PlanSheet.tsx,LimitEditor.tsx,SetLimitDialog.tsx,BudgetPage.tsx,BudgetTable.tsx}`
- `web/src/lib/metrics.ts`
- `docs/regression-test-plan.md`

---

### Task 1: The comment entity and its table

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260922000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260922000000.sql`
- Modify: `internal/model/budget.go` (append after `BudgetElementLimit`, ~line 292)
- Test: `internal/model/budget_comment_test.go`

**Interfaces:**
- Consumes: `model.FirstOfMonth`, `errs.NewValidation`, `errs.FieldError`, `errs.CodeIsBlank`, `errs.CodeTooLong`, `vo.Id`
- Produces: `model.BudgetElementComment{ID, ElementID, Period, UserID, Comment, CreatedAt, UpdatedAt}`, `model.NewBudgetElementComment(id, elementID, userID vo.Id, comment string, period, now time.Time) (*BudgetElementComment, error)`, `(*BudgetElementComment).Edit(comment string, now time.Time) error`, `model.BudgetCommentRow{Comment BudgetElementComment, BudgetID, ExternalID vo.Id, AuthorName, AuthorAvatar string}`

- [ ] **Step 1: Write the failing model test**

Create `internal/model/budget_comment_test.go`:

```go
package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func commentIDs(t *testing.T) (vo.Id, vo.Id, vo.Id) {
	t.Helper()
	id, err := vo.ParseId("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	elementID, err := vo.ParseId("22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	userID, err := vo.ParseId("33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatal(err)
	}
	return id, elementID, userID
}

func TestNewBudgetElementComment_TrimsAndSnapsPeriod(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	now := time.Date(2026, 5, 17, 10, 30, 0, 0, time.UTC)
	period := time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)

	c, err := model.NewBudgetElementComment(id, elementID, userID, "  Trip to Lisbon  ", period, now)
	if err != nil {
		t.Fatalf("NewBudgetElementComment: %v", err)
	}
	if c.Comment != "Trip to Lisbon" {
		t.Fatalf("Comment=%q want trimmed", c.Comment)
	}
	if !c.Period.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Period=%v want first of month", c.Period)
	}
	if !c.CreatedAt.Equal(now) || !c.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps=%v/%v want %v", c.CreatedAt, c.UpdatedAt, now)
	}
}

func TestNewBudgetElementComment_Blank(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	now := time.Now().UTC()

	_, err := model.NewBudgetElementComment(id, elementID, userID, "   \t\n ", now, now)
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err=%v want ValidationError", err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Key != "comment" || ve.Fields[0].Code != errs.CodeIsBlank {
		t.Fatalf("fields=%+v want one comment/is_blank", ve.Fields)
	}
}

// Review Focus 1: the limit is 500 RUNES, not bytes. A 500-emoji comment is
// 2000 bytes and must be accepted.
func TestNewBudgetElementComment_LengthBoundaryIsRunes(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	now := time.Now().UTC()

	ok := strings.Repeat("\U0001F600", 500)
	if _, err := model.NewBudgetElementComment(id, elementID, userID, ok, now, now); err != nil {
		t.Fatalf("500 runes rejected: %v", err)
	}

	tooLong := strings.Repeat("\U0001F600", 501)
	_, err := model.NewBudgetElementComment(id, elementID, userID, tooLong, now, now)
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("err=%v want ValidationError", err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Code != errs.CodeTooLong {
		t.Fatalf("fields=%+v want comment/too_long", ve.Fields)
	}
}

func TestBudgetElementComment_Edit(t *testing.T) {
	id, elementID, userID := commentIDs(t)
	created := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	edited := created.Add(time.Hour)

	c, err := model.NewBudgetElementComment(id, elementID, userID, "first", created, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Edit("  first  ", edited); err != nil {
		t.Fatal(err)
	}
	if !c.UpdatedAt.Equal(created) {
		t.Fatalf("UpdatedAt moved on a no-op edit: %v", c.UpdatedAt)
	}
	if err := c.Edit("second", edited); err != nil {
		t.Fatal(err)
	}
	if c.Comment != "second" || !c.UpdatedAt.Equal(edited) {
		t.Fatalf("Comment=%q UpdatedAt=%v want second/%v", c.Comment, c.UpdatedAt, edited)
	}
	if err := c.Edit("", edited); err == nil {
		t.Fatal("blank edit accepted")
	}
	if c.Comment != "second" {
		t.Fatalf("Comment=%q mutated by a rejected edit", c.Comment)
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./internal/model/ -run BudgetElementComment -v
```
Expected: FAIL — `undefined: model.NewBudgetElementComment`.

- [ ] **Step 3: Add the entity**

Append to `internal/model/budget.go` (after `func (l *BudgetElementLimit) UpdateAmount`). Add `"strings"` and `"unicode/utf8"` to the file's import block, and `"github.com/econumo/econumo/internal/shared/errs"` if it is not already imported.

```go
// commentMaxRunes caps a cell comment. Runes, not bytes: the limit is about how
// much a person can type, and an emoji is one character to them.
const commentMaxRunes = 500

// BudgetElementComment is one message in a budget cell's thread: an element in
// one month, written by one participant.
type BudgetElementComment struct {
	ID        vo.Id
	ElementID vo.Id
	Period    time.Time
	UserID    vo.Id
	Comment   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewBudgetElementComment(id, elementID, userID vo.Id, comment string, period, now time.Time) (*BudgetElementComment, error) {
	text, err := validateCommentText(comment)
	if err != nil {
		return nil, err
	}
	return &BudgetElementComment{
		ID: id, ElementID: elementID, Period: FirstOfMonth(period), UserID: userID,
		Comment: text, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (c *BudgetElementComment) Edit(comment string, now time.Time) error {
	text, err := validateCommentText(comment)
	if err != nil {
		return err
	}
	if text != c.Comment {
		c.Comment = text
		c.UpdatedAt = now
	}
	return nil
}

func validateCommentText(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", errs.NewValidation("Validation failed", errs.FieldError{
			Key: "comment", Message: "This value should not be blank.", Code: errs.CodeIsBlank,
		})
	}
	if utf8.RuneCountInString(text) > commentMaxRunes {
		return "", errs.NewValidation("Validation failed", errs.FieldError{
			Key: "comment", Message: "This value is too long.", Code: errs.CodeTooLong,
		})
	}
	return text, nil
}

// BudgetCommentRow is a comment joined with the identity a reader needs: the
// budget it belongs to (for the access check), the element's EXTERNAL id (what
// the wire calls elementId) and the author's display fields.
type BudgetCommentRow struct {
	Comment      BudgetElementComment
	BudgetID     vo.Id
	ExternalID   vo.Id
	AuthorName   string
	AuthorAvatar string
}
```

- [ ] **Step 4: Run the test and watch it pass**

```bash
go test ./internal/model/ -run BudgetElementComment -v
```
Expected: PASS (4 tests).

- [ ] **Step 5: Write both migrations**

`internal/infra/storage/migrations/sqlite/20260922000000.sql`:

```sql
-- Budget cell comments: a flat thread per (budget element, month).
-- element_id cascades, so removing an element or a whole budget takes its
-- threads with it. period is the first of the month, stored as bare
-- 'Y-m-d H:i:s' text like every other datetime column.
CREATE TABLE budgets_elements_comments
(
    id           TEXT     NOT NULL
    , element_id TEXT     NOT NULL
    , period     DATETIME NOT NULL
    , user_id    TEXT     NOT NULL
    , comment    TEXT     NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX budgets_elements_comments_element_period_idx ON budgets_elements_comments (element_id, period);
CREATE INDEX budgets_elements_comments_period_idx ON budgets_elements_comments (period);
```

`internal/infra/storage/migrations/pgsql/20260922000000.sql`:

```sql
-- See the sqlite sibling for the semantics.
CREATE TABLE budgets_elements_comments
(
    id           UUID     NOT NULL
    , element_id UUID     NOT NULL
    , period     TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , user_id    UUID     NOT NULL
    , comment    TEXT     NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX budgets_elements_comments_element_period_idx ON budgets_elements_comments (element_id, period);
CREATE INDEX budgets_elements_comments_period_idx ON budgets_elements_comments (period);
```

Column order is identical in both files on purpose: it keeps sqlc's generated models field-compatible so the pgsql adapter can convert whole structs.

- [ ] **Step 6: Verify the migrations apply**

```bash
go test ./internal/infra/storage/... ./internal/test/apiparity/ 2>&1 | tail -20
```
Expected: PASS. Every test database is built by running the migrations, so a broken DDL fails here loudly.

- [ ] **Step 7: Commit**

```bash
git add internal/model/budget.go internal/model/budget_comment_test.go internal/infra/storage/migrations
git commit -m "feat(budget): add the budget cell comment entity and its table

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Persistence — queries, engine adapters, `CommentStore`

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/budgets.sql` (append)
- Modify: `internal/infra/storage/sqlc/query/pgsql/budgets.sql` (append)
- Modify: `internal/budget/repo/engine.go` (querier interface + both adapters)
- Modify: `internal/budget/repo/repo.go` (row aliases, `Repo` methods, hydration)
- Modify: `internal/budget/repository.go` (`CommentStore`, add to `Repository`)
- Modify: `internal/budget/usecase.go` (`comments` + `ops` fields)
- Modify: `internal/server/server.go` (pass `opGuard`)
- Test: `internal/budget/repo/comments_test.go`

**Interfaces:**
- Consumes: `model.BudgetElementComment`, `model.BudgetCommentRow` (Task 1)
- Produces: `budget.CommentStore` with `NextIdentity() vo.Id`, `ListCommentsForWindow(ctx, budgetID vo.Id, from, to time.Time) ([]model.BudgetCommentRow, error)`, `GetCommentRow(ctx, id vo.Id) (*model.BudgetCommentRow, error)`, `InsertComment(ctx, c *model.BudgetElementComment) error`, `UpdateCommentText(ctx, c *model.BudgetElementComment) error`, `DeleteComment(ctx, id vo.Id) error`, `ListCommentsFrom(ctx, budgetID vo.Id, from time.Time) ([]*model.BudgetElementComment, error)`, `RepointComments(ctx, srcElementID, dstElementID vo.Id) error`, `DeleteCommentsByBudget(ctx, budgetID vo.Id) error`

Insert and update are deliberately separate statements rather than one upsert: an upsert would let a create whose client id collides with an existing comment silently overwrite someone else's text, bypassing the author check.

- [ ] **Step 1: Write the failing repository test**

Create `internal/budget/repo/comments_test.go`. Follow the existing repo tests in this package for fixture construction — open the file `internal/budget/repo/repo_test.go` first and reuse its helpers for building a budget with one element.

```go
package repo_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// seedCommentFixture returns a budget id, an element id and the element's
// external id, with the owner user already present. Build it with the same
// helpers the sibling repo tests use.
func TestComments_InsertListUpdateDelete(t *testing.T) {
	ctx, r, fx := newCommentFixture(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC)

	c, err := model.NewBudgetElementComment(r.NextIdentity(), fx.elementID, fx.userID, "Trip to Lisbon", period, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.InsertComment(ctx, c); err != nil {
		t.Fatalf("InsertComment: %v", err)
	}

	rows, err := r.ListCommentsForWindow(ctx, fx.budgetID, period, period.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("ListCommentsForWindow: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	got := rows[0]
	if got.Comment.Comment != "Trip to Lisbon" {
		t.Fatalf("text=%q", got.Comment.Comment)
	}
	if !got.Comment.Period.Equal(period) {
		t.Fatalf("period=%v want %v", got.Comment.Period, period)
	}
	if !got.ExternalID.Equal(fx.externalID) {
		t.Fatalf("externalID=%v want %v", got.ExternalID, fx.externalID)
	}
	if !got.BudgetID.Equal(fx.budgetID) {
		t.Fatalf("budgetID=%v want %v", got.BudgetID, fx.budgetID)
	}
	if got.AuthorName == "" {
		t.Fatal("AuthorName empty: the users join is missing")
	}

	// A window that ends before the comment's month returns nothing.
	empty, err := r.ListCommentsForWindow(ctx, fx.budgetID, period.AddDate(0, -2, 0), period.AddDate(0, -1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("rows=%d want 0 outside the window", len(empty))
	}

	if err := c.Edit("Trip to Porto", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateCommentText(ctx, c); err != nil {
		t.Fatalf("UpdateCommentText: %v", err)
	}
	one, err := r.GetCommentRow(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCommentRow: %v", err)
	}
	if one.Comment.Comment != "Trip to Porto" {
		t.Fatalf("text=%q want updated", one.Comment.Comment)
	}

	if err := r.DeleteComment(ctx, c.ID); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := r.GetCommentRow(ctx, c.ID); err == nil {
		t.Fatal("GetCommentRow found a deleted comment")
	}
}

func TestComments_CascadeWithElement(t *testing.T) {
	ctx, r, fx := newCommentFixture(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := period

	c, err := model.NewBudgetElementComment(r.NextIdentity(), fx.elementID, fx.userID, "gone soon", period, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.InsertComment(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteElement(ctx, fx.elementID); err != nil {
		t.Fatalf("DeleteElement: %v", err)
	}
	if _, err := r.GetCommentRow(ctx, c.ID); err == nil {
		t.Fatal("comment survived its element")
	}
}

func TestComments_RepointAndDeleteByBudget(t *testing.T) {
	ctx, r, fx := newCommentFixture(t)
	period := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	c, err := model.NewBudgetElementComment(r.NextIdentity(), fx.elementID, fx.userID, "moves", period, period)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.InsertComment(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := r.RepointComments(ctx, fx.elementID, fx.otherElementID); err != nil {
		t.Fatalf("RepointComments: %v", err)
	}
	row, err := r.GetCommentRow(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.Comment.ElementID.Equal(fx.otherElementID) {
		t.Fatalf("elementID=%v want repointed", row.Comment.ElementID)
	}

	if err := r.DeleteCommentsByBudget(ctx, fx.budgetID); err != nil {
		t.Fatalf("DeleteCommentsByBudget: %v", err)
	}
	rows, err := r.ListCommentsForWindow(ctx, fx.budgetID, period, period.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows=%d want 0 after the budget-wide delete", len(rows))
	}
}

var _ = vo.NewId
```

Write `newCommentFixture(t)` in the same file, returning `(context.Context, *repo.Repo, fixture)` where `fixture` carries `budgetID, elementID, otherElementID, externalID, userID vo.Id`. Build it with `dbtest.New(t)` and the same seeding the neighbouring repo tests use; if they seed through `internal/test/fixture`, use that instead of raw SQL.

- [ ] **Step 2: Run it and watch it fail**

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./internal/budget/repo/ -run Comments -v
```
Expected: FAIL — `r.InsertComment undefined`.

- [ ] **Step 3: Add the sqlite queries**

Append to `internal/infra/storage/sqlc/query/sqlite/budgets.sql`:

```sql
-- name: ListBudgetCommentsForWindow :many
-- Every comment on every element of a budget inside a half-open month window.
-- period is datetime TEXT, so normalize both sides with datetime() and bind the
-- bounds as 'Y-m-d H:i:s' strings, exactly like the limit queries.
SELECT c.id, c.element_id, c.period, c.user_id, c.comment, c.created_at, c.updated_at,
       e.budget_id, e.external_id, u.name AS author_name, u.avatar AS author_avatar
FROM budgets_elements_comments c
JOIN budgets_elements e ON e.id = c.element_id
JOIN users u ON u.id = c.user_id
WHERE e.budget_id = ? AND datetime(c.period) >= datetime(?) AND datetime(c.period) < datetime(?)
ORDER BY c.period, e.external_id, c.created_at, c.id;

-- name: GetBudgetComment :one
SELECT c.id, c.element_id, c.period, c.user_id, c.comment, c.created_at, c.updated_at,
       e.budget_id, e.external_id, u.name AS author_name, u.avatar AS author_avatar
FROM budgets_elements_comments c
JOIN budgets_elements e ON e.id = c.element_id
JOIN users u ON u.id = c.user_id
WHERE c.id = ?;

-- name: ListBudgetCommentsFrom :many
-- Clone reads every comment at or after the copy's start month.
SELECT c.id, c.element_id, c.period, c.user_id, c.comment, c.created_at, c.updated_at
FROM budgets_elements_comments c
JOIN budgets_elements e ON e.id = c.element_id
WHERE e.budget_id = ? AND datetime(c.period) >= datetime(?)
ORDER BY c.period, c.created_at, c.id;

-- name: InsertBudgetComment :exec
INSERT INTO budgets_elements_comments (id, element_id, period, user_id, comment, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateBudgetCommentText :exec
UPDATE budgets_elements_comments SET comment = ?, updated_at = ? WHERE id = ?;

-- name: DeleteBudgetComment :exec
DELETE FROM budgets_elements_comments WHERE id = ?;

-- name: RepointBudgetComments :exec
UPDATE budgets_elements_comments SET element_id = ? WHERE element_id = ?;

-- name: DeleteBudgetCommentsByBudget :exec
DELETE FROM budgets_elements_comments
WHERE element_id IN (SELECT e.id FROM budgets_elements e WHERE e.budget_id = ?);
```

ASCII only; no `;` on its own line.

- [ ] **Step 4: Add the pgsql queries**

Append the same eight to `internal/infra/storage/sqlc/query/pgsql/budgets.sql` with `$n` placeholders, no per-query comments (the sqlite sibling holds the prose) and native period comparison:

```sql
-- name: ListBudgetCommentsForWindow :many
SELECT c.id, c.element_id, c.period, c.user_id, c.comment, c.created_at, c.updated_at,
       e.budget_id, e.external_id, u.name AS author_name, u.avatar AS author_avatar
FROM budgets_elements_comments c
JOIN budgets_elements e ON e.id = c.element_id
JOIN users u ON u.id = c.user_id
WHERE e.budget_id = $1 AND c.period >= $2 AND c.period < $3
ORDER BY c.period, e.external_id, c.created_at, c.id;

-- name: GetBudgetComment :one
SELECT c.id, c.element_id, c.period, c.user_id, c.comment, c.created_at, c.updated_at,
       e.budget_id, e.external_id, u.name AS author_name, u.avatar AS author_avatar
FROM budgets_elements_comments c
JOIN budgets_elements e ON e.id = c.element_id
JOIN users u ON u.id = c.user_id
WHERE c.id = $1;

-- name: ListBudgetCommentsFrom :many
SELECT c.id, c.element_id, c.period, c.user_id, c.comment, c.created_at, c.updated_at
FROM budgets_elements_comments c
JOIN budgets_elements e ON e.id = c.element_id
WHERE e.budget_id = $1 AND c.period >= $2
ORDER BY c.period, c.created_at, c.id;

-- name: InsertBudgetComment :exec
INSERT INTO budgets_elements_comments (id, element_id, period, user_id, comment, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: UpdateBudgetCommentText :exec
UPDATE budgets_elements_comments SET comment = $1, updated_at = $2 WHERE id = $3;

-- name: DeleteBudgetComment :exec
DELETE FROM budgets_elements_comments WHERE id = $1;

-- name: RepointBudgetComments :exec
UPDATE budgets_elements_comments SET element_id = $1 WHERE element_id = $2;

-- name: DeleteBudgetCommentsByBudget :exec
DELETE FROM budgets_elements_comments
WHERE element_id IN (SELECT e.id FROM budgets_elements e WHERE e.budget_id = $1);
```

- [ ] **Step 5: Generate the sqlc code and read what it produced**

```bash
export PATH=/usr/local/go/bin:$PATH
go generate ./internal/infra/storage/sqlc/...
git diff --stat internal/infra/storage/sqlc/gen
```
Then open `internal/infra/storage/sqlc/gen/sqlite/budgets.sql.go` and `gen/pgsql/budgets.sql.go` and note the exact generated names — the row types for the joined queries are `ListBudgetCommentsForWindowRow` / `GetBudgetCommentRow` in each package, and their fields may be ordered differently across engines. **Read them before writing the adapters**; do not assume.

- [ ] **Step 6: Add the querier methods and both adapters**

In `internal/budget/repo/repo.go`, extend the type-alias block:

```go
	commentRow    = sqlitegen.BudgetsElementsComment
	commentJoined = sqlitegen.ListBudgetCommentsForWindowRow
	inCommentP    = sqlitegen.InsertBudgetCommentParams
```

In `internal/budget/repo/engine.go`, add to the `querier` interface:

```go
	ListBudgetCommentsForWindow(ctx context.Context, db backend.DBTX, budgetID string, from, to time.Time) ([]commentJoined, error)
	GetBudgetComment(ctx context.Context, db backend.DBTX, id string) (commentJoined, error)
	ListBudgetCommentsFrom(ctx context.Context, db backend.DBTX, budgetID string, from time.Time) ([]commentRow, error)
	InsertBudgetComment(ctx context.Context, db backend.DBTX, p inCommentP) error
	UpdateBudgetCommentText(ctx context.Context, db backend.DBTX, id, text string, updatedAt time.Time) error
	DeleteBudgetComment(ctx context.Context, db backend.DBTX, id string) error
	RepointBudgetComments(ctx context.Context, db backend.DBTX, dstElementID, srcElementID string) error
	DeleteBudgetCommentsByBudget(ctx context.Context, db backend.DBTX, budgetID string) error
```

`GetBudgetComment` returns the same joined shape as the window query; if sqlc emits two distinct row structs, convert the single-row one into `commentJoined` field-by-field inside each adapter so the repo sees one type.

sqlite adapter — the window bounds and the inserted period go through `limitPeriodArg` (already in this file), the same belt-and-braces the limit queries use:

```go
func (sqliteQuerier) ListBudgetCommentsForWindow(ctx context.Context, db backend.DBTX, budgetID string, from, to time.Time) ([]commentJoined, error) {
	return sqlitegen.New(db).ListBudgetCommentsForWindow(ctx, sqlitegen.ListBudgetCommentsForWindowParams{
		BudgetID: budgetID, Datetime: limitPeriodArg(from), Datetime_2: limitPeriodArg(to),
	})
}

func (sqliteQuerier) InsertBudgetComment(ctx context.Context, db backend.DBTX, p inCommentP) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO budgets_elements_comments (id, element_id, period, user_id, comment, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ElementID, p.Period.Format(datetime.Layout), p.UserID, p.Comment, p.CreatedAt, p.UpdatedAt)
	return err
}
```

(Check the generated param field names for the two `datetime(?)` binds — sqlc names them `Datetime` and `Datetime_2`; use whatever it actually emitted.)

pgsql adapter — plain passthrough with whole-struct or field-by-field conversion depending on what step 5 showed:

```go
func (pgsqlQuerier) ListBudgetCommentsForWindow(ctx context.Context, db backend.DBTX, budgetID string, from, to time.Time) ([]commentJoined, error) {
	rows, err := pgsqlgen.New(db).ListBudgetCommentsForWindow(ctx, pgsqlgen.ListBudgetCommentsForWindowParams{
		BudgetID: budgetID, Period: from, Period_2: to,
	})
	if err != nil {
		return nil, err
	}
	out := make([]commentJoined, len(rows))
	for i, v := range rows {
		out[i] = commentJoined{
			ID: v.ID, ElementID: v.ElementID, Period: v.Period, UserID: v.UserID, Comment: v.Comment,
			CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, BudgetID: v.BudgetID, ExternalID: v.ExternalID,
			AuthorName: v.AuthorName, AuthorAvatar: v.AuthorAvatar,
		}
	}
	return out, nil
}
```

- [ ] **Step 7: Add the `Repo` methods and hydration**

In `internal/budget/repo/repo.go`:

```go
func (r *Repo) ListCommentsForWindow(ctx context.Context, budgetID vo.Id, from, to time.Time) ([]model.BudgetCommentRow, error) {
	rows, err := r.q.ListBudgetCommentsForWindow(ctx, r.db(ctx), budgetID.String(), from, to)
	if err != nil {
		return nil, err
	}
	out := make([]model.BudgetCommentRow, 0, len(rows))
	for _, row := range rows {
		hydrated, herr := hydrateCommentRow(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, *hydrated)
	}
	return out, nil
}

func (r *Repo) GetCommentRow(ctx context.Context, id vo.Id) (*model.BudgetCommentRow, error) {
	row, err := r.q.GetBudgetComment(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, mapNotFound(err, "BudgetElementComment not found")
	}
	return hydrateCommentRow(row)
}

func (r *Repo) InsertComment(ctx context.Context, c *model.BudgetElementComment) error {
	return r.q.InsertBudgetComment(ctx, r.db(ctx), inCommentP{
		ID: c.ID.String(), ElementID: c.ElementID.String(), Period: c.Period, UserID: c.UserID.String(),
		Comment: c.Comment, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	})
}

func (r *Repo) UpdateCommentText(ctx context.Context, c *model.BudgetElementComment) error {
	return r.q.UpdateBudgetCommentText(ctx, r.db(ctx), c.ID.String(), c.Comment, c.UpdatedAt)
}

func (r *Repo) DeleteComment(ctx context.Context, id vo.Id) error {
	return r.q.DeleteBudgetComment(ctx, r.db(ctx), id.String())
}

func (r *Repo) ListCommentsFrom(ctx context.Context, budgetID vo.Id, from time.Time) ([]*model.BudgetElementComment, error) {
	rows, err := r.q.ListBudgetCommentsFrom(ctx, r.db(ctx), budgetID.String(), from)
	if err != nil {
		return nil, err
	}
	out := make([]*model.BudgetElementComment, 0, len(rows))
	for _, row := range rows {
		c, herr := hydrateComment(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *Repo) RepointComments(ctx context.Context, srcElementID, dstElementID vo.Id) error {
	return r.q.RepointBudgetComments(ctx, r.db(ctx), dstElementID.String(), srcElementID.String())
}

func (r *Repo) DeleteCommentsByBudget(ctx context.Context, budgetID vo.Id) error {
	return r.q.DeleteBudgetCommentsByBudget(ctx, r.db(ctx), budgetID.String())
}
```

Write `hydrateComment(commentRow)` and `hydrateCommentRow(commentJoined)` next to `hydrateLimit`, parsing every id with `vo.ParseId` and returning the first parse error.

- [ ] **Step 8: Declare `CommentStore` and wire it**

In `internal/budget/repository.go`, after `LimitStore`:

```go
// CommentStore is a budget cell's comment-thread persistence surface. Consumed
// by the comment use cases (comments.go), ResetBudget's clear-all (crud.go),
// CloneBudget (clone.go) and MergeService (merge.go). Insert and Update are
// separate statements on purpose: an upsert would let a create with a colliding
// client id overwrite another author's text.
type CommentStore interface {
	NextIdentity() vo.Id

	ListCommentsForWindow(ctx context.Context, budgetID vo.Id, from, to time.Time) ([]model.BudgetCommentRow, error)
	GetCommentRow(ctx context.Context, id vo.Id) (*model.BudgetCommentRow, error)
	InsertComment(ctx context.Context, c *model.BudgetElementComment) error
	UpdateCommentText(ctx context.Context, c *model.BudgetElementComment) error
	DeleteComment(ctx context.Context, id vo.Id) error
	// ListCommentsFrom returns the comments at or after a month (clone).
	ListCommentsFrom(ctx context.Context, budgetID vo.Id, from time.Time) ([]*model.BudgetElementComment, error)
	// RepointComments moves every comment from one element to another (merge).
	RepointComments(ctx context.Context, srcElementID, dstElementID vo.Id) error
	// DeleteCommentsByBudget removes every comment of every element of a budget (reset).
	DeleteCommentsByBudget(ctx context.Context, budgetID vo.Id) error
}
```

Add `CommentStore` to the `Repository` composite.

In `internal/budget/usecase.go`: add `comments CommentStore` and `ops port.OperationGuard` to `Service`, add `ops port.OperationGuard` as a new parameter of `NewService` (place it immediately before `tx port.TxRunner`), and set `comments: repo, ops: ops` in the constructor body.

In `internal/server/server.go`, pass `opGuard` to `appbudget.NewService(...)` in the same position. `opGuard` is already built earlier in the function.

- [ ] **Step 9: Run the repository tests on sqlite, then PostgreSQL**

```bash
export PATH=/usr/local/go/bin:$PATH
go build ./... && go test ./internal/budget/... -run Comments -v
```
Expected: PASS.

```bash
make test-repo-pgsql 2>&1 | tail -20
```
Expected: PASS (this is what proves the pgsql adapter and the `$n` queries actually work).

- [ ] **Step 10: Commit**

```bash
git add internal/infra/storage/sqlc internal/budget internal/server/server.go
git commit -m "feat(budget): persist budget cell comments

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: DTOs, error codes and the read use case

**Files:**
- Modify: `internal/model/budget_dto.go` (append)
- Modify: `internal/shared/errs/codes.go`
- Modify: `locales/de.json`, `en.json`, `es.json`, `fr.json`, `it.json`, `nl.json`, `pl.json`, `pt.json`, `ru.json`, `uk.json`, `zh.json`
- Create: `internal/budget/comments.go`
- Test: `internal/budget/comments_test.go`

**Interfaces:**
- Consumes: `budget.CommentStore` (Task 2), `model.BudgetCommentRow` (Task 1), `s.loadAggregate`, `s.canRead` (existing, `internal/budget/access.go:28`)
- Produces: `model.GetCommentListRequest{BudgetId, From, Months string}`, `model.CommentResult{Id, ElementId, Period, Comment string, Author UserResult, CreatedAt, UpdatedAt string}`, `model.GetCommentListResult{Items []CommentResult, Truncated bool}`, `(*budget.Service).GetCommentList(ctx, userID vo.Id, req model.GetCommentListRequest) (*model.GetCommentListResult, error)`, `errs.CodeBudgetCommentNotFound`, `errs.CodeBudgetCommentForbidden`

- [ ] **Step 1: Write the failing use-case test**

Create `internal/budget/comments_test.go`. Use the same service-construction harness the existing budget use-case tests use (read `internal/budget/merge_test.go` and `internal/budget/access_test.go` first and follow whichever builds a real `Service` over a sqlite `dbtest` database).

```go
package budget_test

import (
	"testing"
	"time"
)

func TestGetCommentList_WindowAndOrder(t *testing.T) {
	h := newCommentHarness(t) // budget started 2026-04-01, owner + guest, element CatFood

	h.postComment(t, h.owner, "cat-food", "2026-05-01", "May note")
	h.postComment(t, h.owner, "cat-food", "2026-06-01", "June note")

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 1 || res.Items[0].Comment != "May note" {
		t.Fatalf("items=%+v want only the May note", res.Items)
	}
	if res.Items[0].Period != "2026-05-01" {
		t.Fatalf("period=%q want Y-m-d", res.Items[0].Period)
	}
	if res.Truncated {
		t.Fatal("Truncated set on a two-row window")
	}

	res = h.list(t, h.owner, "2026-05-01", "2")
	if len(res.Items) != 2 {
		t.Fatalf("items=%d want 2 over a two-month window", len(res.Items))
	}
	if res.Items[0].Comment != "May note" || res.Items[1].Comment != "June note" {
		t.Fatalf("order=%+v want chronological", res.Items)
	}
}

// Review Focus 3: malformed window parameters must never widen the window.
func TestGetCommentList_BadWindowParams(t *testing.T) {
	h := newCommentHarness(t)

	for _, months := range []string{"0", "25", "abc", "-1"} {
		if _, err := h.rawList(h.owner, "2026-05-01", months); err == nil {
			t.Fatalf("months=%q accepted", months)
		}
	}
	if _, err := h.rawList(h.owner, "not-a-date", "1"); err == nil {
		t.Fatal("from=not-a-date accepted")
	}
	// A mid-month "from" snaps to the first of that month rather than erroring.
	h.postComment(t, h.owner, "cat-food", "2026-05-01", "May note")
	res := h.list(t, h.owner, "2026-05-23", "1")
	if len(res.Items) != 1 {
		t.Fatalf("items=%d want the May note after snapping", len(res.Items))
	}
}

func TestGetCommentList_GuestReadsPendingDenied(t *testing.T) {
	h := newCommentHarness(t)
	h.postComment(t, h.owner, "cat-food", "2026-05-01", "shared")

	res := h.list(t, h.guest, "2026-05-01", "1")
	if len(res.Items) != 1 {
		t.Fatalf("guest saw %d items, want 1", len(res.Items))
	}
	if _, err := h.rawList(h.pending, "2026-05-01", "1"); err == nil {
		t.Fatal("a pending invitee read the thread")
	}
	if _, err := h.rawList(h.stranger, "2026-05-01", "1"); err == nil {
		t.Fatal("a stranger read the thread")
	}
}

// Review Focus 5: a revoked member's comments keep rendering their author.
func TestGetCommentList_RevokedAuthorStillRenders(t *testing.T) {
	h := newCommentHarness(t)
	h.postComment(t, h.member, "cat-food", "2026-05-01", "written before the revoke")
	h.revoke(t, h.member)

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 1 {
		t.Fatalf("items=%d want the revoked member's comment", len(res.Items))
	}
	if res.Items[0].Author.Name == "" || res.Items[0].Author.Id == "" {
		t.Fatalf("author=%+v want the stored user's identity", res.Items[0].Author)
	}
}

func TestGetCommentList_ArchivedBudgetStillReadable(t *testing.T) {
	h := newCommentHarness(t)
	h.postComment(t, h.owner, "cat-food", "2026-05-01", "before archiving")
	h.archive(t)

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 1 {
		t.Fatalf("items=%d want the thread on an archived budget", len(res.Items))
	}
}

var _ = time.Now
```

The harness needs `postComment`, `revoke` and `archive` before Task 4 exists; write it to call the service's own `CreateComment`/`RevokeAccess`/`ArchiveBudget`, and expect the comment-posting tests to stay red until Task 4 lands. If that blocks you, seed comments directly through the `CommentStore` in the harness instead and switch to `CreateComment` in Task 4.

- [ ] **Step 2: Run it and watch it fail**

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./internal/budget/ -run Comment -v
```
Expected: FAIL — `GetCommentList undefined`.

- [ ] **Step 3: Add the DTOs**

Append to `internal/model/budget_dto.go`:

```go
// GetCommentListRequest selects a budget and a month window for the comment
// read. From and Months arrive as raw query strings; the service parses them.
type GetCommentListRequest struct {
	BudgetId string `json:"budgetId"`
	From     string `json:"from"`
	Months   string `json:"months"`
}

func (r GetCommentListRequest) Validate() error {
	return ValidateBlank(map[string]string{"budgetId": r.BudgetId})
}

// CommentResult is one comment on the wire. ElementId is the element's EXTERNAL
// id (the identifier set-limit takes); Period is Y-m-d; the timestamps use the
// frozen datetime layout.
type CommentResult struct {
	Id        string     `json:"id"`
	ElementId string     `json:"elementId"`
	Period    string     `json:"period"`
	Comment   string     `json:"comment"`
	Author    UserResult `json:"author"`
	CreatedAt string     `json:"createdAt"`
	UpdatedAt string     `json:"updatedAt"`
}

// GetCommentListResult is {items: [...], truncated: bool}. Truncated is true
// only when the server-side cap cut the tail, so the SPA can say so instead of
// silently showing a partial thread.
type GetCommentListResult struct {
	Items     []CommentResult `json:"items"`
	Truncated bool            `json:"truncated"`
}

// CreateCommentRequest posts a comment. Id is the comment's id AND the
// idempotency key: a retried post with the same id returns the stored comment.
type CreateCommentRequest struct {
	Id        string `json:"id"`
	BudgetId  string `json:"budgetId"`
	ElementId string `json:"elementId"`
	Period    string `json:"period"`
	Comment   string `json:"comment"`
}

func (r CreateCommentRequest) Validate() error {
	return ValidateBlank(map[string]string{
		"id": r.Id, "budgetId": r.BudgetId, "elementId": r.ElementId,
		"period": r.Period, "comment": r.Comment,
	})
}

type UpdateCommentRequest struct {
	Id      string `json:"id"`
	Comment string `json:"comment"`
}

func (r UpdateCommentRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.Id, "comment": r.Comment})
}

type DeleteCommentRequest struct {
	Id string `json:"id"`
}

func (r DeleteCommentRequest) Validate() error {
	return ValidateBlank(map[string]string{"id": r.Id})
}

// CreateCommentResult and UpdateCommentResult are {item: CommentResult}.
type CreateCommentResult struct {
	Item CommentResult `json:"item"`
}

type UpdateCommentResult struct {
	Item CommentResult `json:"item"`
}

// DeleteCommentResult is empty.
type DeleteCommentResult struct{}
```

- [ ] **Step 4: Register the two error codes**

In `internal/shared/errs/codes.go`, add to the budget block of the `const` and the matching block of `AllCodes`, in the same order:

```go
	CodeBudgetCommentNotFound = "budget.comment_not_found"
	CodeBudgetCommentForbidden = "budget.comment_forbidden"
```

Then add both to every catalogue. English (`locales/en.json`, inside `errors.budget`):

```json
        "comment_not_found": "Comment not found",
        "comment_forbidden": "You can't change this comment"
```

Translations to use verbatim:

| lang | `comment_not_found` | `comment_forbidden` |
|---|---|---|
| de | `Kommentar nicht gefunden` | `Du kannst diesen Kommentar nicht ändern` |
| es | `Comentario no encontrado` | `No puedes modificar este comentario` |
| fr | `Commentaire introuvable` | `Vous ne pouvez pas modifier ce commentaire` |
| it | `Commento non trovato` | `Non puoi modificare questo commento` |
| nl | `Opmerking niet gevonden` | `Je kunt deze opmerking niet wijzigen` |
| pl | `Nie znaleziono komentarza` | `Nie możesz zmienić tego komentarza` |
| pt | `Comentário não encontrado` | `Você não pode alterar este comentário` |
| ru | `Комментарий не найден` | `Вы не можете изменить этот комментарий` |
| uk | `Коментар не знайдено` | `Ви не можете змінити цей коментар` |
| zh | `未找到评论` | `您无法修改此评论` |

- [ ] **Step 5: Write the read use case**

Create `internal/budget/comments.go`:

```go
// Budget cell comments: a flat thread per (element, month), shared with the
// budget. Reads are a windowed list; the budget reads are untouched.
package budget

import (
	"context"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	commentMonthsDefault = 1
	commentMonthsMax     = 24
	// commentListCap bounds an unpaginated response. A budget with more
	// comments than this in one window is pathological; the cap is a guard, not
	// a paging design.
	commentListCap = 2000
)

func (s *Service) GetCommentList(ctx context.Context, userID vo.Id, req model.GetCommentListRequest) (*model.GetCommentListResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	from, err := parseCommentFrom(req.From, localMonth(s.clock.Now(), reqctx.Location(ctx)))
	if err != nil {
		return nil, err
	}
	months, err := parseCommentMonths(req.Months)
	if err != nil {
		return nil, err
	}

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canRead(b, userID) {
		return nil, accessDenied()
	}

	rows, err := s.comments.ListCommentsForWindow(ctx, budgetID, from, from.AddDate(0, months, 0))
	if err != nil {
		return nil, err
	}
	truncated := len(rows) > commentListCap
	if truncated {
		rows = rows[:commentListCap]
	}
	items := make([]model.CommentResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, commentResult(row))
	}
	return &model.GetCommentListResult{Items: items, Truncated: truncated}, nil
}

func commentResult(row model.BudgetCommentRow) model.CommentResult {
	return model.CommentResult{
		Id:        row.Comment.ID.String(),
		ElementId: row.ExternalID.String(),
		Period:    row.Comment.Period.Format(datetime.DateLayout),
		Comment:   row.Comment.Comment,
		Author:    model.UserResult{Id: row.Comment.UserID.String(), Avatar: row.AuthorAvatar, Name: row.AuthorName},
		CreatedAt: row.Comment.CreatedAt.Format(datetime.Layout),
		UpdatedAt: row.Comment.UpdatedAt.Format(datetime.Layout),
	}
}

// parseCommentFrom snaps the window start to the first of its month. An empty
// value means the caller's current month.
func parseCommentFrom(raw string, fallback time.Time) (time.Time, error) {
	if raw == "" {
		return model.FirstOfMonth(fallback), nil
	}
	t, err := time.Parse(datetime.DateLayout, raw)
	if err != nil {
		return time.Time{}, model.ValidateBlank(map[string]string{"from": ""})
	}
	return model.FirstOfMonth(t), nil
}

func parseCommentMonths(raw string) (int, error) {
	if raw == "" {
		return commentMonthsDefault, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > commentMonthsMax {
		return 0, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "months", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
		})
	}
	return n, nil
}
```

`localMonth` already exists in `internal/budget/plan.go`; reuse it rather than writing a second one. If its signature differs, adapt the call rather than duplicating the helper.

- [ ] **Step 6: Run the read tests**

```bash
go test ./internal/budget/ -run "GetCommentList" -v
```
Expected: PASS for the window, guest/pending, revoked-author and archived cases. The tests that post through `CreateComment` stay red until Task 4 if you wrote the harness that way.

- [ ] **Step 7: Run the i18n guards**

```bash
go test ./internal/test/i18ntest/ -v 2>&1 | tail -20
```
Expected: PASS — both new codes present in all 11 catalogues with no orphans.

- [ ] **Step 8: Commit**

```bash
git add internal/model/budget_dto.go internal/shared/errs/codes.go locales internal/budget/comments.go internal/budget/comments_test.go
git commit -m "feat(budget): read budget cell comments over a month window

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Create, update and delete

**Files:**
- Modify: `internal/budget/comments.go`
- Test: `internal/budget/comments_test.go` (extend)

**Interfaces:**
- Consumes: `s.comments` and `s.ops` (Task 2), `s.requireNotArchived` (`internal/budget/lifecycle.go:15`), `s.getElementSelfHeal` (`internal/budget/accounts.go:296`), `b.endMonth()`, `errs.CodeOperationLocked`
- Produces: `(*budget.Service).CreateComment(ctx, userID vo.Id, req model.CreateCommentRequest) (*model.CreateCommentResult, error)`, `.UpdateComment(...) (*model.UpdateCommentResult, error)`, `.DeleteComment(...) (*model.DeleteCommentResult, error)`

**Permission rules (from the spec):** post = any accepted participant including guest (`s.canRead`); edit = the author only; delete = the author or an owner/admin (`s.canDelete`). A comment the caller cannot reach at all answers `budget.comment_not_found`, never "forbidden".

- [ ] **Step 1: Write the failing write tests**

Append to `internal/budget/comments_test.go`:

```go
func TestCreateComment_GuestMayPost(t *testing.T) {
	h := newCommentHarness(t)

	res, err := h.create(h.guest, h.newID(), "cat-food", "2026-05-01", "  Trip to Lisbon  ")
	if err != nil {
		t.Fatalf("guest post: %v", err)
	}
	if res.Item.Comment != "Trip to Lisbon" {
		t.Fatalf("comment=%q want trimmed", res.Item.Comment)
	}
	if res.Item.Author.Id != h.guest.String() {
		t.Fatalf("author=%q want the guest", res.Item.Author.Id)
	}
	if res.Item.ElementId != h.catFood {
		t.Fatalf("elementId=%q want the external id", res.Item.ElementId)
	}
}

// Review Focus 4: the same client id posted twice leaves one row.
func TestCreateComment_IdempotentOnClientId(t *testing.T) {
	h := newCommentHarness(t)
	id := h.newID()

	first, err := h.create(h.owner, id, "cat-food", "2026-05-01", "double tap")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.create(h.owner, id, "cat-food", "2026-05-01", "double tap")
	if err != nil {
		t.Fatalf("retry rejected: %v", err)
	}
	if first.Item.Id != second.Item.Id {
		t.Fatalf("ids differ: %q vs %q", first.Item.Id, second.Item.Id)
	}
	list := h.list(t, h.owner, "2026-05-01", "1")
	if len(list.Items) != 1 {
		t.Fatalf("items=%d want exactly one row", len(list.Items))
	}
}

func TestCreateComment_PeriodBounds(t *testing.T) {
	h := newCommentHarness(t) // started 2026-04-01

	if _, err := h.create(h.owner, h.newID(), "cat-food", "2026-03-01", "too early"); err == nil {
		t.Fatal("a period before the budget start was accepted")
	}
	h.setEndMonth(t, "2026-07-01")
	if _, err := h.create(h.owner, h.newID(), "cat-food", "2026-08-01", "too late"); err == nil {
		t.Fatal("a period past the end month was accepted")
	}
}

func TestCreateComment_ArchivedAndUncategorized(t *testing.T) {
	h := newCommentHarness(t)

	if _, err := h.create(h.owner, h.newID(), "uncategorized", "2026-05-01", "nope"); err == nil {
		t.Fatal("uncategorized accepted a comment")
	}
	h.archive(t)
	_, err := h.create(h.owner, h.newID(), "cat-food", "2026-05-01", "nope")
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetArchived {
		t.Fatalf("err=%v want budget.archived", err)
	}
}

func TestUpdateComment_AuthorOnly(t *testing.T) {
	h := newCommentHarness(t)
	own := h.mustCreate(t, h.member, "cat-food", "2026-05-01", "mine")

	if _, err := h.update(h.member, own.Item.Id, "mine, edited"); err != nil {
		t.Fatalf("author edit rejected: %v", err)
	}
	_, err := h.update(h.owner, own.Item.Id, "not yours")
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetCommentForbidden {
		t.Fatalf("err=%v want comment_forbidden for the budget owner", err)
	}
}

func TestDeleteComment_AuthorOrModerator(t *testing.T) {
	h := newCommentHarness(t)

	mine := h.mustCreate(t, h.member, "cat-food", "2026-05-01", "mine")
	if _, err := h.remove(h.member, mine.Item.Id); err != nil {
		t.Fatalf("author delete rejected: %v", err)
	}

	theirs := h.mustCreate(t, h.member, "cat-food", "2026-05-01", "moderated")
	if _, err := h.remove(h.owner, theirs.Item.Id); err != nil {
		t.Fatalf("owner moderation rejected: %v", err)
	}

	guests := h.mustCreate(t, h.guest, "cat-food", "2026-05-01", "guest note")
	_, err := h.remove(h.member, guests.Item.Id)
	ae, ok := errs.AsAccessDenied(err)
	if !ok || ae.Code != errs.CodeBudgetCommentForbidden {
		t.Fatalf("err=%v want comment_forbidden for a non-author non-admin", err)
	}
}

// Review Focus 2: a comment id from another budget must look absent, not
// forbidden — existence is not disclosed to an outsider.
func TestUpdateDeleteComment_ForeignBudgetLooksAbsent(t *testing.T) {
	h := newCommentHarness(t)
	foreign := h.commentInAnotherBudget(t)

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"update", func() error { _, err := h.update(h.owner, foreign, "peek"); return err }},
		{"delete", func() error { _, err := h.remove(h.owner, foreign); return err }},
		{"unknown-id", func() error { _, err := h.update(h.owner, h.newID().String(), "peek"); return err }},
	} {
		ve, ok := errs.AsValidation(tc.run())
		if !ok || ve.MsgCode != errs.CodeBudgetCommentNotFound {
			t.Fatalf("%s: want a coded comment_not_found validation error, got %+v", tc.name, ve)
		}
	}
}
```

The shape is a coded `ValidationError` (HTTP 400 with a translated message), not a `NotFoundError`: `errs.NotFoundError` carries no `Code` field (`internal/shared/errs/errs.go:56-58`), so it could not render `errors.budget.comment_not_found` in the caller's language.

- [ ] **Step 2: Run them and watch them fail**

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./internal/budget/ -run "Comment" -v
```
Expected: FAIL — `CreateComment undefined`.

- [ ] **Step 3: Write the three use cases**

Append to `internal/budget/comments.go`:

```go
func (s *Service) CreateComment(ctx context.Context, userID vo.Id, req model.CreateCommentRequest) (*model.CreateCommentResult, error) {
	commentID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	externalID, err := vo.ParseId(req.ElementId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"elementId": ""})
	}
	period, err := time.Parse(datetime.DateLayout, req.Period)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"period": ""})
	}
	period = model.FirstOfMonth(period)

	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	// Guests may comment: a read-only role in the budget still has something to
	// say about the plan.
	if !s.canRead(b, userID) {
		return nil, accessDenied()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	if period.Before(model.FirstOfMonth(b.budget.StartedAt)) {
		return nil, model.ValidateBlank(map[string]string{"period": ""})
	}
	if end := b.endMonth(); end != nil && period.After(*end) {
		return nil, model.ValidateBlank(map[string]string{"period": ""})
	}

	now := s.clock.Now()
	var stored *model.BudgetCommentRow
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// The comment id doubles as the idempotency key: a retried post finds
		// its own row and returns it rather than writing a second comment.
		already, cerr := s.ops.Claim(txCtx, commentID, now)
		if cerr != nil {
			return cerr
		}
		if already {
			row, gerr := s.comments.GetCommentRow(txCtx, commentID)
			if gerr != nil {
				return &errs.ValidationError{Msg: "Operation is locked", MsgCode: errs.CodeOperationLocked}
			}
			stored = row
			return nil
		}
		element, gerr := s.getElementSelfHeal(txCtx, budgetID, externalID, now)
		if gerr != nil {
			return gerr
		}
		c, verr := model.NewBudgetElementComment(commentID, element.ID, userID, req.Comment, period, now)
		if verr != nil {
			return verr
		}
		if ierr := s.comments.InsertComment(txCtx, c); ierr != nil {
			return ierr
		}
		row, gerr := s.comments.GetCommentRow(txCtx, commentID)
		if gerr != nil {
			return gerr
		}
		stored = row
		return s.ops.MarkHandled(txCtx, commentID, now)
	})
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "budget_id", req.BudgetId)
	reqctx.AddLogAttr(ctx, "comment_id", req.Id)
	return &model.CreateCommentResult{Item: commentResult(*stored)}, nil
}

func (s *Service) UpdateComment(ctx context.Context, userID vo.Id, req model.UpdateCommentRequest) (*model.UpdateCommentResult, error) {
	commentID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, commentNotFound()
	}
	row, b, err := s.reachableComment(ctx, userID, commentID)
	if err != nil {
		return nil, err
	}
	if !row.Comment.UserID.Equal(userID) {
		return nil, commentForbidden()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}

	now := s.clock.Now()
	edited := row.Comment
	if verr := edited.Edit(req.Comment, now); verr != nil {
		return nil, verr
	}
	if err := s.comments.UpdateCommentText(ctx, &edited); err != nil {
		return nil, err
	}
	row.Comment = edited
	reqctx.AddLogAttr(ctx, "budget_id", row.BudgetID.String())
	reqctx.AddLogAttr(ctx, "comment_id", req.Id)
	return &model.UpdateCommentResult{Item: commentResult(*row)}, nil
}

func (s *Service) DeleteComment(ctx context.Context, userID vo.Id, req model.DeleteCommentRequest) (*model.DeleteCommentResult, error) {
	commentID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, commentNotFound()
	}
	row, b, err := s.reachableComment(ctx, userID, commentID)
	if err != nil {
		return nil, err
	}
	// The author cleans up after themselves; owner and admin moderate.
	if !row.Comment.UserID.Equal(userID) && !s.canDelete(b, userID) {
		return nil, commentForbidden()
	}
	if aerr := s.requireNotArchived(b); aerr != nil {
		return nil, aerr
	}
	if err := s.comments.DeleteComment(ctx, commentID); err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "budget_id", row.BudgetID.String())
	reqctx.AddLogAttr(ctx, "comment_id", req.Id)
	return &model.DeleteCommentResult{}, nil
}

// reachableComment loads a comment and its budget, answering "not found" for
// anything the caller cannot read — an id in someone else's budget must not be
// distinguishable from an id that never existed.
func (s *Service) reachableComment(ctx context.Context, userID, commentID vo.Id) (*model.BudgetCommentRow, *budgetAggregate, error) {
	row, err := s.comments.GetCommentRow(ctx, commentID)
	if err != nil {
		return nil, nil, commentNotFound()
	}
	b, err := s.loadAggregate(ctx, row.BudgetID)
	if err != nil {
		return nil, nil, commentNotFound()
	}
	if !s.canRead(b, userID) {
		return nil, nil, commentNotFound()
	}
	return row, b, nil
}

func commentNotFound() error {
	return &errs.ValidationError{Msg: "Comment not found", MsgCode: errs.CodeBudgetCommentNotFound}
}

func commentForbidden() error {
	return &errs.AccessDeniedError{Msg: "You can't change this comment", Code: errs.CodeBudgetCommentForbidden}
}
```

Check `errs.AccessDeniedError`'s field names against `internal/shared/errs/errs.go` before compiling.

- [ ] **Step 4: Run the write tests**

```bash
go test ./internal/budget/ -run Comment -v
```
Expected: PASS, including the read tests from Task 3 that post through `CreateComment`.

- [ ] **Step 5: Run the whole backend suite**

```bash
make go-test 2>&1 | tail -25
```
Expected: PASS, coverage gate satisfied.

- [ ] **Step 6: Commit**

```bash
git add internal/budget
git commit -m "feat(budget): post, edit and delete budget cell comments

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Lifecycle — reset, clone, merge

**Files:**
- Modify: `internal/budget/crud.go` (`ResetBudget`, ~line 216)
- Modify: `internal/budget/clone.go` (~line 143, the `WithLimits` block)
- Modify: `internal/budget/merge.go` (`MergeService`, lines 20-60)
- Modify: `internal/server/server.go` (the `NewMergeService` call, ~line 231)
- Test: `internal/budget/comments_lifecycle_test.go`

**Interfaces:**
- Consumes: `CommentStore.DeleteCommentsByBudget`, `.ListCommentsFrom`, `.RepointComments`, `.InsertComment`, `.NextIdentity` (Task 2)
- Produces: `budget.NewMergeService(elements ElementStore, limits LimitStore, comments CommentStore, clock port.Clock) *MergeService` — note the changed signature

- [ ] **Step 1: Write the failing lifecycle tests**

Create `internal/budget/comments_lifecycle_test.go`:

```go
package budget_test

import "testing"

func TestResetBudget_DeletesComments(t *testing.T) {
	h := newCommentHarness(t)
	h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "before the reset")

	h.reset(t, "2026-05-01")

	res := h.list(t, h.owner, "2026-05-01", "12")
	if len(res.Items) != 0 {
		t.Fatalf("items=%d want 0: reset re-anchors the start month, so kept comments would be orphans", len(res.Items))
	}
}

func TestCloneBudget_CopiesCommentsWithLimitsOnly(t *testing.T) {
	h := newCommentHarness(t)
	h.mustCreate(t, h.owner, "cat-food", "2026-04-01", "before the clone start")
	h.mustCreate(t, h.owner, "cat-food", "2026-06-01", "after the clone start")

	bare := h.clone(t, "Bare copy", "2026-05-01", false)
	if got := h.listIn(t, bare, h.owner, "2026-04-01", "12"); len(got.Items) != 0 {
		t.Fatalf("items=%d want 0 without withLimits", len(got.Items))
	}

	full := h.clone(t, "Full copy", "2026-05-01", true)
	got := h.listIn(t, full, h.owner, "2026-04-01", "12")
	if len(got.Items) != 1 || got.Items[0].Comment != "after the clone start" {
		t.Fatalf("items=%+v want only the comment at or after the start month", got.Items)
	}
	if got.Items[0].Author.Id != h.owner.String() {
		t.Fatalf("author=%q want the original author preserved", got.Items[0].Author.Id)
	}
	if got.Items[0].Id == "" {
		t.Fatal("copied comment kept an empty id")
	}
}

func TestMergeCategories_RepointsComments(t *testing.T) {
	h := newCommentHarness(t)
	h.mustCreate(t, h.owner, "cat-food", "2026-05-01", "on the source")
	h.mustCreate(t, h.owner, "cat-transport", "2026-05-01", "on the target")

	h.mergeCategory(t, "cat-food", "cat-transport")

	res := h.list(t, h.owner, "2026-05-01", "1")
	if len(res.Items) != 2 {
		t.Fatalf("items=%d want both threads on the target", len(res.Items))
	}
	for _, item := range res.Items {
		if item.ElementId != h.catTransport {
			t.Fatalf("elementId=%q want the merge target", item.ElementId)
		}
	}
}
```

Extend the harness with `reset`, `clone`, `listIn` and `mergeCategory`. For the merge, drive `MergeService` the way `internal/budget/merge_test.go` already does.

- [ ] **Step 2: Run them and watch them fail**

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./internal/budget/ -run "Reset|Clone|Merge" -v
```
Expected: FAIL — reset keeps the comment, clone copies nothing, merge leaves the source thread behind.

- [ ] **Step 3: Delete comments on reset**

In `internal/budget/crud.go`, inside `ResetBudget`'s transaction, immediately after `DeleteLimitsByBudget`:

```go
		// Reset re-anchors the start month, so comments below the new start
		// would be rows no view renders and the list endpoint still returns.
		if cerr := s.comments.DeleteCommentsByBudget(txCtx, budgetID); cerr != nil {
			return cerr
		}
```

- [ ] **Step 4: Copy comments on clone**

In `internal/budget/clone.go`, inside the existing `if req.WithLimits {` block, after the limit loop:

```go
			comments, cerr := s.comments.ListCommentsFrom(txCtx, sourceID, startDate)
			if cerr != nil {
				return cerr
			}
			for _, c := range comments {
				mapped, ok := elementMap[c.ElementID]
				if !ok {
					continue
				}
				// Author and timestamps are the record; only the ids are new.
				copied := *c
				copied.ID = s.comments.NextIdentity()
				copied.ElementID = mapped
				if ierr := s.comments.InsertComment(txCtx, &copied); ierr != nil {
					return ierr
				}
			}
```

- [ ] **Step 5: Repoint comments on merge**

In `internal/budget/merge.go`, add a `comments CommentStore` field to `MergeService`, take it as the third parameter of `NewMergeService`, and in the branch where a target element already exists — right before the source element is deleted — repoint:

```go
		if rerr := s.comments.RepointComments(ctx, src.ID, target.ID); rerr != nil {
			return rerr
		}
```

The other branch repoints the element row itself, so its comments travel with it and need no work.

Update the call in `internal/server/server.go`:

```go
	classificationMerger := classificationBudgetMerger{svc: appbudget.NewMergeService(budgetRepo, budgetRepo, budgetRepo, clk)}
```

- [ ] **Step 6: Run the lifecycle tests, then everything**

```bash
go test ./internal/budget/ -run "Reset|Clone|Merge|Comment" -v
make go-test 2>&1 | tail -25
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/budget internal/server/server.go
git commit -m "feat(budget): carry comments through reset, clone and merge

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: REST edge — routes, handlers, swagger, apiparity

**Files:**
- Create: `internal/budget/api/comments.go`
- Modify: `internal/budget/api/routes.go`
- Create: `internal/test/apiparity/catalogue_budget_comments.go`
- Modify: `internal/test/apiparity/guard_test.go` (`minRoutes`), `internal/test/apiparity/catalogue_test.go` (`min`)
- Regenerate: `internal/web/apidoc/docs/*`, `internal/test/apiparity/testdata/golden/budget_comments.golden`

**Interfaces:**
- Consumes: the four service methods from Tasks 3-4
- Produces: `GET /api/v1/budget/get-comment-list`, `POST /api/v1/budget/create-comment`, `POST /api/v1/budget/update-comment`, `POST /api/v1/budget/delete-comment`

- [ ] **Step 1: Write the handlers**

Create `internal/budget/api/comments.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// GetCommentList handles GET /api/v1/budget/get-comment-list.
//
// @Summary  Comment threads for a budget's cells in a month window
// @Tags     Budget
// @Produce  json
// @Param    budgetId query string  true  "Budget id"
// @Param    from     query string  false "Window start (Y-m-d, snapped to first of month; defaults to the current month)"
// @Param    months   query integer false "Window length in months (1-24, default 1)"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.GetCommentListResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/get-comment-list [get]
func (h *Handlers) GetCommentList(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := model.GetCommentListRequest{BudgetId: q.Get("budgetId"), From: q.Get("from"), Months: q.Get("months")}
	res, err := h.svc.GetCommentList(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

// CreateComment handles POST /api/v1/budget/create-comment.
//
// @Summary  Post a comment on a budget cell
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.CreateCommentRequest true "Create comment"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.CreateCommentResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  402 {object} apidoc.JsonResponseError
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/create-comment [post]
func (h *Handlers) CreateComment(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.CreateCommentRequest) (*model.CreateCommentResult, error) {
		reqctx.AddLogAttr(ctx, "element_id", req.ElementId)
		return h.svc.CreateComment(ctx, userID, req)
	})
}

// UpdateComment handles POST /api/v1/budget/update-comment.
//
// @Summary  Edit your own comment
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.UpdateCommentRequest true "Update comment"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.UpdateCommentResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  402 {object} apidoc.JsonResponseError
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/update-comment [post]
func (h *Handlers) UpdateComment(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateComment)
}

// DeleteComment handles POST /api/v1/budget/delete-comment.
//
// @Summary  Delete a comment (author, or the budget owner/admin)
// @Tags     Budget
// @Accept   json
// @Produce  json
// @Param    request body model.DeleteCommentRequest true "Delete comment"
// @Success  200 {object} apidoc.JsonResponseOk{data=model.DeleteCommentResult}
// @Failure  400 {object} apidoc.JsonResponseError
// @Failure  401 {object} apidoc.JsonResponseUnauthorized
// @Failure  402 {object} apidoc.JsonResponseError
// @Failure  500 {object} apidoc.JsonResponseException
// @Security Bearer
// @Router   /api/v1/budget/delete-comment [post]
func (h *Handlers) DeleteComment(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.DeleteComment)
}
```

- [ ] **Step 2: Register the routes**

In `internal/budget/api/routes.go`, after the `set-limit` / `move-element` block:

```go
		mux.Handle("GET /api/v1/budget/get-comment-list", auth(h.GetCommentList))
		mux.Handle("POST /api/v1/budget/create-comment", auth(h.CreateComment))
		mux.Handle("POST /api/v1/budget/update-comment", auth(h.UpdateComment))
		mux.Handle("POST /api/v1/budget/delete-comment", auth(h.DeleteComment))
```

The literal `"METHOD /path"` string on a line containing `auth(` is what the guards source-scan; keep that shape.

- [ ] **Step 3: Write the parity scenario**

Create `internal/test/apiparity/catalogue_budget_comments.go`:

```go
package apiparity

// budget_comments exercises the four comment routes end to end: the owner posts
// on a category cell, a guest posts on the same cell, the owner edits their own
// comment and moderates the guest's, and the list read returns the window. The
// err: calls pin the refusals: a guest editing someone else's comment, an
// unknown comment id, and a period before the budget's start month.
func init() {
	register(Scenario{Name: "budget_comments", Calls: func() []Call {
		const (
			commentBudget  = "b0000000-0000-0000-0000-0000000000c7"
			ownerComment   = "c0000000-0000-0000-0000-0000000000c1"
			guestComment   = "c0000000-0000-0000-0000-0000000000c2"
			unknownComment = "c0000000-0000-0000-0000-0000000000ff"
		)
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": commentBudget, "name": "Comments", "currencyId": USD, "startDate": "2024-04-01", "accountIds": []string{OwnerAccount}}},
			{Label: "share-with-guest", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
				Body: map[string]any{"id": commentBudget, "userId": GuestUser, "role": "guest"}},
			{Label: "guest-accepts", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"id": commentBudget}},

			{Label: "owner-posts", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": ownerComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "Trip to Lisbon"}},
			{Label: "owner-posts-retry", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": ownerComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "Trip to Lisbon"}},
			{Label: "guest-posts", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "guest",
				Body: map[string]any{"id": guestComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "Book the flights"}},

			{Label: "owner-edits-own", Method: "POST", Path: "/api/v1/budget/update-comment", Auth: "owner",
				Body: map[string]any{"id": ownerComment, "comment": "Trip to Lisbon in May"}},
			{Label: "err:guest-edits-owners", Method: "POST", Path: "/api/v1/budget/update-comment", Auth: "guest",
				Body: map[string]any{"id": ownerComment, "comment": "not mine"}},
			{Label: "err:unknown-comment", Method: "POST", Path: "/api/v1/budget/update-comment", Auth: "owner",
				Body: map[string]any{"id": unknownComment, "comment": "nobody home"}},
			{Label: "err:period-before-start", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": unknownComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-01-01", "comment": "too early"}},
			{Label: "err:blank-comment", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": unknownComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "   "}},

			{Label: "list-window", Method: "GET", Auth: "owner",
				Path: "/api/v1/budget/get-comment-list?budgetId=" + commentBudget + "&from=2024-05-01&months=2"},
			{Label: "err:list-bad-months", Method: "GET", Auth: "owner",
				Path: "/api/v1/budget/get-comment-list?budgetId=" + commentBudget + "&from=2024-05-01&months=99"},

			{Label: "owner-moderates-guest", Method: "POST", Path: "/api/v1/budget/delete-comment", Auth: "owner",
				Body: map[string]any{"id": guestComment}},
			{Label: "list-after-delete", Method: "GET", Auth: "owner",
				Path: "/api/v1/budget/get-comment-list?budgetId=" + commentBudget + "&from=2024-05-01&months=2"},
		}
	}})
}
```

Check `internal/test/apiparity/fixture.go` for the real names of the guest user and the shared category constants (`GuestUser`, `CatFood`) and use whatever it actually exports. If the fixture has no second user with an accepted share, model the guest calls on `catalogue_budget_access.go`, which already does this dance.

- [ ] **Step 4: Raise the two floors**

`internal/test/apiparity/guard_test.go` — `minRoutes` 122 -> 126, adding a dated line to the changelog comment above it:

```go
	// 2026-09: +4 budget cell comment routes (get-comment-list, create/update/delete-comment).
	const minRoutes = 126
```

`internal/test/apiparity/catalogue_test.go` — `min` 56 -> 57 with the same kind of note.

- [ ] **Step 5: Regenerate the OpenAPI docs and the goldens, then read both diffs**

```bash
export PATH=/usr/local/go/bin:$PATH
make swagger
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
git diff --stat internal/web/apidoc internal/test/apiparity/testdata
git diff internal/test/apiparity/testdata/golden/budget_comments.golden | head -80
```
**Only `budget_comments.golden` may be new; no existing golden may change.** If another golden moved, stop and find out why — the comment feature must not alter any existing response.

- [ ] **Step 6: Run the guards**

```bash
go test ./internal/test/apiparity/ -v 2>&1 | tail -30
```
Expected: PASS, including `TestGuard_EveryRouteHasScenario`, `TestGuard_EveryRestrictedPostDocuments402` (the three POSTs are NOT on the readonly allowlist, so they must document 402) and `TestGuard_EveryAuthenticatedRouteDocuments401And500`.

- [ ] **Step 7: Commit**

```bash
git add internal/budget/api internal/test/apiparity internal/web/apidoc
git commit -m "feat(budget): expose budget cell comments over REST

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: MCP tools

**Files:**
- Modify: `internal/budget/mcp/mcp.go`
- Modify: `internal/test/mcpparity/catalogue.go`
- Regenerate: `internal/test/mcpparity/testdata/golden/*`

**Interfaces:**
- Consumes: the four service methods (Tasks 3-4)
- Produces: MCP tools `list_budget_comments`, `create_budget_comment`, `update_budget_comment`, `delete_budget_comment`

- [ ] **Step 1: Add the tools**

In `internal/budget/mcp/mcp.go`, inside `Register`'s closure, after the `set_limit` tool. Follow the existing shape exactly: `reqctx.AddLogAttr(ctx, "tool", ...)` first, then `webmcp.UserID(ctx)`, then `webmcp.MapErr(ctx, err)` on the way out.

```go
		type listCommentsInput struct {
			BudgetID string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
			Month    string `json:"month,omitempty" jsonschema:"YYYY-MM; defaults to the current month"`
			Months   int    `json:"months,omitempty" jsonschema:"window length in months, 1-24 (default 1)"`
		}

		sdk.AddTool(s, &sdk.Tool{Name: "list_budget_comments",
			Description: "Comment threads on a budget's cells for a month window. Each comment names its element (the id get_budget returns) and its month."},
			func(ctx context.Context, req *sdk.CallToolRequest, in listCommentsInput) (*sdk.CallToolResult, model.GetCommentListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_budget_comments")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetCommentListResult{}, err
				}
				from := ""
				if in.Month != "" {
					if _, perr := time.Parse("2006-01", in.Month); perr != nil {
						return nil, model.GetCommentListResult{}, errs.NewValidation("month must be YYYY-MM")
					}
					from = in.Month + "-01"
				}
				months := ""
				if in.Months != 0 {
					months = strconv.Itoa(in.Months)
				}
				res, err := svc.GetCommentList(ctx, userID, model.GetCommentListRequest{BudgetId: in.BudgetID, From: from, Months: months})
				if err != nil {
					return nil, model.GetCommentListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		type createCommentInput struct {
			BudgetID  string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
			ElementID string `json:"element_id" jsonschema:"envelope, category or tag id (UUID), from get_budget"`
			Month     string `json:"month" jsonschema:"YYYY-MM; must be inside the budget's months"`
			Comment   string `json:"comment" jsonschema:"plain text, 1-500 characters"`
		}

		sdk.AddTool(s, &sdk.Tool{Name: "create_budget_comment",
			Description: "Post a comment on one budget cell (element + month). Every participant may post, read-only guests included."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createCommentInput) (*sdk.CallToolResult, model.CreateCommentResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_budget_comment")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateCommentResult{}, err
				}
				if _, perr := time.Parse("2006-01", in.Month); perr != nil {
					return nil, model.CreateCommentResult{}, errs.NewValidation("month must be YYYY-MM")
				}
				res, err := svc.CreateComment(ctx, userID, model.CreateCommentRequest{
					Id:       vo.NewId().String(), // comment id, minted server-side for MCP
					BudgetId: in.BudgetID, ElementId: in.ElementID, Period: in.Month + "-01", Comment: in.Comment,
				})
				if err != nil {
					return nil, model.CreateCommentResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		type updateCommentInput struct {
			CommentID string `json:"comment_id" jsonschema:"comment id (UUID), from list_budget_comments"`
			Comment   string `json:"comment" jsonschema:"plain text, 1-500 characters"`
		}

		sdk.AddTool(s, &sdk.Tool{Name: "update_budget_comment",
			Description: "Edit a comment you wrote. Only the author may edit."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateCommentInput) (*sdk.CallToolResult, model.UpdateCommentResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_budget_comment")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateCommentResult{}, err
				}
				res, err := svc.UpdateComment(ctx, userID, model.UpdateCommentRequest{Id: in.CommentID, Comment: in.Comment})
				if err != nil {
					return nil, model.UpdateCommentResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		type deleteCommentInput struct {
			CommentID string `json:"comment_id" jsonschema:"comment id (UUID), from list_budget_comments"`
		}

		sdk.AddTool(s, &sdk.Tool{Name: "delete_budget_comment",
			Description: "Delete a comment. The author may delete their own; the budget owner or an admin may delete any."},
			func(ctx context.Context, req *sdk.CallToolRequest, in deleteCommentInput) (*sdk.CallToolResult, model.DeleteCommentResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "delete_budget_comment")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.DeleteCommentResult{}, err
				}
				res, err := svc.DeleteComment(ctx, userID, model.DeleteCommentRequest{Id: in.CommentID})
				if err != nil {
					return nil, model.DeleteCommentResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
```

Add `strconv` to the file's imports if it is not already there.

- [ ] **Step 2: Add the mcpparity scenario**

In `internal/test/mcpparity/catalogue.go`, next to the other budget scenarios:

```go
	const mcpCommentBudget = "b0000000-0000-0000-0000-0000000000c8"
	register(Scenario{Name: "budget_comments", Steps: []Step{
		{Label: "seed-budget", Method: "POST", Path: "/api/v1/budget/create-budget",
			Body: map[string]any{"id": mcpCommentBudget, "name": "MCP Comments", "currencyId": apiparity.USD, "startDate": "2024-04-01", "accountIds": []string{apiparity.OwnerAccount}}},
		{Label: "create-comment", CaptureAs: "comment_id", MCPCapturePath: []string{"item", "id"},
			RPC: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_budget_comment","arguments":{"budget_id":"` + mcpCommentBudget + `","element_id":"` + apiparity.CatFood + `","month":"2024-05","comment":"Trip to Lisbon"}}}`},
		{Label: "list-comments",
			RPC: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_budget_comments","arguments":{"budget_id":"` + mcpCommentBudget + `","month":"2024-05"}}}`},
		{Label: "update-comment",
			RPC: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"update_budget_comment","arguments":{"comment_id":"{{comment_id}}","comment":"Trip to Lisbon in May"}}}`},
		{Label: "create-comment-bad-month",
			RPC: `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_budget_comment","arguments":{"budget_id":"` + mcpCommentBudget + `","element_id":"` + apiparity.CatFood + `","month":"junk","comment":"nope"}}}`},
		{Label: "delete-comment",
			RPC: `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"delete_budget_comment","arguments":{"comment_id":"{{comment_id}}"}}}`},
		{Label: "list-after-delete",
			RPC: `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"list_budget_comments","arguments":{"budget_id":"` + mcpCommentBudget + `","month":"2024-05"}}}`},
	}})
```

- [ ] **Step 3: Regenerate the MCP goldens and read the diff**

```bash
export PATH=/usr/local/go/bin:$PATH
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/
git diff --stat internal/test/mcpparity/testdata
```
`lifecycle.golden` WILL change — it pins the whole `tools/list` output, and four tools just appeared. Read that diff and confirm the four new entries are the only change.

- [ ] **Step 4: Run the MCP tiers**

```bash
go test ./internal/test/mcpparity/ ./internal/budget/mcp/ -v 2>&1 | tail -20
```
Expected: PASS, including `TestReadonlyBlocksEveryTool` (the three write tools must 402 for a read-only caller) and `TestFullAccessReachesTools`.

- [ ] **Step 5: Full backend gate**

```bash
make go-test 2>&1 | tail -25
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/budget/mcp internal/test/mcpparity
git commit -m "feat(budget): expose budget cell comments over MCP

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: SPA data layer — client, DTOs, hooks, metrics

**Files:**
- Modify: `web/src/api/dto/budget.ts`, `web/src/api/budget.ts`
- Modify: `web/src/features/budgets/queries.ts`
- Modify: `web/src/app/queryKeys.ts`
- Modify: `web/src/lib/metrics.ts`
- Test: `web/src/features/budgets/comments.queries.test.tsx`

**Interfaces:**
- Consumes: the four REST routes (Task 6)
- Produces: `budgetApi.getCommentList/createComment/updateComment/deleteComment`, `useBudgetComments(budgetId, from, months)` returning `{ items, byCell: Map<string, BudgetCommentDto[]>, truncated }`, `commentCellKey(elementId, period)`, `useCreateComment(budgetId)`, `useUpdateComment(budgetId)`, `useDeleteComment(budgetId)`, `METRICS.BUDGET_CREATE_COMMENT | BUDGET_UPDATE_COMMENT | BUDGET_DELETE_COMMENT`

- [ ] **Step 1: Write the failing hook tests**

Create `web/src/features/budgets/comments.queries.test.tsx`, modelled on `web/src/features/budgets/queries.test.tsx` (open it first and reuse its `makeWrapper()`).

```tsx
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { trackEvent } from '@/lib/metrics'
import { useBudgetComments, useCreateComment, commentCellKey } from './queries'

vi.mock('@/lib/metrics', async () => {
  const actual = await vi.importActual<typeof import('@/lib/metrics')>('@/lib/metrics')
  return { ...actual, trackEvent: vi.fn() }
})

const comment = {
  id: 'cm1',
  elementId: 'e1',
  period: '2026-05-01',
  comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-05-17 09:00:00',
  updatedAt: '2026-05-17 09:00:00',
}

beforeEach(() => {
  localStorage.clear()
  server.use(...coreHandlers())
})

it('groups comments by element and period', async () => {
  server.use(
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: false } }),
    ),
  )
  const { result } = renderHook(() => useBudgetComments('b1', '2026-05-01', 1), { wrapper: makeWrapper() })
  await waitFor(() => expect(result.current.byCell.size).toBe(1))
  expect(result.current.byCell.get(commentCellKey('e1', '2026-05-01'))).toHaveLength(1)
  expect(result.current.truncated).toBe(false)
})

it('sends the window as query parameters', async () => {
  let url = ''
  server.use(
    http.get('*/api/v1/budget/get-comment-list', ({ request }) => {
      url = request.url
      return HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } })
    }),
  )
  const { result } = renderHook(() => useBudgetComments('b1', '2026-05-01', 4), { wrapper: makeWrapper() })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(url).toContain('budgetId=b1')
  expect(url).toContain('from=2026-05-01')
  expect(url).toContain('months=4')
})

it('mints the comment id, fires the metric and rolls back on error', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } }),
    ),
    http.post('*/api/v1/budget/create-comment', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: comment } })
    }),
  )
  const { result } = renderHook(() => useCreateComment('b1'), { wrapper: makeWrapper() })
  result.current.mutate({ elementId: 'e1', period: '2026-05-01', comment: 'Trip to Lisbon' })
  await waitFor(() => expect(body).toBeDefined())
  expect(typeof body!.id).toBe('string')
  expect((body!.id as string).length).toBeGreaterThan(30)
  await waitFor(() => expect(vi.mocked(trackEvent)).toHaveBeenCalledWith('appBudgetCreateComment'))
})
```

Add the failing-POST rollback case in the same file: make the create handler answer 400 and assert the optimistic row disappears from `byCell` and `toast.error` was called.

- [ ] **Step 2: Run them and watch them fail**

```bash
cd web && pnpm test -- comments.queries 2>&1 | tail -20
```
Expected: FAIL — `useBudgetComments` is not exported.

- [ ] **Step 3: DTOs and client functions**

`web/src/api/dto/budget.ts`:

```ts
export interface BudgetCommentDto {
  id: Id
  elementId: Id
  /** first of the month, Y-m-d */
  period: string
  comment: string
  author: UserDto
  /** frozen "Y-m-d H:i:s", server time */
  createdAt: string
  updatedAt: string
}
```

(`UserDto` is the existing `{ id, avatar, name }` embed — reuse whatever `AccessDto.user` is typed as rather than declaring a second one.)

`web/src/api/budget.ts`:

```ts
export interface CommentWindow {
  budgetId: Id
  /** first of the month, Y-m-d */
  from: string
  months: number
}

export interface CreateCommentForm {
  id: Id
  budgetId: Id
  elementId: Id
  period: string
  comment: string
}

export async function getCommentList(params: CommentWindow): Promise<{ items: BudgetCommentDto[]; truncated: boolean }> {
  const query = new URLSearchParams({ budgetId: params.budgetId, from: params.from, months: String(params.months) })
  const response = await api.get<Envelope<{ items: BudgetCommentDto[]; truncated: boolean }>>(
    apiUrl(`/api/v1/budget/get-comment-list?${query.toString()}`),
  )
  return response.data.data
}

export async function createComment(form: CreateCommentForm): Promise<BudgetCommentDto> {
  const response = await api.post<Envelope<{ item: BudgetCommentDto }>>(apiUrl('/api/v1/budget/create-comment'), form)
  return response.data.data.item
}

export async function updateComment(form: { id: Id; comment: string }): Promise<BudgetCommentDto> {
  const response = await api.post<Envelope<{ item: BudgetCommentDto }>>(apiUrl('/api/v1/budget/update-comment'), form)
  return response.data.data.item
}

export async function deleteComment(form: { id: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/budget/delete-comment'), form)
}
```

- [ ] **Step 4: Query key, metrics, hooks**

`web/src/app/queryKeys.ts`: add `budgetComments: ['budgetComments'] as const`.

`web/src/lib/metrics.ts`, in the budget block:

```ts
  BUDGET_CREATE_COMMENT: 'appBudgetCreateComment',
  BUDGET_UPDATE_COMMENT: 'appBudgetUpdateComment',
  BUDGET_DELETE_COMMENT: 'appBudgetDeleteComment',
```

`web/src/features/budgets/queries.ts`:

```ts
/** Cache key for one cell's thread: element external id + first-of-month period. */
export function commentCellKey(elementId: Id, period: string): string {
  return `${elementId}|${period}`
}

export function useBudgetComments(budgetId: Id | null, from: string, months: number) {
  const key = [...queryKeys.budgetComments, budgetId ?? 'none', from, months] as const
  const query = useQuery({
    queryKey: key,
    queryFn: () => budgetApi.getCommentList({ budgetId: budgetId as Id, from, months }),
    enabled: budgetId !== null,
    staleTime: TEN_MINUTES,
  })
  const byCell = useMemo(() => {
    const map = new Map<string, BudgetCommentDto[]>()
    for (const item of query.data?.items ?? []) {
      const cell = commentCellKey(item.elementId, item.period)
      const bucket = map.get(cell)
      if (bucket) {
        bucket.push(item)
      } else {
        map.set(cell, [item])
      }
    }
    return map
  }, [query.data])
  return { ...query, items: query.data?.items ?? [], truncated: query.data?.truncated ?? false, byCell, commentsKey: key }
}
```

The three mutations follow `usePlanSetLimit`'s shape — `onMutate` patches every cached `budgetComments` list for this budget, `onError` restores the snapshot and toasts `apiErrorMessage(err)` with a fixed toast id, `onSuccess` fires the metric and invalidates `queryKeys.budgetComments`. The id is minted inside `mutationFn` so no call site can send its own:

```ts
export function useCreateComment(budgetId: Id) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { elementId: Id; period: string; comment: string }) =>
      budgetApi.createComment({ id: uuidv7(), budgetId, ...form }),
    onError: (err) => {
      toast.error(apiErrorMessage(err), { id: 'budget-comment-error' })
    },
    onSuccess: () => {
      trackEvent(METRICS.BUDGET_CREATE_COMMENT)
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetComments })
    },
  })
}
```

Write `useUpdateComment` and `useDeleteComment` the same way with their own metrics. Optimistic patching is worth it only for create (the composer should feel instant); for update and delete, invalidate and let the refetch settle it — but if you add optimistic patches, add the matching rollback test.

Both views must invalidate each other: `queryKeys.budgetComments` is a single prefix covering every window, so one invalidate keeps the plan and monthly caches in sync.

- [ ] **Step 5: Run the hook tests and the type check**

```bash
cd web && pnpm test -- comments.queries 2>&1 | tail -20
cd web && pnpm exec tsc --noEmit && pnpm lint
```
Expected: PASS, 0 type errors, 0 lint errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/api web/src/app/queryKeys.ts web/src/lib/metrics.ts web/src/features/budgets
git commit -m "feat(web): budget comment API client and query hooks

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: The `CommentThread` component and its copy

**Files:**
- Create: `web/src/features/budgets/CommentThread.tsx`
- Create: `web/src/features/budgets/CommentThread.test.tsx`
- Modify: `locales/*.json` (11)

**Interfaces:**
- Consumes: `useCreateComment`, `useUpdateComment`, `useDeleteComment` (Task 8), `pluralPick` (`web/src/lib/plural.ts`)
- Produces:

```tsx
export interface CommentThreadProps {
  budgetId: Id
  elementId: Id
  /** first of the month, Y-m-d */
  period: string
  comments: BudgetCommentDto[]
  currentUserId: Id | undefined
  /** owner/admin may delete anyone's comment */
  canModerate: boolean
  /** archived budget or a month outside the budget's range: read the thread, write nothing */
  readOnly: boolean
}
export function CommentThread(props: CommentThreadProps): JSX.Element
```

- [ ] **Step 1: Add the copy**

In `locales/en.json` under `budgets.page.plan`, add a `comments` block (and the same keys in the other ten catalogues, translated):

```json
        "comments": {
          "title": "Comments",
          "marker_aria": "{count} comment | {count} comments",
          "composer_placeholder": "Add a note for this month",
          "post": "Post",
          "edit": "Edit",
          "delete": "Delete",
          "cancel": "Cancel",
          "delete_confirm": "Delete this comment?",
          "edited": "(edited)",
          "counter": "{count}/500",
          "read_only": "This budget is archived — comments can be read but not changed.",
          "empty": "No comments yet.",
          "truncated": "Showing the first 2000 comments."
        }
```

`marker_aria` is a pipe plural read through `pluralPick`, never through i18next's own plural suffixes.

- [ ] **Step 2: Write the failing component test**

Create `web/src/features/budgets/CommentThread.test.tsx`. Cover, each as its own `it`:

1. renders every comment with its author name and local date, oldest first;
2. shows `(edited)` only when `updatedAt !== createdAt`;
3. own comment offers Edit and Delete; another author's offers neither when `canModerate` is false;
4. another author's offers Delete but not Edit when `canModerate` is true;
5. the composer posts on click and on Cmd/Ctrl+Enter, and is disabled while the textarea is blank;
6. the counter shows `n/500` and the post button disables past 500 characters;
7. `readOnly` hides the composer and every menu, and shows the read-only hint.

Use the `BudgetUpdateDialog.test.tsx` harness shape: a real `QueryClientProvider`, msw handlers for the mutation endpoints, `userEvent.setup()`, and assertions on English strings (the setup loads the real catalogue).

- [ ] **Step 3: Run it and watch it fail**

```bash
cd web && pnpm test -- CommentThread 2>&1 | tail -20
```
Expected: FAIL — the module does not exist.

- [ ] **Step 4: Build the component**

Keep it presentational plus the three mutations; no data fetching inside. Structure:

- a `<ul>` of comments: avatar (reuse the existing avatar renderer used for budget access rows), author name, `toLocaleString()` timestamp, the text, `(edited)` when applicable;
- per-comment actions rendered only when `!readOnly && (isAuthor || canModerate)`: Edit (inline `<textarea>` with Save/Cancel, author only) and Delete (confirm via the existing confirm dialog pattern in this folder);
- a composer `<textarea>` + Post button, `onKeyDown` handling `(e.metaKey || e.ctrlKey) && e.key === 'Enter'`, a live `n/500` counter, disabled when the trimmed value is empty or over 500 runes (`[...value].length`, not `value.length` — Review Focus 1 on the client side);
- when `readOnly`, render the hint and nothing writable.

The whole component must carry `data-slot="popover-content"`-compatible focus behavior: it will be mounted inside a Radix popover and inside `ResponsiveDialog`, both of which the plan grid's `KEYDOWN_ESCAPE_SELECTOR` already matches — do not add a keydown handler that stops propagation globally.

- [ ] **Step 5: Run the tests, types and lint**

```bash
cd web && pnpm test -- CommentThread 2>&1 | tail -20
cd web && pnpm exec tsc --noEmit && pnpm lint
```
Expected: PASS / clean.

- [ ] **Step 6: Run the i18n guard**

```bash
export PATH=/usr/local/go/bin:$PATH
go test ./internal/test/i18ntest/ 2>&1 | tail -10
```
Expected: PASS — every `t('budgets.page.plan.comments.*')` literal resolves and all 11 catalogues carry the block.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/budgets/CommentThread.tsx web/src/features/budgets/CommentThread.test.tsx locales
git commit -m "feat(web): shared budget comment thread component

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Plan view integration

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Modify: `web/src/features/budgets/LimitEditor.tsx`
- Create: `web/src/features/budgets/CommentsDialog.tsx`
- Test: `web/src/features/budgets/comments.plan.test.tsx`

**Interfaces:**
- Consumes: `useBudgetComments`, `commentCellKey` (Task 8), `CommentThread` (Task 9)
- Produces: `CommentsDialog({ open, onClose, title, ...CommentThreadProps })`; `LimitEditor` gains an optional `footer?: ReactNode` prop; `GridCtx` gains `commentsByCell: Map<string, BudgetCommentDto[]>` and `openComments(target: PlanLimitTarget): void`

- [ ] **Step 1: Write the failing integration test**

Create `web/src/features/budgets/comments.plan.test.tsx`, copying the harness from `PlanSheet.test.tsx` (the dnd-kit mock, `mockViewport`/`mockCompactViewport`, `renderPage('/plan')`, `usePlanHandlers()`, the fake-timer `beforeEach`). Add a `get-comment-list` handler returning one comment on element `pe1` for the month rendered in column 1.

```tsx
it('marks a cell that has comments and opens its thread from the limit popover', async () => {
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  expect(within(cell).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(within(cell).getByLabelText(/^limit /))
  await user.click(await screen.findByRole('button', { name: /Comments \(1\)/ }))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})

it('shows no marker on a cell without comments', async () => {
  mockViewport()
  renderPage('/plan')
  const cell = await screen.findByTestId('plan-cell-pe1:2')
  expect(within(cell).queryByTestId('comment-marker')).toBeNull()
})

it('never marks the uncategorized row', async () => {
  // the fixture's uncategorized row carries no element id a comment could name
  mockViewport()
  renderPage('/plan')
  const cell = await screen.findByTestId('plan-cell-uncategorized:1')
  expect(within(cell).queryByTestId('comment-marker')).toBeNull()
})

it('opens the thread with Shift+Enter and leaves Enter editing the amount', async () => {
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(cell)
  screen.getByTestId('plan-sheet').focus()

  await user.keyboard('{Shift>}{Enter}{/Shift}')
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  await user.keyboard('{Escape}')

  await user.keyboard('{Enter}')
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
})

it('opens the thread in a dialog on compact viewports', async () => {
  mockCompactViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(within(cell).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run it and watch it fail**

```bash
cd web && pnpm test -- comments.plan 2>&1 | tail -20
```
Expected: FAIL — no `comment-marker`.

- [ ] **Step 3: Fetch the threads alongside the plan**

In `PlanSheet`, next to the existing `useBudgetPlan(...)` call, add:

```tsx
  const { byCell: commentsByCell } = useBudgetComments(budget.meta.id, fetchFrom, planFetchWindow(firstMonth, visibleMonths).months)
```

Reuse the plan's own window so the two caches cover the same months. Add `commentsByCell` and `openComments` to `GridCtx` and to its `useMemo` dependency list.

- [ ] **Step 4: Render the marker**

In `ElementRow`, inside the `role="gridcell"` div and before the fill handle, when the cell has comments and the element is not `UNCATEGORIZED_ID`:

```tsx
{commentCount > 0 ? (
  <button
    type="button"
    data-testid="comment-marker"
    aria-label={pluralPick(t('budgets.page.plan.comments.marker_aria'), commentCount, i18n.language)}
    className="absolute right-0 top-0 h-0 w-0 border-l-[6px] border-t-[6px] border-l-transparent border-t-primary"
    onClick={(e) => {
      e.stopPropagation()
      ctx.openComments({ el, month: m, monthIndex: idx })
    }}
  />
) : null}
```

A corner triangle drawn with borders takes no layout space, so no column width changes. Copy the `e.stopPropagation()` discipline from the existing fill handle.

- [ ] **Step 5: Mount the thread in the popover and the dialog**

Give `LimitEditor` an optional `footer?: ReactNode` rendered at the bottom of its `PopoverContent`, and pass a "Comments (n)" disclosure button from `ElementRow` that expands `CommentThread` in place. Widen `PopoverContent` from `w-64` to `w-80` only when a footer is present, so the amount-only popover is unchanged.

Create `CommentsDialog.tsx` as a thin `ResponsiveDialog` wrapper around `CommentThread`, following `SetLimitDialog`'s "null target renders nothing" shape. Use it for the compact path and for cells that are not editable (a guest's view), where there is no `LimitEditor` to hang a footer on.

- [ ] **Step 6: Wire Shift+Enter**

In `handleKeyDown`'s element branch, before the existing `case 'Enter'`:

```tsx
        case 'Enter':
          e.preventDefault()
          if (e.shiftKey) {
            const target = selectedMonthCell()
            if (target) {
              openCommentsFromGrid(target)
            }
            return
          }
          handleEnter(entry, selection.col)
          return
```

Set `editorFromGrid.current = true` before opening, the way `handleEnter` does for the compact dialog, so grid focus is restored when the thread closes.

- [ ] **Step 7: Run the plan tests plus the existing suite**

```bash
cd web && pnpm test -- comments.plan 2>&1 | tail -20
cd web && pnpm test 2>&1 | tail -20
```
Expected: PASS. `PlanSheet.test.tsx` must still pass untouched — if a keyboard test broke, the Shift+Enter branch is swallowing a plain Enter.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/budgets
git commit -m "feat(web): budget cell comments in the plan grid

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Monthly view, regression plan, whole-branch gate

**Files:**
- Modify: `web/src/features/budgets/BudgetPage.tsx`, `web/src/features/budgets/BudgetTable.tsx`, `web/src/features/budgets/SetLimitDialog.tsx`
- Modify: `docs/regression-test-plan.md`
- Test: `web/src/features/budgets/comments.monthly.test.tsx`

**Interfaces:**
- Consumes: everything from Tasks 8-10
- Produces: `ElementRowExtras` gains `renderBudgetCellMarker?: (element: BudgetElementDto) => ReactNode`

- [ ] **Step 1: Write the failing monthly test**

Create `web/src/features/budgets/comments.monthly.test.tsx` using `BudgetPage.test.tsx`'s harness:

```tsx
it('marks the budgeted cell and opens the thread from the limit popover', async () => {
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/budget')

  const row = await screen.findByTestId('element-e1')
  expect(within(row).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(within(row).getByLabelText(/^limit /))
  await user.click(await screen.findByRole('button', { name: /Comments \(1\)/ }))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})

it('offers the thread inside SetLimitDialog on compact viewports', async () => {
  mockCompactViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/budget')

  const row = await screen.findByTestId('element-e1')
  await user.click(within(row).getByTestId('cell-available'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})

it('lets a guest open a read-only thread on a cell they cannot edit', async () => {
  // render with a budget whose meta.access gives the current user the guest role
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderGuestPage('/budget')

  const row = await screen.findByTestId('element-e1')
  await user.click(within(row).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Post' })).toBeInTheDocument()
})
```

The last assertion is the point of the guest case: guests are read-only for amounts but **not** for comments, so the composer must be present.

- [ ] **Step 2: Run it and watch it fail**

```bash
cd web && pnpm test -- comments.monthly 2>&1 | tail -20
```

- [ ] **Step 3: Wire the monthly view**

- `BudgetPage` fetches `useBudgetComments(budget.meta.id, selectedDate, 1)`.
- `BudgetTable` gains `renderBudgetCellMarker` in `ElementRowExtras` and renders it inside the `cell-budgeted` span, next to the existing content.
- `BudgetPage` passes a marker that opens `CommentsDialog` for the `(element, selectedDate)` cell, and passes the same `footer` disclosure into the `LimitEditor` it already renders.
- `SetLimitDialog` gains an optional `comments?: ReactNode` slot rendered under the amount card; `BudgetPage` and `PlanSheet` fill it with `CommentThread` for the dialog's cell.

- [ ] **Step 4: Update the regression plan**

In `docs/regression-test-plan.md`, in the budget module section, add verifiable items:

- Post a comment on a plan cell; it appears immediately and survives a reload. 📱
- Open the same cell in the monthly view for that month: the comment is there (cross-view sync). 📱
- Edit your own comment: the text updates and `(edited)` appears.
- Another participant cannot edit your comment; the budget owner can delete it.
- A guest (read-only role) can post, edit and delete their own comment. 📱
- A cell with comments shows the corner marker; a cell without shows none; the uncategorized row never shows one.
- Archive the budget: threads are readable, the composer is gone.
- Reset the budget: planned amounts AND comments are cleared (REST route only — there is no UI for reset).
- Clone a budget with plans: comments at or after the start month come across with their original authors; cloning without plans copies none.
- Merge two categories: the source cell's thread appears on the target cell.
- Revoke a participant: their comments on surviving cells still render their name.

- [ ] **Step 5: Run every gate**

```bash
export PATH=/usr/local/go/bin:$PATH
make go-test 2>&1 | tail -25
make test-repo-pgsql 2>&1 | tail -10
cd web && pnpm test 2>&1 | tail -20
cd web && pnpm exec tsc --noEmit && pnpm lint
```
All four must pass. Then confirm no golden drifted unexpectedly:

```bash
git status --short internal/test
```
Expected: clean (the goldens were committed in Tasks 6-7).

- [ ] **Step 6: Commit and open the PR for review**

```bash
git add web/src/features/budgets docs/regression-test-plan.md
git commit -m "feat(web): budget cell comments in the monthly view

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push -u origin feature/budget-cell-comments
```

PR #246 already exists and is a draft; pushing updates it. Leave it in draft and report the result.

---

## Notes for the executor

- **The spec is the authority.** Where this plan and `docs/superpowers/specs/2026-09-12-budget-cell-comments-design.md` disagree, the spec wins — and say so in your report rather than silently picking one.
- **Test harness names in Tasks 3-5 are invented by this plan** (`newCommentHarness`, `h.list`, `h.create`, ...). Write them once, in Task 3, in whatever shape the existing budget tests already use. If the package has a better-established harness, use that instead and adapt the calls; do not build a parallel test framework.
- **Do not touch `get-budget` or `get-budget-plan`.** If an existing golden changes, something leaked into the budget reads — stop and find it.
- **Generated code is generated.** Never hand-edit `internal/infra/storage/sqlc/gen/**`, `internal/web/apidoc/docs/**`, or any `*.golden`.
