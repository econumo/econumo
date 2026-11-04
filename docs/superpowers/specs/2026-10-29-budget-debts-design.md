# Budget debts: tracking loans without an account

**Date:** 2026-10-29 · **Status:** design, awaiting approval · builder-checked

## Why

A user pays a mortgage, a car loan or a personal loan. Today the payments are
expenses in a category (e.g. "Mortgage") and the user plans them in the budget.
That covers the cash going out, but nothing tells them **how much is left to
pay, and how many payments remain**.

Modelling the loan as an account would answer that, but it is the wrong tool:
a debt account drags a large negative balance into account totals and reports,
and it changes how payments have to be entered (transfers instead of expenses).
The user does not want the loan to be an account.

## Goals

1. For each loan, always show **payments left**, **money left to pay**, and the
   **current balance**, without the user typing anything after setup.
2. Show what the loan costs on top of the money borrowed (the **overpayment**,
   i.e. interest and fees), both already paid and still to come, **without ever
   asking for an interest rate**.
3. In the Budget and the Plan: see that each loan is **planned and paid**, plan
   **extra payments** in chosen months, and see what they do (earlier payoff,
   smaller overpayment), like the savings Balance already projects plans.
4. Turn an existing category into a debt **right from the budget**.

## Non-goals (v1)

- Debts are **not accounts** and appear in **no** account list, account total,
  net worth or other report.
- No interest rate field, no bank sync of the balance (a later version may fill
  the balance from a bank; the snapshot model below is built so it can).
- No balloon payments, variable-rate schedules, grace periods, or loans you lend
  to other people.
- No MCP surface.

## Anti-goals (fails even if it "works")

- A payment is counted twice (once in a category row, once in a debt row), or
  belongs to two debts.
- Changing a debt later rewrites the history of payments already made.
- Entering a payment changes: users keep recording ordinary expenses in a
  category; imports, CSV, recurring posts and MCP keep working untouched.

## Concepts

### A debt

Owned by a user (the owner of its category). Fields:

| Field | Required | Meaning |
|---|---|---|
| name | yes | defaults to the category name |
| currency | yes | the bank's currency; defaults to the user's currency |
| category | yes | an **expense** category of the owner |
| payee | no | narrows matching |
| tag | no | narrows matching |
| start date | no | payments before it don't belong to this debt; empty = from the first matching payment |
| **balance** | yes | what is owed right now (the bank statement number) |
| **payment amount + frequency** | yes | frequency: weekly, biweekly, monthly, quarterly, yearly (reuse `model.RecurringSchedule` and `NextOccurrence`) |
| **last payment date** | yes | when the loan ends; its day also anchors the schedule day |

### Which transactions are payments

An **expense** transaction is a payment on a debt when its category is the
debt's category, its payee is the debt's payee (if set), it carries the debt's
tag (if set), and its date falls in the debt's **matching range**.

- Matching ranges are stored dates only, never derived from the balance, so
  matching is deterministic. *(my call)* Among debts with the same
  category/payee/tag, ranges are partitioned by start date: a debt matches from
  its start date (empty = from the beginning) up to the day before the next
  such debt's start date. Two such debts can't share a start date (refused).
  So a payment always belongs to at most one debt (anti-goal 1). Example: an
  old and a new car loan in one "Car loan" category, the new one with a start
  date.
- Debts with **different** keys that could both match one transaction (one by
  category alone, another by category + payee) are refused at save. Only the
  same category with no payee/tag, or disjoint payee/tag pairs, may coexist.
  *(my call)*
- Matching is by data, not by a field on the transaction: nothing changes when
  entering, importing or posting transactions. Transfers never match.
- Payments from any account count toward the debt's balance (the debt is the
  user's, not a budget's).

### Snapshots

The three values (balance, payment + frequency, last payment date) are stored
as a **snapshot** with an as-of date. Creating a debt makes the first snapshot;
every later edit of those values makes a **new** snapshot as of today. Older
snapshots are kept and never changed. A snapshot's balance is the balance
**after** any payment dated on its as-of day. *(as-of-day rule: my call)*

