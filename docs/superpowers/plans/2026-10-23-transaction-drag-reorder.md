# Transaction drag-to-reorder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user drag a transaction within its day (and into another day) on the account page, stored as explicit per-account-side sort keys.

**Architecture:** Two new columns on `transactions` (`sort_key`, `recipient_sort_key`) order rows within a day per account side. A new `internal/infra/txorder` helper computes "top of day" keys and is used by every insert path (transaction repo, account correction insert, test fixture). A new `move-transaction` endpoint rewrites one account side's keys for a day (and optionally the date). The SPA sorts by day + side key + id and adds dnd-kit drag on the account history list.

**Tech Stack:** Go (sqlc, SQLite + PostgreSQL), React 19, TanStack Query, dnd-kit, vitest.

**Spec:** `docs/superpowers/specs/2026-10-23-transaction-drag-reorder-design.md`

## Global Constraints

- Order everywhere: day (date part of `spent_at`) DESC, side key DESC, `id` ASC. Higher key = higher in the list.
- The time of day is never rewritten by ordering; a cross-day move changes only the date part.
- Key assignment lives in the storage layer (repos + `txorder`), never in use cases.
- Day on SQLite = `substr(spent_at, 1, 10)`; on PostgreSQL = `CAST(spent_at AS date)`; Go side = `spentAt.Format("2006-01-02")` with bounds `[day 00:00:00, day+1 00:00:00)` bound as `datetime.Layout` strings.
- `.sql` files ASCII-only; regenerate with `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`).
- Go toolchain: `/usr/local/go/bin/go` with `GOTOOLCHAIN=go1.27.1`.
- Every coded error gets an `errors.*` key in all 11 `locales/*.json`, registered in `internal/shared/errs/codes.go`.
- Every new user-facing action fires a `METRICS` event.
- Wire fields are additive: `sortKey` (number), `recipientSortKey` (number | null).

## Review Focus

- A CSV import of a day with many rows at `00:00:00` keeps file order top-to-bottom (Task 2 test `TestImportKeepsFileOrderWithinDay`).
- Editing only the description/amount of a transaction keeps its place in the day (Task 2 test `TestSaveKeepsKeysOnPlainEdit`).
- A transfer reordered on its recipient account does not move on its source account (Task 4 test `TestMoveTransferRecipientSideOnly`).
- A stale order (a row added on another device) is refused, not half-applied (Task 4 test `TestMoveRejectsStaleDay`).
- A quick tap on a phone still opens the preview and does not start a drag (Task 7 test `tap opens preview without dragging`).

---

### Task 1: Columns, migration backfill, model and plumbing

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261024000000.sql`, `internal/infra/storage/migrations/pgsql/20261024000000.sql`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/transactions.sql` (every SELECT column list, `UpsertTransaction` insert + `DO UPDATE SET`), regenerate `gen/{sqlite,pgsql}`
- Modify: `internal/model/transaction.go` (fields on `Transaction` + `NewState`, copied in `New`/`FromState`)
- Modify: `internal/transaction/repo/repo.go` (`hydrate`, `write`, `ListByAccountIDs` cols/Scan)
- Test: `internal/infra/storage/migrations/sortkey_backfill_test.go` (or the package that already tests migrations), `internal/transaction/repo/repo_integration_test.go`

**Interfaces:**
- Produces: `model.Transaction.SortKey int64`, `model.Transaction.RecipientSortKey *int64` (same on `NewState`); columns `sort_key INTEGER NOT NULL DEFAULT 0`, `recipient_sort_key INTEGER NULL`.

