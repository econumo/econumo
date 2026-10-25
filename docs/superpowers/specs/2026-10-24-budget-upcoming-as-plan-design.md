# Upcoming recurring payments as a plan hint (revises #363)

Status: design, 2026-10-24. Revises `2026-10-22-budget-upcoming-recurring-design.md`
(merged as #363, not yet released).

## Why

After using #363, the user found that folding upcoming recurring payments into the spent /
actual figures of future months makes those numbers confusing: a future month looks as if
money had already been spent. The projection is still useful, but as input to the plan,
not as a fact.

The rule, in the user's words: **facts are about facts, the plan is about the plan.**

## Goals

1. **Facts are real money only, in every month.** Spent / actual, Available, the bars, the
   over-budget state, folder and total rows, the Plan's Balance and Total savings, savings
   closing balances and carry-over all go back to real transactions plus the plans the
   user set, as before #363.
2. **The spent list shows real transactions only.** The list opened from a spent / actual
   figure (`BudgetTransactionsDialog`) shows no upcoming recurring rows, and Post / Skip /
   Edit of a template is no longer offered there. Due payments are handled in the Inbox
   (Due section) and on the Recurring settings page, as before #363.
3. **Upcoming payments mark a plan that is too small**, in every month from the caller's
   current one onwards (revised after trying it, see below):
   - **No plan set** (planned is 0 / unset): the cell looks exactly as it did before this
     change. No suggested number is shown anywhere.
   - **Expense plan set but below what the month needs**: the plan number shows in **red**.
     What the month needs is `spent so far + upcoming still due`. In a future month spent
     is normally 0, so this is simply "plan < upcoming".
   - **Plan set and at least that**: shown normally.
4. **Editing a plan number lists what is still to come.** While the user edits the plan of
   a cell in the current month or later that has money spent or due, a quiet list (small,
   muted text, below the input) shows that month's still-due recurring payments (repeat
   icon, overdue marked) and the user's own transactions dated later in the month, merged
   by date, every amount in the cell currency in one right-aligned column. Below it: a
   "Spent" line when something was spent, then "Total" (`spent + still due`). A press
   anywhere on the list puts that total into the input (focus
   stays there; the grid's in-cell editor neither commits nor closes). Nothing shows on
   read-only cells, in past months, or when the list is empty.

"Upcoming" keeps #363's projection rules unchanged (server, `internal/budget`): every
template on the budget's member accounts, whoever owns it; in the current month, an
overdue next payment counts once plus the occurrences after today; later months get all
their occurrences; filed into cells exactly as the posted transaction would be. In the
current month, payments already posted are real and are in spent, not in upcoming.

## Revised after trying it (2026-10-24)

1. **Empty plans stay empty.** The grey italic suggestion is gone everywhere (Budget view
   desktop and phone, the item sheet, the Plan grid). Red on expense plans below
   `spent + still due` stays.
2. **The list shows only while a plan is being edited**, never as a hover on a read-only
   cell, and is visually quiet: `text-xs`, muted rows and lines, no bold.
3. **The list fills the editor** (first a "Use {amount}" button; after a second look the
   whole list is the button and the last line reads "Total") with `spent + still due` (the old suggestion), as a
   plain number like the editors start with, replacing the value and keeping focus. A
   "Spent" line sits above it when spent > 0, so the numbers add up. All figures are in
   the item's currency, aligned in one column. Tapping it fires `appBudgetPlanUseUpcoming`.
4. **The list includes the user's own future-dated transactions** of that cell (rows of the
   same `get-transaction-list` dated after today), merged with the recurring rows by date.
   They are already in spent, so Total stays `spent + still due`. Editors get the list for
   any cell in the current month or later with spent or upcoming money; past months never.
5. **Second round of polish (2026-10-25).** Every amount in the list carries a currency
   symbol; a row in another currency keeps its own amount and symbol, Spent/Total are in the
   item's currency. The line above Total is "Already spent / received / saved" by row kind and
   leaves out the future-dated rows listed above it, so the column adds up. "Coming up", that
   line and Total use the foreground colour. On the phone the list sits above the amount; in
   the Plan grid it opens like the multi-cell sum hint, under the cell's frame.
6. **The spent list leads with "Coming up"** (2026-10-25): the cell's future-dated real rows
   and the recurring payments still due, laid out like the editor's list (short date, name,
   repeat icon, aligned amounts), soonest first and display only; then a "Posted" heading
   over the past transactions in their day groups. Both headings appear only together. This replaces "Spent list: real transactions only" below; the
   spent figure itself still counts real money only.
7. **Future-dated transactions are not spent yet** (2026-10-25): a real transaction dated
   from the caller's tomorrow on (their timezone) has not happened, so no `get-budget` /
   `get-budget-plan` figure counts it: spent, available, label spent, received, saved,
   savings closing balances, opening balances, flows and transfers all stop at the end of
   the caller's today. It is upcoming money instead, filed exactly like a real row next to
   the due recurring payments (`upcoming` / `budgetUpcoming`, cell `upcoming`), so a row it
   alone carries still shows. The editor list's "Already …" line is the cell's spent as is,
   and Total stays spent + upcoming. The transaction list keeps the rows under "Coming up";
   `upcoming[]` on `get-transaction-list` stays recurring-only. This replaces "already in
   spent" in 4 and 5 and the "Future-dated real transactions count in spent" call below.

## Non-goals

- No change to how templates are scheduled, posted or skipped, and no posting from the
  Budget or Plan.
- No hint for months before the caller's current one (they have no upcoming money).
- Account page, transaction pages, Inbox, Recurring page: unchanged.

## Anti-goals

- Upcoming money must never move any total or projection.
- A spent / actual figure, or the list behind it, must never include money that does not
  exist yet.
