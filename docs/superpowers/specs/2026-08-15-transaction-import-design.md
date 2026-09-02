# Transaction Import (SimpleFIN + Apple Wallet) — Design

**Date:** 2026-08-15
**Status:** Approved; open questions resolved 2026-09-01 (see Resolved
questions). Supersedes the 2026-07-19 SimpleFIN-only draft.

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
rules, import runs, the reconciler — are provider-agnostic. Any future
provider (Enable Banking, Pluggy, Akahu, Tasker/Macrodroid on Android) plugs
into them, and the existing CSV import adopts them in a later PR.

Two properties shape the pull side:

1. **Zero-knowledge at rest.** Econumo is offered as a hosted service. The
   server stores the provider credential as opaque ciphertext it cannot
   decrypt; the key never leaves the browser. A leaked database or backup
   exposes no bank access.
2. **Sync is triggered by the user's client, never scheduled by the server.**
   A server that cannot decrypt a credential cannot sync unattended — that is
   forced by (1), not a preference. v1 is a **Sync button** in Settings → Data
   that works in the web SPA at every viewport (the mobile shell is the same
   SPA); an automatic sync when the app opens with the key already unlocked
   is the planned follow-up, so that in practice the data is current whenever
   the app is looked at. Actual Budget's bank sync is likewise
   manual-trigger.

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
- **One passphrase per user, and no stored hint.** The passphrase unwraps a
  single data key that every source's credential is encrypted under, so a
  second bank needs no second passphrase and a passphrase change re-wraps one
  key. A hint would have to be server-side plaintext to be readable when it is
  needed, putting a clue to the passphrase beside the ciphertext; recovery is
  "reconnect with a fresh setup token" instead, which costs no data (Part 2).
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
- **A foreign-currency event is converted, not rejected.** Econumo
  transactions carry no currency of their own — the amount is implicitly in
  the account's currency (`internal/model/transaction_dto.go`). A tap in a
  currency other than the linked account's is therefore converted with the
  instance's stored rate for the event's date, with the original amount and
  currency preserved on the link row; no rate available means the event
  queues rather than guessing. A *whole external account* whose currency
  differs is a different thing — a misconfiguration — and is still refused at
  link time.
- **A link whose Econumo account is deleted goes dormant, it does not fail.**
  Accounts soft-delete, and since the deleted-account work landed, creating a
  transaction against one is refused (`transaction.account_deleted`) and
  deleting an account auto-writes a zeroing correction. An importer that kept
  writing to a deleted account would fight that invariant. So: deleting an
  account disables its `import_account_links`, and their incoming events queue
  like any other unmapped event — visible, re-mappable to another account, and
  never silently dropped or errored mid-run. The link is not deleted, so its
  ledger rows keep counting as seen and nothing resurrects.
- **Queue triage is per-event as well as per-account.** A queued transaction
  opens the standard transaction dialog pre-filled for one-off import (no
  account link required), or can be skipped — durably: a skipped event counts
  as seen and never re-queues. Per account, the choice is map **or ignore**:
  an ignored external account (a card, a bank account) skips everything it
  sends on arrival, so unwanted accounts never occupy the queue.

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

- **Server-scheduled sync.** Structurally impossible under client-side keys:
  the server holds no key to sync with. Caching the passphrase or the derived
  access URL in server memory to enable a daily job was considered and
  rejected — it would put a live bank credential in the heap for hours, where
  a core dump, swap, or a container/hypervisor snapshot can serialize it to
  disk, which attacks the one property this design actually promises. It is
  also unreliable in the failure mode that matters least visibly: any deploy
  or OOM empties the cache, so "daily" silently becomes "daily unless we
  shipped", and missing transactions are exactly what a user does not notice.
  Client-triggered sync (the Sync button now, sync-on-open as a follow-up)
  covers the same need without a server-side credential lifetime.
- **True background sync on mobile.** Waking the app on a timer needs a
  background-runner plugin and is deferred; sync-on-open/resume is itself a
  follow-up to the manual button (Part 9).
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
- **Anything touching the CSV importer.** It keeps its current behavior for
  now: no shared ledger, no runs, and re-uploading the same file still
  duplicates. Adopting the ledger is a later PR (resolved question 2), and
  even then its parsing stays in `internal/transaction`.
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

`imports` needs to create transactions, resolve accounts/categories/payees/
tags/labels, and convert foreign-currency amounts — all sibling features
(the convertor is `internal/currency`). Per the repository dependency rule it declares
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

Nine new tables plus one column on an existing table. All ids `TEXT`/UUIDv7
per the existing contract. Migrations paired under
`internal/infra/storage/migrations/{sqlite,pgsql}`.

### `access_tokens.scope` (existing table, new column)

