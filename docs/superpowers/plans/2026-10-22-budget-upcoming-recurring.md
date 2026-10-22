# Upcoming Recurring Payments in Budget and Plan — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recurring templates on a budget's member accounts count, as upcoming amounts, in every month they fall in (Budget monthly view, Plan grid), and the cell's transaction list shows them as upcoming rows with post/skip/edit on the next payment.

**Architecture:** The server projects templates into in-memory occurrences and feeds them to the EXISTING builders through a second `ReadModel` implementation (`projectedRead`), so occurrences are filed by exactly the same code as real transactions. Each builder runs twice (real read, projected read) and the projected result is merged into new additive `upcoming*` fields. The SPA folds the upcoming fields into spent/actual at its query layer (one choke point), so downstream math and rendering keep working on one combined number.

**Tech Stack:** Go (internal/budget, internal/model, internal/server glue), sqlite/pgsql parity suites, React 19 + TanStack Query + vitest.

**Spec:** `docs/superpowers/specs/2026-10-22-budget-upcoming-recurring-design.md`

## Global Constraints

- Frozen fields keep their meaning: `spent`, `budgetSpent`, `available`, `actual`, `closingBalance` on the PLAN, `items` of the transaction list carry real money only. Everything new is additive and always present (`"0"` / `[]` when empty), except `closingBalance` on `get-budget` savings rows, which the spec extends (see Task 4).
- Features never import features: budget reads templates through a port in `internal/budget/ports.go`, implemented in `internal/server/glue_budget_recurring.go`.
- Projection window: from `nextPaymentAt` up to the end of the request's last month. If `nextPaymentAt` is before today (caller timezone, `reqctx.Location`) it is ONE overdue occurrence filed in the caller's current month (`localMonth(now, tz)`); further occurrences dated before today are dropped; expansion continues from today on with `model.NextOccurrence` (keeps `ScheduledDay`).
- An occurrence's month bucket is computed in Go (never SQL), so sqlite and pgsql agree.
- Posting from the Budget/Plan list: next payment dated after today → post with `date` = its scheduled date; due today or overdue → `date` = now. Inbox/settings/account page keep posting as now.
- Act rule (`canEdit`): account owner, or non-guest write grant on the template's account — the same matrix as `recurring.checkWriteAccess`.
- Spent/actual display stays ONE combined number styled as today; the real vs upcoming difference is visible only in the transaction list.
- i18n: every new string in all 11 `locales/*.json`; no new top banners; analytics through existing recurring hooks.
- Golden files: regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/` and INSPECT the diff; never hand-edit.
- Go toolchain: `/usr/local/go/bin/go` with `GOTOOLCHAIN=go1.27.1` (not on PATH).

## Review Focus

1. A monthly template whose `ScheduledDay` is 31 projected through February → occurrences on Feb 28 and Mar 31, never skipping a month (Task 1 test `TestProjectOccurrences_MonthEndClamp`).
2. A template on a deleted member account, or on a member account of a participant who later left, must not crash the projection; deleted members still count like real rows do (Task 2 test `TestProjectedRead_DeletedMemberCounts`).
3. A cross-currency transfer template from an everyday member to a savings member → savings upcoming is in the recipient currency, converted, not the sender's number (Task 2 test `TestProjectedRead_CrossCurrencyTransfer`).
4. A viewer (guest role) of a shared budget opening a cell with the owner's template → row present, `canEdit: false`, no Post/Skip in the UI (Task 5 test `TestTxList_UpcomingGuestCannotEdit`, Task 9 test `renders partner template read-only`).
5. Selected month in the past → no upcoming at all, budget bytes for past months unchanged apart from the new zero fields (Task 3 test `TestBuildBudget_PastMonthNoUpcoming`).

---

### Task 1: Occurrence projection (model)

**Files:**
- Create: `internal/model/recurring_projection.go`
- Test: `internal/model/recurring_projection_test.go`

**Interfaces:**
- Produces:
  ```go
  type RecurringOccurrence struct {
      Template  *RecurringTransaction
      DueAt     time.Time // the payment's own date (overdue keeps its original date)
      FiledAt   time.Time // the date used for month bucketing/range filters (= DueAt, or today 00:00 for the overdue one)
      IsNext    bool      // DueAt == Template.NextPaymentAt
      IsOverdue bool      // DueAt before today
  }
  // today = caller-local midnight of now, expressed in the same wall clock as NextPaymentAt.
  func ProjectOccurrences(rt *RecurringTransaction, today, until time.Time) []RecurringOccurrence
  ```
  `until` is exclusive (first instant after the last requested month).

- [ ] **Step 1: Write failing tests** — table tests in `recurring_projection_test.go`:
  - `TestProjectOccurrences_MonthlyFuture`: next 2026-11-15, today 2026-10-22, until 2027-01-01 → DueAt 11-15 (IsNext), 12-15; FiledAt == DueAt.
  - `TestProjectOccurrences_WeeklyInMonth`: weekly next 2026-11-04, until 2026-12-01 → 4 occurrences (4, 11, 18, 25 Nov).
  - `TestProjectOccurrences_OverdueCollapses`: weekly next 2026-09-30, today 2026-10-22 → first is DueAt 09-30, IsOverdue, IsNext, FiledAt 2026-10-22 00:00; next is 10-28 (Oct 7/14/21 dropped).
  - `TestProjectOccurrences_DueToday`: next == today 10:00 → one occurrence, not overdue, FiledAt == DueAt.
  - `TestProjectOccurrences_MonthEndClamp`: monthly ScheduledDay 31, next 2027-01-31 → 02-28, 03-31.
  - `TestProjectOccurrences_UntilBeforeNext`: until <= next and not overdue → empty.
- [ ] **Step 2: Run** `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/model/ -run TestProjectOccurrences` → FAIL (undefined).
- [ ] **Step 3: Implement** with a loop over `NextOccurrence(cur, rt.Schedule, rt.ScheduledDay)`; cap iterations at 1000 as a guard.
- [ ] **Step 4: Run** the same command → PASS.
- [ ] **Step 5: Commit** `feat(budget): project recurring occurrences`.

### Task 2: Recurring port, glue and the projected read model

**Files:**
- Modify: `internal/budget/ports.go` (add port), `internal/budget/usecase.go` (field + `SetRecurring` setter, same pattern as `SetTransactionLabels`)
- Create: `internal/budget/projected_read.go`, `internal/server/glue_budget_recurring.go`
- Modify: `internal/server/server.go` (call `budgetSvc.SetRecurring(...)` once the recurring repo and account-access grants exist)
- Test: `internal/budget/projected_read_test.go`, `internal/server/glue_budget_test.go`

**Interfaces:**
- Consumes: Task 1 `ProjectOccurrences`.
- Produces:
  ```go
  // ports.go
  type RecurringSource interface {
      ListByAccountIDs(ctx context.Context, accountIDs []vo.Id) ([]*model.RecurringTransaction, error)
      CanEdit(ctx context.Context, userID, accountID vo.Id) (bool, error)
  }
  func (s *Service) SetRecurring(r RecurringSource)

  // projected_read.go
  type projectedRead struct { ReadModel /* real, for LabelsForUsers, LimitsByMonth, AccountsWithTransactions */ ; occ []projectedTx }
  func (s *Service) newProjectedRead(ctx context.Context, f filters, until time.Time) (*projectedRead, error)
  ```
  `projectedTx` is the occurrence flattened like a transaction row: type, accountID, account currency, recipient account + currency + amountRecipient, amount, category/payee/tag ids, label ids, FiledAt, DueAt, templateID, isNext, isOverdue, author (template `UserID`).
- With `SetRecurring` never called (tests, nil), `newProjectedRead` returns an empty model, so every existing test stays green.

`projectedRead.SummarizedLimits` returns nil (Task 3 relies on it). `projectedRead` also overrides, filtering `occ` by account set and `[start,end)` on FiledAt and mirroring each method's documented SQL rules in `readmodel.go` (category set filter with NULL kept, tag wins, type filters, transfer boundary rules, `AccountsNetByMonth` signs, label fan-out): `AccountsBalancesBeforeDate`, `CountSpending`, `CountSpendingByLabel`, `SpendingByMonth`, `IncomeByMonth`, `TransfersByMonth`, `AccountsNetByMonth`, `AccountsIncomeExpenseByMonth`, and every `BudgetTransactions*` list (rows with `ID` = template id, `SpentAt` = DueAt, newest first). `AccountsReport` and `HoldingsReport` return nil (balances summary is a non-goal). Month keys use the same `"YYYY-MM"` form the SQL returns. Amounts are summed with `vo.Decimal`. Cross-currency transfer recipient amounts: one `convertor.BulkConvert` call inside `newProjectedRead`, per occurrence month, sender → recipient currency. Account currencies come from `s.accounts.AccountsByIDs` over members + transfer recipients.

- [ ] **Step 1: Write failing tests** in `projected_read_test.go` using in-memory fakes for `RecurringSource`, `AccountLookup`, `Convertor` (see `builder_structure_test.go` for existing fakes):
  - `TestProjectedRead_CountSpendingFilesLikeSQL`: expense template tagged T, category C → row (C, T, currency); untagged uncategorized → row with nil category.
  - `TestProjectedRead_SpendingByMonthBuckets`: weekly template over Nov–Dec → two month rows with 4×/5× amount.
  - `TestProjectedRead_TransfersBoundary`: transfer member→outside → Out; member→member → no row.
  - `TestProjectedRead_CrossCurrencyTransfer`: USD everyday → EUR savings, fake rate 0.5 → `AccountsNetByMonth` savings row +50 for amount 100.
  - `TestProjectedRead_DeletedMemberCounts`: template on a soft-deleted member account is included.
  - `TestProjectedRead_NoSourceIsEmpty`: nil source → all methods empty.
- [ ] **Step 2: Run** `go test ./internal/budget/ -run TestProjectedRead` → FAIL.
- [ ] **Step 3: Implement** port, setter, `projectedRead`, glue (`ListByAccountIDs` over the recurring repo; `CanEdit` = owner via account lookup, else `HasWriteGrant`), wiring.
- [ ] **Step 4: Run** `go test ./internal/budget/ ./internal/server/ ./internal/test/archtest/` → PASS.
- [ ] **Step 5: Commit** `feat(budget): projected read model over recurring templates`.

### Task 3: get-budget upcoming fields

**Files:**
- Modify: `internal/model/budget_dto.go` (fields), `internal/budget/read.go` (GetBudget), create `internal/budget/upcoming_merge.go`
- Modify: `internal/web/mcp/prompts.go` (glossary: the new fields and that `spent`/`available` stay real-only)
- Test: `internal/budget/upcoming_merge_test.go`, `internal/test/apiparity/catalogue_budget_*.go` (new scenario), goldens

**Interfaces:**
- Consumes: Task 2 `newProjectedRead`.
- Produces (JSON, always present):
  - `ParentElementResult`: `Upcoming`, `BudgetUpcoming`, `UpcomingBefore` (`upcoming`, `budgetUpcoming`, `upcomingBefore`; element currency except `budgetUpcoming`).
  - `ChildElementResult`: `Upcoming`, `BudgetUpcoming`.
  - `LabelSpendResult`: `Upcoming`.
  - `SavingsElementResult`: `Upcoming`.
  - `func mergeBudgetUpcoming(real *model.BudgetResult, up model.BudgetResult)` — matches elements by `(Id, Type)`, children by `Id`, labels and savings by `Id`; an element present only in `up` is ignored (the real build already lists every non-archived element). `upcoming = up.Spent`, `budgetUpcoming = up.BudgetSpent`, `upcomingBefore = −up.Available − up.Spent`. That identity holds because `projectedRead.SummarizedLimits` returns nil (Task 2), so the up build's `budgetedBefore` is 0 and its `Available = −upcomingBefore − upcoming` (`builder_structure_build.go`: `Available = budgetedBefore − spentBefore − spent`).
- GetBudget: when the selected month ends before the caller's current month starts, skip the projected build (zero fields). Otherwise build with a `Service` copy whose `read` is the projected model (`sp := *s; sp.read = pr`), `until = periodEnd`, then merge.

- [ ] **Step 1: Write failing tests** in `upcoming_merge_test.go`:
  - `TestBuildBudget_UpcomingCurrentMonth`: budget with category C, template 50 on day 25, today 22 → C `upcoming "50"`, `spent` unchanged, `upcomingBefore "0"`.
  - `TestBuildBudget_UpcomingFutureMonthCarry`: select next month; monthly template 50 → selected `upcoming 50`, `upcomingBefore 50` (this month's).
  - `TestBuildBudget_PastMonthNoUpcoming`: past month → all upcoming fields `"0"`.
  - `TestBuildBudget_UpcomingTagChild`: tagged template → tag element `upcoming`, child category `upcoming`.
- [ ] **Step 2: Run** `go test ./internal/budget/ -run TestBuildBudget_` → FAIL.
- [ ] **Step 3: Implement** fields, projected build, merge, glossary.
- [ ] **Step 4: Add apiparity scenario** `budget/get-budget with upcoming recurring` (monthly + weekly template on a member account, plus a partner's template in a shared budget), then `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`; inspect the diff: existing goldens gain only the zero fields.
- [ ] **Step 5: Run** `make go-test` → PASS (coverage gate ≥ 80).
- [ ] **Step 6: Commit** `feat(budget): upcoming recurring amounts on get-budget`.

### Task 4: get-budget savings closing balance with upcoming

**Files:**
- Modify: `internal/budget/builder_savings.go` (`addSavingsPending`, `emitMonthlySavings`)
- Test: `internal/budget/builder_savings_test.go` (or the existing savings test file)

**Interfaces:**
- Consumes: Task 2/3 (projected `AccountsNetByMonth` per savings account and month).
- Rule (spec): from the current month through the selected one, per month and savings row, `closingBalance += upcoming_m + max(0, planned_m − actual_m − upcoming_m)` instead of `max(0, planned_m − actual_m)`. Months before the current one unchanged.

- [ ] **Step 1: Failing test** `TestSavingsClosing_UpcomingTransfer`: savings member, plan 100 this month, actual 0, recurring transfer 150 into it → closing = booked + 150; with plan 200 → booked + 200.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** by passing the projected per-month net into the pending computation. **Step 4: Run** `go test ./internal/budget/` + regenerate/inspect goldens → PASS.
- [ ] **Step 5: Commit** `feat(budget): savings closing balance counts upcoming transfers`.

### Task 5: get-budget-plan upcoming fields

**Files:**
- Modify: `internal/model/budget_dto.go`, `internal/budget/builder_plan.go` (or the GetBudgetPlan use case), `internal/budget/upcoming_merge.go`
- Test: `internal/budget/upcoming_merge_test.go`, apiparity scenario + goldens

**Interfaces:**
- Produces (always present):
  - `PlanCellResult.Upcoming`, `PlanChildCellResult.Upcoming`, `PlanSavingsCellResult.Upcoming` (`upcoming`, `"0"` when none).
  - `BudgetPlanResult`: `UpcomingOpeningBalances`, `UpcomingSavingsOpeningBalances` (`[]OpeningBalanceResult`), `UpcomingTransfers` (`[]PlanMonthTransfersResult`), `UpcomingSavingsFlows`, `UpcomingSavingsIncomeExpense` (`[]PlanSavingsFlowResult`).
  - `func mergePlanUpcoming(real *model.BudgetPlanResult, up model.BudgetPlanResult)` — elements by `(Id, Type)`, cells by index, children by `Id`, savings by `Id`; top-level arrays copied from the up build.
- Projected build only when the window's last month ≥ the caller's current month; `until` = window end.

- [ ] **Step 1: Failing tests** `TestBuildPlan_UpcomingCells` (monthly template → upcoming in current and every later window month, actual unchanged), `TestBuildPlan_UpcomingTransfersAndSavings`, `TestBuildPlan_WindowInPastNoUpcoming`.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4:** apiparity scenario for plan, regenerate + inspect goldens, `make go-test` → PASS.
- [ ] **Step 5: Commit** `feat(budget): upcoming recurring amounts on get-budget-plan`.

### Task 6: Transaction list `upcoming` array

**Files:**
- Modify: `internal/model/budget_dto.go`, `internal/budget/txlist.go`
- Test: `internal/budget/txlist_upcoming_test.go`, apiparity scenario + goldens

**Interfaces:**
- Produces: `GetBudgetTransactionListResult.Upcoming []BudgetUpcomingResult` (`upcoming`, always `[]`):
  ```go
  type BudgetUpcomingResult struct {
      BudgetTransactionResult            // id = template id, spentAt = due date
      TemplateId  string `json:"templateId"`
      Schedule    string `json:"schedule"`
      AccountId   string `json:"accountId"`
      AccountName string `json:"accountName"`
      LabelNames  []string `json:"labelNames"`
      IsNext      bool   `json:"isNext"`
      IsOverdue   bool   `json:"isOverdue"`
      CanEdit     bool   `json:"canEdit"`
  }
  ```
  `Direction`/`Type` follow the same selector rules as `items`. Built by running the same selector dispatch against the projected read (`until` = periodEnd) and enriching from the occurrence; `CanEdit` via `RecurringSource.CanEdit(caller, accountID)`, cached per account.

- [ ] **Step 1: Failing tests** `TestTxList_UpcomingCategory` (two weekly rows, newest first, `isNext` only on the earliest overall), `TestTxList_UpcomingOverdue` (overdue row in current month with original due date, `isOverdue`), `TestTxList_UpcomingGuestCannotEdit`, `TestTxList_UpcomingPastMonthEmpty`.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4:** apiparity scenario, regenerate + inspect, `make go-test` → PASS; run `make test-repo-pgsql` (or `DATABASE_TEST_PGSQL_URL=… go test -tags enginecompare ./internal/test/enginecompare/`) → PASS.
- [ ] **Step 5: Commit** `feat(budget): upcoming rows in the budget transaction list`.

### Task 7: SPA DTOs, fold into combined figures, cache invalidation

**Files:**
- Modify: `web/src/api/dto/budget.ts`, `web/src/features/budgets/queries.ts`, `web/src/features/recurring/queries.ts`
- Create: `web/src/features/budgets/upcoming.ts`
- Test: `web/src/features/budgets/upcoming.test.ts`, `web/src/features/recurring/queries.test.tsx`

**Interfaces:**
- Consumes: Tasks 3–6 wire fields (optional in the DTO types — an older server sends none, treated as zero).
- Produces:
  ```ts
  export function foldBudgetUpcoming(b: BudgetDto): BudgetDto   // spent += upcoming, budgetSpent += budgetUpcoming, available -= upcomingBefore + upcoming (elements); children spent/budgetSpent; labels spent; savings spent
  export function foldPlanUpcoming(p: BudgetPlanDto): BudgetPlanDto // cell.actual += upcoming (all cell kinds); openingBalances/savingsOpeningBalances += upcoming*; transfers/savingsFlows/savingsIncomeExpense merged per (month|period, currencyId); savings cell closingBalance += cumulative upcoming net of that row through the month
  ```
  Applied as `select` on `useBudget` and `useBudgetPlan`; optimistic `setQueryData` paths keep operating on the raw cache. Decimal math via `@/lib/decimal`.
- Every recurring mutation (`useCreateRecurring`, `useUpdateRecurring`, `useDeleteRecurring`, `useSkipRecurring`, `usePostRecurring`) invalidates `queryKeys.budget`, `queryKeys.budgetPlan` and the `['budgetTransactions']` key.

- [ ] **Step 1: Failing tests**: `foldBudgetUpcoming adds upcoming to spent and lowers available` (200 budgeted, spent 120, upcoming 50, upcomingBefore 0 → spent 170, available reduced by 50); `fold is identity without upcoming fields`; `foldPlanUpcoming adds cell upcoming and merges transfers by currency`; `savings closing adds cumulative upcoming`; `skip recurring invalidates budget, plan and budget transactions`.
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/budgets/upcoming.test.ts src/features/recurring/queries.test.tsx` → FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `cd web && pnpm test` → PASS.
- [ ] **Step 5: Commit** `feat(web): fold upcoming recurring into budget and plan figures`.

