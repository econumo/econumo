# Upcoming recurring payments in the Budget and the Plan

Status: design, 2026-10-22. Branch `feature/budget-upcoming-recurring`.

## Why

The Budget and the Plan count only transactions that exist. A recurring template (rent,
subscriptions, salary) counts nowhere until it is posted, so a future month shows only the
planned amounts, and those don't cover everything the user already knows is coming. The user
wants the money that is already promised to show up in every month it falls in, and wants
the transaction list behind a cell to make it obvious which rows are real and which are
upcoming recurring payments.

## Goals

1. Every month in the Budget (monthly view) and the Plan grid counts the recurring payments
   still to come in it, for every template on the budget's member accounts, whoever owns it.
2. Upcoming amounts count like spending (and like income/transfers for those types): the
   spent / actual figure is ONE combined number, real + upcoming, shown exactly as today with
   no split or separate style (decided with the user), and Available goes down by the
   upcoming amount. The difference between real and upcoming is visible only in the cell's
   transaction list.
3. Posting a template turns its upcoming amount into real spending without moving the total
   (see "Late payments" for the one case where the month changes). Exception: posting a
   template several cycles behind raises the current month's total, because posting advances
   `nextPaymentAt` by one cycle and the next missed cycle, dropped until then, becomes the new
   overdue row (see "Overdue: one row" below).
4. The transaction list opened from a Budget or Plan cell shows the upcoming payments of that
   cell and month in their date groups (one row per occurrence), each with a repeat icon and its due date.
   - The template's **next** payment (including a late one) can be posted, skipped or edited
     from there, with the same actions as the Inbox's Due section.
   - Later payments of the same template are display only. Tapping one opens the template
     view (`ViewRecurringDialog`) without Post/Skip; Edit stays available to whoever may edit
     the template.
5. Future-dated real transactions keep counting as they do now, and are marked upcoming (the
   same lighter style) in the list until their date. The figures keep them in the real part
   (my call, see below).

## Non-goals

- Account balances, the account page and the transaction pages stay as they are (the account
  page already shows upcoming templates its own way).
- No change to how templates are scheduled, posted or skipped; no end dates on templates.
- No posting or skipping of a future (not next) payment.
- No new MCP tools. MCP output changes only by the additive fields below.

## Anti-goals

- The frozen wire meaning of `spent`, `budgetSpent`, `available`, `actual` must not change:
  old clients and MCP callers must keep reading real money only. Everything new is additive.
- A partner's payment must never be actionable by someone who cannot edit that template.
- Upcoming rows in the transaction list must never be confusable with real ones.

## Decided with the user

- Upcoming counts in Available (option "170 used, 30 available" for 200 planned / 120 spent /
  50 upcoming).
- Next payment: post / skip / edit. Later cycles: seeing is enough.
- Shared budgets: a partner's templates on member accounts show and count for every
  participant.
- Posting from the Budget/Plan list: a next payment dated in the future is posted ON its
  scheduled date (a future-dated transaction, same month, so only the style changes); a
  payment due today or overdue is posted as today, as everywhere else. Other surfaces (Inbox,
  recurring settings, account page) keep posting as today.

## Calls made by me (check these)

- **Who can act on a partner's payment:** the existing recurring write rule (account owner or
  non-guest grant on the template's account, `recurring/usecase.go`). Everyone else sees it
  display only, like later cycles.
- **Late payments count in the current month, not their scheduled month.** The SPA posts with
  `date: now` (`recurring/postPayload.ts`), so a posted late payment lands in today's month.
  Counting it there keeps "posting changes the style, not the total" true. A late payment is
  labelled with its original due date and an "overdue" note (same as the Inbox).
- **Overdue: one row, then only what is after today** (decided with the user). When a
  template's next payment is in the past, that one payment shows and counts in the current
  month as overdue, and it is the actionable one. The cycles between it and today are neither
  shown nor counted; projection continues with the occurrences dated after today. A weekly
  template three weeks behind therefore shows one overdue row plus its remaining weeks.
  Posting that overdue row advances the template by one cycle only, so the next missed week
  becomes the overdue row and the month total goes up by it (the Goal 3 exception); a
  template due once keeps the month total when its overdue payment is posted.