`TEXT NOT NULL`, **no default in the final schema** — scope is always
provided explicitly. The per-engine migrations reach that state each in
their own idiom: PostgreSQL adds the column `NOT NULL DEFAULT 'full'` (which
backfills existing rows) and then `DROP DEFAULT`s it; SQLite has no
`ALTER COLUMN`, so its migration uses the standard table rebuild, writing
`'full'` as a literal in the `INSERT … SELECT`. A rebuild migration writes
plain `PRAGMA foreign_keys = OFF;` / `ON;` statements around its body and the
runner **hoists** them: OFF runs on a pinned connection outside the
transaction (the pragma is a silent no-op inside one), the connection's prior
enforcement state is restored afterwards whatever happens, and
`PRAGMA foreign_key_check` gates the result inside the transaction so a
violation rolls the migration back
(`internal/infra/storage/migrate/migrate.go:236-292`; `20260802000000.sql` is
a worked example). In application code the
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
status, last_synced_at, created_at, updated_at
```

The server treats `credential_ciphertext` as bytes to store and return, nothing
more. It is encrypted under the user's data key, not directly under a
passphrase — the key material and KDF parameters live once per user in
`import_credential_keys` below, not once per source. CSV and push sources carry
no credential at all.

For push providers the source is resolved from `(user_id, provider)` — the
route names the provider, the token names the user — so the service layer
enforces **at most one source per (user, push provider)**. Two iPhones share
one Apple Wallet source; their cards distinguish the accounts. An ingest hit
with no source yet gets a clear 4xx telling the user to set up the
integration first.

### `import_credential_keys`

The user's client-side key material, one row per user.

```
user_id TEXT PRIMARY KEY → users(id) ON DELETE CASCADE,
wrapped_data_key TEXT,   -- the AES-GCM data key, wrapped by the passphrase-derived key
kdf              TEXT,   -- {alg, salt, iterations} the client needs to re-derive
created_at, updated_at
```

**One passphrase per user, not per source.** The design already chose the
data-key indirection ("unwrap a random AES-GCM data key"), and the schema now
follows through: each source's credential is encrypted under that one data
key. Two consequences, both wanted — a user connecting a second bank does not
invent a second passphrase, and changing the passphrase re-wraps a single key
rather than decrypting and re-encrypting every source's credential.

The server stores `wrapped_data_key` and `kdf` as opaque client metadata,
exactly as it does `credential_ciphertext`; neither is actionable without the
passphrase.

No passphrase hint is stored. A hint would have to be server-side plaintext
to be readable at the only moment it is wanted — a new device, or after an
IndexedDB eviction — which puts a clue to the passphrase in the same database
as the ciphertext it protects, weakening the one guarantee this design makes.

Recovery is structural instead: forgetting the passphrase costs **no
financial data**. Already-imported transactions are ordinary rows and stay.
The user clears the unreadable credential and reconnects with a fresh
SimpleFIN setup token — the "forget this device / reconnect" path in
Settings → Data, which must therefore be obvious and documented rather than
an emergency escape hatch.

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
mode                 TEXT NOT NULL ('import'|'ignore'),
account_id           NULL → accounts(id),   -- NULL only when mode='ignore'
is_enabled, created_at, updated_at
UNIQUE(source_id, external_account_id)
CHECK (mode = 'ignore' OR account_id IS NOT NULL)
```

A link is either a mapping (`mode='import'`, account set) or an **ignore**
(`mode='ignore'`, no account): the external account is known and its events
are deliberately dropped. Ignoring is the answer to "I don't want this card /
this bank account in Econumo at all" — without it the only options are
skipping every event by hand forever, or excluding the card on the device.
`is_enabled` is orthogonal: it is the dormancy flag the deleted-account rule
flips (Decisions), and applies to `'import'` links only.

Nothing converts to transactions until a link exists. Unmapped external
accounts queue their events (Part 6) and are **never auto-created** — this
avoids the existing CSV failure mode in `findOrCreateAccount`
(`internal/transaction/import.go:422-440`), where a name matching an unwritable
shared account silently creates a duplicate own account.

#### Card → account for Apple Wallet

A user with several cards in Apple Pay wants each card's taps to land in a
chosen Econumo account. That is exactly what a link row is for the
`'apple-wallet'` source: the `account` field of the pushed payload is the
card's Wallet display name, the recipe copies it from the trigger's
"Card or Pass" property, and it becomes `external_account_id` verbatim (after
the parse-table normalization). The flow the user sees:

1. First tap from a card the server has not seen → the event queues, the
   banner appears.
2. Queue page → "map this card" → pick the Econumo account → link row
   created; the queued taps from that card convert in one run.
3. Every later tap from that card imports straight into the chosen account.
   Re-mapping is editing the link's `account_id`; existing transactions stay
   where they were imported.

Or, at step 2, **"ignore this card"**: the link is created with
`mode='ignore'`, the card's queued taps flip to `status='skipped'`, and every
later tap from it is recorded as `skipped` on arrival — seen, never queued,
never imported, never nagging. Ignoring is reversible from the card list
("map instead" turns the ignore into a mapping); taps skipped meanwhile stay
skipped, un-skippable one by one like any other skip. The same action exists
for a pull source's bank account (a mortgage or brokerage account SimpleFIN
returns that has no place in Econumo).

Two consequences of the card being identified **by name only**:

- **Renaming a card in Wallet** makes it a new external account: its taps
  queue again until the new name is mapped. The old link stays (history +
  `UNIQUE` slot); the user maps the new name to the same account.
- **Two cards with the same display name** are indistinguishable to the
  server. The fix is on the device: the Transaction trigger itself can be
  filtered to a single card, so the user builds **one automation per card**
  and types a distinct literal into `account` (e.g. `"Visa – joint"`)
  instead of using the trigger's property. The setup page documents this as
  the recipe variant for same-named cards; the server needs nothing extra.

