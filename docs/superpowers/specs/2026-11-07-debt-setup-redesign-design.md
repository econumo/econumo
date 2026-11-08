# Debts: short creation, label matching, and a full-screen review

**Date:** 2026-11-07 · **Status:** design, awaiting approval · builder-checked
**Builds on:** `docs/superpowers/specs/2026-10-29-budget-debts-design.md` (branch
`feature/budget-debts`, PR #424, not released). Everything not changed here stays
as that design and the code on the branch describe it.

## Why

Testing the demo, the debt form felt long and unclear. It asks for eleven things at
once, and nothing says that category, payee and tag are how Econumo finds the loan's
payments: they read like properties of the loan. The details sheet is a small
dialog, while the loan deserves a screen of its own.

## Goals

1. Creating a debt asks only for what identifies it: **name**, **which payments
   count**, and **payment + frequency**.
2. "Which payments count" reads as a sentence: payments in a category, optionally
   from a payee, with a tag, with a label.
3. A debt can exist **without terms** (no balance, no last payment date). It still
   collects its payments; it just doesn't count down yet. A **Finish setup** action
   fills in the rest later.
4. A **full-screen review screen** (phone and desktop) replaces the details sheet,
   leading with **payments left** and progress, and showing all of the debt's info.

## Non-goals

- Matching without a category: category stays required.
- "Track as debt" on tag rows: it stays on category rows (*my call*).
- Any change to the split, projection or attribution rules of the base design,
  except where a debt has no terms (below).

## 1. Creating a debt (one short screen)

Opened from a category row's "Track as debt" (category prefilled) or from
Settings → Debts.

- One line on top: "Track a loan without an account. You can fine-tune it
  afterwards."
- **Name** (prefilled with the category name when converting).
- **Payments from**, a sentence of pickers:
  "Payments in **[category]** · from **[any payee]** · tagged **[any tag]** ·
  labelled **[any label]**". Category is required, the rest optional. Pickers use
  `PickerSheet` + `rankByName`. The line under it: "Econumo finds this loan's
  payments with these. They move from the category to this debt."
- **Payment** and **Repeats** (weekly, biweekly, monthly, quarterly, yearly) side by
  side. Both are required.
- **Currency** as a compact line at the bottom, defaulting to the category row's
  currency when converting, else the user's currency (*my call*).
- **Add** saves and opens the review screen.

Balance, last payment date and start date are **not** on this form. They are on
the review screen's terms form (Finish setup / Edit). `create-debt` keeps accepting
an optional `startDate`; when a save is refused because another debt has the same
filters (`debt.start_date_taken` / `debt.key_conflict`), the form reveals a start
date field so a second loan in the same category can be told apart (*my call*).

The whole-category plan move on conversion keeps its current rule: plans and
comments move when the debt has no payee, tag, label or start date and the currency
matches. A debt created without terms is whole-category in that sense.

## 2. Label matching (new)

- A debt may name one **label** (`label_id`, optional, like payee and tag).
- A transaction matches when its category is the debt's, its payee matches (when
  set), its tag matches (when set), and it **carries** the debt's label among its
  labels (when set; `transactions_labels`).
- **Labels can tell two debts apart, with a rule (agreed).** Two debts on one
  category whose (payee, tag) keys overlap may coexist when **both** name a label and
  the labels differ (e.g. "Loans" with label "Car" and label "Student"). Any other
  overlap is refused as today (`debt.key_conflict`), including one debt with a label
  and one without. Identical keys, label included, still need distinct start dates.
