# Plan grid redesign (desktop/tablet)

Status: approved in brainstorming 2026-10-06. Branch `feature/plan-grid-redesign` from
`v1.6-dev`; PR against `v1.6-dev`.

## Goal

The desktop/tablet **Plan** view gets the same structure, look and controls as the
redesigned **Budget** view (#308, #312). The two views differ only in how many months
they show: Budget shows one, Plan shows several side by side. The Plan grid must be
easy to read and edit, and feel close to a spreadsheet.

Out of scope: the phone (unchanged), backend changes (none needed; `set-limit` already
clears a cell with `amount: null`), a long-term "Goals" view.

## 1. Page layout and header

- **Shared header.** Both views render the same row: `Budget · Plan │ ‹ › month strip`
  (`ViewSwitch` + `PeriodStrip`). The selected month is the one in
  `useBudgetPeriodStore.selectedDate`, shared by both views, so switching keeps it.
  Plan's separate window state (`planFirstMonth`, `setPlanFirstMonth`, the Plan-only
  `‹ ›` nav and its month labels) is removed.
- **Window.** The grid shows `n = planVisibleCount(width)` months (max 12, as today). The
  selected month is column 2 with **one month of history** to its left; the other
  columns are future months. When only one column fits, it is the selected month. When the
  history month would fall before the budget's start month, the window starts at the
  start month and the selected month is column 1. When the window would run past an ended
  budget's end month, it ends there. Clicking a strip month moves the window; the strip's
  `‹ ›` pan the strip only, as in Budget.
- **Grid frame.** A sticky month-header row sits under the strip: the month labels in the
  small uppercase style of Budget's column headings, aligned over their columns. The
  name column has a fixed width. The selected month's column is tinted top to bottom.
  There are hairlines between rows and no boxed folder cards, no bold, and no horizontal
  scroll (the month count fits the width).
- **Order.** Income (folders, rows, income Uncategorized) → Savings (accounts) →
  Expenses (folders, No folder, Uncategorized, Archived) → totals lines → a sticky
  Balance line at the bottom.
- **Sections and folders** use Budget's `MonthSectionHeader` and `FolderLine`. Because
  the month heading is sticky at the top, section and folder lines carry **per-month
  sums** whether open or folded; they never repeat headings. Fold state is shared with
  Budget (`planFolds` keys: `income`, `savings`, `expense`, folder ids, `__no_folder__`,
  `archived`, `__income<id>`).

## 2. Rows and cells

- **One line per row**, at Budget's row height, indents (folder → row → envelope child),
  icon and name. A row in a non-budget currency shows `CurrencyTag` next to its name; the
  `$` tail column is removed.
- **Cell content**, right-aligned with tabular figures: `actual · plan`.
  - Months before the selected one and the selected month: `actual · plan`.
  - Months after the selected month: `plan` only.
  - Unplanned month: blank plan (never `0.00`); no actual yet: `— · 55`.
  - The plan figure is foreground; the actual is smaller and muted, and red only when
    `overBudget()` (budgetMath) says the row is over budget and not covered by earlier
    months. Never green; savings are never red.
- **Per section:**
  - Income: received · planned.
  - Savings: saved · planned. The per-row balance line is removed; balances live on the
    Total savings line.
  - Expenses: spent · budget.
  - The carry-over lead-in (`38.47 + 40.00`) stays a Budget-only detail.
- **Folder and section lines:** per-month sums in the same `actual · plan` form, muted,
  regular weight.
- **Totals block**, built from the same `TotalLine` parts as Budget's
  `MonthTotalsLines`: Income, Expenses, Savings, Transfers (only when ≠ 0), Total
  savings, then the sticky Balance line. Past and current months show actuals; future
  months show the projection. A negative balance is red, as today.
- **Column width.** A month column is sized to fit `12,345.67 · 12,345.67` with the
  smaller actual. When a column is narrower than that, past months drop the actual before
  the grid drops a month, so a plan figure is never truncated.
- **Hover.** The row highlights as in Budget, and the hovered cell gets a light outline.
  The ⋮ menu and drag grip appear on hover (§3).

## 3. Editing and structure

### Cell editing (desktop, spreadsheet behavior)

- **Click** selects a cell (ring). The arrow keys move the selection; the name column is
  reachable as today.
- **In-cell editor**, which replaces the `LimitEditor` popover in the Plan grid:
  - typing a digit, `-`, `.` or `,` starts editing and replaces the value with what was
    typed;
  - **F2**, **Enter** or a **double-click** edit the current value, caret at the end.
  - The input sits in the cell at the cell's width.
