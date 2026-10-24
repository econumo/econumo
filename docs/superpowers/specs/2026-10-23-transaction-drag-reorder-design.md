# Drag to reorder transactions within a day, and to move them between days

## Why

Dmitry reconciles an account by putting the bank statement next to the
Econumo account page and going down both lists line by line. Two things get
in the way:

1. **The order within a day doesn't match the bank.** Inside a day group the
   list is sorted by the transaction's time of day, which is hidden. A new
   transaction gets the time it was entered (`formatDateTime(new Date())` in
   `useTransactionForm.ts`) and the date picker keeps that time when the date
   changes. So the list follows the order things were typed in, not the order
   the money was spent, and nothing lets the user change it. It is
   "just uncomfortable to compare as the order is different."
2. **A transaction is sometimes on the wrong day** because it was typed in
   wrong. Today the fix is: open the edit dialog, change the date, save. He
   wants to drag the row onto the right day instead, which is "way faster and
   simpler".

Success: he can make the account page read in the same order as the bank
statement, with no dialogs, on both the computer and the iPhone home-screen
app.

## Goals

- Drag a transaction up or down within its day; the new order is saved and is
  the same after a reload, on other devices, and for other people sharing
  the account.
- Drag a transaction into another day group; it takes that day's date at the
  position where it is dropped.
- Works on desktop and on phone (touch), including the iOS home-screen PWA.
- Each account's order is independent: reordering one account never changes
  the order of another, transfers included.

## Non-goals

- No drag on other lists that render `TransactionRow` (search results, Inbox,
  recurring settings, budget sheets). Account page only.
- No dropping onto a day that has no transactions in the list (there is no
  group to drop into). The edit dialog still covers that.
- No visible or editable time-of-day field in the transaction dialog.
- No MCP tool for reordering.
- No drag while a search filter is active on the account page (the visible
  neighbours aren't the real neighbours).

## Anti-goals (would be a failure even if it "works")

- A tap that should open the preview starts a drag, or a scroll on the phone
  picks up a row. Normal tapping and scrolling must feel exactly as before.
- The visible desktop list gets cluttered: the drag handle shows only on
  hover (and on keyboard focus), not on every row all the time.
- A move bypasses the invariants that `update-transaction` enforces (write
  access, the deleted-account self-healing correction, the
  `transactions_revision` bump).
- The order silently snaps back after a reload because the saved order is not
  what was shown.
- Reordering rewrites the times the user entered (decided by Dmitry: the
  time of day is no help for ordering and must not be repurposed).

## How it behaves (decided with Dmitry)

- **Desktop:** hovering a row shows a grip icon (`GripVertical`, as on the
  accounts settings page). Only the grip starts a drag. Clicking the row
  still opens the preview; the kebab menu stays.
- **Phone:** press and hold a row (about 500 ms) to lift it, then drag. A
  quick tap still opens the preview, and a normal swipe still scrolls. The
  list auto-scrolls near the top/bottom edge so a row can be carried into the
  next or previous day.
- While dragging, a drop indicator shows where the row will land; when the
  drop position is in a different day group, that day's header is
  highlighted so it's obvious the date will change.
- Dropping commits immediately (no confirmation). The list updates at once
  (optimistic); on failure it snaps back and shows the server error via
  `apiErrorMessage`.

## Storage: an explicit sort key per account side (decided with Dmitry)

Order within a day comes from an explicit key, not from the time:

- `transactions.sort_key` (INTEGER NOT NULL): position within the day on the
  SOURCE account (`account_id`).
- `transactions.recipient_sort_key` (INTEGER, NULL except on transfers):
  position within the day on the RECIPIENT account (`account_recipient_id`).
  Two keys because a transfer is one row shown in two accounts; one shared
  key would make reordering account A scramble the transfer's place in
  account B and vice versa.
- **Order:** day (date part of `spent_at`) DESC, then the side's key DESC,
  then `id` as the final tie-break. Higher key = higher in the day's list.
  The time of day is stored and shown as entered (preview dialog, CSV
  export) but no longer sorts anything.
- "The side's key" for a list scoped to account X: `sort_key` when
  `account_id = X`, `recipient_sort_key` when `account_recipient_id = X`.
  Lists not scoped to one account (global search, `get-transaction-list`,
  MCP, multi-account CSV export) use `sort_key`.
- **Upgrade** looks exactly as before: the migration (below) numbers each
  side's days in the current order.
- **New rows** (typed in, CSV import, Apple Wallet, SimpleFIN, recurring
  post, corrections): go to the top of their day on each side: key =
  1 + the highest key on that account's day (looking at both columns for
  that account). In a batch, a row inserted later lands above the earlier
  ones; the CSV importer inserts each day's rows bottom-up so the list shows
  them in file order top to bottom (Claude's call).
- **Edits:** changing the date in the edit dialog puts the row at the top of
  the new day on each side. Changing a transfer's source or recipient
  account puts it at the top of its day on the newly involved account. Any
  other edit keeps the keys.

### Migration (decided with Dmitry: plain SQL, not a command step)

One SQL migration per engine, like `20261017000000`: add the columns
(`sort_key INTEGER NOT NULL DEFAULT 0`, `recipient_sort_key INTEGER`),
backfill each side with `ROW_NUMBER() OVER (PARTITION BY <side account>,
<day> ORDER BY spent_at, id DESC)` via `UPDATE ... FROM`, then bump every
account's `transactions_revision`. Day = `substr(spent_at, 1, 10)` on
SQLite (safe: the `20260915000001` normalizer runs earlier at boot),
`spent_at::date` on PostgreSQL. `recipient_sort_key` is filled for
transfers only.

### Keys everywhere (Dmitry: "make sure we add sort_keys everywhere")

No path may write a transaction without its keys, and no list may sort
without them. Assignment lives in the REPOSITORY layer, never in use cases,
so a new caller can't forget it (Claude's call):

