# Budget savings: savings accounts and planned savings

**Date:** 2026-09-12 · **Revised:** 2026-09-22 (design review — see "Revisions")
**Branch:** `feature/budget-savings` · **Order:** ships **after** budget cell
comments (#246); see "Seam with budget cell comments".

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
| Removal | A savings row whose account leaves the budget or stops being savings is removed, with its plans, on the next element sync (same as categories). *(2026-09-22)* The SPA confirms before turning the flag off when plans would be lost. |
| Comments (#246) | *(2026-09-22)* Savings cells carry comment threads like any other cell; this PR adds the marker and entry point in the Savings section. |
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

- Add `TypeSavings AccountType = 3`; `Valid()` accepts 1, 2, 3. `Valid()` gates
  *writes* only — reads must keep tolerating whatever is already in the column.
  `NewAccount` has always written 2 and nothing has ever written anything else, but
  `data:import-sqlite` copies the column verbatim from a foreign database, so a
  hydrating repo read must never reject an unknown type: it maps through unchanged
  and the UI treats "not 3" as everyday. Add a repo test that loads an account row
  with `type = 0` and gets it back intact.
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
  `change-element-currency` works unchanged. The actual-savings query returns
  amounts in the **savings account's** currency, which is not necessarily the
  element's: savings amounts therefore go through the same single `bulkConvert`
  pass every other element uses (`builder_structure_build.go:76-224`,
  `addSpendingConvert`) — account currency → element currency for the row, element
  currency → budget currency for the section total. No second conversion path.

### Element sync (`internal/budget/move.go` `syncElements`)

After tags, for every savings account among the budget's member accounts
(deleted included): `ensure(accountID, ElementSavings, account.currencyId)`,
marked live (not archived, `folder_id` NULL). Any `ElementSavings` row whose
account is no longer a member or no longer `type = 3` is not seen and is
deleted by the existing unseen-row deletion (cascading its limits).

The element row's own archived flag is therefore always false and is **not** the
source of the wire's `isArchived`: readers derive that from the account's
`is_deleted` at build time (see the wire section). One source of truth — nothing
ever writes `is_archived = 1` on a savings element.

Deletion here is the same rule categories live under, but the trigger is different:
a category leaves a budget through a deliberate multi-step action, whereas savings
is a switch in the account dialog, and the deletion lands later, lazily, on whatever
budget write happens next. The backend rule stands (consistency, no dead rows), and
the SPA carries the warning — see "Account dialog" below.

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
  "isArchived": 0,               // derived: 1 when the account is deleted
  "position": 0,                 // dense within the savings section
  "budgeted": "500.00",          // this month's planned (0 when none)
  "spent": "300.00",             // actual savings this month, converted to element currency
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

**`savingsFlows` deliberately exceeds the Savings row.** The Savings row counts only
what the user *moved in* (everyday↔savings transfers); the balance moves with
everything that happens to the account, interest included. So "Saved 500" this month
alongside a savings balance that rose 512 is correct, not a rounding bug — and it
will be read as a bug unless the UI says otherwise. The Savings balance row gets an
info tooltip ("Includes interest and other activity on savings accounts, which is
not counted as saved"), and the regression plan gets an item asserting the
divergence is intended.

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
- **Turning the switch off is confirmed** when it would destroy plans: on save, if
  the account was savings and is not any more, the dialog asks
  ("Planned savings for this account will be removed from {n} budget(s). Saved
  amounts and transactions are not affected."). The check uses data the SPA already
  holds — the budgets the user can see, their `structure.savings` rows, and whether
  any carries a plan — so it needs no new endpoint; when no visible budget plans
  savings for the account, there is no dialog. A participant's plans in a budget the
  actor cannot see are not counted; that is acceptable (the actor owns the account)
  and is why the confirmation text says "will be removed" without a precise total.
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
- Savings cells carry **comment threads** like every other cell (#246, already
  shipped): the same corner-triangle marker, the same `CommentThread` in the
  `LimitEditor` popover footer, the same Shift+Enter. No backend work — a savings
  element is an element and its external id is the account id. See "Seam with budget
  cell comments".
- Totals (`planMath.ts`), per month, all in budget currency:
  - **Savings** row (below Transfers): actual for past months;
    max(actual, planned) for current and future months ("effective").
  - **Net** = Income − Expenses + Transfers − Savings (actual and effective);
    planned Net = planned Income − planned Expenses − planned Savings.
  - **Savings balance** = Σ `savingsOpeningBalances` + running sum of:
    past months → `savingsFlows`; current month → `savingsFlows` +
    max(0, planned savings − actual savings); future months → planned savings.
  - **Balance** (everyday) = today's combined balance (unchanged computation)
    − Savings balance, so the two rows always sum to the former total. The Savings
    balance row carries the "includes interest and other activity" tooltip above.

### Monthly Budget view (`BudgetPage.tsx`, `budgetMath.ts`)

- Collapsible **Savings** block below the budget table: per account Planned /
  Saved / Remaining; Remaining < 0 uses the over-plan style. Planned edits
  inline the way limits are. Drag reorders within the block only (separate
  `DndContext`, so no drop onto folders).
- Planned cells in the block carry the #246 comment marker and entry point, exactly
  as budgeted cells do in the table above.
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
  - Currency: a savings account in a currency other than the element's converts
    through the shared `bulkConvert` pass — one conversion, element currency on the
    row, budget currency in the section total.
  - Account read tolerance: a stored `accounts.type` outside {1,2,3} round-trips
    through the repo unchanged and reads as everyday.
- **Parity**: new apiparity scenario `budget_savings` (mark savings, add to
  budget, set-limit, transfers, get-budget, get-budget-plan, move-element
  errors); regenerate and inspect budget/account goldens; mcpparity goldens;
  `make test-repo-pgsql` and `enginecompare` pass.
- **SPA (vitest)**: `planMath` savings row, net, balance split; PlanSheet
  savings section render/fold/reorder; BudgetPage savings block; AccountDialog
  switch payloads; metrics coverage. Plus: the switch-off confirmation appears when
  a visible budget plans savings for the account and is skipped when none does
  (both branches), and a savings cell renders the comment marker and opens the
  thread.
- **Docs**: `docs/regression-test-plan.md` — savings toggle (including the
  switch-off confirmation and the plans it removes), plan section and totals,
  monthly block and widget line, reorder/no-folder, transfer shows as saved, clone
  keeps plans, a comment thread on a savings cell, and an item asserting that a
  Savings balance moving by more than the Saved figure (interest) is expected
  behavior (with 📱 markers where applicable).

## Seam with budget cell comments (#246)

Comments ship first and this branch rebases onto them. The two features are
otherwise independent; these are the agreed terms.

- **Savings cells carry threads.** `budgets_elements_comments.element_id` is an
  element FK, so a savings element takes comments with **no backend change** —
  no new route, no permission work, no lifecycle work (the cascade from
  `budgets_elements` already covers a savings row removed by `syncElements`).
- **This branch owns the savings-section UI**: marker, popover footer entry point,
  and the compact-dialog section inside the new Savings section, reusing #246's
  `CommentThread` and marker components as they are.
- **Rebase surfaces** (all additive, all on this side): `internal/model/
  budget_dto.go`, `internal/shared/errs/codes.go` + `AllCodes`, all 11
  `locales/<lang>.json`, `web/src/lib/metrics.ts`, `PlanSheet.tsx`
  (`buildFlatRows` + cell rendering), `BudgetPage.tsx`, the apiparity/mcpparity
  goldens, and `docs/regression-test-plan.md`. Regenerate goldens after the rebase,
  never before, and inspect the diff.

## Revisions

**2026-09-22 — design review (#245 and #246 reviewed together).**

1. **Savings amounts convert through the existing `bulkConvert` pass.** The spec
   computed actuals in the account's currency and reported them as element currency
   with no conversion step named.
2. **`isArchived` has one source of truth** — derived from the account's
   `is_deleted`; the element's own flag is never written.
3. **Turning the savings switch off is confirmed in the SPA** when planned amounts
   would be destroyed. The backend keeps the existing (silent, lazy) sync deletion.
4. **`savingsFlows` vs the Savings row**: the intended divergence (interest is not
   "saved") is now stated, with a tooltip and a regression item, instead of looking
   like a bug.
5. **Legacy `accounts.type` values** must survive a repo round trip; `Valid()` gates
   writes only.
6. **Seam with #246** recorded: savings cells carry comment threads, this branch
   owns the savings-section UI and the rebase.

## Status and next steps (2026-09-23)

**SPEC ONLY — nothing is implemented.** PR #245 carries this document and no code.
The spec was revised on 2026-09-22 after a design review (see Revisions above); that
revision is head `e1c8527` on `feature/budget-savings`.

**This feature is deliberately sequenced SECOND, after budget cell comments (#246).**
#246 is implemented and ready for review; see "Seam with budget cell comments (#246)"
above for the agreed terms. This branch rebases onto #246, never the reverse.

### Next steps, in order

1. **Wait for #246 to merge**, then `git fetch origin main` and rebase this branch
   onto it. The branch currently holds two doc commits off an older `main`.
2. **Write the implementation plan** with the `superpowers:writing-plans` skill, to
   `docs/superpowers/plans/YYYY-MM-DD-budget-savings.md`. Use
   `docs/superpowers/plans/2026-09-22-budget-cell-comments.md` as the model — that
   plan's shape (Global Constraints, a Review Focus list, then bite-sized TDD tasks
   with real code in every step) worked well through 11 tasks, and its backend tasks
   map almost one-to-one onto this feature's: model/migration, persistence with the
   engine-adapter split, read use case, write use cases, lifecycle, REST + apiparity,
   MCP + mcpparity, then the SPA layers.
3. **Implement** with `superpowers:subagent-driven-development`.

### Decisions already settled — carry them into the plan

- `accounts.type = 3` (`TypeSavings`). `Valid()` gates WRITES only; a stored value
  outside {1,2,3} must survive a repo round trip (`data:import-sqlite` copies the
  column verbatim from a foreign database).
- `ElementSavings ElementType = 5`, alias `"savings"`. 3 and 4 are already taken by
  `ElementIncomeCategory`/`ElementIncomeEnvelope`, so 5 is correct and frozen once
  written.
- Savings amounts go through the SAME single `bulkConvert` pass every other element
  uses — the actual-savings query returns the savings ACCOUNT's currency, which is
  not necessarily the element's.
- `isArchived` on the wire is derived from the account's `is_deleted` at build time;
  the element row's own flag is never written.
- Turning the savings switch OFF is confirmed in the SPA when planned amounts would
  be destroyed. The backend keeps its existing silent, lazy sync deletion.
- `savingsFlows` deliberately exceeds the Savings row (interest is not "saved"); this
  needs the tooltip and the regression item, or it reads as a bug.
- Savings cells carry comment threads. Backend-free — a savings element is a real
  `budgets_elements` row and `budgets_elements_comments.element_id` is an element FK.
  THIS branch owns the marker and entry point inside the new Savings section,
  reusing #246's `CommentThread` and marker components as they are.

### Rebase surfaces to expect (all additive, all on this side)

`internal/model/budget_dto.go`, `internal/shared/errs/codes.go` + `AllCodes`, all 11
`locales/<lang>.json`, `web/src/lib/metrics.ts`, `PlanSheet.tsx` (`buildFlatRows` +
cell rendering), `BudgetPage.tsx`, the apiparity/mcpparity goldens, and
`docs/regression-test-plan.md`. Regenerate goldens AFTER the rebase, never before,
and inspect the diff.

### Environment notes for a new session

- Go is **not on `PATH`**: `export PATH=/usr/local/go/bin:$PATH` first.
- `pnpm test -- <pattern>` does NOT filter by filename in this repo — it silently
  runs all ~1280 tests. Use `pnpm exec vitest run <path>`.
- There is one pre-existing unrelated vitest failure on `main`,
  `web/src/api/transaction.test.ts`'s Blob test. It is not yours.
- Guard floors move: read the CURRENT `minRoutes` in
  `internal/test/apiparity/guard_test.go` and `min` in `catalogue_test.go` from the
  files and add to them; never trust a literal quoted in a plan.
- A `;` on its own line in a sqlc query file silently truncates the generated SQL
  constant, and a multibyte character in a `.sql` comment does the same.