There is deliberately **no per-link default category**. Econumo allows nil
categories on every transaction type (`internal/model/transaction_dto.go:62`
documents categoryId as optional; there is no category-required check for
non-transfers), so an unclassified import is legal, and leaving it nil is also
the better default: nil reads as "needs attention" and gets corrected, whereas
a plausible-but-wrong category silently pollutes the budget. `import_rules` is
the one classification mechanism; a second, weaker one competing with it would
only raise the question of which wins.

The case this gives up is blanket categorization of a single-purpose account
("everything on the fuel card is Fuel"), which rules cannot express — they
match on the description/payee/external-category strings, not on the account.
If that turns out to be wanted, the clean form is an optional account scope on
a rule, not a column here.

`external_currency` lets the link step refuse a mismatch against the Econumo
account's currency instead of silently importing wrong-currency amounts. This
is the *account-level* check — the whole external account being denominated
differently is a misconfiguration. A single foreign-currency event on an
otherwise correct link is a different case and is converted (stage 3).

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
applied_category_id, applied_payee_id, applied_tag_id,
applied_rule_id NULL,

imported_at

UNIQUE(source_id, external_account_id, external_transaction_id)
CREATE INDEX ... ON import_transaction_links(transaction_id);
```

Row states:

- `status='queued'`, `transaction_id NULL` — event received, account not yet
  mapped, or its currency could not be converted for want of a rate; no
  transaction exists. Convertible.
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

The `applied_*` columns — plus the link's `import_link_applied_labels` rows —
snapshot the transaction's classification as of that link's creation: a creating link records what rules/defaults assigned; an
adopting link (Part 5) records the transaction's values at adoption time.
Rule learning (Part 7) diffs against the **most recent** link's snapshot.

**External ids.** SimpleFIN supplies one. CSV rows and Apple Wallet events do
not, so synthesize: CSV (when it adopts the ledger) uses
`sha256(date|amount|description|row-ordinal)`; push uses `sha256(external_account|occurred_at|amount|merchant)`, overridable
by an explicit client `eventId` (Shortcuts has a UUID action). Same table,
same uniqueness constraint — a Shortcuts automation double-fire, an HTTP
retry, and (once CSV adopts the ledger) a re-uploaded CSV all dedupe to
"already seen" for free,
retiring the "no idempotency at any level" gap
(`internal/transaction/ports.go:130-131` documents `SaveTransaction` as
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

Category/payee/tag/label mapping, or a pattern to skip.

```
id, user_id, source_id NULL,       -- NULL = applies to every source
action      ('classify'|'skip'),
match_field ('description'|'external_payee'|'external_category'),
match_type  ('exact'|'contains'|'prefix'),
match_value, is_case_sensitive,
target_category_id NULL, target_payee_id NULL, target_tag_id NULL,
priority, created_at, updated_at
CHECK (action = 'classify' OR (target_category_id IS NULL AND target_payee_id IS NULL AND target_tag_id IS NULL))
```

**One row, multiple nullable targets** — not one row per target kind. Editing a
transaction typically changes payee *and* category together, and that must
produce a single rule with a single priority, not two rows that can later
disagree. `NULL` target means "leave alone"; the label equivalent is having no
`import_rule_labels` rows.

A plain dictionary mapping ("external category `Groceries` → my Food category")
is just `match_field='external_category', match_type='exact'`. Pattern rules
("description contains `STARBUCKS`") use `contains`. One table covers both.

Classify rules only ever **add** classification. A rule that clears a
category or strips a label is not expressible in v1 and is not wanted: a rule
fires on every future import of a matching string, so a subtractive rule would
silently undo the user's own later corrections.

#### Skip rules

`action='skip'` is the third layer of "I don't want this", after per-event
skip (triage) and per-account ignore (link mode): **by pattern** — credit-card
payments that are already the other side of a transfer ("PAYMENT - THANK
YOU"), Apple Cash top-ups, a subscription tracked elsewhere. A skip rule has
no targets and no `import_rule_labels` rows (the CHECK covers the columns; the
service refuses label rows on a skip rule).

Semantics, kept deliberately blunt:

- Evaluated in stage 3, right after currency conversion and **before the
  matcher**: a matching skip rule ends the pipeline with a ledger row
  `status='skipped'`, `applied_rule_id` = the rule, no transaction — the same
  row a hand skip produces, so it counts as seen and never re-queues. Skip
  rules therefore act only on events that reached stage 3, i.e. whose account
  is mapped; an unmapped account's events queue regardless.
- **A matching skip rule beats every classify rule, whatever the priorities.**
  Skipping makes classification moot, and an "exception" ordering (classify
  overrides skip for a narrower pattern) is a rabbit hole not worth digging in
  v1: the user narrows the skip pattern instead. Priority orders classify
  rules among themselves and orders skip rules in the list; it never decides
  skip-vs-classify.
- **No backfill.** `apply-rule` refuses a skip rule: backfilling it would mean
  deleting already-imported transactions by pattern, which is a destructive
  action hiding behind a benign-looking button. Existing imports go through
  `delete-run` / per-transaction delete; the rule covers the future.
- **Not LLM-suggestable.** `suggest-rules` proposes classify rules only, and
  the server drops any `skip` row a model returns. A model proposing which
  money to drop from the books is a different risk class from proposing a
  category.

The blast radius of a broad pattern (`contains "PAY"`) is the one real
hazard: money silently missing from the books. Two things bound it: the live
match count at rule creation shows what the pattern would have caught in
recent imports, and every rule-skipped row is visible and counted under run
and source detail (`skipped_count`, with the rule named on the row) — a
skip is never a silent drop.

### `import_rule_labels` / `import_link_applied_labels`

Labels are the one classification axis that is a **set**, not a scalar: a
transaction carries many (`transactions_labels`, `PRIMARY KEY (transaction_id,
label_id)`), where `transactions.tag_id` is a single nullable column. So the
label target of a rule, and the label snapshot on a link, each need their own
junction table mirroring that shape:

```
import_rule_labels(rule_id → import_rules(id) ON DELETE CASCADE,
                   label_id → labels(id) ON DELETE CASCADE,
                   PRIMARY KEY (rule_id, label_id))

