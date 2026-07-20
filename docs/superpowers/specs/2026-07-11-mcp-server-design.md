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
  custom connectors (OAuth-only) are out of scope for now. A read-only caller
  (lapsed trial or manual restriction) loses the endpoint **entirely** — every
  tool, reads included — because the 402 rule matches `POST` and every JSON-RPC
  call is one; see the enforcement section of
  `2026-07-19-cloud-monetization-trial-access-design.md` and the guard in
  `internal/test/mcpparity/readonly_test.go`.
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
  helpers (auth context, error mapping).
- `internal/<feature>/mcp/` — per-feature tool/prompt registration
  for: `account`, `budget`, `category`, `connection`, `currency`, `payee`,
  `tag`, `transaction`, `user`. Each imports only its own feature's `Service`
  methods, in-process — no loopback HTTP, no cross-feature imports, so
  `archtest` stays green without exemptions. Connected users live on their own
  `list_connections` tool, owned by `connection/mcp` — no cross-feature port
  is needed for it; `user/mcp` stays profile-only.
- `internal/server/server.go` — composition root composes the per-feature
  registrations and mounts the endpoint, exactly like it composes
  `RegisterAPI` today. Cross-feature needs (if any) get `glue_*.go` adapters
  as usual.

### Capability surface

**Resources — removed (2026-07-15).** The original design shipped eight
`econumo://` read-only resources alongside the tools below. They were dropped
before merge: resources are application-controlled and Claude Desktop (and
most other clients that matter here) won't autonomously read them — the user
has to attach each one manually — while every dataset they exposed is already
served by a `list_*`/`get_user` tool. Tools and prompts are now the whole MCP
surface. The resource registrations are restorable from git history
(`internal/web/mcp/helpers.go`'s `AddJSONResource` helper and the per-feature
`webmcp.AddJSONResource(...)` calls) if a resource-native client need arises.

**Tools** — thirteen:

1. `get_budget` — inputs `budget_id` (UUID), `month` (`YYYY-MM`). Returns the
   full monthly budget state (folders/envelopes/categories/tags, limits,
   spent, available) via the budget read service.
2. `list_transactions` — optional inputs `account_id` (UUID),
   `period_start`, `period_end` (datetimes). The filters compose: an
   account, a full period window, or both together; a lone period bound is
   rejected at the MCP edge. Returns transaction items.
3. `create_transaction` — inputs mirror `CreateTransactionRequest` minus
   `id`: `type` (`expense`/`income`/`transfer`), `amount` (decimal string),
   `account_id`, `date`, optional `category_id` (required for non-transfers),
   `account_recipient_id` + `amount_recipient` (transfers), `description`,
   `payee_id`, `tag_id`. The handler mints the UUIDv7 transaction id
   server-side (the client-supplied-id idempotency guard stays internal to
   the REST edge).
4. `update_transaction` — same fields plus required `id`.
5. `delete_transaction` — input `id`.
6. `list_accounts`, `list_categories`, `list_tags`, `list_payees`,
   `list_currencies`, `list_budgets`, `list_connections`, `get_user` — no
   input; each returns the corresponding read-service result, via the same
   service call as the equivalent REST list/get endpoint. This is a
   model-driven client's only way to discover account, category, payee, tag,
   currency, budget, connection or user ids. The `list_*` naming (rather than
   e.g. `get_accounts`) anticipates future per-object write tools
   (`list_accounts` → `create_account` → ...), matching the existing
   `list_transactions`/`create_transaction`/... family.

Tool input/output schemas are generated by the SDK from typed Go structs; the
eight `list_*`/`get_user` tools use an empty input struct (the SDK requires a
struct type for schema inference even when a tool takes no arguments); output
schemas are the frozen REST shapes (or, for `list_currencies`, an ad-hoc
wrapper struct).

**Prompts** — four:

