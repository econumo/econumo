# MCP Server for Econumo — Design

Date: 2026-07-11
Status: approved (brainstormed with the maintainer; see decisions below)

## Goal

Let any MCP client (Claude Code, Claude Desktop, Cursor, …) talk to a
self-hosted Econumo instance over the network: read accounts, budgets, and
transactions, and log/edit/delete transactions — "log this expense",
"how is my budget doing this month".

## Decisions (made during brainstorming)

- **Remote-first**: the Econumo binary itself speaks MCP over Streamable HTTP.
  No separate binary, no stdio mode, no new port.
- **Scope**: reads + transaction writes only. No account/budget/structure
  mutation via MCP in v1.
- **Auth**: existing opaque bearer tokens (PATs `eco_pat_*` are the intended
  credential; session tokens also pass). No OAuth in v1, so claude.ai web
  custom connectors (OAuth-only) are out of scope for now.
- **Implementation**: the official Go SDK
  (`github.com/modelcontextprotocol/go-sdk`, v1.x) — the MCP protocol
  (JSON-RPC lifecycle, capability negotiation, transport) is exactly the
  "stdlib can't reasonably deliver" territory the dependency policy carves
  out, and the SDK absorbs spec churn.
- **Stateless, JSON-response mode** (no SSE, no server-held sessions):
  every tool is a sub-second DB read/write with nothing to stream, and v1 has
  no server-initiated messages. Stateless mode is restart-safe, needs no
  session affinity behind a reverse proxy, and gives byte-deterministic
  responses for golden-file tests. Spec-legal (clients negotiate it
  transparently); SSE/subscriptions can be enabled later without breaking
  clients.
- **Mount point**: `/mcp` at the root (not under `/api`).
- **Stored user timezone** (folded in, minimal version): a `users.timezone`
  column, opportunistically persisted from the `X-Timezone` header the SPA
  already sends, used as the fallback timezone for MCP requests (which carry
  no header). No new endpoint, no frontend change.

## Architecture

### Mounting

`/mcp` is registered on the **root** mux in `internal/web/router` (like
`/health`), wrapped in the same global chain (requestid → accesslog → recover
→ cors → timezone) plus `middleware.Auth` and the stored-timezone fallback
(below). The SPA catch-all does not shadow it (exact patterns win over `/`).
Unauthenticated requests get the standard 401 JSON envelope before the MCP
layer runs; spec-conformant clients surface that as "credentials needed".

The SDK's Streamable HTTP handler is a plain `http.Handler` configured
stateless + JSON responses. Server identity: name `econumo`, version from
build info.

### Package layout (mirrors the two-edge pattern: `api/` = REST, `mcp/` = MCP)

- `internal/web/mcp/` — shared MCP edge infrastructure, no feature logic:
  builds the `mcp.Server`, mounts the Streamable HTTP handler, defines the
  registration seam (analogous to `router.RegisterAPI` — a
  `RegisterTools func(*mcp.Server)` type plus a `Compose`), and shared
  helpers (error mapping, JSON resource marshalling).
- `internal/<feature>/mcp/` — per-feature tool/resource/prompt registration
  for: `account`, `budget`, `category`, `currency`, `payee`, `tag`,
  `transaction`, `user`. Each imports only its own feature's `Service`
  methods, in-process — no loopback HTTP, no cross-feature imports, so
  `archtest` stays green without exemptions. The one cross-feature need —
  connected users on `econumo://user` — follows the standard pattern:
  `user/mcp` declares a small consumer port, `internal/server` wires a
  `glue_*.go` adapter over the `connection` feature's public API.
- `internal/server/server.go` — composition root composes the per-feature
  registrations and mounts the endpoint, exactly like it composes
  `RegisterAPI` today. Cross-feature needs (if any) get `glue_*.go` adapters
  as usual.

### Capability surface

**Resources** — read-only reference data, URI scheme `econumo://`, all JSON,
scoped to the authenticated user:

| URI | Content |
|---|---|
| `econumo://accounts` | the same per-account fields the REST account list returns (id, name, type, currency, archived flag, owner/shared-access info) plus current balance ("as of end of today" in the resolved timezone — see below) |
| `econumo://categories` | id, name, type (`expense`/`income`), archived flag |
| `econumo://tags` | id, name, archived flag |
| `econumo://payees` | id, name, archived flag |
| `econumo://currencies` | currency codes + current rates vs the instance base currency |
| `econumo://budgets` | the user's budgets: id, name, currency (monthly state is parameterized → tool) |
| `econumo://user` | current user profile (id, name, avatar, base currency) + connected users (shared-access context) |

No resource subscriptions in v1 (that is the SSE-later case); clients re-read
cheaply. Unknown URI → resource-not-found JSON-RPC error.

**Tools** — five:

1. `get_budget` — inputs `budget_id` (UUID), `month` (`YYYY-MM`). Returns the
   full monthly budget state (folders/envelopes/categories/tags, limits,
   spent, available) via the budget read service.
2. `list_transactions` — optional inputs `account_id` (UUID),
   `period_start`, `period_end` (datetimes) — mirrors the existing
   `TransactionListRequest` filter surface exactly. Returns transaction items.
3. `create_transaction` — inputs mirror `CreateTransactionRequest` minus
   `id`: `type` (`expense`/`income`/`transfer`), `amount` (decimal string),
   `account_id`, `date`, optional `category_id` (required for non-transfers),
   `account_recipient_id` + `amount_recipient` (transfers), `description`,
   `payee_id`, `tag_id`. The handler mints the UUIDv7 transaction id
   server-side (the client-supplied-id idempotency guard stays internal to
   the REST edge).
