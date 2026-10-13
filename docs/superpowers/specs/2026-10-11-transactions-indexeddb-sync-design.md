# Transactions in IndexedDB with per-account revision sync — design

Date: 2026-10-11 · Branch: `feature/transactions-indexeddb-sync` (base `main`)

## Goal

Stop re-downloading every transaction the user can see on every boot, focus
refresh, import and sync tap, and stop keeping that list in localStorage, where
it silently stops persisting once it outgrows the ~5 MB origin quota (the sync
persister swallows `QuotaExceededError`, and the next reload blocks on the boot
loader instead of starting from cache).

Transactions live in IndexedDB, grouped by account. Each account carries a
server-side revision counter; the client downloads an account's transactions
again only when that counter moved. Every screen keeps reading the same
`['transactions']` query and the same `TransactionDto[]` shape, so no consumer
changes.

Success: a user with tens of thousands of transactions reloads instantly from
local data, a refresh with no changes downloads no transactions at all, and an
edit on another device or by a shared-account partner shows up after the next
accounts refresh.

## Decisions (agreed in brainstorming)

| Topic | Decision |
|---|---|
| Sync model | Per-account revision counter (`accounts.transactions_revision`); the account is the unit of download. Row-level change log rejected (out-of-order commit handling on PostgreSQL, tombstones). |
| Where bumps happen | In Go repository methods, inside the same DB transaction as the write. No DB triggers (hard to debug). |
| Making the bumps solid | A behavioural guard suite replaying the whole `apiparity` + `mcpparity` catalogues, plus a unit test per write path. |
| New endpoint | `GET /api/v1/transaction/get-account-transaction-list?accountId=` returning `{accountId, revision, items, authors}`. `get-transaction-list` stays frozen (MCP, older clients). |
| Client storage | One IndexedDB database, `econumo`. Stores: `importKeys` (the SimpleFIN data key, migrated from `econumo-import`), `accountTransactions`, `meta`. |
| What stays in localStorage | Token, `backendHost`, UI prefs, analytics opt-out, and the TanStack query cache minus `['transactions']`. |
| Sync button tap | Unchanged call (`invalidateQueries()`); the transactions sync now downloads only accounts whose revision changed. |
| Sync button long-press | Full reload: wipe stored transactions, download every account, invalidate everything. Toast confirms. Long-press, not double-tap, because impatient repeated tapping would otherwise trigger the most expensive action. The same action is the last row of Settings → Data → Import & export ("Reload all data"). |
| Scheduled full refresh | None. Author name/avatar staleness is fixed at the source (`update-name`/`update-avatar` bump). |
| IndexedDB failure | Not a separate mode: on any IDB error the sync keeps working in memory for the session and logs a warning. |
| Hidden-folder accounts | Synced and included in the `['transactions']` list like every other account. Folder visibility no longer affects transactions; the separate `['accountTransactions', id]` query goes away. Global search therefore also finds (and its recent feed shows) hidden-folder transactions, consistent with it already listing hidden-folder accounts. |
| Request size | An account with no stored bucket (first load, resumed load, new account) and every account on a full reload goes account by account: one request per account, at most two in flight, each stored on arrival, so an interrupted first load resumes. Accounts whose stored revision changed go in one request (account by account over 200). Server caps the list at 200 ids as input validation; no `accountIds` (every accessible account) stays supported but the client no longer sends it. |
| Older servers | Accounts without `transactionsRevision` → the client uses today's full `get-transaction-list` path and stores nothing. |

## Background (current state)

- `useTransactions()` (`web/src/features/transactions/queries.ts:9`) fetches
  `get-transaction-list` with no params: every transaction on every visible
  account (owned + accepted shares, not deleted, not in a hidden folder). It is
  part of the boot gate (`ApplicationLayout.tsx` `useIsFullyLoaded`).
- Consumers filter it in memory: account page, global search, budget dialog's
  edit lookup, onboarding. Hidden-folder account pages use a separate
  `['accountTransactions', id]` query with `gcTime: 0`.
- Mutations patch it in place (`useApplyTransactionItem`, classification
  merge/delete, recurring); imports, CSV import, avatar change, connection
  delete and the sync button invalidate it, which re-downloads the whole list.
- After every restore, `refreshRestoredQueries` invalidates every query, so
  every boot re-downloads the list.
