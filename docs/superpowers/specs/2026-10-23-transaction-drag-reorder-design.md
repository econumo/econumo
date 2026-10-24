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

## Storage: fractional sort keys per account side (decided with Dmitry)

Order within a day comes from explicit keys, never from the time of day. They
are the fractional index keys the rest of the app already uses for categories,
tags, payees, accounts and budget elements (`internal/shared/sortkey`): short
base-62 text keys compared byte-wise, with `Between(prev, next)` always finding
a key between two others, so **a move writes one row and never renumbers the
rest of the day** (Dmitry: "I don't want to renumber other transactions").

- `transactions.sort_key` (TEXT, `COLLATE "C"` on PostgreSQL, `''` = not placed):
  position within the day on the SOURCE account.
- `transactions.recipient_sort_key` (same, NULL except on transfers): position on
  the RECIPIENT account. Two keys because a transfer is one row shown in two
  accounts.
- **Order:** day DESC, then the side key of the listed account DESC (`sort_key`
  for lists spanning several accounts), then `id`. The time of day is stored and
  shown as entered but sorts nothing.
- **New rows** (typed in, CSV, Apple Wallet, SimpleFIN, recurring post,
  corrections) go to the top of their day: a key just above the highest either
  column holds for that account and day. The CSV importer inserts each day
  bottom-up so the list shows the file's order.
- **Edits** keep the keys unless the day or that side's account changes; then
  that side goes to the top of its new day.

### Migration (decided with Dmitry: a command step)

`20261024000000.sql` only adds the two columns. The command step
`migration:seed-transaction-sort-keys` (`20261024000001`) generates the keys in
Go with the same `sortkey` package every later write uses, numbering each
account's days in the order the lists showed before (spent_at DESC, id), the
rows the account is the source of and the transfers it receives counted
together, so an upgrade changes nothing visible. One account per DB
transaction, its revision bumped; idempotent (only unplaced rows are touched).

### Keys everywhere (Dmitry: "make sure we add sort_keys everywhere")

Assignment lives in the storage layer only, so no caller can store an unplaced
row: the transaction repo's write (every create/update path, MCP and imports
included), the account correction insert, and the test fixture all use
`internal/infra/txorder`. `data:import-sqlite` copies the columns by schema. The
REST and MCP transaction output carry `sortKey` / `recipientSortKey`; CSV export
sorts by them but adds no column. `revisionguard` fails any write that leaves a
row without its keys.

## Server: move-transaction

`POST /api/v1/transaction/move-transaction`:

```json
{ "id": "<moved transaction>", "accountId": "<page account>",
  "date": "YYYY-MM-DD", "aboveId": "<row now directly above it, or null for the top>" }
```

- The server gives the moved row, on that account's side only, a key between
  `aboveId`'s key and the key of the row currently below it (or above the
  highest key for the top of the day). One row is written; the other account of
  a transfer keeps its order.
- Rows whose keys tie (two inserts that raced for the same key) have no key
  between them; those tied rows are split first, keeping the order the list
  shows. This is the only case a second row is written.
- An `aboveId` that is no longer on that account's day (moved or deleted on
  another device) is refused with `transaction.day_changed`; a row merely added
  to the day elsewhere does not block the drop.
- A different `date` changes the date (time kept) through the same checks as an
  edit: write access to every account the transaction touches
  (`transaction.cannot_move`), the deleted-account re-zeroing. Reordering alone
  needs write access to the page account only.
- One `transactions_revision` bump per affected account; the response carries the
  rewritten rows and the account list.

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
- The page keeps the dropped arrangement on screen until the server answers,
  then follows the cache again (the server's keys, or the old order after a
  refusal, which also triggers a sync); the client never computes keys.
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
