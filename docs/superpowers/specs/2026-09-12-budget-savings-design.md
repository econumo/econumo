# Budget savings: savings accounts and planned savings

**Date:** 2026-09-12 · **Branch:** `feature/budget-savings`

## Problem

A user holds savings accounts (TFSA, RRSP, …) inside a budget and wants to
plan how much goes into each of them every month, then see how much was
actually saved.

Today there is no way to do this. Budget math counts an explicit set of member
accounts (`budgets_accounts`), and a transfer between two member accounts nets
out of every figure: moving 500 from chequing into a TFSA that is also a member
shows up nowhere — not as spending, not as a transfer, not as anything that can
be planned. Accounts carry an `accounts.type` column (`TypeCash=1`,
`TypeCreditCard=2`, `internal/model/account.go:22-35`) that no code path or UI
reads.

## Decisions (agreed during brainstorming)

| Question | Decision |
|---|---|
| What is "actual saved"? | **Net money moved in**: transfers from the budget's everyday member accounts into a savings account, minus withdrawals back to them. Interest/gains are not "saved". |
| Granularity | **One row per savings account** that is a member of the budget. |
| Effect on totals | **Savings is an outflow**: Net = Income − Expenses + Transfers − Savings; balance splits into everyday **Balance** and **Savings balance**. |
| Views | **Both** the Plan view (`/plan`) and the monthly Budget view. |
| Account flag | `accounts.type = 3` (`TypeSavings`) — reuse the existing column, no migration. |
| Plan storage | New budget element type `ElementSavings = 5` (`external_id` = account id); planned amounts in `budgets_elements_limits` via the existing `set-limit`. |
| Layout | Savings rows live in their **own collapsible section at the bottom**; they **cannot be put into folders**, only reordered. |
| Carry-over | None: monthly Remaining = this month's planned − this month's saved. |
| Removal | A savings row whose account leaves the budget or stops being savings is removed, with its plans, on the next element sync (same as categories). |
| Out of scope (v1) | Drill-down from a savings row into its transactions. |

## Definitions

- **Member accounts**: `budgets_accounts` of every participant — the existing
  `filters.includedAccountIDs` (`internal/budget/builder.go:117-199`), deleted
  accounts included.