- Server: transaction deletes are hard deletes; category/payee/tag/recurring
  deletes null the FK via `ON DELETE SET NULL`; merges and label changes do not
  touch `transactions.updated_at`; share accept/revoke and folder hide/show
  change visibility without touching any transaction row. `updated_at` is
  second-precision and unindexed. Hence a dedicated counter rather than an
  `updated_at` cursor.

## Backend

### 1. Revision column

Migration (sqlite + pgsql): `accounts.transactions_revision BIGINT NOT NULL
DEFAULT 0`. Exposed on the account DTO as `transactionsRevision` (number),
so it travels with `get-account-list` and every response that already carries
accounts (transaction and account mutations). Additive field; old clients
ignore it. `data:import-sqlite` copies it like any other column.

### 2. Bump helper — `internal/infra/txrevision` + a before-commit hook

- `backend.TxManager` gains a generic per-transaction hook: a repo can attach
  a value to the active top-level transaction and register a function that
  runs **once, inside the transaction, right before the outermost COMMIT**
  (savepoints share it). Outside any transaction the hook runs immediately.
- `txrevision.Bumper` (built per repo from `driver` + `*backend.TxManager`):
  - `Bump(ctx, accountIDs ...string)` adds the ids to the transaction's
    pending set.
  - `BumpWhere(ctx, cond string, args ...any)` first selects the
    `account_id`/`account_recipient_id` of transactions matching `cond`, then
    `Bump`s them. Callers pass fixed condition strings only; values go through
    `args`.
  - At commit the pending set is flushed: one
    `UPDATE accounts SET transactions_revision = transactions_revision + 1 WHERE id = ?`
    per account, **in sorted id order** (a consistent lock order, so two
    PostgreSQL transactions bumping overlapping accounts cannot deadlock).
- Consequence: **each account moves by exactly 1 per committed DB
  transaction**, however many writes touched it. Every request that writes
  transactions runs in one `WithTx`, so that is exactly +1 per request. A
  savepoint rolled back after a `Bump` leaves its ids pending — an extra bump
  is harmless.
- Infrastructure package, so every feature may import it.

### 3. Bump sites (inside the repository method, same DB transaction)

Rule: **a repository method that changes what `get-account-transaction-list`
would return for an account bumps that account.** The use case never bumps.

| Write | Repo | Bump |
|---|---|---|
| Transaction create / update (incl. labels) / bulk update (MCP) / CSV import / recurring post / account corrections / §5a re-zero corrections | `transaction` repo `Save` (labels written inside it) | `BumpAccounts(old account, old recipient, new account, new recipient)` — **once per request** (see below) |
| Transaction delete | `transaction` repo `Delete` | `BumpAccounts(account, recipient)` read before the delete |
| Link transaction to recurring | `transaction` repo link method | `BumpAccounts` of that row |
| Category / payee / tag merge | each feature's repo merge method | `BumpWhere("<fk> = ?", sourceID)` before the `UPDATE` |
| Category / payee / tag delete | each repo delete method | `BumpWhere("<fk> = ?", id)` **before** the delete (after it, `SET NULL` has erased which rows were affected) |
| Label merge / delete | `label` repo | `BumpWhere("id IN (SELECT transaction_id FROM transactions_labels WHERE label_id = ?)", id)` before the write |
| Recurring template delete | `recurring` repo | `BumpWhere("recurring_id = ?", id)` before the delete |
| Import adopt (link inserted for an existing transaction) | `imports` repo link insert | `BumpAccounts` of the linked transaction |
| Import source delete (links cascade, `isImported` flips 1→0) | `imports` repo source delete | `BumpWhere("id IN (SELECT transaction_id FROM import_transaction_links WHERE source_id = ?)", id)` before the delete |
| User name / avatar change (author is embedded per row) | `user` repo name/avatar update | `BumpWhere("user_id = ?", userID)` |

Visibility changes (share accept/revoke, account delete, folder hide/show,
leaving a shared account) need **no** bump: the client derives them from the
accounts list itself.