import_link_applied_labels(link_id → import_transaction_links(id) ON DELETE CASCADE,
                           label_id → labels(id) ON DELETE CASCADE,
                           PRIMARY KEY (link_id, label_id))
```

A JSON array column would have been two fewer tables and is the wrong call:
when the user deletes a label, a rule still targeting it must stop applying a
dead id, and a snapshot must stop claiming it. The FK cascade does that in the
database — the same argument Part 3 makes for `ON DELETE SET NULL` on
tombstones. A text blob would instead need Go cleanup in the label-delete
path, which is another feature's code and would violate the dependency rule.

A rule's label set is capped at `maxLabelsPerImportRow` (10,
`internal/transaction/import.go:25`) — the cap the CSV importer already
applies per row. One number for "how many labels may an import put on one
transaction", wherever the import came from.

Labels are ignored on transfers, per the transaction contract. Imports create
only non-transfers, so this never binds in practice; it is noted so a future
transfer-aware provider does not rediscover it.

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
   `status='queued'`. An `'ignore'` link → ledger row `status='skipped'`,
   pipeline ends (the raw event stays, per stage 0).
3. **Match transaction & import** — the reconciler below; on create, apply
   `import_rules` by priority; unmatched fields stay nil. A matching **skip
   rule** (Part 2) ends the stage first, before the matcher runs: ledger row
   `status='skipped'`, no transaction.
   Currency conversion happens here, before the amount is compared to
   anything: when the event's currency differs from the linked account's,
   convert with the stored rate for the event's date, and match on the
   converted amount. `external_amount` + `external_currency` on the link keep
   the original auditable forever, and the run summary reports how many were
   converted. No rate available for that currency and date → the event queues
   with a visible reason rather than importing a guessed amount.

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
   instead of discarding them; ignored → `skipped` links (seen, counted on
   the run, never queued). Parse failures are recorded on the run and do
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
   has the same amount within ±`ECONUMO_IMPORT_MATCH_DAYS` days (default 3).
   Do not create a duplicate; **add a link
   to the existing transaction** and count it as `matched`. This is what makes
   the first sync tolerable for anyone who has been entering transactions
   manually or via CSV — otherwise every manual entry gets a twin.
3. **Cross-source adopt (the tip problem)** — no exact-amount match, but a
   transaction on the same account whose live links include a *push* source
   matches all of:
   - bank date within **+`ECONUMO_IMPORT_TIP_DAYS` days after** the tap
     (default 5; posting is strictly later);
   - amount within **±`ECONUMO_IMPORT_TIP_TOLERANCE`%** of the tap amount
     (default 20; tips, gas holds, FX);
   - **merchant token-containment**: normalize both strings (lowercase, strip
     non-alphanumerics, split on whitespace), match when every token of at
     least `ECONUMO_IMPORT_TOKEN_MIN_LENGTH` characters (default 3) of the
     push merchant appears in the bank payee/description token set —
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

#### Matcher configuration

The four thresholds are guesses until real bank data has been through them,
so they are **environment variables with defaults**, not constants — an
instance can be tuned without a release, and the defaults are the spec
numbers:

| Variable | Default | Range | Used by |
|---|---|---|---|
| `ECONUMO_IMPORT_MATCH_DAYS` | `3` | `0`–`31` | stage 2, ± window in days |
| `ECONUMO_IMPORT_TIP_DAYS` | `5` | `0`–`31` | stage 3, forward window in days |
| `ECONUMO_IMPORT_TIP_TOLERANCE` | `20` | `0`–`100` | stage 3, percent of the tap amount |
| `ECONUMO_IMPORT_TOKEN_MIN_LENGTH` | `3` | `1`–`16` | stage 3, shortest merchant token that must be contained |

Parsed once in `config.Load`, strictly: malformed, negative or out-of-range
values fail at boot (the `ECONUMO_CURRENCY_UPDATE_INTERVAL` precedent), and
`0` on a window means exact-date only. They are read into one
`imports.MatcherConfig` struct passed to the matcher, so the matcher itself
stays a pure function of `(event, candidates, config)` — the property the
unit tests rely on. Each one is documented in `CLAUDE.md`'s Configuration
list and `.env.example` in the PR that introduces it (stage 1 for the first
two, stage 2 for the tip-matching pair).

The queue page's client-side grouping (Part 9) uses the same date, amount and
token thresholds, so the server merges the effective values into the served
`econumo-config.js` as `IMPORT_MATCHER` — the same mechanism as
`BILLING_URL` — rather than letting the SPA carry a second copy of the
numbers.

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
- **Date** — `posted` is a bare unix timestamp, so there is no "bank's own
  date" to store verbatim; a timezone must be chosen to derive the day. Use
  the same fallback chain the rest of the codebase uses for day boundaries:
  the caller's `X-Timezone` → the stored `users.timezone` → UTC. Then format
  `2006-01-02 15:04:05`. A UTC-naive conversion would put late-evening
  purchases on the wrong day for anyone west of UTC, surfacing as wrong-day
  budget attribution.
- **Payee** — `payee` when the bridge supplies it, else `description`.
- **Classification** — `import_rules` by priority set category, payee, tag
  and labels; a field no rule assigns stays unset. All four are optional — nil category, nil payee, nil tag
  and an empty label set are legal everywhere in Econumo. A matching skip
  rule means no transaction at all (above).

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
| `account` | trim, collapse inner whitespace; used verbatim as the external account id (the card's Wallet display name, or the literal the user typed for a per-card automation) | missing/empty → parse failure |
| `amount` | decimal string, tolerant normalization: strip currency symbols and spaces; `.` and `,` both accepted, the **last** separator is the decimal point (`1.234,56` → `1234.56`); must yield a positive decimal | unparseable → parse failure |
| `currency` | 3-letter ISO code, uppercased; symbols rejected (`$` is ambiguous — the recipe sends the trigger's currency code) | missing/invalid → parse failure |
| `occurredAt` | RFC3339 **with offset** — the device's local time; the wall-clock date derives from the event's own offset, not `X-Timezone` (a Shortcut sends no meaningful header), stored in the frozen `2006-01-02 15:04:05` format | missing/invalid → **fallback to `received_at`** — a lost timestamp must not lose a purchase; taps reach the server within seconds |
| `type` | `expense`/`income`, optional, default `expense` (Wallet amounts are positive for charges) | unknown value → parse failure |
| `payee` | trim; empty allowed (description is the fallback) | never fails |
| `eventId` | optional; overrides the synthesized external id | — |

The synthesized external id is computed from **parsed** values —
`sha256(account|occurredAt|amount|payee)` — so it stays stable across
retries even when raw bytes differ (field order, whitespace).

Response: `{status: "created"|"queued"|"skipped"|"duplicate"|"failed"}`
(`skipped` = the card is ignored) — always
HTTP 200 once the event is persisted. A 4xx is invisible to an unattended
automation; a failed event surfaces in the web UI (queue page, "needs
attention"), where the user sees the raw payload and can retry or discard.

Rate limit: `ECONUMO_RATE_LIMIT_INGEST`, per user, every request counts,
following the `accept-invite` precedent.

### The queue

One queue across all providers: `status='queued'` links, whatever their
source. Pull syncs feed it (transactions of unmapped bank accounts), push
events feed it (unmapped cards), and unconvertible-currency holds land in it.
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
source detail with an "un-skip" (back to queued) escape hatch. A hand skip
offers a **skip rule** the same way an edit offers a classify rule (Part 7):
*"Always skip 'APPLE CASH'?"* with the editable match value and live count.

**At-rest note:** queued pull events keep bank amounts/payees for accounts
the user may never map. Accepted for v1 (simplicity, and the user sees and
controls the queue); revisit if it proves objectionable.

## Part 7 — Rule learning from edits

Nobody authors mapping rules up front; they emerge from correcting the first
sync. This is the loop that populates `import_rules`.

**Detection is client-side.** The SPA knows a transaction is imported (it has
`isImported`), fetches its links on open (`get-transaction-import-list`), and
on save diffs the current values against the **most recent** link's
`applied_category_id` / `applied_payee_id` / `applied_tag_id` snapshot and
the link's applied label set. Category, payee and tag compare as scalars;
labels compare as **sets**, so adding one label to a transaction that already
had two is a detected change. If a target changed, it offers a rule.

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
are dropped), match types must be in the `exact`/`contains`/`prefix` set,
and `action` must be `classify` (skip rows are dropped, Part 2).

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
| POST | `create-source` | Store a source; SimpleFIN sends `credential_ciphertext`, push providers nothing. |
| GET  | `get-credential-key` | The caller's `wrapped_data_key` + `kdf` — what the client needs to unlock on a new device. |
| POST | `set-credential-key` | Store/replace the wrapped data key and KDF params (a passphrase change re-wraps and rewrites this one row). |
| GET  | `get-source-list` | Sources with status and `last_synced_at`. |
| POST | `delete-source` | Remove a source, its links, and its queued rows. |
| POST | `list-external-accounts` | Proxy: client supplies access URL, server returns accounts for mapping (pull). |
| POST | `link-account` | Map external account → Econumo account (`mode='import'`; also turns an ignore into a mapping). Triggers the conversion run for the account's `queued` rows; `skipped` rows stay skipped. |
| POST | `ignore-account` | `mode='ignore'` link (created or flipped from a mapping): the account's `queued` rows become `skipped`, future events skip on arrival. |
| POST | `unlink-account` | Removes the link (either mode); also purges the account's queued rows. |
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
| POST | `create-rule` / `update-rule` / `delete-rule` | `action` is `classify` or `skip`; a skip rule with targets or labels is a validation error. |
| POST | `preview-rule` | Match counts before applying. |
| POST | `apply-rule` | Backfill; classify rules only (a skip rule → coded 400, Part 2). |

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
- **SimpleFIN** — connect (paste setup token, set the encryption passphrase,
  claim, encrypt, store), unlock (passphrase prompt when the IndexedDB key is
  absent; "forget this device" clears the local key), account mapping, the
  **Sync button** (below), run history. The unlock
  prompt states plainly that a forgotten passphrase is recovered by
  reconnecting, not by support — and offers that path inline, so a stuck user
  is never cornered.
- **Apple Wallet** — setup flow below; the **card list**: every card name the
  server has seen (from links and queued events) in one of three states —
  mapped to an Econumo account, **ignored**, or "unmapped — N queued" — with
  map / ignore / "map instead" actions; per-card tap count and last-seen;
  activity. The SimpleFIN source detail lists its bank accounts the same way.

### Apple Wallet setup — a signed shortcut, configured by deep link

The goal is "Get the Shortcut" → a ready-to-run automation with the server
URL and token already in place. Two facts about the platform shape how that
can work:

- **A `.shortcut` file must be signed** (iOS ≥15 refuses unsigned ones), and
  the signature covers the whole plist. Signing runs on a Mac (`shortcuts
  sign --mode anyone`, or Share → "Anyone" in Shortcuts.app) against Apple's
  servers; the only Linux implementations need Apple ID keys dumped from a
  jailbroken device. So the server **cannot mint a per-user file** with the
  token baked in: the artifact is static, built once on a maintainer's Mac,
  and per-instance values must reach it some other way.
- **Automations are not shareable.** A shortcut file carries the actions, not
  the Transaction trigger; the user always creates the automation on the
  phone (Automation → Transaction → Run Immediately → pick the shortcut).
  Four taps, unavoidable.

So the design is a static signed shortcut plus a **configuration deep link**:

1. **Install.** "Get the Shortcut" serves two signed files from
   `web/public/shortcuts/`: **Econumo Wallet** (the automation body) and
   **Econumo Setup** (writes the configuration). Safari hands each to
   Shortcuts, which shows the standard "Add Shortcut" sheet.
2. **Configure — one tap, nothing typed.** The setup page's "Configure on
   this iPhone" button (rendered only on iOS) creates the `'ingest'`-scoped
   PAT server-side and opens
   `shortcuts://run-shortcut?name=Econumo%20Setup&input=text&text=<url-encoded JSON {url, token}>`.
   Econumo Setup receives that text as its input and saves it as
   `Shortcuts/econumo-wallet.json` in iCloud Drive (the `Save File` action,
   overwrite on), then shows "Configured". The deep link is handled by the
   OS on the same device — the token never crosses the network in a URL —
   and the file holds an ingest-only, revocable token, exactly what the
   hand-built recipe would hold inside the shortcut itself.