### Task 8: Show the combined figure in future months

**Files:**
- Modify: `web/src/features/budgets/BudgetTable.tsx`, `BudgetPage.tsx`, `budgetMath.ts` (only where `future` hides spent), `planCell.ts`, `planMath.ts` (`planSelectionSum`)
- Test: `BudgetTable.test.tsx`, `planCell.test.ts`, `planMath.test.ts`

**Interfaces:**
- Budget future month: spent shows the (folded) figure instead of `<Dash/>` when non-zero, and is clickable; zero keeps the dash. Progress/over-budget state uses the combined figure.
- `planCellView`: a month after `selected` shows `actual` when it is non-zero (it now carries upcoming); zero keeps today's plan-only rendering.
- `planSelectionSum`: Actual includes future months.
- `planTotals`: unchanged code (its `max(actual, planned)` now sees the folded actual) — assert it in a test.

- [ ] **Step 1: Failing tests**: `future month shows spent when upcoming exists` (BudgetTable), `future plan cell shows folded actual` (planCell), `planTotals counts unplanned upcoming in a future month`, `selection sum includes future actual`.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** `cd web && pnpm test && pnpm lint` → PASS.
- [ ] **Step 5: Commit** `feat(web): show upcoming amounts in future budget and plan months`.

### Task 9: Upcoming rows in the cell transaction list