**Exactly +1 per account per request** follows from the commit-time flush
(§2): the client's local shortcut trusts a response revision only when it is
exactly the stored revision + 1. Account create/update responses re-read the
revision after the correction row is written (the account row loaded at the
start of the request predates the bump), and the transaction/recurring
responses build `accounts` after their `WithTx` returns, so they carry the
committed value. Import link writes that set or clear `transaction_id`
(`InsertLink`, `UpdateLink`) bump the linked transaction's accounts, because
`isImported` is "any link row points at this transaction". The `user` repo
bumps only when `name` or `avatar` actually changes (it compares with the
stored row), so routine saves (login, options) never cause downloads.

### 4. Endpoint — `GET /api/v1/transaction/get-account-transaction-list`

- Query: `accountIds` — optional, comma-separated UUIDs, at most 200.
  **Absent = every account the caller can access** (first load, full reload),
  so the URL never grows with the number of accounts. Comma-separated rather
  than repeated `accountIds[]=` because the API has no array-param precedent
  and this sidesteps axios/Go serialisation differences. The 200 cap is input
  validation only: a few hundred UUIDs reach common proxy request-line limits
  (nginx's default 8 KB ≈ 200 ids) and it keeps the `IN (...)` list bounded.
  Stays a `GET` (reads only; the API reserves `POST` for writes). Present but
  empty, malformed, or more than 200 ids → 400.
- Access: the caller's `AvailableAccountIDs` (owned or accepted share, not
  deleted; hidden folders allowed) — the same rule as today's
  `get-transaction-list?accountId=`. Requested ids outside that set are
  **omitted** from the response, never a 403: an account can disappear between
  the client reading the accounts list and calling this endpoint (a share
  revoked in that moment), and the client treats "requested but absent from
  `revisions`" as gone.
- Reads all the requested accounts' `transactions_revision` values in one
  query **before** the rows, so a write landing in between yields an older
  revision with newer rows (one redundant download later, never stale data).
- Rows: `ListByAccountIDs` over the accessible ids (`account_id IN (...) OR
  account_recipient_id IN (...)`, ordered `spent_at DESC, id`), labels and
  `isImported` as today. Each row appears once, so a transfer between two
  requested accounts is not sent twice.
- Response `data`:
  ```json
  {
    "revisions": { "<accountId>": 42, "<accountId>": 7 },
    "items": [ { …TransactionResult fields…, "authorId": "…" } ],
    "authors": [ { "id": "…", "name": "…", "avatar": "face:fuchsia" } ]
  }
  ```
  Items are today's `TransactionResult` with `authorId` instead of the
  embedded `author`; `authors` lists each distinct author once for the whole
  response (sorted by id for stable goldens).
- Hand-written handler (query-param read, like the other query-param reads),
  swag annotations, route in `internal/transaction/api/routes.go`. No MCP tool.
- Once shipped it is frozen like every route (installed app bundles call it);
  later changes are additive only (e.g. an optional `sinceRevision` for
  row-level deltas).

### 5. Backend tests

- **Unit, per bump site** (sqlite via `dbtest`, and under `make
  test-repo-pgsql`): each row of the table above asserts the affected
  accounts' revision moved and unrelated accounts did not; the single-transaction
  writes assert exactly +1 (including transfers, labels, and an update moving a
  row between accounts — both old and new accounts +1).
- **Revision guard suite** (new, `internal/test/revisionguard`): replays the
  `apiparity` scenario catalogue and the `mcpparity` catalogue through the real
  `server.BuildAPI`. Around every request it fingerprints each account —
  a hash over the rows `get-account-transaction-list` would return (all
  columns, sorted labels, `isImported`, author id/name/avatar) — and fails,
  naming the route and account, when a fingerprint changed while the revision
  did not. Extra bumps are allowed. Runs in the smoke tier on sqlite and under
  `-tags enginecompare` on both engines. Because the apiparity guard already
  forces a scenario for every route, a new route that writes transactions
  without bumping fails CI.
- **apiparity**: scenarios + goldens for `get-account-transaction-list`
  (owner, shared reader, hidden-folder account, a transfer between two
  requested accounts appearing once, a foreign id omitted from `revisions`,
  no `accountIds` returning every accessible account including hidden-folder
  ones, empty / malformed / 201 ids → 400). Account-carrying goldens change by the new `transactionsRevision` field —
  inspect that diff.
- `enginecompare` covers the new endpoint; swagger regenerated (`make swagger`).

## Client

### 6. One IndexedDB database — `web/src/lib/db.ts`