- **Tiebreaker:** a transaction that matches more than one debt (it carries both
  debts' labels) belongs to the **older debt** (earlier `created_at`, then `id`). The
  sync assigns each payment to exactly one debt by that order, so a payment never
  counts twice; `debts_payments.transaction_id` stays unique.
- Deleting a label a debt uses is refused (`label.used_by_debt`); merging a label
  moves the debt to the target, like payee and tag merges.
- A label changed on a transaction re-matches it, like any other edit: the sync
  removes or adds the split. Changing a transaction's labels must also bump the
  account's `transactions_revision` as it already does (`ReplaceLabels`); no new
  rule.
- Recurring templates match the label the same way (they carry `labelIds`).

## 3. A debt without terms ("not set up")

A debt is **set up** once it has a balance and a last payment date. Until then:

- It stores name, matching filters, currency, **payment** and **schedule**. Balance,
  last payment date, as-of date and start date are empty. No snapshot exists yet.
- **It collects its payments:** matching, attribution, Paid in Budget/Plan and the
  drill-downs work exactly as for a set-up debt. Payments move off the category.
- **No split yet:** its payments have no interest/principal split (no snapshot
  means no rate). When setup is finished, the first snapshot is written and the
  payments are split by the existing rules (those before the snapshot by the
  backward estimate).
- **Hidden figures:** current balance, payments left, money left, overpayment, payoff
  date and Left are not shown (the wire sends them empty, `paymentsLeft` 0, plus an
  explicit `isSetUp` 0/1). The review screen and Settings list show **Finish setup**
  instead. The budget row shows Planned and Paid, and "Finish setup" in place of
  Left (owner) or a dash (others).
- **No scheduled amount:** `scheduledTotal` and `upcoming` are 0, so there is no
  "plan below the payment" warning (*my call*: without a last payment date there is
  no due-date anchor).
- **Planning bounds:** start = the start date's period if set, else the earlier of
  the first matched payment's period and the period the debt was **created** in, both
  before and after setup (this replaces "first snapshot" in the base rule, so
  finishing setup later never moves the start and hides plans); no end until set up
  (*my call*).
- **Mark as paid off** is not offered until set up.
- **Finish setup** and **Edit** open the same terms form: what you owe now, payment,
  repeats, last payment date, start date. On a debt that is not set up, payment and
  repeats come prefilled; saving with balance and last payment date filled finishes
  setup; saving without them only updates payment, repeats and start date on the
  debt row (no snapshot) (*my call*). Saving writes the first snapshot as of
  today with the stored payment and schedule and copies the terms onto the debt, as
  every snapshot write does today.

Schema: the unreleased migration `20261108000000` on this branch may be edited in
place (it was never released): `as_of`, `balance` and `last_payment_date` on
`debts` become nullable; `label_id` (nullable, FK to `labels`, indexed) is added.
`payment` and `schedule` stay required.

## 4. The review screen (full screen, phone and desktop)

Replaces `DebtDetailsSheet`. Opened from the budget row's Details, from Settings →
Debts, and right after creating a debt. Owner only, as now.

- **Header:** debt name, close, and a menu with Mark as paid off and Delete.
- **Top (set up):** a large **"283 payments left"**, a progress bar **"77 of 360
  paid"**, and below it **owed now** and **"Paid off by Jul 2050"**. "Paid" counts the
  **due periods that received a payment**, up to today (an extra payment or a second
  payment in the same period doesn't add one; payments from before setup count when
  the filters cover them); the total is paid + left. A debt set up mid-loan with no
  recorded history honestly shows "0 of 283" (*my call*). Paid off: a
  "Paid off" state instead.
- **Top (not set up):** the debt name, what has been paid so far, and a prominent
  **Finish setup** button with one line on why ("Add what you owe and when the loan
  ends to see payments left").
- **Loan** card: all the debt's info: owed now, payment and how often, next payment,
  **loan ends** (the last payment date, labelled so it isn't read as the most
  recent payment), start date, money left to pay, overpayment so far, overpayment still
  to come, currency. An **Edit** button opens the terms form (balance, payment,
  repeats, last payment date, start date), with the existing behaviours: editing
  balance or payment recalculates the last payment date with the current rate.
- **Which payments count:** the same sentence of pickers as on the create form,
  editable in place; saving re-matches as edits do today.
- **Payments:** date, amount, interest, principal, newest first (no split columns
  while not set up).
- **History:** the earlier balances entered, collapsed by default.

The screen is a full-screen dialog on both breakpoints, scrollable, following the
house rules (no bold for hierarchy; cards, colour and size instead). Settings →
Debts list rows show payments left and owed now, or "Finish setup".

## Analytics

Existing `DEBT_CREATE` / `DEBT_CONVERT` / `DEBT_UPDATE` / `DEBT_DELETE` /
`DEBT_MARK_PAID_OFF` keep firing at the same points; add `DEBT_FINISH_SETUP` when
the first terms are saved.

## Left to the builder

- Exact wire field names beyond `isSetUp` and `labelId`, and how the empty figures
  are encoded (empty strings, matching the base design's "" for no date).
- Layout details of the review screen, the progress bar component, the sentence
  pickers' visual form.
- Copy, in all 11 locales.
- The regression test plan and CLAUDE.md updates.
