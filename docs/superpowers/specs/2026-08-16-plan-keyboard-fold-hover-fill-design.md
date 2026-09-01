# Plan view: keyboard fold/unfold, selectable folders, hover fill handle

**Date:** 2026-08-16 · **Branch:** `feature/plan-sheet-keyboard-fold-hover-fill` · **Scope:** `web/` only

## Problem

Two friction points in the plan sheet (`web/src/features/budgets/PlanSheet.tsx`):

1. Folding is mouse-only and coarse. An envelope's whole name (chevron + icon +
   text) is one button, so a click meant to highlight the row springs its
   breakdown open; the keyboard reaches an envelope's name cell but can only
   toggle it with Space, and a folder header is not reachable at all.
2. The fill handle (the corner dot that drags a plan value across months) only
   renders on the *selected* cell, so copying a value costs a click first.

## Rules

### Folding by keyboard

- **Envelope name click selects only.** Just the chevron button (with
  `aria-expanded` and the Expand/Collapse title) toggles the children.
- On a **highlighted name cell (col −1) of a row with children**:
  `ArrowRight` unfolds a collapsed row — only when already unfolded does it move
  to the first month; `ArrowLeft` folds an unfolded row — only when already
  collapsed does it page the window back (the existing behaviour). Rows without
  children navigate exactly as before. Space keeps toggling.
- **Folder headers are selectable rows.** Clicking the header still folds/unfolds
  (as today) and additionally selects it (`pfolder:<id>` in the selection's
  rowKey space). On a highlighted folder: `ArrowRight` unfolds when folded,
  `ArrowLeft` folds when unfolded (each a no-op otherwise — no window paging),
  `Enter`/`Space` toggle. `ArrowUp`/`ArrowDown` walk headers and rows in visual
  order and keep the selection's column, so a trip through a header lands back
  on the same month. Neutral (member-less) folders are in the order too.
- The header cell carries the same selection ring/`aria-selected` as a name
  cell; `aria-activedescendant` and the scroll-into-view logic resolve a folder
  selection to its single (−1) cell.

### Fill handle on hover

- The handle also renders on an **editable month cell under the mouse**, whether
  or not another cell is selected (and when nothing is). Same gates as before
  otherwise (editable, fetched cell, not compact, >1 visible month).
- The in-flight drag's **source cell keeps its handle mounted** after the pointer
  leaves it (the handle holds the pointer capture; unmounting it would drop the
  `pointerup` that commits the fill).
- **On release, the source cell becomes the selection** (and the grid takes
  focus), so the keyboard picks up where the mouse left.

## Implementation

- `FlatRow` becomes a `kind: 'element' | 'folder'` union; `buildFlatRows` pushes a
  folder entry before each folder's visible members (income, neutral, expense).
- `handleKeyDown` branches on `entry.kind`; the element branch consults
  `unfoldedElements` for the name-cell Left/Right fold rule.
- `FolderRows` gets `role="row"` on the header, a `role="gridcell"` name group
  with the selection id/ring, and calls `ctx.select` on both click paths.
- `ElementRow` keeps a row-local `hoverCol` state (`onMouseEnter`/`onMouseLeave`
  per month cell) so a hover re-renders one row, not the grid; `fillEnd` calls
  `select(rowKey, startCol)`.

## Tests

`PlanSheet.test.tsx`: name click selects without expanding / chevron toggles;
ArrowRight/ArrowLeft fold rules on an envelope name cell; folder header
selection + fold keys + column preservation; hover handle presence, survival of
the drag source, and source selection after the drag. Existing tests that
expanded by clicking the name text now click the chevron, and the keyboard-order
tests account for the folder header entry.
