# Repository Guidelines

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

Econumo is a self-hosted personal finance and budgeting application. It consists of:
- **Backend**: Go (HTTP API + static SPA server) with hexagonal architecture (the Go module is the repo root).
- **Frontend**: React 19 + Vite SPA with TypeScript (Tailwind 4 / shadcn, TanStack Query, Zustand), in `web/`.
- **Database**: SQLite (default) or PostgreSQL — selected at runtime by `DATABASE_URL`.

> History: the backend was originally a Symfony 5.4 (PHP) app. It has been fully
> replaced by the Go backend and the PHP code has been removed. A single "cloud"
> edition is shipped (there is no separate "ce" edition).

The production artifact is a single self-contained Go binary in a distroless image
(`ghcr.io/econumo/econumo`) that serves both the JSON API and the built SPA (embedded
via `go:embed` — `web/embed.go`) and runs database migrations on boot. Releases
additionally attach standalone `econumo-linux-{amd64,arm64}` binaries + `SHA256SUMS`
for Docker-free hosting.

## Development Commands

### Go backend

The Go module is the repo root. Tests run with the standard toolchain — no Docker
required for the smoke tier.

```bash
make go-test         # SMOKE: build + vet + gofmt + OpenAPI-docs-fresh + sqlite unit/integration + coverage gate
make test            # FULL: go-test + the sqlite-vs-PostgreSQL engine-comparison suite + the release
                     #       upgrade suite + the frontend suite
make test-upgrade    # upgrade databases written by published release binaries to this checkout
make go-build        # Compile the binary to ./econumo (regenerates OpenAPI docs first)
make go-run          # Run the server locally (reads .env; regenerates OpenAPI docs first)
make go-lint         # build + vet + gofmt + OpenAPI-docs-fresh check
make swagger         # Regenerate the committed OpenAPI docs (swag, pinned to go.mod)

# Or directly with the go toolchain (run from the repo root):
go test ./...                        # all tests
go build ./...                        # build everything
go run ./cmd/econumo serve            # run the server (reads .env)
go run ./cmd/econumo user:create "Name" user@example.test secret
```

The full suite needs a PostgreSQL; `make test` auto-provisions one via the
compose stack, or set `DATABASE_TEST_PGSQL_URL` to an existing database.

### Frontend (React/Vite) — in `web/`

```bash
make web-install   # cd web && pnpm install
make web-run       # cd web && pnpm dev       (dev server, port 9000, proxies /api to :8181)
make web-test      # cd web && pnpm test      (vitest)
make web-lint      # cd web && pnpm lint      (oxlint)
make web-bundle    # cd web && pnpm build     (production SPA build -> web/dist)
```

### Publishing

```bash
make publish-dev   # build + push the multi-arch `dev` image to ghcr.io/econumo/econumo:dev
                   # AND publish the `dev` channel to the private R2 bucket with ship
```

Releases (`latest` + `vX.Y.Z`) are cut by the GitHub release workflow
(`.github/workflows/publish-release.yml`), not locally; its "Publish Dev"
checkbox can also move the `dev` tag. The workflow's `branch` input selects
the source branch (default `main`; a `release/*` branch for hotfixes of older
versions). A release from `main` auto-creates `release/vX.Y.Z` at the released
commit as the base for future hotfixes; a hotfix dispatches from a new
`release/*` branch copied by hand from the fixed version's branch, and the
workflow then creates no branch. `latest` only moves for the highest version
released so far. See `.claude/skills/publish-release` for the full process.
Everything publishes to `ghcr.io/econumo/econumo` only.

**R2 release (ship).** Both `make publish-dev` and the release workflow also
publish the cross-compiled linux binaries (`econumo-linux-{amd64,arm64}`), the
systemd unit template `deploy/ship/econumo.service` and
`deploy/ship/econumo.env.example` to the **private** Cloudflare R2 bucket `ship`
with [ship](https://github.com/dmtrkzntsv/ship). Two Makefile targets, the same
names in every ship project: `make ship-dist` builds (`release-binaries`) and
stages the flat folder `dist/ship/` (ship rejects any other file;
`SHIP_SKIP_BUILD=1` takes already-built binaries from `release-out/`), and
`make ship-publish VERSION=… CHANNELS=…` runs `ship publish econumo --path
dist/ship [--version $(VERSION)] [--channels $(CHANNELS)]` without building.
ship writes its own `SHA256SUMS` and moves the channels. A release publishes the
immutable version `vX.Y.Z` plus `latest`/`dev` only when their checkbox is set
(exactly like the image tags); `VERSION=dev` (`make publish-dev`, the Publish
Dev workflow) publishes only the moving `dev` channel. Hosts run `ship
install|update econumo`. The unit is a TEMPLATE:
only ship's placeholders (`{{SERVICE}} {{USER}} {{BIN}} {{BIN_DIR}}
{{ENV_FILE}} {{DATA_DIR}} {{RUN_DIR}} {{UNIT}}`) are allowed, and it must not
use `DynamicUser=`/`StateDirectory=`/`RuntimeDirectory=` (ship creates the user
and directories). Credentials come from ship's config or `SHIP_R2_ENDPOINT`,
`SHIP_R2_ACCESS_KEY_ID`, `SHIP_R2_SECRET_ACCESS_KEY` (Read & Write token; the
bucket defaults to `ship`); in CI the GitHub secrets `R2_ENDPOINT`,
`R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` are mapped to them and
`dmtrkzntsv/ship@v0` puts `ship` on the PATH. The workflows run both targets
and the unit template from the workflow's own commit (`make -f
.ship-tools/Makefile ship-dist SHIP_SKIP_BUILD=1
SHIP_TEMPLATES=.ship-tools/deploy/ship`, then `ship-publish`), so hotfixes from
release branches cut before ship still publish. Local use needs `ship` installed
(`brew install dmtrkzntsv/tap/ship`).

### Branch naming

- `feature/<slug>` — feature work, and the default for EVERY change that is
  not a bug fix (chores, docs, refactors, CI, dependency bumps).
- `bug/<slug>` — bug fixes.
- `release/vX.Y.Z` — reserved for releases (auto-created when releasing from
  `main`; copied by hand from the previous release branch of the line for
  hotfixes); never used for development work.

## Architecture

### Feature packages (vertical slices)

The backend is organized as vertical feature packages rather than horizontal
layers. Each of the seventeen features (`account`, `admin`, `authserver`, `budget`, `category`, `connection`,
`currency`, `imports`, `label`, `oauth`, `onboarding`, `payee`, `recurring`, `system`, `tag`, `transaction`, `user`) is a single `internal/<feature>`
tree (`internal/version` is a build-info leaf that archtest also auto-detects, not a slice) holding its own use cases, persistence, and HTTP edge; the entities and
DTOs those use cases operate on live in the shared `internal/model` package
(below), so a feature package is behavior-only:

```
. (repo root = the Go module; web/ and deployment/ live alongside)
├── cmd/econumo/main.go ............ binary entrypoint; dispatches serve / healthcheck / resource:action commands
├── internal/
│   ├── shared/ .................... dependency-free kernel: vo (value objects), errs (domain error taxonomy),
│   │                                datetime (frozen wire/persistence layouts),
│   │                                port (Clock/TxRunner/OperationGuard seams), reqctx (request-scoped
│   │                                context values, e.g. caller timezone + log attrs)
│   ├── model/ ...................... the shared type universe: every feature's entities (with their
│   │                                invariant-preserving mutators), value objects, and request/result DTOs,
│   │                                one file per feature (account.go, account_dto.go, ...); imports only
│   │                                the shared kernel; part of the archtest kernel alongside `shared`
│   ├── <feature>/ ................. one package per feature (account, budget, category, connection,
│   │   │                            currency, imports, onboarding, payee, system, tag, transaction, user); root package holds only
│   │   │                            behavior — the entities/DTOs it operates on live in `internal/model`:
│   │   │   <verb>.go .............   one file per use case or a closely related group (create.go,
│   │   │                             update.go, delete.go, read.go, ...), naming a package-level `Service`
│   │   │                             (or `ReadService`/`WriteService`) method per use case
│   │   │   repository.go .........   the repository INTERFACE(s), in `model` types; account and budget
│   │   │                             split theirs into role interfaces + a composite for wiring
│   │   │   ports.go ..............   consumer-side interfaces for capabilities OTHER features provide
│   │   ├── repo/ ..................  repository implementation (engine-adapter pattern, see below)
│   │   ├── api/ ...................  HTTP edge: handlers + route registration (see API handler pattern below)
│   │   └── mcp/ ...................  MCP edge (ten features have one): tool registration, mirroring
│   │                                 api/ (see MCP endpoint below; prompts live in internal/web/mcp)
│   ├── infra/ .................... engine-agnostic infrastructure shared by every feature:
│   │   ├── storage/sqlc/ ......... sqlc config + per-engine queries (query/{sqlite,pgsql}) and generated code (gen/{sqlite,pgsql})
│   │   ├── storage/migrations/ ... SQL migrations per engine ({sqlite,pgsql}); run on boot
│   │   ├── operation/ ............ shared row-based idempotency guard (operation_requests_ids) for
│   │   │                          client-supplied operation ids on create endpoints
│   │   ├── idempotency/ .......... the `Idempotency-Key` rows (idempotency_keys): one per (user, key), see "API conventions"
│   │   ├── auth/ ................ password hashing + AES email encryption
│   │   ├── clock/ ................ time source abstraction
│   │   ├── ai/ .................. OpenAI-compatible / Anthropic Messages client (AI integration); imports nothing internal
│   │   └── mailer/ .............. transactional email; transport from MAILER_DSN (console stdout | Resend API | SMTP)
│   ├── web/ ..................... HTTP-edge infrastructure shared by every feature (the Go server edge —
│   │                              distinct from the repo-root web/, the React SPA): middleware, router,
│   │                              response envelope (httpx), SPA server, apidoc — no feature logic
│   ├── server/ .................. composition root: server.BuildAPI wires every feature (used by the
│   │                              binary AND tests); glue_*.go files hold the cross-feature adapters
│   │                              (see below)
│   ├── cli/ ..................... the resource:action management commands (stdlib dispatch; no cobra)
│   ├── config/ .................. environment configuration loading
│   └── test/ .................... shared test support: dbtest, fixture, testkeys, enginecompare, archtest
```

Not every feature has a `repository.go`/`ports.go` — e.g. `currency` has no
per-user persistence shape (it's rates + conversion + admin lookups), so it
keeps `read.go`/`admin.go`/`convertor.go` but no `repository.go`; `system` is
similar — it's in-memory poller state only (no persistence at all), so it has
no `repository.go` either. `imports` (the bank/phone transaction-import
subsystem, spec in `docs/superpowers/specs/2026-08-15-transaction-import-design.md`)
ships in stages: stage 1 (persistence + matcher core), stage 2 (the Apple
Wallet push provider), stage 3 (the SimpleFIN pull provider), and stage 4
(rule-based classification) are in. The
root package holds no provider-specific code: each provider is a subpackage —
`internal/imports/applewallet` (the push-event parser), `internal/imports/simplefin`
(the bridge client + stored-row parser) — that plugs into the service's
`EventParser` and `Provider` registries in `internal/server/server.go` (a new
provider = one subpackage + one registration line). Rules (stage 4) live in
the root package (`rules.go` matcher, `rule_*.go` use cases). The optional
`Completer` (`internal/infra/ai`, an OpenAI-compatible / Anthropic Messages client,
injected in `internal/server`) drives **rule suggestions** (`suggest.go`, `suggest_job.go`,
`suggestion.go`; spec `docs/superpowers/specs/2026-11-11-ai-import-rule-suggestions-design.md`):
with AI on, after a LIVE import (an Apple Wallet push or a SimpleFIN sync, posted at most 7 days
before import, the import time read as a wall clock in the owner's stored timezone, UTC when
unknown; never the `link-account`/retry/queued-import replays) leaves a non-transfer row
without a category or payee after the account OWNER's rules ran (owner not `readonly`), a
background job (50 rows/request, 60 s timeout per owner started once that owner's lock is held, scope `import-ai-suggest` 30 jobs per rate-limit
window, IP-exempt, one job however many calls) asks in two steps. Step 1, one request per
transaction type, sends the rows (opaque key, bank payee text, income/expense, account name,
currency code; never the amount, the date or the description, and a row with no payee text is
not sent at all) with only that type's non-archived categories and no payees,
and asks per row for a match text, the merchant's common name and, when confident, a category.
The server then picks, without AI, up to 5 of the owner's non-archived payees per row whose
names resemble the merchant name or the match text (`payeeCandidates`: equal, one containing
the other, Levenshtein ratio >= 0.8, or a shared word of 3+ letters; lowercased, punctuation
stripped), and step 2, only when some row has candidates, sends those rows (key, payee text,
merchant name) with their own candidates and keeps a payee only when it is one of that row's (a failed step 2 keeps step 1's
categories); the opt-out is re-read before every call after the first. The suggestion lands in the OWNER's Inbox only (a partner's tap included).
Match text holds at least 3 letters or digits and must occur in the payee; there is one open suggestion per (owner, merchant, income/expense): the suggestion
stores its type (`import_suggestions.transaction_type`), and a row (before the AI call) or a
group's rows (after it) join an open one only when the types are equal, the text contains the
suggestion's text and the suggestion fills a field that row lacks (a group's other rows open a
suggestion of their own). A new suggestion lists only the rows its classification (the most rows'
pair, ties the newest) fills; the others form suggestions under their own pair; an open suggestion's archived or deleted category or payee counts as
unset there, and one left with neither takes no rows. One owner's jobs run
one at a time in-process (`ownerLocks`), so imports landing together do not open two suggestions
for one merchant. A join first touches the suggestion row (`TouchOpenImportSuggestion`, a row
lock): one deleted meanwhile sends its rows to the AI (before the call) or into a new suggestion
(after it), never a foreign-key failure. Removal goes through `ReleaseSuggestionLinks`, which
takes the same row lock first, drops only the links its caller read and deletes the suggestion
only when none is left, so a row a job joined meanwhile is never cascaded away (tested in both
orders on both engines). `get-suggestion-list` (oldest first) is computed at read time (archived
categories and payees dropped, rows already filled or no longer of the stored type left out, each
item's `type` the stored one); a suggestion left with nothing is released there, best effort.
Accept skips rows no longer of the stored type, moves a row's `applied_*` snapshot only for the
fields it wrote, so a field the user set by hand still reads as edited for apply-rule, and
releases only the links it walked; dismiss likewise releases only the links it read, so a row a
job joined meanwhile keeps the suggestion open. Dismissal is not remembered (the next tap may suggest again).
Accept opens the rule prompt (`RulePromptDialog`), whose category picker offers only categories
of the suggestion's `type`; the rule a Save makes has a deterministic id, UUIDv5 of the
suggestion id under a fixed namespace (`suggestionRuleId`, `web/src/features/imports/suggestionRule.ts`),
so a retry (after Cancel and a reopen, a reload, or a create whose answer was lost: `create-rule`
answers an id of the caller's own already stored with that rule, also when an overlapping create
with that id commits first and its insert hits the key) replays that rule and, when its stored
spec differs from the current one, updates it, rather than creating a second one; a rule deleted
meanwhile is created again under the same id. Nothing about it is kept on the device. A
successful dismiss drops that id from the cached list and invalidates it (never replacing the
cache with the answer's list, which out-of-order answers would make resurrect a row); a dismiss
that committed answers 200 even when its follow-up list read fails (an empty list, WARN).
An accept or dismiss refused for a suggestion no longer listed (its first attempt landed, the
answer lost) counts as done.
apply skips rows the rule would not change; `preview-rule` answers both `matched` (every match: the
editors' live counts) and the additive `changes` (the rows apply would change, `alreadyEdited` among
them), and the prompt's apply questions read `changes`, asking to apply to all imports only when at
least one OTHER transaction would change. A failed live count shows no count. An accept writes all
its rows, snapshots and the release in one transaction, so its pinned `Idempotency-Key` is kept only
when everything committed. The Inbox hides suggestions (section and badge)
from a `readonly` user, who could neither accept nor dismiss them. `suggest-match-text` (owner of
the link's account, 10 s, counted under `preview-rule`'s limit, not persisted in the query cache)
gives the rule editor an AI match text from the payee text alone (with the language and
timezone; a row without payee text asks nothing); it answers an empty text, never an error, when AI is off or
the call fails, and the SPA (not the server) skips it for readonly users, offline, for a link a
rule classified (it updates that rule) and until the rule list has loaded; an answer arriving once
Save has started is ignored. The SPA suggestion query is stale after 60 s and refetches on focus
(the job writes after the import answers, and an Apple Wallet push never reaches the device), is
invalidated after every successful write (`invalidateAfterWrite`; a read sent as a POST,
`preview-rule` and `claim-setup-token`, opts out with `meta: { skipInvalidate: true }`), and after
a sync that imported rows, with AI on, again at 15 s and 60 s (`scheduleSuggestionRefresh`,
module-level timers). A failed suggestion list shows the Inbox's import error and Retry refetches
it; a 404 (an older server) does not and is not retried, and a readonly user's list never raises
the Inbox's offline notice. Accept invalidates the plan with the other ledger views. A Save whose
stored rule was deleted since (the update or accept refused and the rule list no longer has it)
forgets it and creates a fresh rule in the same Save; analytics `appImportSuggestionAccept`, `appImportSuggestionDismiss`,
`appImportRuleAiMatchUsed`. It has `repository.go`, `ports.go` (account /
currency / transaction-creation ports, wired in `internal/server/glue_imports.go`),
`repo/`, and `api/` with 33 routes under `/api/v1/import/` (`create-source`,
`get-source-list`, `delete-source`, `link-account`, `ignore-account`,
`unlink-account`, `ingest-apple-wallet-event`, `get-queued-event-list`,
`import-queued-event`, `skip-queued-event`, `unskip-queued-event`, `retry-event`,
`discard-event`, `reimport-link`, `get-transaction-import-list`, `claim-setup-token`,
`get-credential-key`, `set-credential-key`, `list-external-accounts`,
`sync-source`, `get-run-list`, `get-run`, `get-source-history`, `get-rule-list`, `create-rule`,
`update-rule`, `delete-rule`, `preview-rule`, `apply-rule`, `get-suggestion-list`,
`accept-suggestion`, `dismiss-suggestion`, `suggest-match-text`).
The package name is `imports`
(not `import`, a Go keyword). No MCP surface yet.

### Dependency rule

Features never import features. When one feature needs another's data (e.g.
account needs a currency lookup, or budget needs to resolve a user), the
*consuming* feature declares a small interface in its own package (typed in
`model`), and `internal/server` wires a concrete adapter — usually a thin one
implemented in a `glue_<feature>_<purpose>.go` file (e.g.
`glue_account_currencylookup.go`, `glue_budget_userlookup.go`) — over the
*providing* feature's public API at composition time. This keeps every
feature package independently buildable and testable without pulling in its
siblings.

Shared leaves (`shared`, `model`, `web`, `infra`) never import a feature; the
kernel (`internal/shared` + `internal/model`) imports nothing internal outside
itself — `model` may only depend on `shared`. This is enforced by
`internal/test/archtest`, which auto-detects feature packages (any
`internal/<top>` not in its infrastructure set) so newly moved features come
under enforcement without edits to the test.

### The engine-adapter (sqlc) pattern

sqlc generates a SEPARATE package per engine (`gen/sqlite`, `gen/pgsql`) with
distinct Go types, and the two dialects differ (`?` vs `$N` placeholders, float vs
NUMERIC aggregates, date formats). A repo therefore:

- declares a small `querier` interface in the **canonical (sqlite-generated) types**,
- has two thin adapters — `sqlite.go` (native passthrough) and `pgsql.go` (whole-struct
  conversion shim) — selected ONCE in the constructor by `cfg.DatabaseDriver`,
- writes every method ONCE against the interface, so method bodies carry no
  per-driver branching.

Reference repos: `internal/{tag,user,currency}/repo` (the `user` repo also
covers password-reset requests, in `passwordrequest*.go` alongside the user
queries). The one exception is `internal/budget/repo/read.go`, which is
hand-built dynamic SQL (variadic `IN` lists, real per-engine value/date
handling) and branches explicitly by design.

### API handler pattern

HTTP handlers are thin named methods under `internal/<feature>/api/` (the
name carries the swag `@` annotation block); the body is one call into the
generic combinators in `internal/web/endpoint`: `endpoint.Handle` (auth + JSON
body), `endpoint.HandleNoBody` (auth, no body), `endpoint.HandlePublic`
(no auth) — each doing require-user/decode/`Validate()`/call/envelope in the
frozen order. Per-endpoint extras (e.g. `reqctx.AddLogAttr`) live in a small
closure. A handful of endpoints stay hand-written because their shape is
special: CSV export, multipart import, query-param reads, and login's raw
`{token,user}` response. Routes are registered in
`internal/<feature>/api/routes.go` (`RegisterAPI`). Request/result DTOs live
in `internal/model/<feature>_dto.go`.

### MCP endpoint

`/mcp` is a second, independent HTTP edge — mounted on the root mux next to
`/health`, outside `/api`, so the REST `apiparity` scanner never sees it (its
own golden suite, `internal/test/mcpparity/`, owns coverage instead). It's the
official Go SDK's Streamable HTTP handler in **stateless, JSON-response**
mode (no SSE, no server-held sessions — every tool call is a sub-second DB
round trip). Auth is the same bearer-token middleware as REST (PATs are the
intended credential for MCP clients). Shared edge infra lives in
`internal/web/mcp/`; each feature that exposes MCP surface registers its own
tools/prompts from an `internal/<feature>/mcp/` package, composed at
`server.BuildAPI` exactly like `RegisterAPI`.

**MCP OAuth** (`internal/authserver`, not to be confused with `internal/oauth`,
which is Econumo as a *client* of Google/Apple/OIDC): a hand-written OAuth 2.1
authorization server so MCP clients (Claude, Codex) connect by URL alone and
sign in through the browser instead of pasting a PAT. Setup guide:
`docs/mcp-setup.md`; design: `docs/superpowers/specs/2026-10-03-mcp-oauth-design.md`.
It is enabled only when `ECONUMO_URL` is an `https://` origin with no path (plain `http://`
only on a loopback host: `localhost`, `127.0.0.1`, `[::1]`; issuer = `scheme://host`,
resource = `<issuer>/mcp`). A path other than `/` is refused because discovery, `/mcp` and
every OAuth route are root-mounted, so a path-based issuer could not publish its RFC 8414 /
RFC 9728 metadata. Any other value, a path included, leaves it disabled and `serve` logs one
WARN, because refresh tokens, client secrets and bearer tokens must not cross a network in
the clear. The handler is always mounted on the root mux, but while disabled all five public
routes answer a JSON 404 and `/mcp` sends no `WWW-Authenticate`.
- Discovery: `/.well-known/oauth-protected-resource[/mcp]` and
  `/.well-known/oauth-authorization-server`; an unauthenticated `/mcp` 401 adds
  `WWW-Authenticate: Bearer resource_metadata=…, scope="mcp"` (body unchanged, REST 401s untouched).
