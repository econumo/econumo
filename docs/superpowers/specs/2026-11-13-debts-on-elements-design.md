# Debts attached to budget elements

**Date:** 2026-11-13 · **Status:** design, awaiting approval
**Replaces:** the budget-row model of `2026-10-29-budget-debts-design.md`,
`2026-11-07-debt-setup-redesign-design.md` and `2026-11-12-shared-budget-debts-design.md`
(branch `feature/budget-debts`, PR #424, not released). What a debt IS (terms, snapshots,
statements, splits, the projection, the review screen, rate changes from
`2026-11-13-debt-rate-changes-design.md`) stays; HOW it sits in the budget changes.

## Why

Today a debt takes its payments and plans away from the categories it covers and shows them on a
debt row of its own, in a Debts block outside every folder. The owner's budget broke in two ways:
the "Base expenses" folder, used to estimate mandatory costs, dropped by the mortgage, and the
housing envelope (mortgage, insurance, strata) lost the mortgage's spending while its plan still
counted it. The money should stay where the user organized it; the debt should be information and
planning on top of it.

## Decisions (agreed with the owner)

1. **A debt is attached to one expense element of a budget**: a category row or an envelope.
   "Track as debt" on that element creates it. The element keeps everything: its plans, comments,
   spending, folder and position. Nothing moves; every folder, envelope and total stays as without
   debts.
2. **The element shows the debt** (what the debt row shows today: payments left, payoff, the plan
   hint, Details). The Debts block, debt rows (`ElementDebt`) and plan moves go away.
3. **One active debt per element at a time, plus a history.** A debt has a start and an end. It can
   be **completed** (it stops counting after its end date; the element is a regular element again)
   or **deleted**. Neither touches plans. Past debts are listed in the element's menu and open
   read-only.
4. **Payments = exactly the money the element's Spent counts**, dated between the debt's start and
   end: the expenses the budget attributes to that element (its category, or an envelope's child
   categories), so a tagged expense counted on its tag row, or one on a savings member, is not a
   payment (*my call*: the debt and the element's Spent and plan must agree). No payee, tag or label
   filters, no rules.
5. **A planned extra = the element's plan above the scheduled payment** in that period. Planned once,
   on the element.
6. **The debt shows its element's icon, name and currency.** It has none of its own (the debt icon
   picker goes away).
7. **An element carrying an active debt cannot be moved into an envelope.** The owner splits a mixed
   envelope (a "Mortgage" envelope with the loan categories, "Housing" with insurance and strata).
8. **Existing debt data is deleted** by a new migration, as last time.

## Approaches considered

Only one fits the decisions. Rejected before them: debt rows allowed in folders (the envelope
still loses the mortgage), a read-only mirror line in the folder (money shown twice), a debt-type
envelope replacing the envelope (plans and history tied to a converted element, harder to undo).

## Model

- `debts`: `budget_id`, the element it is attached to as the budget element's external id
  (`element_external_id`, the category or envelope id, as `budgets_elements.external_id`) plus its
  type, `status` (`active` | `completed`), `ended_at` (DATE, NULL while active), `currency_id`, and
  the existing terms columns, payment, schedule, `start_date`, `created_day`, timestamps. Gone:
  `name`, `icon`, and `debts_rules`. Snapshots, splits and rate changes stay keyed by debt.
- **The debt's currency is the element row's currency**, copied at creation; changing the element's
  currency (`change-element-currency`) moves its ACTIVE debt to it, amounts read unconverted (as a
  debt currency change does today: splits dropped and redone). A completed debt keeps the currency
  it ended in. *My call.*
- At most one `active` debt per (budget, element). A new debt on an element with history starts
  after the latest completed debt's end (`debt.start_before_previous_end` otherwise). *My call.*
- Only a top-level expense element can carry a debt: a category not inside an envelope, or an
  envelope. Not tags, savings, income, or a category inside an envelope. *My call* (tags left out
  for now).
- **The migration** (both engines, a version above every migration on the branch): deletes every
  debt (snapshots, splits, rate changes) and every `ElementDebt` (type 6) budget element row with
  its plans and comments, then reshapes `debts` and drops `debts_rules`.

## Matching and figures

- A transaction is a payment of debt D when it is an expense (transfers never), on a member account
  of D's budget (soft-deleted members included), in one of D's element's categories (an envelope's
  children as they are now), dated on or after D's start date (or any date while not set up) and
  on or before its end date when completed. Categories cannot be in two elements of a budget, and an
  element has one active debt with non-overlapping history, so there are no conflicts, keys or
  tiebreaks any more.
