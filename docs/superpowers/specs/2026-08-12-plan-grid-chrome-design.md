# Plan sheet grid chrome: sticky scope, header consolidation, ruled rows, hover crosshair

Date: 2026-08-12
Branch: `feature/budget-plan-view`

## Problem

Three issues with the plan sheet as it stands:

1. **Too much is pinned.** All four totals rows (Income / Expenses / Net / Balance)
   are one `sticky bottom-0` block, costing four rows of vertical space on every
   screen for numbers that are consulted occasionally rather than tracked
   continuously.
2. **Two full-width lines of chrome.** The Бюджет/План mode toggle and the
   "Скрывать пустые строки" switch each occupy their own line above the grid,
   while the page header has unused horizontal room.
3. **The grid does not read as a grid.** Rows float with gaps and rounded corners
   rather than sitting flush with dividers, and there is no hover feedback to
   help track a row across months or confirm which month column you are in.

## Design

### 1. Totals split — trio scrolls, Balance stays pinned

`TotalsFooter` splits into two components:

- **`PlanTotals`** — Income / Expenses / Net. Rendered as the last child *inside*
  the scrolling content, after the archived section. Not sticky. Retains
  `data-testid="plan-totals"` and its three `role="row"` children in order.
- **`PlanBalanceRow`** — Balance alone. `sticky bottom-0 z-10 bg-background
  border-t`, still inside the scroll container so sticky resolves against it.
  New `data-testid="plan-balance-row"`. Retains the per-cell
  `data-testid={plan-balance-${i}}` hooks.

Both share `gridCols`, so columns stay aligned with the rows above.

Visual separation between the two (the trio is a distinct block when you scroll
to the end, not visually fused with the pinned Balance): the trio gets a top
border and a small top margin; the pinned Balance keeps its own top border.

**Rationale.** Balance is the "where do I end up" figure worth tracking while
scanning rows. Income/Expenses/Net are a summary consulted deliberately, so they
do not earn permanent screen space.

### 2. Header consolidation

Header becomes one stable line:

```
[back?] TITLE [currency pills — budget mode only] ——— [Бюджет|План] [Настроить]
```

In `BudgetPage.tsx`:

1. **Currency pills become mode-scoped** — wrap the `budgetCurrencyIds.map(...)`
   span in `budgetMode === 'budget' &&`. `selectedCurrencyId` only feeds
   `ExpenseWidget`, which renders in budget mode only, so the pills are already
   inert in plan mode; this removes a dead control rather than changing behavior.
   `selectedCurrencyId` state is otherwise unchanged.
2. **Mode toggle moves into the header**, between the flex spacer and Настроить.
   Markup unchanged — same `role="tablist"` / `role="tab"` / `aria-selected` and
   the same labels. The standalone `<div role="tablist">` block is deleted.
3. **Hide-empty becomes a `DropdownMenuCheckboxItem`** in the Настроить dropdown,
   rendered only when `budgetMode === 'plan'`, with `checked={hideEmpty}` and
   `onCheckedChange={togglePlanHideEmpty}`. The menu closes on toggle (default
   behavior; no `onSelect` preventDefault) — it is a one-shot preference, so
   staying open would be the surprising option. `BudgetPage` picks up the two
   store hooks.

In `PlanSheet.tsx`: the toolbar div holding the Switch + Label is deleted, along
with the `Switch` / `Label` imports if otherwise unused. `PlanSheet` continues to
read `planHideEmpty` from the store and no longer owns the toggle.

**Rationale.** `planHideEmpty` is already persisted in `budgetStore`, so it is
set-once-and-forget; permanent header space for a long Russian label buys little.
Demoting it keeps the header to one stable line at every width.

**Net effect:** two full-width lines reclaimed; the month header row moves up
directly beneath the page header.

### 3. Ruled rows

Rows currently float (`flex flex-col gap-1` on the bands, `rounded-md` per row).
To read as a ledger they sit flush and share hairline dividers, while the tinted
income/expense bands are preserved.

