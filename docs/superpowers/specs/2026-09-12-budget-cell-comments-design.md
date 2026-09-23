# Budget Cell Comments — Design

Date: 2026-09-12 · Revised: 2026-09-22 (design review — see "Revisions")
Status: Approved design, pending implementation plan
Ships **before** the budget savings feature (#245); see "Seam with budget savings".

## Overview

Every budget cell — one budget element (category, tag, envelope, income category,
income envelope) in one month — can carry a **flat discussion thread**: any number
of short plain-text comments, each with its author, ordered oldest first. Use case:
note plans against a month ("Trip to Lisbon", "buying a TV") in the plan view, and
see them again in the monthly budget view when that month arrives.

Comments are a data source **independent of limits**: a cell can have comments and
no limit, clearing a limit never touches its comments, and the existing
`get-budget` / `get-budget-plan` reads are not changed at all.

## Decisions made during brainstorming

| Topic | Decision |
|---|---|
| Where | Both views — plan view and monthly budget view — can read and write |
| Visibility | Shared with the budget: every participant sees the same threads |
| Relation to limits | Independent; comment-only cells are normal |
| Shape | Flat thread per cell (multiple comments, author stored); no replies/nesting |
| Who posts | owner, admin, user **and guest** (read-only budget role may comment) |
| Who edits | the author only |
| Who deletes | the author, plus budget owner/admin (moderation) |
| Storage | New table, not a column on `budgets_elements_limits` |
| Reads | One dedicated window endpoint `get-comment-list`; budget reads untouched |
| UI | Implement the design below; opening/reading UX will be refined after implementation |
| Reset budget | *(2026-09-22)* Deleted with the limits — reset re-anchors the start month, so survivors would be orphans |
| Savings rows (#245) | *(2026-09-22)* Savings cells carry threads like any other cell |

## Backend

### Table `budgets_elements_comments`

SQLite + PostgreSQL migrations (next version after the latest in each engine dir).

| column | type | notes |
|---|---|---|
| `id` | TEXT / UUID-as-text PK | client-minted UUIDv7 |
| `element_id` | TEXT NOT NULL | FK → `budgets_elements(id)` ON DELETE CASCADE |
| `period` | DATETIME / TIMESTAMP(0) NOT NULL | first of month, stored `Y-m-d H:i:s` — bind a formatted string, never a raw `time.Time` (modernc stores `t.String()` otherwise) |
| `user_id` | TEXT NOT NULL | FK → `users(id)` ON DELETE CASCADE (users are never hard-deleted in practice; cascade keeps the FK valid if they are) |
| `comment` | TEXT NOT NULL | trimmed, 1–500 runes |
| `created_at`, `updated_at` | DATETIME / TIMESTAMP(0) NOT NULL | |

Indexes: `(element_id, period)`, `(period)`. No uniqueness beyond the PK.

Thread order everywhere: `created_at ASC, id ASC` (UUIDv7 ids break same-second ties
in insertion order).

### Model

`internal/model/budget.go`: `BudgetElementComment{ID, ElementID, Period, UserID,
Comment, CreatedAt, UpdatedAt}` with an invariant-preserving constructor/`Edit`
that trims and validates the text (`common.is_blank` when empty, `common.too_long`
over 500 runes, field `comment`).

DTOs in `internal/model/budget_dto.go`:

- `GetCommentListRequest{BudgetId, From, Months}` — `From` `Y-m-d` (snapped to the
  first of the month), `Months` 1–24, default 1.
- `CreateCommentRequest{Id, BudgetId, ElementId, Period, Comment}` — all required.
- `UpdateCommentRequest{Id, Comment}`.
- `DeleteCommentRequest{Id}`.
- `CommentResult{Id, ElementId, Period, Comment, Author UserResult, CreatedAt, UpdatedAt}`
  — `ElementId` is the element's **external id** (unique within a budget, same
  identifier `set-limit` takes); `Period` is `Y-m-d`; datetimes use the frozen
  `2006-01-02 15:04:05` layout; `Author` is the shared `{id, avatar, name}` embed.
- `GetCommentListResult{Items []CommentResult, Truncated bool}` →
  `{"items": [...], "truncated": false}` (`items` is `[]`, never `null`, when empty,
  matching `GetBudgetListResult`; `truncated` is always present and true only when
  the 2000-item cap cut the tail).

### Use cases (`internal/budget/comments.go`)

**List** — `GetCommentList`: requires read access (any accepted role, incl. guest;
pending invite denied). Window `[from, from + months)`. Returns every comment on
every element of the budget in that window, ordered `period, external_id,
created_at, id`. Archived budgets and months outside start/end are still readable.
The response is unpaginated, so it is capped: `months` ≤ 24 (already validated) and
at most 2000 comments, oldest first; hitting the cap truncates the tail and sets
`truncated: true` on the result so the SPA can say so rather than silently showing a
partial thread. A budget that large is pathological — the cap is a guard, not a
paging design.

**Create** — `CreateComment`:
1. Validate request; parse and snap `period`.
2. Access: owner, admin, user, guest (accepted).
3. `requireNotArchived` → 403 `budget.archived`.
4. Period bounds exactly as `SetLimit`: before `startedAt` or past `endMonth()` →
   validation error.
5. In the transaction: shared `OperationGuard` claim on the client `id` (the
   category/tag create pattern) — a retry of an already-handled id returns the
   existing comment instead of inserting a duplicate; `getElementSelfHeal` for the
   element (rejects `uncategorized`/non-UUID like `set-limit`); insert; mark handled.
6. Return the `CommentResult`.

**Update** — `UpdateComment`: load comment → element → budget; caller must have
access to the budget (else `budget.comment_not_found`, so existence is not
disclosed); caller must be the author (else 403 `budget.comment_forbidden`);
`requireNotArchived`; edit text; save (`updated_at = now`). Return the result.

**Delete** — `DeleteComment`: same lookup/not-found rule; allowed for the author or a
budget owner/admin (else `budget.comment_forbidden`); `requireNotArchived`; delete.
`requireNotArchived` on delete means threads can never be cleaned up once a budget
is archived. Deliberate, and consistent with the lifecycle spec's allowlist (only
unarchive/delete/revoke/decline/accept escape the archived guard): archiving freezes
the budget's record, comments included. Unarchive, edit, re-archive is the escape
hatch.