**Files:**
- Modify: `web/src/features/budgets/BudgetTransactionsDialog.tsx`, `web/src/api/dto/budget.ts`, `web/src/features/recurring/postPayload.ts`, `web/src/features/recurring/ViewRecurringDialog.tsx` (read-only mode), `locales/*.json`
- Test: `BudgetTransactionsDialog.test.tsx`, `postPayload.test.ts`, `ViewRecurringDialog.test.tsx`

**Interfaces:**
- Consumes: Task 6 `upcoming` items.
- `recurringPostPayload(recurring, accounts, exchangeFn, opts?: { date?: Date })` — `date` defaults to now.
- Dialog: upcoming rows merge into the day groups by `spentAt` (same `dayKey`), rendered with `TransactionRow` via a mapper like `recurringAsTransaction`, with the repeat icon, `isInFuture` dimming for future rows, and the overdue note (reuse the Inbox key) on `isOverdue`. Future-dated real rows (`isFuture(spentAt)`) get the same dimming.
- Tap an upcoming row:
  - `isNext && canEdit` → `ViewRecurringDialog` for the template from `useRecurring()` with Post / Skip / Edit; Post calls `recurringPostPayload(..., { date: isFuture(dueAt) ? dueAt : new Date() })`.
  - otherwise → `ViewRecurringDialog` in a `readOnly` mode built from the item (no Post/Skip; Edit only when `canEdit` and the template is in `useRecurring()`).