From each snapshot Econumo derives the **implied periodic rate** `r`: the rate
at which `n` equal payments of the given amount, on the schedule from the
as-of date to the last payment date, exactly repay the balance (standard
annuity; solve numerically). `payment × n = balance` gives `r = 0`
(interest-free installments: every payment goes fully to the debt).
`payment × n < balance` is refused (the payments can't cover the debt).

### Splitting each payment (agreed: "bank-style")

Each payment is split into an **interest part** (overpayment) and a **debt
part**, and the **interest part is stored as an amount** on that payment, in
the debt's currency, when the payment is first counted. It is never recomputed
by later snapshot edits (anti-goal 2).

- Interest accrues per **schedule period**: for each period the interest due is
  `balance at period start × r`. Payments in that period consume it first, up
  to the payment amount; everything else in the period goes to the debt.
  Consequences, all agreed: an **extra payment goes fully to the debt** (no
  interest is taken from the extra), whether it is a bigger single payment or
  a second transaction in the same period. *(per-period accrual: my call)*
- A period is a **window around its due date**, from halfway after the
  previous due date to halfway before the next, so a payment made a few days
  early or late still lands on the due date it pays. *(my call)*
- `r` and the starting balance come from the snapshot in force at the
  payment's date. Payments dated **before** the first snapshot (back to the
  start date or the first matching payment) are estimated **backward** from
  the first snapshot with its `r`, so "overpaid so far" covers the whole loan.
- **Missed periods stay flat (agreed).** A period with no payment, or a
  payment smaller than the interest due, adds nothing to the balance: unpaid
  interest is not carried. The balance never rises just because a payment
  hasn't been entered yet; the user corrects drift with a new snapshot from
  their statement.
- Payment in another currency: converted to the debt's currency at that day's
  rate, as elsewhere.
- **Edits:** a payment whose amount is corrected keeps its stored interest
  (capped at the new amount); the difference goes to the debt (agreed). A
  payment edited so it no longer matches, or deleted, loses its split. A
  payment newly matching (late entry, back-dated, re-categorised) gets its split
  from the snapshot and running balance at its date. *(edit rules beyond the
  amount: my call)*
- When the split is computed (on write through a port, or lazily on first read,
  in date order) is the builder's choice, as long as the stored result is the
  one described above.

### What Econumo shows

- **Current balance** = latest snapshot's balance − debt parts of the payments
  after it.
- **Payments left** and **projected last payment date**: amortise the current
  balance forward with `r` and the regular payment; the last payment may be
  smaller. Extra payments therefore shorten the count (agreed).
- **Money left to pay** = sum of those projected payments.
- **Overpayment left** = money left to pay − current balance.
- **Overpaid so far** = sum of the stored interest parts.
- **Paid off** when the current balance (rounded to the currency) is 0 or
  below; a negative balance shows as 0. The user can also **mark it paid off**
  by hand: this writes a final snapshot with balance 0 as of that day, which
  covers the small leftover that approximate splits always leave. A paid-off
  debt drops out of the budget in months after its last payment; its history
  stays. Paid-off state is display only: it doesn't end the matching range,
  and a later payment or a new snapshot with a balance reopens it. *(my call)*

### Changing what a debt matches (my calls)

- Category, payee, tag and start date are **editable**. After an edit, payments
  that still match keep their stored splits; payments that stop matching lose
  them; newly matching payments are split as late entries. The key and range
  checks above run again. Currency is not editable (delete and recreate).
- **Category merge** (and payee/tag merge): the debt follows to the merge
  target; a merge that would break the checks above is refused with a clear
  error. **Deleting** a category, payee or tag that a debt uses is refused until
  the debt is deleted or repointed.

### Editing (agreed)

The edit form shows the three values. Changing the balance or the payment
amount **recalculates the last payment date** in the form, keeping the current
`r` (so "edit balance → payments left recalculated"); the user may still
overwrite the date. Changing the date recalculates nothing. Saving makes a new
snapshot, deriving a fresh `r` from the three values as saved.

## Budget and Plan

### Debts block (like Savings)

- New budget element type **`ElementDebt = 6`** (persisted, frozen), external
  id = debt id. Debt rows live in their own **Debts** section (own ordering
  group, never in a folder), like `structure.savings`, and stay out of
  `structure.elements` on the wire for older clients.
- A debt row appears in every budget whose participants include the debt's
  owner. **Visibility is the same as savings (agreed):** every participant,
  guests included, sees the row's name, icon and figures, and the budget
  settings say so. Savings rows have no drill-down, so the debt's details
  (snapshot history, payment list with splits) stay owner-only, and only the
  owner edits the debt. *(owner-only details: my reading of "same as savings")*
- Monthly figures, in the debt's currency:
  - **Planned**: the plan (`set-limit`, as for any element; comments work too).
  - **Paid**: matched payments in the month **from the budget's member
    accounts**.
  - **Left**: balance at the month's end, plus payments left. Past months:
    booked. Current month onward: projected, like the savings
    `closingBalance`.
