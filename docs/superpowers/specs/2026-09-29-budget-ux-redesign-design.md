# Budget & Plan UX Redesign — Design

Date: 2026-09-29
Status: Approved in brainstorming, pending written-spec review
Base branch: `v1.6-dev` (the savings block from #285/#291 is part of the baseline)
Supersedes: the interim fix in draft PR #292 (a "Comments (N)" button inside the
phone set-limit sheet) — that change is folded into Part 2 below.

## Overview

An audit of the Budget and Plan views (seeded demo budget; 390 / 820 / 1440 px)
found three problems:

1. **Phone:** the two views look like different apps (cards and pills vs. stacked
   unlabelled numbers), every row carries hidden gestures (tap the Available pill
   = set budget, tap Spent = transactions, long-press = set budget), the budgeted
   amount is invisible, and the Budget/Plan switch is buried in the settings menu.
2. **Comments** are bolted onto the amount editor. On a phone the thread inside
   the set-budget sheet pushed the amount out of view above the keyboard; on
   desktop the corner marker behaves differently per view.
3. **Desktop** is overwhelming: every number has the same weight, ~20 green pills
   compete, a `$` column repeats on every row, and the Plan grid shows `—` over
   `0.00` in every future cell.

This redesign is **frontend only**. No endpoint, DTO, permission, or analytics
contract changes.

## Decisions made during brainstorming

| Topic | Decision |
|---|---|
| Phone row interaction | One tap on a row opens the **item sheet**; all other row gestures go |
| Phone modes | **Single month view** on phones — no Budget/Plan switch |
| Phone row numbers | **Budget** and **Spent**; Available moves into the item sheet |
| Row colour | Driven by Available: neutral / amber (over this month, covered by carry-over) / red (Available < 0) |
| Income on phone | One collapsed summary row at the top, expands to income rows |
| Comments | Fully separate from the amount editor, both views, all viewports |
| Starting a thread | Right-click menu (desktop); long-press actions modal (tablet); item sheet (phone) |
| Desktop density | One emphasised number per view, colour only for problems, drop repeated chrome |
| Currency | No symbol on budget-currency amounts; budget currency named once in the column-heading row; foreign-currency items get a tag next to the name |
| Header currency chips + "Spending progress" widget | **Removed**; the average-rate note moves into the item sheet |

## Breakpoints

- **Phone:** below Tailwind `sm` (< 640 px). Gets the single month view (Part 1).
- **Tablet:** 640–1023 px (`useIsCompact()` true, `sm` true). Keeps the Budget and
  Plan table layouts with Part 3's density changes; touch interactions per Part 2.
- **Desktop:** ≥ 1024 px. Budget and Plan views with Part 2 + Part 3.

The phone view is selected by a new `useIsPhone()` hook (`(max-width: 639px)`),
mirroring `useIsCompact`. Existing `useIsCompact` call sites keep their meaning
(dialog vs. popover presentation) on tablets.

## Row state rule (shared by all viewports)

For an expense element in a month, with `budget`, `spent`, `available` as today:

| State | Condition | Signal |
|---|---|---|
| none | `budget` is zero and `spent` is zero | no bar |
| ok | `available ≥ 0` and `spent ≤ budget` | neutral Spent, neutral bar |
| covered | `available ≥ 0` and `spent > budget` | **amber** Spent and bar |
| over | `available < 0` | **red** Spent and bar; Available shown as a red pill where visible |

- Progress bar value is `min(spent / budget, 1)`; hidden when `budget` is zero.
- Future months (after the current month) have no Spent: shown as `—`, no bar,
  state `none`.
- Carry-over shown in the sheet = `available − (budget − spent)`.
- The rule is one pure function (`rowState(element)`) in `budgetMath`, unit-tested
  against the table above, used by the phone rows, the desktop rows, and the sheet.

Income and savings rows do not use amber/red: received above plan and saved above
plan are not problems.

## Currency display rule (all views, all viewports)

1. Amounts in the budget currency never carry a symbol — rows, folder headers,
   savings rows, totals, plan cells.
2. The budget currency code is shown once, at the left end of the column-heading
   row (`USD    BUDGET   SPENT   AVAILABLE`); on the phone in the single heading
   row above the list; on the Plan grid at the left of the month header row.
3. An element whose currency differs from the budget's gets a small tag next to
   its name (`Travel [EUR]`); its row amounts are in that currency. The item sheet
   repeats the code beside each amount.
4. Folder and grand totals are already converted to the budget currency (rule 1).

The trailing symbol column (`BudgetTable.tsx` rows/folders/totals,
`SavingsBlock.tsx`, `PlanSheet.tsx` row tail) is removed.

## Part 1 — Phone single month view

Replaces both `BudgetPage` modes below 640 px. `/budget` and `/plan` both render
it on a phone (URLs keep working; no redirect).

### Screen, top to bottom

