# Budget lifecycle: end date, archived state, clone

**Date:** 2026-08-17 · **Branch:** `feature/budget-lifecycle` (builds on
`bug/budget-account-membership`, spec
`2026-08-16-budget-account-membership-design.md`)

## Problem

A budget today has no end and no way to be copied. Users who want a safety
copy before restructuring, or who want to close a budget and continue in a
fresh one from this month, can only create a new empty budget by hand and
rebuild folders, envelopes, sharing and plans.

## Design summary

- A budget gains an **end date** (`ended_at`, the last covered month) and an
  **archived** flag (hidden + read-only). Both are settable independently;
  the UI's "Complete budget" sets both in one flow.
- A budget can be **cloned** by its owner: structure, membership, sharing
  (same roles and accepted state) and, optionally, plans — starting either
  from the source's start (a full backup) or from this month (a
  continuation).
- Start dates are never changed (decided; `reset-budget` stays as it is).

## 1. Data model

Migration `20260818000000`, both engines (`ALTER TABLE … ADD COLUMN`; no
sqlite rebuild):

```sql
ALTER TABLE budgets ADD COLUMN ended_at    DATETIME NULL;
ALTER TABLE budgets ADD COLUMN is_archived BOOLEAN NOT NULL DEFAULT 0;
```

`model.Budget` gains `EndedAt *time.Time` and `IsArchived bool`, with
mutators `EndAt(month *time.Time, now)` (snapped to first-of-month; must be
≥ `StartedAt`, else `ErrBudgetEndBeforeStart`; nil clears), `Archive(now)`,
`Unarchive(now)`. sqlc budget queries carry the two columns.

## 2. Semantics

### End date

`ended_at` is the **last month the budget covers** (inclusive).

- `get-budget` and `get-transaction-list` snap a requested period later
  than `ended_at` down to `ended_at` (they already snap to a month).
- `get-budget-plan`'s window ends at `ended_at`.
- `set-limit` refuses `period > ended_at` (mirror of the existing
  `period < started_at` guard: blank-validation on `period`).
- The membership removal boundary (membership spec §4) becomes
  `min(localMonth(now), firstOfMonth(ended_at) + 1 month)`.

### Archived

`is_archived` = hidden + read-only.

