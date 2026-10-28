# Upcoming recurring as a plan hint: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop folding upcoming recurring payments into spent / actual and the projections, and use them only as a hint on the plan number: a grey italic suggestion when there is no plan, red when an expense plan is short, and a list of the payments while the plan is being edited.

**Architecture:** The server keeps #363's projection and the three per-cell `upcoming` fields the hint needs. It drops what only fed the projections. The SPA rolls back #363's folding and display: no budget file has changed since #363 (`54c626a5`) apart from `BudgetPage.test.tsx`, so `git show 54c626a5^:<path>` is the baseline. One pure function, `planHint`, decides the hint everywhere, and one component, `UpcomingPaymentsPanel`, renders the list inside every plan editor.

**Tech Stack:** Go 1.27 (sqlc, apiparity / mcpparity goldens, swag), React 19 + TypeScript, TanStack Query, vitest, Tailwind.

**Spec:** `docs/superpowers/specs/2026-10-24-budget-upcoming-as-plan-design.md` (revises `docs/superpowers/specs/2026-10-22-budget-upcoming-recurring-design.md`).

## Global Constraints

- Go: `export PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1` before any go / make command.
- Facts (spent, actual, Available, bars, red spent/Available, folder/group/total rows, Balance, Total savings, savings `closingBalance`, carry-over, the multi-cell selection sum) are real money plus plans the user set. Upcoming money is never added to them.
- The grey suggestion is display only. Nothing is saved, and no total counts it.
- Suggestion style: grey **and italic**, i.e. `italic text-muted-foreground`. Short (red) style: `text-expense`.
- Red only on expense rows (`CATEGORY`, `TAG`, `ENVELOPE`). Income (`INCOME_CATEGORY`, `INCOME_ENVELOPE`) and `SAVINGS` rows never go red.
- No hint on folder, tag-group, total, label or child rows; only on rows that hold a plan.
- Spent lists (`BudgetTransactionsDialog`) render real transactions only. Future-dated real rows keep the upcoming style.
- Every recurring mutation keeps invalidating `budget`, `budgetPlan` and `budgetTransactions` (`useInvalidateBudgetUpcoming` in `web/src/features/recurring/queries.ts` stays).
- Comments: sparing; why, not what (CLAUDE.md).
- Every catalogue key is added to all 11 `locales/*.json`.

## Review Focus

1. **Foreign-currency element:** a row whose currency differs from the budget's compares plan, spent and upcoming in the ELEMENT currency (`budgeted` / `spent` / `upcoming`), never mixing in `budgetSpent` / `budgetUpcoming`. Test in Task 3 (`planHint` is currency-blind) and Task 4 (a USD row in a EUR budget).
2. **Plan `"0"` vs unset `""`:** both mean "no plan" and show the suggestion. Task 3 test.
3. **Savings net `upcoming` ≤ 0:** no suggestion, never red. Task 3 test.
4. **Cell with no upcoming money:** looks exactly as before (no extra request when its editor opens). Tasks 3 and 6 tests.
5. **Totals ignore the suggestion:** a budget with one suggested row and no plans shows a Budgeted total of 0 and a Plan Balance unchanged by it. Tests in Tasks 4 and 5.

---

### Task 1: Server — drop the projection-only upcoming fields

**Files:**
- Modify: `internal/model/budget_dto.go` (remove `UpcomingBefore`, `UpcomingOpeningBalance`, plan `UpcomingOpeningBalances`, `UpcomingSavingsOpeningBalances`, `UpcomingTransfers`, `UpcomingSavingsFlows`, `UpcomingSavingsIncomeExpense`)
- Modify: `internal/budget/upcoming_merge.go`, `internal/budget/envelopes.go:276`, `internal/budget/builder_plan.go:95-99`, `internal/budget/builder_savings.go` (`addSavingsPending`, `pendingSavingsMonth`, `emitMonthlySavings`, `emitPlanSavings`)
- Modify: `internal/web/mcp/prompts.go` (glossary lines for the removed fields; reword `upcoming` as "recurring payments still due in the month, a planning hint; never part of spent/actual/closingBalance")
- Test: `internal/budget/api/upcoming_test.go`, `internal/budget/upcoming_merge_test.go`, apiparity / mcpparity goldens, `internal/web/apidoc/docs/*` (via `make swagger`)