- **Header:** budget name (normal case, smaller than today's all-caps), settings
  menu. No mode switch, no currency chips.
- **Month strip:** the Budget view's scrollable `PeriodStrip`. Past, current and
  future months share the screen.
- **Heading row** (once): `USD` at the left, `BUDGET` and `SPENT` above the two
  number columns.
- **Income summary row** (collapsed by default, state kept for the session):
  `Income · {received} of {planned}`. Expands into the income rows (same row
  component; labels Planned / Received in the sheet). The income "Uncategorized"
  row appears only when non-zero.
- **Expense folder cards:** header = folder name + Budget sum + Spent sum; rows as
  below. The unfoldered bucket is labelled "No folder" (was "Default folder").
- **Savings card:** Planned and Saved columns; row tap opens the sheet (which adds
  the month-end Balance).
- **Totals card:** Expenses (`spent of budget`), Savings (`saved of planned`),
  Available (total, incl. carry-over), Balance at month end, Total savings;
  Transfers only when non-zero.

### Expense row

```
[icon] Groceries          700.00   801.37 ◤
       ▓▓▓▓▓▓▓▓▓▓▓▓▓░ (amber)
```

- Spent coloured per the row state rule; thin progress bar under the name.
- `◤` = comment indicator only (not a tap target on phone).
- Envelopes/tags with children: the chevron expands; the row itself opens the sheet.
- Whole row is one button (≥ 44 pt tall) with an accessible name
  `{name}, budget {x}, spent {y}`.

### Item sheet (`ElementSheet`, bottom sheet)

```
Groceries · September
Budget 700.00   Spent 801.37   Available 649.32
Over by 101.37 — covered by 750.69 left from earlier months
💬 Next month back to 700 — Dmitry        Comments (2) ›
[ Set budget ]   [ Transactions ]
```

- State sentence per row state (none for `ok`/`none`; "covered by …" for amber;
  "Overspent by …" for red).
- Latest comment preview (author + text, one line) and a `Comments (N)` link; with
  no comments the link reads "Add comment" (hidden when the thread is read-only
  and empty).
- **Set budget** → existing `SetLimitDialog` (amount keypad only). Hidden for
  guests, readonly access, archived budgets, out-of-range months, deleted
  elements, uncategorized.
- **Transactions** → existing `BudgetTransactionsDialog` for that month.
- Foreign-currency element: each amount shows its code, plus
  `≈ {converted} {budgetCode} · avg rate for {month}: 1 {budgetCode} = {rate}`
  (the note formerly in the Spending progress widget).
- Income row: Planned / Received. Savings row: Planned / Saved / Balance at
  month end.
- Sheets replace one another (sheet → set budget / comments → back closes to the
  list), never stack.

### Removed on phone

Tap-Available-to-set-budget, tap-Spent-to-transactions, long-press row, the
Budget/Plan radio in the settings menu. Edit-structure mode (drag to reorder)
is unchanged.

### Data

- Expenses, savings, per-element Available: `get-budget` for the selected month
  (as the Budget view today).
- Income planned/received, Balance at month end, Total savings: `get-budget-plan`
  with a one-month window (`from` = selected month, `months=1`), reusing
  `planMath` (`balanceRow`, savings split) exactly as the Plan view does.
- Comments: `useBudgetComments` for the selected month (unchanged).

## Part 2 — Comments, separate from the amount editor

### Everywhere

- `LimitEditor` loses its `footer`; both `CommentsFooter` components
  (`BudgetPage.tsx`, `PlanSheet.tsx`) and `commentsAutoExpandKey` are deleted.
- `SetLimitDialog` loses comments entirely (PR #292's button is removed; on the
  phone comments are reached from the item sheet).
- The thread UI (`CommentThread`) is unchanged in behaviour; it is presented in
  one of two shells: `CommentsPopover` (anchored, desktop/tablet) or
  `CommentsSheet` (bottom sheet, phone). In the sheet the list scrolls and the
  composer is pinned to the bottom so the keyboard never hides context.

### Desktop (mouse) — Budget and Plan views

- **Corner marker:** kept on commented cells; visible triangle 8 px, hit area
  20 px.
- **Hover preview:** pointer resting ~300 ms on a commented cell shows a
  read-only `HoverCard`: latest two comments (author, date, text) and "N more".
  Closes on leave. Not shown while the amount popover or thread popover is open.
- **Click marker:** opens `CommentsPopover` anchored to the cell. Esc closes and
  returns focus to the cell.
- **Click amount:** amount editor only.
- **Right-click cell:** `ContextMenu` with Set budget, Add comment /
  Comments (N), Show transactions. Items the caller cannot use are omitted
  (guest: Comments + Transactions only; uncategorized: Transactions only).
- **Plan grid keyboard:** Shift+F2 opens the thread for the selected cell;
  Shift+Enter kept as an alias; plain Enter unchanged.
- **Non-editable cells:** clicking the amount opens `CommentsPopover` (replacing
  today's mix of popover and dialog).

### Tablet (touch, 640–1023 px)

- No hover preview.
- Tap marker → `CommentsPopover`.
- Long-press a cell → a **modal** titled with the item's name, listing the same
  actions as the desktop right-click menu (Set budget, Comments (N) / Add
  comment, Show transactions) as full-width buttons — the phone's item-sheet
  pattern rather than a small popup menu. The finger's release must not also
  trigger the cell's tap action.

### Phone

Per Part 1: indicator only; item sheet → `CommentsSheet`.

## Part 3 — Calmer desktop and tablet

### Budget view

- Available is the emphasised column (semibold); Budget and Spent regular weight.
- No green pills: Available is plain text; a red pill only when negative. Spent
  coloured per the row state rule; thin progress bar under the item name.
- Currency per the currency display rule (symbol column removed).
- Folder header: subtle tinted band, semibold name and totals in the same
  columns, more space between folders. Unfoldered bucket labelled "No folder".
- Title: normal case, smaller (both views).

### Plan view

- Actuals only where they exist: past/current months show the small actual above
  the plan; future months show only the plan.
- A month with no plan is blank (no `0.00`); clicking still edits.
- Actual coloured red only when over plan; the green under-plan colouring is
  removed.
- Current month column gets a light background tint.
- Currency per the currency display rule. Transfers total row and the income
  "Uncategorized" row appear only when non-zero.

### Deliberately unchanged

Amounts keep their decimals (alignment and exactness). Column structure, keyboard
grid, fill handle, drag-and-drop, edit-structure mode.

## Removals

- Header currency chips and `ExpenseWidget` ("Spending progress") — its spending
  progress is covered by the rows and totals, "saved of planned" by the totals
  card, and the average-rate note moves to the item sheet. Delete
  `ExpenseWidget.tsx`, `selectedCurrencyId` state, and the
  `budgets.modal.expense_widget.*` catalogue keys in all 11 locales except
  `conversion_rate`, which stage 2's item sheet reuses.
- `ElementLongPress` in `BudgetPage.tsx`, `onAvailableClick`,
  `onAvailableCommentsClick`, and the phone-only `renderRowWrapper` branch.

## i18n

New keys (all 11 catalogues, `en` as reference): item-sheet labels and state
sentences, income summary row, "Add comment", context-menu items, "No folder"
for the monthly view's unfoldered bucket (reuse the plan view's existing
`no_folder` value "No folder"). Removed keys per Removals. `i18ntest` guards
catch gaps.

## Analytics

No new user-facing actions: set limit, comment create/update/delete keep their
existing `METRICS` events at the same choke points. Opening a sheet or menu is
navigation, not an action. `metrics-coverage.test.ts` must stay green.

## Testing

- `rowState` unit tests covering every row of the state table plus future months.
- Currency rule: a foreign-currency element shows the tag and no symbol column;
  budget-currency rows show no symbol (Budget, Plan, phone).
- Phone view (`matchMedia` < 640): row tap opens the item sheet; Set budget,
  Comments and Transactions route correctly; guest sheet lacks Set budget;
  income row collapses/expands; `/plan` renders the same view.
- Comments: amount popover has no comments UI; marker opens `CommentsPopover`;
  hover preview appears after the delay and not while a popover is open;
  context menu items per role; Shift+F2 and Shift+Enter open the thread; tablet
  long-press opens the menu.
- Plan view: future cells show no actual; empty plan cells render blank; current
  month tinted.
- Existing comment/savings suites updated where they assert removed entry points
  (`comments.monthly`, `comments.plan`, `comments.plan.guest`, `BudgetPage`,
  `SavingsBlock`, `PeriodStrip`).
- `docs/regression-test-plan.md`: rewrite the comment, savings-edit and expense-
  widget items; add phone single-view and item-sheet items (📱).

## Delivery

Three stages, each a reviewable PR into `v1.6-dev`:

1. **Comments separation** (Part 2) + removal of the currency chips/widget.
   Phones keep today's comment paths (the set-limit sheet's "Comments (N)"
   button, tap-Available, long-press) until stage 2 gives them the item sheet;
   long-press becomes phone-only so it cannot clash with the tablet menu.
2. **Phone single month view** (Part 1), including the item sheet.
3. **Desktop/tablet density** (Part 3) + the currency display rule.

Stage 1 closes the original bug (amount hidden by the thread) on its own.

## Out of scope

Server changes; changing what Available means (carry-over semantics); a
multi-month plan editor on phones; hiding decimals; comment features
(mentions, reactions, notifications).

**Budget configuration** (budget details, edit-structure mode, folder/envelope
management, the income/expense sides, lifecycle and sharing on
`/settings/budgets`, a budget switcher) is a separate design, brainstormed on its
own. This redesign leaves edit-structure mode and the Configure menu working as
they are (minus the mode radio and currency chips it removes), and invests
nothing new in them, since that design may move structure editing out of the
tables.
