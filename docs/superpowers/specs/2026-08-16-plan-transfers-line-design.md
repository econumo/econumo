# Plan sheet: Transfers totals line + drill-down links

**Date:** 2026-08-16 · **Branch:** `feature/budget-plan-view`

## Problem

A transfer between a budgeted (included) account and an excluded one moves
real money across the budget boundary, but the plan sheet never sees it:
`get-budget-plan` fills cells from `SpendingByMonth` (`type=0`) and
`IncomeByMonth` (`type=1`) only, and transfers carry no category. Within the
fetched window the Balance row does not move; once the transfer month scrolls
*before* the window start it silently lands in the opening balance
(`AccountsBalancesBeforeDate` is full account arithmetic). The same future
month therefore shows a different Balance depending on where the window
begins, and the user has no line explaining the jump.

## Behaviour

### Totals block

Order becomes: Income · Expenses · Uncategorized · **Transfers** · Net, then
the sticky Balance row.

- **Transfers** — per month, the net of transfers that crossed the budget
  boundary: `in − out`, where *in* = transfers whose recipient is an
  included account and whose source is not, measured in the recipient
  account's currency; *out* = the mirror, measured in the source account's
  currency. Each (currency, amount) is converted to the budget currency with
  that month's plan rates (`MonthExchange`), like every other cell. Transfers
  between two included accounts cancel; transfers touching no included
  account are ignored. Cross-currency transfers ARE counted (no
  `amount = amount_recipient` guard — unlike budget mode's `HoldingsReport`,
  each side is measured in its own account's currency, so the figure is
  correct without it). Positive = money came in; negative = money left,
  rendered with the same `text-destructive` class the Balance row uses.
- **Net** = `Income − Expenses + Transfers` (effective figures, per the
  existing per-cell max(actual, planned) rule; transfers have no planned
  value, so they contribute their actual in every month). Balance keeps
  running on Net, so `Balance[m] − Balance[m−1] = Net[m]` still holds and the
  Transfers line is exactly the explanation for the change. `netActual`
  includes transfers too; `netPlanned` stays income − expense (transfers are
  never planned).
- Always rendered, like Uncategorized — a stable layout while scrolling.
- Label: `Transfers` / «Переводы».

### Drill-down links