3. **Automate.** Instructions with screenshots for the four taps.

Econumo Wallet, on every tap: `Get File` (`Shortcuts/econumo-wallet.json`)
→ `Get Dictionary from Input` → `Get Contents of URL` (POST `{url}/api/v1/import/ingest-apple-wallet-event`,
`Authorization: Bearer {token}`, JSON body from the trigger's Card or Pass /
Merchant / Amount / Currency / Date properties, Part 6 shape). Missing
configuration file → the shortcut stops with a notification pointing at
Settings → Data, rather than posting garbage. Reconfiguring (new server,
rotated token) is pressing "Configure" again — the file is overwritten, the
shortcut is untouched.

Desktop browsers show the "open Settings → Data on your iPhone" note instead
of the Configure button — the SPA's mobile view is the same page.

**Building the artifacts** (maintainer, on a Mac): the action-by-action
recipe for both shortcuts, the configuration-file and request contract, the
device test, and the sign/export steps are in `web/shortcuts/README.md`.
The signed files go to `web/public/shortcuts/`, the unsigned plist source
(via `shortcut-sign extract`) next to the README, so the recipe is
reviewable in diffs and re-signable by anyone with a Mac. The served file
name carries a version (`econumo-wallet-v1.shortcut`) so an installed
shortcut can be told apart from the current one.

