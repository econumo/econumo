# MCP budget create + configure tools — Design

Date: 2026-07-19
Status: approved (brainstormed with the maintainer)

Sub-project 2 of the three-part MCP write-surface expansion (sub-project 1,
category/tag/payee management, is merged on this branch). Sub-project 3
(transaction richer-filters + bulk-update) follows later. This one lets an MCP
client build and configure a budget: create it, organize it into folders and
envelopes, set monthly limits, and choose which accounts it tracks.

## Decisions (from brainstorming)

- **No hard delete, no reorder.** Consistent with sub-project 1. Envelope
  archiving is folded into `update-envelope` (an `isArchived` field), which fits
  archive-not-delete. Budgets and folders have no archive in the REST API
  (delete-only), so they simply aren't removable via MCP — acceptable.
- **Sharing out of scope.** grant/accept/revoke/decline budget access is a
  collaboration handshake, separate from configuring one's own budget; deferred
  (own sub-project later).
- **change-element-currency, reset-budget, move-element/order-folder out.**
  Advanced/rare or destructive/reorder — YAGNI.
- **Eight tools.** MCP surface 22 → 30.

## Architecture

No new package. The tools are added to `internal/budget/mcp/mcp.go`'s existing
`Register` func (which today registers `list_budgets` + `get_budget`), each a
thin handler over the budget `Service` write methods, following the established
MCP write-tool pattern (`internal/transaction/mcp`, `internal/category/mcp`):
`reqctx.AddLogAttr` → `webmcp.UserID` → service call → `webmcp.MapErr`. No
cross-feature imports (only the budget service, already wired in `server.go`).

## The eight tools

| Tool | Service method | Inputs | Output |
|---|---|---|---|
| `create_budget` | `CreateBudget` | `name`, `currency_id`, `start_date` (YYYY-MM-DD) | `model.CreateBudgetResult` (`{item}`) |
| `update_budget` | `UpdateBudget` | `budget_id`, `name`, `currency_id` | `model.UpdateBudgetResult` |
| `create_folder` | `CreateFolder` (`CreateBudgetFolderRequest`) | `budget_id`, `name` | `model.CreateBudgetFolderResult` (`{item}`) |
| `update_folder` | `UpdateFolder` (`UpdateBudgetFolderRequest`) | `budget_id`, `id`, `name` | `model.UpdateBudgetFolderResult` |
| `create_envelope` | `CreateEnvelope` | `budget_id`, `name`, `icon`, `currency_id`, `folder_id?`, `category_ids[]` | `model.CreateEnvelopeResult` (`{item}`) |
| `update_envelope` | `UpdateEnvelope` | `budget_id`, `id`, `name`, `icon`, `currency_id`, `category_ids[]`, `archived` (bool → `isArchived` 0/1) | `model.UpdateEnvelopeResult` |
| `set_limit` | `SetLimit` | `budget_id`, `element_id`, `month` (YYYY-MM), `amount?` (decimal string; omit → clear) | `{budget_id, element_id, month, amount}` confirmation |
| `set_budget_account_included` | `IncludeAccount` / `ExcludeAccount` | `budget_id`, `account_id`, `included` (bool) | `{budget_id, account_id, included}` confirmation |

The implementer MUST verify the exact budget folder/envelope service method
names and DTO field names against source before writing; if any differ from the
DTOs referenced here (`CreateBudgetFolderRequest{BudgetId,Id,Name}`,
`UpdateBudgetFolderRequest{BudgetId,Id,Name}`, `CreateEnvelopeRequest`,
`UpdateEnvelopeRequest` with `IsArchived int` + `Categories []string`,
`SetLimitRequest{BudgetId,ElementId,Period,Amount *vo.FlexString}`,
`Include/ExcludeAccountRequest{BudgetId (json "id"), AccountId}`), STOP and
report the discrepancy.

## Semantics

- **Server-minted ids.** `CreateBudget`/create-folder/`CreateEnvelope` take the
  client-supplied id as the ENTITY id (verified: `CreateBudget` does
  `vo.ParseId(req.Id)` as the budget id — no separate op-id guard). The tools
  mint `vo.NewId().String()`; the model never supplies ids. `update_*`,
  `set_limit`, and the account toggle take target ids as input (from
  `list_*`/`get_budget`).
- **Envelope** groups categories: `category_ids` from `list_categories`,
  `currency_id` from `list_currencies` or the budget's currency, `icon` a
  Material icon name (same set as accounts/categories). `element_id` for
  `set_limit` is an envelope (or standalone category/tag) id read from
  `get_budget`'s structure.
- **Envelope archive.** `update-envelope` is a full replace including
  `isArchived`; `update_envelope` exposes `archived` (bool) alongside the other
  fields. To archive, the model re-sends the envelope's current fields (from
  `get_budget`) with `archived: true`. No dedicated archive shortcut in v1.
- **`set_limit`.** `month` (YYYY-MM) → `month + "-01"` → the service snaps to
  first-of-month; the month must be ≥ the budget's start month (else a domain
  validation error). Omitted/empty `amount` → nil `*vo.FlexString` (clears the
  limit). Use `vo.NewFlexString` for a present amount.
- **Account inclusion.** One `included` bool dispatches to
  `IncludeAccount`/`ExcludeAccount` (mirrors `set_*_archived`).

## Error handling

Unchanged: `webmcp.MapErr` renders domain failures (bad/foreign currency,
category, account, or element id; name length; out-of-range limit month) as the
localized `{message, messageCode, errors, errorCodes}` envelope; infra stays
`{"message":"Internal error"}`. `set_limit` and the account toggle build small
confirmation structs on success (their service results are empty).

## Testing

- **Feature mcp tests** (`internal/budget/mcp/mcp_test.go`, extending it): a
  full build-a-budget flow over the in-memory SDK session — create_budget →
  create_folder → create_envelope (in the folder, grouping a seeded category) →
  set_limit on it → set_budget_account_included — asserting the result via
  `get_budget`/`list_budgets`. One domain-error path (e.g. `set_limit` with a
  month before the budget start, or an unknown element id → `isError` with the
  localized envelope). Reuse the file's existing session/harness helpers; the
  budget service is already constructed there for the `get_budget` tests.
- **mcpparity**: extend the catalogue with the build-a-budget flow (+ one error
  path), regenerate goldens (`tools/list` +8), inspect (created ids normalized),
  run twice plain and under `-tags enginecompare`.
- **Guards**: `tools/list` golden +8 (schema-drift tripwire); archtest green.
- Full `make go-test` (coverage gate 78).

## Docs

- README MCP tools table: 22 → 30 (add the 8 rows).
- CLAUDE.md: MCP endpoint section, if it enumerates the surface.

## Out of scope (future)

- Budget sharing (grant/accept/revoke/decline access) — own sub-project.
- delete/reset budget/folder/envelope; reorder (order-folder, move-element);
  change-element-currency; a dedicated envelope-archive shortcut.
- Sub-project 3: transaction richer-filters + bulk-update.