- **Savings accounts** (of a budget): member accounts with `type = 3`.
- **Everyday accounts**: member accounts with `type ≠ 3`.
- **Actual savings** of savings account *S* in month *M* (in *S*'s currency):

  ```
  + Σ amount_recipient  of type=2 transactions in M with account_id ∈ everyday, account_recipient_id = S
  − Σ amount            of type=2 transactions in M with account_id = S, account_recipient_id ∈ everyday
  ```

  Transfers savings↔savings and transfers between *S* and non-member accounts
  do not count as savings (the latter stay in the existing boundary
  `transfers`/`holdings` figures). Income/expense booked directly on *S*
  (interest, fees, deletion corrections) stays in its category rows.

## Backend

### Accounts (`internal/model/account.go`, `internal/account`)

- Add `TypeSavings AccountType = 3`; `Valid()` accepts 1, 2, 3.
- `CreateAccountRequest` and `UpdateAccountRequest`
  (`internal/model/account_dto.go`) gain optional `type *int`. `Validate()`
  rejects values outside {1,2,3} with a coded field error on `type`
  (new code `account.invalid_type`, catalogued in every locale).
  - Create: absent → `TypeCreditCard` (today's `NewAccount` default).
  - Update: absent → unchanged. A new mutator `UpdateType(AccountType)` on the
    entity.
- Permission to change the type is exactly the permission to update the
  account today.
- The repo already writes `type` on upsert (`repo/repo.go:152`,
  `accounts.sql` `UpsertAccount`) — no query or migration change.
- `AccountResult.type` is already on the wire.
- MCP (`internal/account/mcp`): `create_account` and `update_account` inputs
  gain optional `type` with the same semantics.

### Budget element type

- `model.ElementSavings ElementType = 5`, alias `"savings"` (append to
  `elementAliases`). `ElementTypeFromAlias` keeps accepting only
  envelope/category/tag (drill-down stays out of scope). `IsIncomeSide()` is
  false.
- Element currency defaults to the account's currency;
  `change-element-currency` works unchanged.

### Element sync (`internal/budget/move.go` `syncElements`)

After tags, for every savings account among the budget's member accounts
(deleted included): `ensure(accountID, ElementSavings, account.currencyId)`,
marked live (not archived, `folder_id` NULL). Any `ElementSavings` row whose
account is no longer a member or no longer `type = 3` is not seen and is
deleted by the existing unseen-row deletion (cascading its limits).

The account type and currency needed here come through the budget feature's
existing account lookup port (the one `builder.go` uses for member accounts);
extend its result with `Type` if it does not already carry it. No new
cross-feature import.

Sync remains lazy — it runs on budget writes (`move-element`, envelope writes,
and the `set-limit`/`change-element-currency` self-heal), never on reads.
Readers therefore derive savings rows from the member savings accounts and
use the element row only for position, currency and limits (see builders).

`assignMissingKeys` must key savings elements in their **own ordering group**
(no-folder group is split: savings vs everything else) so their sort keys
never interleave meaningfully with non-folder expense elements.

### `move-element` (`internal/budget/move.go` `MoveElement`)

- When the resolved element is `ElementSavings` and `folderId` is non-null →
  coded validation error `budget.savings_folder_not_allowed` (HTTP 400, field
  `folderId`, catalogued in every locale).
- `groupElements` for a savings element groups only `ElementSavings` siblings;
  `afterId` is resolved within that group (an `afterId` from outside the group
  behaves like an unknown `afterId` does today).
- For non-savings elements, the no-folder group excludes savings elements.

### `set-limit`, archive, end month, clone

Unchanged. `set-limit` resolves the element by external id with self-heal, and
applies the archived/start/end-month guards generically.
`clone-budget` copies elements generically (external id kept for non-envelope
types) and copies membership, so savings rows and — with `WithLimits` — their
plans carry over. Add a test asserting it.

### Repository (`internal/budget/repo/read.go`)

New method on the read repository:

```go
SavingsByMonth(ctx, savingsIDs, everydayIDs []string, from, to time.Time) ([]SavingsMonthRow, error)
// SavingsMonthRow{ AccountID, Month (first-of-month), Amount vo.DecimalNumber }
```

Shape mirrors `transfersByMonthSQL` (`read.go:811-836`), one query per
direction, merged by (account, month):

```sql
-- in
SELECT <month(t.spent_at)> AS month, t.account_recipient_id AS account_id, SUM(t.amount_recipient) AS amount
FROM transactions t
WHERE t.type = 2 AND t.account_recipient_id IN (<savings>) AND t.account_id IN (<everyday>)
  AND t.spent_at >= ? AND t.spent_at < ?
GROUP BY month, t.account_recipient_id
-- out: account_id IN (<savings>), account_recipient_id IN (<everyday>), SUM(t.amount), negated on merge
```

Hand-built dynamic SQL with per-engine month expression and value handling,
exactly like the existing plan queries. Empty `savingsIDs` or `everydayIDs`
short-circuits to no rows.

Also needed: per-account opening balances split by kind. The existing
`AccountsBalancesBeforeDate` already returns per-account rows; the builders
split them by account type in Go.

A second new method provides the per-month net change of the savings accounts
(for `savingsFlows`):

```go
AccountsNetByMonth(ctx, accountIDs []string, from, to time.Time) ([]SavingsMonthRow, error)
```

Per account per month: income − expense + incoming transfers
(`amount_recipient`) − outgoing transfers (`amount`), all counterparties — the
same per-transaction sign rules as `balanceSQL` (`read.go:65-104`), bucketed
by month. The plan builder aggregates it per account currency.

### Wire: monthly `get-budget`

`structure` gains `savings: []SavingsElementResult` (always present, `[]` when
none). Kept out of `elements` on purpose: older SPA/mobile builds ignore an
unknown field, whereas a new element type inside `elements` would be summed
into their expense totals. No `minAppVersion` bump needed.

```jsonc
{
  "id": "<account id>", "type": 5, "name": "<account name>", "icon": "<account icon>",
  "currencyId": "<element currency>", "ownerUserId": "<account owner>",
  "isArchived": 0,               // 1 when the account is deleted
  "position": 0,                 // dense within the savings section
  "budgeted": "500.00",          // this month's planned (0 when none)
  "spent": "300.00",             // actual savings this month, element currency
  "available": "200.00"          // budgeted − spent (no carry-over)
}
```

Visibility: shown when the account is not deleted, or when it has a plan or
non-zero actual in the period. Amount formatting follows existing element
decimals. `buildFinancialSummary` is unchanged.

### Wire: `get-budget-plan`

- `structure.savings: []PlanSavingsElementResult` — same identity fields as
  above plus `cells: [{actual, planned}]` per month (planned `""` when no
  limit, same convention as element cells). Visibility: non-deleted accounts
  always; deleted ones only with a plan or actual in the window.
- `savingsOpeningBalances: [{currencyId, amount}]` — opening balance (before
  `from`) of the savings accounts, per currency, budget currency first; shape
  mirrors `openingBalances`.
- `savingsFlows: [{month, currencyId, amount}]` — per month and currency, the
  savings accounts' actual net change (savings transfers + income/expense +
  boundary transfers booked on them); shape mirrors `transfers`.

### MCP (`internal/budget/mcp`)

`get_budget` / `get_budget_plan` return the new fields automatically.
Update the `set_limit` and `move_element` descriptions to mention savings rows
(id = account id, no folder).

## SPA (`web/`)

### DTOs

- `AccountType.SAVINGS = 3` (`web/src/api/dto/account.ts`).
- `BudgetElementType.SAVINGS = 5` (`web/src/api/dto/budget.ts`).
- Budget/plan structure types gain `savings`, `savingsOpeningBalances`,
  `savingsFlows`; create/update account payloads gain optional `type`.

### Account dialog and lists

- `AccountDialog.tsx`: "Savings account" switch. Create sends `type: 3` when on,
  `2` when off. Update sends `3` when on; when off, the account's previous type
  if it was 1 or 2, else `2`.
- Small "Savings" marker on savings accounts in `SidebarAccountTree.tsx` and
  `AccountsSettingsPage.tsx`.

### Plan view (`PlanSheet.tsx`, `planMath.ts`, `budgetStore.ts`)

- New collapsible **Savings** section rendered after Expenses and before
  Archived; fold key `planFolds['savings']` (persisted like the others).
  Included in `buildFlatRows` in display order so keyboard navigation works.
- One row per `structure.savings` entry; no folder header, no "new folder"
  affordance. Own `DndContext` band: drag reorders within the section via
  `useMoveElement` with `folderId: null`. Planned cells edit via the existing
  `set-limit` cell editor. Section total row in budget currency.
- Totals (`planMath.ts`), per month, all in budget currency:
  - **Savings** row (below Transfers): actual for past months;
    max(actual, planned) for current and future months ("effective").
  - **Net** = Income − Expenses + Transfers − Savings (actual and effective);
    planned Net = planned Income − planned Expenses − planned Savings.
  - **Savings balance** = Σ `savingsOpeningBalances` + running sum of:
    past months → `savingsFlows`; current month → `savingsFlows` +
    max(0, planned savings − actual savings); future months → planned savings.
  - **Balance** (everyday) = today's combined balance (unchanged computation)
    − Savings balance, so the two rows always sum to the former total.

### Monthly Budget view (`BudgetPage.tsx`, `budgetMath.ts`)

- Collapsible **Savings** block below the budget table: per account Planned /
  Saved / Remaining; Remaining < 0 uses the over-plan style. Planned edits
  inline the way limits are. Drag reorders within the block only (separate
  `DndContext`, so no drop onto folders).
- `budgetTotals` / `bucketElements` untouched (savings are not in `elements`).
- `ExpenseWidget` secondary line "Saved X of Y planned" (budget currency),
  shown only when `structure.savings` is non-empty.

### Analytics

- `METRICS.appAccountSavingsToggle`, fired in the create/update account
  mutations' `onSuccess` when the resulting savings state differs from the
  previous one (create: when created as savings).
- Planned-savings edits are covered by the existing set-limit event at its
  shared hook.

### i18n

All 11 catalogues: savings switch label/hint, "Savings" marker, plan section
title, `budgets.page.plan.totals.savings`, `…savingsBalance`, monthly block
labels (Planned / Saved / Remaining, "Saved {saved} of {planned} planned"),
`errors.account.invalid_type`, `errors.budget.savings_folder_not_allowed`.

## Testing

- **Go unit/integration**
  - Account: type validation on create/update; absent type defaults/keeps;
    type persisted and returned.
  - Sync: savings element created for a member savings account; removed (with
    limits) when the account leaves the budget or its type changes.
  - Move: folder rejected with `budget.savings_folder_not_allowed`; reorder
    within savings group; non-savings no-folder ordering unaffected.
  - `SavingsByMonth`: everyday→savings counts; savings→everyday subtracts;
    savings↔savings ignored; non-member→savings ignored; cross-currency uses
    `amount_recipient`/`amount` in the savings account's currency; month
    bucketing at boundaries.
  - Monthly builder: budgeted/spent/available, deleted-account visibility.
  - Plan builder: cells, `savingsOpeningBalances`, `savingsFlows`.
  - Clone keeps savings rows and (with limits) plans.
- **Parity**: new apiparity scenario `budget_savings` (mark savings, add to
  budget, set-limit, transfers, get-budget, get-budget-plan, move-element
  errors); regenerate and inspect budget/account goldens; mcpparity goldens;
  `make test-repo-pgsql` and `enginecompare` pass.
- **SPA (vitest)**: `planMath` savings row, net, balance split; PlanSheet
  savings section render/fold/reorder; BudgetPage savings block; AccountDialog
  switch payloads; metrics coverage.
- **Docs**: `docs/regression-test-plan.md` — savings toggle, plan section and
  totals, monthly block and widget line, reorder/no-folder, transfer shows as
  saved, clone keeps plans (with 📱 markers where applicable).
