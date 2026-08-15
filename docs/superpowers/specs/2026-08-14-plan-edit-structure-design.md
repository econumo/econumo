# Plan sheet: edit structure (currency, folders, ordering)

Date: 2026-08-14
Branch: `feature/budget-plan-view`

## Problem

The plan sheet is read-only. Income categories render only there, so there is no
way to change their currency, group them into folders, or order them. Three
capabilities are missing from the view that is the sole home of income structure.

The budget view has all three behind **Configure → Edit structure**, but it has no
concept of income at all: `isIncomeType` appears only in `planMath.ts` and
`PlanSheet.tsx`, and `BudgetPage.tsx:224` forces `budgetMode = 'budget'` whenever
edit mode turns on. Entering edit mode from the plan sheet currently ejects you
from the view you were trying to edit.

## What already exists

This is frontend work. The backend supports every operation:

- `ChangeElementCurrency` takes any `elementId` with no side or type restriction.
- `MoveElement` takes `{id, folderId, position, afterId}` and is side-agnostic. It
  already **enforces** that a folder cannot mix sides (`sideMixed` / `folderSide`
  in `internal/budget/move.go`), rejecting with `CodeBudgetFolderSideMixed`.
- `CreateEnvelope` takes `Side: "income"`, and `validateEnvelopeCategories`
  rejects mixed-side membership with `CodeBudgetEnvelopeSideMixed`.
- `EnvelopeDialog` already accepts a `side` prop and filters its category picker.

Plan-side row menus and a side-filtered move-to-folder dialog shipped in
`63b11b25` and were removed in `cb5f16a8`. Restoring them is cheaper than
rebuilding; recover from git history rather than writing fresh.

## Design

### 1. Edit mode

One shared `editMode` flag, entered via **Configure → Edit structure** and exited
with the **Done** button — unchanged from today.

1. **Delete the forced redirect** at `BudgetPage.tsx:224`. Entering edit mode keeps
   the current view.
2. **Switching Budget ↔ Plan sets `editMode = false`**, so edit state never carries
   across views.
3. **Plan affordances gate on `editMode`**: drag handles, the row `⋮` menu, and the
   Create folder button appear only in edit mode. With it off, the plan sheet is
   visually unchanged from today.

Budget mode behaviour is otherwise untouched.

### 2. Row actions menu

A `⋮` menu on every plan row in edit mode. Budget mode's menu changes to match, so
one rule holds across both views:

| Element | Menu items |
|---|---|
| Category / tag | Change currency · Move to folder |
| Envelope | Change currency · Move to folder · Edit · Delete\* |

\* Delete only when `canDeleteEnvelope` permits, as today.

Two changes from current behaviour:

- **Change currency becomes universal.** `BudgetPage.tsx:518` currently gates it on
  `element.type !== BudgetElementType.ENVELOPE`, routing envelopes through their
  Edit dialog. That condition is removed in both views.
- **Move to folder is new in budget mode** and restored in plan mode. It opens the
  side-filtered `MoveToFolderDialog` from `63b11b25`: same-side folders, neutral
  folders, and "No folder".

Envelopes keep Edit and Delete because the Edit dialog is the only entry point to
name, icon, archived state, and category membership. Removing it would strand
existing income envelopes, which render only in the plan sheet.

Trigger is the existing `MoreVertical` button with
``aria-label={`element actions ${name}`}``.

### 3. Drag to reorder

Plan rows get drag handles in edit mode, reusing budget mode's machinery
unchanged: `DndContext` + `useSortable`, `PointerSensor` with a 4px activation
distance, and the same `FolderGrip` / element grip components.

- **Elements** reorder within their band and move between folders in the same band,
  via `MoveElement`. This includes moving a loose element into a folder and dragging
  a foldered element back out to the loose list (`folderId: null`). A neutral
  (memberless) folder accepts a drop from either band — the drop is what gives it a
  side, after which it is bound to that band.
- **Folders** reorder among themselves via `order-folders`.

Two constraints, enforced at the drop target so the user never sees a failed save:

1. **No cross-band element drags.** An element's side comes from its type, not its
   placement. The server already rejects mixing with `CodeBudgetFolderSideMixed`;
   the client refuses the drop so the error never fires.
2. **No cross-band folder drags.** A folder's side is derived from its members and
   `order-folders` persists position only, so a cross-band drag would appear to
   work and snap back on reload.

`⋮` → Move to folder remains the non-drag path, which also covers touch.

### 4. Create folder

A **Create folder** button in plan mode's edit bar, mirroring `BudgetPage.tsx:637`.

Its dialog has three parts:

1. **Name** — required, same validation as the existing create-folder form.
2. **Income / Expense switch** — selects which side to pick members from.
3. **Element list** — elements on the selected side, multi-select. Flipping the
   switch replaces the list and clears any selection from the other side, so a
   folder can never be created mixed.

At least one member is required. This is load-bearing: `folderSides()` derives a
memberless folder as `'neutral'`, and `bucketPlanRows` places neutral folders in
the **expense** band (`planMath.ts:133`, "A memberless (neutral) folder defaults to
the expense area for display"). An empty folder intended for income would render
under Expenses and could not be dragged out, because folder side is not stored.
Requiring a member means the folder lands in the correct band immediately.

On submit: `create-folder`, then `move-element` per selected element.

Budget mode's Create folder button is unchanged (name only) — everything in that
view is expense-sided, so the neutral-folder trap does not arise.

### 5. Income envelopes

Creation is not offered anywhere. Income envelopes are technically supported
end-to-end, but an envelope's purpose is holding a limit, and income has no limits
— folders cover the grouping need without a second overlapping mechanism.

Existing income envelopes (the feature shipped in `63b11b25`) **still render** and
stay fully manageable through the row menu. They are not hidden; hiding them would
make money disappear from the totals.

Accepted trade-off: folders show no subtotal row, so an income group has no rolled-up
figure. The Income grand total still covers the whole side.

## Testing

- Edit mode: entering it from plan mode stays in plan mode; switching views clears it.
- Row menu: the four-item envelope menu and two-item category menu, in both views;
  Change currency present on an envelope (the condition being removed).
- Move to folder: an income element is offered income and neutral folders only.
- Drag: an element drag across bands is refused; a folder drag across bands is refused.
- Create folder: submitting with no members is blocked; flipping the side switch
  clears the previous side's selection; a folder created with an income member
  renders in the income band.
- Income envelope creation is absent from both views; an existing income envelope
  still renders and its Edit/Delete items work.

## Out of scope

- Storing `side` on the folder record (a DB migration). Revisit if empty,
  speculatively-created income folders turn out to be needed.
- Subtotal rows for folders.
- Income envelope creation.
- Any change to budget mode's Create folder dialog.