- [ ] **Step 1: Failing backfill test** `TestSortKeyBackfillKeepsOrder`: on a DB migrated to the previous version, insert for account A on 2026-10-01: r1 `10:00:00`, r2 `10:00:00` (r2 id > r1 id), r3 `09:00:00`; a transfer A→B on that day at `08:00:00`; run the new migration. Assert A's keys: r1=4, r2=3, r3=2, transfer `sort_key`=1; transfer `recipient_sort_key`=1 (only row on B); non-transfers have NULL `recipient_sort_key`; every account's `transactions_revision` moved by 1. Run on sqlite (and pgsql under `-tags enginecompare` / `DBTEST_ENGINE=pgsql`).
- [ ] **Step 2: Run it, expect FAIL** (no columns).
- [ ] **Step 3: Write the migrations.** ALTER TABLE add both columns; `UPDATE transactions SET sort_key = r.n FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY account_id, <day> ORDER BY spent_at, id DESC) AS n FROM transactions) r WHERE transactions.id = r.id;` then the same for `recipient_sort_key` partitioned by `account_recipient_id`, `WHERE account_recipient_id IS NOT NULL` inside the subquery; `UPDATE accounts SET transactions_revision = transactions_revision + 1;`.
- [ ] **Step 4: sqlc** — add `sort_key, recipient_sort_key` to every transactions SELECT and to `UpsertTransaction` (insert values and `DO UPDATE SET sort_key = excluded.sort_key, recipient_sort_key = excluded.recipient_sort_key`); `sqlc generate`. Thread through `hydrate`/`write`/`ListByAccountIDs`; `pgsqlQuerier` conversions keep compiling.
- [ ] **Step 5: Run tests, expect PASS**; `go build ./... && go vet ./...`.
- [ ] **Step 6: Commit** `feat: add per-side transaction sort keys`.

### Task 2: Assign keys on every write

**Files:**
- Create: `internal/infra/txorder/txorder.go`, `internal/infra/txorder/txorder_test.go`
- Modify: `internal/transaction/repo/repo.go` (`write` assigns), `internal/model/transaction.go` (`Update` resets keys), `internal/account/repo/repo.go` + `query/{sqlite,pgsql}/account_corrections.sql` (`sort_key` param), `internal/test/fixture/entities.go` (insert computes keys), `internal/transaction/import.go` (insert order)
- Test: `internal/transaction/repo/repo_integration_test.go`, `internal/model/transaction_test.go`, `internal/transaction/import_test.go`, `internal/account/repo` tests

**Interfaces:**
- Produces: `txorder.New(driver string, tm *backend.TxManager) *Placer`; `(*Placer).Top(ctx, accountID string, spentAt time.Time) (int64, error)` = 1 + max of `sort_key` where `account_id = accountID` and of `recipient_sort_key` where `account_recipient_id = accountID`, both within the day of `spentAt`; 1 for an empty day.
- Produces: in `write`, when `t.SortKey == 0` → `t.SortKey = Top(t.AccountID, t.SpentAt)`; when transfer and `t.RecipientSortKey == nil` → `Top(*t.AccountRecipID, t.SpentAt)` (computed after the source key so a self-referencing day sees it); non-transfer → `RecipientSortKey = nil`.
- Produces: `model.Transaction.Update` sets `SortKey = 0` when the source account or the day of `SpentAt` changes, and `RecipientSortKey = nil` when the recipient account or the day changes or the row stops being a transfer.

- [ ] **Step 1: Failing tests.**
  - `TestPlacerTop` (txorder): empty day → 1; with source keys {1,3} and a recipient key 5 on the same account/day → 6; other days/accounts ignored.
  - `TestSaveNewGoesToTopOfDay` (repo): two new rows same account/day → keys 1 then 2; transfer gets both keys.
  - `TestSaveKeepsKeysOnPlainEdit`: reload, change description, Save → keys unchanged.
  - `TestUpdateResetsKeysOnDayOrAccountChange` (model): day change resets both; recipient change resets only recipient; same-day time change keeps both.
  - `TestCorrectionGoesToTopOfDay` (account repo): `SaveCorrection` on a day with key 2 → 3.
  - `TestImportKeepsFileOrderWithinDay` (transaction import): CSV rows a, b, c with the same date and no time → listed a (highest key), b, c.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** `txorder` (hand-built SQL like `txrevision`, `rebind` for pgsql); wire a `*txorder.Placer` into the transaction repo and account repo constructors; correction SQL takes `sort_key` as a parameter set from `Top`; fixture insert uses `(SELECT COALESCE(MAX(k),0)+1 FROM (...))` subqueries for both sides; CSV importer saves a batch's built transactions in reverse file order (row numbers in the result unchanged, sort each error's row list).