**Fallback that stays:** the manual recipe — PAT shown once with a copy
button, the server URL, the pre-rendered JSON body with the user's values
substituted, five actions with screenshots — remains on the page below the
one-tap path, for anyone whose device cannot import the file. Its "same-named
cards" note (Part 2, card → account) applies to both paths: the user
duplicates Econumo Wallet, replaces the `account` value with a literal, and
points a per-card automation at the copy (a Transaction automation cannot
pass extra input, so a copy is the only way to vary the label).

**Rejected:** per-user signing by the server (impossible without Apple ID key
material; a Mac-hosted signing service would be a third party receiving every
self-hosted instance's tokens); **import questions** (typing, and unreliable
across iOS 18.x releases); baking the token into the file (would need
per-user signing).

**Device test before shipping:** the `Get File` read inside a
Transaction-triggered run (background, no interaction), `Save File`
behavior on a device without iCloud Drive enabled (fallback: the on-device
Shortcuts folder), and a QR code carrying the `shortcuts://` link when the
user is on the desktop page.

### Sync button, then sync-on-open

The server never schedules a sync, so the client does — and the client is
the web SPA, at every viewport: the mobile view of the web app is the
primary daily surface today, the Capacitor shell is not in use yet, so
nothing about sync may depend on being inside the native app.

