# Plan Sheet Edit Structure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make income structure editable by extending Configure → Edit structure to the plan sheet: universal change-currency, move-to-folder in both views, drag ordering, and a create-folder dialog that requires members.

**Architecture:** Frontend only — every backend operation already exists and is side-aware. Much of the work restores components removed in `cb5f16a8` (recover from `63b11b25`). Edit mode stays a single shared `editMode` flag; the plan sheet gates new affordances on it and clears it when the view switches.

**Tech Stack:** React 19, TypeScript, Tailwind 4, shadcn/ui (Radix), @dnd-kit, vitest + Testing Library, oxlint.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-14-plan-edit-structure-design.md`.
- Branch: `feature/budget-plan-view` (already checked out; do not create a new branch).
- Run the frontend suite from `web/`: `pnpm test`. Lint: `pnpm lint`.
- **`pnpm test` does NOT typecheck.** Run `pnpm build` before every commit — it runs `tsc -b`. A prior task in this repo shipped a type error the suite did not catch.
- Comments: this repo's CLAUDE.md requires comments only for non-obvious rationale. Do not add comments restating what code does.
- The i18n keys `budgets.page.plan.menu.move_to_folder` and `budgets.page.plan.menu.no_folder` already exist in all 11 catalogues — do NOT add them. Any genuinely new key must be added to all 11 (`locales/*.json`) or the Go guard `go test ./internal/test/i18ntest/` fails.
- Product analytics rule (CLAUDE.md): every new user-facing action fires a `METRICS` event. Check `web/src/lib/metrics.ts` before adding one; `metrics-coverage.test.ts` fails if a `METRICS` key is never fired.
- Do NOT offer income **envelope** creation anywhere. Existing income envelopes must keep rendering and stay editable.
- Follow TDD: write the failing test, watch it fail, implement, watch it pass, commit.

---

### Task 1: Scope edit mode to the current view

**Files:**
- Modify: `web/src/features/budgets/BudgetPage.tsx` (the `editMode` effect at ~line 223)
- Test: `web/src/features/budgets/BudgetPage.test.tsx`

**Interfaces:**
- Consumes: nothing (first task).
- Produces: `editMode` no longer forces the view to budget mode, and switching views clears it. Later tasks gate plan-sheet affordances on this same flag, passed into `PlanSheet` as a new `editMode: boolean` prop (added in Task 3).

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/BudgetPage.test.tsx`:

```tsx
it('keeps edit structure in the current view and clears it when the view switches', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByRole('tab', { name: /budget/i })

  // enter plan mode, then turn on edit structure: it must NOT bounce back to budget
  await user.click(screen.getByRole('tab', { name: /plan/i }))
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  expect(useBudgetPeriodStore.getState().budgetMode).toBe('plan')
  expect(screen.getByRole('button', { name: 'Done' })).toBeInTheDocument()

  // switching views turns edit mode off
  await user.click(screen.getByRole('tab', { name: /budget/i }))
  expect(screen.queryByRole('button', { name: 'Done' })).not.toBeInTheDocument()
})
```

Confirm the accessible names first — `Configure`, `Edit structure` and `Done` come from
`budgets.page.budget.settings.button`, `...menu.edit_structure` and
`...menu.edit_structure_done`. Look them up in `locales/en.json` and use the real strings.

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- BudgetPage.test.tsx -t 'keeps edit structure in the current view'`
Expected: FAIL — `budgetMode` is `'budget'`, because the effect forces the redirect.

- [ ] **Step 3: Delete the redirect and clear edit mode on view switch**

In `BudgetPage.tsx`, delete this effect entirely:

```tsx
  // shows the budget table, never the plan sheet
  useEffect(() => {
    if (editMode) {
      setBudgetMode('budget')
    }
  }, [editMode, setBudgetMode])
```

Then find the mode toggle's button `onClick` (the `role="tab"` buttons, currently
`onClick={() => setBudgetMode(m)}`) and clear edit mode alongside the switch:

```tsx
              onClick={() => {
                setEditMode(false)
                setBudgetMode(m)
              }}
```

- [ ] **Step 4: Run tests to verify they pass**

Run from `web/`: `pnpm test -- BudgetPage.test.tsx`
Expected: PASS. If an existing test asserted the old redirect, it was asserting the
behaviour this task removes — update it to match, and say so in the commit body.

- [ ] **Step 5: Typecheck, then commit**

```bash
cd web && pnpm build 2>&1 | tail -3
```
Expected: `✓ built`, no `error TS`. Then:

```bash
git add web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/BudgetPage.test.tsx
git commit -m "feat(web): keep edit structure in the current view"
```

---

### Task 2: Universal change-currency + move-to-folder in budget mode

**Files:**
- Modify: `web/src/features/budgets/BudgetPage.tsx` (`elementActions`, ~lines 510-534)
- Test: `web/src/features/budgets/BudgetPage.test.tsx`

**Interfaces:**
- Consumes: nothing from Task 1 (independent).
- Produces: the budget-mode row menu shape that Task 4 mirrors in the plan sheet:
  - category/tag → `Change currency`, `Move to folder…`
  - envelope → `Change currency`, `Move to folder…`, `Edit`, `Delete` (Delete only when `canDeleteEnvelope(budget.meta, user?.id)`)
  - Move-to-folder uses the existing `moveElement` mutation with `{budgetId, item: {id, folderId, position: 0, afterId: null}}`.

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/BudgetPage.test.tsx`:

```tsx
it('offers change currency on every element, and move to folder', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByRole('tab', { name: /budget/i })
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  // an ENVELOPE previously had no Change currency item — only Edit/Delete
  await user.click(await screen.findByRole('button', { name: 'element actions Living' }))
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Move to folder…' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
})
```

`Living` is the envelope in the budget fixture; confirm with
`grep -n "Living" src/test/fixtures.ts`. Use the real `en` strings for the menu labels
(`budgets.page.budget.structure.element.action.change_currency`,
`budgets.page.plan.menu.move_to_folder`, `common.button.edit.label`).

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- BudgetPage.test.tsx -t 'offers change currency on every element'`
Expected: FAIL — no `Change currency` item on an envelope.

- [ ] **Step 3: Rewrite `elementActions`**

In `BudgetPage.tsx`, replace the whole `elementActions` menu content. The current version
gates Change currency on `element.type !== BudgetElementType.ENVELOPE`; that condition goes.

```tsx
  const elementActions = (element: BudgetElementDto) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" aria-label={`element actions ${element.name}`}>
          <MoreVertical className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => setCurrencyTarget(element)}>
          {t('budgets.page.budget.structure.element.action.change_currency')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setMoveFolderTarget(element)}>
          {t('budgets.page.plan.menu.move_to_folder')}
        </DropdownMenuItem>
        {element.type === BudgetElementType.ENVELOPE ? (
          <>
            <DropdownMenuItem onSelect={() => setEnvelopeDialog({ open: true, envelope: element, folderId: element.folderId })}>
              {t('common.button.edit.label')}
            </DropdownMenuItem>
            {canDeleteEnvelope(budget.meta, user?.id) ? (
              <DropdownMenuItem variant="destructive" onSelect={() => setDeleteEnvelopeTarget(element)}>
                {t('common.button.delete.label')}
              </DropdownMenuItem>
            ) : null}
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
```

- [ ] **Step 4: Add the move-to-folder state and dialog**

Next to the other dialog state (near `const [currencyTarget, setCurrencyTarget] = useState...`):

```tsx
  const [moveFolderTarget, setMoveFolderTarget] = useState<BudgetElementDto | null>(null)
```

Budget mode is entirely expense-sided, so its picker needs no side filter. Render it
beside the other dialogs (near the `PromptDialog`s at the end of the component):

```tsx
      {moveFolderTarget ? (
        <ResponsiveDialog
          open
          onOpenChange={(o) => !o && setMoveFolderTarget(null)}
          title={t('budgets.page.plan.menu.move_to_folder')}
        >
          <ul className="flex max-h-72 flex-col overflow-y-auto scrollbar-slim">
            {budget.structure.folders.map((f) => (
              <li key={f.id}>
                <button
                  type="button"
                  className="w-full truncate rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover"
                  onClick={() => {
                    moveElement.mutate({
                      budgetId: budget.meta.id,
                      item: { id: moveFolderTarget.id, folderId: f.id, position: 0, afterId: null },
                    })
                    setMoveFolderTarget(null)
                  }}
                >
                  {f.name}
                </button>
              </li>
            ))}
            <li>
              <button
                type="button"
                className="w-full rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover"
                onClick={() => {
                  moveElement.mutate({
                    budgetId: budget.meta.id,
                    item: { id: moveFolderTarget.id, folderId: null, position: 0, afterId: null },
                  })
                  setMoveFolderTarget(null)
                }}
              >
                {t('budgets.page.plan.menu.no_folder')}
              </button>
            </li>
          </ul>
        </ResponsiveDialog>
      ) : null}
```

`ResponsiveDialog` is imported from `@/components/ResponsiveDialog`; add the import if
absent. `moveElement` is `useMoveElement()` — check whether `BudgetPage` already calls it
(`grep -n "useMoveElement\|moveElement" BudgetPage.tsx`) and add the hook only if missing.

- [ ] **Step 5: Run tests, typecheck**

Run from `web/`: `pnpm test -- BudgetPage.test.tsx && pnpm build 2>&1 | tail -3`
Expected: PASS and `✓ built`.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/BudgetPage.test.tsx
git commit -m "feat(web): change currency on every element, add move to folder"
```

---

### Task 3: Restore the plan row menu behind edit mode

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (add `RowMenu` + `MoveToFolderDialog`, thread `editMode`)
- Modify: `web/src/features/budgets/BudgetPage.tsx` (pass `editMode` to `<PlanSheet>`)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`

**Interfaces:**
- Consumes: `editMode` from Task 1. `PlanSheet` gains a prop: `editMode: boolean` on `PlanSheetProps`.
- Produces: `GridCtx` gains three fields used by Task 5's drag work:
  - `editMode: boolean`
  - `onChangeCurrency: (el: PlanElementDto) => void`
  - `onMoveToFolder: (el: PlanElementDto) => void`

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/PlanSheet.test.tsx`:

```tsx
it('shows plan row actions only in edit mode, with side-filtered move-to-folder', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // read-only by default: no row menus
  expect(screen.queryByRole('button', { name: /element actions/i })).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'element actions Living' }))
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Move to folder…' }))

  // pe1/Living is expense-sided: expense + neutral folders only, never income ones
  const dialog = await screen.findByRole('dialog', { name: 'Move to folder…' })
  expect(within(dialog).getByRole('button', { name: 'Essentials' })).toBeInTheDocument()
  expect(within(dialog).getByRole('button', { name: 'No folder' })).toBeInTheDocument()
  expect(within(dialog).queryByRole('button', { name: 'Bonuses Folder' })).not.toBeInTheDocument()
})
```

Folder names come from the plan fixture — confirm with
`grep -n "Essentials\|Bonuses Folder\|Misc Folder" src/test/fixtures.ts`.

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx -t 'shows plan row actions only in edit mode'`
Expected: FAIL — no `element actions Living` button exists.

- [ ] **Step 3: Add the two components**

In `PlanSheet.tsx`, add above `ChildRow`. These are recovered from `63b11b25` with the
menu contents updated to match Task 2:

```tsx
function RowMenu({ el, ctx }: { el: PlanElementDto; ctx: GridCtx }) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon" className="size-5 shrink-0" aria-label={`element actions ${el.name}`}>
          <MoreVertical className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => ctx.onChangeCurrency(el)}>
          {t('budgets.page.budget.structure.element.action.change_currency')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => ctx.onMoveToFolder(el)}>
          {t('budgets.page.plan.menu.move_to_folder')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// Side-filtered folder picker: an income element may only land in an income or
// neutral folder. The server enforces this too (CodeBudgetFolderSideMixed); the
// filter keeps the user from ever seeing that error.
function MoveToFolderDialog({
  target,
  folders,
  folderSideMap,
  onClose,
  onPick,
}: {
  target: PlanElementDto | null
  folders: BudgetFolderDto[]
  folderSideMap: Map<Id, FolderSide>
  onClose: () => void
  onPick: (folderId: Id | null) => void
}) {
  const { t } = useTranslation()
  if (!target) {
    return null
  }
  const side: 'income' | 'expense' = isIncomeType(target.type) ? 'income' : 'expense'
  const targets = folders.filter((f) => {
    const s = folderSideMap.get(f.id) ?? 'neutral'
    return s === side || s === 'neutral'
  })
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={t('budgets.page.plan.menu.move_to_folder')}>
      <ul className="flex max-h-72 flex-col overflow-y-auto scrollbar-slim">
        {targets.map((f) => (
          <li key={f.id}>
            <button
              type="button"
              className="w-full truncate rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover"
              onClick={() => onPick(f.id)}
            >
              {f.name}
            </button>
          </li>
        ))}
        <li>
          <button type="button" className="w-full rounded-md px-2 py-2 text-left text-sm hover:bg-econumo-hover" onClick={() => onPick(null)}>
            {t('budgets.page.plan.menu.no_folder')}
          </button>
        </li>
      </ul>
    </ResponsiveDialog>
  )
}
```

Restore these imports at the top of `PlanSheet.tsx` (all were removed in `cb5f16a8`):

```tsx
import { ChevronDown, ChevronLeft, ChevronRight, MoreVertical } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { BudgetDto, BudgetFolderDto, BudgetMetaDto, PlanChildDto, PlanElementDto } from '@/api/dto/budget'
import { folderSides, ... } from './planMath'   // add folderSides to the existing import list
import type { FolderSide, ... } from './planMath'  // add FolderSide to the existing type import
```

Keep the existing members of those import lists; only add what is missing.

- [ ] **Step 4: Thread edit mode and the handlers**

Add to `PlanSheetProps`:

```tsx
export interface PlanSheetProps {
  /** the ALREADY-LOADED budget (meta for permissions/currency); plan data is fetched inside */
  budget: BudgetDto
  currencies: CurrencyDto[]
  userId: Id | undefined
  editMode: boolean
}
```

Destructure it: `export function PlanSheet({ budget, currencies, userId, editMode }: PlanSheetProps) {`

Add to the `GridCtx` interface:

```tsx
  editMode: boolean
  onChangeCurrency: (el: PlanElementDto) => void
  onMoveToFolder: (el: PlanElementDto) => void
```

Add the state and the folder-side map inside `PlanSheet`:

```tsx
  const [moveFolderTarget, setMoveFolderTarget] = useState<PlanElementDto | null>(null)
  const [currencyTarget, setCurrencyTarget] = useState<PlanElementDto | null>(null)
  const moveElement = useMoveElement()
  const folderSideMap = useMemo(() => (plan ? folderSides(plan) : new Map<Id, FolderSide>()), [plan])
```

`useMoveElement` comes from `./queries` — add it to that import list.

Add the three fields to the `ctx` object and to its dependency array:

```tsx
      editMode,
      onChangeCurrency: setCurrencyTarget,
      onMoveToFolder: setMoveFolderTarget,
```

Render `RowMenu` in `ElementRow`'s name cell, after the name span, gated on edit mode.
`isUncategorized` already exists in that scope:

```tsx
          {!isUncategorized && ctx.editMode ? <RowMenu el={el} ctx={ctx} /> : null}
```

Render the dialog near `SetLimitDialog` at the end of the component:

```tsx
      <MoveToFolderDialog
        target={moveFolderTarget}
        folders={plan.structure.folders}
        folderSideMap={folderSideMap}
        onClose={() => setMoveFolderTarget(null)}
        onPick={(folderId) => {
          if (moveFolderTarget) {
            moveElement.mutate({
              budgetId: budget.meta.id,
              item: { id: moveFolderTarget.id, folderId, position: 0, afterId: null },
            })
          }
          setMoveFolderTarget(null)
        }}
      />
```

For the currency picker, reuse `CurrencyPickerDialog` — the same component `BudgetPage`
renders for its own `currencyTarget` (read `BudgetPage.tsx` around line 820 for the exact
props). The mutation is `useChangeElementCurrency()` from `./queries` (verified):

```tsx
  const changeCurrency = useChangeElementCurrency()
```

and on pick:

```tsx
            changeCurrency.mutate(
              { budgetId: budget.meta.id, elementId: currencyTarget.id, currencyId },
              { onSuccess: () => setCurrencyTarget(null) },
            )
```

- [ ] **Step 5: Pass `editMode` from BudgetPage**

In `BudgetPage.tsx`, the `<PlanSheet>` call site:

```tsx
        <PlanSheet budget={budget} currencies={currencies} userId={user?.id} editMode={editMode} />
```

- [ ] **Step 6: Run tests, typecheck**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx BudgetPage.test.tsx && pnpm build 2>&1 | tail -3`
Expected: PASS and `✓ built`.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/PlanSheet.test.tsx
git commit -m "feat(web): restore plan row actions behind edit mode"
```

---

### Task 4: Create folder from the plan sheet

**Files:**
- Create: `web/src/features/budgets/PlanCreateFolderDialog.tsx`
- Modify: `web/src/features/budgets/PlanSheet.tsx` (edit bar button + wiring)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`

**Interfaces:**
- Consumes: `editMode` from Task 3.
- Produces: a new component:
  ```tsx
  export function PlanCreateFolderDialog({ open, elements, onClose, onSubmit }: {
    open: boolean
    elements: PlanElementDto[]
    onClose: () => void
    onSubmit: (form: { name: string; memberIds: Id[] }) => void
  })
  ```
  `elements` is the full unfiltered element list; the dialog does its own side filtering.

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/PlanSheet.test.tsx`:

```tsx
it('creates a plan folder with members, switching sides clears the selection', async () => {
  let folderBody: unknown
  const moves: unknown[] = []
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/create-folder', async ({ request }) => {
      folderBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'nf1', name: 'Employment', position: 9 } } })
    }),
    http.post('*/api/v1/budget/move-element', async ({ request }) => {
      moves.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'Create folder' }))
  const dialog = await screen.findByRole('dialog', { name: 'Create folder' })

  // submit is blocked with no members
  await user.type(within(dialog).getByLabelText('Name'), 'Employment')
  expect(within(dialog).getByRole('button', { name: 'Create' })).toBeDisabled()

  // default side is expense; switch to income and pick an income element
  await user.click(within(dialog).getByRole('tab', { name: 'Income' }))
  await user.click(within(dialog).getByRole('checkbox', { name: 'Salary' }))
  expect(within(dialog).getByRole('button', { name: 'Create' })).toBeEnabled()

  // flipping back to expense clears the income selection
  await user.click(within(dialog).getByRole('tab', { name: 'Expenses' }))
  expect(within(dialog).getByRole('button', { name: 'Create' })).toBeDisabled()

  await user.click(within(dialog).getByRole('tab', { name: 'Income' }))
  await user.click(within(dialog).getByRole('checkbox', { name: 'Salary' }))
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))

  await waitFor(() => expect(folderBody).toMatchObject({ name: 'Employment' }))
  await waitFor(() => expect(moves).toHaveLength(1))
  expect(moves[0]).toMatchObject({ id: 'ie1', folderId: 'nf1' })
})
```

`Salary`/`ie1` is the income element in the plan fixture — confirm with
`grep -n "ie1\|Salary" src/test/fixtures.ts` and substitute the real id and name.
The `Income`/`Expenses` labels reuse `budgets.page.plan.section.income` / `...section.expenses`;
`Name`, `Create` and `Create folder` reuse `budgets.form.budget.folder_name.label`,
`common.button.create.label` and `budgets.modal.create_folder_form.header`. Look each up
in `locales/en.json` and use the real strings — no new i18n keys are needed.

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx -t 'creates a plan folder with members'`
Expected: FAIL — no `Create folder` button in the plan sheet.

