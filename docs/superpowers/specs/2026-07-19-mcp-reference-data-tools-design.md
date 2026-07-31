# MCP reference-data management tools (categories / tags / payees) — Design

Date: 2026-07-19
Status: approved (brainstormed with the maintainer)

Sub-project 1 of a three-part MCP write-surface expansion. The other two —
budget create + configure, and transaction richer-filters + bulk-update — get
their own specs later. This one is the smallest and most uniform, and
establishes the write-tool pattern the others reuse.

## Goal

Let an MCP client create, rename, and archive/unarchive the user's categories,
tags, and payees — enough to "clean up" messy reference-data lists — without
adding hard-delete or any new backend capability.

## Decisions (from brainstorming)

- **No hard delete.** Create / update / archive only. Archive is reversible and
  already hides unused items; delete is irreversible (and can cascade or
  reassign transactions), so it stays out of the MCP surface.
- **No merge, no reorder.** Merge has no REST endpoint (would be a backend
  project); reorder is low value for an LLM. Both out of scope here.
- **Nine tools**, three per domain: `create_*`, `update_*`, `set_*_archived`.
  Total MCP surface 13 → 22.
- **Caller-owned only.** The tools omit the REST `accountId` ownership-reassign
  option (advanced shared-account case, low MCP value); new items are owned by
  the caller.

## Architecture

No new packages or infrastructure. The write tools are registered in the
existing `internal/{category,tag,payee}/mcp/mcp.go` `Register` funcs (which
today register only the `list_*` read tools). Each tool is a thin handler over
the feature's existing `Service` write method, following the established MCP
tool shape exactly (as in `internal/transaction/mcp`):

- `reqctx.AddLogAttr(ctx, "tool", "<name>")`
- `userID, err := webmcp.UserID(ctx)`
- call the service method with a `model.*Request`
- `return nil, *res, webmcp.MapErr(ctx, err)` on the result/error

No cross-feature imports (each tool calls only its own feature's service), so
`archtest` stays green.

## The nine tools

| Tool | Service method | Input | Output DTO |
|---|---|---|---|
| `create_category` | `CreateCategory` | `name`, `type` (`expense`\|`income`), `icon?` | `CreateCategoryResult` |
| `update_category` | `UpdateCategory` | `id`, `name`, `icon` | `UpdateCategoryResult` |
| `set_category_archived` | `ArchiveCategory` / `UnarchiveCategory` | `id`, `archived` (bool) | `{id, archived}` confirmation |
| `create_tag` | `CreateTag` | `name` | `CreateTagResult` |
| `update_tag` | `UpdateTag` | `id`, `name` | `UpdateTagResult` |
| `set_tag_archived` | `ArchiveTag` / `UnarchiveTag` | `id`, `archived` | `{id, archived}` confirmation |
| `create_payee` | `CreatePayee` | `name` | `CreatePayeeResult` |
| `update_payee` | `UpdatePayee` | `id`, `name` | `UpdatePayeeResult` |
| `set_payee_archived` | `ArchivePayee` / `UnarchivePayee` | `id`, `archived` | `{id, archived}` confirmation |

Notes:
- `type` is create-only for categories (the REST `UpdateCategoryRequest` has no
  `type` field — type is immutable after creation); `icon` is settable on both
  create and update. Tags/payees carry only a name.
- `set_*_archived` takes a boolean and dispatches to the archive or unarchive
  service method — one tool instead of two, keeping the surface tight. Both
  underlying service results are empty structs, and a typed MCP handler needs a
  single `Out` type, so the tool returns a small handler-built `{id, archived}`
  confirmation (not a REST DTO) on success — enough for the model to confirm the
  new state.
- Tool descriptions point the model at the sibling `list_*` tool for ids, and
  state the archive-not-delete policy (e.g. "hide an unused category — use
  set_category_archived rather than deleting").

## Idempotency

All three `create-*` service methods claim `req.Id` as an operation id in
`operation_requests_ids` (the entity id is minted internally). The MCP `create_*`
tools mint that operation id server-side with `vo.NewId().String()`, exactly as
`create_transaction` does — MCP callers never supply or see ids, so a model
can't collide or replay. `update_*` and `set_*_archived` take the target id as
input (obtained from `list_*`).

## Error handling

Unchanged from the current MCP edge: tool errors go through `webmcp.MapErr`, so
domain failures (name length `"…must be 3-64 characters"`, bad `type`, name
collision, unknown/foreign id) surface as the localized JSON envelope
(`message` + `messageCode` + `errors`/`errorCodes`), rendered in the caller's
language. Infrastructure errors stay the sanitized `{"message":"Internal
error"}`. Nothing leaks.

## Testing

- **Feature mcp unit tests** (`internal/{category,tag,payee}/mcp/mcp_test.go`,
  extending the existing files): drive each new tool over the in-memory SDK
  session (`mcptest.CtxWithUser`) and assert state round-trips — create then
  confirm via `list_*`; `set_*_archived(true)` then confirm `isArchived: 1`;
  update then confirm the new name. One domain-error case per domain (e.g. a
  too-short name → `isError` with the localized envelope).
- **mcpparity golden scenarios**: a create → update → set-archived flow per
  domain plus one domain-error path, freezing the new tools' JSON-RPC contract.
  Regenerate goldens, inspect, run twice plain, and under `-tags enginecompare`
  for SQLite/PostgreSQL byte-parity.
- **Guards**: the `tools/list` golden grows by 9 (schema-drift tripwire);
  archtest stays green.
- Full `make go-test` (coverage gate 78).

## Docs

- README MCP tools table: 13 → 22 tools (add the create/update/set-archived
  rows; note archive-not-delete).
- CLAUDE.md: note the reference-data write tools under the MCP endpoint section
  if it enumerates the surface.
- Prompts: no change (the existing `log-expense`/`budget-review` don't reference
  these; a future prompt could offer a "tidy up my categories" flow — out of
  scope here).

## Out of scope (future sub-projects / follow-ups)

- Budget create + configure (sub-project 2).
- Transaction richer-filters + bulk-update (sub-project 3).
- delete / merge / reorder for reference data; shared-account ownership on
  create; a "clean-up" prompt.