- Database `econumo`, version 1, object stores:
  - `importKeys` — the SimpleFIN data key (out-of-line key `dataKey`, value
    `StoredKey`), moved from `econumo-import`.
  - `accountTransactions` — keyPath `accountId`; value
    `{ accountId, revision, items: StoredTransaction[], authors: UserDto[] }`
    (`authors` = the response's authors referenced by this bucket's rows).
  - `meta` — out-of-line key `state`; value `{ buster, userId, savedAt }`
    (`savedAt`: when buckets were last written, the seeded list's
    `dataUpdatedAt`; additive, a missing one reads as unknown).
- `db.ts` owns `open` / `onupgradeneeded` / `onversionchange` (close the
  connection so another tab's upgrade is never blocked) / a small promise
  `tx()` helper. One cached connection per page.
- **Migration from `econumo-import`**: on the first open, if `econumo-import`
  exists, copy its `dataKey` record into `importKeys`, then delete the old
  database. A `CryptoKey` survives structured clone, so nobody re-unlocks
  SimpleFIN. `importCrypto.ts` switches to `db.ts`.

### 7. Transactions store — `web/src/lib/transactionStore.ts`

- `buster = "<FORMAT_VERSION>:<getVersion()>"`. On load, a `meta` whose
  `buster` or `userId` differs from the current one wipes `accountTransactions`
  first. A release therefore re-downloads once (same rule as the query cache);
  `FORMAT_VERSION` covers `dev` builds whose version string does not change.
- `loadBuckets()`, `putBuckets(buckets)`, `deleteBuckets(ids)`,
  `clearTransactions()` (wipes `accountTransactions` + `meta`, leaves
  `importKeys`), and `tryLoadStored()` for the boot restore: this build's
  buckets with the `userId` and `savedAt` from `meta`, read-only (another
  build's data reads as nothing and is wiped by the next sync).
- Every IDB failure is caught, logged once (`console.warn`), and turns the
  store into a no-op for the rest of the session; callers never see an error.
  Slowness is no failure: only the waits that hold boot or a sign-in/out (a
  load, a clear) give up after 1.5 s, and the store stays on. Loads are
  readonly; a readwrite transaction is opened only to wipe a mismatched
  `meta`.

### 8. Sync — the new `queryFn` for `['transactions']`

1. `fetchQuery` the accounts and user queries (deduped with any fetch already
   in flight), and drop accounts with a share invite still pending for this
   user (the same `isPendingForMe` filter `useAccounts` applies — the server
   would omit them, and they would otherwise be re-requested on every sync).
   If any account lacks `transactionsRevision` (older server), return
   `getTransactionList()` as today and stop.
2. Load buckets: from IndexedDB only while the page holds no synced or
   restored list (first boot, after a login/logout); otherwise from memory
   (the cached list regrouped by account under the revisions it was synced
   at), so a sync never re-reads the whole store. Collect the accounts that have no bucket or whose revision
   differs (every account on a full reload). Nothing stale → no request.
   - **Accounts with no stored bucket** (a first load, a resumed one, a newly
     shared or created account) and **every account on a full reload** go
     **account by account**: one `get-account-transaction-list?accountIds=<id>`
     per account, at most two in flight, started in the accounts list order.
     Each account's bucket is built from its own response (its revision
     included) and stored as soon as it arrives (a background write,
     epoch-fenced, never awaited). A failed request fails the sync (the usual
     retry policy) without starting further requests, while the one in flight
     still lands and is stored; a cancel starts no further request. So an
     interrupted first load resumes: the next sync finds the finished accounts
     at their current revision and downloads each missing one on its own.
   - **Accounts with a bucket whose revision changed** go in one request with
     `accountIds` listing them; more than 200 of them go account by account
     too. When both kinds are stale, the batch runs first, then the
     per-account downloads.
   - The list lands only once every account is in.

   A response rebuilds the buckets of the accounts it was asked for: a row
   belongs to account A's bucket when its `accountId` or `accountRecipientId`
   is A.
3. Delete buckets whose account is no longer in the accounts list (share
   revoked, account deleted, account left) or was requested (explicitly, or
   implicitly by an all-accounts request) but is absent from `revisions`.
4. Persist the downloaded buckets (account by account as they arrive, or
   the one request's at once), then assemble the list from **every**
   bucket, hidden-folder accounts included, deduplicated by id (a transfer
   between two accounts sits in both buckets), each row's `author` filled from
   the bucket's `authors`. Sorting stays in `select: byDateDesc`.

The `staleTime` stays 10 minutes; nothing else about the query's options
changes. Because hidden-folder accounts are in the list, their account pages
filter it like any other account page: `useAccountTransactionList` and the
`['accountTransactions', id]` query (`gcTime: 0`) are removed, which also
removes the up-to-10-minute gap after un-hiding a folder. Folder hide/show no
longer touches transactions at all.

### 9. Transactions follow the accounts list

A query-cache subscription watches `['accounts']`: when its data changes and
any account's `transactionsRevision` differs from the stored bucket (or an
account appeared/disappeared), it invalidates `['transactions']`. So accepting
a share, an accounts
refetch on focus, or a mutation response all pull in exactly the changed
accounts, without each call site having to remember to invalidate
transactions.

### 10. Local edits

- Existing in-place `setQueryData` patches (create/update/delete, merges,
  classification deletes, recurring) stay as they are.
- **Write-through**: a query-cache subscription on `['transactions']`
  regroups the changed rows by account (object identity tells which rows
  changed) and writes the affected buckets, debounced ~500 ms, keeping each
  bucket's stored revision.
- **+1 shortcut** in `useApplyTransactionItem` (and recurring post, account
  correction): for each account the item touches (old and new account and
  recipient), if `response.accounts[id].transactionsRevision === stored + 1`,
  record the new revision on the bucket; otherwise leave it stale so the next
  sync downloads that account. Before applying, an in-flight transactions sync
  is cancelled (`cancelQueries`); the accounts subscription re-runs it if a
  revision still differs.
- A wrong local patch is therefore never permanent: any account whose stored
  revision is behind the server is downloaded again on the next sync.

### 11. Boot

- `['transactions']` is excluded from the localStorage persister
  (`shouldDehydrateQuery`).
- The persister used by `PersistQueryClientProvider` is wrapped: after the
  localStorage restore, `restoreClient` seeds `['transactions']` from
  IndexedDB **whenever a session token exists**, whether or not there is a
  snapshot and however old it is. This runs inside the existing restore, so
  `isRestoring` still suppresses the boot loader, and the load keeps the
  1.5 s limit.
  - Owner: the store's `meta.userId`. Every login and logout wipes the store,
    so `meta` names the session's user; a snapshot that holds `user` must
    match it or nothing is seeded. No token, nothing seeded.
  - Build: `meta.buster` must be the current one (a release wipes, as before).
  - Accounts: when the snapshot holds `accounts`, only their buckets (pending
    invites skipped, buckets of accounts no longer listed deleted); without
    them every stored bucket is shown and the sync that follows corrects the
    set (gone accounts dropped, new ones downloaded). A snapshot whose
    accounts carry no revisions (an older server) seeds nothing.
  - Snapshot age: a snapshot of this release older than the 24 h `maxAge` is
    kept, re-stamped, when the store seeds; without its accounts and the rest
    the boot loader would cover the seeded list on an offline start after a
    day away. A snapshot of another release is dropped and the seeded list
    is restored alone. `updatedAt` = the snapshot's time for a fresh
    snapshot, else `meta.savedAt`.
- `refreshRestoredQueries` then invalidates as today (also when the seeded
  list is the only restored query); the transactions sync fetches the
  accounts itself and downloads only accounts whose revision moved.

### 12. Sync button

Both sidebar variants in `ApplicationLayout.tsx`:

- **Tap**: `invalidateQueries()` — unchanged code, now incremental for
  transactions.
- **Long-press** (pointer held ≥ 600 ms; mouse or touch, pointer events):
  `clearTransactions()`, mark the next sync as a full reload, `invalidateQueries()`,
  toast `settings.sync.full_reload` ("Reloading all data from the server…").
  The click that follows the long-press is suppressed; `pointerleave` /
  `pointercancel` abort the timer; `-webkit-touch-callout: none` and
  `user-select: none` stop the iOS callout. `Shift`+click does the same, for
  keyboard and desktop users.
- Tooltip gains a second line `settings.sync.hold_hint` ("Hold to reload
  everything").
- The same full reload (one `useFullReload()` hook,
  `web/src/features/transactions/useFullReload.ts`) is the **last row of
  Settings → Data → Import & export** (`/settings/data`,
  `ImportsDataPage`): "Reload all data" (`settings.data.reload_all`) with the
  muted hint `settings.data.reload_all_hint` ("Downloads everything from the
  server again. Use it if something looks out of date."). No confirmation:
  it only downloads again. The row is disabled while the transactions load,
  so it cannot be triggered twice.

### 13. Logout / login

`clearPersistedQueryCache()` becomes `async` and also awaits
`clearTransactions()`; its callers (`useLogin`, `LogoutPage`, oauth exchange)
await it. The `meta.userId` check is the second line of defence for paths that
skip it (a 401 that sends the user to the login page).

## Analytics

New key `DATA_FULL_RELOAD: 'appDataFullReload'` in `METRICS`, fired when a
full reload starts, with `{ source: 'sync_button' }` (long press or
Shift+click) or `{ source: 'settings' }` (the Import & export page). The plain tap
fires nothing (unchanged).

## i18n

Two keys under `settings.sync` in all 11 catalogues: `full_reload`,
`hold_hint`; two under `settings.data`: `reload_all`, `reload_all_hint`.

## Testing (frontend, vitest + `fake-indexeddb` dev dependency)

- `db.ts`: fresh create; migration of `econumo-import` → `importKeys` and
  deletion of the old database; `onversionchange` closes the connection.
- `transactionStore`: buster/userId mismatch wipes; failure turns it into a
  no-op with one warning.
- Sync `queryFn`: missing bucket → download; equal revision → no request;
  changed revision → download; several stale accounts → one request listing
  them; 201 changed accounts, a first load, the missing accounts of a
  resumed load and a full reload → one request per account, at most two in
  flight, each stored on arrival; changed and missing accounts in one sync →
  one batch plus one request per missing account; a failure keeps what
  arrived and the next sync downloads only the rest, one by one; a cancel starts no
  further request; removed account, or one absent from `revisions` → bucket
  deleted; hidden-folder account's rows are in the list; transfer between two
  accounts appears once; authors filled in; older server → legacy
  `get-transaction-list`, nothing stored; full reload downloads every account.
- Accounts subscription: a revision change and a share accept each trigger a
  sync of exactly the affected accounts; a folder hide/show triggers none.
- +1 shortcut: no download at exactly +1; download at +2.
- Write-through: a category-merge patch reaches IndexedDB.
- Boot (`ApplicationLayout.test.tsx`): restored from IndexedDB with no boot
  loader, also with a snapshot older than 24 h and with none at all; no
  stale transactions after a user switch; nothing seeded without a token,
  for a snapshot of another user, or from another build's store.
- Sync button: tap → incremental; long-press and Shift+click → full reload,
  toast, metric (`source: 'sync_button'`); a short press does not trigger the
  full reload. "Reload all data" is the last row of Import & export and does the
  same (`source: 'settings'`).

## Docs

- `docs/regression-test-plan.md`: offline reload still shows transactions
  📱; edit on device A appears on device B after refocus; a partner's edit on a
  shared account appears; accepting / revoking a share adds / removes that
  account's history; a hidden-folder account's page shows its transactions,
  and global search finds them;
  logout → login as another user shows none of the previous data; sync tap
  refreshes; sync long-press 📱 / Shift+click reloads everything with a toast;
  an offline start more than 24 h after the last visit still shows
  transactions 📱; an interrupted first load resumes; Settings → Data →
  Import & export → Reload all data.
- `CLAUDE.md`: the revision counter and the "bump inside the repository
  method" rule with the guard suite; the `econumo` IndexedDB layout; the new
  endpoint in the transaction feature.

## Rollout

One PR. Backend and SPA ship in the same binary, so there is no deploy-order
issue. The first load after upgrading downloads every account once; after that
only changed accounts. A mobile app bundle talking to an older self-hosted
server takes the legacy path.

## Out of scope

- Moving the rest of the query cache out of localStorage.
- Row-level deltas within an account (an edit re-downloads that whole account).
- Server-side paging or windowing for the account page or search.
- Offline writes / a mutation queue.
- The budget `get-transaction-list` endpoint (server-filtered per period;
  unchanged).
