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
3. **Upcoming payments inform the plan number of a cell**, in every month from the caller's
   current one onwards:
   - **No plan set** (planned is 0 / unset) and the cell has upcoming money: the plan spot
     shows `spent so far + upcoming still due` (the smallest plan that would not be red) in
     **grey italic** as a suggestion. In a future month spent is normally 0, so it is the
     upcoming total. It is display only:
     nothing is saved, there is no accept button, and it is **not counted in any total**
     (cell Available, folder / group / total rows, Balance, Total savings, carry-over,
     multi-cell selection sum). Typing a plan replaces it.
   - **Plan set but below what the month needs**: the plan number shows in **red**. What
     the month needs is `spent so far + upcoming still due`. In a future month spent is 0,
     so this is simply "plan < upcoming".
   - **Plan set and at least that**: shown normally.
4. **Editing a plan number shows the upcoming payments.** When the user opens the plan
   editor of a cell that has upcoming payments in that month, a popover (or, on a phone
   sheet, a block inside the sheet) lists them: date, name / payee, amount, with their
   total. Read-only, no actions. With no upcoming payments nothing extra shows.

"Upcoming" keeps #363's projection rules unchanged (server, `internal/budget`): every
template on the budget's member accounts, whoever owns it; in the current month, an
overdue next payment counts once plus the occurrences after today; later months get all
their occurrences; filed into cells exactly as the posted transaction would be. In the
current month, payments already posted are real and are in spent, not in upcoming.

## Non-goals

- No change to how templates are scheduled, posted or skipped, and no posting from the
  Budget or Plan.
- No hint for months before the caller's current one (they have no upcoming money).
- Account page, transaction pages, Inbox, Recurring page: unchanged.

## Anti-goals

- A suggested (grey) plan must never move any total or projection.
- A spent / actual figure, or the list behind it, must never include money that does not
  exist yet.
- The grey suggestion must never be mistaken for a saved plan (hence grey + italic).

## Decided with the user

- Future months: spent / actual real only (the combined grey figure from #363 is removed).
- No plan: upcoming total as the plan, grey italic, not in totals.
- Plan below what is needed: red; the popover with the planned transactions appears when
  editing the plan number, if there are any.
- Spent list: past / real transactions only; upcoming rows removed from it.
- Current month: facts only for facts; the plan hint uses the payments still due (overdue
  next + later this month), red when `plan < spent + still due` ("let's try as you
  described").
- No-plan suggestion = `spent + still due` (550 for 300 spent + 250 due), so typing the
  suggested number never turns the cell red.

## Calls made by me (check these)

- **Red is for expense cells only** (categories, tags, envelopes on the expense side, and
  their children). On income and savings rows, receiving or saving more than planned is
  not an overrun, so they get the grey suggestion and the editor popover but never red.
- **Folder, tag-group and total rows show no suggestion and no red**; they only sum real
  plans. The hint lives only on rows that hold a plan; child categories under an envelope
  or a tag show nothing, their upcoming money appears on the parent.
- **The red rule uses the month's plan alone, not carry-over**, so the Budget view and the
  Plan grid share one rule (the grid has no carry-over). The existing red Available / bar
  in the Budget view stays as it is; the two can disagree when earlier months left money
  (red plan, positive Available), which reads as "this month's plan is short".
- **Savings rows**: their `upcoming` is a net change (deposits minus withdrawals). The grey
  suggestion shows only when that net is positive; savings never go red; the editor
  popover lists deposits and withdrawals with their sign and the net total.
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
- **Analytics**: no new metric; this is display only, and the plan write already fires
  its event.

## How it reads in the UI

Expense cell "Groceries", future month, recurring 650 due that month:

| plan | shown |
|------|-------|
| none | *650* in grey italic in the plan spot; Available 0 |
| 500  | **500** in red |
| 700  | 700, normal |

Same cell, current month, spent 300, still due 250: plan 500 shows red (300 + 250 > 500);
plan 600 normal; no plan shows *550* grey italic (300 spent + 250 still due).

Opening the editor on any of them shows the list of that month's still-due payments with
their total next to the input.

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
