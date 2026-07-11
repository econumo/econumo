# Transaction Partial Loading & Server-Side Classification Sorting — Design

**Date:** 2026-07-11
**Status:** Approved

## Problem

The SPA fetches **every** transaction of every visible account in one unbounded
`GET /api/v1/transaction/get-transaction-list` call at app boot. The call blocks
the boot loader, the full array lives in memory for the whole session, and the
TanStack Query persister writes it to `localStorage` (~5 MB budget, fails
silently when exceeded). Both problems grow linearly with account history.

Goals (in priority order):
1. **Bound memory / localStorage** — hold only a window of transactions.
2. **Fast boot** — bounded initial payload.

Companion change: the classification "sort list" action (alphabetical today,
usage-based planned) sorts the full list client-side and pushes a position diff.
Move the sorting to the backend, which also unlocks usage-based sorting (the
data lives server-side).

## Current state (verified)

- Boot: `useTransactions()` (`web/src/features/transactions/queries.ts`) fetches
  with no params; backend returns all transactions of visible accounts (hidden
  folders already excluded server-side in `internal/transaction/read.go` via
  `VisibleAccountIDs`). Cached under `['transactions']`, persisted to
  localStorage, patched in place by create/update/delete mutations.
- Rendering: `AccountPage.tsx` already windows the DOM in 100-row chunks with an
  IntersectionObserver sentinel — over the full in-memory array.
- Search: client-side only (`useAccountTransactions.ts` haystack over resolved
  names, amounts, author, type, date), per account.
- Consumers of the full list: account list+search (`useAccountTransactions`),
  budget dialog **editability id-lookup** (`BudgetTransactionsDialog.tsx` —
  a row not found in the list is rendered read-only), onboarding `length > 0`,
  mutation cache-patchers. Category/payee/tag ordering does **not** derive from
  transactions (server `position` field). Balances, budget math, CSV export are
  already server-side.
- Backend: `get-transaction-list` supports optional `accountId`,
  `periodStart`/`periodEnd`. No pagination anywhere in the API. Sort order is
  `spent_at DESC, id ASC` with `(account_id, spent_at)` /
  `(account_recipient_id, spent_at)` indexes.
- Classification sorting: `SortDialog.tsx` (alphabetical asc/desc live;
  usage-count options dormant since the Vue app) sorts client-side, then
  `ClassificationList.tsx` diffs positions and POSTs `{changes: [{id, position}]}`
  to `order-{category,payee,tag}-list`.

## Decisions made

- **Load model**: boot fetches the ~50 newest transactions **per visible
  account**; scrolling an account loads older chunks for that account.
- **Chunking**: N transactions with a **keyset cursor** over the existing
  `(spent_at DESC, id ASC)` order. (Day-based and offset chunking rejected:
  empty/oversized chunks, shifting offsets.)
- **Search**: fetch-all-on-search per account (legacy `accountId` mode), then
  the existing client-side haystack — identical results, cost paid only when
  searching. (Backend search rejected for v1: would have to replicate haystack
  semantics.)
- **API**: extend `get-transaction-list` additively; legacy responses stay
  byte-identical. (New endpoints rejected: more surface for the same result.)
- **Frontend cache**: keep the single flat `['transactions']` array (union of
  loaded windows) + a small per-account page-state map. (Per-account
  `useInfiniteQuery` rejected: transfer rows duplicate across caches, every
  mutation would patch N caches.)
- **Classification sorting**: new `sort-*-list` endpoints; backend sorts and
  renumbers. `by=name` and `by=usage` (sliding 1–6 month window, user-picked
  in the dialog) both ship in v1.

## 1. API contract — `get-transaction-list` (additive)

Three new optional query params give the endpoint two new modes. **No params →
today's behavior, byte-identical** (goldens untouched, other clients
unaffected).

### Boot mode — `?perAccountLimit=50`

