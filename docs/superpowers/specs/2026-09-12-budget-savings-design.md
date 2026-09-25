# Budget savings: savings accounts and planned savings

**Date:** 2026-09-12 · **Revised:** 2026-09-22 (design review), 2026-09-25 (savings
becomes a per-budget membership flag) — see "Revisions"
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
be planned.

## Decisions (agreed during brainstorming)

| Question | Decision |
|---|---|
| What is "actual saved"? | **Net money moved in**: transfers from the budget's everyday member accounts into a savings account, minus withdrawals back to them. Interest/gains are not "saved". |
| Granularity | **One row per savings account** that is a member of the budget. |
| Effect on totals | **Savings is an outflow**: Net = Income − Expenses + Transfers − Savings; balance splits into everyday **Balance** and **Savings balance**. |
| Views | **Both** the Plan view (`/plan`) and the monthly Budget view. |
| Savings flag | *(2026-09-25)* **Per budget, on the membership**: `budgets_accounts.is_savings`. The same account can be savings in one budget and everyday in another. Set by the account's owner (who must be able to update the budget), at any time, from the budget's settings. Accounts carry no savings marker. |
| Plan storage | New budget element type `ElementSavings = 5` (`external_id` = account id); planned amounts in `budgets_elements_limits` via the existing `set-limit`. |
| Layout | Savings rows live in their **own collapsible section at the bottom**; they **cannot be put into folders**, only reordered. |
| Carry-over | None: monthly Remaining = this month's planned − this month's saved. |
| Removal | *(2026-09-25)* Turning the flag off, or removing a savings member from the budget, deletes its savings row with its plans and comment threads **in the same write**. When the row carries any plan or comment, the server refuses the write with `budget.savings_removal_unconfirmed` unless the request confirms it; the SPA asks and resends. |
| Visibility | *(2026-09-25)* **Accepted and documented**: every participant of the budget, guests included, sees each savings row's account name, icon and amounts, and the savings balances. The budget settings say so next to the savings toggle. |
| Comments (#246) | *(2026-09-22)* Savings cells carry comment threads like any other cell; this PR adds the marker and entry point in the Savings section. |
| Out of scope (v1) | Drill-down from a savings row into its transactions. A real credit-card account type (balance shown as debt, credit limit) is a separate feature: `accounts.type` stays untouched here. |

## Definitions

- **Member accounts**: `budgets_accounts` of every participant — the existing
  `filters.includedAccountIDs` (`internal/budget/builder.go:117-199`), deleted
  accounts included.
- **Savings accounts** (of a budget): member accounts whose `budgets_accounts.is_savings` is set.
- **Everyday accounts**: every other member account.
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

### Savings flag on budget membership (`budgets_accounts`)

*(2026-09-25: replaces the `accounts.type = 3` design; see Revisions.)*

- Migration (both engines, same version): `budgets_accounts.is_savings BOOLEAN NOT
  NULL DEFAULT false`. Existing rows read as everyday. `model.BudgetAccount` gains
  `IsSavings`; `MemberAccounts` returns it; `AddAccount` takes it; a new repo
  method sets it on an existing member.
- **Who**: the account's owner, when they can update the budget — the rule every
  membership write already follows (`membershipPrelude`). Archived budgets refuse it
  like every other budget write.
- **Write paths**:
  - `create-budget`: optional `savingsAccountIds []string`, each one of the request's
    `accountIds` (else coded 400 `budget.savings_account_not_member`, field
    `savingsAccountIds`).
  - `update-budget`: optional `savingsAccountIds` — absent = untouched; present =
    replace-set over the caller's OWN member accounts after `accountIds` is applied
    (same semantics as `accountIds`). An id that is not then one of the caller's
    member accounts → `budget.savings_account_not_member`.
  - `add-account`: optional `isSavings` — absent leaves an existing member's flag
    alone and adds a new member as everyday; present sets it.
  - `remove-account`: unchanged, except for the confirmation guard below.
- **Confirmation guard**: a write that turns a member's flag off or removes a savings
  member, while that member's savings element carries at least one limit or comment,
  is refused with coded 400 `budget.savings_removal_unconfirmed` (field
  `savingsAccountIds`, params `{count}` = affected accounts) unless the request
  carries `confirmSavingsRemoval: true` (`update-budget`, `add-account`,
  `remove-account`). The check runs on the server, so it holds whatever the client
  has loaded. Nothing is written when it refuses.
- Every write above runs `syncElements` in its own transaction, so a savings row
  appears or disappears together with the flag (with its limits and #246 comments,
  by cascade) — no lazy deletion is left waiting for an unrelated write.
- `filters.accounts` entries (the requester's own member accounts) gain
  `isSavings` (bool, like `removable`), which the settings dialog round-trips.
- `clone-budget` copies the flag with membership; `accept-access` seeds new
  members as everyday.
- MCP: `add_budget_account` gains `is_savings` and `confirm_savings_removal`;
  `remove_budget_account` gains `confirm_savings_removal`.

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

After tags, for every savings member of the budget (deleted accounts included):
`ensure(accountID, ElementSavings, account.currencyId)`, marked live (not archived,
`folder_id` NULL). Any `ElementSavings` row whose account is no longer a member or
no longer flagged is not seen and is deleted by the existing unseen-row deletion
(cascading its limits and comments).

The element row's own archived flag is therefore always false and is **not** the
source of the wire's `isArchived`: readers derive that from the account's
`is_deleted` at build time (see the wire section). One source of truth — nothing
ever writes `is_archived = 1` on a savings element.

*(2026-09-25)* The flag and membership writes run this sync themselves (see the
flag section), behind the server-side confirmation guard. The account currency and
name come through the budget feature's existing account lookup port; the flag
comes from the budget's own membership rows. No new cross-feature import.

Sync otherwise stays lazy for the rest of the budget (it runs on budget writes,
never on reads). Readers derive savings rows from the flagged members and use the
element row only for position, currency and limits (see builders), so a row whose
element was not synced yet still renders.

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

- `BudgetElementType.SAVINGS = 5` (`web/src/api/dto/budget.ts`).
- Budget/plan structure types gain `savings`, `savingsOpeningBalances`,
  `savingsFlows`; `filters.accounts` entries gain `isSavings`; create/update
  budget payloads gain optional `savingsAccountIds` and `confirmSavingsRemoval`.

### Budget settings (`BudgetUpdateDialog`, `BudgetDialog`, `BudgetAccountsField`)

*(2026-09-25: replaces the account-dialog switch, its cache-based confirmation and
the account-level marker.)*

- `BudgetAccountsField` (shared by the create and update budget dialogs): each
  selected own account gets a "Savings" toggle, initialised from
  `filters.accounts[].isSavings`. Available at any time, including for members
  locked by the removal rule. Next to the toggles, the note: "Savings accounts are
  shown by name, with their saved amounts and balances, to everyone with access to
  this budget."
- Create sends `savingsAccountIds`; update sends the full own savings set.
- When the server answers `budget.savings_removal_unconfirmed`, the dialog asks
  ("Planned amounts and comments for {count} savings account(s) will be deleted from
  this budget. Saved amounts and transactions are not affected.") and resends with
  `confirmSavingsRemoval: true`; cancelling leaves the dialog open, unchanged.
- No account-level switch, type field or marker.

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
    past months → `savingsFlows`; current month → `savingsFlows` + the per-row
    gap Σ(effective − actual) over the savings rows (never negative; a deleted
    account's row contributes 0, its effective being its actual); future months →
    the Savings row's effective total (= planned savings when nothing is booked, so
    a future-dated transfer counts exactly as the Savings row shows it). The gap is
    per row, never the aggregate max(0, planned − actual): one account over its plan
    must not offset another's shortfall, or the balance would move by less than the
    Savings row shows (planned 500 / saved 0 next to planned 0 / saved 300 shows
    Savings 800, and the balance must add 800, not 300 + 200).
  - **Balance** (everyday) = today's combined balance (unchanged computation)
    − Savings balance, so the two rows always sum to the former total. The Savings
    balance row carries the "includes interest and other activity" tooltip above.
  - *(2026-09-25)* The split shows whenever the plan carries savings data — any
    savings row, **or** any non-zero `savingsOpeningBalances` / `savingsFlows`
    amount. A deleted savings account with a balance but no plan or activity in the
    window renders no row, yet its money must not be counted as everyday.

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

- *(2026-09-25)* `METRICS.appBudgetSavingsToggle`, fired once in the create/update
  budget mutations' `onSuccess` when the request changed any savings flag
  (create: when any account is created as savings).
- Planned-savings edits are covered by the existing set-limit event at its
  shared hook.

### i18n

All 11 catalogues: the budget-settings savings toggle label and visibility note, the
removal confirmation, plan section title, `budgets.page.plan.totals.savings`,
`…savingsBalance`, monthly block labels (Planned / Saved / Remaining, "Saved
{saved} of {planned} planned"), `errors.budget.savings_folder_not_allowed`,
`errors.budget.savings_account_not_member`, `errors.budget.savings_removal_unconfirmed`.

## Testing

- **Go unit/integration**
  - Flag writes: create/update-budget and add-account set the flag; absent leaves it;
    a non-member id is refused; another user's account is refused; clone copies it.
  - Guard: turning a planned or commented savings member off, or removing it, is
    refused without `confirmSavingsRemoval` and writes nothing; with it, the row,
    its limits and its comments are gone after the same request; an unplanned
    member needs no confirmation.
  - Sync: savings element created for a flagged member; removed (with limits) when
    the account leaves the budget or the flag is turned off.
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
- **Parity**: new apiparity scenario `budget_savings` (flag savings through
  update-budget, set-limit, transfers, get-budget, get-budget-plan, move-element
  errors, the unconfirmed-removal refusal and the confirmed removal); regenerate and inspect budget/account goldens; mcpparity goldens;
  `make test-repo-pgsql` and `enginecompare` pass.
- **SPA (vitest)**: `planMath` savings row, net, balance split; PlanSheet
  savings section render/fold/reorder; BudgetPage savings block; the settings
  toggle payloads (create and update) and the removal confirmation round trip
  (confirm resends, cancel sends nothing more); the balance split with no rows but
  a savings opening balance; metrics coverage; a savings cell renders the comment
  marker and opens the thread.
- **Docs**: `docs/regression-test-plan.md` — the budget-settings savings toggle
  (including the visibility note, the removal confirmation and what it deletes), plan section and totals,
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

**2026-09-25 — review of the implementation (PR #245).**

7. **Savings is a per-budget membership flag, not an account type.** The account
   switch deleted plans in budgets the switch could not see: its confirmation only
   counted budgets already in the browser's cache, and the deletion landed later on
   an unrelated write. The flag now lives on `budgets_accounts`, is edited in the
   budget's own settings, deletes the row in the same write, and is guarded by a
   server-side confirmation. An account can be savings in one budget and everyday in
   another. `accounts.type` is left untouched (a credit-card type is its own
   feature).
8. **Visibility accepted and documented**: participants, guests included, see savings
   rows by account name with amounts; the settings say so beside the toggle.
9. **The balance split follows the savings data, not only the rows**: a deleted
   savings account with a balance but no row in the window still moves its money
   out of the everyday Balance.

## Status (2026-09-25)

**Revision 7 in progress.** The first implementation (account type `3`, account
dialog switch) landed on `feature/budget-savings` and was reviewed; revision 7 moves
the flag to budget membership. Everything below the flag — `ElementSavings`, the
read queries, both builders' wire fields, the plan and monthly UIs — carries over
unchanged.

### Deviations from the spec as written

1. **The plan view keeps `effectiveNet` as the combined-balance contribution only**;
   no separate "Net" row is rendered in the UI. Net = Income − Expenses + Transfers
   − Savings is spec math backing the balance split, not a row a caller sees.