The **Transfers** and **Uncategorized** totals numbers become links (the same
`button` treatment as `BudgetTable`'s spent cell) when non-zero; a zero
renders as plain text (nothing to list). Click opens the existing
`BudgetTransactionsDialog` scoped to **that column's month**:

- Transfers → the transfer list (new backend selector, below). Title
  "Transfers", icon `swap_horiz`. Row amounts are signed by direction: out →
  negative, in → positive.
- Uncategorized → the existing `uncategorized: true` list (uncategorized,
  untagged expenses). Known limitation, inherent to how the line is already
  defined: the line nets out uncategorized *income* (which has its own row),
  so when such income exists in the month the list does not sum to the
  number. Not widened in this change.
- Tooltips (`title`): Transfers → "In {in} · Out {out}. Show transactions";
  Uncategorized → "Show transactions" (the shared `budgets.page.plan.totals.
  show_transactions` string).

## Backend

### `get-budget-plan`

- `ReadModel.TransfersByMonth(ctx, accountIDs, from, to) ([]model.MonthlyTransferRow, error)`
  — `MonthlyTransferRow{Month, CurrencyID, In, Out string}`. Two grouped
  queries in the `IncomeByMonth` style (`planMonthExpr`, `sqliteDatetime`
  bounds, float-vs-NUMERIC scan split), merged per (month, currency):
  - out: `t.type = 2 AND t.account_id IN (set) AND t.account_recipient_id NOT IN (set)`,
    `SUM(t.amount)`, `JOIN accounts a ON t.account_id = a.id` for the currency;
  - in: `t.type = 2 AND t.account_recipient_id IN (set) AND t.account_id NOT IN (set)`,
    `SUM(t.amount_recipient)`, joined on the recipient account.
  - Empty `accountIDs` short-circuits to `nil` (the PostgreSQL empty-`IN` guard).
- `BudgetPlanResult.Transfers []PlanMonthTransfersResult` —
  `{period, items: [{currencyId, in, out}]}`, exactly one entry per window
  month in month order (`items` is `[]`, never null, when nothing moved).
  Items are ordered budget currency first, then by currency id, so the wire
  is deterministic across engines.
- MCP `budget_plan` reuses the result unchanged.

### `get-transaction-list`

- `BudgetTransactionListRequest.Transfers bool \`json:"transfers,omitempty"\``
  (query `transfers=1`, parsed like `uncategorized`).
- Mutually exclusive with `categoryId`, `tagId`, `envelopeId`, `labelId`,
  `uncategorized` — any combination is rejected with
  `CodeBudgetTransactionFilterRequired`, exactly like `labelId+tagId`.
- `ReadModel.BudgetTransactionsTransfers(ctx, accountIDs, start, end) ([]model.BudgetTransactionRow, error)`
  — type=2 rows in `[start, end)` with exactly one side included, newest
  first (same ordering as the other lists). `Amount` and `CurrencyID` come
  from the included side; the row carries `Direction` (`"in"`/`"out"`).
- `BudgetTransactionRow` and `BudgetTransactionResult` gain
  `Direction string` (`json:"direction,omitempty"`), set only on transfer
  rows — the existing lists' bytes are unchanged.

### Tests / artefacts

- Repo tests: `TransfersByMonth` (in, out, cross-currency counted,
  both-included cancels, neither-included ignored, first-of-month boundary
  row kept, empty set → nil) and `BudgetTransactionsTransfers` (direction,
  included-side amount/currency, ordering).
- `plan_test.go`: the `transfers` block on the wire; `txlist_test.go`: the
  selector and its exclusivity.
- Regenerate `apiparity`/`mcpparity` goldens (`budget_plan` gains
  `transfers`; the transaction-list goldens are unchanged unless a scenario
  is added) and the OpenAPI docs; inspect the diffs.

## Frontend

- DTOs: `BudgetPlanDto.transfers`, `BudgetTransactionDto.direction?`,
  `BudgetTransactionsParams.transfers?` (+ `getBudgetTransactions` sends
  `transfers=1`).
- `planMath.planTotals`: `transfersIn`, `transfersOut`, `transfersNet` per
  month (converted per currency with `ex`); `effectiveNet` and `netActual`
  add `transfersNet`. `balanceRow` is unchanged (it already runs on
  `effectiveNet`).
- `PlanSheet`: `TOTALS_ROWS` gains `transfers` before `net`; a `link` spec
  field marks Uncategorized and Transfers as drill-down cells; the sheet
  owns `transactionsTarget: { target, month } | null` and renders one
  `BudgetTransactionsDialog` with `periodStart={month}`.
- `BudgetTransactionsDialog`: optional `periodStart` prop (default: the
  period store's `selectedDate`); `BudgetTransactionsTarget.type` gains the
  client-only discriminant `'transfers'` (like `'label'`) → params
  `{ transfers: true }`; row amount sign by `direction`; the synthesized
  read-only fallback uses `type: 'transfer'` for transfer rows.
- Locales (`en`/`ru`): `budgets.page.plan.totals.transfers`,
  `budgets.page.plan.totals.transfers_tooltip` (`{in}`, `{out}`),
  `budgets.page.plan.totals.show_transactions`, and the dialog title reuses
  `budgets.page.plan.totals.transfers`.
- No new `METRICS` key: the drill-down is the existing transaction-list
  view, not a new user action.
- Tests: `planMath.test.ts` (transfers totals + Net/Balance folding),
  `PlanSheet.test.tsx` (line renders, link opens the dialog for the clicked
  month, zero is not a link), fixture updates for the new DTO field.

## Out of scope

- A "planned" value for transfers.
- Exchange gain/loss between two included accounts of different currencies.
- Widening the uncategorized list to income rows.
