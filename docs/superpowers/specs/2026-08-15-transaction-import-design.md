# Transaction Import (SimpleFIN + Apple Wallet) — Design

**Date:** 2026-08-15
**Status:** Approved (brainstorm); supersedes the 2026-07-19 SimpleFIN-only draft

## Overview

Automatic transaction import from external providers, shipping with two
ingestion modes from day one:

- **Pull** — the server fetches from a bank aggregator on user demand, starting
  with [SimpleFIN Bridge](https://www.simplefin.org/protocol.html) (North
  America).
- **Push** — an external client sends single transaction events to the server,
  starting with **Apple Wallet**: an iOS Shortcuts automation fires on every
  Apple Pay tap and POSTs the purchase to Econumo.

The feature is deliberately built as a **generic import subsystem** that
SimpleFIN and Apple Wallet are merely the first clients of. The durable parts —
external→internal id mapping, deduplication, the unmapped-event queue, mapping
rules, import runs, the reconciler — are provider-agnostic and are reused by
the existing CSV import and by any future provider (Enable Banking, Pluggy,
Akahu, Tasker/Macrodroid on Android).

Two properties shape the pull side:

1. **Zero-knowledge at rest.** Econumo is offered as a hosted service. The
   server stores the provider credential as opaque ciphertext it cannot
   decrypt; the key never leaves the browser. A leaked database or backup
   exposes no bank access.
2. **Manual sync only.** There is no scheduler. The user clicks "Sync". This
   matches the reference implementation (Actual Budget's bank sync is also
   manual-trigger) and is a *requirement* of (1) — a server that cannot decrypt
   a credential cannot sync unattended.

The push side involves **no bank credential at all** — the phone authenticates
with a scoped personal access token and sends data the user's own device
already has. Zero-knowledge machinery does not apply to it.

### Prior art

