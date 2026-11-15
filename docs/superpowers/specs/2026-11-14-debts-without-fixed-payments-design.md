# Debts without fixed payments

**Date:** 2026-11-14 · **Status:** approved in chat 2026-11-14
**Builds on:** `2026-11-13-debts-on-elements-design.md` (debts attached to budget elements) and
`2026-11-13-debt-rate-changes-design.md`. Everything not changed here stays.

## Why

A personal loan from a friend or family member has no interest and no fixed instalments: you pay
back what you can, when you can, and agree to return it by a date. Today a debt needs a payment
and a schedule, and its rate and payments left are worked out from them, so such a loan cannot be
tracked.

## Decisions (agreed with the owner)

1. **A "No fixed payments" schedule** next to every week / every 2 weeks / month / quarter / year.
   With it the payment may be 0 (the field can be left at 0).
2. **Setup asks only for the amount borrowed, the start date and the "return by" date** (the last
   payment date, relabelled for this schedule). **No interest**: every payment comes fully off the
   balance; no rate is worked out (rate 0), no due dates exist.
3. **The row and the Overview show what is left and the date**: "3,000 left · return by Dec 2027",
   not payments left, plus a hint of what a month finishes on time (balance / months from the
   current period through the return-by month, rounded to the currency).
4. **Plans still count**: the element's plan in a period is money planned toward the loan; the
   forecast (payoff) shows when it would be repaid at those plans. Without plans there is no
   payoff date, only the return-by date.
5. Paid so far, the payments list, statements, rate changes (refused for this schedule, since there
   is no rate: `debt.no_fixed_payments`), Mark as paid off, reopen, Past debts work as now.

## Approaches

One fits: a new debt schedule value `none` stored in `debts.schedule` (debt-only; the recurring
transactions' schedule type is untouched). Rejected: modelling it as a monthly schedule with a 0
payment (the math would still create dues and refuse the terms as not repayable).

## Model and math (builder's details)

- `none` is accepted wherever a debt schedule is (create, update, snapshots); `payment` may be 0
  only with `none` (other schedules keep requiring a positive payment).
- Math for `none`: period rate 0; no dues (`DueDatesBetween` returns nothing); splits put 0
  interest on every payment; balance = amount borrowed less payments (statements still reset it);
  `periodsPaid` 0, `paymentsLeft` 0, `extraPaid` 0; projection: only plans repay (each period's
  element plan, from the current period on), payoff = the period the plans clear the balance, else
  none; `scheduledTotal` 0, no red "below the scheduled amount" hint; the setup's
  `payments_dont_cover` / repayable checks are skipped for `none`.
- Wire: `schedule: "none"`; new additive `monthlyToFinish` on the debt result and the element's
  `debt` object (decimal string, `""` when not `none`, not set up, paid off or past the date).
- SPA: the schedule picker gets "No fixed payments"; with it the payment input may be 0 or empty
  (sent as "0"), the last-payment label reads "Return by"; the Overview and the row's debt line
  show "X left · return by <month>" and "≈ Y a month to finish on time" when `monthlyToFinish` is
  set; the Rate changes card is hidden for `none`. Locales in all 11 catalogues.

## Non-goals

Interest on informal loans; several partial due dates.