1. Income and expense `<section>` elements drop `gap-1`. Bands keep their tint,
   rounding, and padding.
2. `ElementRow` gets `border-b border-border/60` and drops `rounded-md`.
   Low contrast is deliberate — full-strength dividers on a dense grid create a
   cage effect that fights the hover highlight.
3. Trailing divider suppressed at the end of each band via the existing marker
   classes, which currently have no CSS rule behind them:
   ```css
   .plan-band-income > :last-child,
   .plan-band-expense > :last-child { border-bottom: 0 }
   ```
   This avoids threading an `isLast` prop through `FolderRows` into `ElementRow`.
4. Folder sub-sections keep their grouping; their rows get the same flush
   treatment internally so a folder reads as a ruled block.

Section headers and the archived section are unchanged.

**Side effect:** closing `gap-1` tightens vertical rhythm, fitting more rows per
screen — which partly offsets the totals trio now requiring a scroll.

### 4. Row and column hover

**Mechanism.** Each row is its own CSS grid, so cells in a column are siblings
only by index. Rather than per-column DOM writes or a generated stylesheet:

- The grid container gets `data-hover-col={index}`, set from one `onMouseOver` /
  `onMouseLeave` pair that reads the index off `event.target.closest('[data-col]')`.
- Each month cell renders `data-col={i}` — a markup addition in `ElementRow`, the
  month header, and both totals blocks.
- `web/src/index.css` carries twelve static rules (the maximum visible months):
  ```css
  [data-hover-col="0"] [data-col="0"], … [data-hover-col="11"] [data-col="11"] { … }
  ```

No per-row DOM writes, no React re-render on pointer move — one attribute write
per column change.

**Visuals (hover always yields):**

- **Row:** strengthened to a clearly visible tint across the whole row, including
  the name cell — starting point `bg-accent/60` (up from the current `/50`),
  applied to the row wrapper so the name cell is covered.
- **Column:** a very faint wash, deliberately weaker than the row — starting
  point `accent` at roughly `/15`. Its job is confirming which month, which the
  highlighted header largely does already.

Both values are starting points to be confirmed against the running app in light
and dark mode; the invariant is the ordering — column strictly fainter than row,
and both fainter than selection and fill.
- **Precedence:** hover sits below selection (`aria-selected`), fill-drag
  (`bg-ring/15`), and the current-month tint, so none of the four existing tint
  meanings are muddied. On the current-month column the added hover is subtle —
  an accepted trade-off.
- The month header cell for the hovered column also highlights, so the crosshair
  terminates somewhere meaningful.

**Touch:** all hover rules are wrapped in `@media (hover: hover)` so tapping on
mobile does not leave sticky highlight state.

## Testing

Existing coupling is light — two touchpoints across `PlanSheet.test.tsx` and
`BudgetPage.test.tsx`:

- `PlanSheet.test.tsx:77` clicks the mode tab via `getByRole('tab', {name: /plan/i})`.
  Unaffected: the toggle keeps its role and labels when it moves into the header.
- The `plan-totals` test destructures `getAllByRole('row')` as
  `[incomeRow, expensesRow, netRow]` and asserts computed values. Unaffected by
  the split, since `plan-totals` keeps exactly those three rows.
- **One edit required:** that test's `within(totalsBlock).getByText('Balance')`
  moves to target `plan-balance-row`. The substantive assertions (computed totals
  and balance values) are untouched.

The hide-empty switch is not asserted anywhere, so relocating it into the
dropdown breaks no test.

New coverage to add:

- Balance row is pinned and the totals trio is not (assert the two testids are
  separate elements, and that `plan-balance-row` carries the sticky class).
- Hide-empty is reachable from the Настроить menu in plan mode and absent in
  budget mode.
- Currency pills render in budget mode and not in plan mode.

## Out of scope

- Changing what the currency pills do in budget mode.
- Removing the current-month column tint (considered, rejected — not asked for).
- Dropping the income/expense band tinting in favor of a plain ruled sheet
  (considered, rejected — recent deliberate work in `361daf76`).