- **Templates on deleted accounts are not projected.** A deleted account keeps its budget
  membership and its templates, but no one can see, post or remove them there, so a template
  whose account, or a transfer's recipient, is deleted would be upcoming money nothing clears.
- **A non-member account's name is not shown.** A transfer into a member from an account
  outside the budget (a partner's private account) carries that account's id but an empty
  `accountName`.
- **Projection end:** the last month the request covers (the selected month for `get-budget`,
  the window end for `get-budget-plan`). Templates have no end date, so this bounds the work.
- **Future-dated real transactions** are styled upcoming in the list only (from tomorrow
  00:00, `isFuture` in `lib/datetime.ts`); in the figures they stay in `spent`/`actual` as
  today and are not part of the new `upcoming` fields, so no extra wire field is needed.
- **Display-only rows carry their own names.** A partner's template on their unshared account
  is not in the caller's recurring list or classification lists, so each upcoming item carries
  what the read-only view needs (schedule, account id and name, resolved category/payee/tag
  names, label names). Rows the caller can edit open the full `ViewRecurringDialog` from the
  caller's own `useRecurring` list; the rest open a lightweight read-only view built from the
  item.
- **Analytics:** posting/skipping/editing from the budget list goes through the existing
  recurring hooks, so their existing events fire. Add a `source: 'budget'` property if the
  hooks already take a source; otherwise no new metric.

## How it works

### Projection (server, `internal/budget`)

The partner's templates on their own non-shared member accounts are not visible through
`get-recurring-transaction-list` (it filters by the caller's visible accounts), so the
projection must be built server-side, inside the budget builder, over the budget's member
accounts (`buildFilters` → `includedAccountIDs`).

- The budget feature declares a port (`ports.go`) to list templates by account ids; a
  `glue_budget_recurring.go` adapter in `internal/server` implements it over
  `internal/recurring`'s repository (`ListByAccountIDs` already exists). Features never
  import features.
- For each template, expand occurrences with `model.NextOccurrence` (it keeps the scheduled
  day, so month-end clamping is right) from `nextPaymentAt` while the occurrence is before
  the projection end. If `nextPaymentAt` is before today (caller's timezone) it is filed in
  the caller's current month (`localMonth(now, tz)`) as the overdue occurrence; the further
  occurrences dated before today are dropped, and expansion continues with those from today on.
- Each occurrence is filed with the SAME rules as real transactions, so it lands in the same
  cell the posted transaction will:
  - expense (type 0) on an everyday member: by tag (tag wins, category as child), else
    category (NULL → Uncategorized), envelopes collect their untagged categories;
  - income (type 1) on an everyday member: income category/envelope rules of the Plan;
  - transfer (type 2): the transfers line and savings flows exactly as `TransfersByMonth` /
    `AccountsNetByMonth` would treat it (savings member in/out, crossing the included-account
    boundary). The template carries one amount: use it for both sides, converting the
    recipient side with the period rate as for real data.
  - templates on savings members feed the savings rows' saved amount.
- Amounts are queued into the existing `toConvert` / `BulkConvert` pass (new key prefixes,
  e.g. `upcoming_`, `upcoming-budget_`, `upcoming-before_`) so they get the same rates.

### Wire (additive)

`get-budget` — on each structure element (and child), `savings` row and label:
- `upcoming` (element currency) and `budgetUpcoming` (budget currency): projected amount in
  the selected month.
- The SPA subtracts `budgetUpcoming` / `upcoming` from what it shows as Available. For a
  future selected month, carry-over must also include upcoming amounts of the months between
  the current month and the selected one: send `upcomingBefore` per element for that, the
  same way `spentBefore` feeds `available` today. Server `available` stays real-only.

`get-budget-plan` — each cell gets `upcoming` next to `{actual, planned}`; top-level
`savingsFlows` / `transfers` get parallel upcoming arrays (names left to the builder, additive).
Each savings row also carries `upcomingOpeningBalance` (element currency, `"0"` unless the
window starts after the current month): the projected balance before the window start, so a
row's closing balance is booked + that + the cumulative cell upcoming, and the rows add up to
the savings balance line.

`get-transaction-list` (budget) — adds `upcoming: []`, each item:
`{templateId, date, amount, currencyId, description, category, payee, tag, labelIds,
labelNames, type, direction, author, schedule, accountId, accountName, isNext, isOverdue,
canEdit, accountRecipientId, accountRecipientName, amountRecipient, currencyRecipientId,
amountSource, currencySourceId}`
(category/payee/tag as resolved `{id, name, icon}` like the real rows; the transfer fields are
`""` on non-transfers; `amountSource` is the template amount in the source account's currency;
the recipient id/name are `""` when the recipient is not a member, as for the source account). Filtered by the same cell params and month
as the real rows. `canEdit` = the recurring write rule for the caller.

All three need apiparity/mcpparity goldens regenerated and inspected; add scenarios with a
template on a member account (monthly, plus a weekly one to prove several occurrences per
month, plus a partner's template in a shared budget). Update the MCP budget glossary
(`internal/web/mcp/prompts.go`) for the new fields. Run the enginecompare suite: occurrence
dates must bucket into the same month on both engines (compute months in Go, not SQL).

### Budget monthly view (SPA)

- Spent shows the single combined figure real + upcoming, styled as today; the progress bar
  and over-budget state use the same combined figure.
- Future months: today the spent column shows a dash (`future` in `BudgetTable.tsx`). It
  now shows the combined figure (upcoming plus any future-dated real transactions), and the
  cell opens the list when there is anything to show.
- Folders, tag groups and totals (`budgetMath.ts`) sum upcoming alongside spent.

### Plan grid (SPA)

- Current and future cells show the single combined actual (real + upcoming), styled as the
  actual is today; future cells, which today show only the plan, now show that combined
  actual too.
- `planTotals`' effective value for current/future months becomes
  `max(planned, actual + upcoming)`; Balance and savings projections (`projectSavingsClosings`)
  use the same effective value, so a recurring salary or savings transfer moves the projected
  balance even without a plan. The server's savings `closingBalance` pending gap uses the same
  rule (`addSavingsPending`).
- `planSelectionSum` includes upcoming in the "Actual" figure for current/future months.

### Transaction list (`BudgetTransactionsDialog.tsx`)

- One row per occurrence, also for weekly/biweekly templates (decided with the user: no
  folded "4 x 10" line). A weekly 10 on Wednesdays shows four or five rows in a month and
  counts their sum in the cell.

- Upcoming rows sit in their own date groups like real rows (so posting doesn't reshuffle
  the list), styled upcoming, rendered with `TransactionRow` through
  `recurringAsTransaction`-style mapping, repeat icon, due date, lighter style; the late one
  carries the overdue note.
- `isNext && canEdit`: tapping opens `ViewRecurringDialog` with Post / Skip / Edit (reuse
  `recurringPostPayload`, `usePostRecurring`, skip, `openRecurringModal`). Post here passes
  the scheduled date when it is after today and today otherwise (a `date` option on
  `recurringPostPayload`; the server already takes the date from the payload). The posted
  transaction keeps the template's time of day only if that is how the form already handles
  dates; otherwise builder's call.
- Otherwise: `ViewRecurringDialog` read-only for the occurrence (Edit shown only with
  `canEdit`; never Post/Skip).
- Future-dated real rows in the normal groups get the upcoming style.

### Cache invalidation

Every recurring mutation (create, update, delete, post, skip) must invalidate `budget`,
`budgetPlan` and `budgetTransactions` (today post skips `budgetPlan`; the others touch no
budget query).

### Docs and tests

- `docs/regression-test-plan.md`: budget and plan cases for upcoming amounts, the list's
  Upcoming group, post/skip from the list, partner template display-only, late payment in the
  current month, weekly template in one month, future month view.
- i18n: new strings ("Upcoming", "incl. {amount} upcoming", overdue reuse) in all 11 catalogues.
- Go unit tests for occurrence expansion and filing (incl. 31st → Feb clamp, weekly, late
  collapse, window end) and vitest for the math and the dialog.

## Left to the builder

- Names of new plan top-level fields and internal helpers.
- Whether the projection lives in a new `builder_upcoming.go` or alongside existing builders.