- `POST /oauth/register`: open dynamic client registration (public clients, or
  `client_secret_post`/`client_secret_basic`). Redirect URIs must be https (ASCII host
  only) or http on a loopback host (`127.0.0.1`, `[::1]`, `localhost`; the port is
  ignored when matching), at most 2048 bytes; clients never approved are purged
  on the next registration once 30 days old, and at most 1000 are kept, oldest out first, so
  anonymous registration cannot grow the database without bound. Registration is behind the per-IP per-endpoint rate cap; the token
  endpoint has no limiter of its own. It creates user apps only; global apps come from the
  CLI's `app:register` (see "CLI / management commands").
- Consent lives at the SPA route `/oauth/authorize`. The session is a localStorage
  bearer token, so the browser cannot carry it to a Go-rendered page: the SPA calls
  `get-authorization-request` and then `POST /api/v1/authserver/approve-authorization`
  (or `decline-authorization`) and navigates to the returned `redirectUrl`. An unknown
  client or unmatched redirect URI is an error page that never redirects; any other
  request fault yields an error `redirectUrl`, but the page shows a "Return to {host}"
  button instead of navigating on its own. `code_challenge` must be a 43-char base64url
  S256 challenge, `state` at most 1024 bytes. Any requested `scope` is accepted (on
  authorize and refresh); `mcp` is granted unless the request asks for `read` without `mcp`
  (below), and a refresh keeps the grant's scope. Approve takes the user row lock and
  re-checks that the presenting session is unrevoked before storing the code with the
  generation read under that lock, so a reclaim racing the approval leaves no code behind.
  The Allow button stays disabled until a real pointer/key event or ~500 ms of the page
  being visible and focused (double-clickjacking guard).
- `POST /oauth/token`: PKCE S256 only; `resource`, when sent, must be
  `<ECONUMO_URL>/mcp` (trailing slash tolerated), else `invalid_target`. Errors are RFC 6749
  JSON, not the envelope, with `Cache-Control: no-store`.
- **The `read` scope.** Any OAuth client, dynamically registered included, gets a read-only
  connection by asking for it: requested scopes (space-separated) that contain `read` and not
  `mcp` grant `read`; anything else, including no scope at all or both, grants `mcp` (mcp wins
  so a client requesting everything discovery lists still reaches `/mcp`). The scope is fixed
  on the code and grant at approval; exchange and refresh carry it and ignore what they are
  asked for. `read` tokens are `oauth` tokens (same `eco_oat_` format, 1 h, rotating refresh)
  admitted only on `GET` of the nine paths in `middleware.ReadScopePaths` (`get-user-data`,
  `get-account-list`, `get-category-list`, `get-tag-list`, `get-label-list`,
  `get-currency-list`, `get-currency-rate-list`, `get-account-transaction-list`,
  `get-transaction-list`); any other path or method, `/mcp` included, gets the frozen 401.
  While the OAuth server is off, already-issued `read` tokens stop being admitted
  (`server.NewReadScopeGate`, as `AllowMCPScope` does for `mcp`; a wrapper because those
  routes sit behind each feature's own `middleware.Auth`). So a self-hosted instance with the
  OAuth server on serves `read` too; with no client asking for it nothing changes. Discovery:
  the authorization-server metadata lists `mcp` and `read`, the protected-resource metadata
  (the resource is `/mcp`) lists `mcp` only. The consent page shows the granted scope
  (`get-authorization-request` carries it): "wants to read your Econumo data" for `read`, the
  read-and-change sentence for `mcp`. A readonly (lapsed trial) user may approve a `read`
  request: `approve-authorization` is on the readonly allowlist but still answers 402 for an
  `mcp` approval. A read token writes nothing to the user row: the timezone write-through skips
  it. The one write its request makes is token bookkeeping, the token row's own `last_used_at`,
  at most once per 5 minutes (its `expires_at` never moves).
- Access tokens are `access_tokens` rows of kind `oauth` / scope `mcp` (`eco_oat_…`, fixed
  1 h, linked to their grant by `grant_id`), admitted only on `/mcp` — anywhere else they
  get the frozen 401. Tokens of a `read` grant carry scope `read` instead and are admitted
  only on the nine `GET` paths above, never on `/mcp`. Turning OAuth off also stops admitting already-issued OAuth access
  tokens on `/mcp` (the chain passes `AllowMCPScope: authSrv.Enabled()`). Refresh tokens
  rotate on every use; a grant idles out 90 days after its last refresh. Every rotated-away
  hash is kept in `oauth_refresh_tokens_spent` for as long as the grant row lives. Replaying
  the token rotated away most recently, within 60 s, is refused with `invalid_grant` and
  leaves the grant alone (two processes sharing credentials); a replay of any older spent
  token at any age, or of the latest one after 60 s, is treated as theft and revokes the
  grant and its tokens. A successful code exchange revokes the user's other grants for the
  same client (re-authorizing replaces the connection). After successful code exchanges and
  refreshes, at most once every 10 minutes per process, it purges, best-effort and outside
  the transaction, expired codes and grants and `oauth` access tokens revoked/expired more
  than 30 days ago.
- Ownership: a self-registered (user) app belongs to its first approver. Approve claims
  an unowned client (`ClaimClient`) under the user row lock, in the transaction that stores
  the code; a claim matching no row re-reads the client and fails unless the caller now owns
  it or it has since become global. Describe, approve and decline refuse anyone else with the coded
  `authserver.app_not_yours` (which names no owner) BEFORE the redirect-URI check, so a
  stranger cannot probe registered URIs; decline never claims; exchange and refresh never
  re-check the owner (grants from before the ownership backfill keep working). Global
  apps (`is_global`, registered by the operator) have no owner, are open to every user, and
  are never purged. `get-authorization-request` and `get-connected-app-list` carry
  `isGlobal`.
- Revocation: `revoke-connected-app` takes the user row lock first. The reclaim
  (`reset-password`, CLI `user:change-password`) and `user:deactivate` revoke every grant and
  every `oauth` token in their own transaction (through the user feature's
  `MCPGrantRevoker` port); `update-password` keeps them, like PATs. A code approved
  before a reclaim fails at exchange via the credentials-generation fence.

### Frontend architecture (React 19 + Vite)

Directory structure in `web/src/`: `pages/` (routes), `features/`, `components/`
(shadcn-style UI), `api/` (typed API clients), `hooks/`, `app/` (providers,
router, i18n setup), `lib/`, `locales/`, `test/`. The shadcn CLI is not a dependency (its tree
brought unpatchable advisories into the lockfile): add components with `pnpm dlx shadcn add`;
its Tailwind helpers are a copy in `src/shadcn-tailwind.css`. Anything that waits on the
user (share invites, imported transactions to review, failed imports, failing
syncs, changes the server refused) surfaces in the Inbox (`web/src/features/inbox`, `/inbox`), fed by one
`useInbox()` hook that also drives the sidebar badge; new attention items go
there, never into a new top banner (that slot is reserved for the subscription
and server-version notices, and the install-the-app banner,
`web/src/features/install/InstallBanner.tsx`, which also tops the signed-out
`AuthFrame` pages); `TopBanner` in `web/src/app/layouts` shows only one of them
at a time, in that order). Runtime config
(`window.econumoConfig`) is served at `/econumo-config.js`: for a running
instance the Go server generates the whole document (see the "Web UI config"
bullet below); `public/econumo-config.js` is only the static fallback used
when there is no Go server in front of the SPA (`pnpm dev` without a backend). Versions are two separate keys:
`VERSION` is the real running version (`getVersion()` — every comparison:
analytics `$app_version`, the update check), `VERSION_REF` the source revision of the build
(`getVersionRef()`, `""` when unknown; `internal/version.Ref()`: the
ldflags-stamped `version.Commit`, else the toolchain's `vcs.revision`), which only
keys the persisted caches — the query-cache and IndexedDB busters are
`getCacheBuster()`, `VERSION+VERSION_REF`, so a published dev deploy (every dev
build reports `VERSION` `dev`) restores nothing an older build cached. A build with no
ref (a bare `docker compose up --build`, which has no `.git` in its context) or one
from a dirty tree shares its key with the previous build; bump `FORMAT_VERSION` in
`transactionStore.ts` when such a build changes the stored shape — `VERSION_LABEL` is the text the UI DISPLAYS
(`getVersionLabel()` — the sidebar footer and the settings row, nothing else),
set by the runtime `ECONUMO_VERSION` and falling back to `VERSION`. Vite also
inlines a build-time `ECONUMO_VERSION` as the bundle's own version (the Docker
build arg of the same name sets it per image build, default `dev`) — that one
IS a real version, used where no server config is present;
`ECONUMO_COMMIT` (the build arg and env var of the same name, set by both publish
workflows and the Makefile from the checked-out sha) is inlined the same way as the
bundle's own `VERSION_REF`.
The sidebar's "What's new in vX.Y" card (`web/src/features/whatsNew`) shows when the
running `VERSION` is a release in a newer major or minor line than the user's
`last_seen_version` option (absent =
an existing user who upgraded; new accounts are seeded with the running release, a `dev`
build seeds and shows nothing; the option only moves forward; web only, never in the native
app); opening `https://econumo.com/releases/<version>` or
dismissing it writes the running version through `update-last-seen-version`. The card
names the line and links its `.0` notes (v1.6.x -> v1.7.2 shows v1.7, links v1.7.0); a patch
upgrade within a line shows no card. Hidden until onboarding is completed.
Lint is oxlint, tests are vitest (`pnpm test`).

**Onboarding Day 0 / Getting started** (`web/src/features/onboarding`; spec
`docs/superpowers/specs/2026-11-01-onboarding-day-0-design.md`, "Revision 2026-11-08"): a new
user's first run is the Getting started stepper at `/` (`OnboardingPage` shell + `Stepper`,
bodies in `STEP_BODIES`; phones land there too, with a Menu link to `/?menu=1` for the nav
list). The step model is `useOnboardingSteps`, keys `import` (optional CSV import; while onboarding is in progress and `import` is not done,
every successful CSV import, from this step or the Settings data page, settles it through ONE
helper, `finishImportStep` in `web/src/features/onboarding/finishImport.ts`, called from
`ImportCsvDialog`'s success path: `finish-import` under a draft-persisted operation id
(concurrent callers share one request), the answered user into the cache, every non-user
query invalidated; the client never writes `import` through `update-onboarding`, since a
pre-marked step would make `finish-import` a no-op with no budget and no AI job. With
`AI_ENABLED` and the user's AI option on, every successful CSV import (onboarding or not; the
server asks for the icons of what it created) and every finish-import then refetch
categories/accounts/budgets/plan at 10, 30, 60 and 110 s (`scheduleAiRefresh`, module-level timers,
not stopped by completing onboarding; dropped once the cached user changes). The Import step only retries a failed `finish-import` (Import is disabled meanwhile,
so the file cannot be imported twice) and moves on to Categories; Categories and Accounts are
NOT ticked by an import, they become review steps), `categories`, `accounts`, `plan` (budget
and plan: Income/Expenses cards of the user's own non-archived categories prefilled from `get-budget-plan`,
since `get-budget` lists expense categories only; the budget's own period; EMPTY inputs are then
prefilled once from `get-plan-suggestions` when it arrives, never over a stored plan or anything
typed before it arrived, and a prefilled value saves like a typed one; `save-plan` writes
only changed rows),
then the device-dependent `home_screen` (client-only install step, never sent to the server)
and `connect` ("Connect your accounts", `ConnectStep`: a card per option `connectOptions()` offers,
Apple Wallet on an iPhone or Mac and SimpleFIN on a secure page with its import on, each a line of
text and a link to its settings page, plus the MCP address with Copy when `MCP_URL` is set; left
out when it offers nothing; ticked once any import source exists; its own "Complete setup" marks it
done and completes onboarding, so the page's header button is hidden there; a stored `apple_wallet`
answer, from the step it replaced, reads as its answer). A done step is read-only (the stepper will not open it; a link to it from
another step opens its settings page instead); skipped steps can be reopened, and "Complete
setup" skips every open step (so do Connect's settings links once categories and accounts are
done, landing on that settings page instead of the budget). New category rows are created unless removed (trash button);
there is no savings toggle and no name-based savings guess (new rows start with the card icon and the default Cash row with the cash icon, the name never changes it; the user picks any icon freely and the icon never decides savings), and the client sends no savings at all. The client is thin: each of those three steps is ONE server write in the `internal/onboarding` feature (`internal/onboarding/steps.go`), and `useDay0` only keeps the submitted form plus one operation id per step in the draft (minted on the first "Save and continue", the main button of Categories, Accounts and Plan whatever changed, replayed by every retry until the server answers, cleared after success), puts the answered `user` into the user query cache (so the stepper's ticks come from the server, and `useOnboardingSteps` writes no option for a step already marked) and keeps the answered `budgetId` for the plan step. A failed call refetches every query (the step may have committed with its answer lost), and a 400 on `id` (claimed by another write) drops the step's id so the next "Save and continue" mints a fresh one. The plan step only plans a budget the user may edit (owner, admin or user role). `POST /api/v1/onboarding/save-categories` (`{id, categories:[{key,name,type,icon}] 0..60, order?}`: creates the rows through category create, a row named like an active category of its type or an earlier row maps to that one, applies `order` (existing ids and row keys, each type in its own slots), and creates the budget only when the user owns a live account and has none), `save-accounts` (`{id, accounts:[{key,name,balance,currencyId,icon}] 0..50, currencyIds, mainCurrencyId}`; the client sends only the new rows, and `currencyIds` carries the picked extras plus every listed account's currency: creates the accounts through account create (the first one of a user without folders creates "General"), sets the profile currency, hides every global then shows main + listed + the accounts' currencies (refusals skipped), then creates the budget over all live own accounts when there is none (and the user has one), else adds the new accounts to the default budget (the `budget` option when readable, else the first owned/accepted one; a read-only budget takes none)) and `save-plan` (`{id, budgetId?, amounts:[{categoryId,amount}] 0..200}`, non-negative decimals, `"0"` a real plan: set-limit for each in the budget's current period, `budget.CurrentPeriod`). No rows is valid on the first two: a user who already has categories or accounts still finishes the step. So the budget appears at whichever of categories/accounts comes second with accounts present, named `onboarding.budget_name` in the request language, in the profile currency; the plan step never creates one (a user without a budget sees "needs an account" with a button to the Accounts step). Each runs in ONE transaction that first takes the user row lock (`user.LockForOnboarding`), then claims `id` with the operation guard; the nested feature use cases join it as savepoints, so a failure on any row rolls back every row, the claim and the revision bumps. A step already done (in the option, or onboarding completed/absent; also the replay of the `id` that marked it) writes nothing and answers the current state, row keys matched back by name (accounts newest first, so an older account of the same name is never taken); an `id` already claimed while the step is open (the operation id table is shared by every create endpoint) is a 400 on `id` (`common.invalid_choice`). `mainCurrencyId` must be usable by the caller (a global or their own live custom, else `mainCurrencyId` invalid_choice), and the visible set always keeps the currencies of every live account the user owns. Each marks its step done (`user.MarkOnboardingStep`, replacing a `!step`) and answers `{user, categories|accounts: {key: id}, budgetId}` (`save-plan`: `{user, budgetId}`). When the server has an AI provider (`ECONUMO_AI_DSN`), `save-accounts` then starts a background job, after the commit and only for the accounts it created, into the budget it created or added them to (goroutine, fresh context, 15 s timeout, panics recovered; the runner is injectable so tests run it inline) that sends the caller's own non-deleted accounts among those ids (id, name, the account's own currency code) plus the caller's timezone (header, else stored) and UI language, never the profile currency, asks for `{"savings":[ids]}`, and flags the chosen accounts that are still live members through the budget feature's `MarkSavingsMembers` (add-account's access rules, never adds a member, skips deleted ones). Jobs are not drained on shutdown and a replay starts none, so a restart can leave the accounts unclassified. A failure, a timeout or an unusable reply changes nothing and logs a WARN with `user_id` + `err_type` only. The job is capped at 20 per user per rate-limit window (over the cap the write still succeeds, unclassified). Without AI nothing is ever marked savings. The SPA invalidates its queries after the write and does not poll, so savings show up on the next budget fetch after the job lands. A reused budget and the user's pre-existing accounts keep their flags. The former per-entity orchestration routes `onboarding/create-budget` and `onboarding/add-budget-accounts` are gone. **Import path**: `POST /api/v1/onboarding/finish-import` (`{id}` -> `{user, budgetId}`, same lock/claim/replay rules) creates the budget when the user has none (over their live own accounts outside hidden folders, every category seeded as budget create does, `onboarding.budget_name`, profile currency) and marks ONLY `import` (`import` is a server step too: `update-onboarding` never reopens it); categories and accounts stay open as review steps. The import review (`internal/onboarding/review.go`) is split by trigger, so nothing is asked twice: EVERY successful CSV import (`import-transaction-list`, during onboarding or not) asks for the icons of the categories and accounts THAT import created, after its commit (the transaction feature's `ImportIconReviewer` port, implemented by `onboarding.Service.ReviewImportedIcons`, wired in `internal/server`; categories it created for another owner, through a shared account, are left out). A large file arrives in several requests, so the review is debounced per user in process: every successful request (one that created nothing too) resets the timer and adds what it created to a pending set, a request that created nothing starts no review of its own, and the review starts once 15 s pass with no further request (an injectable `Scheduler`, `Seams.OnboardingScheduler`; a restart in that window drops the review), one categories and one accounts request over everything the chunks created; and `finish-import`, after its commit, asks only for the savings. Each request goes through the injectable runner (so concurrent in production, inline in tests) with its own 90 s timeout (the icon replies take real providers 20-60 s; the savings-only job keeps 15 s), its own panic recovery and its own WARN (`user_id` + `err_type` + `part`: `categories`/`accounts`/`savings`), so one that fails, times out or answers garbage never affects the others; an import's two icon requests count ONCE against their own per-user cap (`import-ai-icons`, 20 per window, IP-exempt, so imports never use up the savings job's `onboarding-ai` allowance), finish-import's savings request once against `onboarding-ai`; the opt-out is checked when the review starts and again before each call. (a) category icons: the import's created categories that are active and still on a default icon (`model.DefaultIcon` or the CSV import's `model.ImportCategoryIcon`; names + types), reply `{categories:{id:icon}}`; (b) account icons: the import's created accounts still live and on the import's `model.ImportAccountIcon` (names + own currencies), reply `{accounts:{id:icon}}`; an import that created neither sends nothing; (c) savings, only when finish-import created the budget: every live own account through the save-accounts job's savings prompt (`classify`, reply `{savings:[ids]}`), flagged through `MarkSavingsMembers` in that budget. Each carries the timezone and language and never the profile currency. Icons are kept only from `internal/onboarding/icons.go` (a copy of `web/src/lib/icons.ts`, kept in sync by a test) and applied through the owning feature's `ReplaceIcon` (one conditional `UPDATE ... WHERE icon = <read when the request started>`, so a user edit wins and a failed compare writes nothing; an icon the user picked is never repainted). `save-categories` also takes `archive`/`restore` (own category ids; another id, or one id in both, is a 400 on that list) applied through category archive/unarchive in the step's transaction. `save-accounts` also takes `hide`/`unhide` (own live account ids, same 400s): hide moves the account into the user's hidden folder named `onboarding.archive_folder` in the request language (created hidden when missing; a visible folder of that name is the user's and is never hidden, the archive then gets a numbered name) and removes it from the default budget through remove-account, never confirming a savings removal (`budget.account_not_removable`, history in a past period, and `budget.savings_removal_unconfirmed`, a savings member with plans or comments, are tolerated: the account stays a member); unhide moves it into the first visible folder (a `General` folder, shown again or created, when there is none) and adds it back. New accounts also go to the first visible folder, and a budget a step creates leaves hidden-folder accounts out. `save-accounts` also takes `currencies` (`[{accountId, currencyId}]`, own live accounts and a currency usable like `mainCurrencyId`, one entry per account, else a 400 on `currencies`), and `balances` (`[{accountId, balance}]`, same ownership rule, a plain decimal, else a 400 on `balances`), both applied in the step's transaction through account update-account with the account's own name and icon (an untouched balance is resent as shown, so it writes no correction; a changed one books update-account's "Balance adjustment" dated now in the caller's timezone); those currencies join the visible set. The CSV import creates every account in the instance base currency, so the review screen gives existing rows the same currency chip and balance input (parsed in the UI language) as new ones, with the name read-only and an Archive / Restore toggle (the hide/unhide above); balance, currency and action cells have fixed widths so the columns line up. The Currencies block is one wrapping row of chips: the main currency first (filled, a star, "· Main"), then the other selected currencies, then "+ Add currency", with a hint under them. Tapping the main chip opens the full searchable currency list (the same PickerSheet as Add; both, like the account rows' currency picker, offer every live global and the user's own live customs, currencies the user hid included, `usableCurrencies`, since save-accounts shows the chosen ones again): the pick becomes main and joins the chips, the old main stays selected. Tapping another chip opens a menu with "Make main" and, unless a listed account uses that currency (those always stay visible), "Remove". "Add currency" only adds; it never changes the main currency; the plan step shows each row's currency code (its element's, else the budget's). In the account list the first row of the combined list (the first shown existing account, else the first new row) leads, its pick moves every row the user has not changed by hand (existing ones go out in `currencies`) and the main currency. When `save-accounts` lands in an existing default budget that the caller owns alone (no participant, invited or accepted), that is not archived and has no limit in any period, it also switches that budget to `mainCurrencyId` through budget update-budget: the budget finish-import or save-categories created took the profile currency of that moment. There is no marker for an onboarding-created budget; an owned budget of a user still in the accounts step was created after onboarding began, and "unshared, nothing planned" is what makes the switch harmless. `GET /api/v1/onboarding/get-plan-suggestions?budgetId=` (a named budget needs update access, else 403; without it the default budget, or `{budgetId:"", items:[]}` when the user has none they can update; read-only) answers `{budgetId, items:[{categoryId, amount}]}`: for each own active category, income and expense, the average over the three complete periods before the budget's current one (`budget.CategoryHistory`: the plan sheet's per-category actuals, but over every account the CALLER owns, hidden-folder (archived) and soft-deleted ones included, rather than the budget's members, so archiving the account that holds the history keeps the suggestions while membership rules are unchanged (an archived account still leaves the budget); another user's account shared into the budget never counts; tagged expenses counted for their category, converted into the element currency with each period's average rates), sum divided by three and rounded to the element currency's fraction digits, only when at least two of those periods have a transaction and the average is positive. Categories and accounts the user already has are
reviewed with their names read-only (category rows wholly; an account's balance and currency
stay editable, see `balances`/`currencies` above): every own category (archived ones too, muted,
with Restore) carries an Archive/Restore toggle and every own account (one in a hidden folder
reads Archived, muted) an Archive/Restore toggle (the account moves to the hidden Archive folder
and usually leaves the budget; there is no hint line); a new row named like an archived category restores it instead; Save and continue sends the new rows plus `archive`/`restore` (`hide`/`unhide`)
diffs against the server state, and the step-complete events carry `archived`/`restored`
(`hidden`/`shown`) counts; the unsaved
draft is resumable in `localStorage` (`econumo-day0:<userId>`) and the writes are idempotent, so
a reload or retry creates nothing twice. Onboarding completes automatically once every visible
step is done or skipped (also when answered on another device). The install banner and popup
stay hidden until it is complete. There are no budget-page "get ready" cards and no Inbox "Next
steps" section any more. Per-user progress lives in the `onboarding` user option itself: registration seeds
`started` (nothing answered), each answer replaces it with the comma-separated step states
(keys `import`/`categories`/`accounts`/`plan`/`connect`, plus `apple_wallet` still accepted from older clients, each at most once, a `!` prefix =
skipped (`!plan`)), written by the step routes for a done `import` (finish-import)/`categories`/`accounts`/`plan` and otherwise (skips, import, install, Connect) by `POST /api/v1/user/update-onboarding` (which never reopens an `import`/`categories`/`accounts`/`plan` the server marked done: a stale stepper write keeps it done, so a later save under a fresh id cannot create the rows twice) (`web/src/features/onboarding/onboardingProgress.ts`,
optimistic, cancel-in-flight, last write reconciles). `completed` is terminal: only
`complete-onboarding` writes it, `update-onboarding` refuses the value (`common.invalid_choice`)
and is a no-op returning the current user once onboarding is complete (or the option is absent,
a user predating onboarding). Older clients check only `!== 'completed'`, so they still route an
in-progress user into onboarding. Migration `20261110000000` marks onboarding
complete for existing users with at least one transaction (one they wrote, a partner's shared
account included, or one on an account they own, either side, deleted accounts included);
owning accounts alone does not count, and everyone else keeps their option (or has none).