- **Writes.** Both SQL statements that insert transactions:
  `UpsertTransaction` (`query/{sqlite,pgsql}/transactions.sql`) and the
  correction insert (`query/{sqlite,pgsql}/account_corrections.sql`). The
  transaction repo's `Save` assigns top-of-day keys for a new row, and on an
  update whose day or side account changed; otherwise it keeps them. The
  correction insert takes top-of-day in SQL. Every creator goes through one
  of these: REST and MCP create/update, MCP bulk update (keeps keys), CSV
  import, recurring post, Apple Wallet / SimpleFIN imports (the
  transaction-creation port), account corrections, deleted-account and
  deleted/edited-transaction re-zeroing corrections.
- **Copies.** `data:import-sqlite` must carry both columns;
  `internal/test/fixture` must set them the way the repo does.
- **Entity and wire.** `model.Transaction` carries both keys; the REST DTO
  (`get-transaction-list`, `get-account-transaction-list`, create/update
  responses) and the MCP transaction output expose `sortKey` /
  `recipientSortKey`. CSV export sorts by them but adds no column
  (Claude's call: it's internal ordering, not user data).
- **Every sort.** Server `ORDER BY spent_at DESC, id` queries become day,
  side key, id. Client: the account list comparator
  (`useAccountTransactions.ts`), `byDateDesc` (`queries.ts`), global search
  (`useGlobalSearch.ts`), and any other place that sorts transactions by
  `date` (the builder greps for them, budget/plan transaction sheets
  included).