The account-level `readonly` access state (402 middleware) applies unchanged — none
of the new POST routes are allowlisted.

New coded errors in `internal/shared/errs/codes.go` (+ `AllCodes`), with
`errors.budget.comment_not_found` / `errors.budget.comment_forbidden` in all 11
`locales/<lang>.json`.

Operation log attrs via `reqctx.AddLogAttr`: `budget_id`, `element_id`,
`comment_id` (ids only).

### Repository

A `CommentStore` role interface in `internal/budget/repository.go`, folded into the
composite; sqlc queries in `query/{sqlite,pgsql}/budgets.sql` via the
engine-adapter pattern (no hand-built SQL needed):

- `ListBudgetCommentsForWindow(budget_id, from, to)` — join `budgets_elements` (for
  `budget_id`, `external_id`) and `users` (name, avatar); SQLite compares periods
  with `datetime()` on both sides like the limit queries.
- `GetBudgetComment(id)` (with element's `budget_id`), `InsertBudgetComment`,
  `UpdateBudgetCommentText`, `DeleteBudgetComment`.
- `ListBudgetCommentsFrom(element_id, from)` — for clone.
- `RepointBudgetComments(src_element_id, dst_element_id)` — for merge.
- `DeleteBudgetCommentsByBudget(budget_id)` — for reset.

Watch the sqlc semicolon-placement landmine (a `;` on its own line truncates the
generated SQL).

### Lifecycle

| Event | Effect on comments |
|---|---|
| Delete budget / delete envelope / element removed by `syncElements` | cascade with the element |
| Member revoked / leaves | their category/tag elements cascade (threads go with them); their comments on surviving elements stay and keep rendering their author |
| Archive category/tag | element stays → threads stay |
| Type (side) change of an element | updated in place → threads stay |
| Reset budget | deleted together with limits (`DeleteBudgetCommentsByBudget` next to `DeleteLimitsByBudget`, same transaction) — see below |
| Clone budget with `withLimits` | comments with `period >= startDate` copied to the new elements (envelope ids remapped as for limits), **original authors and timestamps kept**, new ids; not copied without `withLimits` |
| Merge category/tag — no target element in that budget | element repointed in place → threads come along |
| Merge category/tag — target element exists | source comments repointed onto the target element before the source element is deleted; threads interleave by `created_at` |

`reset-budget` (`internal/budget/crud.go:195-235`, owner/admin, blocked on archived)
is not "clear the numbers": alongside `DeleteLimitsByBudget` it calls
`StartFrom(startedAt)` and **re-anchors the budget's start month**. Surviving
comments on periods before the new start would be unreachable — the plan and
monthly readers clamp to the start month, and `create-comment`'s period bounds
refuse a sibling — while `get-comment-list` would still return them for an early
window. Invisible rows the API keeps serving is worse than losing the notes, and
deleting every comment (not just the ones below the new start) keeps the rule
symmetric with limits, which die regardless of period. Note that reset has **no
client surface today** — no SPA call, no MCP tool, no CLI, only the REST route —
so there is no confirmation dialog to design here; whoever surfaces it owes the
user one that names both effects.

A cloned thread keeps its original author, so the clone's participants see the name
and avatar of someone who may not be a member of the new budget (and the clone can
be shared with new people later). Accepted: the alternative — restamping every
comment with the cloner — misattributes plain statements of fact, which is worse.
Clone is owner/admin-only, and those authors were participants of the source budget
the cloner already belonged to.

### REST routes (`internal/budget/api`)

| Method + path | Handler combinator |
|---|---|
| `GET /api/v1/budget/get-comment-list?budgetId&from&months` | hand-written query-param read (like `get-budget-plan`) |
| `POST /api/v1/budget/create-comment` | `endpoint.Handle` |
| `POST /api/v1/budget/update-comment` | `endpoint.Handle` |
| `POST /api/v1/budget/delete-comment` | `endpoint.Handle` |

Swag annotations on each; `make swagger` regenerates committed docs.

### MCP (`internal/budget/mcp`)

- `list_budget_comments{budget_id, month "YYYY-MM", months?}`
- `create_budget_comment{budget_id, element_id, month, comment, id?}` — server mints
  a UUIDv7 when `id` is omitted.
- `update_budget_comment{comment_id, comment}`
- `delete_budget_comment{comment_id}`

Existing budget tools are unchanged.

### Tests

- Model: trimming, blank, 500/501 runes (multi-byte).
- Service: permission matrix (owner/admin/user/guest/pending × create/update own/
  update other/delete own/delete other), archived rejections, period bounds, window
  bounds (1 and 24 months, invalid values), idempotent create retry, uncategorized
  rejection, self-heal.
- Lifecycle: clone with/without `withLimits` and `startDate` filter + envelope
  remap, merge both branches, reset, revoke member, envelope delete.
- Repo tests on both engines (`make test-repo-pgsql`).
- apiparity: new `budget_comments` scenario covering all four routes plus coded
  errors; guard tests pick up the new routes. Existing goldens must not change.
- mcpparity: new scenario for the four tools; `tools/list` golden grows.
- enginecompare covers both via the catalogues.

## Frontend

### API + hooks

- `web/src/api/budget.ts`: `getCommentList`, `createComment`, `updateComment`,
  `deleteComment`; DTO types in `api/dto/budget.ts`.
- `features/budgets/queries.ts`:
  - `useBudgetComments(budgetId, from, months)` — query key
    `['budget-comments', budgetId, from, months]`; returns the list plus a memoized
    `Map<"elementId|period", Comment[]>`.
  - `useCreateComment` (mints the UUIDv7 at this choke point — no call site sends
    its own), `useUpdateComment`, `useDeleteComment` — optimistic patch of the
    matching cached lists, then invalidate every `budget-comments` query for the
    budget so the monthly and plan views stay in sync; errors toast via
    `apiErrorMessage`.
- Monthly view requests `from = selected month, months = 1`; plan view requests its
  existing fetch window.

### Cell marker

A cell with ≥1 comment shows a small top-right corner triangle (no layout shift)
with an aria-label / tooltip "{count} comments" (pipe-delimited plural via
`pluralPick`). Monthly view: on the budgeted cell. Plan view: on the planned cell.
Uncategorized rows never show a marker or a comment entry point.

### `CommentThread` component (shared)

- List: avatar, author name, local date/time, "(edited)" when `updatedAt ≠ createdAt`.
- Own comments: Edit (inline textarea, save/cancel) and Delete (confirm). Owner/admin
  additionally get Delete on others' comments.
- Composer: textarea, post button + Cmd/Ctrl+Enter, live `n/500` counter, disabled
  when blank.
- Read-only (composer + menus hidden) when the budget is archived or the month is
  outside start/end month. Guests are not read-only for comments.

### Entry points

- **Desktop, editable cell**: `LimitEditor` popover gains a "Comments (n)" footer
  that expands the `CommentThread` in the same popover.
- **Desktop, non-editable cell** (e.g. guest): clicking the cell opens a popover with
  only the thread.
- **Plan grid keyboard**: Shift+Enter on the selected cell opens the thread (Enter
  keeps editing the amount).
- **📱 Compact**: `SetLimitDialog` gains a Comments section under the amount; for
  non-editable cells long-press/tap opens a `CommentsDialog` (`ResponsiveDialog`).
- Fill-right and paste never touch comments.

This opening/reading UX is explicitly provisional — the user will refine it after
the feature is implemented.

### Analytics

`METRICS` keys `BUDGET_CREATE_COMMENT`, `BUDGET_UPDATE_COMMENT`,
`BUDGET_DELETE_COMMENT` (→ `appBudgetCreateComment`, …), fired in the three
mutation hooks' `onSuccess` so every surface is covered once.

### i18n

New `budgets.comments.*` keys (marker label, thread title, composer placeholder,
post/edit/delete/cancel, delete confirm, "(edited)", counter, read-only hint) in all
11 catalogues; plus the two `errors.budget.*` codes.

### Tests

vitest for the hooks (id minted and sent, optimistic patch + rollback on error,
grouping map), `CommentThread` (own vs others' menus, owner moderation, read-only,
counter/blank/limit, Cmd+Enter), marker rendering in both views, guest entry
point, and `metrics-coverage` passing.

## Docs

`docs/regression-test-plan.md`: add items for posting/editing/deleting (own vs
owner/admin moderation), guest posting, archived read-only threads, markers in both
views (📱 compact dialog), cross-view sync, and the reset/clone/merge/revoke
effects — reset via the REST route, since it has no client entry point.

## Seam with budget savings (#245)

The two features are independent and ship separately, comments first. They meet in
exactly three places; these are the agreed terms, so neither branch has to guess.

- **Savings cells carry threads.** A savings row is a real `budgets_elements` row of
  type `ElementSavings = 5` whose external id is the account id, so `create-comment`,
  `get-comment-list` and the whole permission/lifecycle story work on it with **no
  backend change** — `element_id` is an element FK, not a category FK. Nothing in
  this spec needs to name savings.
- **The marker and entry point inside the Savings section are #245's work**, not this
  branch's: the section does not exist yet. That PR reuses `CommentThread` and the
  corner-triangle marker as they are, so the components here must stay row-type
  agnostic — keyed by `elementId|period`, never by category/tag/envelope.
- **Conflict surfaces when #245 rebases onto this:** `internal/model/budget_dto.go`,
  `internal/shared/errs/codes.go` + `AllCodes`, all 11 `locales/<lang>.json`,
  `web/src/lib/metrics.ts`, `PlanSheet.tsx` (`buildFlatRows` + cell rendering), the
  apiparity/mcpparity goldens, and `docs/regression-test-plan.md`. All additive;
  #245 rebases, this branch does not wait.

## Out of scope

Replies/nesting, mentions, notifications/email, reactions, rich text, comment
search, comments on uncategorized rows, carrying comments via fill-right/paste.

## Revisions

**2026-09-22 — design review (#245 and #246 reviewed together).**

1. **Reset still deletes comments** — revisited and confirmed. The first pass of
   this review reversed it on "a limit is a number you can retype, a note is not",
   which missed that reset also re-anchors the start month: kept comments below the
   new start would be orphans that no view renders but `get-comment-list` still
   returns. `DeleteBudgetCommentsByBudget` stays, and the reasoning is written down
   in the lifecycle section so it does not get reversed again.
2. **`get-comment-list` is capped** at 2000 items with a `truncated` flag — it was
   unbounded over 24 months × every element of the budget.
3. **Clone author disclosure** written down and accepted rather than inherited.
4. **Archived budgets block deletion too** — confirmed intended, with the reason.
5. **Seam with #245** recorded: savings cells carry threads, the savings PR owns the
   savings-section UI.