**Reports page** (`/reports`, `web/src/features/reports`; an icon before the sync button in the
sidebar footer, not a menu entry): Net worth, plus a "Suggest a report" card that opens the
feedback form on its feature-request type. The tab list is stored per user on
the device only (localStorage `econumo.reportTabs`) and Reports always opens on the FIRST tab.
Every report query key is invalidated after any successful mutation
(`web/src/lib/appQueryClient.ts`), so a report never shows stale figures.

**Service worker** (`web/sw/sw.js`, registered by `web/src/lib/serviceWorker.ts`, production
builds only): it makes the installed PWA start offline. A Vite
plugin in `web/vite.config.ts` writes `dist/sw.js` after the build, prepending the precache
list (the bundle, the manifest and the icons it and `index.html` name)
and a build hash, a digest of the worker and those files rather than the release version (every
dev deploy reports `dev`), so a deploy that changes any of them installs a new worker
(`skipWaiting` + `clients.claim`, every other shell and the runtime cache dropped on activate).
Navigations are network-first with the shell cached under `/` (Go's file server redirects
`/index.html`); `/assets/*` is cache-first; other shell files network-first. Network-first falls
back to the cache after 4 s, body included, so a dead connection opens the app in about 4 s, and
on a 5xx or 404 when a copy exists (the server has these files and answers client routes with the
shell, so those are from a proxy whose upstream is down). `/api`,
`/mcp`, the OAuth server routes, `/.well-known`, `/health`, `/_` and liltag are never intercepted,
and nothing user-specific is cached (the data lives in IndexedDB, below). Two rules keep an
offline launch consistent: the offline `/` is only ever the precached one (a newer build's page
would name a bundle that is not cached), and `econumo-config.js` lives in the `econumo-config`
cache, which outlives every worker. Only the page that booted with a config stores it
(`rememberRunningConfig`), the worker just reads it, and a page opened from the cached shell gets
the cached config even if the server answers that request. Its
`VERSION_REF` keys the stored data (`getCacheBuster`), so a worker that updates while the page
stays open must not make the next offline launch discard the transactions; the price is that
such a launch runs the new bundle over data the previous build stored, until it is next online,
so a change that breaks reading stored data must bump `FORMAT_VERSION`, which both the store and
the query cache key on. "Clear cache and reload" also asks
the worker to download its shell files again. Install and refresh share one
check: nothing changes until every file has arrived and belongs to this build, so an unreachable
server, a redirect, an HTML page under an asset's name, a shell page naming no asset (a proxy's)
or a server already on another build leave the offline copy in place. The hook's lock
then lasts until that answer too (at most 60 s).