- New i18n keys (all 11 catalogues): `budgets.transactions.upcoming_hint` ("Upcoming recurring payment"), `budgets.transactions.later_cycle` ("Can be posted after the earlier payment") — exact wording builder's call, en reference.

- [ ] **Step 1: Failing tests**: `shows weekly upcoming rows in date groups`, `next payment offers post and skip`, `later cycle has no post or skip`, `renders partner template read-only`, `post from budget uses scheduled date when in future` (payload `date` = due date), `post overdue uses now`.
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/budgets/BudgetTransactionsDialog.test.tsx src/features/recurring` → FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** `cd web && pnpm test && pnpm lint` and `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/` → PASS.
- [ ] **Step 5: Commit** `feat(web): upcoming recurring rows in the budget transaction list`.

### Task 10: Docs and full verification

**Files:**
- Modify: `docs/regression-test-plan.md` (Budget + Plan sections), `CLAUDE.md` (one "Notable behaviours" bullet: upcoming recurring projection, additive fields, overdue rule, post-on-scheduled-date from the budget list)

- [ ] **Step 1:** Add regression items: upcoming in current month (combined Spent, Available lowered); future month shows upcoming instead of dash; Plan future cell shows upcoming, Balance moves without a plan; weekly template = one row per week; overdue = one row in current month then only after today; post from list (future → scheduled date, overdue → today); later cycle display only; partner template counts, display only for non-editors 📱; recurring edit refreshes Budget and Plan.
- [ ] **Step 2: Run** `make test` (full: Go smoke + pgsql engine comparison + web) → PASS.
- [ ] **Step 3: Commit** `docs: regression plan and notes for upcoming recurring payments`.
