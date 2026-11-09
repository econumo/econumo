# Offline transactions and limits

## Why

The installed PWA already opens and reads offline, but every write pauses until a
connection returns. On a plane, tapping Add on a transaction leaves the dialog frozen, and
the write is lost if the app closes. The owner wants to log transactions and adjust the
budget while offline, see the result at once, and never meet a control that looks available
but does nothing.

## Goals

- Add, edit and delete transactions offline.
- Change a limit offline, in the Budget view and the Plan grid (single cell and multi-cell
  fill).
- Changes appear immediately and are identifiable as not saved yet.
- Waiting changes survive closing the app or restarting the phone, and send by themselves,
  in order, when the app is open and the server is reachable.
- Each operation carries an `Idempotency-Key` (UUIDv7), and the server refuses one it has
  already handled, so a retry can never apply a change twice.
- A change the server refuses appears in the Inbox, where tapping it lets the user save
  again.
- Everything that cannot work offline is disabled in the UI.

## Non-goals

- Offline budget structure (folders, envelopes, moves, currency), budget comments, budget
  create/archive/share.
- Offline accounts, categories, payees, tags, labels, recurring, imports, settings, sharing.
- Offline reordering of transactions by drag.
- Exact Spent / Available / carry-over figures for offline transactions. The budget figures
  update from the server once the change is confirmed.
- Sending while the app is closed (iOS gives a PWA no background sync).
- Merging concurrent edits from two devices: the last write to reach the server wins, as it
  does today.
- Months the device never loaded stay unavailable offline, as they are today.

## Anti-goals

- A dialog or button that hangs waiting for a network.
- A change that silently disappears.
- A retry that creates a duplicate transaction.
- One user's waiting changes being sent under another user's session.

## User-visible behaviour

### Saving

- **Offline:** Add/Save closes the dialog at once and the change is applied locally.
- **Online:** the dialog waits for the server as today, and a validation refusal shows in
  the dialog as today. If there is no answer within about 10 seconds, or the request fails
  with no response, the change becomes a waiting change and the dialog closes.

### How a waiting change looks

The user decided this: **the "Not saved" label is shown in the transaction preview
(`ViewTransactionDialog`), not in the list.**

- A waiting added or edited transaction appears in every list with its new values and looks
  like any other row. Its preview shows the "Not saved" label; a failed one shows the reason.
- A waiting deleted transaction disappears from lists at once. *(Author's call: the earlier
  idea of a crossed-out row was dropped because the list carries no marker.)*