**Transactions are not in the localStorage query cache; they live in IndexedDB.** One
database, `econumo` (`web/src/lib/db.ts`), holds four stores: `pendingWrites` (see "Offline writes" below), `importKeys` (the SimpleFIN
data key, migrated once from the old `econumo-import` database), `accountTransactions`
(one row per account: its `transactionsRevision` and transactions) and `meta` (`buster`,
`userId`, `savedAt`, plus a copy of the query-cache snapshot). `web/src/lib/transactionStore.ts` is the only reader/writer: loads
are readonly (a readwrite transaction only to wipe another user's or build's data), waits that hold boot
or auth time out after 1.5 s (a slow database is no failure and stays on), background writes
are never awaited, and errors never surface (a broken IndexedDB turns the store off for the
session; the sync keeps working in memory). `clearTransactions` (called by
`clearPersistedQueryCache` on every login and logout) bumps an epoch
that fences any in-flight sync so it cannot write the previous user's data back. A full reload
never clears: `markForReload` sets every stored bucket's revision to `RELOAD_REVISION` (-1)
and keeps its rows, behind the same epoch fence, so a reload whose downloads fail (a server
unreachable while the browser reports online) still leaves the history readable offline. A stored
`buster`/`userId` that no longer matches wipes the store on load without an epoch bump. The
query-cache restore seeds `['transactions']` from IndexedDB inside `restoreClient` whenever
a session token exists, independent of the localStorage snapshot: `meta.userId` must be the
session's user (`sessionUserId`, stored by `setToken` at login/oauth exchange next to the token,
filled in once from `get-user-data` for older sessions; a snapshot `user` must match it too;
the login clear can time out, so the store alone does not prove its owner), `meta.buster`
must be this build's, and without the
snapshot's accounts every stored bucket is shown until the refresh sync corrects the set. A
snapshot of this release past its 24 h `maxAge` is kept (re-stamped) when the store seeds, so
an offline start after a day away shows data instead of the boot loader. The persister also
copies each snapshot into the store (`putSnapshot`, throttled like the localStorage write,
fenced by the epoch, dropped with the buckets when another user writes); the restore uses
that copy when localStorage holds no snapshot of this release, so a device that lost its
localStorage but kept IndexedDB still starts offline. The default budget's previous, current and
next period (`get-budget`, a month before the budget's first included) and one 21-month
`get-budget-plan` window, from six periods before the previous one to the next one plus the widest
Plan grid (12 columns; `keptPlanWindow`, capped at 24 months keeping the months ahead; within one
period of today `monthPlanSpan` gives the Budget view that very window), are kept for offline use by
`useKeepBudgetMonths` (`web/src/features/budgets/offlineMonths.ts`, mounted in `ApplicationLayout`):
it holds those keys with DISABLED observers, so they are never garbage collected and invalidating
the budget queries does not refetch them on every page; it fetches them itself, online only, at once
when missing and 3 s after the LAST write that invalidated them (a burst of writes refetches the set
once). Writes are seen through the mutation cache's success events and `noteWrite`
(`web/src/lib/writeEvents.ts`, called by the pending-writes sender), never through the 'invalidate'
cache event alone, which fires only for a query not already invalidated and so misses a write that
lands while a fetch is in flight. A Budget or Plan page on screen refetches its own month at once as
before. They outlive `maxAge` through the re-stamp above; a new build's buster drops them until the
next online start. Offline, `useBudgetPlan` serves a window never fetched under its own key (another
grid width, a clamped start, a period that rolled over) from a cached window of the same budget
holding all its months (`web/src/features/budgets/planCover.ts`: trailing months cut, the start kept
because the opening balances belong to it, parts an older server omits tolerated), returned only and
never stored under that key, and keeps serving it while the connection returns until the window's
own fetch lands. That
key's `queryFn` is the revision-driven sync
(`web/src/features/transactions/{sync.ts,revisionTracking.ts}`): it compares the bucket
revisions with the `transactionsRevision` of `get-account-list` and calls
`get-account-transaction-list` with `accountIds` naming only the stale accounts (no request
when none are stale) for accounts whose stored revision changed (over 200 of them go one by
one). An account with no stored bucket (first load, resumed load, new account), a bucket marked
for reload and every account on a full reload go in requests packed by size (`packDownloads`:
by the account's `transactionsCount`, largest first, small accounts sharing a request of up to
2,000 rows, an account that big or bigger alone; one request per account against a server
without counts), at most two in flight, each request's buckets written to the store in the
background as they arrive, so an interrupted first load resumes with only the missing
accounts; the list lands once every account is in.
A full reload drops the in-memory state, so a retry after it fails resumes from the store,
downloading only the buckets still marked. It reads
IndexedDB only while the page holds no synced or restored list (first boot, after a
login/logout); otherwise the buckets come from memory: the cached list regrouped
by account under the revisions it was synced at. A write whose response carries no
transactions (a classification merged or deleted, a recurring template linked or deleted)
patches the list through `patchTransactions`, which cancels a sync in flight (it read the
rows before the write and would put them back) and runs it again on the patched list.
Hidden-folder accounts are included and pending share invites skipped; against a server
whose accounts carry no revisions it falls back to `get-transaction-list` (hidden-folder
account pages then show no transactions). The user's own edits are recorded locally without
a download when the response revision is exactly +1 of the stored one. The sync button
(`ApplicationLayout`, via `useLongPress`): a tap is an incremental refresh, and also syncs
every SimpleFIN source when this device holds the unlocked key and no SimpleFIN sync ran
from this device in the last 10 minutes (`syncPullSourcesIfDue`,
`web/src/features/imports/sidebarSync.ts`; the SimpleFIN page's own button stamps the same
time and has no limit; silent unless something came in, failures surface in the Inbox); holding it
>= 600 ms or Shift+click is a full reload (every account re-downloaded, toast,
`METRICS.DATA_FULL_RELOAD` with `source: 'sync_button'`). The same full reload
(`useFullReload`, `web/src/features/transactions/useFullReload.ts`) is the last row of
the Settings page's Data group (not inside Import & export), "Clear cache and reload" (`source: 'settings'`; no confirmation).
The hook ignores a second trigger while its reload runs (until the download lands or fails
once, or pauses offline), and the row is disabled for that time. Offline it clears nothing,
only toasting `settings.sync.full_reload_offline`.

**Offline writes** (`web/src/features/pendingWrites`; spec
`docs/superpowers/specs/2026-11-08-offline-writes-design.md`): adding, editing and deleting a
transaction and changing a budget limit work with no connection. `queue.ts` holds the pure
rules, `store.ts` persists the queue in the `pendingWrites` store of the `econumo` IndexedDB
(one row per operation, tagged with the user id; never in localStorage), `sender.ts` is the
only sender and `submit.ts` the entry point the hooks call. `startPendingWrites` runs from
`main.tsx` once the query cache has restored (`onSuccess`/`onError` of the persist provider)
and sends oldest first, one at a time, on load, on reconnect and after a sign-in, under the
Web Lock `econumo-pending-writes`, so only one tab sends. The lock does not serialize queue
edits across tabs, IndexedDB transactions do: the sender marks a row attempted in a readwrite
transaction that returns the row as stored and sends that (`claimForSend`), and a persist that
would fold into a row another tab's sender has taken leaves it and lands the newer values
behind it under a key of their own (a row another tab has already settled and deleted likewise:
the key is spent, so the newer values take a fresh one, as an edit of an add). Offline, `submitChange` queues at
once and the dialog closes; online it sends directly (10 s timeout) and a refusal (a 4xx with
the envelope) reaches the dialog as usual, while no answer, no response or a 5xx with the
envelope (the server may have committed first) turns the change into a waiting one under the
same key and the dialog closes. An online change to a target that still has an operation queued goes
through the queue, so a direct request never overtakes it; while the stored queue is still
loading, `submitChange` waits for it (at most `STORE_TIMEOUT_MS`), and when it has not arrived the
change is queued anyway, the sender holds until the rows arrive, and the change is replayed behind
them (`replayBehind`). Each operation stores one UUIDv7
`Idempotency-Key`, reused on every attempt. A target (a transaction id, or a limit's budget,
element and period) has at most one operation besides one in flight: editing a waiting change
replaces it, deleting a transaction added offline drops it with nothing sent, a change sent
and unanswered is never altered and the next one queues behind it. A transaction added
offline has a temporary device-only id, swapped for the server's on success, or from the
replay's `resourceId` on a 409. On a replay of a transaction change the operation stays overlaid
until accounts and transactions have been read back (at most the 10 s write timeout), so the row
never reverts; a read that fails keeps it, attempted, for a retry after a backoff. That read
cancels a sync already running (it may predate the write) instead of joining it, and the `landed`
mark that stops the balance counting the change again is stored with the row, so a reload does
not count it twice. At boot, online, with attempted changes in the queue, `refreshRestoredQueries`
waits for them to resolve (`attemptedWritesResolved`, at most the 10 s write timeout), and the
restored accounts and transactions are stamped fresh until then (`holdRestoredReads`, awaited
before the observers mount), so neither that refresh nor a mount past their staleTime can bring a
lost-response add before the sender knows about it. A reconnect with attempted changes holds the
same two queries the same way (a fetch paused offline is cancelled) and invalidates them once
`attemptedWritesResolved` settles.
The caches keep server rows only: the waiting changes are applied at READ time
(`overlay.ts`, through `select` on the transactions, accounts, budget and plan queries), so a
sync or a restore cannot erase them; the budget's Spent/Available figures do not move for a
waiting transaction, the account balance does. A limit's optimistic patch is undone cell by cell
(`limitPatches.ts`), never from a whole-cache snapshot, which would hold other edits still in flight.
`overlayBudgetTransactions` also drops a row an edit took out of the listed selector or period, as
far as the client can judge from `BudgetTransactionsParams` (everything else stays listed). A waiting transaction looks like any other
row and its preview says "Not saved" (a failed one shows the reason), a waiting limit shows
`NotSavedIcon` next to the number, and the sync area's badge counts what is waiting. Outcomes
(`classify.ts`): 409 = done; a 4xx with the app's envelope = refused (402 included); a 5xx with
the envelope retries 5 times with backoff, then is refused, stepping aside meanwhile for
operations on other targets; no response, a timeout, a 429 or a body that is not the envelope
(a proxy's page) keeps it waiting with no limit; 401 pauses the queue. A refused change stays
local, marked failed, and appears in the Inbox's "Not saved" section
(`features/inbox/FailedWritesSection.tsx`, counted in the Inbox badge): a failed add or edit
opens the normal transaction dialog with the user's values and saving sends it again, a failed
delete or limit has Try again, and all have Discard; other changes still go through. Signing out
discards the queue without a warning (`LogoutPage` clears it; the owner's decision); the
sign-out after "Set a password" (every session revoked, the same user signs back in) passes
`{ keepPendingWrites: true }` as router state and keeps it. A
login keeps the queue when the same user signs back in, and a different user's rows are
deleted on load. Everything else is **disabled while offline** through one hook, `useOnline`
(`web/src/hooks/useOnline.ts`, on `onlineManager`), and fails at once if something slips past
(`networkMode: 'always'`, no retry) with one global no-response toast (fixed id, so repeats
collapse) unless the mutation has its own `onError` or sets `meta: { handlesOwnErrors: true }`,
which a hook needs when its callers show the failure themselves (a per-call `onError`, a try
around `mutateAsync`, an inline `isError`); a hook with mixed callers takes it as an option. `web/src/lib/offline-coverage.test.ts` fails the suite
when a source file fires a mutation without reading `useOnline`, unless it is listed with a
reason in `OFFLINE_EXEMPT` or goes through a shared component in `OFFLINE_VIA_SHARED`. So a new
write control needs the hook or an entry there. A page whose writes are all off says so offline
with one inline notice under its header, `OfflineNotice` (`web/src/components/OfflineNotice.tsx`,
"No internet connection. Data is view-only."; never a top banner or
a toast): `SettingsShell` renders it unless the page passes `editableOffline` (the Inbox), and the
Settings hub, Getting started and the OAuth consent page render it themselves; Home, account pages,
Budget/Plan, the Inbox and Reports show none. `web/src/lib/offline-notice-coverage.test.ts` fails
when a routed page neither shows it nor is listed with a reason. Data the device never loaded is
`OfflineUnavailable` instead (the page or dialog never spins offline). Analytics: the usual create/update/delete/limit
events fire when the server confirms, `METRICS.WRITE_QUEUED`/`WRITE_RETRIED`/`WRITE_DISCARDED`
at those points.

**App lock** (`web/src/features/appLock`; spec `docs/superpowers/specs/2026-11-11-app-lock-design.md`,
"Revision 1" wins, "Revision 4" for patterns, "Revision 5" removed biometrics): a per-device secret, a PIN (4-8 digits) or an Android-style pattern
(4-9 of the 3x3 dots, `pattern.ts`, encoded `p:0-4-8-5`, a dot crossed straight on joins first; `PatternPad` with
pointer capture, `touch-action: none` and `data-vaul-no-drag` so a swipe never drags the sheet, and dot buttons
plus Done for the keyboard), one or the other, both hashed the same way (PBKDF2-HMAC-SHA256 in `secret.ts`: `crypto.subtle` with 150 000
iterations, else, on plain http, the pure-JS `@noble/hashes` derive with 25 000; the count is stored
with the hash and either implementation verifies the other's), frontend only, never sent to the server.
There is no biometric unlock (the owner's decision; a config stored with a `biometric` field still reads, the
field ignored). Storage:
localStorage `econumo.appLock` (config: owner `userId`, `secret` {kind, hash, length; a config from before
patterns holds `pin` and reads as kind pin}, `lockAfterMs` 0/1/5/15 min) and `econumo.appLock.attempts`, re-read on every check, plus the
sessionStorage marker `econumo.appLock.fresh` that `setToken` sets so the reload after a sign-in does
not ask for the PIN (it counts for 30 s; any lock but the one taken as the page hides, and turning the
lock on, spend it), and the sessionStorage `econumo.appLock.seen` (when this tab was last unlocked and
on screen, written on unlock, hide and heartbeat, dropped by any lock): a reload within `lockAfterMs` of
it starts unlocked, while a new tab or a fresh launch has none and starts locked. `setToken` is also the only place that deletes another user's lock (both ids
known and different); `activeConfig()` fails closed (no `sessionUserId` yet still locks) and works the same over plain
http and https. `initAppLock()`
(`lockStore.ts`, from `main.tsx` before the first render) starts every page load locked, locks after
`lockAfterMs` in the background or a slept device (15 s heartbeat), toggles a static `#privacy-cover`
on hide outside React, and reloads a locked tab when another tab changes the lock in any way or
changes the session (a storage event never unlocks). The gate is in `RequireAuth`, which renders `LockScreen`
INSTEAD of its outlet, so nothing financial stays mounted. Every current-PIN prompt (lock screen and
settings, `AppLockPage` at `/settings/profile/app-lock`) goes through `checkSecret`, which stores the
failure before deriving; from the 5th failure a lockout, the 10th signs out. Sign-out:
`LogoutPage` drops the lock unless it was sent `{ keepPendingWrites: true }` alone ("Set a password");
"Forgot PIN" and the attempt limit send `{ keepPendingWrites: true, resetAppLock: true }`, keeping the
owner's offline queue but not the lock. Threat model: someone handling an unlocked device through the
UI; anyone with devtools or the browser profile can read the unencrypted caches or delete the config.

**Product analytics rule:** every new user-facing feature/action MUST fire an
analytics event — add a key to `METRICS` (`web/src/lib/metrics.ts`, frozen
`app`-prefixed camelCase names; the collector's snake_case name derives
automatically) and call `trackEvent` at the action's success point (mutation
`onSuccess`; or inside `mutationFn` after the API call for hooks with a dedupe
short-circuit, e.g. the classification creates). Prefer the shared hook/store
choke point over per-page call sites so every surface (pages, dialogs, inline
creates) is covered once. `web/src/lib/metrics-coverage.test.ts` fails the
suite if a `METRICS` key is never fired; a catalogue key may only be excused
via its documented `NOT_WIRED` list. Analytics are identified, never anonymous:
the collector receives authenticated sessions only. Delivery is the
Twillingate JS SDK (`web/src/lib/analytics.ts`, the only file naming the
vendor), injected from `https://t.econumo.com/js/twillingate.js` on the first
capture — it cannot be bundled, the served file carries the collector's
origin — as the named instance `econumo` (the cloud's liltag snippet owns the
default one), `identity: "identified"`, no consent (so it keeps nothing on the
device, sends `$consent: 0` and no `$install_id`, and detects `$os`/`$browser`/
`$device` itself) and no automatic tracking. Twillingate is the only sink —
there is no `window.dataLayer`. `trackEvent` sends nothing without a session
token (login/register page views and the pre-login auth events go nowhere);
`PAGE_VIEW` goes out as a real `$page_view` (no `$referrer`) and every other
metric as a product event carrying its `eventData`. Both kinds carry the
system keys `$host` (synthetic, below), `$path` (the route with every UUID
templated to `:id`) and `$app_locale` (the UI language); `$device` is left to
the SDK's detection. The wrapper holds calls until the SDK
has loaded AND the user id is set (the boot page view fires before
`get-user-data` resolves), discarding anything still unattributed on
logout/401, and flushes the SDK before every identity change, because the SDK
reads identity at flush time — so every batch carries a hashed user id (`$user_id`, a truncated
SHA-256 over the user's id, computed client-side in
`web/src/lib/analyticsId.ts`) and a
per-instance group (`$group_id`, the bare per-deployment digest from
`internal/infra/instance`; `$group_name`, the same `$host` value every event
carries — `econumo.com`/`*.econumo.com` verbatim, every other hostname as
`selfhosted_<instance-digest>` (`isCloudHost`/`analyticsHost` in
`web/src/lib/metrics.ts`), so a self-hosted deployment's real hostname never
appears in an event payload (the browser's request still discloses it via the
mandatory `Origin` header, which the collector does not record)),
plus session-wide account-profile facts (`web/src/lib/analyticsProfile.ts`:
total counts of connections, budgets, accounts, categories, payees, tags and
labels — active, hidden and archived alike, unaccepted invites excluded — plus
`signup_year`, `months_since_signup` and `import_apple_wallet` (`on` when the cached
source list contains an Apple Wallet integration, `off` otherwise; omitted
until the source list loads)) and the chosen `theme` (`system`/`light`/`dark`, the
local preference, not the resolved appearance), stamped onto
each event at capture time (the SDK has no custom batch attributes). A per-user
`analytics` option (`users_options`, on by default) gates capture, is
mirrored to `localStorage` for a synchronous boot-time check (`get-user-data`
resolves after the first pageview), and is toggled via
`POST /api/v1/user/update-analytics`. A per-user `ai` option (absent = on, `"0"` off) is
the AI opt-out, toggled via `POST /api/v1/user/update-ai` (`{enabled}`, answers the user)
from a Profile row shown only when the served `AI_ENABLED` is true; see `ECONUMO_AI_DSN`. The
row and its dialog (`AiDialog`) mirror the analytics ones exactly: the privacy promise first
("Your financial and personal data stays private and is never sent to AI."), then a deliberately
generic muted description that never lists what the AI does, then the card with the switch;
opening it fires `USER_AI_SETTINGS_OPEN`.

### i18n (`locales/`, `internal/infra/i18n`, `web/src/app/i18n`)

Translations live in per-language catalogues `locales/<lang>.json` — currently
11: `de`, `en`, `es`, `fr`, `it`, `nl`, `pl`, `pt`, `ru`, `uk`, `zh` — shared by
both stacks, no per-stack duplication. Every catalogue must carry every key
(`en` is the reference; enforced by `internal/test/i18ntest`). The Go side embeds them (`package locales`,
`go:embed *.json` in `locales/embed.go`, read via `locales.FS`); the SPA
imports the same files through Vite JSON imports for `react-i18next`. Keys are
namespaced to mirror `web/src/features/<name>` plus three cross-cutting
namespaces: `common`, `errors` (error-code catalogue, see the envelope section
above), and `emails` (backend-rendered mail). `{var}` placeholders use the same
`{name}` syntax in both stacks. Adding a language means a new
`locales/<lang>.json` plus entries in `i18n.Supported`
(`internal/infra/i18n/i18n.go`), registering the catalogue in `resources`
(`web/src/app/i18n.ts`) and `getLocaleOptions()` (`web/src/lib/config.ts`);
the `internal/test/i18ntest` guards derive their language list from
`i18n.Supported`, so they cover the new catalogue automatically.
- **Backend runtime**: `internal/infra/i18n` (`i18n.T(lang, key, params)`) translates
  server-rendered text — the password-reset email, and the error `message`/`errors`
  strings on both edges (REST envelope and MCP tool errors) for errors that carry a
  catalogue code. The `Language` middleware resolves `Accept-Language` to a supported
  two-letter tag and stashes it in `reqctx`; the middleware chain is
  `requestid -> accesslog -> recover -> cors -> timezone -> language -> [auth]`
  (`internal/web/middleware/middleware.go`).
- **Frontend runtime**: `web/src/app/i18n` wires `react-i18next`; UI language
  choice persists in `localStorage` (`locale()` in `web/src/lib/config.ts`) and
  is applied via `LanguageSelector` (`web/src/components/LanguageSelector.tsx`,
  surfaced on auth pages and in Settings). `apiErrorMessage`
  (`web/src/lib/apiError.ts`) renders a failed response from the envelope's
  server-translated strings: the first field error when present (the top-level
  message is then the generic form label), else `message`; `apiFieldErrors`
  serves inline per-field form errors.
  `pluralPick` (`web/src/lib/plural.ts`) reads pipe-delimited catalogue values
  (`"one | many"`, Russian `"one | few | many"`) and picks the variant via
  `Intl.PluralRules` — i18next's own plural suffixes are not used, so all
  plural strings are authored as a single pipe-joined value. The selected
  language is also persisted server-side (`users.language`, default `en` —
  written by `update-language` and on login from `Accept-Language`; read back
  as the stored-language fallback for header-less authenticated requests on
  both edges, and reserved for future background email rendering).
- **Guards** (`internal/test/i18ntest`, run inside `make go-test`): catalogue
  key parity of every language against `en`, `{var}` placeholder-set parity per key,
  frontend-source `t()`-call key coverage against the catalogue, two-way
  coverage between `errs.AllCodes` and the `errors.*` catalogue keys (every
  registered code has a translation and vice versa), and that every mailer
  email key has an `emails.*` entry in every language.

## Testing

Tests live alongside the Go code:
- `*_test.go` unit/integration tests per package (sqlite via `internal/test/dbtest`;
  `dbtest.NewSQLite` opens through the production opener `sqlite.Backend.Open`,
  so tests get the same connection settings as `serve`: the frozen datetime
  layout DSN parameters, `foreign_keys = ON`, one connection). Feature API
  harnesses and the CLI test env go through the same opener — never open a
  test SQLite with a raw `sql.Open`, or rows land in the driver's default
  `time.Time.String()` form that production no longer writes.
- `internal/test/apiparity/` — the shared API scenario catalogue: every registered
  route is replayed against the REAL production handler (`server.BuildAPI`).
  Two consumers: the untagged **smoke suite** (every `make go-test`) diffs each
  scenario's responses against committed golden files in `testdata/golden/`
  (normalized: generated UUIDs, datetimes, bearer tokens redacted), and the build-tagged
  parity suite below. Guard tests enforce that every route has a scenario, the
  scenario count and scanned-route count never shrink, and no golden is orphaned.
  Regenerate goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`,
  then INSPECT the diff — a golden change means observable behavior changed;
  never hand-edit a golden. If route-registration files move, update
  `handlerGlobs` in `guard_test.go`. For a 3xx response (the oauth callback error
  redirects) the harness records the response as `Location: <value>` instead of a
  body, so the golden captures the redirect target rather than an empty page.
- `dbtest.New(t)` selects the engine by `DBTEST_ENGINE` (default sqlite; `pgsql`
  under `-tags enginecompare` → Postgres, each test in its own schema). `make
  test-repo-pgsql` reruns the whole repo/unit suite against PostgreSQL so the
  pgsql adapters + generated queries are exercised, not just the parity
  catalogue; it is part of `make test` and the CI regression job. Raw SQL
  in a test must use `db.Rebind(query)` for `?`→`$N`, and decimal assertions
  compare normalized `vo.Decimal` values (sqlite text vs pgsql NUMERIC differ).
- `internal/test/enginecompare/` — the strongest contract: replays the same
  catalogue on BOTH SQLite and PostgreSQL and asserts byte-identical
  responses (build tag `enginecompare`).
- `internal/test/upgradetest/` — the release upgrade suite (build tag `upgrade`, `make
  test-upgrade`, its own CI job; linux only). Every other suite starts from an empty database, so
  none can see a migration that misbehaves only on a database an older release left behind (#413).
  For the newest patch of every minor line from v1.2.0 on (read from the git tags;
  `ECONUMO_UPGRADE_FROM` overrides) and for SQLite and PostgreSQL, it downloads the release's
  `econumo-linux-<arch>` from GitHub Releases (checked against `SHA256SUMS`, cached in
  `ECONUMO_UPGRADE_CACHE`), boots it on an empty database, seeds a household through its HTTP API
  (version-tolerant: fields an old release lacks are ignored, routes it lacks are skipped), records
  its reads, then boots this checkout's binary on the same database (twice) and requires: a schema
  equal to a fresh install's (columns by name, indexes, foreign keys), the same migration set, no
  newly applied migration numbered below the release's newest one (the hazard behind #413; past
  cases are allowlisted in `releasedOutOfOrder`), the release's sessions and PATs still valid and
  login working, every value the release returned still returned (extra fields and rows allowed;
  changes a release made on purpose are listed in `intended`, scoped to older sources), exact
  balances and budget figures, working writes, and no ERROR log line. A new migration therefore
  needs a version above every RELEASED one, a hotfix release included.
- `internal/test/fixture` — shared fixture builder (users, accounts, access tokens, ...);
  `internal/test/authstub` — a stub `middleware.TokenAuthenticator` for feature api tests
  (the bearer token IS the user id string).
- `internal/test/mcpparity/` — the MCP counterpart to `apiparity`: a golden-file JSON-RPC
  scenario catalogue (`initialize`, `tools/list`, `prompts/list`, each tool/prompt)
  replayed against the real `server.BuildAPI` handler over `/mcp`,
  normalized the same way as the REST goldens. Runs in the smoke tier and, under
  `-tags enginecompare`, against both engines. Regenerate goldens with
  `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`, then INSPECT the diff — same rule
  as `apiparity`.
- `internal/test/revisionguard/` — guards the per-account `transactions_revision` counter
  (see "Transaction revisions" below): replays the `apiparity` and `mcpparity` catalogues plus
  guard-only scenarios against the real `server.BuildAPI` handler and fails when any account's
  transaction fingerprint (rows, labels, author/recurring links, the display names and avatars
  of the authors) changes without its revision moving. Smoke on sqlite; under
  `-tags enginecompare` also on PostgreSQL. A new write path that touches an account's
  transactions fails here until it bumps, and one that stores a row without its sort keys
  (see "Transaction order") fails here too.
- `internal/infra/oidc/oidctest` — an in-process fake OpenID provider (discovery, authorize,
  token, JWKS, userinfo, end-session) used to drive the real `internal/oauth` callback through
  `httptest`, so provider-flow tests exercise actual protocol code rather than stubbing it out.

### Manual regression test plan

`docs/regression-test-plan.md` is the canonical manual regression checklist
(all modules, mobile/tablet/desktop viewports, multi-user sharing scenarios).
**Any change to user-observable behavior MUST update the affected checklist
items in the same PR** — add cases for new features, edit changed flows,
delete removed ones, and add an item for any regression that escaped to
production. Keep items phrased as verifiable outcomes and keep the 📱
(mobile/tablet) markers accurate.

Coverage gate: `make go-test` enforces a cross-package minimum (`GO_COVER_MIN`,
default 80). CI surfaces the coverage % in the Actions job summary plus an HTML
artifact (`.github/workflows/go-tests.yml`).

## Configuration

The Go server reads its environment from `.env`. The root `.env.example` is the short
quick-start template (the settings most self-hosters need); `docs/full.env.example` documents
every setting, so a new variable goes there (and into `.env.example` only if most setups need
it). Key vars:

- `DATABASE_URL` — `sqlite:///abs/path/db.sqlite` or `postgres://…`. Selects the engine.
  Required, and sourced from `.env` only — it is NOT baked into the image.
- `PORT` — HTTP listen port (required by the server), from `.env`. The image keeps a
  fallback `PORT=80` (compose maps `8181:80`); for a host `go run`, set a non-privileged
  port (e.g. `8181`) in `.env`.
- Auth tokens are stored in the database (`access_tokens`) — there are no signing keys and
  no token-related configuration. Persist the `/app/var` volume (the db) to keep data and
  logins valid. Leftover `ECONUMO_JWT_*` variables from older builds are ignored.
- `ECONUMO_DATA_SALT` — **Deprecated and IGNORED by the API/repositories**, which always run
  salt-free (plaintext normalized emails, `email` as the lookup key). It is consumed by exactly one
  code path, the `data:remove-salt` migration (below), which reads it to decrypt existing data.
  Set it to your old salt, run that command, then unset it. Until you migrate, a still-salted
  database has unreadable emails, so those users cannot log in (the intended push to migrate);
  `serve` logs a WARN at boot while it is set.
- `ECONUMO_ALLOW_REGISTRATION` — enable/disable self-service sign-up: the register endpoint
  and first-sign-in provisioning through a provider.
- `ECONUMO_PASSWORD_LOGIN` — email + password sign-in (default `true`; strict boolean). `false`
  refuses, with the coded 400 `user.password_login_disabled`, every password flow:
  `login-user`, `register-user`, `remind-password`, `reset-password` (so also the passwordless
  "Set a password"), `update-password`, and the login-time `confirm-email` /
  `resend-verification-code`. The check runs before rate-limit accounting and any lookup.
  Sessions, PATs, provider sign-in and the CLI are unaffected. It requires at least one
  provider slot (boot fails otherwise), and `ECONUMO_ALLOW_REGISTRATION` then governs provider
  sign-up only — together they give provider-only sign-up. The `account_exists_password`
  refusal is unchanged, so accounts with a password must link a provider BEFORE the switch
  (`docs/oidc-setup.md`). While it is off, a stored password no longer counts as a sign-in
  method: `unlink-identity` refuses the last identity with `oauth.last_sign_in_method`. The
  config field is inverted (`PasswordLoginDisabled`) so a zero `config.Config` in tests keeps
  passwords on, and the served config carries it as `PASSWORD_LOGIN`. The SPA hides the
  password forms and Settings' password entries through `passwordLoginAvailable()`
  (`web/src/features/auth/oauthQueries.ts`), which keeps the form whenever the page's backend
  is a custom one on another origin, since `PASSWORD_LOGIN` describes the serving instance.
- `ECONUMO_TRIAL` — trial length in DAYS for a newly self-registered user. `0`
  (default; also `none`/empty) grants no trial, so the user keeps permanent full
  access — the self-hosted default. A positive integer `N` grants full access
  until `N` days after registration, after which they lapse to `readonly`.
  Negative, non-numeric, or the old `end-of-next-month` literal fail at boot.
  CLI-created users never get a trial. See `user:set-access` / `user:show`
  below and the 402 rule in API conventions for how access is enforced afterward.
- `ECONUMO_EMAIL_VERIFICATION` — require newly registered users to confirm an emailed
  code at login before their first session (default `false`; strict boolean, malformed
  fails at boot). The code email is sent at the first blocked login attempt, not at
  registration; `serve` WARNs at boot when enabled with the console mail transport.
  Existing rows and CLI/admin-created users are always verified.
- `ECONUMO_ADMIN_PORT` / `ECONUMO_ADMIN_TOKEN` — the private admin listener the payment
  portal talks to (`POST /admin/set-access`, `GET /admin/user-context`). A **second**
  `http.Server`, started by `serve` only when BOTH are set, so a self-hosted instance
  never serves those routes and they sit on no public mux at all (enforced by
  `TestAdminRoutesAreNotOnThePublicMux`). Auth is `Authorization: Bearer <ECONUMO_ADMIN_TOKEN>`
  compared in constant time; the same token is the HMAC key for billing-handoff tokens
  (minimum 32 characters, and it must be RANDOM — authenticated users see HMAC samples in
  their billing links, so a guessable passphrase is offline-attackable; generate with
  `openssl rand -hex 32`). A half-configured pair fails at boot. Every admin action is
  audit-logged on its operation line: `set-access` carries `user_id`, the written
  `access_level`/`access_until` AND the `old_*` pair (logged even when the write fails, so
  attempts are recorded); `user-context` carries `user_id` + `connections`; CLI
  `user:set-access` emits the same `set-access` line. Ids and levels only — never emails. The port accepts a
  host-qualified value (`127.0.0.1:9090`) to pin the listener to loopback/an internal
  interface on bare-host deployments; a bare port binds all interfaces (the container
  default, where compose controls exposure). Unlike the public API,
  this surface returns a real 404 for an unknown user: its consumer is a machine.
- `ECONUMO_BILLING_URL` — payment portal URL (**https required** except loopback hosts —
  billing links carry signed identity tokens in the query string). Empty (default) means
  `POST /api/v1/user/create-billing-link` returns 400 and the SPA shows no billing UI.
  Merged into the served `econumo-config.js` as `BILLING_URL`, so one variable drives
  both halves. Requires `ECONUMO_ADMIN_TOKEN` (the signing key).
- `ECONUMO_URL` — this instance's public URL. When set, `mailer.WithAppLink` wraps the
  mail transport once at composition time (`server.BuildAPI` and the CLI container) so
  every transactional email ends with the bare URL on its own line; unset (default) leaves
  bodies byte-for-byte unchanged (the wrapper is not installed). Must be an absolute
  http(s) URL — plain http is allowed (unlike `ECONUMO_BILLING_URL`, an app link carries no
  signed token). Not a translatable string, so it touches no `emails.*` catalogue key.
  It is also the issuer of the MCP OAuth server (`internal/authserver`): an `https://` origin
  with no path (plain `http://` only on a loopback host) enables that server; unset or anything else leaves it off.
- `ECONUMO_OAUTH_GOOGLE_*` / `ECONUMO_OAUTH_APPLE_*` / `ECONUMO_OIDC_*` — three independent
  "Sign in with…" provider slots (`internal/oauth`), each **all-or-nothing**: any one variable
  of a slot set without the rest fails at boot naming the missing variable. Google needs
  `ECONUMO_OAUTH_GOOGLE_CLIENT_ID` + `_CLIENT_SECRET`; Apple needs `ECONUMO_OAUTH_APPLE_CLIENT_ID`
  (the Services ID) + `_TEAM_ID` + `_KEY_ID` + `_PRIVATE_KEY_FILE` (a path to the `.p8` file,
  read verbatim at boot; a missing or empty file fails at boot). The key is a file rather than
  an env value because systemd's `EnvironmentFile=` eats the backslash of an unquoted `\n`,
  which silently corrupted the one-line form the removed `_PRIVATE_KEY` variable took; that
  variable is now rejected at boot with a message naming its replacement. The custom OIDC slot needs
  `ECONUMO_OIDC_ISSUER_URL` (absolute `https://`, plain `http` only for loopback hosts) +
  `_CLIENT_ID` + `_CLIENT_SECRET`, plus optional `_NAME` (button label, default `SSO`) and
  `_SCOPES` (comma-separated, default `openid,profile,email`; must contain `openid` or boot
  fails). Any enabled slot requires `ECONUMO_URL` — every callback URI derives from it as
  `<ECONUMO_URL>/api/v1/oauth/callback-<google|apple|oidc>`, the one URL to register with the
  provider; there is no separate redirect-URI variable. Discovery documents are fetched lazily
  and cached for the process lifetime; the JWKS is cached and re-fetched when a token names an
  unknown key id (at most once a minute per issuer), so a signing-key rotation needs no restart.
  Discovered endpoints must be absolute https URLs (plain http only for loopback issuers); a
  document that violates that is rejected.
  `serve` builds the provider clients once,
  probes each configured issuer's discovery document in the background right after the listener
  is up (one 5-second timeout per provider) and logs a WARN (never fails boot) when one is
  unreachable, so sign-in through it fails until the issuer answers.
  `ECONUMO_OIDC_TRUST_EMAIL` (strict bool, default `false`) treats the custom issuer's email
  claim as verified; `false` rejects any token without `email_verified=true` with
  `email_unverified`, for every intent (login, auto-link,
  and linking from Settings) — an issuer that never sends that claim needs the flag set or no
  sign-in through it will ever succeed. Google and Apple are fixed issuers/scopes; Apple is
  always treated as trusted (every Apple ID address is verified) but Google's `email_verified`
  claim is honoured, not trusted blindly — Google always sends it and documents `false` for
  unverified addresses. See `docs/oidc-setup.md`.
- `ECONUMO_CORS_ALLOW_ORIGIN` — comma-separated cross-origin allowlist. Empty (default) = same-domain
  only (no `Access-Control-Allow-Origin` emitted; the bundled SPA and API share an origin so it
  just works). A configured origin is reflected back with `Vary: Origin`; `*` allows any origin.
- `ECONUMO_CURRENCY_BASE` — base currency (default `USD`).
- `ECONUMO_CHECK_UPDATES` — daily check for new releases against `econumo.com/releases/latest.json` (single server-side request; result served to the SPA via `get-update-info`). `false` disables it.
- `ECONUMO_ANALYTICS` — **Deprecated and IGNORED outside the one-time migration**, genuinely parallel
  to `ECONUMO_DATA_SALT` now: read by exactly one migration, safe to delete once that
  migration has run. Product analytics is now a per-user `users_options` preference
  (default on for every new user, toggled in Settings via `update-analytics`), not an
  instance-wide switch, so this variable no longer reaches the SPA or the served
  config at all — it is dropped from the `econumo-config.js` merge entirely (see
  `INSTANCE_ID` below) and no longer influences registration either way. Its one
  consumer: the CLI reads `c.cfg.Analytics` and passes it into
  `migration:seed-analytics-option` (above), the one-time backfill that seeds the
  `analytics` option for users predating it (`false` seeds opted out; anything else,
  including unset, seeds opted in). Once that migration has run on an instance, the
  variable does nothing — the runner records the version and never reruns it — so it
  can be removed from the environment. `ANALYTICS` itself carries no served
  config key at all (see the "Web UI config" bullet below for how the rest of
  `econumo-config.js` — `ALLOW_CUSTOM_API`, `INSTANCE_ID`, etc. — is generated).
- `MAILER_DSN` — mail transport for password-reset email; the scheme selects the provider, exactly
  as `DATABASE_URL`'s scheme selects the DB engine. Empty (default) = the **console** transport (renders
  each email to stdout — a dev aid that never silently drops mail); `resend://<api_key>` sends via Resend;
  `smtp://[user:pass@]host[:port]` (default 587, STARTTLS REQUIRED — a relay that does not offer it
  is refused rather than sent a reset code in the clear) and `smtps://…` (implicit TLS, default 465)
  send through any SMTP server (`internal/infra/mailer/smtp.go`, stdlib `net/smtp`, AUTH PLAIN or
  LOGIN, quoted-printable UTF-8 text). SMTP requires `?from=` (the envelope sender). A loopback host
  may skip TLS and is not certificate-checked; `?insecure=true` extends that to any host (a relay on
  a trusted Docker network). Parse errors never echo the DSN, which carries the password.
  From / Reply-To fold in as query params: `resend://<key>?from=…&reply_to=…` (also accepted by
  `console://`/`log://`). Parsed once in `config.Load` (a bad scheme fails at boot). Replaces the former
  `RESEND_API_KEY` / `ECONUMO_MAIL_FROM` / `ECONUMO_MAIL_REPLY_TO`.
- `OPEN_EXCHANGE_RATES_TOKEN` — currency-rate updates.
- `ECONUMO_CURRENCY_UPDATE_INTERVAL` — refresh exchange rates in-process every N
  DAYS. `0` (default/unset) = off (drive `currency:update-rates` from an external
  cron as before). A positive `N` starts a background poller in `serve` **only
  when `OPEN_EXCHANGE_RATES_TOKEN` is also set** (interval-without-token logs a
  WARN at boot and stays off). Valid range is `1`-`31`; negative/malformed/over-31
  fails at boot. The poller
  refreshes on boot then every N days and is DB-aware: it skips the fetch while
  the newest stored rate is within N days, so a restart loop never burns API
  quota. Idempotent per `(date, currency, base)`, so it is safe alongside an
  existing external cron.
- `ECONUMO_IMPORT_SIMPLEFIN` — the SimpleFIN pull provider (strict boolean, default `true`;
  malformed fails at boot; the config field is inverted, `SimpleFINDisabled`, so a zero
  `config.Config` keeps it on). Connecting needs a secure page (https or localhost): the browser
  encrypts the access URL with WebCrypto, so on a plain-http page the SimpleFIN page shows only an
  explanation (`window.isSecureContext`) and Settings → Data offers no sync. Apple Wallet import has no switch and is always on (the former
  `ECONUMO_IMPORT_APPLE_WALLET` is ignored), so `internal/imports/api` is always mounted with
  import rules, the queue and run history. While SimpleFIN is off it loses its own routes
  (`claim-setup-token`, `get-credential-key`, `set-credential-key`, `list-external-accounts`,
  `sync-source`), `create-source` refuses it with `import.provider_unsupported`, and its existing
  sources and their queued/failed rows drop out of `get-source-list` and `get-queued-event-list`
  (`imports.Service.DisableProvider`; stored data is untouched). The SPA reads the served
  `IMPORT_SIMPLEFIN` key for SimpleFIN's settings pages and routes; Apple Wallet, Import rules,
  import history and the Inbox's import sections are always shown (there is no
  `IMPORT_APPLE_WALLET` key any more). CSV import/export is not part of it and is always on.
  The apiparity harness relies on the default (on) so the catalogue keeps covering its routes.