- **Matched payments leave their category row** (and any tag row they would
  otherwise count in) and count only on the debt row (anti-goal 1). A category
  fully covered by a debt therefore shows no spending.
- **Totals:** debt payments stay expenses. Expense totals, Available, Balance
  and every other total are unchanged; the Debts block regroups where they are
  shown. *(my call; the alternative, a separate "Debt payments" outflow line
  like Savings, is a bigger change to totals)*

### Planning extra payments (agreed)

In a month, the scheduled amount = regular payment × scheduled payments in
that month (one for monthly; two or three for biweekly). A plan **above** that
counts the difference as an extra payment: it goes fully to the debt in the
projection, so Left, payments left, the projected payoff date and the
overpayment left all reflect it from that month on. The UI says what it buys
("paid off 14 months earlier, $21,000 less overpayment").

A month with **no plan or a plan below** the scheduled amount still projects
the scheduled payments (the bank collects them anyway); a plan below the
scheduled amount shows in red, like the existing plan hints. *(my call)*

**Plans only inside the debt's life (agreed).** A debt can't be planned in a
month before it starts or after it ends; `set-limit` refuses such a month with
a coded error, the way it refuses months past a budget's end month, and the UI
shows those cells as not plannable. *(my calls on the bounds:)*
- **Start**: the month of the start date; with no start date, the earlier of
  the first matched payment's month and the first snapshot's month (a first
  payment after the snapshot must not drop plans moved at conversion).
- **End**: the month of the latest snapshot's last payment date (the loan's
  term, so planning an extra payment never locks later months). Once the debt
  is paid off, the end is its payoff month instead. If the end moves earlier
  (a new snapshot, a payoff), plans already stored past it stay but are shown
  as out of range and ignored by the projection; they disappear from the
  display, not from the database.

Recurring templates and future-dated transactions that match a debt move to the
debt row as upcoming money, like everywhere else. On a debt row the scheduled
payments are the still-due amount: for each due date not yet paid, the
schedule counts once, whether or not a template also exists for it. The only
red rule on a debt row is "plan below the scheduled amount". *(my call)*
Budget periods follow the budget's period start day when that feature has
landed (#407).

### Converting a category (agreed)

The expense category row's menu gets **"Track as debt"**: the debt form with
the category filled in. On save, the debt and its Debts-block row are created.
When the debt covers the **whole** category (no payee, tag or start date), the
category row's plans and comments move to the debt row in every budget, in the
same transaction; otherwise they stay where they are. *(move rule: my call)*
The form defaults the debt's currency to the category row's currency. If the
user picks another currency, plans don't move. *(my call)*

**Envelopes:** a matched payment leaves its envelope's Spent too, since it
counts only on the debt row. "Track as debt" is offered on a category inside an
envelope as well, but the envelope's plans stay on the envelope. *(my call)*

Deleting a debt (with confirmation) moves them back for a whole-category debt,
otherwise drops the debt row's plans and comments after the same kind of
confirmation savings removal uses. Payments are never touched.

### Managing debts

A debt is opened from its budget row (details: the figures above, snapshot
history, the payments with their split) and from a **Debts** list in Settings,
next to Categories, where debts can also be created without a budget.
*(placement: my call)*

## Constraints

- Follow the repo's patterns (`CLAUDE.md`): a new feature package
  `internal/debt` (service, repo with the sqlite/pgsql adapters, api), budget
  reaching debts only through a port wired in `internal/server`; migrations on
  both engines; `apiparity` / `enginecompare` coverage; all 11 locales; a
  `METRICS` event for create, edit, convert, delete; the regression test plan
  updated.
- The stored splits are not part of the transaction list, so no transaction
  revision bump is needed. If a later version shows the split on a
  transaction, revisions apply.
- Money is `vo.Decimal`; the rate solve may use floats but stored amounts are
  rounded to the currency's fraction digits.
- If it ships in stages, unfinished stages go behind `ECONUMO_DEV` (if that gate
  has landed on `main`; it currently lives on `feature/reports`).

## Left to the builder

- Schema details (debts, snapshots, per-payment splits). Agreed: the debt row also
  carries a copy of the latest snapshot's values (balance, payment, schedule,
  last payment date, as-of date), written with every new snapshot in the same
  transaction; the snapshots stay the history. Endpoint names under
  `/api/v1/debt/`, and the additive `get-budget` / `get-budget-plan` fields.
- When splits are computed (see above), numeric method for `r`.
- English wording; "Overpayment" is the user's term for interest + fees.
- Layout of the debt form, details view and Debts block, following the savings
  section and the house rules (no bold for hierarchy; `PickerSheet` and
  `rankByName` for pickers).