The `perAccountLimit` newest transactions per **visible** account (same
hidden-folder exclusion as legacy), deduplicated by id (a transfer in both
accounts' top-N appears once). Response adds a per-account pagination block:

```json
{ "items": [ ... ],
  "accounts": [ { "id": "<accountId>", "nextCursor": "...", "hasMore": true }, ... ] }
```

Every queried visible account gets an entry; an account with fewer than N rows
comes back `hasMore: false` with no cursor.

### Page mode — `?accountId=…&limit=50[&cursor=…]`

Keyset pagination for one account (view-access checked as today). Without
`cursor`: the newest `limit` rows. With it: the next `limit` rows strictly older
than the cursor position. Response adds:

```json
{ "items": [ ... ], "page": { "nextCursor": "...", "hasMore": true } }
```

### Cursor

Opaque `base64url("<spent_at in the frozen Y-m-d H:i:s layout>|<id>")`, pointing
at the last returned row. Keyset predicate (mixed-direction order):
`spent_at < s OR (spent_at = s AND id > i)`. Malformed cursor → 400 validation
error. Predicates compare values, not row existence — pagination survives the
anchor row being deleted mid-scroll.

### Response envelope

`page` and `accounts` are nested optional objects (pointer structs with
`omitempty`), so legacy responses serialize byte-identically. No top-level
scalar additions (avoids the `omitempty`-drops-`false` trap).

### Param rules

- `perAccountLimit` is exclusive with every other param.
- `cursor` requires `accountId` + `limit`; `limit` requires `accountId`.
- `periodStart`/`periodEnd` stay legacy-only (exclusive with the new params).
- `limit`, `perAccountLimit`: integers 1–500.

Legacy modes stay load-bearing: `accountId` alone = search fetch-all;
`periodStart`+`periodEnd` = budget-dialog month window.

## 2. Backend implementation

- **Page query**: sqlc-generated, two variants (first page / after-cursor) per
  engine; `WHERE (account_id = ? OR account_recipient_id = ?)` + keyset
  predicate + `ORDER BY spent_at DESC, id LIMIT ?`, served by the existing
  `(account_id, spent_at)` / `(account_recipient_id, spent_at)` indexes.
- **Boot query**: hand-built dynamic SQL in `internal/transaction/repo`
  (precedent: `ListByAccountIDs`): `UNION ALL` of account-side and
  recipient-side rows, `ROW_NUMBER() OVER (PARTITION BY account ORDER BY
  spent_at DESC, id)`, filter `rn <= perAccountLimit`. Window functions work on
  both engines (SQLite ≥3.25 under modernc; PostgreSQL native). Go code dedupes
  by id and derives each account's `nextCursor`/`hasMore` from its partition
  **before** dedup.
- **Use case** (`internal/transaction/read.go`): cursor encode/decode lives
  here; boot mode reuses `VisibleAccountIDs`, page mode reuses
  `checkViewAccess`. Request parsing/validation in
  `model.TransactionListRequest.Validate()`.

## 3. Server-side classification sorting

New endpoints (one per classification, POST like every write):

```
POST /api/v1/category/sort-category-list   {"by": "name"|"usage", "direction": "asc"|"desc", "periodMonths": 3}
POST /api/v1/payee/sort-payee-list         (same body)
POST /api/v1/tag/sort-tag-list             (same body)
```

- `periodMonths` (integer 1–6) is **required** with `by=usage`, **rejected**
  with `by=name`.
- **`by=name`**: load the user's full list (including archived — same set the
  client sorts today), sort case-insensitively **in Go** (no SQL collation →
  no engine drift; slight fidelity difference vs the browser's `localeCompare`
  for accented/non-Latin names — accepted).
- **`by=usage`**: usage = count of transactions referencing the entity
  (`category_id`/`payee_id`/`tag_id`) with `spent_at >= cutoff`; cutoff =
  now − `periodMonths` months, computed in Go via the Clock port (UTC) and
  bound as a plain SQL parameter (no per-engine date math). One `GROUP BY`
  query per sort call. Sort by count per `direction`; ties (incl. zero usage)
  break by name asc, then id.
- Positions renumbered `0..N` in one transaction; response is the full
  reordered list in the same `{items: [...]}` shape as `order-*-list`, so the
  frontend's existing `replaceAll` cache op just works.
- **Frontend**: `SortDialog` calls the new endpoint instead of sorting locally;
  the usage buttons return, with a 1–6 month period selector whose last choice
  is remembered client-side. Drag-and-drop keeps using `order-*-list`
  unchanged (a genuine "apply my explicit order" operation).

## 4. Frontend data layer (transaction windows)

Two cached artifacts replace the single unbounded list:

- **`['transactions']`** — the same flat, deduped-by-id array as today, now
  holding the **union of loaded windows**. All existing consumers and mutation
  patchers keep working against it unchanged.