**Interfaces:**
- Produces (wire, unchanged and kept): `get-budget` element / child / savings / label `upcoming` + `budgetUpcoming`; `get-budget-plan` cell / child cell / savings cell `upcoming`; `get-transaction-list` `upcoming[]`.
- Produces: savings `closingBalance` on `get-budget` = booked + each month's unmet plan gap (`planned − actual`, when > 0), current month through the selected one. This is the pre-#363 rule (`git show 54c626a5^:internal/budget/builder_savings.go`).

- [ ] **Step 1: Update the tests to the new contract.** In `internal/budget/api/upcoming_test.go`, change every assertion on a savings `closingBalance` that counted upcoming money to booked + plan gap, and delete the assertions on the removed fields. Add `TestUpcomingNotInSavingsClosing`: a savings member with no plan and a monthly 100 transfer template into it; `get-budget` for next month returns that row's `upcoming` = `"100"` and `closingBalance` equal to the booked balance.
- [ ] **Step 2: Run them and confirm they fail.** `go test ./internal/budget/...` → FAIL (closingBalance includes 100; the removed fields still compile).
- [ ] **Step 3: Remove the fields and their computation.** In `addSavingsPending`, drop `upcomingByKey` / `upcomingKey`; `pendingSavingsMonth` goes back to `{planned vo.DecimalNumber; actualKey string}`. `emitMonthlySavings` drops the upcoming add. In `upcoming_merge.go`, remove `upcomingBefore(...)`, the opening-balance helper (around line 249) and the five plan top-level assignments (306-310). Keep the per-cell / per-element `upcoming` merge and the "rows carrying only upcoming money are added" behavior.
- [ ] **Step 4: Run** `go build ./... && go test ./internal/budget/... ./internal/server/...` → PASS.
- [ ] **Step 5: Regenerate the contract artifacts.** `make swagger`, then `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`. Inspect `git diff internal/test`: only the removed keys disappear, and savings `closingBalance` values change only in `budget_upcoming_recurring.golden`.
- [ ] **Step 6: Run** `make go-test` → PASS (coverage gate included).
- [ ] **Step 7: Commit** `fix(budget): upcoming money no longer feeds projections` (paths: `internal/`).

---

### Task 2: SPA — roll back #363's folding and the spent-list actions