- Every budget write on an archived budget is refused with a **coded
  `errs.AccessDeniedError`, code `budget.archived`** ("This budget is
  archived") — checked right after `loadAggregate` in each use case —
  **except**: `unarchive-budget`, `delete-budget`, `revoke-access`,
  `decline-access`, `accept-access` (a pending invite may still be
  answered).
- Reads are unchanged. Active-budget pointers are not touched; the SPA
  renders an archived active budget read-only with a banner.
- Archiving does not set `ended_at`; unarchiving does not clear it.

## 3. Wire contract

`MetaResult` (get-budget, get-budget-list, create/update/clone results) gains:

- `endedAt`: `"Y-m-d H:i:s"` (first-of-month) or `""` when unset — the
  codebase's empty-string-for-NULL convention;
- `isArchived`: int `0`/`1` (the category convention).

| Endpoint | Change |
|---|---|
| `POST update-budget` | + optional `endDate` (`*string`): absent → untouched; `""` → clear; a date (`Y-m-d`) → set, snapped to first-of-month, must be ≥ `started_at` (coded `budget.end_before_start`, field `endDate`). `canUpdate`. |
| `POST archive-budget` `{id}` | owner\|admin (`canDelete`). `{item: meta}`. Idempotent. |
| `POST unarchive-budget` `{id}` | owner\|admin. `{item: meta}`. Idempotent. Not blocked by the archived guard. |
| `POST clone-budget` | see §4. |

New coded errors: `budget.archived`, `budget.end_before_start`, registered
in `errs.AllCodes` and translated in all 11 catalogues.

## 4. Clone

`POST /api/v1/budget/clone-budget`
`{id, newId, name, startDate?, withLimits}` → `{item: BudgetResult}` (the
copy, built for the caller's current month — the create-budget shape).

- **Authorization**: the caller must be the source's **owner**
  (`budgetRole == BudgetRoleOwner`). The source may be archived or ended.
- `newId`: client-supplied id (as `create-budget`), blank-validated;
  `name`: `ValidateName`; `withLimits`: bool.
- `startDate`: absent/empty → the source's `started_at`; otherwise
  `Y-m-d`, snapped to first-of-month, validated
  `source.started_at ≤ startDate ≤ localMonth(now)` (blank-validation on
  `startDate` otherwise). The source's `ended_at` does not constrain it.
- **One transaction**, fresh UUIDv7 ids, an old→new id map. Copied:
  - budget row: `currency_id`, `name`, `started_at = startDate`,
    `ended_at = NULL`, `is_archived = 0`, `user_id = caller`,
    `created_at = updated_at = now`;
  - `budgets_access`: every row (`user_id`, `role`, `is_accepted` as-is),
    timestamps `now`;
  - `budgets_accounts`: every row, `created_at = now`;
  - `budgets_folders`: name, sort key;
  - `budgets_envelopes` (+ `budgets_envelopes_categories`): all fields,
    folder remapped;
  - `budgets_elements`: same `external_id`/`type`, folder remapped,
    currency, sort key;
  - `budgets_elements_limits`: **only when `withLimits`**, and only rows
    with `period ≥ startDate`, element remapped.
- Not copied / not done: `ended_at`, `is_archived`, the source's timestamps,
  anyone's active-budget pointer (the SPA switches it for the "continue"
  flow via `user/update-budget`).
- Every participant sees the copy in their list immediately (accepted state
  copied — the point of "same access for all participants").

Repository: a `Clone` write path in `internal/budget/repo` (sqlc inserts
already exist for every table; the use case drives them with the id map).

## 5. Frontend (`web/src/features/budgets`)

- **BudgetsPage** per-budget actions gain **Duplicate…**, **Complete…**,
  **Archive** / **Unarchive** (owner|admin visibility rules mirror the
  server). The list gains a collapsed **Archived** group; the budget
  switcher lists archived budgets in the same group.
- **Duplicate…** dialog: name (prefilled with the source name), "Copy plans"
  (default on), "Start from this month" (default off = full backup) →
  `clone-budget`.
- **Complete…** dialog: end month (default last month; min `started_at`),
  "Continue in a copy starting this month" (default on) with "Copy plans"
  (default on) and a name field (prefilled). Runs sequentially:
  `update-budget(endDate)` → `archive-budget` → `clone-budget(startDate =
  this month)` → `user/update-budget` (active = the copy) → navigate to
  it. Each step is a valid end state on its own; a failure surfaces the
  standard error toast and stops.
- **BudgetUpdateDialog**: a clearable **End month** field.
- **BudgetPage**: archived → read-only controls + banner
  (`budgets.page.archived_banner`); period navigation and the plan sheet
  respect `endedAt`.
- `METRICS`: `appBudgetCloned`, `appBudgetCompleted`, `appBudgetArchived`,
  `appBudgetUnarchived`, `appBudgetEndDateSet`, fired at the mutations'
  success points.

## 6. MCP

`clone_budget`, `archive_budget`, `unarchive_budget`; `update_budget` gains
`end_date`; `list_budgets` / `get_budget` output carries
`endedAt`/`isArchived`. mcpparity goldens regenerated and inspected.

## 7. i18n

`errors.budget.archived`, `errors.budget.end_before_start`, plus the dialog
copy (Duplicate, Complete, Archive/Unarchive, End month, archived banner,
"Copy plans", "Start from this month", "Continue in a copy") in all 11
catalogues.

## 8. Testing

1. **Clone equivalence**: `get-budget` of the copy for a shared period equals
   the source's structure/access/membership with ids remapped; limits copied
   only for `period ≥ startDate`; `withLimits=false` → no limits; archived
   source cloneable; admin/user/guest → 403; `startDate` before the source
   start or after this month → 400.
2. **Archived write matrix**: every budget write route on an archived budget
   → `budget.archived`, except the allowlist (unarchive, delete, revoke,
   decline, accept) which succeed.
3. **End date**: get-budget / get-budget-plan / get-transaction-list clamp to
   `ended_at`; set-limit after it → 400; `endDate < started_at` → 400;
   `""` clears.
4. Membership removal boundary honours `ended_at`.
5. apiparity + mcpparity goldens regenerated and diff-inspected; `make test`
   for engine parity of the migration and new queries.
6. SPA vitest: Duplicate and Complete dialogs (call sequence + stop-on-error),
   Archived group, banner, End month field.

## 9. Out of scope

- Changing a budget's start date (decided against; `reset-budget` unchanged).
- Cloning by non-owners ("fork").
- Automatic archiving when `ended_at` passes.