- Everything about splitting, snapshots, statements, setup, corrections, the projection, rate
  changes and the review screen stays, with "the debt row's plan" replaced by **the element's
  plan**: the planned extra of a period is the element's plan above the scheduled payment
  (`Projection` input `Plans` comes from the element's limits), plans outside the debt's life are
  ordinary plans, and the plan-aware forecast reads the element's plans.
- **Completing** a debt (`complete-debt`, a date, default today, not before its start and not after
  today): stores a statement of 0 as of that date (replacing one that day; this replaces "Mark as
  paid off"), sets `status = completed` and `ended_at`. Payments after the end stop matching (their
  splits go on the next sync). Refinancing = complete the old debt, then track a new one on the same
  element. *My call:* completing a debt that still owes is allowed; the 0 statement says the loan
  was closed (paid off or refinanced).
- **Deleting** removes the debt and its history; plans and transactions are untouched. Offered on
  active and past debts.
- **Reopening** a completed debt (`reopen-debt` `{id}`, *my call*): removes its end, its status and
  the 0 statement completion stored, allowed only while its element has no active debt and no debt
  started after it (`debt.cannot_reopen` otherwise). It is how a wrong completion date is fixed
  (reopen, complete again). `delete-debt-reconciliation` refuses the closing statement of a
  completed debt (`debt.completed`); a completed debt is otherwise read-only.

## Budget integration

- **No debt rows.** In a period, an element shows **the debt in force in that period** (the active
  one, or a completed one whose start..end covers it; *my call*: last year's budget shows the loan
  that was being paid then). `get-budget` elements (and envelope rows) of a top-level expense
  category or envelope with a debt in force in the period get an additive `debt` object: `{id, isSetUp, paymentsLeft,
  payoffDate, basePayoffDate, closingBalance, scheduledTotal, upcoming, isPaidOff,
  overpaymentLeft, baseOverpaymentLeft}` (the figures debt rows carry today, same rules, now
  computed from the element's plans and payments); `get-budget-plan` rows the same per row plus the
  per-cell figures debt cells carry today. `structure.debts` and the debt rows' plan/limit handling
  go away (in the Plan grid each cell carries the id of the debt in force in its period). Spent,
  Available and every total are the element's own, unchanged by the debt.
- The red plan hint of a debt-carrying element: plan below the scheduled amount (current and future
  periods), replacing the expense hint on that row while the debt is active.
- **Envelopes:** moving a category that carries an active debt into an envelope is refused
  (`budget.element_has_debt`). Adding a category to an envelope that carries a debt is allowed: its
  expenses count as payments from then on.
- **Classification deletes and merges** are never refused. Deleting a category or envelope deletes
  the debts attached to it (all history). A category merge moves the source's debts to the target
  only when the target has no debts at all (active or past), can carry one (a top-level element) and
  its row is in the same currency; otherwise the source's debts are deleted with it. *My call:* it
  keeps "one debt per element at a time, no overlapping history" without conflict rules.
- **Participants and clones:** a participant who leaves takes their accounts (their payments stop
  counting) and their category elements; debts attached to those elements stay and show again if
  the element returns. *My call.* `clone-budget` copies the debts with their element ids, snapshots
  and rate changes; splits rebuild.
- **Onboarding** debt detection attaches each detected debt to its category; the plan handover
  (`TakeCategoryPlans`) goes away.
- **MCP:** `get_budget` loses `structure.debts` and gains the `debt` object on elements; `set_limit`
  no longer takes a debt id.

## API (unreleased, so the wire may change)

- `create-debt` `{budgetId, elementId, payment, schedule, startDate?}` (elementId = the category or
  envelope id): refused when the element is not a top-level expense element of the budget, already
  has an active debt, or the caller may not plan there.
- `update-debt` loses name, icon, currency and rules; keeps terms, payment, schedule, statements.
- New `complete-debt` `{id, date?}`; `mark-debt-paid-off` and `add-debt-rule` go away;
  `get-debt-rule-options` goes away.
- `get-debt-list` lists active and completed debts of the caller's budgets, each with `budgetId`,
  `elementId`, `elementType`, and the element's `name`, `icon`, `currencyId`, `status`, `endedAt`.
  `DebtResult` drops `rules`, `icon` of its own, and takes name/icon/currency from the element.

## SPA

- **Row menu** of a top-level expense category or envelope (anyone who may plan, unarchived
  budget): "Track as debt" when no active debt; with one, "Debt details" (opens the review screen)
  and, in Danger zone terms, Complete and Delete. "Past debts (N)" when there is history: a short
  list (dates, amount borrowed) opening each read-only.
- **The row shows the debt**: its normal Planned / Spent / Available, plus a quiet debt line or
  badge with payments left and the payoff month; the item sheet gets a Debt section with Details.
  The Plan grid cell gets the debt cell hints. Exact layout left to the builder within the existing
  row styles.
- **Create form**: payment and schedule (and the start date only after a conflict), no name, icon,
  category pickers or rules; the header shows the element's icon and name.
- **Review screen**: header = the element's icon and name; Settings loses name, icon, rules and
  currency; Danger zone has Complete (with a date) and Delete. Overview, Balance (statements, rate
  changes) as now.
- Removed: the Debts block, debt rows in Budget and Plan, `RulesEditor`, `PaymentsFromEditor`,
  `AddToDebtDialog`, the debt icon picker.
- Analytics: `DEBT_COMPLETE` replaces `DEBT_MARK_PAID_OFF`; `DEBT_RULE_ADD` is retired (NOT_WIRED or
  removed); the others keep firing.

## Non-goals

- Debts on tags, income, savings, or a category inside an envelope.
- Payee/tag/label filters.
- Converting existing debts.

## Left to the builder

- Exact column names, the element-type encoding, and whether the attachment uses the budget
  element row id or the external id (the external id survives a row being re-created).
- The row layout of the debt figures in Budget and Plan.
- Updating CLAUDE.md, the earlier specs' pointers, the regression test plan, the 11 locales,
  apiparity/mcpparity goldens, revisionguard, onboarding.