**Files** (baseline = `git show 54c626a5^:<path>`):
- Delete: `web/src/features/budgets/upcoming.ts`, `upcoming.test.ts`
- Restore to baseline, then re-apply only the KEEP items below: `budgets/{queries.ts, planMath.ts, planMath.test.ts, budgetMath.ts, budgetMath.test.ts, BudgetTable.tsx, BudgetTable.test.tsx, MonthFlows.tsx, PlanRows.tsx, planCell.ts, planCell.test.ts, phoneMonth.ts, phoneMonth.test.ts, PlanSheet.tsx, monthLayout.ts, ElementSheet.tsx, ElementSheet.test.tsx, PhoneMonthView.tsx, PhoneMonthView.test.tsx, BudgetTransactionsDialog.tsx, BudgetTransactionsDialog.test.tsx}`, `recurring/{ViewRecurringDialog.tsx, ViewRecurringDialog.test.tsx, asTransaction.ts, asTransaction.test.ts, postPayload.ts, postPayload.test.ts}`, `transactions/ViewTransactionDialog.tsx`
- Modify: `web/src/api/dto/budget.ts` (drop `upcomingBefore`, `upcomingOpeningBalance` and the five plan `upcoming*` arrays; keep element / child / savings / label `upcoming` + `budgetUpcoming`, cell `upcoming`, `BudgetUpcomingDto`, `BudgetTransactionListDto.upcoming`)
- Modify: `budgets/BudgetPage.test.tsx`, `BudgetPage.phone.test.tsx`, `PlanSheet.test.tsx`, `PlanSheet.savings.test.tsx`, `queries.test.tsx`, `recurring/queries.test.tsx` (drop #363 cases that assert combined figures, upcoming list rows or post/skip from the budget list)

**KEEP from #363** (re-apply after restoring):
- `budgetMath.ts`: `hasUpcoming` and its two uses in `bucketElements` (upcoming-only uncategorized / archived rows stay visible, so their suggestion has a row). Revert `rowState`, `rowProgress`, `overBudget`; delete `overAvailable` (and its uses).
- `PlanSheet.tsx`: the `moved` visibility predicate (rows with upcoming stay visible). Revert `planSelectionSum` to the baseline signature `planSelectionSum(cells, monthIndex, ex)`.
- `phoneMonth.ts`: `received` also true for `upcoming ≠ 0`. `transfersNet` back to `totals[index].transfersNet`.
- `BudgetTransactionsDialog.tsx`: future-dated REAL rows keep the upcoming style.
- `recurring/queries.ts`: untouched (invalidation stays).
- `api/budget.ts`: `getBudgetTransactions` still returns `upcoming`.

**Interfaces:**
- Produces: `useBudget()` / `useBudgetPlan()` return raw wire data (no `select`).

- [ ] **Step 1: Restore and delete.** `git checkout 54c626a5^ -- <files above>` plus `git rm` of `upcoming.ts` / `upcoming.test.ts`, then re-apply the KEEP items.
- [ ] **Step 2: Add the regression test** `BudgetTransactionsDialog.test.tsx` › `lists real transactions only`: the mocked response has `items: [one real]` and `upcoming: [one template]`; only the real description renders, with no Post / Skip button.
- [ ] **Step 3: Add the regression test** `BudgetTable.test.tsx` › `future month spent is real only`: a future month, element `spent: '0'`, `upcoming: '650'`; the spent cell shows the dash, not 650.
- [ ] **Step 4: Run** `cd web && pnpm exec tsc --noEmit && pnpm test -- features/budgets features/recurring features/transactions` → PASS.
- [ ] **Step 5: Run** `pnpm lint` → clean, and `pnpm test` (whole suite) → PASS (the metrics coverage test may need a removed metric call restored or excused; check the baseline).
- [ ] **Step 6: Commit** `revert(web): upcoming recurring no longer folds into budget figures`.

---

### Task 3: `planHint` — the one rule

**Files:**
- Create: `web/src/features/budgets/planHint.ts`
- Test: `web/src/features/budgets/planHint.test.ts`

**Interfaces:**
- Produces:
  ```ts
  export type PlanHintKind = 'expense' | 'income' | 'savings'
  export type PlanHint = { kind: 'suggest'; amount: string } | { kind: 'short' } | null
  export function planHintKind(type: BudgetElementType): PlanHintKind
  /** planned: '' or a decimal (element currency); spent: real money so far; upcoming: still due (absent = 0) */
  export function planHint(input: { planned: string; spent: string; upcoming?: string; kind: PlanHintKind }): PlanHint
  export const PLAN_SUGGEST_CLASS = 'italic text-muted-foreground'
  ```

Rule: `need = spent + upcoming`. If `upcoming` is absent or zero → `null`. For savings, an `upcoming` ≤ 0 → `null`. No plan (`planned` is `''` or zero) → `need > 0 ? { kind: 'suggest', amount: need } : null`. Plan set → `kind === 'expense' && planned < need ? { kind: 'short' } : null`.

- [ ] **Step 1: Write the failing tests** (`planHint.test.ts`):
  - `suggests spent + still due with no plan`: `{planned: '', spent: '300', upcoming: '250', kind: 'expense'}` → `{kind: 'suggest', amount: '550'}`
  - `plan "0" counts as no plan`: `planned: '0'`, `spent: '0'`, `upcoming: '650'` → suggest `'650'`
  - `short when the expense plan is below the need`: `planned: '500'`, `spent: '300'`, `upcoming: '250'` → `{kind: 'short'}`
  - `normal when the plan covers the need`: `planned: '600'` → `null`
  - `nothing without upcoming`: `upcoming: undefined` and `upcoming: '0'` with `planned: ''` → `null`
  - `income never short`: `kind: 'income'`, `planned: '100'`, `upcoming: '500'` → `null`; with `planned: ''` → suggest `'500'`
  - `savings net out shows nothing`: `kind: 'savings'`, `planned: ''`, `spent: '0'`, `upcoming: '-200'` → `null`; `upcoming: '0'` → `null`
  - `planHintKind` maps `CATEGORY`, `TAG`, `ENVELOPE` → `'expense'`; `INCOME_CATEGORY`, `INCOME_ENVELOPE` → `'income'`; `SAVINGS` → `'savings'`
- [ ] **Step 2: Run** `pnpm test -- planHint` → FAIL (module missing).
- [ ] **Step 3: Implement** with `add`, `cmp`, `isZero` from `@/lib/decimal`.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat(web): planHint rule for upcoming recurring`.

---

### Task 4: Budget monthly view shows the hint

**Files:**
- Modify: `web/src/features/budgets/BudgetTable.tsx` (expense element rows and tag-group member rows; budgeted column: the `LimitEditor` trigger and its read-only text)
- Modify: `web/src/features/budgets/MonthFlows.tsx` (income rows' Planned, savings rows' Planned)
- Modify: `web/src/features/budgets/PhoneMonthView.tsx`, `ElementSheet.tsx` (phone rows and the sheet's Budget / Planned figure)
- Modify: `web/src/features/budgets/LimitEditor.tsx` (trigger accepts a `hint: PlanHint` prop and renders it)
- Test: `BudgetTable.test.tsx`, `PhoneMonthView.test.tsx`, `BudgetPage.test.tsx`

**Interfaces:**
- Consumes: `planHint`, `planHintKind`, `PLAN_SUGGEST_CLASS` (Task 3).
- Inputs per row: expense element → `{planned: el.budgeted, spent: el.spent, upcoming: el.upcoming}`; savings row → `{planned: row.budgeted, spent: row.spent, upcoming: row.upcoming}`; income row (plan cell figures) → `{planned: cell.planned, spent: cell.actual, upcoming: cell.upcoming}`. Everything is in element currency.
- Produces: `LimitEditor` prop `hint?: PlanHint`. Suggest → the trigger shows `moneyFormat(hint.amount)` with `PLAN_SUGGEST_CLASS`; short → the plan with `text-expense`.

- [ ] **Step 1: Write the failing tests:**
  - `BudgetTable.test.tsx` › `suggests the upcoming total in grey italic with no plan`: a future month, `budgeted '0'`, `spent '0'`, `upcoming '650'` → the budgeted cell text is `650` and has the classes `italic` and `text-muted-foreground`.
  - › `plan below upcoming is red`: `budgeted '500'`, `upcoming '650'` → the budgeted cell has `text-expense`.
  - › `suggestion is not in totals`: one such row, no plans → the totals' Budgeted shows 0.
  - › `foreign-currency row compares in its own currency`: a USD element in a EUR budget, `budgeted '500'` (USD), `upcoming '400'` (USD), `budgetUpcoming '9999'` → not red.
  - › `envelope child shows no hint`: an envelope child with `upcoming` set → its row has no suggestion text.
  - `PhoneMonthView.test.tsx` › `phone row suggests with no plan`: same data as the first test → the row's plan figure shows `650` in italic.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.** Compute the hint where each row renders its plan figure. Totals and folder sums keep reading `budgeted`.
- [ ] **Step 4: Run** `pnpm test -- features/budgets` → PASS.
- [ ] **Step 5: Commit** `feat(web): budget view shows upcoming recurring as a plan hint`.

---

### Task 5: Plan grid shows the hint

**Files:**
- Modify: `web/src/features/budgets/planCell.ts` (`PlanCellView` gains `hint: PlanHint`)
- Modify: `web/src/features/budgets/PlanRows.tsx` (`ElementRow` plan figure)
- Test: `planCell.test.ts`, `PlanSheet.test.tsx`

**Interfaces:**
- Consumes: Task 3.
- Produces: `planCellView(input)` returns `hint` computed from `{planned: cell.planned, spent: cell.actual, upcoming: cell.upcoming, kind: planHintKind(type)}`. It is `null` for a month before `selected` (the caller's current month), and for the savings `balance` view `null` unless `planned` is unset (the savings hint compares saved, not the closing balance: use `cell.actual`).
- With `hint.kind === 'suggest'`, `PlanRows` renders `hint.amount` in the plan slot with `PLAN_SUGGEST_CLASS`; with `'short'`, the plan in `text-expense`.

- [ ] **Step 1: Write the failing tests:**
  - `planCell.test.ts` › `future cell with no plan suggests upcoming`: `{planned: '', actual: '0', upcoming: '650'}`, `month > selected` → `hint` = suggest `'650'`, `plan` stays `null`.
  - › `current month short`: `{planned: '500', actual: '300', upcoming: '250'}`, `month === selected` → `hint` = `{kind: 'short'}`.
  - › `past month has no hint` → `null`.
  - `PlanSheet.test.tsx` › `Balance ignores a suggested plan`: an income template of 1000 next month and no plans → that month's Balance equals the opening balance.
  - › `grid shows the suggestion in italic`: the cell's plan slot shows `650` with `italic`.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `pnpm test -- features/budgets` → PASS.
- [ ] **Step 5: Commit** `feat(web): plan grid shows upcoming recurring as a plan hint`.

---

### Task 6: The plan editors list the upcoming payments

**Files:**
- Create: `web/src/features/budgets/budgetTransactionsParams.ts` (extracted from `BudgetTransactionsDialog.tsx:105-140`, the dialog then imports it)
- Create: `web/src/features/budgets/UpcomingPaymentsPanel.tsx`
- Modify: `LimitEditor.tsx`, `SetLimitDialog.tsx`, `PlanCellInput.tsx`, and their callers `BudgetPage.tsx`, `PlanRows.tsx`, `PlanSheet.tsx`
- Modify: `locales/*.json` (11): `budgets.page.upcoming.title` = "Recurring payments due" and `budgets.page.upcoming.total` = "Total {amount}" (translated in each catalogue's register, see the locale-register memory: de Sie, nl u, fr vous, pt European); delete #363 keys nothing reads any more
- Test: `UpcomingPaymentsPanel.test.tsx`, `budgetTransactionsParams.test.ts`, `LimitEditor` / `PlanSheet.editing.test.tsx` cases

**Interfaces:**
- Produces:
  ```ts
  export function budgetTransactionsParams(budgetId: Id, target: BudgetTransactionsTarget, periodStart: string): BudgetTransactionsParams
  export interface UpcomingQuery { budget: BudgetDto | { meta: { id: Id } }; target: BudgetTransactionsTarget; periodStart: string; currency: CurrencyDto | undefined }
  export function UpcomingPaymentsPanel(props: { query: UpcomingQuery }): ReactElement | null
  ```
- The panel calls `useBudgetTransactions(budgetTransactionsParams(...))` and renders `data.upcoming`, one row each: due date (`isOverdue` → the Inbox's overdue wording key), `description || payee?.name || category?.name`, and the amount in `query.currency`; then a total line. A savings target shows the signed amounts (direction out = negative) and the net total. It renders `null` while loading or when the list is empty.
- `LimitEditor`, `SetLimitDialog` and `PlanCellInput` gain `upcoming?: UpcomingQuery | null`. Callers pass it ONLY when the cell's `upcoming` is non-zero, so a cell without upcoming money triggers no request. `LimitEditor` and `SetLimitDialog` place the panel under the amount field. `PlanCellInput` places it as an absolute panel below the input (`top-full right-0 z-30`, `w-64`, `bg-popover shadow`), which keeps the grid's key handling.

- [ ] **Step 1: Write the failing tests:**
  - `budgetTransactionsParams.test.ts`: a category under a tag → `{categoryId, tagId}`; Uncategorized → `{uncategorized: true}`; savings → `{accountId}`; income envelope → `{income: true, envelopeId}` (the cases the dialog builds today).
  - `UpcomingPaymentsPanel.test.tsx` › `lists payments with their total`: two upcoming (`Rent 500`, `Gym 50`) → both rows render and the text `Total 550` appears; › `renders nothing when the list is empty`; › `marks an overdue payment`.
  - `PlanSheet.editing.test.tsx` › `editing a cell with upcoming shows the payments`; › `a cell without upcoming makes no transactions request` (assert the mocked `getBudgetTransactions` was not called).
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement**, then switch `BudgetTransactionsDialog` to `budgetTransactionsParams`.
- [ ] **Step 4: Run** `pnpm test -- features/budgets && pnpm exec tsc --noEmit && pnpm lint`, then `make go-test` (the i18n guards) → PASS.
- [ ] **Step 5: Commit** `feat(web): plan editors list upcoming recurring payments`.

---

### Task 7: Docs

**Files:**
- Modify: `docs/regression-test-plan.md`: replace #363's budget / plan items (combined figure, the list's upcoming rows, post / skip from the list, current month fact-only dash link) with the spec's list (grey italic suggestion; red when the expense plan is short, future and current month; not in totals; the editor lists payments; the spent list is real only; income / savings never red). Keep the 📱 markers accurate.
- Modify: `CLAUDE.md`, "Upcoming recurring in Budget/Plan" bullet: rewrite it to the new behavior, keeping the projection rules (member accounts, partners' templates, overdue once, after today).
- Modify: the header of `docs/superpowers/specs/2026-10-22-budget-upcoming-recurring-design.md`: one line saying its display and projection parts are superseded by the 2026-10-24 spec.

- [ ] **Step 1: Edit the three files.**
- [ ] **Step 2: Run** `make go-test && make web-test` → PASS.
- [ ] **Step 3: Commit** `docs: upcoming recurring as a plan hint`.