- [ ] **Step 4: Run** `go test ./internal/transaction/... ./internal/account/... ./internal/infra/txorder/ ./internal/model/`, expect PASS.
- [ ] **Step 5: Commit** `feat: place new and moved transactions at the top of their day`.

### Task 3: Order by keys, wire fields, guards

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/transactions.sql` (`ListTransactionsByAccount` ORDER BY), `internal/transaction/repo/repo.go` (`ListByAccountIDs` ORDER BY per driver), `internal/model/transaction_dto.go` (`TransactionResult.SortKey int64 json:"sortKey"`, `RecipientSortKey *int64 json:"recipientSortKey"`; same on the account-transaction-list item type), `internal/transaction/usecase.go` (`buildResult`)
- Modify: `internal/test/revisionguard/guard.go` (fingerprint includes both keys; new invariant check in the "after" phase)
- Regenerate: `internal/test/apiparity/testdata/golden`, `internal/test/mcpparity/testdata/golden`, swagger (`make swagger`)

**Interfaces:**
- Produces: server ORDER BY `<day> DESC, sort_key DESC, id` (single-account query: `CASE WHEN account_id = ? THEN sort_key ELSE recipient_sort_key END DESC` as the key).
- Produces: `revisionguard` fails when any row has `sort_key <= 0`, a transfer has NULL `recipient_sort_key`, a non-transfer has a non-NULL one, or two rows share a key on the same account side + day.

- [ ] **Step 1: Failing tests** — `TestListByAccountOrdersByDayThenKey` (repo): same-day rows with keys inverted against their times come back key-ordered; a recipient-side query uses `recipient_sort_key`. Add the invariant to revisionguard and run it once to confirm it passes on current data (it guards future paths).
- [ ] **Step 2: Implement** ORDER BY + DTO fields + fingerprint columns.
- [ ] **Step 3: Regenerate goldens** (`UPDATE_GOLDEN=1` for apiparity and mcpparity), inspect diff: only the two new fields and order changes where keys disagree with times.
- [ ] **Step 4: Run** `make go-test`, expect PASS.
- [ ] **Step 5: Commit** `feat: order transactions by day and sort key`.

### Task 4: move-transaction endpoint

**Files:**
- Create: `internal/transaction/move.go`, `internal/transaction/move_test.go`
- Modify: `internal/model/transaction_dto.go` (`MoveTransactionRequest`, `MoveTransactionResult`), `internal/transaction/repository.go` + repo (`ListDayIDs`, `SetSideKeys`), `internal/transaction/api/{handlers,routes}.go`, `internal/shared/errs/codes.go`, `locales/*.json` (11), apiparity scenario catalogue, revisionguard `scenarios.go`
- Test: `internal/transaction/move_test.go`

**Interfaces:**
- Produces: route `POST /api/v1/transaction/move-transaction`.
- Produces: `model.MoveTransactionRequest{Id string "id"; AccountId string "accountId"; Date string "date" (Y-m-d); TransactionIds []string "transactionIds"}`; `Validate()` → `common.is_blank` for blank id/accountId/date, empty list.
- Produces: `model.MoveTransactionResult{Items []TransactionResult "items"; Accounts []AccountResult "accounts"}` — every row whose key or date changed.
- Produces: `Repository.ListDayIDs(ctx, accountID vo.Id, day time.Time) ([]vo.Id, error)`; `Repository.SetSideKeys(ctx, accountID vo.Id, orderedTopFirst []vo.Id) (changed []vo.Id, error)` — writes N..1 to `sort_key` where `account_id = accountID`, else `recipient_sort_key`; only rows whose value changes are written; bumps via `BumpWhere("id = ?")` per written row.
- Produces: error codes `transaction.day_changed` (EN "This day changed meanwhile. Refresh and try again.") and `transaction.cannot_move` (EN "This transaction can't be moved to another day.").
- Produces: `Service.MoveTransaction(ctx, userID vo.Id, req model.MoveTransactionRequest) (*model.MoveTransactionResult, error)`.

Algorithm, in one `WithTx`: check write access to `accountId`; load the moved row; it must be on `accountId` (either side) else `transaction.transaction.not_available`; if the date differs: `checkWriteAccess(t.AccountID)` (else `transaction.cannot_move`), set `SpentAt` = new date + same clock time, `t.Update(...)`-equivalent key reset, `repo.Save`, then `zeroer.ZeroDeleted` for every party at the new `SpentAt` (`correctionEditedTx`); then compare the set of `req.TransactionIds` (no duplicates) with `ListDayIDs(accountId, day)` → mismatch = `transaction.day_changed`; `SetSideKeys`. Return the changed rows via `toResult` plus `accountListEmbed`.

- [ ] **Step 1: Failing tests:** `TestMoveReordersWithinDay`, `TestMoveToAnotherDayKeepsTime`, `TestMoveTransferRecipientSideOnly` (source-side keys untouched), `TestMoveRejectsStaleDay` (nothing written), `TestMoveRequiresWriteAccess`, `TestMoveCrossDayNeedsSourceWriteAccess`, `TestMoveCrossDayRezeroesDeletedAccount`, `TestMoveBumpsRevisionOnce`.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** service, repo methods, handler (`endpoint.Handle`, swag block), route, codes, 11 catalogue entries (translate properly).
- [ ] **Step 4: apiparity scenario** `move-transaction` (reorder + cross-day + stale), golden regenerated and inspected; revisionguard guard scenario with `Want` on the move call.
- [ ] **Step 5: Run** `make go-test`, expect PASS.
- [ ] **Step 6: Commit** `feat: move-transaction endpoint`.

### Task 5: Client ordering by keys

**Files:**
- Create: `web/src/features/transactions/order.ts`, `web/src/features/transactions/order.test.ts`
- Modify: `web/src/api/dto/transaction.ts` (`TransactionDto.sortKey?: number`, `recipientSortKey?: number | null`), `useAccountTransactions.ts`, `queries.ts` (`byDateDesc`), `web/src/features/search/useGlobalSearch.ts`, any other `date`-comparing sort of transactions (grep `\.date <`)

**Interfaces:**
- Produces: `sideKey(tx, accountId?: Id): number | undefined`; `compareTransactions(a, b, accountId?: Id): number` — day DESC (`date.slice(0,10)`), then side key DESC when both defined, else full `date` DESC, then `id` ASC.

- [ ] **Step 1: Failing tests** in `order.test.ts`: key beats time within a day; day beats key; recipient side picked for `accountRecipientId === accountId`; missing keys fall back to datetime; id tie-break.
- [ ] **Step 2: Implement + switch callers** (the pinned-recurring rule in `useAccountTransactions` stays first within today).
- [ ] **Step 3: Run** `cd web && pnpm test -- order useAccountTransactions queries`, expect PASS.
- [ ] **Step 4: Commit** `feat(web): sort transactions by day and sort key`.

### Task 6: Move mutation

**Files:**
- Modify: `web/src/api/transaction.ts` (`moveTransaction(req: MoveTransactionDto): Promise<MoveTransactionResultDto>`), `web/src/api/dto/transaction.ts`, `web/src/features/transactions/queries.ts` (`useMoveTransaction`), `web/src/lib/metrics.ts` (`TRANSACTION_MOVE: 'appTransactionMove'`)
- Create: `web/src/features/transactions/move.ts` (`applyLocalMove(rows, req, accountId): TransactionDto[]`), tests

**Interfaces:**
- Produces: `MoveTransactionDto { id: Id; accountId: Id; date: string; transactionIds: Id[] }`, `MoveTransactionResultDto { items: TransactionDto[]; accounts: AccountDto[] }`.
- Produces: `useMoveTransaction()` — `onMutate`: `patchTransactions` with `applyLocalMove` (assign N..1 to the side keys of the listed ids, change the moved row's date part), keep the previous list for rollback; `onSuccess`: `noteOwnWrite(result.accounts, every account of every item)`, set accounts, replace items by id, invalidate budget queries, `trackEvent(METRICS.TRANSACTION_MOVE, { dayChanged })`; `onError`: restore the previous list and toast `apiErrorMessage`.

- [ ] **Step 1: Failing tests:** `applyLocalMove` reorders and re-dates; hook rolls back on error and fires the metric once on success.
- [ ] **Step 2: Implement.**
- [ ] **Step 3: Run** `pnpm test -- move queries metrics-coverage`, expect PASS.
- [ ] **Step 4: Commit** `feat(web): move transaction mutation`.

### Task 7: Drag on the account page

**Files:**
- Create: `web/src/features/accounts/historyDrag.ts` (pure drop resolution), `historyDrag.test.ts`, `web/src/features/accounts/useHistoryDrag.tsx` (dnd-kit wiring)
- Modify: `web/src/features/accounts/AccountPage.tsx`, `web/src/features/accounts/TransactionHistory.tsx`, `locales/*.json` (`accounts.page.transaction_list.drag_handle` aria label)
- Test: `AccountPage.test.tsx`

**Interfaces:**
- Produces: `resolveDrop(entries: DailyListEntry[], activeId: Id, overIndex: number): { date: string; transactionIds: Id[] } | null` — removes the active row, inserts it at `overIndex` in the flat list, returns the day group it lands in (the nearest separator above) and that group's real-transaction ids in order; null when it lands in the pinned-recurring part of today or the result equals the current order.
- Produces: `isDraggable(tx, { canWrite, search })`: false for `recurring` rows, when `search` is non-empty, or `!canWrite`; cross-day allowed only when `canTouchTransaction`.

Behavior: desktop — `GripVertical` button left of the row, `opacity-0 group-hover:opacity-100 focus-visible:opacity-100`, `PointerSensor` activated from that handle only (`setActivatorNodeRef`), plus `KeyboardSensor`. Touch (`useIsCompact`) — `TouchSensor` `{ delay: 500, tolerance: 8 }` on the whole row, `-webkit-touch-callout: none; user-select: none` on rows, `navigator.vibrate?.(10)` on start, suppress the click that follows a drag. `DragOverlay` shows the lifted row; drop indicator line; the target day separator gets `bg-accent` while the drop would change the day. Auto-scroll via dnd-kit `autoScroll` on the history container; `TransactionHistory` keeps `range` from shrinking during a drag.

- [ ] **Step 1: Failing tests:** `resolveDrop` within day, across day (top/bottom of a group), no-op drop, pinned-recurring region; AccountPage: `tap opens preview without dragging`, handle hidden for readonly accounts and while searching, no handle on recurring rows.
- [ ] **Step 2: Implement.**
- [ ] **Step 3: Run** `pnpm test -- historyDrag AccountPage TransactionHistory`, `pnpm lint`, `pnpm exec tsc -b`, expect PASS.
- [ ] **Step 4: Browser check** (desktop + 390px touch emulation) with a running server: reorder, cross-day, reload keeps order.
- [ ] **Step 5: Commit** `feat(web): drag to reorder transactions on the account page`.

### Task 8: Docs, full verification, PR

**Files:**
- Modify: `docs/regression-test-plan.md` (Account page items from the spec), `CLAUDE.md` (sort keys under Database/Notable behaviours; `move-transaction` sites in the revision list)

- [ ] **Step 1:** Update the docs.
- [ ] **Step 2:** `make go-test` and `cd web && pnpm test && pnpm lint`; pgsql repo suite (`make test-repo-pgsql` or `DATABASE_TEST_PGSQL_URL`) if a PostgreSQL is reachable.
- [ ] **Step 3:** Commit, push `feature/transaction-drag-reorder`, open a draft PR against `main` (note: iPhone on-device check pending).