- **While editing:**
  - Enter commits and moves down.
  - Tab / Shift+Tab commit and move right / left.
  - ↑ / ↓ commit and move.
  - ← / → move the caret.
  - Esc cancels.
  - Clicking another cell commits.
  - An invalid value keeps the editor open with the existing validation message.
  - Committing an unchanged value sends nothing.
- **Delete / Backspace** on a selected (not editing) cell clears the plan (`set-limit`
  with `amount: null`); the cell goes blank.
- **Unchanged:**
  - Ctrl/⌘+C / V (single cell);
  - the fill handle and Shift+→ fill;
  - Shift+Enter / Shift+F2 comments;
  - the hover comment corner;
  - Enter on the name cell opening the element's edit dialog.
- **Window edges.** → past the last column or ← past the first moves the selected month
  by one (window and strip move together), clamped at the budget's start and end.
- **Read-only cells** (archived, before the start, no write access) can be selected but
  never open an editor; Delete does nothing on them.

### Transactions

Clicking the small **actual** figure opens that row's transactions for that month (the
existing `BudgetTransactionsDialog`), as clicking Spent / Received / Saved does in
Budget. This applies to every row the list supports (all but income Uncategorized) and
to the Transfers totals line, as today.

### Structure

- The same as the Budget view: hover ⋮ menus on section, folder and row lines (with the
  "(no access)" / "(not empty)" reasons), and hover drag grips for rows, folders and
  envelope children (`MonthDrag` parts, including into/out of envelopes).
- The menu builders (`editAction`, `structureActions`, `classificationActions`,
  `folderActionsFor`, `sectionMenu`, …) and the drag handlers move out of `BudgetPage`
  into a shared hook, so both views offer identical actions.
- Removed from Plan:
  - the desktop edit mode;
  - Plan's own `RowMenu`, `MoveToFolderDialog`, `PlanCreateFolderDialog`,
    `PlanSortableRow`/`PlanSortableFolder`/`PlanFolderGrip` and `PlanBand`;
  - the Configure chooser on desktop. Configure opens Budget settings directly in both
    views.
- **Tablet:** tapping a cell opens the item sheet, as today. Configure → Edit structure
  turns on the shared "always show controls" mode (`LineControlsContext` = `always`), as
  in the Budget view on tablet.

## Analytics

- In-cell commits fire the existing `BUDGET_UPDATE_ELEMENT_LIMIT`; fills and pastes keep
  `BUDGET_PLAN_FILL_RIGHT` / `BUDGET_PLAN_PASTE_CELL`.
- An edge-paging month shift fires `BUDGET_PLAN_CHANGE_WINDOW`.
- New: `BUDGET_PLAN_CLEAR_CELL: 'appBudgetPlanClearCell'`, fired on a successful
  Delete/Backspace clear.

## Architecture

- **Column layout context.** `monthLayout.ts` gains a layout context: `budget` (today's
  three fixed figure columns) or `plan` (N month columns of equal width plus the tinted
  selected column). `MonthSectionHeader`, `FolderLine` and the totals lines take their
  figures as an array of cells and render them through the context, so both views share
  one set of line components.
- **PlanSheet** keeps its data (`useBudgetPlan`, `planMath`), selection, keyboard, fill,
  copy/paste and comments logic, but renders through the shared lines. The in-cell
  editor is a small new component (`PlanCellInput`) owned by the grid.
- **Shared hook** (`useBudgetLineMenus`, plus the drag handlers next to it) builds the
  menus and handles drops for both views; `BudgetPage` and `PlanSheet` consume it.
- `planFirstMonth` is removed from `budgetStore`, and a persisted value is ignored. The
  window derives from `selectedDate` + width + the budget's start and end.

## Testing

- Existing `PlanSheet` tests are ported to the new structure: keyboard grid, fill,
  paste, comments, savings, folds and the guest read-only cases.
- New tests:
  - the window derivation (history month, start/end clamping, one-column case);
  - typing to edit, F2/Enter/double-click edit, Enter/Tab/arrow commit-and-move, Esc;
  - Delete clearing (and doing nothing on read-only cells);
  - edge paging moving the selected month;
  - cells showing `actual · plan` only up to the selected month;
  - red actual only when over and uncovered;
  - clicking an actual opening transactions;
  - shared menus present in Plan;
  - Configure opening settings directly on desktop.
- `docs/regression-test-plan.md`: the Plan view items are updated for the new header,
  window, cell format, in-cell editing and hover menus/drag.
- `metrics-coverage.test.ts` must see `BUDGET_PLAN_CLEAR_CELL` fired.