**v1 (stage 3): a Sync button in Settings → Data**, per source and one
"Sync all" above the list, with the last-synced time next to each. Pressing
it unlocks the key (passphrase prompt only if the IndexedDB key is absent),
decrypts the access URL, and runs the Part 5 pull flow with a visible
progress state and the run summary on completion (imported / matched /
queued / skipped / failed, linking to the run). It is the same control at
every viewport — a full-width button in the mobile layout, not an icon that
needs a hover. Errors are shown inline on the source row, and a source whose
sync failed shows that state until the next successful run.

**Follow-up: sync-on-open.** Once the button exists, the SPA also syncs
every enabled pull source in the background on app open (and, in the mobile
shell, on resume via the `@capacitor/app` listener already available). Four
rules keep that from being obnoxious or expensive:

- **Silent when locked.** It runs only if the non-extractable key is already
  in IndexedDB. A missing key (evicted, cleared, new device) skips the sync
  and shows an unobtrusive "unlock to sync" affordance — it must never
  interrupt app open with a passphrase prompt.
- **Throttled per source.** At most one automatic sync per source per
  `AUTO_SYNC_MIN_INTERVAL` (start at 6h, a named constant), measured against
  `last_synced_at`. SimpleFIN is an external service with its own limits, and
  opening the app five times before lunch must not mean five bank calls. The
  explicit Sync button ignores the throttle.
- **Never blocking.** It runs after first paint and never gates rendering.
  A failure is silent apart from the source's status in Settings → Data —
  an automatic action the user did not ask for must not raise an error
  dialog.
- **Distinguishable.** The run records its trigger (`auto` | `manual`) so run
  history reads honestly, and the `IMPORT_SYNC` metric carries it as a
  property.

This is what replaces a scheduler: not "synced at 3am", but "current whenever
you look at it". Someone who never opens the app never syncs — an acceptable
consequence, since they are also not reading their budget. Until the
follow-up lands, "current whenever you look at it" is one tap away in
Settings → Data.

### Queue page + banner

- A **banner** on the main screens whenever the queue is non-empty — same
  pattern as the sharing-request banner — linking to the queue page.
- The **queue page** resembles the transaction list: queued events with their
  provider badge, grouped **client-side** when they plausibly describe the
  same purchase across providers (amount + date proximity + merchant-token
  overlap; the shared normalizer). Per external account: "map this account"
  → link dialog → conversion run → result summary, or **"ignore this
  account"** → its queued rows leave the queue for good. Per row: **tap → the
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
- **Rules management** — list (skip rules visibly distinct from classify
  rules), edit, reorder by priority; a **"Suggest
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
`METRICS` keys: `IMPORT_SOURCE_CONNECT`, `IMPORT_ACCOUNT_LINK`, `IMPORT_ACCOUNT_IGNORE`,
`IMPORT_SYNC` (with an `auto`/`manual` trigger property), `IMPORT_QUEUE_MAP`, `IMPORT_QUEUE_IMPORT`,
`IMPORT_QUEUE_SKIP`, `IMPORT_SHORTCUT_DOWNLOAD`, `IMPORT_SHORTCUT_CONFIGURE`, `IMPORT_RUN_DELETE`,
`IMPORT_RULE_CREATE` (with an `action` property, `classify`/`skip`),
`IMPORT_RULE_APPLY`, `IMPORT_RULES_SUGGEST`. Fired at the
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
  timestamp fallback, failure taxonomy) + synthesized-id stability, skip
  rules (beats a higher-priority classify rule; produces a seen `skipped`
  row with the rule recorded; never runs on an unmapped account's events;
  `apply-rule` refuses it; `suggest-rules` drops model-returned skip rows),
  event-level dedupe vs ledger dedupe, retry-after-parser-fix,
  amount/sign mapping, timezone/date conversion on both paths, and currency
  conversion (converted amount is what the matcher compares; the original
  survives on the link; a missing rate queues rather than guesses).
- Config: the four `ECONUMO_IMPORT_*` matcher variables parse strictly
  (defaults when unset; malformed / negative / out-of-range fail at boot),
  reach the matcher as one `MatcherConfig`, and the effective values appear
  in the served `econumo-config.js`.
- Repo: engine-adapter coverage for all nine tables; `make test-repo-pgsql`.
- Labels as a set, not a scalar: a rule applying several labels; the applied
  label snapshot round-tripping; rule learning detecting an added label on a
  transaction that already had others; deleting a label cascading out of both
  junction tables so no rule or snapshot keeps a dead id.
