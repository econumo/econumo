# Bank Import (SimpleFIN) — Design

**Date:** 2026-07-19
**Status:** Proposed

## Overview

Automatic transaction import from a bank aggregator, starting with
[SimpleFIN Bridge](https://www.simplefin.org/protocol.html) (North America).

The feature is deliberately built as a **generic import subsystem** that
SimpleFIN is merely the first client of. The durable parts — external→internal
id mapping, deduplication, mapping rules, import runs — are provider-agnostic
and are reused by the existing CSV import and by any future provider
(Enable Banking, Pluggy, Akahu).

Two properties shape every decision below:

1. **Zero-knowledge at rest.** Econumo is offered as a hosted service. The
   server stores the provider credential as opaque ciphertext it cannot
   decrypt; the key never leaves the browser. A leaked database or backup
   exposes no bank access.
2. **Manual sync only.** There is no scheduler. The user clicks "Sync". This
   matches the reference implementation (Actual Budget's bank sync is also
   manual-trigger) and is a *requirement* of (1) — a server that cannot decrypt
   a credential cannot sync unattended.

### Prior art

Actual Budget brokers to five region-locked aggregators and reconciles with a
three-stage matcher keyed on a per-provider `imported_id`, plus an
`imported_payee` column holding the raw bank string. That data model — not the
provider list — is the transferable insight. GoCardless Bank Account Data (the
free Nordigen tier Actual's EU users rely on) has been **closed to new signups
since July 2025**, which is why this design starts with SimpleFIN and treats
the provider as swappable from day one.

## Decisions

- **First provider:** SimpleFIN. Simplest protocol (token → access key → one
  REST call), no eIDAS broker, no PSD2 licensing. Enable Banking is the
  intended second provider for EU coverage.
- **Credential ownership:** per-user, client-side encrypted. Not `.env`
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
- **Server is a stateless proxy.** The plaintext access URL arrives per sync
  request, is used, and discarded. Never persisted, never logged.
- **Posted transactions only.** `pending=1` stays off in v1 (see Non-Goals).
- **Account mapping is manual.** Nothing syncs until the user explicitly links
  an external account to an Econumo account.
- **Deleting a transaction does not delete its import mapping** (tombstone),
  so deletions survive re-sync.

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

## Non-Goals (v1)

- **Scheduled/background sync.** Structurally impossible under client-side
  keys, and explicitly not wanted.
- **Pending transactions.** Their ids mutate when they post; Actual gates this
  behind a preference and still carries open bugs. Deferred.
- **Balance reconciliation.** SimpleFIN returns `balance`, but Econumo derives
  balance from transactions rather than storing it. Comparing them is a useful
  "sync is complete" signal but has its own correction-transaction semantics.
- **Regex match rules.** A denial-of-service surface (catastrophic
  backtracking on user-supplied patterns evaluated per row).
  `exact`/`contains`/`prefix` covers the realistic cases.
- **Migrating the CSV importer into this package.** CSV *adopts* the shared
  dedupe ledger and run concept, but its parsing stays in
  `internal/transaction`.

## Part 1 — Package & architecture

New feature package **`internal/imports`** — not `internal/import`, since
`import` is a Go keyword and cannot be a package name. The route namespace
`/api/v1/import/...` is a string and is unaffected.

Owns: sources, id mappings, rules, import runs, the reconciler, the provider
seam.

### Provider seam

```go
type Provider interface {
    ListAccounts(ctx context.Context, cred Credential) ([]model.ExternalAccount, error)
    FetchTransactions(ctx context.Context, cred Credential, req FetchRequest) ([]model.ExternalTransaction, error)
}
```

`internal/imports/simplefin` implements it. A future `enablebanking` package is
purely additive.

### Dependency rule

`imports` needs to create transactions and resolve accounts/categories/payees/
tags — all sibling features. Per the repository dependency rule it declares
consumer-side interfaces in its own `ports.go`, and `internal/server` wires
`glue_imports_*.go` adapters at composition time. `archtest` auto-detects new
feature packages, so this is enforced from the first commit.

**One deliberate exception**, see Part 4: the transaction list query joins
`import_transaction_links` directly in SQL. This is a knowing shortcut, to be
refactored later.

### Persistence

Follows the engine-adapter (sqlc) pattern: a `querier` interface in the
canonical sqlite-generated types, with `sqlite.go` passthrough and `pgsql.go`
conversion shims selected once in the constructor.

## Part 2 — Schema

Five new tables. All ids `TEXT`/UUIDv7 per the existing contract. Migrations
paired under `internal/infra/storage/migrations/{sqlite,pgsql}`.

### `import_sources`

One connected provider per user.

```
id, user_id, provider ('simplefin'|'csv'), name,
credential_ciphertext TEXT NULL,   -- opaque; the server never decrypts this
credential_kdf        TEXT NULL,   -- {alg, salt, iterations} — client metadata
status, last_synced_at, created_at, updated_at
```

The server treats `credential_ciphertext` as bytes to store and return, nothing
more. `credential_kdf` is metadata the *client* needs to re-derive its wrapping
key; the server cannot act on it either. CSV sources carry neither.

### `import_account_links`

The manual account mapping step.

```
id, source_id, external_account_id, external_name, external_currency,
account_id           → accounts(id),
default_category_id  → categories(id),
is_enabled, created_at, updated_at
UNIQUE(source_id, external_account_id)
```

Nothing syncs until a link exists. Unmapped external accounts surface as
"needs mapping" and are **never auto-created** — this avoids the existing CSV
failure mode in `internal/transaction/import.go:323`, where a name matching an
unwritable shared account silently creates a duplicate own account.

`default_category_id` is load-bearing. `internal/transaction/usecase.go:234`
requires a category for non-transfers, but the CSV importer bypasses that check
by calling `model.New` directly (`import.go:309`) and can therefore write nil
categories. Requiring a default at mapping time closes the hole for imports
rather than inheriting the bug.

`external_currency` lets the link step refuse a mismatch against the Econumo
account's currency instead of silently importing wrong-currency amounts.

### `import_transaction_links`

The dedupe ledger — the piece that generalizes beyond SimpleFIN.

```
id, source_id, run_id → import_runs(id),
external_account_id, external_transaction_id,
transaction_id TEXT NULL REFERENCES transactions(id) ON DELETE SET NULL,

-- what the provider actually said (audit + rule debugging)
external_payee, external_description, external_amount, external_posted_at,

-- what the import assigned (diff base for rule learning)
applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id NULL,

imported_at

UNIQUE(source_id, external_account_id, external_transaction_id)
CREATE UNIQUE INDEX ... ON import_transaction_links(transaction_id)
    WHERE transaction_id IS NOT NULL;
```

The composite unique is **required, not stylistic**: SimpleFIN transaction ids
are unique *within an account*, not globally.

The partial unique index on `transaction_id` guarantees at most one live link
per transaction, which the Part 4 `LEFT JOIN` depends on — without it, a
transaction with two links would be duplicated in the transaction list. Partial
so unlimited tombstones (`NULL`) coexist. Both engines support partial indexes.

Keeping `external_*` here rather than on `transactions` is the equivalent of
Actual's `imported_payee`, without widening a frozen wire type.

**This is where CSV import gets fixed.** CSV rows have no external id, so
synthesize one: `sha256(date|amount|description|row-ordinal)`. Same table, same
uniqueness constraint, and re-uploading a file becomes idempotent — retiring
the "no idempotency at any level" gap (`internal/transaction/ports.go:93`
documents `SaveTransaction` as explicitly having no idempotency id) without a
second mechanism.

### `import_runs`

Makes each import an auditable, undoable unit.

```
id, user_id, source_id, provider,
params TEXT,                       -- {start_date, end_date, account_links[]}
status ('running'|'completed'|'failed'|'partial'),
imported_count, matched_count, skipped_count, failed_count,
started_at, finished_at
```

### `import_rules`

Category/payee/tag mapping.

```
id, user_id, source_id NULL,       -- NULL = applies to every source
match_field ('description'|'external_payee'|'external_category'),
match_type  ('exact'|'contains'|'prefix'),
match_value, is_case_sensitive,
target_category_id NULL, target_payee_id NULL, target_tag_id NULL,
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
id = ?`, `internal/transaction/repo/repo.go:108`) — unlike accounts, which
soft-delete. A link row would therefore genuinely dangle.

| Action | Transaction | Link | Re-imports on next sync? |
|---|---|---|---|
| Delete transaction (default, from anywhere) | deleted | **kept, tombstoned** | no |
| Delete mapping only ("allow re-import") | kept | deleted | yes — creates a duplicate |
| Delete both (from run view) | deleted | deleted | yes |
| Delete entire run | all deleted | all deleted | yes, all of it |

**Why tombstone rather than cascade.** The dedupe ledger is the only memory
that an external transaction was ever seen. Delete the link and the next sync
sees a brand-new external id and re-imports it — the classic zombie-resurrection
bug. Since every link is created with a non-null `transaction_id`,
`transaction_id IS NULL` unambiguously means "imported, then deleted", and the
reconciler treats it as "already seen — skip".

**`ON DELETE SET NULL` requires zero cross-feature coupling.** The alternative
— `transaction` calling into `imports` on delete — would violate the dependency
rule and need a port plus a glue adapter. The FK does it in the database,
atomically, inside the existing `s.tx.WithTx` block in
`internal/transaction/delete.go`, with **no Go changes to the transaction
feature at all**.

Two things to verify during implementation rather than assume:

- That `foreign_keys = ON` is genuinely set on the **production** SQLite
  connection, not only in `dbtest`. If it were test-only the FK would silently
  no-op in production. Needs an explicit assertion.
- Engine parity on the cascade — a delete-then-resync sequence must behave
  identically on both engines. Belongs in `enginecompare`.

### Guardrails

- **"Delete mapping only" is a footgun.** It undoes nothing; it re-imports the
  transaction next sync, producing a *duplicate* alongside the one kept. It
  must be worded as **"allow re-import"**, since that is what it does.
- **"Delete entire run" can destroy real work.** Imported transactions get
  edited. The confirmation must count transactions modified since import
  (`transactions.updated_at > import_transaction_links.imported_at`) and say
  so: *"12 transactions, 4 of which you've edited since importing."*

## Part 4 — Marking imported transactions in the list

The transaction list exposes `importRunId` so the UI can badge the row and link
to its run.

**Implementation: a `LEFT JOIN` from the transaction list query into
`import_transaction_links`.** This is a knowing shortcut — the transaction
slice reads a table another feature owns. It was chosen over the architecturally
clean option (a `ports.go` interface + glue adapter doing a batched lookup)
because it touches SQL only, adds no Go-level dependency, and is cheap to
refactor later.

Consequences to accept:

- `archtest` will **not** catch this coupling — it is SQL, not a Go import.
  The query files must carry a comment marking the cross-feature reference so
  the later refactor is greppable.
- Two query files (`query/{sqlite,pgsql}`) plus `sqlc generate`.
- Correctness depends on the partial unique index from Part 2.
- `TransactionResult` gains `importRunId`, so **every golden file changes**.
  Additive and safe for clients (unknown fields ignored), but per the repo
  convention a golden diff means observable behavior changed — regenerate with
  `UPDATE_GOLDEN=1` and review the diff deliberately.

## Part 5 — Sync flow & reconciler

### Flow

1. Client unlocks the data key from IndexedDB (prompting for the passphrase
   only if absent) and decrypts the access URL.
2. Client `POST`s the access URL + `source_id` + date range.
   **Never persisted, never logged.**
3. Server opens an `import_runs` row and calls
   `GET {accessUrl}/accounts?start-date=…` — one call returns accounts *and*
   their nested transactions.
4. Per enabled account link, **in its own DB transaction**: reconcile → apply
   rules → insert transactions + links.
5. Discard the credential. Finalize the run. Return
   `{runId, imported, matched, skipped, failed, errors}`.

### Three-stage matcher

1. **Exact** — `(source_id, external_account_id, external_transaction_id)`
   already present in links → skip. **Tombstones count as seen**, which is what
   makes deletions stick.
2. **Fuzzy adopt** — no link, but an existing transaction in the same account
   has the same amount within ±3 days. Do not create a duplicate; **link to the
   existing transaction** and count it as `matched`. This is what makes the
   first sync tolerable for anyone who has been entering transactions manually
   or via CSV — otherwise every manual entry gets a twin.
3. **Create** — otherwise a new transaction plus its link.

Actual's payee-based fuzzy stage is deliberately dropped: their stage 2
(amount + date + payee) and stage 3 (amount + date) differ only by a payee
check that on first sync is almost always absent, adding a branch without
adding discrimination.

### Field mapping

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
  `default_category_id`. Never nil.

### Failure handling

SimpleFIN returns a structured `ErrorList` with `gen`/`con`/`act` scoping.
Per-account errors must not kill the run: record them, mark the run `partial`,
keep the accounts that succeeded. Per-account DB transactions mean a mid-run
failure leaves completed accounts durably imported and safely re-syncable —
the links make retry idempotent.

### Logging

The sync endpoint must be excluded from any body logging, and the access URL
must never reach a log attribute. The existing access log records UUIDs only
(no bodies, no query strings), so the default is already safe — but this needs
an explicit regression test rather than reliance on convention.

## Part 6 — Rule learning from edits

Nobody authors mapping rules up front; they emerge from correcting the first
sync. This is the loop that populates `import_rules`.

**Detection is client-side.** The SPA knows a transaction is imported (it has
`importRunId`), fetches the link record on open, and on save diffs the current
values against `applied_category_id` / `applied_payee_id` / `applied_tag_id`.
If a target changed, it offers a rule.

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
otherwise.

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

## Part 7 — Endpoints

All under `/api/v1/import/`, following the `{module}/{action}-{subject}`
convention. `GET` for reads, `POST` for every write.

| Method | Route | Purpose |
|---|---|---|
| POST | `claim-setup-token` | Exchange a SimpleFIN setup token for an access URL. Returned to the **client**, never stored. |
| POST | `create-source` | Store `credential_ciphertext` + `credential_kdf`. |
| GET  | `get-source-list` | Sources with status and `last_synced_at`. |
| POST | `delete-source` | Remove a source and its links. |
| POST | `list-external-accounts` | Proxy: client supplies access URL, server returns accounts for mapping. |
| POST | `link-account` | Map external account → Econumo account + default category. |
| POST | `unlink-account` | |
| POST | `sync` | The main flow. Client supplies access URL. |
| GET  | `get-run-list` | Import history. |
| GET  | `get-run` | Run detail: links, including tombstones. |
| POST | `delete-run` | Delete transactions + mappings for a run. |
| POST | `delete-link` | "Allow re-import" for a single transaction. |
| GET  | `get-rule-list` | |
| POST | `create-rule` / `update-rule` / `delete-rule` | |
| POST | `preview-rule` | Match counts before applying. |
| POST | `apply-rule` | Backfill. |

The two-step claim (`claim-setup-token` returns the access URL to the client,
which encrypts it and calls `create-source`) is what keeps the server from ever
persisting plaintext. The claim itself is one-time: a 403 means the token was
already claimed or never existed.

### Rate limiting

`claim-setup-token` and `sync` are outbound-request endpoints and should carry
per-user caps in the existing `ECONUMO_RATE_LIMIT_*` family, following the
`accept-invite` precedent.

## Part 8 — Frontend

`web/src/features/imports/`:

- **Connect** — paste setup token, set/enter encryption passphrase, claim,
  encrypt, store.
- **Unlock** — passphrase prompt when the IndexedDB key is absent; "forget this
  device" clears it.
- **Account mapping** — external accounts on the left, Econumo accounts +
  default category on the right. Currency mismatch blocks the link.
- **Sync** — date range, progress, result summary linking to the run.
- **Run list / run detail** — history; detail shows every link including
  tombstones ("transaction deleted, mapping kept"), with per-row
  delete-transaction / allow-re-import / delete-both.
- **Rule prompt** — the Part 6 flow.
- **Rules management** — list, edit, reorder by priority.

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
`METRICS` keys: `IMPORT_SOURCE_CONNECT`, `IMPORT_ACCOUNT_LINK`, `IMPORT_SYNC`,
`IMPORT_RUN_DELETE`, `IMPORT_RULE_CREATE`, `IMPORT_RULE_APPLY`. Fired at the
shared hook choke point so every surface is covered once.
`metrics-coverage.test.ts` fails the suite if a key is never fired.

### i18n

New `imports.*` namespace in `locales/{en,ru}.json`. Any new error codes need
`errs.AllCodes` registration plus `errors.*` catalogue entries in **both**
languages — the two-way coverage guard in `internal/test/i18ntest` enforces it.

## Testing

- Unit: reconciler stages, rule matching/priority, SimpleFIN response
  normalization, amount/sign mapping, timezone date conversion.
- Repo: engine-adapter coverage for all five tables; `make test-repo-pgsql`.
- **`apiparity`**: every new route needs a scenario — the guard tests enforce
  that route and scenario counts never shrink. Existing goldens change from the
  `importRunId` addition.
- **`enginecompare`**: delete-then-resync, tombstone behavior, and the FK
  cascade must be byte-identical across engines.
- Security regression: the access URL must never appear in logs or in any
  persisted column.
- Frontend: vitest for crypto round-trip, rule-value extraction, mapping
  validation. Run `pnpm exec tsc -b` — vitest and oxlint do not type-check.

## Open questions

1. **Timezone for `posted`** — confirm caller-`X-Timezone` conversion is right
   versus storing the bank's own date verbatim.
2. **CSV run integration** — the SPA uploads CSV in sequential 500-row chunks
   (`web/src/features/transactions/importCsv.ts:5`). Making a run span an
   import means creating the run first and threading `run_id` through every
   chunk, finalizing at the end. Real scope; possibly a follow-up PR.
3. **Fuzzy-adopt window** — ±3 days proposed; unverified against real bank data.
4. **Multi-currency accounts** — v1 blocks a currency mismatch at link time.
   Confirm that is acceptable versus converting.

## Deferred / follow-up

- Enable Banking provider (EU coverage; needs an eIDAS broker).
- Passkey / WebAuthn PRF as an alternative key source. The wrapped-data-key
  design means this is just another way to unwrap the same data key — no
  re-encryption, no schema change.
- Refactor the Part 4 `LEFT JOIN` to a proper port + glue adapter.
- Pending-transaction support.
- Balance reconciliation.