- `ECONUMO_FEEDBACK` — the in-app "Send feedback" form (strict boolean, default `true`, malformed
  fails at boot; the config field is inverted, `FeedbackDisabled`, so a zero `config.Config` keeps
  it on). Served as `FEEDBACK`; `false` hides its entry points (the Settings row and Reports'
  "Suggest a report" card). The form (`web/src/features/feedback`, opened through
  `useUiStore.openFeedback(type, topic)`) posts `type`, a hidden `topic` (where it was opened:
  `general` from Settings, `reports` from Reports), `message`, an optional `email` (empty by
  default, the user's own offered as a one-tap chip), `app_version` and `hosting` (`cloud`/`self-hosted`) from the
  BROWSER straight to the Twillingate form `feedback` of the product-analytics project
  (`submitForm` in `web/src/lib/analytics.ts`, the SDK's wire shape without the SDK), so it
  works with analytics off and carries no `$user_id` — only the synthetic `$host`.
- `ECONUMO_IMPORT_MATCH_DAYS` / `ECONUMO_IMPORT_TIP_DAYS` / `ECONUMO_IMPORT_TIP_TOLERANCE` /
  `ECONUMO_IMPORT_TOKEN_MIN_LENGTH` — transaction-import matcher thresholds (defaults 3 / 5 / 20 / 3;
  ranges 0–31 days, 0–31 days, 0–100 percent, 1–16 chars). Strict parse: malformed or out-of-range
  fails at boot. Read into `imports.MatcherConfig`; the matcher itself is a pure function of
  `(event, candidates, config)`.
- `ECONUMO_IMPORT_ALLOW_PRIVATE_HOSTS` — lift the SimpleFIN client's SSRF guard (strict
  boolean, default `false`, malformed fails at boot). With the guard on, the client resolves
  every bridge host and refuses any address that is not global unicast (loopback, RFC1918,
  link-local, unique-local, multicast, unspecified) — the setup token and the access URL are
  user-supplied, so without it the server fetches whatever its own network can reach. A
  self-hosted bridge on a LAN needs it on. The check runs on the address the client dials,
  so behind an `HTTPS_PROXY` it inspects the proxy, not the bridge: a private proxy needs
  the guard lifted, and a public proxy resolves the bridge host itself, outside the guard.
- `ECONUMO_AI_DSN` — `openai://<api-key>@<host>[:port][/prefix]?model=<model>` configures the
  AI client integration. Its consumers today are onboarding's background jobs (see the Day 0
  paragraph): when onboarding creates the budget or adds accounts to it, a job sends those
  accounts' names and their own currencies, plus the user's timezone and UI language, to the
  configured provider for savings detection; after every CSV import, two separate requests go out for
  what that import created (category names and types for category icons, the names and currencies
  of accounts still on the import icon for account icons), and after `finish-import` one more with
  every live account's name and currency for savings; import rule suggestions
  send the new transactions' bank payee text, income/expense, account name and currency code with
  the account owner's category names of the rows' type, language and timezone, then, for
  rows whose merchant resembles some of the owner's payees, those rows' payee text, the merchant
  name and up to 5 candidate payee names each (never the whole payee list), and `suggest-match-text`
  sends one transaction's payee text (see the imports paragraph). **Data allowlist, every AI
  prompt:** category names and types, payee, tag and label names, account names, currency codes,
  the language, the timezone, a transaction's type and the bank's payee/merchant text, plus the
  opaque keys and entity ids the answer refers to. NEVER amounts, balances, opening balances,
  plans, dates, a transaction's description/memo, user names or emails; a row with no payee text
  is not sent. Each prompt builder has a guard test asserting its exact key set.
  A user who turned the per-user `ai` option off (`update-ai`) sends nothing: no onboarding
  job starts for them (an unreadable preference starts none either), and each onboarding request
  re-reads the option right before its provider call, so a job queued before the opt-out sends
  nothing either; the suggestion job skips
  every account they OWN (the owner decides, also for a partner's taps on it),
  and it re-reads the option right before the AI call and again in the write's transaction, so
  turning AI off mid-job sends or stores nothing (and once more after that transaction commits,
  removing what it just stored when an opt-out committed alongside it unseen, which PostgreSQL's
  read committed allows); `suggest-match-text` answers empty without
  counting the limiter. Turning the option off deletes the user's open suggestions in the same
  request (best effort, through the user feature's `AISuggestionClearer` port, implemented by
  the imports service and wired in `internal/server`), and `get-suggestion-list` also answers
  empty and deletes them on any read while it is off (the imports feature reads the option
  through its `OwnerFacts` port).
  Turning it back on affects only new imports. The SPA hides suggestions and skips
  `suggest-match-text` for such a user.
  The key is optional for local servers (`openai://localhost:11434?model=llama3.1`).
  Unset (default) leaves the provider disabled. A bad scheme/missing host/missing `model`
  fails at boot.
  `anthropic://<api-key>@<host>[:port][/prefix]?model=<model>` uses Anthropic's native
  Messages API instead (e.g. `anthropic://<api-key>@api.anthropic.com?model=claude-sonnet-4-6`).
  It posts to `/messages` under the base prefix (default `/v1`), sends `x-api-key` and
  `anthropic-version: 2023-06-01`, and uses an 8192-token output budget. Custom prefixes,
  keyless local gateways, loopback HTTP, and `?insecure=true` work the same way as for OpenAI.
  Text blocks are combined; incomplete or refused replies return an error.
  `econumo://<gateway-host>[:port][/prefix]?model=<model>` routes through an econumo AI gateway
  instead. It carries no key: each call's bearer is a 5-minute `econumo-ai:v1` token for the
  requesting user, signed with `ECONUMO_ADMIN_TOKEN` (required with this scheme; boot fails
  without it, and a key in the DSN fails boot too). The token only exists as half of the admin
  pair, so this scheme also opens the admin listener, exactly like `ECONUMO_BILLING_URL`; when
  only signing is wanted, pin it to loopback (`ECONUMO_ADMIN_PORT=127.0.0.1:9090`) so nothing
  else can reach it. `?insecure=true` works for `econumo://` too
  and sends the signed token over plain http; use it only on a trusted internal network, for
  example when econumo reaches the gateway over a private Docker network.
- `SQLITE_BUSY_TIMEOUT` — SQLite `busy_timeout` PRAGMA in ms (default `0`); bare name mirrors the engine pragma.
- `ECONUMO_RATE_LIMIT_LOGIN` / `ECONUMO_RATE_LIMIT_RESET` / `ECONUMO_RATE_LIMIT_REMIND` /
  `ECONUMO_RATE_LIMIT_REGISTER` — brute-force protection for the public auth endpoints:
  max attempts per username/email per window (defaults 5/5/3/5; login and reset count
  only FAILED attempts and clear on success, remind and register count every request). A successful
  login clears only the caller's own per-address-and-IP counter, never the per-address ceiling: two
  SQLite accounts can differ only by the case of a non-ASCII letter and share that key.
  `ECONUMO_RATE_LIMIT_ACCEPT_INVITE` — cap on `connection/accept-invite` attempts per
  authenticated user per window (default `10`; every attempt counts), guarding the short
  invite code against online brute force.
  `ECONUMO_RATE_LIMIT_VERIFY_EMAIL` — verification-code emails per username per window (default `3`; every send counts).
  `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL` — failed confirm-email attempts per username per window (default `5`; cleared on success).
  `ECONUMO_RATE_LIMIT_REQUEST_EMAIL_CHANGE` — change-email code sends per user per window (default `3`; every send counts).
  `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL_CHANGE` — failed confirm-email-change attempts per user per window (default `5`; cleared on success).
  `ECONUMO_RATE_LIMIT_INGEST` — `import/ingest-apple-wallet-event` pushes per user per window (default `60`; every request counts).
  `ECONUMO_RATE_LIMIT_CLAIM_SETUP_TOKEN` — `import/claim-setup-token` calls per user per window (default `5`; every request counts).
  `ECONUMO_RATE_LIMIT_SYNC` — `import/sync-source` calls per user per window (default `10`; every request counts — a sync is a real bridge round trip).
  `ECONUMO_RATE_LIMIT_PREVIEW_RULE` — `import/preview-rule` calls per user per window (default `120`; every request counts — the rule editors fire it from a 300 ms typing debounce, and each call walks every matching link).
  `ECONUMO_RATE_LIMIT_WINDOW` — sliding window (Go duration, default `15m`).
  `ECONUMO_RATE_LIMIT_IP` — per-endpoint cap per minute per client IP (default `60`), replacing the
  former cap shared by all callers (`ECONUMO_RATE_LIMIT_GLOBAL`, now ignored with a boot WARN), which
  any one caller could fill to lock everyone out. **No limit may be shared by unrelated callers.**
  `ingest` and `preview-rule` are exempt (`IPExempt` in `internal/server/server.go`): one user's own
  limit could fill the cap for a household behind one IP, and their effects stay inside that account.
  The limits keyed by an email address (`login`, `reset`, `remind`, `register`, `verify-email`,
  `confirm-email`) count per address AND client IP (`allowEmailAttempt`), so a stranger locks out
  only their own IP, under a per-address ceiling across all IPs: 10x the limit, but 2x for `reset`
  and `confirm-email`, because a 6-digit code must not get many more guesses. Enough IPs can still
  reach a ceiling and lock that one address. The `login`, `reset` and `confirm-email` ceilings of real
  accounts and the per-user `accept-invite` and `confirm-email-change` counters are `Pinned` (never
  evicted before they expire), so a spray of invented addresses from many IPs cannot wipe them from
  the 10,000-entry counter map. Addresses with no account count in a separate evictable ceiling
  that answers 429 at the same point, so the limit does not reveal which accounts exist, unless a
  spray happens to evict it first. Accepted residual: many accounts on many IPs have no
  overall ceiling on `accept-invite`, `request-email-change`, `claim-setup-token` and `sync-source`.
  `ECONUMO_CLIENT_IP_HEADER` — the header your reverse proxy puts the client IP in (e.g.
  `CF-Connecting-IP`); a request without a valid value counts as its connecting address. Alone, it
  is believed from anyone, which is safe only when a firewall lets nothing but the proxy reach the
  server: a direct caller could otherwise forge it, dodging the limits or spending another IP's.
  `ECONUMO_CLIENT_IP_TRUSTED_PROXIES` — optional comma-separated IPs/CIDRs of those proxies (for
  Cloudflare, https://www.cloudflare.com/ips/ plus the `cloudflared` address when using a tunnel).
  With it the header counts only on a request from a listed address (default header
  `X-Forwarded-For`), so direct and proxied traffic can share one instance, and a list is walked
  from the nearest entry to the first address outside the proxies. Neither set: the client IP is the
  connecting address, so behind a proxy all its users count as one IP (logged once as a WARN when
  a forwarding header arrives). Resolved in
  `middleware.ClientIP`; IPv6 is grouped by /64; the IP stays in memory and is never logged.
  `0` on a count disables that check (the window must be positive). Over-limit requests get HTTP 429 with the standard error envelope
  (message `"Too many attempts. Try again later."`, frozen). State is in-memory (resets on
  restart); a malformed value fails at boot.
- **Web UI config** — the SPA is ALWAYS embedded in the binary (`web/embed.go`,
  `//go:embed all:dist`); there is no disk-serving mode. For a server-served
  instance, `internal/web/router` builds the COMPLETE `window.econumoConfig`
  document in Go — every key, with its default filled in when the environment
  does not override it — and `internal/web/spa` writes that document verbatim
  as the served `econumo-config.js`; the embedded dist file
  (`web/public/econumo-config.js`) is never read in this path. One rule: the
  backend value overwrites the default when present. Most keys map to
  `ECONUMO_<KEY>`: `ECONUMO_ALLOW_CUSTOM_API` (default `true`),
  `ECONUMO_LILTAG_CONFIG_URL` (default `/liltag-config.json`; load liltag
  config from a URL instead of the bundled `liltag-config.json`),
  `ECONUMO_LILTAG_CACHE_TTL` (default the JS number `0`), and `ECONUMO_VERSION`
  (`VERSION_LABEL`, the version text the UI DISPLAYS — set it to relabel a
  demo/staging box; unset falls back to `VERSION`). `VERSION_REF` (always present, `""` when the build carries no
  revision) has no env override either. `VERSION` itself has NO env
  override: it always carries the binary's `internal/version.Version`, because
  the SPA compares it as a real version (analytics `$app_version`, the update
  check), and a display label must never
  reach those. `ANALYTICS` no longer exists as a config key — analytics is a
  per-user preference now, not instance-wide (see `ECONUMO_ANALYTICS` above).
  `INSTANCE_ID` is the one key with no matching env var: it carries the
  per-deployment digest (`internal/infra/instance`, resolved against the
  migrated database), defaulting to `""` when unresolved; `migrate.Run` always
  runs before `server.Build` (`cmd/econumo/main.go`), so `schema_migrations` is
  already populated and a real id is present from the very first boot.
  `ALLOW_REGISTRATION`, `PASSWORD_LOGIN`, `BILLING_URL`, `MCP_URL`,
  `IMPORT_SIMPLEFIN` and `FEEDBACK` are always present (server truth). `MCP_URL` is the MCP OAuth resource
  URL (`<ECONUMO_URL>/mcp`), `""` while that server is disabled; the SPA shows Connected
  apps (Profile row, copyable address) only when it is non-empty. On the web it describes the
  instance serving the page, so `mcpUrl()` returns `""` when the SPA is pointed at a custom
  backend on another origin.
  `IMPORT_MATCHER` (`{matchDays, tipDays, tipTolerancePct, tokenMinLength}`, the
  effective `ECONUMO_IMPORT_*` values) is always present (typed on
  `EconumoConfig`, not consumed by any surface yet). `AI_ENABLED` (bool,
  always present, `true` iff `ECONUMO_AI_DSN` is set) advertises the AI
  provider configuration; the import rule prompt reads it to ask for an AI match text.
  The composition root resolves the FS
  (`web.DistFS`), both versions, and instance id once in `server.BuildAPI`.
  `web/public/econumo-config.js` itself is a fallback baseline, not a source
  of defaults for a served instance — it only matters for `pnpm dev` without a
  backend running (see that file's
  header comment); mounting a replacement file over it in a container has no
  effect (there is no disk-serving mode to read it from — use the `ECONUMO_*`
  variables above instead).
- `ECONUMO_LOG_LEVEL` — base slog level `debug|info|warn|error` (default `info`). Every command
  (`serve` and all resource:action commands) also accepts `-v`/`-vv`/`-vvv` (force DEBUG; `-vvv` adds source)
  and `-q` (quiet); flags override `ECONUMO_LOG_LEVEL`. Resolution lives in `internal/logging`.

  > **Env naming convention:** app-owned config is prefixed `ECONUMO_`; bare names are reserved for
  > ecosystem standards (`PORT`, `DATABASE_URL`, `MAILER_DSN`) and names the engine/vendor owns
  > (`SQLITE_BUSY_TIMEOUT`, `OPEN_EXCHANGE_RATES_TOKEN`).
- `X-Timezone` request header — the caller's IANA timezone, used for day-boundary math
  (e.g. an account's "balance as of end of today"); the tz database is embedded in the binary.
  A `users.timezone` column opportunistically persists it: a decorator around
  `middleware.TokenAuthenticator` (wired once in `server.BuildAPI`) writes the header's
  value on every authenticated request where it differs from the last value seen for that
  user (throttled by an in-memory per-user cache, same pattern as the token
  `last_used_at` throttle). MCP clients send no header, so `/mcp` (only) falls back to the
  stored value when none is present; the header always wins when it is present, on both
  edges — REST behavior is unchanged.
- Error `message`/`errors` text is rendered in the caller's language on BOTH edges:
  `httpx.WriteError` (REST) and `internal/web/mcp/helpers.go` (`MapErr`) resolve
  `reqctx.Language(ctx)` and render any coded message/field message via
  `i18n.T(lang, "errors."+code, params)`; code-less errors keep their literal English.
  The language follows the same fallback pattern as the timezone above, but on both
  edges: explicit `Accept-Language` header → stored `users.language` → `en`. The stored
  fallback runs inside the auth middleware via the optional
  `middleware.StoredLanguageResolver` capability, implemented by the server's
  authenticator decorator (`StoredLanguage`, `internal/server/glue_language.go`;
  `reqctx.IsLanguageExplicit` mirrors `IsLocationExplicit`).

### Logging

Two tiers of structured `log/slog` output, both with **static messages** and details as
fields (custom dimensions) — UUIDs only, never PII (no emails, bodies, or query strings):

- **Operation result** — one line per request, message = the static operation name (e.g.
  `create-category`), at INFO (2xx) / WARN (4xx) / ERROR (5xx & recovered panics). Always
  carries `request_id`, `status`, `route`, the user dimensions `user_id` + `timezone`, and
  `err`/`err_type` on failure.
- **Edge/transport** — a DEBUG `"http request"` line with `method`, `route`, `status`,
  `duration_ms`.

`request_id` is a UUIDv7 minted in the `RequestID` middleware and echoed on `X-Request-Id`.
The `AccessLog` middleware (`internal/web/middleware/accesslog.go`) installs a request-scoped
accumulator (`reqctx.WithLogAttrs`) and emits both lines; any layer enriches the operation
line with operation-specific params via `reqctx.AddLogAttr(ctx, key, value)` (e.g.
`category_id`). `/health` logs the transport line only; `OPTIONS` preflight is skipped.

## Database

- **SQLite** (default): pure-Go `modernc.org/sqlite` driver (CGO off).
- **PostgreSQL**: `jackc/pgx/v5` (stdlib), simple protocol (PgBouncer-safe).
- Migrations live in `internal/infra/storage/migrations/{sqlite,pgsql}` and run on boot.
- **SQLite datetime storage contract.** DATETIME/TIMESTAMP columns hold bare
  `Y-m-d H:i:s` UTC TEXT (19 characters; PostgreSQL's `TIMESTAMP(0)` is the
  same wall clock). Every SQLite connection is opened with
  `_time_format=datetime&_timezone=UTC` (`sqlite.WithFrozenTimeFormat`, applied
  by `Backend.Open` and therefore by `dbtest`), so a bound `time.Time` is
  converted to UTC and stored in that layout, and stored text parses back as
  UTC. Rows written before this setting held `time.Time.String()` text; the
  command migration `migration:normalize-sqlite-datetimes` (boot step
  `20260915000001`) rewrites them. `currencies_rates.published_at` is the one
  DATE column and is `Y-m-d`: its write query binds that text explicitly
  because the driver would store a time part and split one day into two keys
  of the textual unique index. The hand-built budget/account SQL still binds
  its range bounds as `datetime.Layout` strings (`sqliteDatetime`,
  `limitPeriodArg`) even though a bound `time.Time` now yields the same bytes:
  that is deliberate belt-and-braces so those comparisons never depend on a
  DSN parameter, not a sign that the driver still needs it.
- **`idempotency_keys`** (migration `20261109000000`, both engines): one row per
  `(user_id, idempotency_key)` with a nullable `resource_id` and `created_at`, no foreign keys
  because the rows are short-lived. `internal/infra/idempotency` owns it, and the middleware
  writes it (see "Idempotency-Key" under API conventions). Rows older than 90 days are purged
  after a successful guarded request, at most every 10 minutes per process, set-based and
  best effort.
- **Transaction revisions.** `accounts.transactions_revision` is a per-account counter the
  SPA's transaction sync compares to know which accounts to re-download. It moves by exactly
  +1 per committed DB transaction that changes anything the account's downloaded transaction
  list carries (rows, label links, recurring links, the classification ids on rows, an
  author's name or avatar), however many rows or writes that took. The bump lives INSIDE the repository method that
  does the write, through `internal/infra/txrevision` (`Bumper.Bump(ctx, accountIDs...)`;
  `BumpWhere(ctx, cond, args...)` selects the affected accounts from `transactions`, so it
  runs BEFORE the matching rows change or are deleted). Bumps are collected per DB
  transaction (`backend.TxManager.BeforeCommit` / `TxValue`) and flushed once just before
  COMMIT as one UPDATE per account in sorted id order (a stable lock order, so concurrent
  writers cannot deadlock on it); outside a transaction they apply immediately. Sites:
  the transaction repo (`Save`, `ReplaceLabels`, `LinkRecurring`, `Delete`, `SetSideKey`), account
  `SaveCorrection`, category/payee/tag merge and delete, label merge and delete, recurring
  delete, imports `InsertLink` / `UpdateLink` (only when its `transaction_id` changes) /
  `DeleteSource`, and the user `Save` when the name or avatar changes. The account DTO
  carries it as `transactionsRevision` (additive), next to `transactionsCount`, how many rows
  `get-account-transaction-list` returns for the account (a size hint for packing downloads,
  read from the transactions indexes alone, a transfer counted on both sides). `internal/test/revisionguard` enforces
  the rule (see Testing).
- **Transaction order.** Within a day, transactions are ordered by fractional keys
  (`internal/shared/sortkey`, the scheme categories, tags, payees, accounts and budget
  elements use), never by the time of day: `transactions.sort_key` places the row on its
  source account, `recipient_sort_key` (transfers only, else NULL) on its recipient account,
  so reordering one account never moves a transfer on the other. A higher key is listed
  higher. Order everywhere is day DESC, the side key of the listed account DESC (`sort_key`
  for a list spanning several accounts), then `id`, applied by the database: the
  transaction repo's queries (`ListTransactionsByAccount`, `ListByAccountIDs`), `dayKeyOrder`
  in the budget drill-down SQL; in the SPA `compareTransactions`
  (`web/src/features/transactions/order.ts`) in the SPA. Keys are assigned in the storage
  layer only: the transaction repo's `write` gives a row with no key on a side the top of its
  day there (`internal/infra/txorder`, `Placer.Top`: just above the highest key either column
  holds for that account and day), the account repo does the same for corrections, and
  `model.Transaction.Update` drops a side's key when the day or that side's account changes.
  New rows therefore land on top of their day; the CSV importer inserts bottom-up so a day's
  rows keep file order. `POST /api/v1/transaction/move-transaction` (the account page's drag
  and drop) takes `aboveId`, the row now directly above the moved one on that account (null =
  top of the day), and writes ONE key, between that row and the one below it; nothing else
  is renumbered, except rows whose keys tie (two inserts that raced), which are split first.
  An `aboveId` no longer on that day is refused with `transaction.day_changed`; a different
  `date` also moves the row to that day (time kept), which needs write access to every
  account it touches (`transaction.cannot_move`). `revisionguard` also fails any write that
  leaves a row without its keys.
- After changing a query: edit `query/{sqlite,pgsql}/*.sql` and regenerate with
  `sqlc generate` (config at `internal/infra/storage/sqlc/sqlc.yaml`).
- Keep `.sql` query files ASCII-only, including comments — a multibyte character (an em dash,
  a curly quote) in a comment silently truncates the generated SQL constant, and the failure
  surfaces at runtime as `SQL logic error: incomplete input`, not at `sqlc generate`.
- Migrations may also be **command steps** (`migrations.RegisterCommand(version, "migration:<slug>")`):
  the boot runner invokes the named CLI command in version order between SQL files, records the
  version only on success, and gives it no surrounding transaction — every `migration:*` command
  must be idempotent. `serve` and `data:import-sqlite` pass `cli.MigrationCommandRunner`; test
  databases pass `migrate.NoCommands`. A command step runs TODAY's feature code against the schema
  as it stands at its version, so on a database from an older release it must not write through a
  path that needs a column a later migration adds: number it above every migration its code
  depends on, and when a later migration changes what it writes, move the step to a new version and
  leave a `SELECT 1;` no-op at the old one (as `20260817000001` -> `20261101000000`).

## CLI / management commands

The binary is subcommand-driven (`cmd/econumo/main.go`): `serve` runs the server,
`healthcheck` probes a running one, and everything else routes to `internal/cli`:

```
user:create <name> <email> <password>
user:change-email <old> <new>
user:change-password <email> <password>
user:activate <email>
user:deactivate <email>
user:verify-email <email>
user:set-access <email> <full|readonly> [YYYY-MM-DD]
user:show <email>
currency:update-rates [date]
currency:show-rate-gaps [start] [end] [--daily]
currency:backfill-rates <start> [end] [--daily]
currency:add <code> [name] [fraction-digits]
token:purge [days]
data:remove-salt
data:import-sqlite [--force] <sqlite-path>
migration:zero-deleted-accounts
migration:seed-analytics-option
migration:normalize-sqlite-datetimes
migration:seed-transaction-sort-keys
migration:copy-access-tokens
migration:normalize-emails
app:register <name> <redirect-uri>... [--public]
app:list [--global] [--user]
app:show <id>
app:update <id> [--name <name>] [--redirect-uri <uri>]...
app:make-global <id>
app:rotate-secret <id>
app:disconnect <id> [--user <user-id>]
app:delete <id>
```

The `app:*` commands manage the MCP OAuth server's clients (`internal/authserver/apps.go`,
`internal/cli/app_commands.go`). `app:register` creates a **global** app (`is_global`, no
owner, open to every user, never purged; users see it as "Official"), validated like `/oauth/register`; it is confidential
unless `--public`, and its client secret (like `app:rotate-secret`'s, which invalidates the old
one at once) is printed once to stdout and never logged. A **user** app is one a client
registered for itself; `app:list` shows `N (M from other users)` when users other than its
owner hold live connections (grants from before ownership; an app with no owner yet shows just
`N`), and `app:make-global` converts
such a shared client, one way. `app:show` lists each live connection with its user id, scope
(`mcp`/`read`), connected and last-used times. `app:update` changes global apps only and refuses a call with
nothing to change. `app:disconnect` revokes live grants (one user's with `--user`) and their
access tokens and drops outstanding authorization codes, under the users' row locks and then the
client row lock, starting over (at most 5 times) when a grant for a user it had not locked
appears; `app:delete` does the same, then deletes the client (tokens first, since
`access_tokens.grant_id` has no FK). Running servers notice at once: tokens are read from the
database per request. The commands work with the OAuth server disabled; `app:register` then
warns on stderr. Flags may sit anywhere among the positionals; audit lines carry ids only.

`data:remove-salt` is a one-off migration that decrypts every user's email
back to plaintext, so `ECONUMO_DATA_SALT` can be removed. `email` is the
unique lookup key (unique index `users_email_uniq`), so a
still-salted instance MUST run this command after upgrading, before that index
is relied on — otherwise emails stay encrypted and those users can't log in
(they are already login-broken pre-migration, so this is not a regression).
Run it **while the old salt is still set** (it needs it to decrypt), then
unset `ECONUMO_DATA_SALT` and restart. It refuses to run with an empty salt,
and is idempotent (already-plaintext rows are skipped).
Back up the DB first — the decryption is one-way in practice.

`currency:add` takes a 3- or 4-letter code: global codes may be 4 letters because
Open Exchange Rates publishes some digital currencies that way (DASH, DOGE), and
the rate loader always sends `show_alternative=1` so those (ETH, LTC, XMR, ...)
get rates like any ISO currency. Users' own custom currencies stay at 3 letters.

`currency:show-rate-gaps` lists the months (per day with `--daily`) in which a
global currency in use (on an account, deleted ones included, a budget, a budget
element or a user's display currency; never the base or a custom currency) has
no rate against `ECONUMO_CURRENCY_BASE`. Conversions average each month's rates
and a month without any borrows the latest earlier month, so one rate a month is
enough for them. Dates are `YYYY-MM-DD` or `YYYY-MM`; the start defaults to the
oldest transaction (today when it is future-dated) and the end to today (UTC, a
later end is clamped).
`currency:backfill-rates` loads exactly those gaps from Open Exchange Rates, one
request per gap, dated the gap's first day (the 1st of the month, or the day),
requesting every global currency like `currency:update-rates` (the in-process
updater skips while the newest stored date is recent, so a backfilled date must
be complete). Its start is required because every gap
costs API quota; it stops at the first failed request, and a rerun resumes.

`data:import-sqlite` copies every table from an existing SQLite database into
the configured PostgreSQL (`DATABASE_URL` must be a `postgres://` URL). It runs
the PostgreSQL migrations on the target first (so a bare `createdb` works in one
step), then copies all app data in one transaction — `access_tokens` and
`currencies` included, the dead `messenger_messages` and the migration
bookkeeping tables excluded. It aborts if the target already holds data unless
`--force` is given, which truncates and replaces. The source SQLite must
already be on the current schema — boot the current econumo binary against it
once before importing — and the command refuses with a clear error on a
schema-version mismatch between source and target.

`migration:zero-deleted-accounts` writes a "Balance adjustment (account
deleted)" correction so every deleted account's balance is zero; idempotent;
also invoked automatically at boot as migration step `20261101000000` (it ran at
`20260817000001` until that broke upgrades from v1.3.x, see command steps above).

`migration:seed-analytics-option` backfills the per-user `analytics`
`users_options` row for every user that has none, seeded from the deprecated
`ECONUMO_ANALYTICS` value (above); idempotent, and invoked automatically at
boot as migration step `20260903000000`.

`migration:normalize-sqlite-datetimes` rewrites SQLite DATETIME/TIMESTAMP
values stored in Go's `time.Time.String()` form (what the driver wrote before
every connection was opened with `_time_format=datetime`, see
`sqlite.WithFrozenTimeFormat`) to `Y-m-d H:i:s` UTC. It discovers columns from
the live schema, skips `schema_migrations` (the instance id's anchor), parses
each value with the exact driver layout and leaves anything unparseable as
stored; idempotent, a no-op on PostgreSQL, and invoked automatically at boot as
migration step `20260915000001`.

`migration:seed-transaction-sort-keys` gives every transaction without one a sort key on
each account side (see "Transaction order"), numbering each account's days in the order the
lists used before keys existed (spent_at DESC, id), the rows it is the source of and the
transfers it receives together. One account per DB transaction, with that account's revision
bumped; idempotent (only rows with no key are touched), and invoked automatically at boot as
migration step `20261024000001`, right after the SQL file that adds the columns.

`migration:normalize-emails` rewrites stored addresses to `model.NormalizeEmail` (boot step
`20261103000001`, right after the SQL file that swaps the `lower(email)` index for a unique one on
`email`). This changes the spelling users see, and the original case is not kept. Addresses that
normalize alike could already exist, since SQLite's `lower()` folds ASCII only: an address already
normalized keeps its place, then the oldest account takes the normalized form, and the others keep
their spelling, each logged once at WARN by user id, because their data cannot be merged and failing
the boot would take the instance down. Such an account signs in by its exact spelling (lookups try it
before the normalized form), a save leaves its spelling alone, and it still shares rate-limit counters
with the other account until the operator moves it with `user:change-email`. Idempotent, and rerunning
it by hand normalizes such an account once the other has moved away. Rows with the shape of ciphertext
(`auth.LooksEncrypted`) are skipped, because lowercasing ciphertext would destroy it; `data:remove-salt` stores
what it decrypts normalized by the same rule, and refuses, naming both user ids, when two accounts
decrypt to byte-identical addresses.

`migration:copy-access-tokens` is step 2 of the three-step `access_tokens` repair
(`20261031000010` creates `access_tokens_new` in the final shape, `20261031000011` runs
this command, `20261031000012` drops the old table and renames the new one; sqlite only,
the pgsql files are no-ops that keep the version sets equal; `20261031000000`-`02` stay
unused because unreleased builds of this fix recorded them for different steps).
`provider`/`id_token` go missing on one upgrade path: `20260907000000` (v1.5.0) adds them,
while the lower-numbered `20260901000000` (v1.6.0) was released later and rebuilds the
table to add a NOT NULL `scope` without carrying them over, so a v1.5.x instance applies
that still-pending rebuild afterwards and loses them, and login then fails with `no such
column: provider` because minting a session writes both. The copy is a command because SQL
cannot name a column that may be missing: it copies `provider`/`id_token` where the old
table still has them (fresh installs, anything upgraded from v1.6.x) and writes NULL where
they were dropped (those values are unrecoverable, so such a session skips the
RP-initiated logout redirect); every other column, `scope` included, is copied verbatim.
Idempotent: it replaces the copy in one transaction, and is a no-op once the swap has
removed `access_tokens_new`. Step 1 drops a stale `access_tokens_new` before creating it.
Step 3 only drops the old table, renames the new one and recreates the indexes (their
names are taken until the old table is gone), so a write to `access_tokens` between steps
2 and 3 would be lost; in a boot the two run back to back. The runner also logs a WARN
when it applies any migration numbered below the highest already applied (the ordering
that caused this), excluding anything at or below the newest legacy drop-in version.

In the distroless image these run via the binary directly, e.g.
`docker exec <container> /app/econumo user:create …`.

## API conventions

- **Methods — only two.** `GET` for reads; `POST` for every write — create, update,
  AND delete. There is no `PUT`/`PATCH`/`DELETE`; deletes are POSTs. One exception:
  `/api/v1/import/list-external-accounts` is a POST *read*, because the bridge access URL
  travels in the body — it must never reach a query string (logs, history, referrers).
- **`Idempotency-Key`** (`internal/web/middleware/idempotency.go`): the SPA's API client sends a
  UUIDv7 on every `POST` (one it mints, the offline queue's own, reused on every attempt, or for an
  add the form's id, the same for a retap and a restored draft). The
  server guards authenticated `POST`s of a full-scope token outside `/mcp`; public routes have no
  user. An absent or unparsable key means no guard, so MCP, other clients and the parity
  catalogues are unchanged. The check reads the table BEFORE the handler, so a replay is refused
  ahead of any lookup or access check (the replay of a delete that landed would otherwise answer
  "not found"). The key is written inside the request's own DB transaction through the
  `BeforeCommit` hook, or right after a 2xx for a handler that opens none. A non-2xx releases it so
  the operation can be sent again, unless the handler pinned it (`reqctx.PinIdempotency`, called
  inside the transaction of `create-transaction`, `update-transaction`, `delete-transaction` and
  `set-limit`): a pinned key stays once it committed, so a failure or a dropped client after COMMIT
  makes the retry a replay rather than a duplicate. A replay answers **HTTP 409** with the handled-error
  envelope, the `common.operation_locked` message and an additive `resourceId` when one was
  stored (only create-transaction stores one today). A concurrent duplicate loses on the primary
  key and gets a retryable 5xx with the envelope, not a 409, since the first attempt may still
  fail. `id` on `create-transaction` is optional: the SPA no longer sends it and the server mints
  the id; when present (MCP, older clients) it keeps the body-id guard and its 400. The CORS
  allow-list carries the header. The server never reads a client `X-Request-Id`.
- **Compression:** every response (API, MCP, OAuth and the SPA files) is gzipped when the
  client sends `Accept-Encoding: gzip` (`middleware.Compress`, wrapping the whole tree in
  `internal/web/router`), except bodies under 1 KB, already-compressed types, `HEAD` and
  `Range` requests. The JSON bytes are unchanged; a transaction list shrinks about 10x.
- **Transaction sync read:** `GET /api/v1/transaction/get-account-transaction-list?accountIds=a,b`
  serves the SPA's per-account transaction store: `{revisions, items, authors}` where `revisions`
  maps each requested account id to its `transactionsRevision`, `items` are the transactions
  of the accounts being downloaded (each with `authorId`) and `authors` the users they
  reference. Absent `accountIds` = every accessible account, hidden folders included; at most
  200 comma-separated ids; ids the caller cannot access are omitted from `revisions` (never an
  error). `/api/v1/transaction/get-transaction-list` (the legacy full list, still used by MCP,
  older clients and the SPA's fallback against a server without revisions) is unchanged.
- **Dated exchange rates:** `GET /api/v1/currency/get-currency-rate-list` takes optional
  `from` and `to` (`YYYY-MM-DD`, both or neither, `from` <= `to`, inclusive, spanning at
  most 366 days inclusive): the response
  then lists every rate published in that range plus, per currency, the newest one before `from`, instead of only the latest day's. Every item carries
  an additive `date` field (`YYYY-MM-DD`), in the default response too.
- **Reports:** `GET /api/v1/report/get-net-worth-report`, hosted by the budget feature
  (`internal/budget/report_*.go`, `internal/budget/api/report.go`; no `report` package). Scope is
  a budget (every member account; another member's are named only when they are savings members
  or shared with the caller, the rest come as one nameless sum per owner, and the page lists
  only savings members by name) or the caller's own accounts. It is the
  opening balance plus `AccountsNetByMonth`, converted at each month's average rate, over live
  accounts only (deleted ones are left out entirely, history included); the current month stops
  at the end of the caller's today (as the account balance does), an all-time window opens one
  month before the first transaction (a zero balance), and a budget that ended stops at its end
  month. A window is at most 240
  months: an explicit longer one is a 400 on `to` (`common.invalid_choice`), a defaulted
  (all-time) one is clamped to the last 240. The SPA persists a report response only up to 37
  months (`web/src/lib/queryPersist.ts`).
- **Path shape:** `/api/v1/{module}/{action}-{subject}`, all kebab-case, the action
  verb leading. List endpoints end in `-list`. Examples from the source:
  - Reads (`GET`): `/api/v1/account/get-account-list`, `/api/v1/budget/get-budget`,
    `/api/v1/category/get-category-list`, `/api/v1/user/get-user-data`,
    `/api/v1/import/get-source-list`.
  - Writes (`POST`): `/api/v1/category/create-category`, `/api/v1/account/update-account`,
    `/api/v1/category/delete-category`, `/api/v1/connection/generate-invite`,
    `/api/v1/budget/set-limit`, `/api/v1/payee/archive-payee`,
    `/api/v1/import/ingest-apple-wallet-event`.
- **Authentication is header-based:** send `Authorization: Bearer <token>` (an opaque
  access token, `eco_ses_*`/`eco_pat_*`; the scheme is case-insensitive). The auth
  middleware resolves the token's sha256 against the `access_tokens` table, rejects
  unknown/expired/revoked tokens with the 401 envelope, and puts the user id AND the
  token row id into the request context (the latter is the "current session" for
  logout/revoke/isCurrent). Public routes (login, register, remind-password,
  reset-password, confirm-email, resend-verification-code, `/api/doc`,
  `/api/doc.json`, and the six public `oauth` routes — `get-provider-list`,
  `start-login`, `callback-google`, `callback-oidc`, `callback-apple`,
  `exchange-handoff`) need no header; everything else does.
- **Read-only access is enforced at the edge:** a caller whose access level is
  `readonly` (trial ended, no access granted) gets HTTP 402 on any `POST` route not
  in the middleware's small allowlist (account security actions — logout, session/PAT
  revocation, password update, email change, `oauth/start-link`,
  `oauth/complete-link`, `oauth/unlink-identity`, `authserver/revoke-connected-app`,
  `authserver/decline-authorization` (declining writes nothing) and
  `authserver/approve-authorization` (so a lapsed user can approve a
  `read` request; an `mcp` approval from such a user still answers 402) —
  plus `update-analytics` and `update-ai`: withdrawing from product analytics or from the AI
  is a privacy right, not a
  paid feature, so it must work regardless of access level, and `update-last-seen-version`,
  so the "what's new" card stays dismissable); `GET` reads are never restricted.

## Authentication

- **Method**: opaque bearer tokens stored (sha256-hashed) in the `access_tokens` table.
  Three kinds: `session` (minted at login; sliding 30-day TTL — expiry renews on use, with
  last-used persistence throttled to once per 5 minutes), `personal` (user-created
  PATs with an optional fixed expiry, shown exactly once at creation) and `oauth`
  (minted by the MCP OAuth server, see "MCP endpoint"; 1 h fixed expiry, scope `mcp`, or
  `read` for a client that asked for it).
  Every token carries a scope: `full` (sessions, ordinary PATs) or `ingest` (a PAT
  that may ONLY call `/api/v1/import/ingest-*`; anywhere else it gets the frozen 401
  `"Invalid access token"`) or `mcp` (`oauth` tokens only; admitted solely on `/mcp`, the
  frozen 401 elsewhere) or `read` (`oauth` tokens only; `GET` of the
  `middleware.ReadScopePaths` allowlist, the frozen 401 elsewhere). `create-personal-token` requires `scope` and cannot mint `mcp`.
- The `user` feature owns everything: `Authenticate` (the per-request hot path),
  session/PAT use cases, and the revocation cascades. The middleware seam is
  `middleware.TokenAuthenticator`, wired to the user service in `server.BuildAPI`.
- Revocation cascades: `update-password` (the user changing their own password) rotates
  the credential in one locked transaction: bumps the generation (an in-flight login with
  the old password mints nothing), sweeps pending reset codes and email-change requests,
  and revokes the OTHER sessions; it keeps the presenting session, PATs and linked
  identities — integrations must outlive a password change. `user:deactivate` revokes
  sessions AND PATs and bumps the credentials generation, all in the deactivating transaction
  (which is why per-request auth needs no `is_active` join, and why a login racing the
  deactivation cannot mint a session that outlives it). **`reset-password` and
  CLI `user:change-password` are the account RECLAIM** (one primitive,
  `user.reclaimCredentials`): a completed reset is the account's proof of mailbox
  ownership and an operator setting a password is evicting whoever holds the account, so
  both revoke every session AND every PAT, drop every pending grant (outstanding reset
  codes, a pending email change, and on the oauth side every unredeemed handoff — a
  60-second sign-in code is a session in waiting — plus in-flight link states naming
  the account), and unlink every external identity whose provider vouches for a
  DIFFERENT address (`user.OAuthReclaimer` → `oauth.ReclaimAccount`, wired in
  `internal/server` for the API and by the CLI container over the same
  `server.NewOAuthReclaimer`; the proven address is the presented one on the reset path
  and the account's own on the CLI path, where the operator is authoritative). What the
  reclaim deliberately does NOT touch is data sharing: connections and budget grants
  survive, because a routine reset must not dissolve a family's shared budget.
  Registration does not always verify email, so a squatter could have linked their own
  provider account to an address they never owned; revoking passwords and tokens alone would leave that link
  as a way back in. An identity claiming the proven address survives — obtaining one
  needs that mailbox — which is also why the passwordless "Set a password" flow (the
  same reset endpoint) keeps its provider. All of it shares the password write's
  transaction. `unlink-identity` carries a small cascade of its own: it drops that
  provider's pending sign-in handoffs along with the identity row, and a redemption
  re-checks its identity under the same user row lock, so a code the unlinked provider
  authorized can no longer be claimed.
- **The credentials fence** (`users.credentials_generation`): sweeping what exists is
  not enough on its own, because a request that read its evidence BEFORE the reclaim
  can still write after it — redeeming a handoff it consumed a moment earlier, landing
  an identity from a callback already in flight, or minting a PAT under a session the
  reclaim just revoked. Every such flow captures the user's generation when it reads
  its evidence (the verified password hash, the resolved oauth user — carried on the
  handoff row as `oauth_handoffs.credentials_generation`) and presents it again at
  write time. Session inserts (`InsertAccessTokenIfGeneration`) and identity writes
  (`InsertIdentityIfGeneration` / `UpdateIdentityIfGeneration`) are conditional on it
  inside the SQL, so the DATABASE
  decides the race: the reclaim bumps the generation, and anything in flight writes
  zero rows and fails closed. Checking in Go would be the race it is meant to close.
  The fence only sees COMMITTED rows, so it is paired with a lock: every write to an
  existing user's row and every credential mint — session and PAT inserts, and the
  identity INSERTs of complete-link, auto-link and provisioning — runs under the user
  row lock (`Repository.LockRow`), taken before the row is read and held in the same
  transaction as the write, and the reclaim takes it first
  too (it bumps `users` before it sweeps) — so on PostgreSQL a write racing the
  reclaim's still-open transaction blocks instead of slipping past, and a
  whole-aggregate `Save` can never put back a row read before the reclaim landed.
  The narrow fenced `users` UPDATEs (`rehashLegacyPassword`, `mirrorEmailDrift`) are
  the one exception, and need no lock: PostgreSQL serializes concurrent UPDATEs on the
  row itself and re-evaluates their WHERE against the committed version, so the fence
  is decided after the reclaim rather than before it. The reset re-reads the locked
  row's address too: a confirmed email change landing in the window makes it a
  different account, and the reset is refused rather than written to it. Its code is
  fenced the same way: the reset re-reads the presented code under that lock and
  consumes it row-counted, and `remind-password` replaces a user's codes under the
  same lock, so one code is one reset — a code a fresh remind replaced, or one a
  concurrent reset already took, resets nothing. Verification codes, like reset
  and email-change codes, are issued and consumed under the user row lock, so a
  confirmation can only ever take the row it checked and never sweeps a code a
  concurrent resend has just emailed.
- Dead rows (expired/revoked > 30 days ago) are purged opportunistically at login;
  `token:purge [days]` does the same globally in one indexed DELETE (the
  revoked_at/expires_at indexes exist for it). The login purge covers sessions and
  PATs only: `oauth` tokens (1 h lifetime, so far more of them) are purged by the MCP
  OAuth server's housekeeping after successful code exchanges and refreshes, at most once
  every 10 minutes, in one set-based DELETE.
- Sessions/PAT management endpoints: `get-session-list`, `revoke-session`,
  `revoke-other-sessions`, `get-personal-token-list`, `create-personal-token`,
  `revoke-personal-token` (all under `/api/v1/user/`).
- Login lives under `internal/user/api` (`/api/v1/user/login-user`).
- Token refresh is not implemented (sessions slide instead; clients re-authenticate
  after 30 days of inactivity).
- **Provider sign-in** (`internal/oauth`): a state-carrying redirect to the provider, a
  callback that verifies the ID token and resolves to an existing
  `(provider, issuer, subject)` identity, an auto-link by verified email, or (when
  `ECONUMO_ALLOW_REGISTRATION` is on) a
  new passwordless account (`users.algorithm = 'none'`), then a one-time, 60-second
  handoff code the client exchanges for a normal session — never a token in a redirect URL
  or server log. The `start-*` call also returns a **flow secret** (stored in `sessionStorage`) that the client must present alongside the handoff:
  the flow is bound to the client that began it, so a forged callback cannot log a victim
  into an attacker's account. **Auto-link happens only into a PASSWORDLESS account**
  (one a provider created, whose owner therefore already proved the address); a verified
  email matching an account that has a password is refused with `account_exists_password`,
  because whoever set that password never had to prove they own the address. No eviction
  is thorough enough there — a squatter's other linked identities, shares and invites
  would be inherited by whoever the provider vouched for — so the owner signs in with
  their password (or resets it through the mailbox the provider just proved they hold)
  and links the provider from Settings instead. That reset is itself the reclaim: see
  the revocation cascade above for what it takes away, and the credentials fence below
  for what stops an in-flight sign-in from outrunning it. **A link started from Settings writes nothing on
  the callback**: the provider redirects whichever browser followed the authorization URL,
  so the callback parks the resolved identity in a one-shot *link* handoff and the
  authenticated `POST /api/v1/oauth/complete-link` performs the write once the initiating
  client presents both its flow secret and a session for the account the link started
  from — without that, an attacker's own start-link URL opened by a victim would bind the
  victim's identity to the attacker's account. Identities live in `users_identities`,
  keyed by `(provider, issuer, subject)`; the issuer is in the key because the custom
  slot's provider id is always `oidc`, so repointing `ECONUMO_OIDC_ISSUER_URL` must not
  let a subject collision at the new issuer resolve to the old issuer's user (a returning
  user matched by verified email has their one row per slot repointed instead).
  Every oauth-originated session stamps `provider` on the row (Google, Apple, and the custom
  OIDC slot alike); only the custom OIDC slot also stores `id_token`, since Google and Apple
  publish no end-session endpoint. `logout-user` returns a non-empty `logoutUrl` only for a
  session that carries an `id_token` — i.e. an OIDC session — built from that stored value.

## Wire & data contract (frozen)

These behaviours are **frozen**: the web SPA and other clients parse exact JSON,
and the production database holds data written in these formats. Don't "clean
them up" — changing one breaks live clients, locks users out, or makes stored
data unreadable. Most are also asserted by the test suite.

### Response envelope (`internal/web/httpx/envelope.go`)
- Success (200): `{"success": true, "message": "", "data": <payload>}`
- Error (handled, default 400): `{"success": false, "message": <string>, "code": <int>, "errors": <object>}` — `errors` maps field → `[messages]`, always present (`{}` when none).
- Exception (500): `{"success": false, "message": <string>, "code": 0, "exceptionType": <string>}` — no `errors` key; error detail (message, stack trace) goes to the server logs only, never the response body.
- Not implemented (501): `{"success": false, "message": <string>, "code": 0, "errors": []}` — here `errors` is an array `[]` (the lone exception to the object rule).
- Rate-limited (429): `{"success": false, "message": "Too many attempts. Try again later.", "code": 429, "errors": {}}` — same shape as the handled-error envelope.
- JSON is encoded with HTML escaping disabled (`/`, `<`, `>` appear literally).
- **Server-rendered error language** (handled-error envelope only): `message` and the per-field
  `errors` strings are rendered server-side in the caller's language when the underlying error
  carries a catalogue code (`errors.*` keys in every `locales/<lang>.json`; registry:
  `internal/shared/errs/codes.go`, `AllCodes`); errors without a code keep their literal English
  text. The language is `reqctx.Language`: `Accept-Language` (the SPA sends its selected locale
  on every request) → the authenticated user's stored `users.language` (resolved in the auth
  middleware via `middleware.StoredLanguageResolver`) → `en`. The `en` catalogue text matches the
  historical strings, so English callers see the same wire bytes as before; the envelope carries
  no separate code/translation fields.

### Auth crypto (`internal/infra/auth/`)
- **Password hash**: versioned by `users.algorithm`. `sha512` (legacy, all pre-existing rows): sha512, 500 iterations, base64 (88 chars), salt merged as `password{salt}`; `digest = sha512(salted)` then 499 rounds of `sha512(digest || salted)`; verify rejects len≠88 or a `$`, constant-time. `argon2id` (every new hash: registration and all password changes): PHC string `$argon2id$v=19$m=19456,t=2,p=1$…$…` (OWASP params), salt embedded in the hash — the `salt` column persists for sha512 rows. Verification dispatches on the column; unknown values fail closed.
- **User lookup key**: `users.email` holds `model.NormalizeEmail(email)`, computed in Go (the one form every write, lookup, comparison and rate-limit key goes through) because the engines' `lower()` disagree; enforced unique by `users_email_uniq`. The case an address was typed in is not kept. Trim plus Unicode lowercase, except that a non-ASCII letter whose lowercase is ASCII (`İ`, the Kelvin sign) keeps its form, since it spells a different mailbox. Callers hand `GetByEmail`/`ExistsByEmail` the address as typed; the repo tries that exact spelling, then the normalized form. The services normalize on every email write, and `Save` stores the aggregate's spelling verbatim. The legacy `identifier` column is retained (dropping a NOT NULL UNIQUE column would force a SQLite table rebuild) but retired: it now holds the row's own `id` as a unique non-null placeholder, no longer derived from email. `EncodeService.Hash` (the md5 identifier derivation) was removed — nothing computes it anymore.
- **Email encryption**: emails are stored as plaintext. `EncodeService` still implements AES-128-CBC (key = raw salt, 16 bytes; layout `base64(iv[16] || hmac_sha256[32] || ciphertext)`, PKCS#7, random IV, HMAC verified constant-time before decrypt), but the API constructs it with an empty salt, so Encode/Decode are passthrough. The salted path runs only inside `data:remove-salt`.
- **Salt-free everywhere**: the API and all CLI user commands construct `EncodeService` with `""` and ignore `ECONUMO_DATA_SALT` entirely (`server.BuildAPI`, `cli` container). The salt reaches code through one path only: `data:remove-salt` passes it into `MigrateRemoveDataSalt(ctx, salt)`, which builds a temporary salted encoder to decrypt legacy email data to plaintext (the repo writes the row `id` into `identifier` automatically, so no re-derivation is needed).

### Access tokens (`internal/user/token.go`)
- Format: `eco_ses_` (session) / `eco_pat_` (personal) / `eco_oat_` (oauth) + `base64.RawURLEncoding` of 32
  random bytes (43 chars, alphabet `[A-Za-z0-9_-]`). Only `hex(sha256(token))` is stored.
- 401 messages are exact: `"Access token not found"` (missing/malformed header) and
  `"Invalid access token"` (unknown/expired/revoked token, and any internal
  authenticator error — no internals leak).
- Sessions: sliding `expires_at = last_used_at + 30d`, touched at most every 5 minutes.
  PAT `expires_at` never moves (NULL = never expires).
- `scope`: `"full"` | `"ingest"` on `create-personal-token` (required; blank → `common.is_blank`,
  unknown → `common.invalid_choice`) and echoed on `get-personal-token-list`. `oauth` tokens
  carry `"mcp"` (admitted only on `/mcp`) or, for a client that asked for it, `"read"`
  (admitted only on `GET` of the nine `middleware.ReadScopePaths`); neither can be minted
  through `create-personal-token`.

### Encodings, messages, routes
- Datetimes: `"2006-01-02 15:04:05"` — space separator, no zone, no fractional seconds.
- `isArchived` → int `0`/`1` (not bool); category `type` → alias string `"expense"`/`"income"`; empty string for NULL where the schema does.
- Validation strings are exact per language and asserted by tests in `en` (the default with no `Accept-Language` and no stored preference), e.g. `"Category name must be 3-64 characters"` (field `name`), `"Invalid credentials."` (401), `"This value should not be blank."` (code `common.is_blank`); coded errors render from the `errors.*` catalogue in the caller's language (see the envelope section).
- Exact route paths/methods are contract, e.g. `POST /api/v1/user/login-user`, `POST /api/v1/user/register-user`. Login takes `username` (email) + `password` and returns `{"token", "user"}`; register returns the created user **without** a token. Public routes: login, register, remind-password, reset-password, confirm-email, resend-verification-code, `/api/doc`, `/api/doc.json`, and the six public `oauth` routes (`get-provider-list`, `start-login`, `callback-google`, `callback-oidc`, `callback-apple`, `exchange-handoff`); everything else needs a valid access token.
- Data: ids are stored as `TEXT`. New ids are UUIDv7; existing ids are never rewritten (they're FK targets and held by clients).
- `avatar` (user embeds) → `"<icon>:<color>"`, e.g. `"face:fuchsia"` — a Material
  icon ligature name plus a color slug from the 7-slug allowlist in
  `internal/user/avatar.go` (mirrored by `web/src/lib/avatars.ts`). Set via
  `POST /api/v1/user/update-avatar`; new users get a random default; Gravatar
  is gone.

## Key design decisions

- **One binary, two engines, runtime-selected** — both DB backends are linked in and chosen by `DATABASE_URL`; no Go plugins.
- **sqlc for compile-checked SQL** — a wrong column/arg fails `go build`; per-engine query variants only where the dialects diverge (see the engine-adapter pattern).
- **stdlib-first** — `net/http.ServeMux` routing, `func(http.Handler) http.Handler` middleware, hand-written `Validate()` (no tag DSL), stdlib CLI. Third-party deps only where stdlib can't deliver (decimal, DB drivers, uuid, sqlc, Resend).
- **No assembler layer** — app services build and return the result DTOs directly.
- **SQLite is the reference engine** — it's the default; PostgreSQL must match it byte-for-byte (enforced by the `enginecompare` suite).

### Notable behaviours
- **Category type matches transaction type**: an income category classifies only income and
  an expense category only expenses. `checkReferences` (create, update, bulk, recurring post,
  MCP, imports) refuses a mismatch with the field error `categoryId` /
  `transaction.category_type_mismatch`, except on an update that keeps the stored category
  and type (rows written before the check stay editable); recurring templates are checked
  the same way on save. CSV import resolves a category name within the row's sign (creating
  one of that type when missing) and leaves a constant `categoryId` off rows of the other type.
  Rows written before the check are cleaned by the SQL migration `20261017000000`: the
  category is cleared and an English note appended to the description (the old text is
  shortened, never the note, to fit the 255-character column), and the affected accounts'
  `transactions_revision` moves so synced clients refetch.
- **Budget element visibility**: a tag/envelope/category appears in `get-budget` when it has spending **or** a limit (current or carried-over) — so a tag with a limit but no transactions stays visible.
- **Empty plan vs a plan of 0**: a plan of 0 is stored (a limit row with amount 0) and counts as a
  plan everywhere (an active row stays visible with it, the red short-plan hint), except that an
  archived row with nothing but a plan of 0 stays hidden; `set-limit` with `amount: null` removes the
  row. `get-budget` sends `budgeted: "0"` for both and tells them apart with the additive
  `isBudgeted` (0/1) on elements and savings rows; `get-budget-plan` cells already send `planned: ""`
  for no row. The SPA maps `isBudgeted: 0` to `budgeted: ''` (`getBudget`), shows an empty plan as
  a dash (Budget view) or a blank cell (Plan grid) and a plan of 0 as `0`; typing 0 saves 0, an
  empty input clears, and copying or filling from an empty Plan cell clears the targets.
- **Account balance day boundary**: "balance as of end of today" uses the **caller's** timezone (`X-Timezone` header), not the server's UTC day.
- **Recurring transaction scheduling**: templates advance from the scheduled date with
  month-end clamping via a persisted scheduled day (31st → Feb 28 → Mar 31); posting is
  manual (the user acts on a due template) and idempotent on the client-supplied
  transaction id.
- **Upcoming recurring in Budget/Plan**: the server projects active recurring templates
  (source OR recipient on a budget member account, partners' included; deleted-account
  templates skipped) into occurrences
  (`internal/model/recurring_projection.go`, `internal/budget/{projected_read,upcoming_merge,txlist}.go`,
  glue `internal/server/glue_budget_recurring.go`). An overdue next payment counts once in the
  caller's current month, afterwards only occurrences after today. A real transaction dated
  from the caller's tomorrow on (`X-Timezone`) has not happened yet: every `get-budget` /
  `get-budget-plan` figure reads the stored transactions through `windowRead` capped at that
  instant (`internal/budget/dated_read.go`, `asOfTomorrow`; computed in Go, bound as a
  parameter), and the upcoming pass reads those later rows (`upcomingRead`: the projection
  plus the stored rows from tomorrow to the window's end, summed per key), so they are
  upcoming money exactly like a due template. Upcoming money is a HINT
  on the plan number, never part of the figures: spent/actual, Available, every total and
  projection (Balance, Total savings, savings `closingBalance`, opening balances, flows,
  carry-over) are money dated up to today only. `get-transaction-list` keeps listing every
  real row of the period, future-dated ones included; its `upcoming[]` stays recurring-only. The budget transaction list leads with a "Coming up" group laid out like the
  plan editor's list (the cell's future-dated rows and its recurring payments still due,
  soonest first, display only), then a "Posted" heading over the past rows in their day
  groups; both headings show only when something is still to come. Additive wire fields: `upcoming`/`budgetUpcoming` on
  `get-budget` elements and children (savings rows and labels carry only `upcoming`), `upcoming` on `get-budget-plan` cells,
  an `upcoming` array on `get-transaction-list` (read by the plan editor's list and the transaction list's "Coming up").
  The SPA (`features/budgets/planHint.ts`), from the current month on, on rows
  that hold a plan: an empty plan stays empty, no suggestion is shown (a plan of 0 is a plan); an
  expense row (category/tag/envelope) whose plan is below spent + still due shows the plan
  in red; income and savings are never red; folder, tag-group, total, label and child rows
  get no hint, and Uncategorized (no plan to hint) is listed only for real money. While a
  plan is being edited (desktop `LimitEditor`, phone `SetLimitDialog`, Plan grid in-cell
  input), a cell in the current month or later with spent or upcoming money gets a quiet
  list (`UpcomingPaymentsPanel`): the still-due recurring payments (repeat icon, overdue
  marker) merged by date with the cell's own transactions dated after today, each in its own
  currency with its symbol, in one right-aligned amount column, savings signed; then an
  "Already spent" / "Already received" / "Already saved" line (by row kind; the cell's spent,
  which holds none of the rows listed above, hidden at 0) and a "Total" line (spent + still due, cell
  currency with its symbol), so the column adds up. Headings and Total are set off in the
  foreground colour, and in the Plan grid the list opens below the cell's frame. The whole
  list is one button: a press anywhere fills the input with that total (future-dated rows are
  in upcoming, not in spent, so never added twice) without committing. A cell with nothing still to
  come (no due payment, no future-dated row) shows no list, Spent/Total lines included.
  Past months and read-only cells never show it; there is no Post/Skip/Edit of templates from budget views.
- **Gaps in stored rates**: a period with no rate at or before it snaps to the earliest stored
  month (`RateProvider.snapPeriod`), and a currency that month has no rate for takes its latest
  earlier rate, else its first (`AverageRates`), never face value, so a long report window or an
  old budget month does not count 40,000 RUB as 40,000 CAD. Server conversions and the budget's
  `currencyRates` block read the same rates. The import converter alone reads `StoredRates`, with
  nothing filled: a transaction with no rate for its month queues as no-rate.
- **Currency deletion is always soft**: deleting a custom currency sets `currencies.is_deleted`,
  never removes the row — `accounts.currency_id` and `transactions.account_id` cascade from it,
  so a hard delete would silently destroy every account and transaction in that currency. A
  deleted currency still appears in `get-currency-list`/`get-currency-rate-list` so entities
  that hold it keep resolving their symbol and rate.
- **Budget account membership** is explicit (`budgets_accounts`): a budget counts exactly its
  member accounts, soft-deleted members included (their history keeps counting so
  `budgetedBefore`/`spentBefore` share one population). An account can be removed only while it
  has no transactions between the budget start and the first of the caller's current month
  (`budget.account_not_removable`); a departing participant's accounts leave with them.
  `create-budget` requires ≥1 owned `accountIds`; `update-budget.accountIds` is optional (absent
  = untouched, present = replace-set over the caller's own accounts). Deleting an account
  auto-writes a "Balance adjustment (account deleted)" correction so deleted accounts always
  have zero balance, and the invariant is **self-healing**: creating a transaction against a
  deleted account is rejected (`transaction.account_deleted`), and deleting/editing a
  transaction that still touches one (e.g. an old transfer to a live account) re-zeroes the
  deleted side in the same DB transaction with a correction dated at the affected transaction's
  `spent_at` ("Balance adjustment (deleted transaction)" / "(edited transaction)").
- **Budget lifecycle**: a budget may carry an end month (`ended_at`, inclusive first-of-month;
  `endedAt` on the wire, `""` when unset) and an archived flag (`isArchived` 0/1). Archived =
  hidden + read-only: every budget write returns a coded 403 `budget.archived` except
  unarchive/delete/revoke/decline/accept (and archive itself, idempotent). Readers clamp periods
  to the end month; `set-limit` refuses periods past it; the membership removal window ends at
  `min(current month, end month + 1)`. `clone-budget` is an owner-or-admin single-transaction deep
  copy (structure, sharing, membership, optionally plans from a chosen start month; envelope
  elements' `external_id` remapped) — the copy starts open-ended and unarchived, owned by the
  cloner; when an admin clones, their own grant is dropped (they own the copy) and the former
  owner joins the copy's sharing set as an accepted admin, so every member account keeps a
  participant backing it.
- **Budget period start day**: each budget starts its months on `period_start_day` 1-28
  (default 1; `periodStartDay` on the meta, optional on `create-budget`/`update-budget`, copied
  by `clone-budget`, anything else is the field error `budget.invalid_period_start_day`). A
  period keeps its key `YYYY-MM-01` (`started_at`, `ended_at`, limit and comment periods, Plan
  `months`, MCP `month`); only the date window it covers shifts. The rule lives in one helper
  per stack (`model.PeriodWindow` / `model.PeriodOf`, `web/src/features/budgets/period.ts`):
  every period is named after the month it starts in, key `M` covering day `d` of `M` to day
  `d` of `M+1` (day 1 = the calendar month; day 5: "October" = 5 Oct to 4 Nov; day 28:
  "October" = 28 Oct to 27 Nov). Request dates that pick a period name a key (snapped to the
  first, day ignored),
  never "the period containing this date". `get-budget` `filters.periodStart/periodEnd` carry
  the real window. A budget whose day is not 1 converts at the rates inside each window
  (`RateProvider.Windowed()`, wired through `budget.Service.SetShiftedPeriodRates`;
  `currencyRates[]` reports that window, and a window holding no rate keeps the snap); day-1
  budgets, legacy mid-month windows included, imports and everything else keep the default
  latest-rate-month snap (`internal/currency/repo/convertor_provider.go`). "Current month"
  everywhere (default period, Plan window, plan hints, removal window, overdue recurring) is
  the period containing the caller's local today. The by-month SQL (`internal/budget/repo/read.go`,
  `periodExpr`) keeps grouping by month for day 1, so a day-1 budget stays byte-identical, and
  groups by day and folds the rows onto keys in Go (`foldPeriods`) otherwise. Changing the day
  writes only the day: `started_at` (the first key, `Budget.FirstPeriod()`), `ended_at`, plans
  and comments never move, so changing it back restores everything. Today may then fall
  before the first key's window (key October switched to day 28 on 5 October): the budget
  has not started yet, so the default `get-budget`/`get-budget-plan` period, create/clone
  responses and the SPA's default month (`budgetCurrentPeriod`) open on the first key, and
  days before its window belong to no period. Only the owner or an admin may change the day.
  Reports, recurring schedules and account pages stay on calendar months. The SPA hides the
  day picker against a server whose budgets carry no day. Changing the
  naming rule after release needs a key migration (move `started_at`, `ended_at`, limit and
  comment periods of the affected budgets by one month).
- **Budget savings**: savings is a per-budget MEMBERSHIP flag
  (`budgets_accounts.is_savings`), not an account type — the same account can be a
  savings member of one budget and an everyday member of another, and `accounts.type`
  is untouched. The owning member sets it from the budget's own settings (the
  create/update budget dialogs' per-account "Savings" switch, visible next to every
  own selected account, plus `add-account`'s `isSavings`) — never from the account
  dialog, and never by another participant (only the account's owner may flag it).
  A flagged member gets an `ElementSavings` (type 5) row per budget, in its own
  ordering group (never a folder); the flag or membership write that adds or drops
  the savings member creates or deletes the row in the same transaction, and a
  departing participant's savings rows are deleted at revocation with their other
  element rows. Turning a savings member off, or removing one,
  while it still carries plans or comments is refused unless the write also sets
  `confirmSavingsRemoval: true` (`update-budget`/`add-account`/`remove-account`). The
  guard locks the leaving savings rows before it checks them and the element writers
  share-lock theirs (`getElementSelfHeal`), so on PostgreSQL a concurrent plan or
  comment is either seen by the guard or refused as not found, never cascaded away. The
  SPA asks first (naming what will be deleted) and resends with the flag set on
  confirm — cancel leaves the member's plans and comments untouched. "Actual saved" is
  the net of everyday↔savings transfers only — savings↔savings and transfers with a
  non-member account don't count, and neither does interest — so the Total savings
  can rise by more than what shows as saved. These rows stay out of `structure.elements`
  on the wire (unknown-field-safe for old clients): `get-budget` carries
  `structure.savings`, `get-budget-plan` adds `savingsOpeningBalances`/`savingsFlows`
  alongside it. Each savings row carries `closingBalance`, the account's balance at
  the END of the month (element currency): on `get-budget` it is booked for a month
  before the caller's current one and, from the current month on, projected by the
  server — booked plus each month's plan not yet met by its saved amount, from the
  current month through the selected one, per month and per row (an over-saved month
  covers nothing); a plan cell's `closingBalance` is booked only and the SPA adds the
  same gaps (`projectSavingsClosings`) the Total savings line adds, so the two
  agree. The monthly block's columns are Planned / Saved / Balance (that figure).
  Savings rows and balances are visible to every participant, guests included (the
  budget settings' "Savings accounts" card says so). In the settings dialogs both
  choices sit behind picker rows (`BudgetAccountsField` / `BudgetSavingsField`: a row
  naming the picks, opening a searchable checklist), so the dialog does not grow with
  the account count; the savings picker lists the included accounts only. On a phone
  the monthly block keeps the table's two figures (Planned, Saved); Balance shows
  from `sm` up. A deleted savings account's row stays
  visible only while it still has a plan or actual activity in the period; the plan's
  Balance/Savings-balance split follows the savings DATA rather than only the visible
  rows, so a deleted savings account with a balance but no row in the window still
  moves its money out of the everyday Balance.
- **Transaction import (Apple Wallet)**: one `import_sources` row per user per
  provider (`create-source` is idempotent on the pair); a Wallet card is keyed by its
  normalized name and starts `unmapped` — its events queue (`import_transaction_links`
  with no transaction) until the user maps it to an OWNED account (`link-account`
  replays the queue in one run) or ignores it. Ingest is scope-gated (`ingest` PATs),
  exact-duplicate-safe (`(source, external_account_id, external_transaction_id)`), and never fails the
  request on a bad transaction — parse errors land in `import_events.status = failed`
  for the queue page's "needs attention" list. Imported transactions read
  `isImported: 1`; `get-transaction-import-list` is their provenance.
- **Transaction import (SimpleFIN pull)**: the client claims the one-shot setup token
  through `claim-setup-token` (the server returns the access URL and keeps nothing), encrypts
  it in the browser with a passphrase-wrapped data key (`web/src/lib/importCrypto.ts`; the
  wrapped key lives in `import_credential_keys`, one row per user; the unwrapped key stays
  non-extractable in the `importKeys` store of the `econumo` IndexedDB, removed by `clearPersistedQueryCache` on every sign-in and sign-out like the rest of the user's local data, so each sign-in asks for the passphrase once), and stores only the ciphertext on the source. Every
  `list-external-accounts`/`sync-source` call carries the plaintext access URL in the body —
  "at rest zero-knowledge, in flight trusted" — and it is never persisted, logged, or
  formatted into an error. A sync is one `import_runs` row; per-account failures leave the
  run `partial` (so do rows the bridge returned in an unparsable shape), a bridge failure leaves it `failed`; `last_synced_at` moves only on
  `completed`/`partial`. Sync is manual (a button); sync-on-open is a follow-up.
- **Transaction import (matching and deleted rows)**: before creating, the matcher adopts an
  existing transaction on the mapped account of the same kind and amount within
  `ECONUMO_IMPORT_MATCH_DAYS`. A transfer counts as its leg (expense with the source amount on
  its source account, income with the recipient amount on its recipient account), so a card
  payment entered by hand as a transfer adopts both the bank's debit and the card's credit; a
  link from the same source blocks a transfer only for the same external account, and tip
  adopts never take a transfer. Deleting an imported transaction leaves its ledger row as a
  tombstone, so no sync or re-fired push brings it back; `reimport-link` (the history's
  "Import again" on a deleted row) deletes the tombstone and runs its stored event through the
  pipeline again. The new row keeps the old one's id, run and import time, so it keeps its place in
  the run, and the run summary moves it from imported to matched, queued or skipped when that is
  the outcome (the deleted row is taken to have been created; the ledger does not say).
- **Transaction import (rules)**: one `import_rules` row per rule, per user, optionally scoped
  to a source; `classify` rules only fill empty fields on newly imported rows (first match per
  field in priority order, labels unioned up to 10), `skip` rules drop the row after currency
  conversion and before matching and beat every classify rule; the ledger row snapshots what was
  applied (`applied*` on `get-transaction-import-list`) and the SPA diffs a later edit against it
  to offer a rule; `apply-rule` backfills existing imports by run/source/all, skipping edited
  rows unless `includeEdited`. A rule's category reaches only transactions of its own kind
  (income category → income, expense → expense; `categoryFits`): on the other kind the
  category is held back, the rule's payee/tag/labels still apply, and a lower rule may fill
  the category; apply skips rows the rule would not change (preview counts them apart, as `changes`). Rules belong to the account owner, not the importer: a card
  mapped onto an account shared with its user is classified and skipped by the account
  OWNER's rules (the transaction takes the owner's categories, payees, tags and labels), the
  importer's own rules never apply there, and the owner's "all" apply reaches those rows.
- **OAuth/OIDC auto-link and email verification**: a callback with a verified (or
  `ECONUMO_OIDC_TRUST_EMAIL`-trusted) email auto-links to an existing **passwordless**
  account by `email` (no matching identity yet) — no confirmation screen, since both
  the account's address and the new claim were proven by providers. A match on an account
  that has a password is refused (`account_exists_password`), never merged. An unverified
  email is rejected (`email_unverified`) for every intent, including an already-linked
  identity signing in again and a link started from Settings — there is no path around the
  trust flag. **Every change to the set of sign-in methods emails the account owner** a
  best-effort notice naming the provider's display name in the account's stored language:
  `emails.identity_linked.*` when a provider is auto-linked or linked from Settings
  (`complete-link`, on the INSERT branch only — a relink that just refreshes a stored
  email grants nothing and stays silent), `emails.identity_unlinked.*` on
  `unlink-identity`. The notice is about DETECTION, not surprise — mail is the one channel
  a caller holding a stolen session cannot suppress — which is why the flows the owner
  started notify too. Each send sits outside the write's transaction and never fails it (or
  the callback redirect). The reclaim's identity sweep (`ReclaimAccount`) is the one
  deliberate exception: it runs inside a password reset the owner just completed, which
  announces itself.
- **Notice emails reach every linked address**: the three NOTICES — identity linked,
  identity unlinked, and the change-email heads-up to the old address — are CC'd to the
  addresses the owner's linked providers reported (`users_identities.email`, via
  `oauth.Service.ListIdentityEmails`; the user feature reads it through its
  `IdentityEmailLister` port, wired in `internal/server` like `OAuthReclaimer`). The three
  CODE emails are not: a reset, verification or change-email code exists to prove control
  of one specific mailbox, so copying it elsewhere would defeat the check it exists for.
  `mailer.ccAddresses` drops the To address and dedupes case-insensitively, so the primary
  address never appears twice. Resolving the list is best-effort — a lookup failure sends
  the notice to the primary address alone rather than failing it — and the unlink notice
  copies the addresses that REMAIN, the removed row being gone by the time it sends.
- **OAuth email drift**: when a provider's claimed email differs from the signed-in user's
  stored email, the stored email is left alone UNLESS the user is passwordless, has exactly
  one linked identity, and no other user already holds the new address — in that narrow case
  (the IdP is the account's sole authority) the primary email is replaced immediately after,
  in its own transaction. The rule stops applying the moment the user sets a password or
  links a second identity.
- **RP-initiated logout**: `logout-user` returns `logoutUrl` when the ending
  session's provider published an `end_session_endpoint` (the custom OIDC slot only — Google
  and Apple publish none); the web client navigates there, landing back at
  `<ECONUMO_URL>/login`.

## Deployment

- Image: `ghcr.io/econumo/econumo` (GitHub Container Registry only).
  - `:dev` — published locally via `make publish-dev`, or by the release workflow's "Publish Dev" checkbox.
  - `:latest` + `:vX.Y.Z` — published by the GitHub release workflow (latest only from `main`).
- Docker-free: each release attaches single-file linux binaries (SPA embedded)
  + `SHA256SUMS`; walkthrough in `docs/run-without-docker.md` (linked from the
  README). The same binaries plus the ship unit template
  `deploy/ship/econumo.service` go to R2 for hosts managed with ship.
  `make release-binaries` builds the same artifacts locally.
- Self-hosting: see the root `docker-compose.yml` (+ `.env.example`, copied to `.env`)
  and the README quick-start. The Dockerfile is `deployment/docker/Dockerfile`.

## Code Quality Tools

- `gofmt` (formatting), `go vet` (static analysis), the coverage gate (`make go-test`).
- Frontend: oxlint (`make web-lint`) + vitest (`make web-test`).

## Comments — write sparingly

Comment only **exceptional scenarios** and **non-obvious business logic / frozen-contract
rationale** — the *why*, not the *what*. Do NOT add:

- godoc or inline comments that merely restate a symbol's name or signature
  (`// Id returns the id`, `// CreateCategory creates a category`), accessor docs;
- section-divider / scaffolding comments, or anything that paraphrases the next line;
- references to the former PHP/Symfony implementation. The backend was ported from a
  now-removed Symfony 5.4 (PHP) app; state the behavior or constraint directly in Go
  terms rather than naming the old code (e.g. "timestamps are bare `Y-m-d H:i:s` with no
  zone, so the clock must be UTC" — not "to match the PHP DatetimeService").

**Exempt:** Swagger `// @…` annotation blocks on handlers (they generate the OpenAPI
spec) and `// Code generated … DO NOT EDIT.` markers — leave both intact.