- Nothing unsaved may look like a saved plan (hence no suggested number; the editor's
  list only fills the input).

## Decided with the user

- Future months: spent / actual real only (the combined grey figure from #363 is removed).
- No plan: upcoming total as the plan, grey italic, not in totals (superseded: see
  "Revised after trying it").
- Plan below what is needed: red; the popover with the planned transactions appears when
  editing the plan number, if there are any.
- Spent list: past / real transactions only; upcoming rows removed from it (superseded:
  see "Revised after trying it", 6).
- Current month: facts only for facts; the plan hint uses the payments still due (overdue
  next + later this month), red when `plan < spent + still due` ("let's try as you
  described").
- No-plan suggestion = `spent + still due` (550 for 300 spent + 250 due), so typing the
  suggested number never turns the cell red (now the list's Total).

## Calls made by me (check these)

- **Red is for expense cells only** (categories, tags, envelopes on the expense side, and
  their children). On income and savings rows, receiving or saving more than planned is
  not an overrun, so they get the editor list but never red.
- **Folder, tag-group and total rows show no red**; they only sum real
  plans. The hint lives only on rows that hold a plan; child categories under an envelope
  or a tag show nothing, their upcoming money appears on the parent.
- **The red rule uses the month's plan alone, not carry-over**, so the Budget view and the
  Plan grid share one rule (the grid has no carry-over). The existing red Available / bar
  in the Budget view stays as it is; the two can disagree when earlier months left money
  (red plan, positive Available), which reads as "this month's plan is short".
- **Savings rows**: their `upcoming` is a net change (deposits minus withdrawals). Savings
  never go red; the editor list shows deposits and withdrawals with their sign, and filling
  is offered only when `spent + still due` is positive.
- **Future-dated real transactions** count in spent, so in a future month that has some,
  the rule uses `spent + upcoming` like the current month does.
- **Same treatment in the Budget monthly view (desktop table and phone sheet) and the Plan
  grid**, wherever a plan number is shown or edited (`BudgetTable` / `LimitEditor` /
  `SetLimitDialog` / `ElementSheet` / `PhoneMonthView`, `PlanRows` / `PlanCellInput` /
  `PlanSheet`).
- **The popover's data comes from the existing `upcoming[]` of the budget
  `get-transaction-list`**, fetched for that cell and month when the editor opens (same
  cell params the spent list uses). The spent list stops rendering it. Rows show the
  template's description / payee / category name, the due date (overdue ones marked as in
  the Inbox) and the amount in the cell's currency.
- **Wire clean-up, since #363 is unreleased.** Keep what the new UI needs: per-element
  `upcoming` / `budgetUpcoming` on `get-budget`, per-cell `upcoming` on `get-budget-plan`,
  and `upcoming[]` on `get-transaction-list`. Remove what only fed the projections:
  `upcomingBefore`, `upcomingOpeningBalance`, the top-level `upcoming*` arrays on the plan
  (`upcomingOpeningBalances`, `upcomingSavingsOpeningBalances`, `upcomingTransfers`,
  `upcomingSavingsFlows`, `upcomingSavingsIncomeExpense`), and stop counting upcoming money
  in savings `closingBalance` (`addSavingsPending` goes back to plan gaps only). Goldens
  (apiparity, mcpparity) regenerated and inspected; the MCP glossary updated. Fields on
  the `upcoming[]` items that only served Post / Skip / Edit (`isNext`, `canEdit`,
  `schedule`, ...) may stay; the builder may trim them if nothing reads them.
- **SPA clean-up**: `foldBudgetUpcoming` / the plan fold in `upcoming.ts`, the
  `projectedActual` path in `planMath.ts`, `planSelectionSum`'s upcoming part, the
  budget-list Post / Skip / Edit wiring (`recurringPostPayload`'s scheduled-date option if
  nothing else uses it) and the `FUTURE_FIGURE` combined styling go. The future-month
  spent column goes back to what it showed before #363.
- **Kept from #363**: future-dated real transactions are still styled upcoming in the
  spent list (they are real); every recurring mutation still invalidates `budget`,
  `budgetPlan` and `budgetTransactions` (the hint moves when a template is posted or edited).
- **Analytics**: the plan write fires its own event; a press on the list fires
  `appBudgetPlanUseUpcoming` (added with the revision).

## How it reads in the UI

Expense cell "Groceries", future month, recurring 650 due that month:

| plan | shown |
|------|-------|
| none | empty, as before (the editor's list, Total 650.00, fills it) |
| 500  | **500** in red |
| 700  | 700, normal |

Same cell, current month, spent 300, still due 250: plan 500 shows red (300 + 250 > 500);
plan 600 normal; no plan shows nothing, and its editor's list totals 550.00 (300 spent +
250 still due) and fills it on a press.

Opening the editor on any of them lists that month's still-due payments and later-dated
own transactions under the input, then Spent 300.00 and Total 550.00.

## Docs and tests

- `docs/regression-test-plan.md`: replace #363's budget / plan items (combined figure,
  list Upcoming group, post / skip from the list) with: grey italic suggestion with no
  plan; red below need (future and current month); not counted in totals; editor popover
  lists payments; spent list shows real only.
- i18n: the popover heading / total label in all 11 catalogues; drop #363 strings no
  longer used (guards enforce both).
- vitest for the hint rule (pure function: plan, spent, upcoming, kind, month →
  suggestion / red / normal), totals ignoring the suggestion, the popover, and the spent
  list without upcoming rows; Go tests for the removed fields and `closingBalance`.

## Left to the builder

- Exact component split for the hint and popover, and how the popover is anchored in the
  Plan grid's inline input versus the dialogs.
- Whether a cell with no plan and no upcoming money shows anything (keep today's look).