4. `update_transaction` — same fields plus required `id`.
5. `delete_transaction` — input `id`.

Tool input/output schemas are generated by the SDK from typed Go structs.
Reads that a dropped tool would have covered (account balances) live on the
`econumo://accounts` resource instead.

**Prompts** — two:

- `log-expense` — argument `description` (free text, e.g. "27.50 groceries at
  Lidl yesterday, card"). Template: read `econumo://accounts`,
  `econumo://categories`, `econumo://payees`; parse amount/date/payee; call
  `create_transaction`; echo back what was logged for confirmation.
- `budget-review` — optional argument `month` (default: current month).
  Template: read `econumo://budgets`, call `get_budget`, compare limits vs
  spent, flag overspent/at-risk envelopes, sample notable transactions via
  `list_transactions`, produce a short structured review in the user's
  language.

## Stored user timezone (minimal version)

Account-balance day-boundary math uses the caller's timezone; the SPA sends
`X-Timezone` per request, MCP clients send nothing.

- **Schema**: migration adding `users.timezone TEXT NOT NULL DEFAULT ''`
  (both engines). Not exposed in any API response (no golden churn).
- **Write path (opportunistic persist)**: auth is applied per-handler inside
  each feature's `RegisterAPI`, so there is no single "authenticated group"
  middleware seam. Instead, a decorator around `middleware.TokenAuthenticator`
  (wired once in `server.BuildAPI`, zero signature changes elsewhere) runs on
  every authenticated request: it reads the request timezone that the
  `Timezone` middleware already resolved into the context, and — only when
  the header was explicitly present (a small `reqctx` addition distinguishes
  "explicit header" from the UTC default) and differs from the last value
  seen for that user — calls a user-service method doing one idempotent
  `UPDATE users SET timezone = ? WHERE id = ? AND timezone <> ?`.
  An in-memory per-user last-seen cache (same in-process state pattern as the
  auth rate limiter and the token `last_used_at` throttle) keeps this at ~one
  write per user per boot/change. Persist failures are logged and swallowed —
  they must never fail the request.
- **Read path (fallback)**: on `/mcp` only, after auth, when the request
  carries no `X-Timezone` header, load the stored timezone and install it
  via `reqctx.WithLocation`. Resolution chain: `X-Timezone` header → stored
  `users.timezone` → UTC. REST behavior is unchanged (header always wins).

## Error handling

Two layers, mapped deliberately:

- **Domain errors** (the `errs` taxonomy: validation, not-found, access
  denied) → MCP **tool errors** (`isError: true`) carrying the same
  human-readable message the REST envelope would; the model can self-correct
  (e.g. re-read `econumo://categories` after a bad category id).
- **Infrastructure errors** (DB failure, panics) → a generic `"Internal
  error"` **tool error**, not a JSON-RPC error response: the SDK's typed
  handlers can't emit JSON-RPC errors directly, so infra failures are mapped
  to the same `isError: true` shape as domain errors but with a static
  message — nothing leaks. `ECONUMO_DEBUG` does NOT add stack traces to MCP
  responses; the underlying error is logged at ERROR instead.

## Logging

Same two-tier slog discipline as the REST edge:

- Operation-result line per request; the operation message is the static
  string `mcp` (the access-log middleware derives it from the last path
  segment of `/mcp`, same as every REST route), enriched via
  `reqctx.AddLogAttr` with a `tool` / `resource` / `prompt` attr naming the
  specific tool call, resource read, or prompt get.
- Tool **arguments are never logged** (amounts, payee names, free text — PII
  by this repo's standard). UUIDs only, as everywhere.
- Status mapping as usual: domain tool errors WARN, infra errors ERROR.

## Testing

- Unit tests per tool/resource/prompt handler against sqlite `dbtest` +
  `fixture`, auth via `authstub` — like feature `api` tests.
- A dedicated golden-file JSON-RPC scenario suite (pattern copied from
  `internal/test/apiparity`, but separate — `/mcp` is outside `/api`, so the
  REST parity machinery does not scan it): `initialize`, `tools/list`,
  `resources/list`, `prompts/list`, each tool happy-path + one domain-error
  path, each resource read, each prompt get — replayed against the real
  `server.BuildAPI` handler, normalized like the REST goldens (UUIDs,
  datetimes). Runs in the smoke tier (`make go-test`).
- The same scenarios run under `-tags enginecompare` so SQLite and PostgreSQL
  stay byte-identical over `/mcp` too.
- Committed `tools/list` / `resources/list` / `prompts/list` goldens double
  as a schema-drift guard: any accidental tool-schema change shows up in
  review.
- Stored-timezone: unit tests for the persist middleware (valid/invalid/
  missing header, throttle cache) and the `/mcp` fallback chain; migration
  covered by both engines' migration runs.

## Out of scope / future work

- OAuth 2.1 authorization server (needed for claude.ai web custom
  connectors) — a feature of its own; the PAT design does not block it.
- SSE streaming / resource subscriptions / `listChanged` notifications.
- `ECONUMO_MCP_ENABLED` kill-switch env var (YAGNI — the endpoint grants
  nothing a token holder couldn't already do via REST).
- An explicit user-facing timezone setting (profile UI + endpoint); the
  stored column is forward-compatible with it.
- Richer transaction search filters and a `get_spending_summary` tool — the
  model can derive summaries from `get_budget` + `list_transactions`.
- Docs: README section with client setup snippets (Claude Code / Claude
  Desktop / Cursor static-header config) ships with the implementation.