- **`['transactionPages']`** — map `accountId → { nextCursor, hasMore,
  oldestLoaded }`, seeded from the boot response's `accounts` block, advanced
  by page fetches. (`oldestLoaded` = the `(spent_at, id)` key of the oldest
  loaded row for that account.)

**Boot**: `useTransactions` calls boot mode (`perAccountLimit=50`); items →
`['transactions']`, page state → `['transactionPages']`. Both persist to
localStorage — now bounded (~50 × visible accounts + scrolled chunks, capped by
the existing 24h maxAge/version buster). Restore-then-invalidate refetches boot
and resets windows, as today.

**Scroll**: a new `useAccountTransactionPager(accountId)` hook drives the
account page's existing IntersectionObserver sentinel: when rendered entries
are exhausted and `hasMore`, fetch page mode with the cursor, merge into
`['transactions']` (dedupe by id), advance the page entry. One in-flight
request per account.

**Horizon rule**: `useAccountTransactions` shows only rows within the account's
loaded horizon (`(spent_at, id)` not older than `oldestLoaded`, unless
`hasMore` is false). This hides a transfer loaded via the *other* account's
window that is older than this account's horizon — otherwise it would render
after a misleading gap of "missing" days.

**Ensure-window**: if the account page mounts with no `['transactionPages']`
entry for the account (hidden-folder accounts are excluded from boot), fetch
its first page on mount. Deliberate behavior change: **hidden-folder account
pages now show transactions** (today they silently render empty). Treated as a
fix.

**Mutations**: patch logic unchanged — create prepends (newest → inside the
horizon), delete filters, the account-delete cascade also drops the account's
page entry, classification-delete field-nulling untouched. An update that moves
a transaction's date beyond the horizon leaves it cached but hidden by the
horizon filter (it belongs to an unloaded range).

## 5. Search and dependent consumers

- **Account search**: while the search box is non-empty, query
  `['transactionSearch', accountId]` fetches the account's **full** list
  (legacy `accountId` mode); the existing haystack runs over it — results
  identical to today. Enabled only while searching, short gcTime, existing
  loading state while fetching, **excluded from localStorage persistence**
  (`shouldDehydrateQuery` filter in `lib/queryPersist.ts`).
- **Budget transactions dialog**: on open, a companion query fetches the budget
  month via legacy `periodStart`/`periodEnd`; the editability id-lookup
  consults the flat cache **plus** this month window. Preserves today's
  semantics (own rows editable, shared-partner rows read-only synthesized from
  the budget wire). Also excluded from persistence.
- **Onboarding** (`length > 0`), **CSV export/import**, **boot gating /
  useLastSyncAt**: no changes — none depends on list completeness.

## 6. Edge cases & error handling

- **Failed chunk fetch**: sentinel stays armed; re-intersection retries; error
  surfaces via the existing toast pattern; the list keeps what's loaded.
- **Deleted cursor anchor**: handled structurally (value-based keyset).
- **Cross-device writes**: the boot query's existing 10-min staleTime refetch
  resets windows to the newest 50 per account — stale windows self-heal.
- **Same-timestamp runs**: `id` tie-break makes chunk boundaries stable even
  for bulk-imported rows sharing one `spent_at`.
- **Empty visible-account set**: boot mode returns `items: []`, `accounts: []`
  (matches legacy's early-nil behavior).

## 7. Testing

- **Backend**: repo tests for partition/keyset edges — transfers counted in
  both partitions, same-timestamp ties, exact-limit boundaries, cursor
  round-trip; usage-sort counting per entity/window. New apiparity scenarios +
  goldens for boot mode, page mode (first/after/last page), and the three
  sort endpoints — existing goldens must not change. `make test-repo-pgsql` and
  the enginecompare suite replay everything on PostgreSQL byte-identically.
- **Frontend (vitest)**: pager hook (merge/dedupe/horizon advance, single
  in-flight); `useAccountTransactions` horizon filtering and search mode;
  SortDialog period selection + endpoint call; budget-dialog lookup
  fallback (cache hit, month-window hit, read-only synthesis).

## Out of scope

- Backend full-text transaction search.
- True list virtualization (the grow-only 100-row DOM window stays).
- Pagination for any other endpoint (budget transaction list stays unbounded —
  it is month-scoped, hence naturally bounded).
- Changing the drag-and-drop reorder contract (`order-*-list`).
