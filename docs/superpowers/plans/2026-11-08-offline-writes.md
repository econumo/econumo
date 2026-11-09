# Offline transactions and limits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add, edit and delete transactions and change limits offline, shown at once, sent automatically later, with refused changes in the Inbox and every other write disabled offline.

**Architecture:** The server gains one universal `Idempotency-Key` check on authenticated POSTs, committed with the write through a request-level transaction hook. The SPA gains a persisted, per-target-folding queue in IndexedDB, applied over the server caches at read time by pure overlay functions, and drained one operation at a time.

**Tech Stack:** Go (stdlib `net/http`, sqlc, SQLite + PostgreSQL), React 19, TanStack Query, IndexedDB, vitest.

**Spec:** `docs/superpowers/specs/2026-11-08-offline-writes-design.md`

## Global Constraints

- Go is at `/usr/local/go/bin` and needs `GOTOOLCHAIN=go1.27.1`.
- `Idempotency-Key` is a UUIDv7 from the `uuid` package's `v7`; absent or unparsable means no guard.
- Replay: HTTP 409, handled-error envelope, message from `common.operation_locked`, additive `resourceId` when stored.
- Key rows purge after 90 days, at most every 10 minutes per process (a device can sit offline for weeks with an attempted add still queued).
- Request timeout for a write: 10 seconds.
- A 5xx with the app's envelope retries 5 times with backoff, then fails.
- Migration version `20261109000000` on both engines (above every released one).
- Every new `t()` key goes in all 11 `locales/<lang>.json` in the same task (`internal/test/i18ntest` fails otherwise). Registers: de Sie, nl u, fr vous, pt European.
- No bold for UI hierarchy. Comment only non-obvious rules. `.sql` files ASCII-only.
- Every new `METRICS` key must be fired (`metrics-coverage.test.ts`).
- Never persist or log a key's request body.

## Review Focus

1. A change was sent, the answer was lost, and the user edits it: the edit must queue behind it, not merge into it, or the server refuses the replay and the edit is lost. (Task 4)
2. A sync or a restart lands while changes wait: nothing waiting may vanish from the lists or balances. (Tasks 5, 7)
3. Another user signs in on a device holding someone's waiting changes: nothing is sent under the new session. (Task 6)
4. A deploy behind a proxy returns HTML 502s during a drain: the queue keeps waiting and fills no Inbox. (Task 4)
5. The app is killed after the server committed an add and before the response was read: reopening resends, gets 409, and shows exactly one transaction. (Tasks 3, 6)

---