- Cross-feature adapters come from `internal/test/wiring` (the real
  implementations `internal/server` assembles), not per-test stubs — so the
  currency conversion and transaction creation an import performs are
  exercised for real.
- Deleted-account interaction: a link whose account is soft-deleted queues its
  events instead of erroring; the account's zeroing correction and the
  importer do not fight.
- Card → account: taps from two differently named cards land in two
  accounts; an unmapped card queues and `link-account` converts exactly its
  backlog; a renamed card queues under the new name while the old link is
  untouched; re-mapping a link moves only future taps; ignoring a card
  skips its queued rows and every later tap on arrival (ingest answers
  `skipped`, raw event still stored); "map instead" converts nothing
  retroactively — the skipped rows need a per-row un-skip.
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
- Credential key: a passphrase change re-wraps the data key without touching
  any source's `credential_ciphertext`; the key row is readable only by its
  owner; clearing it leaves imported transactions untouched.
- Security regression: the access URL must never appear in logs or in any
  persisted column; ingest bodies never logged; production SQLite
  `foreign_keys = ON` assertion.
- `docs/regression-test-plan.md`: every stage adds its user-observable flows
  to the manual checklist, per the repository rule.
- Frontend: vitest for crypto round-trip, rule-value extraction, queue
  grouping, mapping validation. Run `pnpm exec tsc -b` — vitest and oxlint do
  not type-check.

## Implementation staging

Four plans, one PR each, **push-first**: the Apple Wallet path exercises the
whole generic core — events, ledger, matcher, queue, runs — while depending on
nothing but a phone. The SimpleFIN path adds an external aggregator account and
the client-side key management, the two riskiest pieces; putting them third
means the core is already proven when they land.

1. **Core** — `internal/imports` package skeleton, the nine tables + the
   `access_tokens.scope` column and its middleware enforcement, the event
   inbox, the ledger, runs, the matcher, tombstone/cascade semantics, and
   `isImported` on the transaction list. No provider yet; the matcher and
   ledger are tested directly.
2. **Apple Wallet** — the `ingest-apple-wallet-event` route and normalizer,
   currency conversion, the queue (page, banner, per-account mapping with its
   conversion run, per-event triage, skip/retry/discard), account links, and
   the Settings → Data page with the signed shortcuts + deep-link
   configuration and the manual recipe as fallback. **This is where the
   feature becomes usable.**
3. **SimpleFIN** — the pull `Provider` seam and its implementation, the
   two-step claim, `importCrypto.ts` (PBKDF2/Argon2id → non-extractable
   IndexedDB key), the Sync button in Settings → Data (every viewport), run
   history, per-account error handling. Sync-on-open is a follow-up PR after
   stage 3, not part of it.
4. **Rules** — `import_rules`, matching by priority, edit-driven rule
   learning with the editable match value and live match counts, preview and
   backfill, the rules management UI, and the optional `ECONUMO_AI_DSN`
   suggestion flow.

Each plan is written only when its predecessor merges — the core's shape will
teach us things about stages 2-4 that are not worth guessing at now.

## Resolved questions

Resolved 2026-09-01, after the spec was approved.

1. **Timezone for pull `posted`** — caller's `X-Timezone` → stored
   `users.timezone` → UTC, matching the codebase's existing day-boundary rule.
   A bare unix timestamp offers no bank-supplied date to store verbatim.
   (Push was already settled: the event's own RFC3339 offset.)
2. **CSV run integration** — deferred to its own PR after stage 2. Threading a
   `run_id` through the SPA's sequential 500-row chunk uploads is a frontend
   contract change unrelated to import correctness. Until it lands, re-uploading
   a CSV still duplicates, as it does today.
3. **Matcher constants** — environment variables with the spec numbers as
   defaults (`ECONUMO_IMPORT_MATCH_DAYS`, `ECONUMO_IMPORT_TIP_DAYS`,
   `ECONUMO_IMPORT_TIP_TOLERANCE`, `ECONUMO_IMPORT_TOKEN_MIN_LENGTH`; Part 5,
   Matcher configuration), so an instance can be tuned without a release
   while they remain unvalidated against real bank data.
4. **Multi-currency** — split in two. A whole external account denominated
   differently from its Econumo account is a misconfiguration and is refused at
   link time. A single foreign-currency event on a correct link is converted
   with the stored rate for its date, falling back to the queue when no rate
   exists. See Decisions and Part 5 stage 3.
5. **Shortcut distribution** — a static signed shortcut (built once on a
   Mac) configured through a `shortcuts://run-shortcut` deep link that carries
   the instance URL and a freshly minted ingest PAT; the written recipe stays
   as the fallback. Ships in stage 2. See Part 9.
6. **Raw event retention** — keep everything in v1. An `import:purge-events`
   CLI, mirroring `token:purge`, is the follow-up when the table gets big.
7. **Suggest-rules prompt shape** — resolved by experiment in stage 4; the
   stubbed transport seam isolates the risk.

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
- Sync-on-open / on-resume (Part 9, the follow-up to the Sync button).
- True periodic background sync on mobile (a Capacitor background-runner
  plugin waking the app on a timer, rather than syncing on resume).
- CSV adoption of the shared ledger and runs (resolved question 2).
- `import:purge-events` CLI for raw-event retention (resolved question 6).