- A limit cell shows the new number with a small not-saved icon, because a limit has no
  preview. *(Author's call.)*
- The account balance includes waiting transactions.
- The budget drill-down (`BudgetTransactionsDialog`, `UpcomingPaymentsPanel`, fed by the
  server's per-cell list) follows the budget figures: a waiting add is absent until
  confirmed, and a waiting edit or delete is applied by id to rows already listed. Placing a
  new row in the right cell would mean rebuilding the server's matching on the client.
  *(Author's call.)*
- The sidebar's sync area shows how many changes are waiting.

### Editing something that is still waiting

- Editing a waiting transaction updates the waiting change, so the server gets only the
  final version.
- Deleting a transaction added offline, before it was sent, removes it with nothing sent.
- Setting the same limit (budget, element, period) again keeps only the last value.
- An operation already sent and not yet answered is never altered; the later change queues
  behind it.
- So a target (a transaction id, or a limit's budget, element and period) has at most one
  operation besides one in flight. *(Author's call.)*

### Failure

A change the server refuses stays local, is marked failed and appears in a new Inbox section,
counted in the Inbox badge, with the server's reason. Other waiting changes still go through.

What counts as refused *(author's call)*:

- A 4xx answered with the app's JSON envelope (409 aside) is a refusal.
- A 5xx with the app's envelope is retried a few times with backoff, then fails, so a
  repeatable server bug cannot block the queue. While retrying it steps aside for the
  operations behind it unless they share its target.
- No response, a timeout, or a response that is not the app's envelope (a proxy's error page
  during a deploy) is not a refusal: the change stays waiting, with no limit.

A later change to a failed target replaces the failed operation and makes it waiting again,
whether it came from the Inbox or from a list, so nothing queues behind a failure. Deleting
a failed add drops it with nothing sent. The dialog always shows the latest values.
*(Author's call.)*

- Failed add or edit: tapping opens the normal transaction dialog with the user's values;
  saving sends it again. The user can also discard it, which drops the local change.
- Failed delete: tapping offers delete again or discard (the row returns).
- Failed limit: tapping sends it again, or discard (the cell returns to the server's value).

A network failure is never a failure in this sense: the change stays waiting.

### Disabled while offline

- Every write other than the goals above is visibly disabled.
- In the transaction dialog, existing categories, recipients and tags can be chosen, but
  creating a new one from the picker or the "+" is off.
- Drag reorder of transactions and "make recurring" are off.
- Posting a due recurring payment is off.
- Actions on a waiting transaction that need it on the server (make recurring, drag) are off
  even when online, until it is confirmed.

### Signing out

If changes are waiting, sign-out warns first, because signing out discards them.

## Technical decisions

### Server

Decided by the owner: one universal `Idempotency-Key` header, a UUIDv7 per operation, kept in
the database; the frontend assigns no record ids.

- The app sends `Idempotency-Key` (UUIDv7, the generator already in use) on every
  authenticated write. It replaces the `X-Request-Id` the SPA sends today, which the server
  never read: the header leaves `web/src/api/client.ts` and the CORS allow-list gains
  `Idempotency-Key`. The server still mints its own `X-Request-Id` for logs and the response.
- The API client mints a key when a request has none; the queue stores one per operation and
  reuses it on every attempt. This key is the only id the frontend creates, because only the
  sender can tell a retry from a new request.
- One shared server check covers every authenticated `POST`. Public routes (login, sign-up)
  have no user and are left out. Absent or malformed key means no guard, so MCP, older clients
  and the parity catalogues are unchanged.
- New table, on both engines, keyed by (user id, key): `created_at` and a nullable resource
  id. The migration's version must be above every released one.
- The check reads the table **before** the handler, so a replay is refused ahead of any lookup
  or access check (otherwise the replay of a delete that already landed would answer "not
  found" and show as a failure).
- The key is written inside the request's own database transaction, through the
  `TxManager.BeforeCommit` hook the revision bumps already use, so it and the write commit
  together. A handler that opens no transaction has the key recorded right after a 2xx, a
  slightly weaker guarantee. Any non-2xx releases the key so the operation can be sent again.
- A concurrent duplicate loses on the primary key and gets a retryable 5xx with the envelope,
  not 409, since the first attempt may still fail.
- A replay answers **HTTP 409** with the handled-error envelope, the `common.operation_locked`
  message and an additive `resourceId` field when one was stored. Non-replays keep their
  existing statuses.
- Rows older than 7 days are purged opportunistically (at most every 10 minutes per process,
  set-based, best effort, like the other housekeeping). A retry only ever needs a key from a
  request the server received, so that is long enough.

**Transaction ids stay server-minted.** `id` on `create-transaction` becomes optional and the
SPA stops sending it. When present (older clients, MCP) it keeps today's body-id guard and
today's 400. The create handler stores the new transaction's id as the key's resource id, so
a lost response can still be resolved through the 409. Other create endpoints keep their
`id` for now.

Multi-cell fill sends one `set-limit` per cell, each with its own key.

### Client

- One queue, the only sender of these writes, persisted in a new IndexedDB store in the
  `econumo` database and tagged with the user id.
- Sent one at a time, oldest first, with backoff on network failures. Starts on load and
  on reconnect. Only one tab may send at a time.
- **Overlay, not cache mutation.** The caches keep holding server rows; the waiting changes
  are applied on top when read (`select` on the transactions, accounts, budget and plan
  queries, or an equivalent). A sync or a restore therefore cannot erase them. The revision
  sync's rules (`noteOwnWrite`, `RELOAD_REVISION`, the epoch fence) stay intact.
- A transaction added offline has a temporary id that exists only on the device and is never
  sent. Queued changes to it wait for the add's answer, then use the real id.
- On success the response is applied as today (`useApplyTransactionItem`) and the temporary
  id is swapped for the server's. On 409 the operation is done: its `resourceId` resolves a
  temporary id, and the queue refreshes from the server. A 409 with no resource id for an add
  (the rare crash between commit and storing it, on a handler with no transaction) sends any
  change queued behind it to the Inbox.
- The 401 path pauses the queue. `clearPersistedQueryCache` runs on every login, and must
  keep the queue when the same user signs back in, and clear it for a different user.
- A 402 (read-only access) is a failure like any other refusal.
- Mutations outside the queue use `networkMode: 'always'` with no retry, so anything the
  disabling sweep misses fails at once with a toast instead of freezing.
- Disabling goes through one shared online hook built on `onlineManager`.
- Analytics: the existing create/update/delete/limit events fire when the server confirms.
  New `METRICS` keys for queued, retried and discarded.

### Docs and catalogues

New strings in all 11 locales. Update `docs/regression-test-plan.md` and the frontend and
API sections of `CLAUDE.md`.

### Older servers

A client on another origin than an older server cannot reach it at all: that server's CORS
allow-list does not carry `Idempotency-Key`, so the preflight of every `POST` fails, login
included. The same-origin web app is unaffected, since it always talks to the server that
served it. Only the native app can point at an older self-hosted backend, and it is parked, so
`minServerVersion` is not raised. *(Author's call.)*

## Left to the builder

File layout, the overlay's exact mechanics, backoff timings, the Inbox section's placement
and wording, the balance arithmetic for cross-currency transfers (use the entered recipient
amount), one-tab locking, and how the disabled controls are found and listed.
