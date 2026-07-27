# MCP transaction review tools (richer filters + bulk update) — Design

Date: 2026-07-19
Status: approved (brainstormed with the maintainer)

Sub-project 3 (final) of the MCP write-surface expansion. Sub-projects 1
(category/tag/payee management) and 2 (budget create + configure) are on this
branch. This one adds transaction *review* capability: richer read filters to
find the right transactions, and a bulk-classification update to fix many at
once. Unlike 1 and 2, it needs NEW backend capability (filter fields + a bulk
operation), not just MCP-wrapping.

## Decisions (from brainstorming)

- **Filters: uncategorized + category/payee/tag.** Simple equality/`IS NULL`
  predicates. (No amount range, no description text search — deliberately
  scoped out.)
- **Bulk update: explicit id list, classification only.** The model passes the
  exact transaction ids it chose (from `list_transactions`) and the
  category/payee/tag to set or clear. Never amount/date/account. No filter-based
  bulk (avoids blind mass-writes).
- **MCP-only.** The new filter params and the bulk operation live in the
  transaction feature but only the MCP tools call them. No new/changed REST
  routes, OpenAPI, or apiparity goldens. The web app has no bulk-edit UI.
- **Cap 100 ids/call, all-or-nothing.**
- One new tool (`bulk_update_transactions`) → 31 total; `list_transactions`
  gains filter inputs.

## Part 1 — Richer read filters

`model.TransactionListRequest` gains four optional fields:

```go
Uncategorized bool   `json:"uncategorized,omitempty"`
CategoryId    string `json:"categoryId,omitempty"`
PayeeId       string `json:"payeeId,omitempty"`
TagId         string `json:"tagId,omitempty"`
```

- Applied in `GetTransactionList` (`internal/transaction/read.go`) and pushed
  into the repo's dynamic-SQL WHERE builder (`ListByAccountIDs` in
  `internal/transaction/repo/repo.go`). All filters compose with account/period
  (AND).
- **REST invariant (critical):** with none of the new fields set, output MUST
  be byte-identical to today — existing `get-transaction-list` callers send
  nothing, so REST behavior is unchanged and apiparity/enginecompare goldens do
  not move. The implementer routes the no-filter path exactly as today
  (`ListByAccount` for single-account-no-period stays untouched when no
  classification filter is present); the filter predicates are only appended
  when a filter is actually supplied.
- `uncategorized: true` XOR a specific `category_id`: if both are set, return a
  validation error (they contradict). `category_id`/`payee_id`/`tag_id` are exact
  matches; `uncategorized` is `category_id IS NULL`.
- Validation: the id fields, when present, must be valid UUIDs (reuse the
  existing UUID validation shape).
- `list_transactions` (`internal/transaction/mcp/mcp.go`) exposes the four as
  optional inputs alongside `account_id`/`period_start`/`period_end`.

## Part 2 — `bulk_update_transactions`

New MCP-only service operation + tool.

**Service:** `func (s *Service) BulkUpdateTransactions(ctx, userID vo.Id, req model.BulkUpdateTransactionsRequest) (*model.BulkUpdateTransactionsResult, error)` in a new `internal/transaction/bulk.go`.

```go
type BulkUpdateTransactionsRequest struct {
    Ids          []string `json:"ids"`
    CategoryId   *string  `json:"categoryId"`   // set to this id
    PayeeId      *string  `json:"payeeId"`
    TagId        *string  `json:"tagId"`
    ClearCategory bool    `json:"clearCategory"` // set to NULL
    ClearPayee    bool    `json:"clearPayee"`
    ClearTag      bool    `json:"clearTag"`
}
type BulkUpdateTransactionsResult struct { Updated int `json:"updated"` }
```

Semantics:
- At least one change (a non-nil set-id OR a clear flag) required, else a
  validation error. Providing both a set-id and the clear flag for the SAME
  field is rejected.
- Cap: `len(Ids) > 100` → validation error ("at most 100 transactions per bulk
  update; batch the rest"). Empty `Ids` → validation error.
- **All-or-nothing** inside one `TxManager.WithTx`: for every id, load the
  transaction and enforce the SAME checks single-update does — write access on
  its account (`checkWriteAccess`) and, for a newly-set category/payee/tag, that
  the reference is owned by the caller (`checkReferences`, or the equivalent
  per-field owned-entity check). If ANY id is not found, not write-accessible,
  or references a foreign/unknown category/payee/tag, the whole call fails with
  a clear domain error and nothing is written.
- Only category/payee/tag columns change; amount/date/account/type are never
  touched.
- Output: `{updated: <len(Ids)>}` on success.

**Implementation — reuse the validated per-transaction update path.** Bulk must
NOT be a raw `UPDATE … SET category_id=… WHERE id IN (…)`: that would bypass the
single-update invariants and create invalid states (e.g. a category on a
transfer, which `update-transaction` forbids). Instead, for each id inside one
`TxManager.WithTx`, load the transaction and apply only the requested
classification change through the SAME validated update logic the single
`UpdateTransaction` uses (which already runs `checkWriteAccess`,
`checkReferences`, and the type/category invariants) — so a transfer rejecting a
category, a foreign reference, or an inaccessible account all surface naturally
and roll the whole call back. A batch of 100 validated updates in one tx is
acceptable (MCP is not a high-QPS path).

**Tool** (`internal/transaction/mcp/mcp.go`): inputs mirror the request (ids +
set/clear per field); output the `{updated}` result. Description: "Re-classify
many transactions at once (set or clear category/payee/tag on an explicit list
of transaction ids from list_transactions); amounts/dates/accounts are never
changed; max 100 per call."

## Error handling

Unchanged edge: `webmcp.MapErr` renders all the above domain errors (bad UUID,
contradictory filter, empty/oversized id list, no-change, access denied, foreign
reference) as the localized envelope; infra stays `{"message":"Internal
error"}`. All-or-nothing means an error leaves the data untouched.

## Testing

- **Service/repo unit tests** (`internal/transaction/...`): each new filter
  (uncategorized, category, payee, tag, and combined with account/period);
  byte-identical no-filter behavior; bulk update happy path; bulk rejections
  (empty ids, >100, no-change, set+clear same field, access-denied id, foreign
  category id) each leave data unchanged (all-or-nothing). Run the repo tier on
  BOTH engines (`DBTEST_ENGINE=pgsql`) — the dynamic SQL and bulk update must
  work on Postgres.
- **MCP feature tests** (`internal/transaction/mcp/mcp_test.go`): filtered
  `list_transactions` (uncategorized, by category) returns the right subset;
  `bulk_update_transactions` sets a category on two ids then `list_transactions`
  confirms; one error path (isError + localized envelope, no leak).
- **mcpparity**: extend the catalogue — a filtered list + a bulk update + an
  error path; regenerate goldens (`tools/list` +1 new tool, `list_transactions`
  schema grows), inspect, run twice plain and under `-tags enginecompare`.
- **REST guard:** apiparity goldens MUST stay byte-identical (no REST change);
  confirm by running apiparity WITHOUT `UPDATE_GOLDEN`.
- Full `make go-test` (coverage gate 78).

## Docs

- README MCP tools table: add `bulk_update_transactions`; note `list_transactions`
  now filters by uncategorized/category/payee/tag. 30 → 31 tools.
- CLAUDE.md: only if it enumerates the MCP surface.

## Out of scope (future)

- Amount-range / description-text filters; filter-based bulk update; bulk edit
  of amount/date/account; a REST bulk endpoint; budget sharing (from
  sub-project 2's deferrals).