- **Guard test (Claude's call).** Alongside `revisionguard`, replay the
  `apiparity` and `mcpparity` catalogues and fail if any transaction ends up
  with `sort_key = 0`, a transfer without `recipient_sort_key`, a
  non-transfer with one, or two rows sharing a key on the same account side
  and day. A future write path that skips the keys fails here.

## Server: a reorder action

New route `POST /api/v1/transaction/move-transaction` (Claude's call; a
dedicated action rather than re-sending `update-transaction`, so the client
can't replace labels or other fields with stale values).

```json
{ "id": "<moved transaction id>", "accountId": "<page account id>",
  "date": "YYYY-MM-DD", "transactionIds": ["<top of day>", "...", "<bottom>"] }
```

- `accountId` is the account page it was dragged on; `transactionIds` is the
  full intended order of that account's `date` after the drop, top first,
  the moved row included. The client sends what the user saw.
- The server sets that side's keys for the listed rows to N..1 (top = N) in
  one DB transaction. Only that account's side is written, so no other
  account's order changes. Rewriting the whole day is fine: keys are never
  shown.
- If `date` differs from the moved row's current day, its `spent_at` date
  changes and the time of day is kept. That date change goes through the
  same validated path as `UpdateTransaction` (write access, the
  deleted-account re-zeroing correction dated at the new `spent_at`). On the
  OTHER side of a transfer moved to a new day, the row goes to the top of
  its new day there.
- **Write access:** reordering needs write access to `accountId` (it only
  writes that account's keys, so the other side's access doesn't matter for
  a pure reorder). Moving to another day changes the transaction itself and
  needs the same access as editing it (`canTouchTransaction` rules).
- **Stale list (Claude's call):** the server compares the set of ids with
  the transactions currently on that day in that account (plus the moved
  row). If they differ (another device or a sharing partner changed the day
  meanwhile) it refuses with a coded error (new `transaction.day_changed`,
  translated in every catalogue); the client snaps back, toasts, and the
  next sync brings the fresh list.
- **Revision:** one `transactions_revision` bump per affected account (the
  page account; plus the other side when the date changed), through
  `txrevision`, as every write does. The keys are on the wire, so
  `revisionguard`'s fingerprint must include them.
- Response: the rewritten transactions in the shape the SPA's sync patches
  from, so the list updates without a full download.
- Readonly access (402) applies as for every other POST.

## Client details

- Wire: `TransactionDto` gains `sortKey` and `recipientSortKey` (number or
  null). A helper picks the key for the page account; the comparators in
  `useAccountTransactions.ts` and `queries.ts` (and global search) sort by
  day, key, id instead of the full datetime. A server too old to send keys
  falls back to the old datetime order.
- dnd-kit is already in use (`SortableList.tsx`, `AccountsSettingsPage.tsx`,
  budget pages). Desktop uses a `PointerSensor` bound to the grip
  (`setActivatorNodeRef`); touch uses a `TouchSensor` with a press delay
  (~500 ms, small tolerance) on the whole row. A keyboard sensor on the grip
  is a nice-to-have (Claude's call: include it if cheap).
- The account history list (`TransactionHistory.tsx` / `HistoryNavigator.tsx`)
  renders day separators and rows as one flat, windowed entry list
  (`entries.slice(range.start, range.end)`); the drop position is resolved
  to (day, ordered ids of that day) from the flat list. The window must keep
  rows near the drag in range while auto-scrolling.
- Optimistic update through the existing `patchTransactions` path.
- iOS: suppress the long-press text selection / callout on the row
  (`-webkit-touch-callout: none`, `user-select: none` while pressing) and
  make sure the press doesn't also fire the row's click. `navigator.vibrate`
  is a no-op on iOS; use it where it exists, don't rely on it.
- Not draggable: recurring preview rows (virtual, and due ones pinned to the
  top of today), rows the user can't touch, and every row while the search
  box has text. Real future-dated transactions are draggable. A row that can
  be reordered but not edited (e.g. a transfer whose other side the user
  can't write) may move within its day but not to another day.
- Dropping is only allowed inside a day group, not onto the "today" pinned
  recurring block.
- Analytics: add `TRANSACTION_MOVE: 'appTransactionMove'` to `METRICS`, fired
  on mutation success with `{ dayChanged: boolean }`.

## Tests and docs

- Go: migration backfill (order before == order after, both engines, both
  sides of transfers); new-row placement (single, batch, both sides);
  edit placement (date change, transfer account change); move-transaction
  (reorder, cross-day, transfer on either side, stale list, access, deleted
  account correction); `apiparity` scenario + golden; `enginecompare`;
  `revisionguard` covering the new fields. Existing goldens gain the two
  fields (inspect the diff).
- Frontend: vitest for the comparator (side selection, fallback), resolving
  a drop to (day, ids), optimistic update + rollback, tap still opens the
  preview.
- Update `docs/regression-test-plan.md` (Account page section): reorder within
  a day, move to another day, a transfer reordered in one account keeps its
  place in the other, tap/scroll on 📱 unaffected, handle only on hover on
  desktop, the entered time is unchanged after a reorder.
- Needs an on-device check on the iPhone home-screen app before calling it
  done (long-press vs scroll vs tap).

## Left to the builder

- Final request/response field names (keep the API conventions: POST,
  kebab-case path, envelope).
- Indexes for the "highest key on this account's day" lookup, if needed.
- Visual details of the drop indicator and the lifted row.
- Whether to reuse `SortableList` or build the sortable directly on the
  history list.