- `log-expense` — argument `description` (free text, e.g. "27.50 groceries at
  Lidl yesterday, card"). Template: call `list_accounts`, `list_categories`,
  `list_payees`; parse amount/date/payee; call `create_transaction`; echo
  back what was logged for confirmation.
- `budget-review` — optional argument `month` (default: current month).
  Template: call `list_budgets`, call `get_budget`, compare limits vs spent,
  flag overspent/at-risk envelopes, sample notable transactions via
  `list_transactions`, produce a short structured review in the user's
  language.
- `budget-setup` — optional argument `name`. Builds a budget from the
  categories and tags the user already has: survey them, total the last 2-3
  months via `list_transactions` so limits reflect real spending, then split
  expense categories into two folders — **Base expenses** (the ones you cannot
  live without: housing, utilities, groceries, commuting, insurance,
  healthcare, debt, childcare, essential subscriptions) and **Additional
  expenses** (everything else). Related categories are grouped into envelopes
  under those folders, and limits are set per envelope. The split is proposed
  and confirmed before anything is created; ambiguous categories default to
  ADDITIONAL and are flagged rather than silently promoted.
- `budget-update` — optional argument `month` (default: current month). The
  reconcile counterpart: find categories/tags in no envelope, missing or stale
  limits, and categories whose real usage no longer matches their group;
  classify anything new under the same base/additional rule; propose the diff
  and apply it on approval. Explicitly warns that `update_envelope` replaces an
  envelope's *full* category set, so existing ids must be resent alongside new
  ones, and that retiring an envelope means archiving it (there is no delete).

The two structure prompts share one vocabulary — base vs additional — so a user
can run setup once and reconcile with update thereafter without the model
reorganizing a layout that already works.

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

Two layers, mapped deliberately. Every tool error is `isError: true` whose
text is a JSON object mirroring the client-facing fields of the REST error
envelope, so a model receives the same signal the web SPA does — with one
deliberate divergence: **message text is localized server-side** to the
caller's language, whereas REST always returns frozen English and leaves
translation to the SPA (MCP clients are LLMs with no catalogue of their own).
The resolution chain mirrors the stored-timezone fallback above: explicit
`Accept-Language` header → stored `users.language` → `en`
(`languageFallback`, `internal/server/glue_language.go`, installed on `/mcp`
next to `timezoneFallback`). Codes are unaffected — they stay for machine use
regardless of language:

```json
{ "message": "<message translated to the caller's language>", "messageCode": "<catalogue code>" }
// field validations additionally carry the envelope's per-field maps:
{ "message": "Form validation error",
  "errors":     { "name": ["<translated per-field message>"] },
  "errorCodes": { "name": [{ "code": "category.name_length",
                             "params": { "min": 3, "max": 64 } }] } }
```

- **Domain errors** (the `errs` taxonomy: validation, not-found, access
  denied) → the envelope-mirroring payload above (`message` + `messageCode`/
  `messageParams`, plus `errors`/`errorCodes` for field validations). Any
  error/field that carries a catalogue code renders `message` via
  `i18n.T(lang, "errors."+code, params)` (`internal/web/mcp/helpers.go`,
  `MapErr`); errors/fields with no code (nothing in the catalogue to look up,
  e.g. the MCP-internal "month must be YYYY-MM") keep their literal Go-side
  text unchanged, and not-found/access-denied errors — which carry only a
  message today, no code — are likewise passed through as-is. The model can
  self-correct: a bad month string, and — since the service-level
  reference-authorization added in the write-endpoint IDOR hardening — an
  unknown or foreign reference id (category/payee/tag/recipient account) now
  returns a proper domain validation error with its code
  (`transaction.item_not_available` / `transaction.account_not_available`)
  rather than an opaque failure.
- **Infrastructure errors** (DB failure, panics) → the same payload shape with
  a static `{"message":"Internal error"}` and no code — the SDK's typed
  handlers can't emit JSON-RPC errors directly, so infra failures map to the
  same `isError: true` shape but nothing leaks. `ECONUMO_DEBUG` does NOT add
  stack traces to MCP responses; the underlying error is logged at ERROR.

## Logging

Same two-tier slog discipline as the REST edge:

- Operation-result line per request; the operation message is the static
  string `mcp` (the access-log middleware derives it from the last path
  segment of `/mcp`, same as every REST route), enriched via
  `reqctx.AddLogAttr` with a `tool` / `prompt` attr naming the specific tool
  call or prompt get.
- Tool **arguments are never logged** (amounts, payee names, free text — PII
  by this repo's standard). UUIDs only, as everywhere.
- Status mapping differs from REST: every tool call rides HTTP 200 (the
  `isError` distinction lives inside the JSON-RPC body, not the status code),
  so the per-request operation line is always INFO, whether or not the tool
  result is an error. Infrastructure errors additionally emit a separate
  ERROR line at the point `MapErr` logs and replaces them.

## Testing

- Unit tests per tool/prompt handler against sqlite `dbtest` +
  `fixture`, auth via `authstub` — like feature `api` tests.
- A dedicated golden-file JSON-RPC scenario suite (pattern copied from
  `internal/test/apiparity`, but separate — `/mcp` is outside `/api`, so the
  REST parity machinery does not scan it): `initialize`, `tools/list`,
  `prompts/list`, each tool happy-path + one domain-error path, each prompt
  get — replayed against the real `server.BuildAPI` handler, normalized like
  the REST goldens (UUIDs, datetimes). Runs in the smoke tier (`make go-test`).
- The same scenarios run under `-tags enginecompare` so SQLite and PostgreSQL
  stay byte-identical over `/mcp` too.
- Committed `tools/list` / `prompts/list` goldens double as a schema-drift
  guard: any accidental tool-schema change shows up in
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