- [ ] **Step 3: Create the dialog**

Create `web/src/features/budgets/PlanCreateFolderDialog.tsx`:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { PlanElementDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { elementDisplayName } from './budgetMath'

type Side = 'income' | 'expense'

export function PlanCreateFolderDialog({
  open,
  elements,
  onClose,
  onSubmit,
}: {
  open: boolean
  elements: PlanElementDto[]
  onClose: () => void
  onSubmit: (form: { name: string; memberIds: Id[] }) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [side, setSide] = useState<Side>('expense')
  const [picked, setPicked] = useState<Id[]>([])

  // A folder's side is derived from its members, and a memberless folder renders
  // in the expense band whatever the user intended — so a member is required, and
  // switching sides clears the selection rather than letting a folder mix sides.
  const options = elements.filter(
    (el) => el.id !== UNCATEGORIZED_ID && el.isArchived === 0 && (isIncomeType(el.type) ? 'income' : 'expense') === side,
  )
  const switchSide = (next: Side) => {
    setSide(next)
    setPicked([])
  }
  const close = () => {
    setName('')
    setSide('expense')
    setPicked([])
    onClose()
  }
  if (!open) {
    return null
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && close()} title={t('budgets.modal.create_folder_form.header')}>
      <form
        className="flex flex-col gap-3"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim() !== '' && picked.length > 0) {
            onSubmit({ name: name.trim(), memberIds: picked })
          }
        }}
      >
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="plan-folder-name">{t('budgets.form.budget.folder_name.label')}</Label>
          <Input id="plan-folder-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>

        <div role="tablist" aria-label="folder side" className="flex w-fit rounded-md border p-0.5">
          {(['expense', 'income'] as const).map((s) => (
            <button
              key={s}
              type="button"
              role="tab"
              aria-selected={side === s}
              className={`rounded px-3 py-1 text-sm ${side === s ? 'bg-accent font-bold' : 'text-muted-foreground'}`}
              onClick={() => switchSide(s)}
            >
              {t(s === 'income' ? 'budgets.page.plan.section.income' : 'budgets.page.plan.section.expenses')}
            </button>
          ))}
        </div>

        <ul className="flex max-h-64 flex-col gap-1 overflow-y-auto scrollbar-slim">
          {options.map((el) => {
            const label = elementDisplayName(el.id, el.name, t)
            return (
              <li key={el.id} className="flex items-center gap-2">
                <Checkbox
                  id={`plan-folder-member-${el.id}`}
                  aria-label={label}
                  checked={picked.includes(el.id)}
                  onCheckedChange={(v) => setPicked((prev) => (v ? [...prev, el.id] : prev.filter((p) => p !== el.id)))}
                />
                <Label htmlFor={`plan-folder-member-${el.id}`} className="truncate font-normal">
                  {label}
                </Label>
              </li>
            )
          })}
        </ul>

        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={close}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={name.trim() === '' || picked.length === 0}>
            {t('common.button.create.label')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
```

`Checkbox`, `Input` and `Label` all exist under `src/components/ui/` (verified). If the
`Checkbox` prop shape differs from the snippet above, mirror the category picker in
`src/features/budgets/EnvelopeDialog.tsx`, which solves the same problem.

- [ ] **Step 4: Wire it into the plan sheet**

In `PlanSheet.tsx`, add state and the mutations:

```tsx
  const [createFolderOpen, setCreateFolderOpen] = useState(false)
  const createFolder = useCreateBudgetFolder()
```

The hook is `useCreateBudgetFolder` (verified — NOT `useCreateFolder`); add it to the
existing `./queries` import list.

Add the edit bar above the month header, rendered only in edit mode:

```tsx
      {editMode ? (
        <div className="flex items-center gap-2 px-2 pb-1">
          <Button type="button" variant="secondary" size="sm" onClick={() => setCreateFolderOpen(true)}>
            {t('budgets.page.budget.structure.action.create_folder')}
          </Button>
        </div>
      ) : null}
```

Render the dialog beside the others. Folder creation and the member moves are two calls;
the moves run after the folder id comes back:

```tsx
      <PlanCreateFolderDialog
        open={createFolderOpen}
        elements={plan.structure.elements}
        onClose={() => setCreateFolderOpen(false)}
        onSubmit={({ name, memberIds }) => {
          const id = uuidv7()
          createFolder.mutate(
            { budgetId: budget.meta.id, id, name },
            {
              onSuccess: () => {
                for (const memberId of memberIds) {
                  moveElement.mutate({ budgetId: budget.meta.id, item: { id: memberId, folderId: id, position: 0, afterId: null } })
                }
                setCreateFolderOpen(false)
              },
            },
          )
        }}
      />
```

`uuidv7` comes from `import { v7 as uuidv7 } from 'uuid'` — restore that import (it was
removed in `cb5f16a8`).

- [ ] **Step 5: Run tests, typecheck**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx && pnpm build 2>&1 | tail -3`
Expected: PASS and `✓ built`.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/budgets/PlanCreateFolderDialog.tsx web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx
git commit -m "feat(web): create plan folders with members"
```

---

### Task 5: Drag to reorder in the plan sheet

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Test: `web/src/features/budgets/PlanSheet.test.tsx`

**Interfaces:**
- Consumes: `editMode` and `ctx.editMode` from Task 3; `moveElement` already added there.
- Produces: nothing later tasks depend on (final task).

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/PlanSheet.test.tsx`:

```tsx
it('shows drag handles only in edit mode', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  expect(screen.queryByRole('button', { name: /^move / })).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  expect(screen.getAllByRole('button', { name: /^move / }).length).toBeGreaterThan(0)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx -t 'shows drag handles only in edit mode'`
Expected: FAIL — no move handles exist.

- [ ] **Step 3: Add the drag context and handles**

Mirror `BudgetPage.tsx`'s setup. Read lines 72-130 of that file for `DraggableElement`,
`preferRowCollisions`, `FolderHandleContext` and `FolderGrip`, and reuse the same
structure. Import from `@dnd-kit/core` and `@dnd-kit/sortable` exactly as `BudgetPage` does.

The sensor:

```tsx
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
```

Wrap each band's rows in a `SortableContext` scoped to that band, so a drag can never
cross the divider — the bands are separate contexts, which is what enforces constraint 1
of the spec. Wrap element rows in a `DraggableElement`-style sortable that renders a grip
with `aria-label={`move ${name}`}` when `ctx.editMode` is true, and nothing otherwise.

On drag end, persist with the mutation already wired in Task 3:

```tsx
    moveElement.mutate({
      budgetId: budget.meta.id,
      item: { id: activeId, folderId: targetFolderId, position: 0, afterId },
    })
```

Derive `afterId` with `afterIdFromDrop`, already exported from `@/lib/ordering` and used
by `BudgetPage.tsx:436`. For element (not folder) drags, `elementMove.ts` exports the
helpers `arrangementFromBuckets`, `moveElementInArrangement` and `arrangementItem` that
`BudgetPage` composes — read its `handleDragEnd` and reuse the same sequence rather than
deriving positions by hand.

Keep folder drags within their band for the same reason: `order-folders` persists position
only, so a cross-band folder drag would snap back on reload.

- [ ] **Step 4: Run the full suite and typecheck**

Run from `web/`: `pnpm test && pnpm build 2>&1 | tail -3`
Expected: PASS and `✓ built`. Keyboard-navigation tests are the likely casualties — the
sortable wrapper adds DOM depth, so any test using `closest('[data-row-id]')` may need
its query adjusted. Adjust the query, never the behaviour.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx
git commit -m "feat(web): drag to reorder plan rows in edit mode"
```

---

### Task 6: Full verification

**Files:** none modified unless a regression surfaces.

- [ ] **Step 1: Frontend suite**

Run from `web/`: `pnpm test`
Expected: PASS. Watch `BudgetTable.test.tsx` and `budgetDetail.test.tsx` — they render
`BudgetPage` and may assert on the old menu shape from Task 2.

- [ ] **Step 2: Typecheck and lint**

Run from `web/`: `pnpm build && pnpm lint`
Expected: `✓ built` with no `error TS`; lint at its pre-existing warning count (21 —
all `only-export-components` in files this work does not touch). A count above 21 means
this work introduced one.

- [ ] **Step 3: i18n guard**

Run from the repo root: `go test ./internal/test/i18ntest/`
Expected: `ok`. This catches a `t()` call whose key is missing from any of the 11
catalogues. Every key this plan uses already exists, so a failure means a typo.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A web/src
git commit -m "test(web): update budget view tests for the plan edit structure"
```

## Self-Review Notes

- **Spec coverage:** §1 edit mode → Task 1. §2 row menu → Tasks 2 (budget) and 3 (plan).
  §3 drag → Task 5. §4 create folder → Task 4. §5 income envelopes → no task needed:
  creation is already absent after `cb5f16a8`, and existing envelopes keep rendering
  because no task changes `bucketPlanRows`.
- **Neutral-folder drop rule** (spec §3): a neutral folder appears in the expense band, so
  it is reachable by an expense drag through the normal in-band path, and by an income
  element through `⋮` → Move to folder, which lists neutral folders. No extra work.
- **Names verified against source** while writing this plan: `useMoveElement`,
  `useCreateBudgetFolder` (NOT `useCreateFolder`), `useChangeElementCurrency`,
  `CurrencyPickerDialog`, `afterIdFromDrop` (from `@/lib/ordering`), and the
  `Checkbox`/`Input`/`Label` primitives. The `move_to_folder` and `no_folder` i18n keys
  survive in all 11 catalogues — only the component was removed in `cb5f16a8`.
