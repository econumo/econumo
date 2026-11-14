# Debt rate changes

**Date:** 2026-11-13 · **Status:** design, awaiting approval
**Builds on:** `2026-11-07-debt-setup-redesign-design.md` and `2026-11-12-shared-budget-debts-design.md`
(branch `feature/budget-debts`, PR #424, not released). Everything not changed here stays as those
designs and the code on the branch describe it.

## Why

A debt has no rate the user enters: Econumo implies one from the loan's terms (amount borrowed,
payment, start and last payment date) and keeps it for the loan's whole life; a statement corrects
the balance, never the rate. Real loans change rate. The owner's mortgage is variable: the bank shows
3.69% now, while the six payments so far imply about 3.48%, and with the payment fixed the remaining
amortization has grown to 31 years 2 months, past the 30 the contract started with. Econumo keeps
splitting at the old rate, so the interest, the balance and the payoff drift away from the bank's.
At the end of a term (May 2031 for this mortgage) the bank also sets a new payment.

## Decisions (agreed with the owner)

1. **An optional list of rate changes per debt**, each a date, a yearly rate and a payment. From
   its date on, payments are split at that rate and the forecast uses that rate and that payment.
   A debt with no change works exactly as today.
2. **The form asks for the date, the rate and the payment**; the rate and the payment are prefilled
   with the ones in force now (the latest row's), so recording a change is usually one edit.
3. **A separate list (approach B)**, not a new kind of snapshot: a rate change carries no balance,
   so a payment entered later for an earlier day never contradicts it. Statements keep correcting
   the balance only.
4. **The first row is the rate implied by the loan's terms.** While the debt has no change dated on
   its start date it is derived, not stored: it follows a correction of the terms. **Adding a change
   dated after the start, when there is no start-date row yet, also stores one**: dated the start
   date, with the implied yearly rate at that moment and the origin's payment, in the same
   transaction. From then on it is an ordinary row: adding a change on the start date replaces it,
   it can be deleted (*my call*: the start then falls back to the derived row), and a later
   correction of the terms no longer moves it. *My call:* the stored implied rate keeps full
   precision (shown with two decimals), so storing it changes no split.

## Approaches considered

- **A. A rate snapshot in `debts_snapshots`.** Reuses "terms from this date", but every snapshot has
  a balance; a rate change would have to freeze one at its date, which a backdated payment then
  contradicts, so the entry silently becomes a statement. Rejected.
- **B. A separate `debts_rate_changes` list.** Rate and payment in force are looked up by date; no
  balances. Picked: rates and balances change independently and neither can corrupt the other.

## Model

- New table `debts_rate_changes`: `id`, `debt_id` (FK `debts`, ON DELETE CASCADE), `effective_date`
  DATE, `annual_rate` NUMERIC (percent, e.g. `3.69`), `payment` NUMERIC (debt currency), `created_at`;
  unique `(debt_id, effective_date)`. **A new migration** on both engines, numbered above
  `20261113000000`.
- **Period rate** = `annual_rate / 100 / periods per year` (weekly 52, biweekly 26, monthly 12,
  quarterly 4, yearly 1): a nominal rate compounded per payment period. *My call:* Canadian
  mortgages compound semi-annually, so for them this is a little high (about 13 a month on 588k at
  3.69%); a statement corrects the balance. A compounding setting is a non-goal.
- **The implied rate** (today's `periodRate`) is shown as a yearly rate: `periodRate x periods per
  year x 100`, two decimals.

## Math

- **Rate in force on a day** = the latest change dated on or before it; before the first change,
  the loan's implied rate (as today, `withLoanRate`). A change dated on the start date replaces the
  implied rate everywhere, the origin's first prorated period included.
- **Interest of a window** (the schedule period ending at its due) is prorated by days between the
  rates in force inside the period: `balance x sum(rate_i x days_i / period days)`. A change on a
  due date applies to the period after it (the payment that day is for the period before). The
  origin's prorated first period combines the same way.
- **Payment in force on a day** = the latest rate change's payment on or before it, else the
  origin's payment (*my call*: statements stop carrying a payment of their own in this rule; a
  correction rewrites them to the origin's anyway, so they could only undo a rate change's payment).
  A change on the start date wins over the origin's payment. It drives
  everything the regular payment drives today: projected payments, payments left, the payoff,
  `extraPaid`, `scheduledTotal`, the still-due and red plan hints, `StatementRepayable`.
- **Statements**: a new statement still stores a payment (the payment in force on its date), but
  the rule above never reads it. `StatementRepayable` and the split after a statement use the rate
  and the payment in force (not only the loan's rate).
- **Contract fields stay as entered**: the last payment date still anchors the dues and shows as the
  "contract" date; the dues continue past it while a balance is left, exactly as after a statement
  above Econumo's balance today, so a higher rate with the same payment makes the payoff later and a
  lower one or a higher payment earlier (`periodsPaid`, `paymentsLeft`, `scheduledTotal` and the
  final partial payment follow from that as they do now).
- **Corrections**: correcting the origin's terms (Settings) rewrites the origin and later snapshots
  as today; rate changes keep their own rate and payment, so after a change's date its payment still
  wins. The implied first-row rate moves with the correction. *My call:* a correction that would put
  the start date after a rate change is refused with a new coded error `debt.conflicts_with_rate_change`
  (on `startDate`), like the statement conflict today; a correction after which the payments can
  never repay the balance is refused with `debt.payments_dont_cover` as today, the check now run
  with the rate changes applied.
- **Future-dated changes** are allowed (an announced change) and apply from their date in the
  forecast; splits of payments dated after today stay provisional as today.

## API

- `get-debt` adds `rateChanges: [{id, date, annualRate, payment}]` (oldest first) and on `item`
  `annualRate` (the rate in force today, yearly percent) and `impliedAnnualRate` (from the terms);
  both `""` while the debt is not set up. `periodRate` stays (rate in force today, per period).
  `get-debt-list` carries the two item fields too.
- `POST /api/v1/debt/add-debt-rate-change` `{id, date, annualRate, payment}` -> the debt details.
  `date` on or after the start date (`common.out_of_range` on `date`), `annualRate` a decimal from 0
  to 100 (`common.out_of_range`), `payment` a positive decimal; a date that already has a change
  replaces it (one row per day, like statements). Refused on a debt not set up (`debt.not_set_up`)
  and with `debt.payments_dont_cover` when the payment can never repay the balance at that rate.
- `POST /api/v1/debt/delete-debt-rate-change` `{id, rateChangeId}` -> the debt details; another
  debt's id is not found.
- Both are debt writes: plan access in an unarchived budget (guest 403, archived
  `budget.archived`), run in `inOneTx` with the sync, and drop the debt's stored splits of payments
  from the start of the schedule window holding the change's date on (*my call*: a window that
  straddles the date mixes the two rates, so its payments are split again too), so the next sync
  splits them at the new rate.
- **`debt.payments_dont_cover` on add/delete** (*my call*): checked after the write, inside its
  transaction, by projecting the debt with the new list (splits recomputed in memory as `Views`
  does) from the later of today and the change's date: refused when the balance is never repaid
  within the projection's horizon. The same check runs on a delete.
- Everything that projects a debt (`get-debt-forecast`, the budget's `Timelines` and `Views`,
  onboarding's plan handover) takes the rate changes through the same math input, so the planned
  extra is measured against the payment in force. Deleting a debt cascades its rate changes; a
  currency change reads their payments in the new currency unconverted, like the stored terms.
- `clone-budget` copies the rate changes with the debts. No MCP tool.

## SPA

- **Balance tab**, a "Rate changes" card under the statements history (set-up debts only):
  - a table Date | Rate | Payment, oldest first; with no stored start-date row the first row is the
    derived "From the start · 3.44% · 2,647.82", captioned "from your loan's terms", with no delete;
    every stored row (the start-date one included) has a delete button behind a confirm;
  - a form "Record a rate change": Date (default today), Rate in % (prefilled with the rate in force
    now, plain decimal input like the amounts), Payment (prefilled with the payment in force now);
    Add. A refusal shows under its field.
  - Guests and archived budgets see the table without the form or the delete buttons (`canEdit`).
- After an add or delete the debt, the lists and every forecast are refreshed (as for statements).
- Analytics: `DEBT_RATE_CHANGE_ADD`, `DEBT_RATE_CHANGE_DELETE`.
- Offline: the form and deletes are disabled with the tab's existing offline handling.
- The Overview gains no new row (the owner asked for less there); its payments left, Last payment
  and the future payments follow the new rate through the forecast.

## Non-goals

- Compounding conventions other than per payment period.
- Editing a rate change in place (delete and add again; adding on the same date replaces).
- Rate changes on a debt not set up.
- Showing the rate on the Overview.

## Left to the builder

- Exact names of locale keys and test fixtures; the 11 catalogues.
- How the rate list is threaded through `MathSnapshot`/`ProjectionInput` (a slice of
  `{Date, PeriodRate, Payment}` beside the snapshots is the obvious shape).
- apiparity scenarios and goldens, revisionguard (no transaction revisions move), enginecompare,
  CLAUDE.md, the regression test plan.
