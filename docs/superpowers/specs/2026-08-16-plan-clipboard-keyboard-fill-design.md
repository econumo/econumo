# Plan sheet: clipboard copy/paste + keyboard fill-right

Date: 2026-08-16
Scope: `web/src/features/budgets/PlanSheet.tsx` (+ tests, one metrics key)

## Goal

Three keyboard shortcuts on the plan sheet's roving single-cell selection:

- **Ctrl/Cmd+C** — copy the selected cell's value to the system clipboard.
- **Ctrl/Cmd+V** — paste a single amount from the clipboard into the selected cell.
- **Shift+→ / Shift+←** — extend/shrink a fill-right range from the selected cell
  (same highlight as the pointer fill-drag); releasing **Shift** commits the source
  value into every covered cell.

Out of scope: multi-cell selection, row/2D range paste, fill-down, window paging
during a keyboard fill.

## Existing pieces reused

- Selection: `PlanSelection { rowKey, col }`, `col -1` = name cell.
- `FillDrag` state + `fill-covered` cell highlight + `fillEnd` commit via
  `useFillPlannedCells` (`BUDGET_PLAN_FILL_RIGHT` metric).
- Single-cell write: `commit()` → `usePlanSetLimit` (`BUDGET_UPDATE_ELEMENT_LIMIT`).
- Amount parsing: `limitAmountFromInput` (`limitAmount.ts`) — the exact rule the
  cell editor applies (empty/0 → clear, formulas allowed, garbage → not ok).
- Guards: `KEYDOWN_ESCAPE_SELECTOR` (keystrokes inside an inner input/popover/
  dialog/menu are never hijacked); `isEditableCell`.

## Design

### A. Copy — native `copy` event

`onCopy` on the `role="grid"` div (fires for Cmd/Ctrl+C, Shift+Insert-equivalents
and the Edit menu whenever the grid itself is focused; needs no clipboard permission).

- No selection, or `e.target` inside `KEYDOWN_ESCAPE_SELECTOR` → do nothing (the
  inner control owns its own copy).
- Name cell (`col === -1`) → clipboard text = element name.
- Month cell → clipboard text = the raw planned decimal string of that cell; an
  unset/unfetched cell copies `"0"` (same rule as the drag source).
- Copy works on every row, editable or not. `clipboardData.setData('text/plain', …)`
  + `preventDefault()`.

### B. Paste — native `paste` event

`onPaste` on the grid, same target guard.

- No selection or name cell → ignore.
- Non-editable month cell (`!isEditableCell`) → `toast.error` with a fixed id
  (`plan-paste-blocked`), message: new key `budgets.page.plan.paste.not_editable`.
- Editable → `limitAmountFromInput(text)`:
  - `ok` → `commit(el.id, month, idx, amount)`; `trackEvent(METRICS.BUDGET_PLAN_PASTE_CELL)`
    (new key `appBudgetPlanPasteCell`, fired at the paste handler's success point).
  - `!ok` → `toast.error` fixed id `plan-paste-blocked`, key `budgets.page.plan.paste.invalid`.
- `preventDefault()` in every handled case.

### C. Keyboard fill — Shift+Arrow

`FillDrag` gains `source: 'pointer' | 'keyboard'`; `startX`/`colWidth` become
irrelevant for keyboard (set to 0). Only one fill can be active at a time.

In `handleKeyDown`, after the existing target guard and only with a valid selection
(a fill needs a source cell):

- `Shift+ArrowRight` on a month cell whose source is editable
  (`isEditableCell(el, month, idx)`), no fill active → start
  `{ source: 'keyboard', rowKey, elementId, amount, startCol: col, targetCol: min(col+1, visible-1) }`
  with `amount` derived exactly like `fillStart` (unset → `"0"`). Fill active →
  `targetCol = min(targetCol+1, visible-1)`. Never pages the window.
- `Shift+ArrowLeft` with a keyboard fill active → `targetCol = max(targetCol-1, startCol)`
  (armed but empty when equal — releasing Shift then writes nothing).
- While a keyboard fill is active: `Escape` cancels; other arrows/Enter are swallowed
  (`preventDefault`, no navigation) — the same modal rule the pointer drag applies.
- Shift+Arrow on the name cell / non-editable source / while a pointer drag is active
  → falls through to existing behaviour (plain navigation is NOT triggered by a
  Shift+Arrow; it is simply ignored so the selection does not jump).

`onKeyUp` on the grid: `e.key === 'Shift'` and an active `keyboard` fill →
`fillEnd()` (existing: writes `startCol+1..targetCol` via `useFillPlannedCells`,
skips unfetched months, metric already fired in the hook), then clear.

`onBlur` on the grid: an active `keyboard` fill is cancelled (Shift released while
focus is elsewhere must never commit). Pointer drags are unaffected.

Highlight: the existing `fill-covered` class over `startCol < i <= targetCol` — no
render changes. Selection stays on the source cell throughout.

### D. Metrics

- New `METRICS.BUDGET_PLAN_PASTE_CELL: 'appBudgetPlanPasteCell'`, fired in the paste
  handler after a successful parse (`commit` is fire-and-forget).
- Copy fires nothing (no server-side effect). Keyboard fill reuses
  `BUDGET_PLAN_FILL_RIGHT` through the shared hook.

### E. i18n

Two new keys in every `locales/<lang>.json`:
`budgets.page.plan.paste.not_editable`, `budgets.page.plan.paste.invalid`.

## Tests (`PlanSheet.test.tsx`)

- Copy on a month cell sets `text/plain` to the planned amount; unset cell → `"0"`;
  name cell → element name; no selection → clipboard untouched.
- Paste `"150"` on an editable cell → set-limit call with `150`; paste `""` → clears
  (`amount: null`); paste `"abc"` → toast, no call; paste on a child/archived row →
  toast, no call; paste inside the open LimitEditor input → grid handler does nothing.
- Shift+→ ×2 then keyup Shift → one fill mutation with two targets, `fill-covered`
  visible on both cells before release; Shift+→ then Shift+← then keyup → no call;
  Shift+→ then Escape → no call and highlight gone; Shift+→ then blur → no call;
  Shift+→ at the last visible column → clamps, window does not page; Shift+→ on the
  name cell / non-editable source → nothing.
- Metrics coverage test picks up `BUDGET_PLAN_PASTE_CELL` automatically.