Actual Budget brokers to five region-locked aggregators and reconciles with a
three-stage matcher keyed on a per-provider `imported_id`, plus an
`imported_payee` column holding the raw bank string. That data model — not the
provider list — is the transferable insight. GoCardless Bank Account Data (the
free Nordigen tier Actual's EU users rely on) has been **closed to new signups
since July 2025**, which is why this design starts with SimpleFIN and treats
the provider as swappable from day one.

## Decisions

- **First pull provider:** SimpleFIN. Simplest protocol (token → access key →
  one REST call), no eIDAS broker, no PSD2 licensing. Enable Banking is the
  intended second provider for EU coverage.
- **First push provider:** Apple Wallet via the iOS Shortcuts "Transaction"
  automation trigger — fires automatically on every Apple Pay tap and exposes
  merchant name, amount, currency, card/pass name, and timestamp. Notably it
  exposes **no stable transaction id**; the server synthesizes one.
- **Push endpoints are per-provider, the core is shared.** Each push provider
  gets a dedicated route (`ingest-apple-wallet-event`) that validates that
  provider's exact payload shape and normalizes it into a canonical internal
  event; one shared ingest service does dedupe/queue/create. Same
  architecture as the pull Provider seam: generic subsystem, provider-specific
  adapter at the edge. `import_sources.provider` stores the integration name
  (`'simplefin' | 'csv' | 'apple-wallet'`), never a generic `'push'`; the
  pull/push distinction is a property of the provider registry in code.
- **Push auth: scoped PATs**, not a new token kind. `access_tokens` gains a
  required `scope` column (`'full'` = today's behavior; `'ingest'` = valid
  only for `ingest-*` routes). **Every creation path provides the scope
  explicitly** — sessions are minted `'full'` by the login code, and
  `create-personal-token` requires the field; there is no implicit fallback
  in application code. One token system, existing revocation/purge/UI infra
  reused, future scopes (e.g. read-only) enabled.
- **Pull credential ownership:** per-user, client-side encrypted. Not `.env`
  (breaks on a multi-user instance), not server-side plaintext (what Actual
  does; there is an open issue against them for it).
- **Key derivation:** a **separate passphrase**, distinct from the login
  password. Deriving from the login password would be zero-knowledge in name
  only — the server receives the raw password at login and could derive the
  key. Doing it properly (Bitwarden-style split derivation) would break the
  frozen `login-user` contract.
- **Key persistence:** a **non-extractable `CryptoKey` in IndexedDB**. Never a
  cookie — cookies are transmitted to the server on every request, which would
  silently destroy the zero-knowledge property.
- **Server is a stateless proxy** on the pull path. The plaintext access URL
  arrives per sync request, is used, and discarded. Never persisted, never
  logged.
- **Posted transactions only** on the pull path. `pending=1` stays off in v1
  (see Non-Goals). Push events are inherently tap-time (pre-posting); the
  cross-source matcher reconciles them with the posted reality (Part 5).
- **Account mapping is manual.** Nothing becomes a transaction until the user
  explicitly links an external account (bank account or Wallet card) to an
  Econumo account. Events for unmapped accounts are **queued, never dropped
  and never auto-created** (Part 6).
- **A transaction can carry multiple live links** — one per source that
  reported it (e.g. Apple Wallet tap + SimpleFIN posted record). Provenance is
  first-class: the user can see every source of a resulting transaction.
- **Deleting a transaction does not delete its import mappings** (tombstones),
  so deletions survive re-sync — from every source that ever saw it.
- **Rules cover all four classification axes** — category, payee, tag, and
  label (the reporting-labels second classification).
- **An optional LLM assists rule authoring — it never imports.** Imports stay
  deterministic; the model is invoked offline ("suggest rules"), reads the
  user's already-classified transactions, and proposes rule rows the user
  reviews/edits/accepts through the normal rules UI. Configured by
  `ECONUMO_AI_DSN` (instance-level, `MAILER_DSN` pattern); unset hides the
  feature entirely.
- **Queue triage is per-event as well as per-account.** A queued transaction
  opens the standard transaction dialog pre-filled for one-off import (no
  account link required), or can be skipped — durably: a skipped event counts
  as seen and never re-queues.

### Threat model — what this does and does not protect

**Protects:** database dumps, stolen backups, compromised replicas, a
rogue/compelled operator. The operator cannot decrypt what users have
encrypted.

**Does not protect:** the sync call itself. SimpleFIN is server-to-server HTTP
Basic; the browser cannot call it directly (CORS, and it would ship bank
credentials into JS). The server therefore handles a plaintext access URL in
memory during each sync. A compromised *running* server can capture
credentials as users sync.

The honest summary is **at-rest zero-knowledge, in-flight trusted**. This is
the correct trade for the realistic threat (leaked backup), but it is not
end-to-end encryption and must not be described as such in user-facing copy.

**Push-path threat:** the Shortcut stores its contents in plaintext (and syncs
via iCloud), so the embedded token is exposable. The `'ingest'` scope bounds
the blast radius: a leaked token can inject fake events into the user's
import queue — it cannot read balances or touch existing data. The token is
user-level, not source-bound; a leak covers all of that user's push
integrations. Accepted.

## Non-Goals (v1)

- **Scheduled/background sync.** Structurally impossible under client-side
  keys, and explicitly not wanted.
- **Pending transactions** on the pull path. Their ids mutate when they post;
  Actual gates this behind a preference and still carries open bugs. Deferred.
  (Push tap-time events are not "pending" rows — they are real transactions,
  later amount-corrected by the matcher.)
- **Balance reconciliation.** SimpleFIN returns `balance`, but Econumo derives
  balance from transactions rather than storing it. Comparing them is a useful
  "sync is complete" signal but has its own correction-transaction semantics.
- **Regex match rules.** A denial-of-service surface (catastrophic
  backtracking on user-supplied patterns evaluated per row).
  `exact`/`contains`/`prefix` covers the realistic cases.
- **Migrating the CSV importer into this package.** CSV *adopts* the shared
  dedupe ledger and run concept, but its parsing stays in
  `internal/transaction`.
- **Per-provider scopes.** A single `'ingest'` scope covers all `ingest-*`
  routes; a scope matrix adds ceremony for no realistic threat difference.

## Part 1 — Package & architecture

New feature package **`internal/imports`** — not `internal/import`, since
`import` is a Go keyword and cannot be a package name. The route namespace
`/api/v1/import/...` is a string and is unaffected.

Owns: sources, id mappings, the queue, rules, import runs, the reconciler,
the provider seam (pull), the ingest normalizers (push).

### Provider seam (pull)

```go
type Provider interface {
    ListAccounts(ctx context.Context, cred Credential) ([]model.ExternalAccount, error)
    FetchTransactions(ctx context.Context, cred Credential, req FetchRequest) ([]model.ExternalTransaction, error)
}
```

`internal/imports/simplefin` implements it. A future `enablebanking` package is
purely additive.

### Ingest seam (push)

Each push provider is an HTTP edge adapter in `internal/imports/api`: a
dedicated route whose handler validates the provider's exact payload and maps
it to the canonical internal event (`model.IngestEvent`), then calls the one
shared ingest service (dedupe → queue or create). Adding a push provider is
one route + one normalizer; the core does not change.

### Dependency rule

`imports` needs to create transactions and resolve accounts/categories/payees/
tags — all sibling features. Per the repository dependency rule it declares
consumer-side interfaces in its own `ports.go`, and `internal/server` wires
`glue_imports_*.go` adapters at composition time. `archtest` auto-detects new
feature packages, so this is enforced from the first commit.

**One deliberate exception**, see Part 4: the transaction list query embeds an
`EXISTS` subquery against `import_transaction_links` directly in SQL. This is
a knowing shortcut, to be refactored later.

### Persistence

Follows the engine-adapter (sqlc) pattern: a `querier` interface in the
canonical sqlite-generated types, with `sqlite.go` passthrough and `pgsql.go`
conversion shims selected once in the constructor.

## Part 2 — Schema

Six new tables plus one column on an existing table. All ids `TEXT`/UUIDv7
per the existing contract. Migrations paired under
`internal/infra/storage/migrations/{sqlite,pgsql}`.

### `access_tokens.scope` (existing table, new column)

`TEXT NOT NULL`, **no default in the final schema** — scope is always
provided explicitly. The per-engine migrations reach that state each in
their own idiom: PostgreSQL adds the column `NOT NULL DEFAULT 'full'` (which
backfills existing rows) and then `DROP DEFAULT`s it; SQLite has no
`ALTER COLUMN`, so its migration uses the standard table rebuild (the
migration runner already handles these — it toggles `foreign_keys` around
rebuilds, `internal/infra/storage/migrate/migrate.go:221-230`), writing
`'full'` as a literal in the `INSERT … SELECT`. In application code the
scope is explicit everywhere: sessions are minted `'full'` by the login code
path, and `create-personal-token` **requires** `scope` in the request — a
blank value fails validation. Requiring a previously-nonexistent field is a
deliberate contract change to `create-personal-token`: any external script
creating PATs must start sending `scope`. `'full'` = unrestricted (today's
behavior); `'ingest'` = the token authenticates **only** `ingest-*` routes;
unknown values fail closed, same posture as the password-algorithm dispatch.
Enforcement lives in the auth middleware: the authenticator already
resolves the token row, so it also returns the scope, and the middleware
rejects a scoped token on any non-matching route with the standard 401
envelope (`"Invalid access token"` — no scope information leaks). Ingest
routes otherwise ride the normal authenticated pipeline, so the readonly-402
check, rate limiting, and access logging apply for free.

`get-personal-token-list` returns the scope so the settings UI can badge
scoped tokens.

### `import_sources`

One connected integration per row.

```
id, user_id, provider ('simplefin'|'csv'|'apple-wallet'), name,
credential_ciphertext TEXT NULL,   -- opaque; the server never decrypts this
credential_kdf        TEXT NULL,   -- {alg, salt, iterations} — client metadata
status, last_synced_at, created_at, updated_at
```

The server treats `credential_ciphertext` as bytes to store and return, nothing
more. `credential_kdf` is metadata the *client* needs to re-derive its wrapping
key; the server cannot act on it either. CSV and push sources carry neither.

For push providers the source is resolved from `(user_id, provider)` — the
route names the provider, the token names the user — so the service layer
enforces **at most one source per (user, push provider)**. Two iPhones share
one Apple Wallet source; their cards distinguish the accounts. An ingest hit
with no source yet gets a clear 4xx telling the user to set up the
integration first.

### `import_events`

The raw inbox — stage 0 of the pipeline (Part 5). **Every** external
transaction enters here first, whatever the provider: the push request body
verbatim, the per-transaction JSON slice of a SimpleFIN response, the CSV
row. Nothing is interpreted before it is persisted, so nothing is ever
silently lost to a parse failure — and a later parser fix can retroactively
process old events (retry).

```
id, source_id, run_id NULL → import_runs(id),
payload TEXT,                      -- the raw datum, untouched
payload_hash,                      -- sha256(payload)
status ('processed'|'failed'|'discarded'),
parse_error TEXT NULL,             -- machine-readable reason when failed
received_at
UNIQUE(source_id, payload_hash)
```

The hash uniqueness is the first dedupe layer: an identical retry (Shortcuts
double-fire, HTTP retry, a re-synced date range returning byte-identical
slices) dies here as an idempotent no-op before parsing runs. It is a
*best-effort* layer — a provider may re-serialize the same transaction with
mutated bytes — so the ledger's external-id uniqueness below remains the
authoritative second layer. `run_id` ties events fetched during a batch run
to it, so run detail can list its parse failures; live push events carry
NULL. Raw payloads are user data like any other (payee names, amounts) and
are covered by the same at-rest posture as the rest of the database.

### `import_account_links`

The manual account mapping step. For pull providers an external account is a
bank account; for Apple Wallet it is the normalized card/pass name from the
event.

```
id, source_id, external_account_id, external_name, external_currency,
account_id           → accounts(id),
default_category_id  → categories(id) NULL,
is_enabled, created_at, updated_at
UNIQUE(source_id, external_account_id)
```

Nothing converts to transactions until a link exists. Unmapped external
accounts queue their events (Part 6) and are **never auto-created** — this
avoids the existing CSV failure mode in
`internal/transaction/import.go:424-442`, where a name matching an unwritable
shared account silently creates a duplicate own account.

`default_category_id` is a **convenience, not a correctness fix**: Econumo
allows nil categories on every transaction type
(`internal/model/transaction_dto.go:62` documents categoryId as optional;
there is no category-required check for non-transfers). A default at mapping
time simply means imports land pre-categorized when no rule matches. It is
optional; nil-category imports are legal, same as the REST path.

`external_currency` lets the link step refuse a mismatch against the Econumo
account's currency instead of silently importing wrong-currency amounts.

### `import_transaction_links`

The dedupe ledger and provenance record — the piece that generalizes across
every provider.

```
id, source_id, run_id NULL → import_runs(id),
event_id NULL → import_events(id),
external_account_id, external_transaction_id,
transaction_id TEXT NULL REFERENCES transactions(id) ON DELETE SET NULL,
status TEXT NOT NULL ('linked'|'queued'|'skipped'),

-- what the provider actually said (audit + rule debugging)
external_payee, external_description, external_amount, external_currency NULL,
external_posted_at,

-- what the import assigned / observed at link time (diff base for rule learning)
applied_category_id, applied_payee_id, applied_tag_id, applied_label_id,
applied_rule_id NULL,

imported_at

UNIQUE(source_id, external_account_id, external_transaction_id)
CREATE INDEX ... ON import_transaction_links(transaction_id);
```

Row states:

- `status='queued'`, `transaction_id NULL` — event received, account not yet
  mapped (or currency-mismatched); no transaction exists. Convertible.
- `status='linked'`, `transaction_id` set — a live link. A transaction may
  have **several** live links, one per source that reported it; the plain
  index on `transaction_id` serves the lookups (there is deliberately no
  uniqueness on it).
- `status='linked'`, `transaction_id NULL` — a **tombstone**: imported, then
  the transaction was deleted. Derived, not written: the FK cascade nulls
  `transaction_id` but cannot touch `status`, which is exactly why the
  tombstone must stay a derived state — it requires zero Go coupling on the
  delete path.
- `status='skipped'`, `transaction_id NULL` — the user triaged the queued
  event away without importing it. Counts as **seen** in the matcher, so it
  never re-queues on a future sync.

The composite unique is **required, not stylistic**: SimpleFIN transaction ids
are unique *within an account*, not globally.

`event_id` points back to the raw datum that produced the row, completing
the provenance chain transaction → link → raw payload. Nullable in the
schema, but every row the pipeline creates sets it.

`external_currency` is carried on the link row for queued push events, which
have no account link yet to hold a currency; pull rows leave it NULL (the
account link owns it).

The `applied_*` columns snapshot the transaction's classification as of that
link's creation: a creating link records what rules/defaults assigned; an
adopting link (Part 5) records the transaction's values at adoption time.
Rule learning (Part 7) diffs against the **most recent** link's snapshot.

**External ids.** SimpleFIN supplies one. CSV rows and Apple Wallet events do
not, so synthesize: CSV uses `sha256(date|amount|description|row-ordinal)`;
push uses `sha256(external_account|occurred_at|amount|merchant)`, overridable
by an explicit client `eventId` (Shortcuts has a UUID action). Same table,
same uniqueness constraint — re-uploading a CSV, a Shortcuts automation
double-fire, and an HTTP retry all dedupe to "already seen" for free,
retiring the "no idempotency at any level" gap
(`internal/transaction/ports.go:116-117` documents `SaveTransaction` as
explicitly having no idempotency id). The repo's shared operation guard
(`internal/infra/operation`) stays untouched; the ledger is the imports-wide
dedupe mechanism.

### `import_runs`

Makes each batch import an auditable, undoable unit.

```
id, user_id, source_id, provider,
params TEXT,                       -- {start_date, end_date, account_links[]} | {mapping}
status ('running'|'completed'|'failed'|'partial'),
imported_count, matched_count, skipped_count, failed_count,
started_at, finished_at
```

Three kinds of runs share the table: pull syncs, CSV uploads, and **queue
conversions** (mapping an account converts its queued events as a run, so
history/undo/audit come along for free). Individual live push events belong
to no run — their link's `run_id` is NULL.

### `import_rules`

Category/payee/tag/label mapping.

```
id, user_id, source_id NULL,       -- NULL = applies to every source
match_field ('description'|'external_payee'|'external_category'),
match_type  ('exact'|'contains'|'prefix'),
match_value, is_case_sensitive,
target_category_id NULL, target_payee_id NULL, target_tag_id NULL,
target_label_id NULL,
priority, created_at, updated_at
```

**One row, multiple nullable targets** — not one row per target kind. Editing a
transaction typically changes payee *and* category together, and that must
produce a single rule with a single priority, not two rows that can later
disagree. `NULL` target means "leave alone".

A plain dictionary mapping ("external category `Groceries` → my Food category")
is just `match_field='external_category', match_type='exact'`. Pattern rules
("description contains `STARBUCKS`") use `contains`. One table covers both.

## Part 3 — Deletion semantics

Transactions are **hard-deleted** in Econumo (`DELETE FROM transactions WHERE
id = ?`, `internal/transaction/repo/repo.go:203-205`) — unlike accounts, which
soft-delete. A link row would therefore genuinely dangle.

| Action | Transaction | Links | Re-imports on next sync? |
|---|---|---|---|
| Delete transaction (default, from anywhere) | deleted | **all kept, tombstoned** | no — from no source |
| Delete one mapping ("allow re-import") | kept | that link deleted | yes — that source re-creates a duplicate |
| Delete both (from run view) | deleted | all deleted | yes |
| Delete entire run | all deleted | all deleted (incl. sibling links, tombstoned) | see below |

**Why tombstone rather than cascade.** The dedupe ledger is the only memory
that an external transaction was ever seen. Delete the link and the next sync
sees a brand-new external id and re-imports it — the classic
zombie-resurrection bug. Tombstones count as seen, per source, forever.

**Multi-link makes this stronger:** deleting a transaction cascades NULL onto
**all** its links, so it cannot resurrect from *any* source that ever saw it.

**`ON DELETE SET NULL` requires zero cross-feature coupling.** The alternative
— `transaction` calling into `imports` on delete — would violate the
dependency rule and need a port plus a glue adapter. The FK does it in the
database, atomically, inside the existing `s.tx.WithTx` block in
`internal/transaction/delete.go`, with **no Go changes to the transaction
feature at all**. `foreign_keys = ON` is confirmed on the production SQLite
connection (`internal/infra/storage/sqlite/sqlite.go:49`, single pinned
connection; the migration runner toggles it only around table rebuilds and
restores it) — implementation adds a regression assertion, and engine parity
on the cascade belongs in `enginecompare`.

### Guardrails

- **"Delete mapping only" is a footgun.** It undoes nothing; it re-imports the
  transaction next sync, producing a *duplicate* alongside the one kept. It
  must be worded as **"allow re-import"**, and with multi-link it is per-link
  ("allow re-import from SimpleFIN").
- **"Delete entire run" can destroy real work.** Imported transactions get
  edited, and with multi-link a run's transactions may be co-owned by other
  sources. The confirmation must count both: transactions modified since
  import (`transactions.updated_at > import_transaction_links.imported_at`)
  and transactions holding live links from *other* sources — *"12
  transactions; 4 edited since import; 3 also linked from SimpleFIN (a bank
  re-sync will NOT restore them)."*

## Part 4 — Marking imported transactions in the list

The transaction list exposes `isImported` (int `0`/`1`, per the wire
contract) so the UI can badge the row. **Implementation: an `EXISTS` subquery**
from the transaction list query into `import_transaction_links` — no join, so
multiple links cannot duplicate rows, and no uniqueness constraint is needed
for correctness.

Full provenance lives on transaction-open: `get-transaction-import-list`
returns every link for the transaction (provider, source, run, external
payee/amount/date, applied snapshot). The rule-learning flow (Part 7) already
fetches links on open, so this is one endpoint doing double duty. With
multiple links a single `importRunId` in the list would be ill-defined —
another reason the list carries only the flag.

Consequences to accept:

- The subquery reads a table another feature owns. `archtest` will **not**
  catch this coupling — it is SQL, not a Go import. The query files must carry
  a comment marking the cross-feature reference so the later refactor is
  greppable.
- Two query files (`query/{sqlite,pgsql}`) plus `sqlc generate`.
- `TransactionResult` gains `isImported`, so **every golden file changes**.
  Additive and safe for clients (unknown fields ignored), but per the repo
  convention a golden diff means observable behavior changed — regenerate with
  `UPDATE_GOLDEN=1` and review the diff deliberately.

## Part 5 — Sync flow & reconciler

### The pipeline

Every external transaction, from every provider, moves through the same four
stages; only stages 0–1 have provider-specific code:

0. **Receive & store** — persist the raw datum into `import_events` before
   any interpretation. Byte-identical duplicates (payload hash) stop here as
   idempotent no-ops.
1. **Parse** — the provider's parser turns the raw datum into a canonical
   internal event (account label, amount, currency, timestamp, payee,
   description, external id or the synthesized hash). Failure → the event is
   marked `failed` with a machine-readable `parse_error`, surfaced in the UI
   with the raw payload visible, and **retryable** — a parser bug-fix in a
   later release makes old failures importable.
2. **Match account** — the parsed account label is looked up against the
   source's `import_account_links`: **exact match, case-insensitive,
   whitespace-normalized — never fuzzy, never auto-created** (a wrong
   account guess is far worse than a queued event). No link → ledger row
   `status='queued'`. Currency mismatch → queued with a visible error.
3. **Match transaction & import** — the reconciler below; on create, apply
   `import_rules` by priority, else the link's `default_category_id`.

### Pull flow

1. Client unlocks the data key from IndexedDB (prompting for the passphrase
   only if absent) and decrypts the access URL.
2. Client `POST`s the access URL + `source_id` + date range.
   **Never persisted, never logged.**
3. Server opens an `import_runs` row and calls
   `GET {accessUrl}/accounts?start-date=…` — one call returns accounts *and*
   their nested transactions.
4. Per external account, **in its own DB transaction**: store each
   transaction's raw JSON slice as an `import_events` row (stage 0), parse,
   then — mapped & enabled → reconcile → apply rules → insert transactions +
   links; **unmapped → store the transactions as queued links** (Part 6)
   instead of discarding them. Parse failures are recorded on the run and do
   not kill it.
5. Discard the credential. Finalize the run. Return
   `{runId, imported, matched, amountsUpdated, queued, skipped, failed, errors}`.

### The matcher

Runs for every batch source (pull sync, CSV, queue conversion), per event:

1. **Exact** — `(source_id, external_account_id, external_transaction_id)`
   already present in links with `status='linked'` or `'skipped'` → skip.
   **Tombstones and user-skips count as seen**, which is what makes deletions
   and triage decisions stick. (`status='queued'` rows are not "seen" — they
   are the queue.)
2. **Fuzzy adopt** — no link, but an existing transaction in the same account
   has the same amount within ±3 days. Do not create a duplicate; **add a link
   to the existing transaction** and count it as `matched`. This is what makes
   the first sync tolerable for anyone who has been entering transactions
   manually or via CSV — otherwise every manual entry gets a twin.
3. **Cross-source adopt (the tip problem)** — no exact-amount match, but a
   transaction on the same account whose live links include a *push* source
   matches all of:
   - bank date within **+5 days after** the tap (posting is strictly later);
   - amount within **±20%** of the tap amount (tips, gas holds, FX);
   - **merchant token-containment**: normalize both strings (lowercase, strip
     non-alphanumerics, split on whitespace), match when every ≥3-char token
     of the push merchant appears in the bank payee/description token set —
     `starbucks` ⊂ `sq starbucks 12345 seattle wa`. Deterministic; no regex,
     no fuzzy library.
   Best candidate = smallest amount delta, then nearest date. On match: add
   the bank link, and **correct the amount to the posted amount** — but only
   if the transaction's current amount still equals the push link's
   `external_amount` (the user hasn't edited it; if they have, their number
   wins and we just link). Corrections are counted (`amountsUpdated`) and
   surfaced in the run summary; the tap-time amount remains auditable on the
   push link.
4. **Create** — otherwise a new transaction plus its link.

Actual's payee-based fuzzy stage is deliberately dropped: their stage 2
(amount + date + payee) and stage 3 (amount + date) differ only by a payee
check that on first sync is almost always absent, adding a branch without
adding discrimination. Stage 3 here is different — it exists to reconcile
tap-time push events with posted bank records, a case Actual does not have.

### Field mapping (pull)

- **Amount/sign** — SimpleFIN `amount` is a decimal string, positive = deposit
  → `income`; negative → `expense`. Store `abs()`; Econumo encodes sign in
  `TransactionType`. Decimal string end to end, no float, consistent with the
  decimal-on-the-wire contract.
- **Date** — `posted` (unix) converted via the caller's `X-Timezone`, then
  formatted `2006-01-02 15:04:05`. **This needs a deliberate decision**: a
  UTC-naive conversion puts late-evening transactions on the wrong day for
  anyone west of UTC, which surfaces as wrong-day budget attribution.
- **Payee** — `payee` when the bridge supplies it, else `description`.
- **Category** — rules by priority, falling back to the link's
  `default_category_id`; nil when neither applies (legal everywhere in
  Econumo).

### Failure handling

SimpleFIN returns a structured `ErrorList` with `gen`/`con`/`act` scoping.
Per-account errors must not kill the run: record them, mark the run `partial`,
keep the accounts that succeeded. Per-account DB transactions mean a mid-run
failure leaves completed accounts durably imported and safely re-syncable —
the links make retry idempotent.

### Logging

The sync and ingest endpoints must be excluded from any body logging; the
access URL and merchant strings must never reach a log attribute. The
existing access log records UUIDs only (no bodies, no query strings), so the
default is already safe — but this needs an explicit regression test rather
than reliance on convention.

## Part 6 — Push ingestion & the queue

### `POST /api/v1/import/ingest-apple-wallet-event`

Bearer: a PAT with the `'ingest'` scope (a full-scope token also works — the
scope *restricts*, membership in the pipeline is normal). Payload, shaped by
what the Wallet trigger provides:

```json
{
  "account":    "Amex Platinum",
  "payee":      "Starbucks",
  "amount":     "4.50",
  "currency":   "USD",
  "occurredAt": "2026-08-15T10:42:03-07:00",
  "type":       "expense",
  "eventId":    "8B7C…"
}
```

Behavior: authenticate (scope check) → resolve the source from
`(user, 'apple-wallet')` (none → clear 4xx: set up the integration first) →
**persist the raw body as an `import_events` row** → run the pipeline
inline (parse → match account → match transaction & import; no job queue —
this is a sub-second single-row path). The event row is the audit and retry
substrate, not an async work queue.

Parse rules (v1):

| Field | Rule | On failure |
|---|---|---|
| `account` | trim, collapse inner whitespace; used verbatim as the external account id | missing/empty → parse failure |
| `amount` | decimal string, tolerant normalization: strip currency symbols and spaces; `.` and `,` both accepted, the **last** separator is the decimal point (`1.234,56` → `1234.56`); must yield a positive decimal | unparseable → parse failure |
| `currency` | 3-letter ISO code, uppercased; symbols rejected (`$` is ambiguous — the recipe sends the trigger's currency code) | missing/invalid → parse failure |
| `occurredAt` | RFC3339 **with offset** — the device's local time; the wall-clock date derives from the event's own offset, not `X-Timezone` (a Shortcut sends no meaningful header), stored in the frozen `2006-01-02 15:04:05` format | missing/invalid → **fallback to `received_at`** — a lost timestamp must not lose a purchase; taps reach the server within seconds |
| `type` | `expense`/`income`, optional, default `expense` (Wallet amounts are positive for charges) | unknown value → parse failure |
| `payee` | trim; empty allowed (description is the fallback) | never fails |
| `eventId` | optional; overrides the synthesized external id | — |

The synthesized external id is computed from **parsed** values —
`sha256(account|occurredAt|amount|payee)` — so it stays stable across
retries even when raw bytes differ (field order, whitespace).

Response: `{status: "created"|"queued"|"duplicate"|"failed"}` — always
HTTP 200 once the event is persisted. A 4xx is invisible to an unattended
automation; a failed event surfaces in the web UI (queue page, "needs
attention"), where the user sees the raw payload and can retry or discard.

Rate limit: `ECONUMO_RATE_LIMIT_INGEST`, per user, every request counts,
following the `accept-invite` precedent.

### The queue

One queue across all providers: `status='queued'` links, whatever their
source. Pull syncs feed it (transactions of unmapped bank accounts), push
events feed it (unmapped cards), and currency-mismatch holds land in it.
The queue page also carries a **"needs attention"** section for `failed`
events (parse errors) — raw payload visible, retry / discard per row.
`unlink-account` / `delete-source` purge their queued rows and events.

**Mapping converts.** Creating an account link converts that external
account's queued events in one batch — a **conversion run** (`import_runs`
row, `params` = the mapping) through the full matcher, per-account DB
transaction, counts reported. Cross-provider duplicates resolve here
naturally: the first source's conversion creates the transaction, the second
source's conversion adopts it (stage 2 or 3), ending with one transaction,
two links.

**Per-event triage.** Tapping a queued transaction opens the **standard
transaction dialog pre-filled** from the parsed values, with rules
pre-applied as defaults — every field editable, including the account picker,
so a one-off import needs **no account link at all**. Saving imports that
single event through the matcher (its link becomes `linked`, run-less;
`applied_*` snapshot = what the user saved). **Skip** flips the link to
`status='skipped'` — seen forever, never re-queued; visible under the run/
source detail with an "un-skip" (back to queued) escape hatch.

**At-rest note:** queued pull events keep bank amounts/payees for accounts
the user may never map. Accepted for v1 (simplicity, and the user sees and
controls the queue); revisit if it proves objectionable.

## Part 7 — Rule learning from edits

Nobody authors mapping rules up front; they emerge from correcting the first
sync. This is the loop that populates `import_rules`.

**Detection is client-side.** The SPA knows a transaction is imported (it has
`isImported`), fetches its links on open (`get-transaction-import-list`), and
on save diffs the current values against the **most recent** link's
`applied_category_id` / `applied_payee_id` / `applied_tag_id` /
`applied_label_id` snapshot. If a target changed, it offers a rule.

This deliberately leaves `update-transaction` untouched — no change to a frozen
contract, and an advisory suggestion does not belong in a write path.

**Only prompt when the diff is against `applied_*`.** Re-editing an
already-corrected transaction must not re-prompt, and editing an amount or date
must not prompt at all — those are not rule-able targets.

### The match value must be user-editable

Bank descriptions are noisy:

```
SQ *STARBUCKS 12345 SEATTLE WA 03/14
```

A naive `contains` on the full string matches exactly once, ever. So: propose a
normalized candidate (strip trailing dates, reference numbers, store numbers,
trailing state codes; take the longest alphabetic run), **pre-fill it into an
editable field, and show a live count of how many transactions in the run it
would match.** The count is what makes a bad guess obvious before it is saved.
Token extraction from bank strings is unreliable and the UI must not pretend
otherwise. (The normalizer is the same one stage 3 and the queue grouping use,
shipped once in `web/src/lib` and mirrored in Go.)

`applied_rule_id` lets the UI offer *"update the existing rule"* instead of
stacking a near-duplicate — which is how rule sets rot in tools that skip this.

### Backfill

```
POST /api/v1/import/preview-rule  → { matched: 7, alreadyEdited: 2 }
POST /api/v1/import/apply-rule    → { ruleId, scope: 'run'|'source', runId }
```

Preview always precedes apply — this rewrites existing transactions.

- **Skip transactions the user has already edited** (current ≠ `applied_*`) by
  default. Backfilling must not overwrite a manual correction with a
  generalization inferred from a *different* transaction. Surface as
  "2 skipped (you've edited these)" with an opt-in override.
- **Scope defaults to the current run.** "All imports from this source" is a
  second, clearly-labelled step — it can touch a lot of history.

Flow: save edit → *"You changed the category to Coffee. Create a rule?"* →
editable match value + live match count → *"Apply to 7 matching transactions in
this import?"* → apply, or save for future syncs only.

### LLM-suggested rules

Edit-driven learning captures corrections one at a time; an optional LLM
bootstraps a whole ruleset at once from what the user has already
classified — manually added transactions and import-corrected ones alike.

**The model authors rules; it never imports.** The import path stays fully
deterministic. `POST /api/v1/import/suggest-rules` gathers the user's
classification history (external payee/description strings paired with the
category/payee/tag/label the user chose, plus the names of the user's
existing categories/payees/tags/labels), sends **one** completion request,
and returns proposed rule rows. The suggestions are **transient** — nothing
persists until the user accepts a row, which goes through the ordinary
`create-rule`; accepted, edited, or discarded per row in the rules UI, with
the same live match-count preview as edit-driven rules. There is no
"suggested" state in the schema and no path for unreviewed model output to
touch data. The server validates every proposed rule before returning it:
target ids must belong to the user (models hallucinate ids — invalid rows
are dropped), match types must be in the `exact`/`contains`/`prefix` set.

Configuration is a boot-parsed DSN, exactly the `MAILER_DSN` pattern:
`ECONUMO_AI_DSN` — e.g. `openai://<api-key>@api.openai.com?model=gpt-5-mini`.
The transport speaks the OpenAI-compatible chat-completions API, so hosted
providers and local models (Ollama, LM Studio — keyless
`openai://localhost:11434?model=…`) both work; a bad scheme fails at boot;
unset (default) disables the feature and hides its UI (an `AI_ENABLED` flag
merged into the served `econumo-config.js`, same mechanism as
`BILLING_URL`). Privacy is explicit and structural: payee/description
strings leave the instance only when the admin configured an endpoint AND
the user pressed the button — never in the background.

**Rejected alternative: per-transaction LLM classification at import time.**
Non-deterministic imports, per-sync cost and latency, an unauditable
decision trail, and a hard dependency of the core flow on an external
service. Ruleset generation delivers the same categorization benefit while
keeping the model out of the data path.

## Part 8 — Endpoints

All under `/api/v1/import/`, following the `{module}/{action}-{subject}`
convention. `GET` for reads, `POST` for every write.

| Method | Route | Purpose |
|---|---|---|
| POST | `claim-setup-token` | Exchange a SimpleFIN setup token for an access URL. Returned to the **client**, never stored. |
| POST | `create-source` | Store a source; SimpleFIN sends `credential_ciphertext` + `credential_kdf`, push providers neither. |
| GET  | `get-source-list` | Sources with status and `last_synced_at`. |
| POST | `delete-source` | Remove a source, its links, and its queued rows. |
| POST | `list-external-accounts` | Proxy: client supplies access URL, server returns accounts for mapping (pull). |
| POST | `link-account` | Map external account → Econumo account (+ optional default category). Triggers the conversion run for queued events. |
| POST | `unlink-account` | Also purges the account's queued rows. |
| POST | `sync` | The pull flow. Client supplies access URL. |
| POST | `ingest-apple-wallet-event` | Push ingest (Part 6). Requires `'ingest'`-or-full scope. |
| GET  | `get-queued-event-list` | Queued links + failed events for the queue page. |
| POST | `import-queued-event` | One-off import of a single queued event via the pre-filled dialog (no account link required). |
| POST | `skip-queued-event` | Durable skip (`status='skipped'`); `unskip-queued-event` reverses. |
| POST | `retry-event` | Re-run the pipeline for a failed event. |
| POST | `discard-event` | Mark a failed event discarded. |
| POST | `suggest-rules` | LLM ruleset proposal (Part 7); 501-style 400 when `ECONUMO_AI_DSN` is unset. |
| GET  | `get-run-list` | Import history (syncs, CSV uploads, conversions). |
| GET  | `get-run` | Run detail: links, including tombstones. |
| GET  | `get-transaction-import-list` | All links for one transaction (provenance + rule-learning base). |
| POST | `delete-run` | Delete transactions + mappings for a run (guardrails in Part 3). |
| POST | `delete-link` | "Allow re-import" for a single link. |
| GET  | `get-rule-list` | |
| POST | `create-rule` / `update-rule` / `delete-rule` | |
| POST | `preview-rule` | Match counts before applying. |
| POST | `apply-rule` | Backfill. |

Outside `/import`: `create-personal-token` gains a **required** `scope`
field (`'full' | 'ingest'`); `get-personal-token-list` returns it.

The two-step claim (`claim-setup-token` returns the access URL to the client,
which encrypts it and calls `create-source`) is what keeps the server from ever
persisting plaintext. The claim itself is one-time: a 403 means the token was
already claimed or never existed.

### Rate limiting

`claim-setup-token`, `sync`, and `suggest-rules` are outbound-request
endpoints; `ingest-apple-wallet-event` is an unattended write. All four
carry per-user caps in the existing `ECONUMO_RATE_LIMIT_*` family, following
the `accept-invite` precedent (`suggest-rules` deserves a tight cap — each
call is a paid completion).

## Part 9 — Frontend

### Settings → Data

A new Settings subpage consolidating every way data enters or leaves Econumo:

- **CSV** — the existing import/export dialogs
  (`web/src/features/transactions/{ImportCsvDialog,ExportCsvDialog}.tsx`)
  surfaced here (the transaction-list entry points may stay; this page is the
  canonical home).
- **SimpleFIN** — connect (paste setup token, set/enter encryption
  passphrase, claim, encrypt, store), unlock (passphrase prompt when the
  IndexedDB key is absent; "forget this device" clears it), account mapping,
  sync trigger, run history.
- **Apple Wallet** — setup flow below, card mapping, activity.

### Apple Wallet setup — the pre-configured shortcut

The app ships the shortcut; the user should not build one by hand. The setup
page: create the `'ingest'`-scoped PAT (shown once) → **"Get the Shortcut"**
link serving the bundled, **signed** `.shortcut` file → on import, the
shortcut's **import questions** prompt for exactly two values: the server URL
and the token. The user pastes, enables the Wallet "Transaction" automation
("Run Immediately"), done.

Mechanics to verify during implementation (open question 5): iOS refuses
unsigned shortcut files, so the artifact is signed once with
`shortcuts sign --mode anyone` (macOS-only tooling), committed to the repo
(`web/public/econumo-apple-wallet.shortcut`), and served by the SPA like any
static asset; regeneration is a documented manual step when the recipe
changes. If import questions prove unable to carry the values, fallback is a
first-run in-shortcut setup prompt that stores them in the shortcut's own
storage. If file signing proves unworkable, fallback is a hosted iCloud share
link (external dependency, documented).

### Queue page + banner

- A **banner** on the main screens whenever the queue is non-empty — same
  pattern as the sharing-request banner — linking to the queue page.
- The **queue page** resembles the transaction list: queued events with their
  provider badge, grouped **client-side** when they plausibly describe the
  same purchase across providers (amount + date proximity + merchant-token
  overlap; the shared normalizer). Per external account: "map this account"
  → link dialog → conversion run → result summary. Per row: **tap → the
  standard transaction dialog pre-filled** (parsed values, rules as
  defaults, account picker free) → save = one-off import; or **skip**
  (durable, un-skippable from source detail). A "needs attention" section
  lists parse-failed events with raw payload, retry, discard.

### Other surfaces

- **Run list / run detail** — history; detail shows every link including
  tombstones ("transaction deleted, mapping kept"), with per-row
  delete-transaction / allow-re-import / delete-both.
- **Transaction provenance** — the transaction dialog shows the import badges
  and, on open, each link's source/run/external values (one purchase, two
  sources reads as e.g. "Apple Wallet 08-15 · SimpleFIN 08-17, amount updated
  4.50 → 5.40").
- **Rule prompt** — the Part 7 flow.
- **Rules management** — list, edit, reorder by priority; a **"Suggest
  rules"** action (visible only when the instance advertises `AI_ENABLED`)
  runs the Part 7 LLM flow and presents proposals for per-row
  accept/edit/discard with live match counts.

Crypto lives in `web/src/lib/importCrypto.ts`: PBKDF2 (Web Crypto native) or
Argon2id (WASM) → wrapping key → unwrap a random AES-GCM data key → store
**non-extractable** in IndexedDB.

Why non-extractable matters:

| Mechanism | Sent to server | XSS can exfiltrate | Persists |
|---|---|---|---|
| Cookie | **every request** | — | yes |
| `localStorage` | no | **yes** | yes |
| `sessionStorage` | no | yes | tab only |
| **IndexedDB, non-extractable** | **no** | **no** | yes |

XSS can *use* a non-extractable key while on the page but cannot exfiltrate the
raw bytes. Browsers may evict IndexedDB under storage pressure, and clearing
site data wipes it — both mean "re-enter the passphrase", which is recoverable,
not data loss.

### Analytics

Per the repository rule, every new user-facing action fires an event. New
`METRICS` keys: `IMPORT_SOURCE_CONNECT`, `IMPORT_ACCOUNT_LINK`,
`IMPORT_SYNC`, `IMPORT_QUEUE_MAP`, `IMPORT_QUEUE_IMPORT`,
`IMPORT_QUEUE_SKIP`, `IMPORT_SHORTCUT_DOWNLOAD`, `IMPORT_RUN_DELETE`,
`IMPORT_RULE_CREATE`, `IMPORT_RULE_APPLY`, `IMPORT_RULES_SUGGEST`. Fired at the
shared hook choke point so every surface is covered once. (Server-side ingest
hits are not SPA metrics.) `metrics-coverage.test.ts` fails the suite if a
key is never fired.

### i18n

New `imports.*` namespace in `locales/{en,ru}.json`. Any new error codes need
`errs.AllCodes` registration plus `errors.*` catalogue entries in **both**
languages — the two-way coverage guard in `internal/test/i18ntest` enforces it.

## Testing

- Unit: matcher stages (incl. cross-source adopt: tolerance windows, token
  containment, amount-correction-only-if-unedited), rule matching/priority,
  SimpleFIN response normalization, parse rules (amount locale normalization,
  timestamp fallback, failure taxonomy) + synthesized-id stability,
  event-level dedupe vs ledger dedupe, retry-after-parser-fix,
  amount/sign mapping, timezone/date conversion on both paths.
- Repo: engine-adapter coverage for all five tables; `make test-repo-pgsql`.
- **`apiparity`**: every new route needs a scenario — the guard tests enforce
  that route and scenario counts never shrink. Existing goldens change from
  the `isImported` addition and the PAT `scope` field.
- **`enginecompare`**: delete-then-resync, tombstone behavior, multi-link
  adoption, queue conversion, and the FK cascade must be byte-identical
  across engines.
- Auth: scoped-token enforcement (ingest token on a non-ingest route → the
  exact 401 envelope; full token on ingest → allowed; readonly user → 402).
- LLM seam: the completion transport is an interface, stubbed in tests (no
  network); suggestion validation (hallucinated target ids dropped, match
  types constrained, malformed model output → clean error, never a 500).
- Security regression: the access URL must never appear in logs or in any
  persisted column; ingest bodies never logged; production SQLite
  `foreign_keys = ON` assertion.
- Frontend: vitest for crypto round-trip, rule-value extraction, queue
  grouping, mapping validation. Run `pnpm exec tsc -b` — vitest and oxlint do
  not type-check.

## Open questions

1. **Timezone for pull `posted`** — confirm caller-`X-Timezone` conversion is
   right versus storing the bank's own date verbatim. (Push is settled: the
   event's RFC3339 offset.)
2. **CSV run integration** — the SPA uploads CSV in sequential 500-row chunks
   (`web/src/features/transactions/importCsv.ts:5`). Making a run span an
   upload means creating the run first and threading `run_id` through every
   chunk, finalizing at the end. Real scope; possibly a follow-up PR.
3. **Matcher constants** — ±3 days exact-adopt, +5 days / ±20% cross-source:
   unverified against real bank data; ship behind named constants.
4. **Multi-currency accounts** — v1 blocks a currency mismatch at link time
   and holds mismatched push events in the queue. Confirm versus converting.
5. **Shortcut distribution mechanics** — signed-file import questions carrying
   URL + token needs a device test before the flow is finalized (fallbacks in
   Part 9).
6. **Raw event retention** — `import_events` grows with every imported
   transaction across all providers. Keep forever (full audit) vs purge
   processed events after N days (a `token:purge`-style CLI command). Ship
   v1 keeping everything; decide before the table gets big.
7. **Suggest-rules prompt shape** — how much history fits one completion
   (cap the sample; most-frequent payee strings first), and whether the
   proposal quality justifies a second "refine" round-trip. Needs
   experimentation with a real catalogue; the seam isolates it.

## Deferred / follow-up

- Enable Banking provider (EU coverage; needs an eIDAS broker).
- Android push recipe (Tasker/Macrodroid) — the core already supports it; a
  new route + normalizer + docs.
- Passkey / WebAuthn PRF as an alternative key source. The wrapped-data-key
  design means this is just another way to unwrap the same data key — no
  re-encryption, no schema change.
- Refactor the Part 4 `EXISTS` subquery to a proper port + glue adapter.
- Pending-transaction support (pull).
- Balance reconciliation.
- Additional PAT scopes (e.g. read-only).