### Task 1: Idempotency key storage

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261109000000.sql`, `internal/infra/storage/migrations/pgsql/20261109000000.sql`
- Create: `internal/infra/storage/sqlc/query/sqlite/idempotency.sql`, `internal/infra/storage/sqlc/query/pgsql/idempotency.sql`
- Create: `internal/infra/idempotency/store.go`, `internal/infra/idempotency/store_test.go`
- Regenerate: `internal/infra/storage/sqlc/gen/**`

**Interfaces:**
- Produces, in package `idempotency`: `NewStore(driver string, tx *backend.TxManager, clock port.Clock) *Store`, and on `*Store`: `Find(ctx, userID vo.Id, key string) (resourceID string, found bool, err error)`, `Insert(ctx, userID vo.Id, key, resourceID string) error`, `Delete(ctx, userID vo.Id, key string) error`, `MaybePurge(ctx)`.

- [ ] **Step 1: Write the failing tests** in `store_test.go` with `dbtest.New(t)`: `TestInsertThenFind` (resource id round-trips, and an empty resource stores NULL and reads `""`), `TestFindIsPerUser` (same key under another user is not found), `TestInsertDuplicateErrors`, `TestDelete`, `TestMaybePurgeRemovesOnlyOlderThanSevenDays` (fake clock; a second call within 10 minutes deletes nothing).
- [ ] **Step 2: Run** `go test ./internal/infra/idempotency/` — expected: build failure.
- [ ] **Step 3: Add the migration.** Table `idempotency_keys`: `user_id` TEXT/UUID as other id columns, `idempotency_key` TEXT, `resource_id` nullable, `created_at` datetime per the engine's convention, `PRIMARY KEY (user_id, idempotency_key)`, index on `created_at`. No foreign key.
- [ ] **Step 4: Add queries** `GetIdempotencyKey`, `InsertIdempotencyKey`, `DeleteIdempotencyKey`, `PurgeIdempotencyKeys`, then `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`).
- [ ] **Step 5: Implement `Store`** following `internal/infra/operation/guard.go` (per-engine querier, every query through `TxManager.Querier(ctx)`).
- [ ] **Step 6: Run** `go test ./internal/infra/idempotency/` — expected: PASS. Then `go build ./...`.
- [ ] **Step 7: Commit** `feat: Store idempotency keys`.

### Task 2: Universal Idempotency-Key check

**Files:**
- Modify: `internal/infra/storage/backend/tx*.go` (the `top` path)
- Modify: `internal/shared/reqctx/reqctx.go`
- Create: `internal/web/middleware/idempotency.go`, `internal/web/middleware/idempotency_test.go`
- Modify: `internal/web/middleware/auth.go` (end of `AuthWith`), `internal/web/middleware/middleware.go:178`
- Modify: `internal/web/httpx/envelope.go`, `internal/server/server.go`, `internal/server/glue_language.go` or a new `internal/server/glue_idempotency.go`
- Test: `internal/infra/storage/backend` tests, `internal/server/idempotency_test.go`

**Interfaces:**
- Consumes: Task 1 `Store`.
- Produces:
  - `backend.WithCommitHook(ctx, fn func(ctx context.Context) error) context.Context`: `top` runs `fn(txCtx)` after `fn` of the unit of work and before the `BeforeCommit` hooks. An error rolls back.
  - `reqctx.WithIdempotencyResource(ctx) context.Context`, `reqctx.SetIdempotencyResource(ctx, id string)`, `reqctx.IdempotencyResource(ctx) string` (a mutable holder).
  - `middleware.IdempotencyStore` (Find/Insert/Delete/MaybePurge) and the optional capability `middleware.IdempotencyProvider { IdempotencyStore() IdempotencyStore }` on the authenticator, the same pattern as `StoredLanguageResolver`.
  - `httpx.WriteReplay(ctx, w, resourceID string)`.

- [ ] **Step 1: Write the failing tests** in `internal/server/idempotency_test.go` against `server.BuildAPI` on a real committed SQLite (`dbtest.NewSQLite`, `fixture` for user/account/token):

```go
func TestReplayedCreateIsRefusedAndCreatesOneRow(t *testing.T)   // same key twice: 200 then 409; one transaction; 409 body has resourceId == first item.id
func TestFailedWriteReleasesTheKey(t *testing.T)                 // bad accountId with key K: 400; fixed body with K: 200
func TestReplayedDeleteIsRefusedNotNotFound(t *testing.T)        // delete with K: 200; again with K: 409
func TestReplayedSetLimitIsRefused(t *testing.T)
func TestKeyIsPerUser(t *testing.T)                              // user B reusing A's key: 200
func TestNoKeyAndMalformedKeyAreUnguarded(t *testing.T)          // both create twice with distinct body ids: 200, 200
func TestReadsArePassedThrough(t *testing.T)                     // GET with the same key twice: 200, 200
func TestHandlerWithoutTransactionRecordsAfterSuccess(t *testing.T) // a POST route that opens no tx: 200 then 409
```
  Add `TestCommitHookRunsInsideTheTransaction` and `TestCommitHookErrorRollsBack` in the backend tests.
- [ ] **Step 2: Run** `go test ./internal/server/ -run 'Replay|Key|Reads|Failed|Handler'` — expected: FAIL.
- [ ] **Step 3: Implement `backend.WithCommitHook`.**
- [ ] **Step 4: Implement `serveIdempotent` in `idempotency.go`**, called at the end of `AuthWith` instead of `next.ServeHTTP` when: method POST, path is not `MCPPath`, scope is `full`, the header parses as a UUID, and `authn` implements `IdempotencyProvider`. Sequence:
  1. `Find`; found → `httpx.WriteReplay`, return. A `Find` error logs WARN and proceeds unguarded.
  2. Build ctx with the resource holder and a commit hook that inserts `(user, key, reqctx.IdempotencyResource(ctx))` once, setting a `recorded` flag.
  3. Serve through a status-recording `ResponseWriter`.
  4. 2xx and not recorded → `Insert` (best effort, WARN on error). Non-2xx and recorded → `Delete`. 2xx → `MaybePurge`.

  A primary-key conflict in the hook surfaces as an ordinary 500 exception envelope, which the client retries.
- [ ] **Step 5: Add `httpx.WriteReplay`** with its own struct carrying `ResourceID string json:"resourceId,omitempty"` so `errEnvelope` stays frozen, status 409, message rendered as `WriteError` renders `errs.CodeOperationLocked`.
- [ ] **Step 6: Wire** `idempotency.NewStore` in `server.Build` through the existing authenticator decorator. Add `Idempotency-Key` to `Access-Control-Allow-Headers`, keeping `X-Request-Id` (cached older SPAs still send it).
- [ ] **Step 7: Run** the tests above — expected: PASS. Then `UPDATE_GOLDEN= go test ./internal/test/apiparity/ ./internal/test/mcpparity/ ./internal/test/revisionguard/` — expected: PASS with no golden change.
- [ ] **Step 8: Commit** `feat: Refuse replayed writes by Idempotency-Key`.

### Task 3: Server-minted transaction id on create

**Files:**
- Modify: `internal/model/transaction_dto.go:70-87`, `internal/transaction/create.go:27-65`, and the `@Param`/docs if swag output changes
- Test: `internal/transaction/create_test.go` (or the existing file for create), `internal/server/idempotency_test.go`

**Interfaces:**
- Consumes: `reqctx.SetIdempotencyResource` from Task 2.
- Produces: `POST create-transaction` accepts a body without `id`.

- [ ] **Step 1: Write the failing tests:** `TestCreateWithoutIdMintsAnId` (blank `id` → 200, non-blank `item.id`), `TestCreateWithIdKeepsItsLockedReplay` (same body `id` twice with no header → 200, then 400 `Operation is locked`), `TestReplayReturnsResourceIdForCreateWithoutBodyId` (Review Focus 5: no body id, same key twice → 409 whose `resourceId` equals the first `item.id`, one row).
- [ ] **Step 2: Run** them — expected: FAIL.
- [ ] **Step 3: Implement.** Remove `id` from `Validate`'s blank list. In `createTransaction`, when `strings.TrimSpace(req.Id) == ""` skip parsing it and skip `Claim`/`MarkHandled`. After `repo.Save`, `reqctx.SetIdempotencyResource(ctx, id.String())`.
- [ ] **Step 4: Regenerate and inspect goldens:** `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, then `git diff --stat internal/test/apiparity/testdata`. The only acceptable change is a blank-`id` create scenario. Run `make swagger`.
- [ ] **Step 5: Run** `make go-test` — expected: PASS.
- [ ] **Step 6: Commit** `feat: Let the server mint a created transaction's id`.

### Task 4: Queue rules

**Files:**
- Create: `web/src/features/pendingWrites/types.ts`, `queue.ts`, `queue.test.ts`, `classify.ts`, `classify.test.ts`

**Interfaces:**
- Produces, `types.ts`:

```ts
type OpBase = { key: string; seq: number; attempted: boolean; status: 'waiting' | 'failed'; error?: string; retries: number; notBefore: number }
export type OpChange =
  | { kind: 'tx-create'; txId: Id; payload: CreateTransactionDto }
  | { kind: 'tx-update'; txId: Id; payload: CreateTransactionDto }
  | { kind: 'tx-delete'; txId: Id; previous: TransactionDto | null }
  | { kind: 'limit'; budgetId: Id; elementId: Id; period: string; amount: string | null }
export type PendingOp = OpBase & OpChange
```
- Produces, `queue.ts` (pure): `targetOf(op)`, `addChange(ops, change, mint: { key: string; seq: number }, attempted?: boolean): PendingOp[]`, `markAttempted(ops, key)`, `settleDone(ops, key)`, `settleRefused(ops, key, message)`, `retryFailed(ops, key)`, `discard(ops, key)`, `resolveTempId(ops, tempId, realId)`, `nextSendable(ops, now): PendingOp | null`.
- Produces, `classify.ts`: `classifyWriteError(err: unknown): 'done' | 'refused' | 'retry' | 'wait' | 'pause'`.

- [ ] **Step 1: Write the failing tests.** `queue.test.ts` table of folding (each on an unattempted op unless stated):

| Existing | New change | Result |
|---|---|---|
| create | update | create with the new payload, same key |
| update | update | update with the new payload |
| create | delete | queue empty |
| update | delete | one delete |
| limit | same-target limit | one limit, last amount |
| limit | other period | two ops |
| create, **attempted** | update | two ops, the first untouched (Review Focus 1) |
| create, **attempted** | delete | create and a delete |
| failed update | update | one op, `status: 'waiting'`, `attempted: false`, no `error` |
| failed create | delete | queue empty |

  Plus: `settleRefused` on an attempted create with an update behind folds the update in and is `failed`; on an attempted create with a delete behind removes both; `nextSendable` returns oldest first, skips `failed`, skips `notBefore > now`, and never returns an op with an earlier op on the same target, even when that earlier one is backing off; `resolveTempId` rewrites `txId`, and `payload.id` of later ops.

  `classify.test.ts`: no response → `wait`; timeout → `wait`; 409 with envelope → `done`; 400 and 402 with envelope → `refused`; 429 with envelope → `wait`; 500 with envelope → `retry`; 502 with HTML body → `wait` (Review Focus 4); 404 with HTML → `wait`; 401 → `pause`. The envelope test is `typeof data?.success === 'boolean'`.
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/pendingWrites` — expected: FAIL.
- [ ] **Step 3: Implement both files.**
- [ ] **Step 4: Run** them — expected: PASS.
- [ ] **Step 5: Commit** `feat: Add the rules of the offline write queue`.

### Task 5: Overlays

**Files:**
- Create: `web/src/features/pendingWrites/overlay.ts`, `overlay.test.ts`
- Modify: `web/src/features/budgets/queries.ts:163-210` (move the `useSetLimit` patch into `applyLimitToBudget` and call it from `onMutate`)

**Interfaces:**
- Consumes: Task 4 types.
- Produces: `overlayTransactions(rows, ops, me: UserDto)`, `overlayAccounts(accounts, ops, serverRows)`, `applyLimitToBudget(budget, { elementId, period, amount })`, `overlayBudget(budget, ops, budgetId, period)`, `overlayPlan(plan, ops, budgetId)`, `overlayBudgetTransactions(rows, ops)`, `PENDING_SORT_KEY = '~'`.

- [ ] **Step 1: Write the failing tests:** an added row is present with `author: me`, `recurringId: null`, `isImported: 0`, and sorts above a same-day server row by `compareTransactions`; an update replaces the payload fields and keeps server-owned fields; a date change takes `PENDING_SORT_KEY`; a delete removes; a failed op still overlays; applying to fresh server rows twice gives the same output (Review Focus 2: the overlay never mutates its input). Accounts: expense subtracts, income adds, transfer uses `amount` on the source and `amountRecipient` on the recipient, an update reverses the server row's effect first, a delete reverses it, and a row on an account absent from the list is ignored. Budget and plan: only the matching budget, element and period change; a null amount clears (`budgeted: ''`, `isBudgeted: 0`, plan `planned: ''`). Drill-down: updates and deletes by id; a create is absent.
- [ ] **Step 2: Run** them — expected: FAIL.
- [ ] **Step 3: Implement** using the decimal helpers the budget queries already import.
- [ ] **Step 4: Run** `pnpm vitest run src/features/pendingWrites src/features/budgets` — expected: PASS.
- [ ] **Step 5: Commit** `feat: Show waiting changes over the server data`.

### Task 6: Persistence, sender and direct submit

**Files:**
- Modify: `web/src/lib/db.ts` (`DB_VERSION = 2`, new `pendingWrites` store keyed by `key`, `upgrade` creating by `oldVersion`)
- Create: `web/src/lib/online.ts`, `web/src/features/pendingWrites/store.ts`, `store.test.ts`, `sender.ts`, `sender.test.ts`, `submit.ts`, `submit.test.ts`, `hooks.ts`
- Modify: `web/src/api/client.ts:22`, `web/src/api/transaction.ts`, `web/src/api/budget.ts:129`, `web/src/lib/appQueryClient.ts`, `web/src/features/reports/ReportUpdatedAt.tsx`, `web/src/features/auth/LogoutPage.tsx`, the file rendering the query persist provider (`grep -rl PersistQueryClientProvider web/src/app`), `web/src/lib/metrics.ts`

**Interfaces:**
- Consumes: Tasks 4-5.
- Produces:
  - `useOnline(): boolean` from `lib/online.ts`.
  - `store.ts`: `loadPendingWrites(userId)` (wipes rows of another user), `getOps()`, `subscribe(fn)`, `mutate(fn: (ops) => ops)` (writes through, puts per row, tells other tabs over `BroadcastChannel`), `clearPendingWrites()`. A broken IndexedDB keeps the queue in memory, as `transactionStore` does.
  - `sender.ts`: `startPendingWrites(queryClient)`, `kickPendingWrites()`.
  - `submit.ts`: `submitChange(queryClient, change, send): Promise<{ queued: boolean; result?: T }>`.
  - `hooks.ts`: `usePendingOps()`, `usePendingOpForTransaction(id)`, `usePendingLimit(budgetId, elementId, period)`, `useFailedWrites()`.
  - API: `createTransaction(form, opts?)`, `updateTransaction(form, opts?)`, `deleteTransaction(id, opts?)`, `setLimit(form, opts?)` with `opts: { idempotencyKey?: string; timeout?: number }`. `createTransaction` strips `id` from the body.
  - `METRICS.WRITE_QUEUED`, `WRITE_RETRIED`, `WRITE_DISCARDED`.

- [ ] **Step 1: Write the failing tests.**
  - `store.test.ts` (`fake-indexeddb`, as the existing store tests): ops survive a reload; `loadPendingWrites('B')` over A's rows returns none and deletes them (Review Focus 3); `clearPendingWrites` empties.
  - `sender.test.ts`: sends oldest first with the stored key as `Idempotency-Key`; `markAttempted` is persisted before the request; success applies the response and removes the op; 409 with `resourceId` resolves the temp id on the op behind it and invalidates transactions and accounts, leaving one row (Review Focus 5); a refusal becomes `failed` and the next op still goes; network failure stops the drain and keeps everything; 5xx sets backoff and `retries`, and fails after 5; 401 pauses; nothing is sent while the session's user differs from the queue's; `navigator.locks` holder only.
  - `submit.test.ts`: offline queues without a request and resolves at once; online success returns the result and queues nothing; online refusal rejects and queues nothing; online timeout queues with `attempted: true`; a waiting change on the same target queues without a request.
  - `client.test.ts`: `Idempotency-Key` present on POST as a UUIDv7, kept when already set, absent on GET; `X-Request-Id` no longer sent.
- [ ] **Step 2: Run** them — expected: FAIL.
- [ ] **Step 3: Implement.** Mutation defaults in `createAppQueryClient`: `mutations: { networkMode: 'always', retry: false }`. Backoff: network 2 s doubling to 60 s; kicks on `onlineManager` reconnect and on app start. Temporary row ids are the form's existing `uuidv7()`. `startPendingWrites` runs after the persisted restore, with `sessionUserId`. `LogoutPage` calls `clearPendingWrites()` beside `clearPersistedQueryCache()`; `clearPersistedQueryCache` itself does not, so a same-user re-login keeps the queue. Existing confirm metrics fire at server confirmation from one place in `sender.ts`/`submit.ts`.
- [ ] **Step 4: Run** the tests, then `pnpm vitest run` — expected: PASS.
- [ ] **Step 5: Commit** `feat: Persist and send waiting changes`.

### Task 7: Transactions through the queue

**Files:**
- Modify: `web/src/features/transactions/queries.ts`, `web/src/features/accounts/queries.ts:14-27`, `web/src/features/transactions/TransactionDialog.tsx:210-235`, `web/src/features/transactions/ViewTransactionDialog.tsx`, the callers' error handling where a queued delete resolves
- Test: `web/src/features/transactions/queries.test.tsx`, `TransactionDialog.test.tsx`, `ViewTransactionDialog.test.tsx`, `useAccountTransactions.test.tsx`

**Interfaces:**
- Consumes: Task 6 `submitChange`, `usePendingOps`, `overlayTransactions`, `overlayAccounts`.
- Produces: a plain `applyTransactionItem(queryClient, result, mode)` (extracted from `useApplyTransactionItem`, used by the sender). Hook signatures unchanged, so the five delete callers need no edit.

- [ ] **Step 1: Write the failing tests:** offline Add closes the dialog with no request and the row is in `useTransactions()`; the account balance moved; a sync landing rows from the server keeps the waiting row (Review Focus 2); the preview of a waiting row shows `t('transactions.not_saved')`, a failed one shows its reason, and a server row shows neither; the list row renders no marker; an online validation error still shows in the dialog; editing a waiting row offline leaves one op.
- [ ] **Step 2: Run** them — expected: FAIL.
- [ ] **Step 3: Implement.** Hooks call `submitChange`; `select` closures depend on the ops from `usePendingOps`. On `useDeleteTransaction`, read `previous` from the cache by id. In the dialog the existing `mutateAsync` branches stay; the `postRecurring` and `importQueued` branches keep their network path. Pass `onMakeRecurring` as undefined for a waiting row. Add `transactions.not_saved` in all locales.
- [ ] **Step 4: Run** `pnpm vitest run src/features/transactions src/features/accounts` — expected: PASS.
- [ ] **Step 5: Commit** `feat: Add, edit and delete transactions offline`.

### Task 8: Limits through the queue

**Files:**
- Modify: `web/src/features/budgets/queries.ts` (`useBudget`, `useSetLimit`, `useBudgetPlan`, `usePlanSetLimit`, `useFillPlannedCells`, `useBudgetTransactions`), `PlanSheet.tsx`, `PhoneMonthView.tsx`, the desktop Budget cell in `BudgetPage.tsx`'s table component
- Create: `web/src/features/pendingWrites/NotSavedIcon.tsx`
- Test: `web/src/features/budgets/queries` tests, `PlanSheet` tests

**Interfaces:**
- Consumes: Task 6 `submitChange`, `usePendingLimit`; Task 5 overlays.
- Produces: `<NotSavedIcon />` with `aria-label={t('budgets.limit.not_saved')}`.

- [ ] **Step 1: Write the failing tests:** an offline `useSetLimit` shows the new `budgeted` from `useBudget` with no request and survives a refetch; `usePlanSetLimit` likewise; fill-right queues one op per target with distinct keys; a waiting cell renders the icon and others do not; after the op settles the budget and plan queries are invalidated.
- [ ] **Step 2: Run** them — expected: FAIL.
- [ ] **Step 3: Implement.** `onError` rollback applies only to a real refusal. The sender invalidates `[...queryKeys.budget, budgetId]` and `[...queryKeys.budgetPlan, budgetId]` when a limit op settles.
- [ ] **Step 4: Run** `pnpm vitest run src/features/budgets` — expected: PASS.
- [ ] **Step 5: Commit** `feat: Change limits offline`.

### Task 9: Inbox, waiting count and sign-out warning

**Files:**
- Create: `web/src/features/inbox/FailedWritesSection.tsx`, `FailedWritesSection.test.tsx`
- Modify: `web/src/features/inbox/useInbox.ts`, `InboxPage.tsx`, `web/src/app/layouts/ApplicationLayout.tsx`, `web/src/features/settings/SettingsPage.tsx:229`, `ProfilePage.tsx:287`, `web/src/features/auth/LogoutPage.tsx`
- Test: `useInbox.test.tsx`, `InboxPage.test.tsx`

**Interfaces:**
- Consumes: `useFailedWrites`, `retryFailed`, `discard`, `useUiStore.openTransactionModal` (edit params).
- Produces: `Inbox.failedWrites: PendingOp[]`, included in `count`. Section first in the page (author's call: the user's own data is at risk).

- [ ] **Step 1: Write the failing tests:** the failed op is listed with its reason and counted; tapping a failed add or edit opens the transaction dialog with the op's values, and Save makes it waiting; a failed delete and a failed limit offer Try again and Discard; discard removes it and fires `WRITE_DISCARDED`; retry fires `WRITE_RETRIED`; the sign-out confirm shows the warning only when ops wait; the sync control shows the count online and offline (the no-connection icon too).
- [ ] **Step 2: Run** them — expected: FAIL.
- [ ] **Step 3: Implement** with `inbox.sections.failed_writes`, `inbox.failed_writes.*` and `settings.logout.pending_warning` in all locales.
- [ ] **Step 4: Run** `pnpm vitest run src/features/inbox src/features/settings src/app` — expected: PASS.
- [ ] **Step 5: Commit** `feat: Surface refused changes in the Inbox`.

### Task 10: Disable everything else offline

**Files:**
- Create: `web/src/lib/offline-coverage.test.ts`
- Modify: every non-test `.tsx` under `web/src` that calls `.mutate(` or `.mutateAsync(` on a write that is not queued: accounts, budgets (structure, menus, comments, settings dialogs), classifications, connections, currencies, imports, inbox (`DueRecurringSection`, `SharingSection`, `QueueSections`), recurring, settings, search (non-delete actions), feedback, avatar picker
- Modify: `web/src/features/transactions/TransactionDialog.tsx` (`onCreate` undefined offline), `ClassificationChips.tsx` ("+"), `web/src/features/accounts/AccountPage.tsx` (drag), `ViewTransactionDialog.tsx` (make recurring)

**Interfaces:**
- Consumes: `useOnline`.

- [ ] **Step 1: Write the failing guard test.** `offline-coverage.test.ts`, styled on `metrics-coverage.test.ts`: every matching source file must reference `useOnline` unless listed in `OFFLINE_EXEMPT`, with a reason per entry. Exempt: the signed-out auth flows (`LoginPage`, `RegistrationPage`, `RecoveryDialog`, `VerifyEmailDialog`, `OAuthCallbackPage`, `ProviderButtons`) and `WhatsNewNotice` (dismissal is best effort).
- [ ] **Step 2: Run** `pnpm vitest run src/lib/offline-coverage` — expected: FAIL listing the files.
- [ ] **Step 3: Sweep file by file.** Buttons get `disabled={!online}`; menu items get the menu's disabled flag; pickers lose `onCreate`; drag handles set `disabled`. Add one behaviour test per feature group asserting the main control is disabled with `onlineManager.setOnline(false)`, plus `TransactionDialog` offline has no create row and no "+".
- [ ] **Step 4: Run** `pnpm vitest run` and `make web-lint` — expected: PASS.
- [ ] **Step 5: Commit** `feat: Disable writes that need a connection`.

### Task 11: Docs and full verification

**Files:**
- Modify: `docs/regression-test-plan.md`, `CLAUDE.md` (API conventions: `Idempotency-Key`; frontend: pending writes; database: the table)

- [ ] **Step 1: Update docs.** Regression items with 📱 for: offline add, edit, delete; offline limit; reopening with waiting changes; refused change in the Inbox; disabled controls; sign-out warning.
- [ ] **Step 2: Run** `make test` — expected: all suites PASS, including PostgreSQL and upgrade.
- [ ] **Step 3: Run the real app** (`go run ./cmd/econumo serve` with a seeded user, built SPA) in Playwright with `context.setOffline(true)` at phone size: add → dialog closes, row and balance move, preview says Not saved, picker has no create; change a limit → icon; reload offline → all still there; `setOffline(false)` → requests go out, marker gone, one row. Force a refusal (delete the account online from a second session) → Inbox item, tap, save again.
- [ ] **Step 4: Commit** `docs: Describe offline writes`, push, open a draft PR mentioning `@dmtrkzntsv`, and run the `agent-review` skill. Mark ready after fixes and green CI.
