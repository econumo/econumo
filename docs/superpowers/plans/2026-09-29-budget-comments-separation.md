# Budget Comments Separation (Redesign Stage 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take budget-cell comments out of the amount editor everywhere and give them their own Excel-style entry points (corner marker → anchored thread popover, hover preview, right-click / long-press cell menu, Shift+F2), and remove the header currency chips and the "Spending progress" widget.

**Architecture:** Frontend only (`web/`). One page-level "open thread" state per view now carries an optional DOM anchor; a new `CommentsPanel` renders it as a popover anchored to that cell (desktop/tablet) or as a bottom sheet (phone, or no anchor). A new `CellShell` wraps a budget cell with the hover preview and the context menu, so the monthly table, the savings block and the plan grid share one implementation. `LimitEditor` becomes amount-only.

**Tech Stack:** React 19, TypeScript, Radix primitives via shadcn (`popover`, `hover-card`, `context-menu` in `web/src/components/ui/`), TanStack Query, react-i18next, vitest + Testing Library + MSW, pnpm.

**Spec:** `docs/superpowers/specs/2026-09-29-budget-ux-redesign-design.md` — Part 2 ("Comments, separate from the amount editor") and the "Removals" section's currency chips / `ExpenseWidget` item. Parts 1 and 3 are stages 2 and 3 (separate plans).

## Global Constraints

- Base branch `v1.6-dev`; branch name `feature/budget-comments-separation`; PR into `v1.6-dev`.
- Run all web commands from `web/`: `pnpm exec tsc -b --noEmit`, `pnpm lint`, `pnpm exec vitest run <path>`.
- Breakpoints: phone = `useIsPhone()` (`(max-width: 639px)`, `web/src/hooks/useIsPhone.ts`, already exists); compact/touch = `useIsCompact()` (`(max-width: 1023px)`).
- No server, DTO, permission, or analytics changes. Comment create/update/delete keep firing their existing `METRICS` events inside `useCreateComment` / `useUpdateComment` / `useDeleteComment` — do not touch `web/src/lib/metrics.ts`.
- Every new catalogue key goes into all 11 `locales/<lang>.json` files (`de en es fr it nl pl pt ru uk zh`); `en` is the reference. The Go guard `go test ./internal/test/i18ntest/` must pass (Go is at `/usr/local/go/bin/go`; use `GOTOOLCHAIN=go1.27.1`).
- Comments in code: only the non-obvious *why* (repo rule in `CLAUDE.md`).
- **Stage-1 phone path stays as today:** `SetLimitDialog`'s "Comments (N)" button (PR #292), tap-Available, and long-press remain on phones; stage 2 replaces them with the item sheet. Long-press (`ElementLongPress`) becomes **phone-only** in this stage so it cannot collide with the tablet context menu.
- `docs/regression-test-plan.md` must be updated in the same PR (repo rule).

## Review Focus

1. **Long-press on a tablet cell opens only the menu** — the release after a 700 ms touch hold must not also fire the cell's tap action (set-limit dialog / select). Test: Task 3 Step 1 (`suppresses the click that ends a touch long-press`).
2. **Clicking another cell's marker while a thread popover is open switches to that cell's thread** (outside-press closes the first, the click opens the second) — it must not end with nothing open. Test: Task 4 Step 1 (`switches the thread when another marker is clicked`).
3. **A phone never gets an anchored popover**, even when the caller passes an anchor (marker taps on savings/plan cells do) — it gets the bottom sheet. Test: Task 2 Step 1 (`ignores the anchor on a phone`).
4. **The hover preview never sits on top of an open amount editor or thread** — a pointer press on the cell closes it and blocks it until the pointer leaves; it is disabled while a thread is open. Test: Task 3 Step 1 (`closes the preview on press and keeps it closed until the pointer leaves`).
5. **The uncategorized row gets no comment entry anywhere** (no marker, no menu item, no Shift+F2) — its synthetic id names no element the server accepts. Test: Task 5 Step 1 (`offers no comment item on the uncategorized row`).

---

## File Structure

| File | Responsibility |
|---|---|
| `web/src/features/budgets/cellDom.ts` (create) | DOM helpers shared by all three surfaces: find a cell's comment anchor, open the inline amount editor inside a cell |
| `web/src/features/budgets/CommentsPanel.tsx` (create; replaces `CommentsDialog.tsx`) | Presents one thread: popover anchored to a cell, or a bottom sheet |
| `web/src/features/budgets/CellShell.tsx` (create) | Wraps one budget cell with the hover preview and the right-click / long-press menu |
| `web/src/features/budgets/CommentThread.tsx` (modify) | `layout` prop (pinned composer in a sheet); exported date/sort helpers; bigger `CommentMarker` that reports its anchor |
| `web/src/features/budgets/LimitEditor.tsx` (modify) | Amount-only: `footer` prop removed; trigger tagged `data-limit-trigger` |
| `web/src/features/budgets/BudgetTable.tsx` (modify) | Budgeted cell becomes a comment anchor; `renderBudgetCellComments` popover → `onBudgetCellComments` callback; `wrapBudgetCell` extra |
| `web/src/features/budgets/SavingsBlock.tsx` (modify) | Planned cell becomes a comment anchor; `onOpenComments(row, anchor)`; `wrapPlannedCell` prop |
| `web/src/features/budgets/BudgetPage.tsx` (modify) | Monthly wiring; `CommentsFooter` deleted; currency chips + `ExpenseWidget` removed |
| `web/src/features/budgets/PlanSheet.tsx` (modify) | Plan wiring; `CommentsFooter` + auto-expand deleted; Shift+F2; cell transactions |
| `web/src/features/budgets/ExpenseWidget.tsx` (delete) | — |
| `locales/*.json` (modify ×11) | +`budgets.page.plan.comments.add`, +`budgets.page.plan.comments.more`; −`budgets.modal.expense_widget.header`, −`budgets.modal.expense_widget.saved_of_planned` (`conversion_rate` stays for stage 2) |
| `docs/regression-test-plan.md` (modify) | Comment, savings-edit and expense-widget items |

---

### Task 1: Remove the header currency chips and the Spending progress widget

**Files:**
- Modify: `web/src/features/budgets/BudgetPage.tsx` (chips block ~`:705-723`, `selectedCurrencyId` `:313`, `budgetCurrencyIds` `:492`, widget mounts `:783`, `:808`, import `:74`)
- Delete: `web/src/features/budgets/ExpenseWidget.tsx`
- Modify: `web/src/features/budgets/BudgetPage.test.tsx`, `web/src/features/budgets/PeriodStrip.test.tsx`, `web/src/features/budgets/SavingsBlock.test.tsx`
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`
- Modify: `docs/regression-test-plan.md`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing new. `budgets.modal.expense_widget.conversion_rate` is intentionally left in every catalogue for stage 2's item sheet (the rate computation to reuse is in `ExpenseWidget.tsx` at commit `186eed5`).

- [ ] **Step 1: Change the tests to the new expectation**

In `BudgetPage.test.tsx`, in the first test (the one asserting `getAllByRole('tab')` has length 49), replace the four chip/widget lines:

```tsx
  // currency chips from balances
  expect(screen.getByRole('button', { name: 'currency USD' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'currency EUR' })).toBeInTheDocument()
  // widget hidden until a chip is selected
  expect(screen.queryByTestId('expense-widget')).not.toBeInTheDocument()
```

with:

```tsx
  // the header carries no currency chips and there is no spending widget
  expect(screen.queryByRole('button', { name: /^currency / })).not.toBeInTheDocument()
  expect(screen.queryByTestId('expense-widget')).not.toBeInTheDocument()
```

Delete the whole test `it('toggling a currency chip mounts the expense widget', ...)` and the whole test `it('shows the currency pills on both routes; on /plan a pill toggles the period widget above the sheet', ...)`.

In `PeriodStrip.test.tsx` delete the `import { ExpenseWidget } from './ExpenseWidget'` line and the two tests `it('widget renders spent/total, progress and the conversion hint', ...)` and `it('widget shows the conversion hint for a non-base currency', ...)`; remove any import that becomes unused (run lint to find them).

In `SavingsBlock.test.tsx` delete the `import { ExpenseWidget } from './ExpenseWidget'` line and the whole `describe('ExpenseWidget savings line', ...)` block; remove imports that become unused.

- [ ] **Step 2: Run the page test to verify it fails**

Run: `cd web && pnpm exec vitest run src/features/budgets/BudgetPage.test.tsx -t "tab"`
Expected: FAIL — `queryByRole('button', { name: /^currency / })` finds the USD chip.

- [ ] **Step 3: Remove the chips, the widget and its keys**

In `BudgetPage.tsx`:
- delete `import { ExpenseWidget } from './ExpenseWidget'`;
- delete `const [selectedCurrencyId, setSelectedCurrencyId] = useState<Id | null>(null)`;
- delete `const budgetCurrencyIds = budget.balances.map((b) => b.currencyId)`;
- delete the whole header block that starts with the comment `{/* both views: the pills toggle the period widget above the table / the sheet */}` and the `<span className="flex shrink-0 items-center gap-1">…</span>` that maps `budgetCurrencyIds` to chip buttons;
- in the plan branch replace

```tsx
        <>
          {/* the same period widget as the budget view, for the page's selected period */}
          {selectedCurrencyId ? <ExpenseWidget budget={budget} currencyId={selectedCurrencyId} /> : null}
          <PlanSheet budget={budget} currencies={currencies} userId={user?.id} editMode={editMode} />
        </>
```

with

```tsx
        <PlanSheet budget={budget} currencies={currencies} userId={user?.id} editMode={editMode} />
```

- in the budget branch delete the line `{selectedCurrencyId ? <ExpenseWidget budget={budget} currencyId={selectedCurrencyId} /> : null}`.

Delete `web/src/features/budgets/ExpenseWidget.tsx`.

In every `locales/<lang>.json`, inside `budgets.modal.expense_widget`, delete the `header` and `saved_of_planned` entries and keep `conversion_rate`. Use this script from the repo root (it preserves key order and 2-space indentation; check `git diff locales` afterwards):

```bash
python3 - <<'EOF'
import json, collections
for lang in 'de en es fr it nl pl pt ru uk zh'.split():
    p = f'locales/{lang}.json'
    d = json.load(open(p, encoding='utf-8'), object_pairs_hook=collections.OrderedDict)
    w = d['budgets']['modal']['expense_widget']
    w.pop('header'); w.pop('saved_of_planned')
    open(p, 'w', encoding='utf-8').write(json.dumps(d, ensure_ascii=False, indent=2) + '\n')
EOF
git diff --stat locales
```

Expected: 11 files, 2 deletions each. If the diff shows any other change (e.g. escaping or trailing-newline churn), revert with `git checkout locales` and delete the two lines by hand instead.

- [ ] **Step 4: Update the regression plan**

In `docs/regression-test-plan.md`:
- delete the item that starts `- [ ] 📱 Expense widget (select a currency chip): with savings accounts it` (the whole bullet, through `absent.`);
- in the item that ends `totals (sidebar total, budget expense widget note).` change that line to `totals (sidebar total).`;
- in the item `Budget with accounts in two currencies: per-currency balances section is` change `correct; expense widget shows the conversion note.` to `correct.`;
- add under the budget-page section:

```markdown
- [ ] 📱 The budget header shows no currency chips on /budget or /plan, and
      no "Spending progress" widget appears anywhere on the page.
```

- [ ] **Step 5: Run tests, types, lint, i18n guard**

Run:
```bash
cd web && pnpm exec vitest run src/features/budgets && pnpm exec tsc -b --noEmit && pnpm lint
cd .. && PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1 go test ./internal/test/i18ntest/
```
Expected: all PASS; lint 0 errors.

- [ ] **Step 6: Commit**

```bash
git add -A web/src/features/budgets locales docs/regression-test-plan.md
git commit -m "feat: Remove the budget currency chips and spending widget"
```

---

### Task 2: `CommentsPanel` — one thread as an anchored popover or a sheet

**Files:**
- Create: `web/src/features/budgets/cellDom.ts`
- Create: `web/src/features/budgets/CommentsPanel.tsx`
- Create: `web/src/features/budgets/CommentsPanel.test.tsx`
- Modify: `web/src/features/budgets/CommentThread.tsx` (props + list/composer wrappers)
- Modify: `web/src/features/budgets/BudgetPage.tsx`, `web/src/features/budgets/PlanSheet.tsx` (swap `CommentsDialog` → `CommentsPanel`)
- Delete: `web/src/features/budgets/CommentsDialog.tsx`

**Interfaces:**
- Produces:
  - `cellDom.ts`: `export const COMMENT_ANCHOR_ATTR = 'data-comment-anchor'`; `export function commentAnchorOf(el: Element): HTMLElement` (closest `[data-comment-anchor]`, else `el`); `export function openLimitEditorIn(anchor: HTMLElement): void` (clicks the first `[data-limit-trigger]` inside `anchor`).
  - `CommentsPanel` props: `CommentThreadProps & { open: boolean; onClose: () => void; title: string; anchor: HTMLElement | null }`. Renders `data-testid="comments-popover"` (popover) or `data-testid="comments-sheet"` (sheet).
  - `CommentThreadProps` gains `layout?: 'popover' | 'sheet'` (default `'popover'`).

- [ ] **Step 1: Write the failing tests**

Create `web/src/features/budgets/CommentsPanel.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { CommentsPanel } from './CommentsPanel'

const comment = {
  id: 'cm1',
  elementId: 'cat-food',
  period: '2026-07-01',
  comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-07-17 09:00:00',
  updatedAt: '2026-07-17 09:00:00',
}

function mockMatchMedia(matches: (q: string) => boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: matches(q), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPanel(anchor: HTMLElement | null) {
  server.use(...coreHandlers())
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CommentsPanel
        open
        onClose={() => {}}
        title="Groceries"
        anchor={anchor}
        budgetId="b1"
        elementId="cat-food"
        period="2026-07-01"
        comments={[comment]}
        currentUserId="u1"
        canModerate={false}
        readOnly={false}
        truncated={false}
      />
    </QueryClientProvider>,
  )
}

function makeAnchor() {
  const el = document.createElement('div')
  document.body.appendChild(el)
  return el
}

it('anchors the thread to the cell on desktop', async () => {
  mockMatchMedia(() => false)
  renderPanel(makeAnchor())
  expect(await screen.findByTestId('comments-popover')).toBeInTheDocument()
  expect(screen.getByText('Trip to Lisbon')).toBeInTheDocument()
  expect(screen.queryByTestId('comments-sheet')).toBeNull()
})

it('anchors the thread on a tablet too', async () => {
  mockMatchMedia((q) => q.includes('1023'))
  renderPanel(makeAnchor())
  expect(await screen.findByTestId('comments-popover')).toBeInTheDocument()
})

it('falls back to the sheet without an anchor', async () => {
  mockMatchMedia(() => false)
  renderPanel(null)
  expect(await screen.findByTestId('comments-sheet')).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Groceries' })).toBeInTheDocument()
})

it('ignores the anchor on a phone', async () => {
  mockMatchMedia(() => true)
  renderPanel(makeAnchor())
  expect(await screen.findByTestId('comments-sheet')).toBeInTheDocument()
  expect(screen.queryByTestId('comments-popover')).toBeNull()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/features/budgets/CommentsPanel.test.tsx`
Expected: FAIL — `Failed to resolve import "./CommentsPanel"`.

- [ ] **Step 3: Implement**

Create `web/src/features/budgets/cellDom.ts`:

```ts
export const COMMENT_ANCHOR_ATTR = 'data-comment-anchor'

// A thread popover anchors to the whole cell, not to the control that opened it
// (a 20px marker or an amount button), so it lines up the same way from every
// entry point.
export function commentAnchorOf(el: Element): HTMLElement {
  return (el.closest(`[${COMMENT_ANCHOR_ATTR}]`) as HTMLElement | null) ?? (el as HTMLElement)
}

export function openLimitEditorIn(anchor: HTMLElement): void {
  anchor.querySelector<HTMLButtonElement>('[data-limit-trigger]')?.click()
}
```

In `CommentThread.tsx`:
- add to `CommentThreadProps`:

```ts
  /** 'sheet' pins the composer to the bottom of the scrolling sheet body so the
   *  on-screen keyboard never pushes the thread out of view */
  layout?: 'popover' | 'sheet'
```

- change the signature to `export function CommentThread({ budgetId, elementId, period, comments, currentUserId, canModerate, readOnly, truncated, layout = 'popover' }: CommentThreadProps) {`;
- on the `<ul …>` replace `className="flex max-h-64 flex-col gap-3 overflow-y-auto"` with ``className={`flex flex-col gap-3 ${layout === 'popover' ? 'max-h-64 overflow-y-auto' : ''}`}``;
- on the composer wrapper (the `<div className="flex flex-col gap-1.5">` in the `readOnly ? … : (…)` branch) replace its `className` with ``className={`flex flex-col gap-1.5 ${layout === 'sheet' ? 'sticky bottom-0 bg-background pt-2' : ''}`}`` and add `data-testid="comment-composer"`.

Create `web/src/features/budgets/CommentsPanel.tsx`:

```tsx
import { useMemo } from 'react'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsPhone } from '@/hooks/useIsPhone'
import { CommentThread } from './CommentThread'
import type { CommentThreadProps } from './CommentThread'

interface CommentsPanelProps extends CommentThreadProps {
  open: boolean
  onClose: () => void
  title: string
  /** the cell the thread belongs to; null (or a phone) presents a bottom sheet instead */
  anchor: HTMLElement | null
}

export function CommentsPanel({ open, onClose, title, anchor, ...threadProps }: CommentsPanelProps) {
  const isPhone = useIsPhone()
  const virtualRef = useMemo(() => ({ current: anchor }), [anchor])
  if (!open) {
    return null
  }
  if (anchor && !isPhone) {
    return (
      <Popover open onOpenChange={(o) => !o && onClose()}>
        <PopoverAnchor virtualRef={virtualRef as { current: HTMLElement }} />
        <PopoverContent className="w-80 p-3" align="end" aria-label={title} data-testid="comments-popover">
          <CommentThread {...threadProps} />
        </PopoverContent>
      </Popover>
    )
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={title}>
      <div data-testid="comments-sheet">
        <CommentThread {...threadProps} layout="sheet" />
      </div>
    </ResponsiveDialog>
  )
}
```

Swap the call sites:
- `BudgetPage.tsx`: replace `import { CommentsDialog } from './CommentsDialog'` with `import { CommentsPanel } from './CommentsPanel'`, rename the `<CommentsDialog` element to `<CommentsPanel` and add `anchor={null}` to its props (Task 4 wires real anchors).
- `PlanSheet.tsx`: the same import swap, rename `<CommentsDialog` to `<CommentsPanel`, add `anchor={null}` (Task 5 wires real anchors).

Delete `web/src/features/budgets/CommentsDialog.tsx`.

- [ ] **Step 4: Run the tests**

Run: `cd web && pnpm exec vitest run src/features/budgets && pnpm exec tsc -b --noEmit`
Expected: PASS (the page suites still see the sheet everywhere, as before).

- [ ] **Step 5: Commit**

```bash
git add -A web/src/features/budgets
git commit -m "feat: CommentsPanel presents a thread as an anchored popover or a sheet"
```

---

### Task 3: `CellShell` — hover preview and cell menu; bigger marker

**Files:**
- Create: `web/src/features/budgets/CellShell.tsx`
- Create: `web/src/features/budgets/CellShell.test.tsx`
- Modify: `web/src/features/budgets/CommentThread.tsx` (export helpers, `CommentMarker`)
- Modify: `web/src/features/budgets/CommentThread.test.tsx` (marker test)
- Modify: `locales/*.json` ×11 (two new keys)

**Interfaces:**
- Consumes: `commentAnchorOf` (Task 2).
- Produces:
  - `CommentThread.tsx`: `export function sortByCreatedAt(comments: BudgetCommentDto[]): BudgetCommentDto[]`; `export function formatCommentTime(createdAt: string, lang: string): string`; `CommentMarker` props become `{ count: number; onOpen: (anchor: HTMLElement) => void }`.
  - `CellShell` props:

```ts
interface CellShellProps {
  /** the cell element: must be a single DOM element that accepts a ref and props */
  children: ReactElement
  comments: BudgetCommentDto[]
  /** no hover preview: touch viewports, or while a thread is open */
  previewDisabled?: boolean
  /** no menu: phones (stage 2 gives them the item sheet) and edit-structure mode */
  menuDisabled?: boolean
  onSetBudget?: (anchor: HTMLElement) => void
  /** omitted for the uncategorized row */
  onOpenComments?: (anchor: HTMLElement) => void
  onShowTransactions?: () => void
}
```

- [ ] **Step 1: Write the failing tests**

Create `web/src/features/budgets/CellShell.test.tsx`:

```tsx
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CellShell } from './CellShell'

const c = (id: string, text: string, at: string) => ({
  id,
  elementId: 'cat-food',
  period: '2026-07-01',
  comment: text,
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: at,
  updatedAt: at,
})
const three = [
  c('a', 'First note', '2026-07-01 09:00:00'),
  c('b', 'Second note', '2026-07-02 09:00:00'),
  c('d', 'Third note', '2026-07-03 09:00:00'),
]

function renderShell(props: Partial<Parameters<typeof CellShell>[0]> = {}, onCellClick = vi.fn()) {
  render(
    <CellShell comments={three} {...props}>
      <div data-testid="cell" data-comment-anchor="" onClick={onCellClick}>
        700.00
      </div>
    </CellShell>,
  )
  return screen.getByTestId('cell')
}

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

afterEach(() => {
  vi.useRealTimers()
})

it('previews the latest two comments on hover, with a count of the rest', async () => {
  const user = userEvent.setup()
  const cell = renderShell()
  await user.hover(cell)
  const preview = await screen.findByTestId('comment-preview', {}, { timeout: 1500 })
  expect(preview).toHaveTextContent('Second note')
  expect(preview).toHaveTextContent('Third note')
  expect(preview).not.toHaveTextContent('First note')
  expect(preview).toHaveTextContent('+1 more')
})

it('shows no preview when disabled or when the cell has no comments', async () => {
  const user = userEvent.setup()
  const cell = renderShell({ previewDisabled: true })
  await user.hover(cell)
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('closes the preview on press and keeps it closed until the pointer leaves', async () => {
  const user = userEvent.setup()
  const cell = renderShell()
  await user.hover(cell)
  await screen.findByTestId('comment-preview', {}, { timeout: 1500 })
  await user.pointer({ keys: '[MouseLeft]', target: cell })
  await waitFor(() => expect(screen.queryByTestId('comment-preview')).toBeNull())
  await new Promise((r) => setTimeout(r, 500))
  expect(screen.queryByTestId('comment-preview')).toBeNull()
})

it('offers set budget, comments and transactions on right-click and runs the picked action with the cell', async () => {
  const user = userEvent.setup()
  const onSetBudget = vi.fn()
  const onOpenComments = vi.fn()
  const onShowTransactions = vi.fn()
  const cell = renderShell({ onSetBudget, onOpenComments, onShowTransactions })
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(await screen.findByRole('menuitem', { name: 'Set budget' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Comments (3)' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Show transactions' })).toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Comments (3)' }))
  await waitFor(() => expect(onOpenComments).toHaveBeenCalledWith(cell))
  expect(onSetBudget).not.toHaveBeenCalled()
})

it('omits the items the caller cannot use, and names an empty thread "Add comment"', async () => {
  const user = userEvent.setup()
  const cell = renderShell({ comments: [], onOpenComments: vi.fn() })
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(await screen.findByRole('menuitem', { name: 'Add comment' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Set budget' })).toBeNull()
  expect(screen.queryByRole('menuitem', { name: 'Show transactions' })).toBeNull()
})

it('opens no menu when disabled', async () => {
  const user = userEvent.setup()
  const cell = renderShell({ menuDisabled: true, onOpenComments: vi.fn() })
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(screen.queryByRole('menu')).toBeNull()
})

it('opens the menu on a touch long-press and suppresses the click that ends it', async () => {
  vi.useFakeTimers()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  const onCellClick = vi.fn()
  const cell = renderShell({ onOpenComments: vi.fn() }, onCellClick)
  await user.pointer({ keys: '[TouchA>]', target: cell })
  act(() => {
    vi.advanceTimersByTime(750)
  })
  expect(screen.getByRole('menu')).toBeInTheDocument()
  await user.pointer({ keys: '[/TouchA]', target: cell })
  expect(onCellClick).not.toHaveBeenCalled()
})
```

Add to `CommentThread.test.tsx` (it already imports from `./CommentThread`; add `CommentMarker` to that import and `render, screen` / `userEvent` if not present):

```tsx
it('the marker reports the cell it sits in as the anchor', async () => {
  const user = userEvent.setup()
  const onOpen = vi.fn()
  render(
    <div data-comment-anchor="" data-testid="cell">
      <CommentMarker count={2} onOpen={onOpen} />
    </div>,
  )
  await user.click(screen.getByTestId('comment-marker'))
  expect(onOpen).toHaveBeenCalledWith(screen.getByTestId('cell'))
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/features/budgets/CellShell.test.tsx src/features/budgets/CommentThread.test.tsx`
Expected: FAIL — `./CellShell` does not resolve; the marker test fails because `onOpen` is called with no argument.

- [ ] **Step 3: Add the catalogue keys**

Add `add` and `more` to `budgets.page.plan.comments` in all 11 catalogues (from the repo root):

```bash
python3 - <<'EOF'
import json, collections
vals = {
  'en': ('Add comment', '+{count} more'),
  'de': ('Kommentar hinzufügen', '+{count} weitere'),
  'es': ('Añadir comentario', '+{count} más'),
  'fr': ('Ajouter un commentaire', '+{count} de plus'),
  'it': ('Aggiungi commento', '+{count} altri'),
  'nl': ('Opmerking toevoegen', '+{count} meer'),
  'pl': ('Dodaj komentarz', '+{count} więcej'),
  'pt': ('Adicionar comentário', '+{count} mais'),
  'ru': ('Добавить комментарий', 'ещё {count}'),
  'uk': ('Додати коментар', 'ще {count}'),
  'zh': ('添加评论', '还有 {count} 条'),
}
for lang, (add, more) in vals.items():
    p = f'locales/{lang}.json'
    d = json.load(open(p, encoding='utf-8'), object_pairs_hook=collections.OrderedDict)
    c = d['budgets']['page']['plan']['comments']
    c['add'] = add; c['more'] = more
    open(p, 'w', encoding='utf-8').write(json.dumps(d, ensure_ascii=False, indent=2) + '\n')
EOF
git diff --stat locales
```

Expected: 11 files, +2 lines each.

- [ ] **Step 4: Implement**

In `CommentThread.tsx`, add `import { commentAnchorOf } from './cellDom'`, then export the helpers (keep `parseServerDateTime` where it is):

```tsx
export function formatCommentTime(createdAt: string, lang: string): string {
  return parseServerDateTime(createdAt).toLocaleString(lang)
}

// createdAt is the server's fixed-width "Y-m-d H:i:s" wire format: plain
// ordinal comparison, not locale-aware collation, is what sorts it correctly.
export function sortByCreatedAt(comments: BudgetCommentDto[]): BudgetCommentDto[] {
  return [...comments].sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : 0))
}
```

Inside `CommentThread`, replace the `const sorted = [...comments].sort(…)` line (and the two comment lines above it) with `const sorted = sortByCreatedAt(comments)`, and replace `parseServerDateTime(c.createdAt).toLocaleString(i18n.language)` with `formatCommentTime(c.createdAt, i18n.language)`.

Replace `CommentMarker` with:

```tsx
// The corner triangle on a commented amount cell. The 8px triangle is drawn on an
// inner span so the button keeps a 20px hit area without taking layout space or
// changing column width; the cell must be `relative`.
export function CommentMarker({ count, onOpen }: { count: number; onOpen: (anchor: HTMLElement) => void }) {
  const { t, i18n } = useTranslation()
  return (
    <button
      type="button"
      data-testid="comment-marker"
      aria-label={pluralPick(t('budgets.page.plan.comments.marker_aria'), count, i18n.language)}
      className="absolute right-0 top-0 flex size-5 items-start justify-end"
      onClick={(e) => {
        e.stopPropagation()
        onOpen(commentAnchorOf(e.currentTarget))
      }}
    >
      <span className="h-0 w-0 border-l-[8px] border-t-[8px] border-l-transparent border-t-primary" />
    </button>
  )
}
```

Create `web/src/features/budgets/CellShell.tsx`:

```tsx
import { useRef, useState } from 'react'
import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from '@/components/ui/context-menu'
import type { BudgetCommentDto } from '@/api/dto/budget'
import { commentAnchorOf } from './cellDom'
import { formatCommentTime, sortByCreatedAt } from './CommentThread'

interface CellShellProps {
  children: ReactElement
  comments: BudgetCommentDto[]
  previewDisabled?: boolean
  menuDisabled?: boolean
  onSetBudget?: (anchor: HTMLElement) => void
  onOpenComments?: (anchor: HTMLElement) => void
  onShowTransactions?: () => void
}

const PREVIEW_COUNT = 2

export function CellShell({
  children,
  comments,
  previewDisabled = false,
  menuDisabled = false,
  onSetBudget,
  onOpenComments,
  onShowTransactions,
}: CellShellProps) {
  const { t, i18n } = useTranslation()
  const cellRef = useRef<HTMLElement | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  // a press means the user is acting on the cell (amount editor, selection): the
  // preview stays shut until the pointer leaves, or its pending open timer would
  // pop it over the editor the press just opened
  const pressed = useRef(false)
  // The menu runs its action only after it has closed and handed focus back, or
  // the returning focus would steal it from the popover/dialog the action opens.
  const pending = useRef<((anchor: HTMLElement) => void) | null>(null)
  // A touch long-press opens the menu, and the finger's release then fires a click
  // on the cell underneath; swallow that one click so the tap action (set-limit
  // dialog, selection) does not run on top of the menu.
  const lastPointerType = useRef<string>('mouse')
  const swallowClickUntil = useRef(0)

  const previewable = !previewDisabled && comments.length > 0
  const hasMenu = !menuDisabled && (onSetBudget || onOpenComments || onShowTransactions)
  if (!previewable && !hasMenu) {
    return children
  }

  const latest = sortByCreatedAt(comments).slice(-PREVIEW_COUNT)
  const rest = comments.length - latest.length

  return (
    <HoverCard
      open={previewable && previewOpen}
      onOpenChange={(open) => setPreviewOpen(open && !pressed.current)}
      openDelay={300}
      closeDelay={100}
    >
      <ContextMenu
        onOpenChange={(open) => {
          if (open && lastPointerType.current !== 'mouse') {
            swallowClickUntil.current = Date.now() + 700
          }
        }}
      >
        <ContextMenuTrigger asChild disabled={!hasMenu}>
          <HoverCardTrigger
            asChild
            ref={cellRef}
            onPointerDown={(e) => {
              lastPointerType.current = e.pointerType || 'mouse'
              pressed.current = true
              setPreviewOpen(false)
            }}
            onPointerLeave={() => {
              pressed.current = false
            }}
            onClickCapture={(e) => {
              if (Date.now() < swallowClickUntil.current) {
                swallowClickUntil.current = 0
                e.preventDefault()
                e.stopPropagation()
              }
            }}
          >
            {children}
          </HoverCardTrigger>
        </ContextMenuTrigger>
        {hasMenu ? (
          <ContextMenuContent
            className="w-52"
            onCloseAutoFocus={(e) => {
              const action = pending.current
              pending.current = null
              if (action && cellRef.current) {
                e.preventDefault()
                action(commentAnchorOf(cellRef.current))
              }
            }}
          >
            {onSetBudget ? (
              <ContextMenuItem onSelect={() => (pending.current = onSetBudget)}>{t('budgets.modal.set_limit_form.header')}</ContextMenuItem>
            ) : null}
            {onOpenComments ? (
              <ContextMenuItem onSelect={() => (pending.current = onOpenComments)}>
                {comments.length > 0
                  ? t('budgets.page.plan.comments.disclosure', { count: comments.length })
                  : t('budgets.page.plan.comments.add')}
              </ContextMenuItem>
            ) : null}
            {onShowTransactions ? (
              <ContextMenuItem onSelect={() => (pending.current = () => onShowTransactions())}>
                {t('budgets.page.budget.structure.element.action.show_transactions')}
              </ContextMenuItem>
            ) : null}
          </ContextMenuContent>
        ) : null}
      </ContextMenu>
      {previewable ? (
        <HoverCardContent align="end" className="w-72" data-testid="comment-preview">
          <div className="flex flex-col gap-2">
            {latest.map((c) => (
              <div key={c.id} className="flex flex-col gap-0.5">
                <div className="flex items-baseline gap-1.5">
                  <span className="truncate text-xs font-medium">{c.author.name}</span>
                  <span className="text-[11px] text-muted-foreground">{formatCommentTime(c.createdAt, i18n.language)}</span>
                </div>
                <p className="line-clamp-3 whitespace-pre-wrap text-sm">{c.comment}</p>
              </div>
            ))}
            {rest > 0 ? <p className="text-xs text-muted-foreground">{t('budgets.page.plan.comments.more', { count: rest })}</p> : null}
          </div>
        </HoverCardContent>
      ) : null}
    </HoverCard>
  )
}
```

Note for the implementer: `HoverCardTrigger` in `web/src/components/ui/hover-card.tsx` spreads `...props` onto the Radix trigger, so `ref`, `onPointerDown`, `onPointerLeave` and `onClickCapture` reach the cell through both `asChild` slots (React 19 passes `ref` as a prop). If `tsc` rejects `ref` on `HoverCardTrigger`, type `cellRef` as `useRef<HTMLDivElement | null>(null)` — the cells are `div`/`span` elements and `commentAnchorOf` only needs `Element`.

The existing `CommentMarker` call sites (`BudgetPage.tsx`, `SavingsBlock.tsx`, `PlanSheet.tsx`) need no change in this task: their `() => …` handlers are assignable to `(anchor: HTMLElement) => void`. Tasks 4 and 5 make them use the anchor.

- [ ] **Step 5: Run the tests**

Run: `cd web && pnpm exec vitest run src/features/budgets/CellShell.test.tsx src/features/budgets/CommentThread.test.tsx && pnpm exec tsc -b --noEmit && cd .. && PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1 go test ./internal/test/i18ntest/`
Expected: PASS (user-event's `TouchA` pointer reports `pointerType: 'touch'`, which is what arms the click swallow).

- [ ] **Step 6: Commit**

```bash
git add -A web/src/features/budgets locales
git commit -m "feat: CellShell adds the comment hover preview and the cell menu"
```

---

### Task 4: Monthly view — anchored threads, preview and menu; amount editor without comments

**Files:**
- Modify: `web/src/features/budgets/LimitEditor.tsx`
- Modify: `web/src/features/budgets/BudgetTable.tsx` (`ElementRowExtras` `:20-39`, budgeted cell `:225-241`, archive extras `:606-621`)
- Modify: `web/src/features/budgets/SavingsBlock.tsx` (`SavingsRow`, `SavingsBlockProps`)
- Modify: `web/src/features/budgets/BudgetPage.tsx`
- Modify: `web/src/features/budgets/comments.monthly.test.tsx`, `web/src/features/budgets/BudgetPage.test.tsx`, `web/src/features/budgets/SavingsBlock.test.tsx` (only where they assert the removed popover path)

**Interfaces:**
- Consumes: `CommentsPanel` (Task 2), `CellShell`, `CommentMarker(onOpen(anchor))` (Task 3), `commentAnchorOf`, `openLimitEditorIn`, `COMMENT_ANCHOR_ATTR` (Task 2).
- Produces:
  - `LimitEditorProps` without `footer`; the trigger button carries `data-limit-trigger=""`.
  - `ElementRowExtras`: `renderBudgetCellComments` **removed**; added `onBudgetCellComments?: (element: BudgetElementDto, anchor: HTMLElement) => void` and `wrapBudgetCell?: (element: BudgetElementDto, cell: ReactElement, opts: { readOnly: boolean }) => ReactNode`.
  - `SavingsBlockProps.onOpenComments: (row: BudgetSavingsElementDto, anchor?: HTMLElement) => void`; added `wrapPlannedCell?: (row: BudgetSavingsElementDto, cell: ReactElement) => ReactNode`.

- [ ] **Step 1: Write the failing tests**

In `comments.monthly.test.tsx`, add this helper next to `mockCompactViewport`:

```tsx
function mockTabletViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}
```

Replace the test `it('marks the budgeted cell and opens the thread from the limit popover', …)` with:

```tsx
it('opens the thread from the marker as a popover beside the cell, and keeps the amount editor comment-free', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(within(row).getByLabelText(/^limit /))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
  await user.keyboard('{Escape}')

  await user.click(within(row).getByTestId('comment-marker'))
  const popover = await screen.findByTestId('comments-popover')
  expect(within(popover).getByText('Trip to Lisbon')).toBeInTheDocument()
})

it('switches the thread when another marker is clicked', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: { items: [comment, { ...comment, id: 'cm2', elementId: 'env-1', comment: 'Envelope note' }], truncated: false },
      }),
    ),
  )
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  await user.click(within(screen.getByTestId('element-env-1')).getByTestId('comment-marker'))
  expect(await screen.findByText('Envelope note')).toBeInTheDocument()
  expect(screen.queryByText('Trip to Lisbon')).toBeNull()
})

it('offers set budget, comments and transactions on a right-clicked budgeted cell', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const cell = within(await screen.findByTestId('element-cat-food')).getByTestId('cell-budgeted')
  await user.pointer({ keys: '[MouseRight]', target: cell })
  await user.click(await screen.findByRole('menuitem', { name: 'Comments (1)' }))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')

  await user.keyboard('{Escape}')
  await user.pointer({ keys: '[MouseRight]', target: cell })
  await user.click(await screen.findByRole('menuitem', { name: 'Set budget' }))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
})

it('lets a guest comment from the menu but not set a budget', async () => {
  mockViewport()
  const user = userEvent.setup()
  renderGuestPage('/budget')

  const cell = within(await screen.findByTestId('element-env-1')).getByTestId('cell-budgeted')
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(await screen.findByRole('menuitem', { name: 'Add comment' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Set budget' })).toBeNull()
})

it('opens the thread as a popover from a marker tap on a tablet', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByTestId('comment-marker'))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
})
```

Keep every other test in the file as it is — they still hold (the phone tests assert the sheet path, the guest/archived desktop tests click `/^comments /` or the marker).

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/features/budgets/comments.monthly.test.tsx`
Expected: FAIL — the amount popover still shows "Comments (1)"; no `comments-popover`; no menu items.

- [ ] **Step 3: Make `LimitEditor` amount-only**

In `LimitEditor.tsx`: remove `import type { ReactNode } from 'react'`, the `footer?: ReactNode` prop and its doc comment, `footer` from the destructuring, and `{footer}` from the JSX; change `<PopoverContent className={footer ? 'w-80 p-2' : 'w-64 p-2'} align="end">` to `<PopoverContent className="w-64 p-2" align="end">`; add `data-limit-trigger=""` to the trigger `<button …>`.

- [ ] **Step 4: Make the budgeted cell an anchor in `BudgetTable`**

In `ElementRowExtras` replace the `renderBudgetCellComments` member and its doc comment with:

```ts
  /** the entry point of a cell without `renderBudgetCell` (non-editable, or an
   *  archived element's row): the plain budgeted value opens the cell's thread */
  onBudgetCellComments?: (element: BudgetElementDto, anchor: HTMLElement) => void
  /** wraps the budgeted cell (hover preview + cell menu); `readOnly` marks a row
   *  whose limit can never be set here (the Archive section) */
  wrapBudgetCell?: (element: BudgetElementDto, cell: ReactElement, opts: { readOnly: boolean }) => ReactNode
```

(add `ReactElement` to the `react` type import). Add `import { COMMENT_ANCHOR_ATTR, commentAnchorOf } from './cellDom'` and remove the `Popover, PopoverContent, PopoverTrigger` import if nothing else in the file uses it.

Replace the budgeted `<span … data-testid="cell-budgeted">…</span>` block with:

```tsx
        {(() => {
          const cell = (
            <span
              {...{ [COMMENT_ANCHOR_ATTR]: '' }}
              className="relative hidden w-24 text-right text-[15px] tabular-nums sm:block"
              data-testid="cell-budgeted"
            >
              {isUncategorized ? (
                EMPTY_CELL
              ) : extras.renderBudgetCell ? (
                extras.renderBudgetCell(element)
              ) : extras.onBudgetCellComments ? (
                <button
                  type="button"
                  className="w-full text-right underline-offset-2 hover:underline"
                  aria-label={`comments ${displayName}`}
                  onClick={(e) => extras.onBudgetCellComments!(element, commentAnchorOf(e.currentTarget))}
                >
                  {moneyFormat(element.budgeted, currency, opts)}
                </button>
              ) : (
                moneyFormat(element.budgeted, currency, opts)
              )}
              {extras.renderBudgetCellMarker?.(element)}
            </span>
          )
          return !isUncategorized && extras.wrapBudgetCell ? extras.wrapBudgetCell(element, cell, { readOnly: false }) : cell
        })()}
```

In the Archive-section extras (the `isReadOnlySection ? { … }` object), replace `renderBudgetCellComments: extras.renderBudgetCellComments,` with:

```ts
                          onBudgetCellComments: extras.onBudgetCellComments,
                          wrapBudgetCell: extras.wrapBudgetCell
                            ? (el, node) => extras.wrapBudgetCell!(el, node, { readOnly: true })
                            : undefined,
```

and in the comment above it change "(the popover)" to "(the thread)".

- [ ] **Step 5: Make the savings planned cell an anchor**

In `SavingsBlock.tsx`:
- `SavingsBlockProps`: change `onOpenComments: (row: BudgetSavingsElementDto) => void` to `onOpenComments: (row: BudgetSavingsElementDto, anchor?: HTMLElement) => void` and add `wrapPlannedCell?: (row: BudgetSavingsElementDto, cell: ReactElement) => ReactNode` (import `ReactElement`); thread `wrapPlannedCell` through `SavingsBlock` → `SavingsRow` props the same way `renderPlannedEditor` is threaded.
- add `import { COMMENT_ANCHOR_ATTR, commentAnchorOf } from './cellDom'`.
- in `SavingsRow` replace the planned `<span className={`relative ${AMOUNT_COL} …`} data-testid="savings-planned">…</span>` with a `cell` constant built the same way, then render `{wrapPlannedCell ? wrapPlannedCell(row, cell) : cell}`:

```tsx
  const cell = (
    <span {...{ [COMMENT_ANCHOR_ATTR]: '' }} className={`relative ${AMOUNT_COL} text-right text-[15px] tabular-nums`} data-testid="savings-planned">
      {editMode ? (
        planned
      ) : editable && renderPlannedEditor ? (
        renderPlannedEditor(row)
      ) : (
        // a cell that cannot be edited (guest, pre-start month, deleted account)
        // still opens its thread, like a non-editable budgeted cell in the table
        <button
          type="button"
          className="w-full text-right underline-offset-2 hover:underline"
          aria-label={`${editable ? 'planned' : 'comments'} ${row.name}`}
          onClick={(e) => (editable ? onEditPlanned(row) : onOpenComments(row, commentAnchorOf(e.currentTarget)))}
        >
          {planned}
        </button>
      )}
      {comments.length > 0 ? <CommentMarker count={comments.length} onOpen={(anchor) => onOpenComments(row, anchor)} /> : null}
    </span>
  )
```

- [ ] **Step 6: Wire `BudgetPage`**

In `BudgetPage.tsx`:

1. Imports: add `import { useIsPhone } from '@/hooks/useIsPhone'`, `import { CellShell } from './CellShell'`, `import { openLimitEditorIn } from './cellDom'`; add `ReactElement` to the `react` type import; remove `CommentThread` from `import { CommentMarker, CommentThread } from './CommentThread'`.
2. Delete the whole `CommentsFooter` function and the comment block above it.
3. After `const isCompact = useIsCompact()` add `const isPhone = useIsPhone()`.
4. Replace `const [commentsTarget, setCommentsTarget] = useState<CellTarget | null>(null)` with:

```tsx
  const [commentsTarget, setCommentsTarget] = useState<{ el: CellTarget; anchor: HTMLElement | null } | null>(null)
  const openComments = (el: CellTarget, anchor: HTMLElement | null = null) => setCommentsTarget({ el, anchor })
```

5. In `inlineLimitEditor` delete the whole `footer={…}` prop.
6. Add, next to `inlineLimitEditor`:

```tsx
  const transactionsTargetOf = (element: BudgetElementDto): BudgetTransactionsTarget => ({
    id: element.id,
    type: element.type,
    name: elementDisplayName(element.id, element.name, t),
    icon: element.icon,
    currencyId: element.currencyId,
  })
  const setBudgetFor = (target: CellTarget) => (anchor: HTMLElement) =>
    isCompact ? setLimitTarget(target) : openLimitEditorIn(anchor)
  // phones keep stage 1's tap/long-press paths; edit mode owns the pointer for dragging
  const cellMenuDisabled = isPhone || editMode
  const wrapBudgetCell = (element: BudgetElementDto, cell: ReactElement, { readOnly }: { readOnly: boolean }) => (
    <CellShell
      comments={commentsByCell.get(commentCellKey(element.id, selectedDate)) ?? []}
      previewDisabled={isCompact || commentsTarget !== null}
      menuDisabled={cellMenuDisabled}
      onSetBudget={limitsEditable && !readOnly ? setBudgetFor(element) : undefined}
      onOpenComments={(anchor) => openComments(element, anchor)}
      onShowTransactions={() => setTransactionsTarget(transactionsTargetOf(element))}
    >
      {cell}
    </CellShell>
  )
  const wrapPlannedCell = (row: BudgetSavingsElementDto, cell: ReactElement) => (
    <CellShell
      comments={commentsByCell.get(commentCellKey(row.id, selectedDate)) ?? []}
      previewDisabled={isCompact || commentsTarget !== null}
      menuDisabled={cellMenuDisabled}
      onSetBudget={limitsEditable && row.isArchived === 0 ? setBudgetFor(row) : undefined}
      onOpenComments={(anchor) => openComments(row, anchor)}
    >
      {cell}
    </CellShell>
  )
```

(import `BudgetSavingsElementDto` from `@/api/dto/budget` if not already imported).

7. On `<BudgetTable …>`: replace the whole `renderBudgetCellComments={…}` prop (and the comment above it) with

```tsx
                    onBudgetCellComments={!editMode ? (element, anchor) => openComments(element, anchor) : undefined}
                    wrapBudgetCell={wrapBudgetCell}
```

change the marker to `return <CommentMarker count={cellComments.length} onOpen={(anchor) => openComments(element, anchor)} />`; in `renderRowWrapper` change the `isCompact ? (element, _bucket, row) => (<ElementLongPress …/>)` branch's condition from `isCompact` to `isPhone` (update the comment above it: "a phone: ..."), and its `onLongPress={limitsEditable ? setLimitTarget : setCommentsTarget}` to `onLongPress={limitsEditable ? setLimitTarget : (el) => openComments(el)}`; change `onAvailableCommentsClick={isCompact && !editMode ? setCommentsTarget : undefined}` to `onAvailableCommentsClick={isCompact && !editMode ? (el) => openComments(el) : undefined}`.
8. On `<SavingsBlock …>`: `onOpenComments={(row, anchor) => openComments(row, anchor ?? null)}` and add `wrapPlannedCell={wrapPlannedCell}`.
9. `SetLimitDialog`'s `onOpenComments`:

```tsx
        onOpenComments={() => {
          if (limitTarget) {
            openComments(limitTarget)
          }
          setLimitTarget(null)
        }}
```

10. The panel:

```tsx
      <CommentsPanel
        open={commentsTarget !== null}
        onClose={() => setCommentsTarget(null)}
        title={commentsTarget ? elementDisplayName(commentsTarget.el.id, commentsTarget.el.name, t) : ''}
        anchor={commentsTarget?.anchor ?? null}
        budgetId={budget.meta.id}
        elementId={commentsTarget?.el.id ?? ''}
        period={selectedDate}
        comments={commentsTarget ? commentsByCell.get(commentCellKey(commentsTarget.el.id, selectedDate)) ?? [] : []}
        currentUserId={user?.id}
        canModerate={canConfigureBudget(budget.meta, user?.id)}
        readOnly={commentsTarget ? commentsReadOnly(budget.meta, selectedDate) : true}
        truncated={commentsTruncated}
      />
```

- [ ] **Step 7: Run the budgets suite and fix stale assertions**

`SavingsBlock.test.tsx` renders `SavingsRow`/`SavingsBlock` with an `onOpenComments` mock: where it asserts `toHaveBeenCalledWith(row)` after a marker or non-editable click, change it to `toHaveBeenCalledWith(row, expect.any(HTMLElement))`.

Run: `cd web && pnpm exec vitest run src/features/budgets && pnpm exec tsc -b --noEmit && pnpm lint`
Expected: the new tests PASS. Any remaining failure should be an assertion on the removed in-popover disclosure (`/Comments \(1\)/` inside the amount popover on desktop) — change it to open the thread via the marker (`within(row).getByTestId('comment-marker')`) and expect `comments-popover`. Do not change phone-viewport tests that go through `SetLimitDialog`'s button; that path is kept in this stage.

- [ ] **Step 8: Commit**

```bash
git add -A web/src/features/budgets
git commit -m "feat: Monthly budget cells open comments on their own"
```

---

### Task 5: Plan view — anchored threads, preview and menu, Shift+F2

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Modify: `web/src/features/budgets/comments.plan.test.tsx`

**Interfaces:**
- Consumes: `CommentsPanel`, `CellShell`, `CommentMarker(onOpen(anchor))`, `commentAnchorOf`, `openLimitEditorIn`, `COMMENT_ANCHOR_ATTR`.
- Produces (inside `PlanSheet.tsx`, `GridCtx`):
  - `openComments: (target: PlanLimitTarget, opts?: { fromGrid?: boolean; anchor?: HTMLElement | null }) => void`
  - `openTransactions: (el: PlanElementDto, month: string) => void`
  - `commentsOpen: boolean`, `isPhone: boolean`
  - `commentsAutoExpandKey` and `consumeCommentsAutoExpand` **removed**.

- [ ] **Step 1: Write the failing tests**

In `comments.plan.test.tsx` add the `mockTabletViewport` helper (same code as Task 4 Step 1). Replace the test `it('marks a cell that has comments and opens its thread from the limit popover', …)` with:

```tsx
it('opens the thread from the marker as a popover, and keeps the amount editor comment-free', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  expect(within(cell).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(within(cell).getByLabelText(/^limit /))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
  await user.keyboard('{Escape}')

  await user.click(within(cell).getByTestId('comment-marker'))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
})
```

In `it('shows the truncated notice when get-comment-list reports its cap was hit', …)` replace the two lines

```tsx
  await user.click(within(cell).getByLabelText(/^limit /))
  await user.click(await screen.findByRole('button', { name: /Comments \(1\)/ }))
```

with `await user.click(within(cell).getByTestId('comment-marker'))`.

Delete the test `it('clicking the marker while the popover is already open expands the thread in place, …')` — there is no in-popover thread any more.

In `it('opens the thread in a dialog on compact viewports', …)` (all media queries match = a phone) add after the existing expectation: `expect(screen.getByTestId('comments-sheet')).toBeInTheDocument()`. Rename it to `'opens the thread in a sheet on a phone'`.

Add:

```tsx
it('opens the thread with Shift+F2', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(cell)
  screen.getByTestId('plan-sheet').focus()
  await user.keyboard('{Shift>}{F2}{/Shift}')
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
})

it('offers set budget, comments and transactions on a right-clicked plan cell', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(await screen.findByRole('menuitem', { name: 'Set budget' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Show transactions' })).toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Comments (1)' }))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
})

it('offers no comment item on the uncategorized row', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-uncategorized:1')
  await user.pointer({ keys: '[MouseRight]', target: cell })
  expect(screen.queryByRole('menuitem', { name: /comment/i })).toBeNull()
})

it('opens the thread as a popover from a marker tap on a tablet', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  await user.click(within(await screen.findByTestId('plan-cell-pe1:1')).getByTestId('comment-marker'))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
})
```

The existing `'does not steal focus from a later mouse-opened dialog after a keyboard-opened thread closes'` and `'opens the thread with Shift+Enter and leaves Enter editing the amount'` tests stay unchanged.

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/features/budgets/comments.plan.test.tsx`
Expected: FAIL — no `comments-popover`, Shift+F2 does nothing, no menu.

- [ ] **Step 3: Implement**

In `PlanSheet.tsx`:

1. Imports: `CommentsPanel` is already imported (Task 2); add `import { CellShell } from './CellShell'`, `import { COMMENT_ANCHOR_ATTR, commentAnchorOf, openLimitEditorIn } from './cellDom'`, `import { useIsPhone } from '@/hooks/useIsPhone'`; remove `CommentThread` from the `./CommentThread` import if it becomes unused (keep `CommentMarker`).
2. Delete the `CommentsFooter` component and its comment block (the one starting `// The amount popover's own comments entry point`).
3. `GridCtx`: remove `commentsAutoExpandKey` and `consumeCommentsAutoExpand` (with their doc comments); replace the `openComments` member and its doc comment with:

```ts
  /** `fromGrid` marks a keyboard-originated open (Shift+Enter / Shift+F2) so the
   *  grid reclaims focus when the thread closes; `anchor` is the cell to pin the
   *  popover to (looked up from the grid when omitted) */
  openComments: (target: PlanLimitTarget, opts?: { fromGrid?: boolean; anchor?: HTMLElement | null }) => void
  openTransactions: (el: PlanElementDto, month: string) => void
  /** a thread is open: hover previews stay shut */
  commentsOpen: boolean
  isPhone: boolean
```

4. State: delete `commentsAutoExpandKey` / `setCommentsAutoExpandKey` / `consumeCommentsAutoExpand`; change the `commentsDialogTarget` state to

```ts
  const [commentsDialogTarget, setCommentsDialogTarget] = useState<(PlanLimitTarget & { anchor: HTMLElement | null }) | null>(null)
```

and update the comment above it to: `// the open comment thread: anchored to its cell on desktop/tablet, a sheet on a phone`. Add `const isPhone = useIsPhone()` next to the existing `isCompact`.

5. Replace the whole `openComments` `useCallback` (and the long comment above it) with:

```tsx
  // The uncategorized row's synthetic id names no real element the server would
  // accept, so it gets no comment entry point at all — guarded here too since the
  // keyboard (Shift+Enter / Shift+F2) reaches this without the marker's gate.
  const openComments = useCallback(
    (target: PlanLimitTarget, opts: { fromGrid?: boolean; anchor?: HTMLElement | null } = {}) => {
      if (target.el.id === UNCATEGORIZED_ID) {
        return
      }
      const col = visibleMonths.indexOf(target.month)
      const anchor = opts.anchor ?? (col >= 0 ? document.getElementById(cellDomId(`${target.el.id}:${target.el.type}`, col)) : null)
      if (opts.fromGrid) {
        editorFromGrid.current = true
      }
      setCommentsDialogTarget({ ...target, anchor })
    },
    [visibleMonths],
  )
  const openTransactions = useCallback(
    (el: PlanElementDto, month: string) =>
      setTransactionsTarget({
        target: { id: el.id, type: el.type, name: elementDisplayName(el.id, el.name, t), icon: el.icon, currencyId: el.currencyId },
        month,
      }),
    [t],
  )
```

Move the `transactionsTarget` `useState` above this block if it is declared later (it must exist before `openTransactions`).

6. `openCommentsFromGrid`: change its body to `openComments({ el: cell.entry.el, month: cell.month, monthIndex: cell.idx }, { fromGrid: true })` and trim its comment to: `// Shift+Enter / Shift+F2: the keyboard route into the same thread the marker opens.`

7. Keyboard: in `handleKeyDown`, directly after the `if (e.shiftKey && (e.key === 'ArrowRight' || e.key === 'ArrowLeft')) { … }` block, add:

```tsx
    // Excel's "edit comment" shortcut, alongside the older Shift+Enter
    if (e.shiftKey && e.key === 'F2') {
      e.preventDefault()
      const cell = selectedMonthCell()
      if (cell) {
        openCommentsFromGrid(cell)
      }
      return
    }
```

8. The ctx object and its dependency array: remove `commentsAutoExpandKey`, `consumeCommentsAutoExpand`; add `openTransactions`, `commentsOpen: commentsDialogTarget !== null`, `isPhone` (and add `openTransactions`, `commentsDialogTarget`, `isPhone` to the dependency array).

9. `ElementRow` month cell. Today the `.map` callback ends with `return ( <div key={m} role="gridcell" …>…</div> )`. Change it in three moves: (a) assign that existing `<div>` — every attribute, handler and child kept — to `const cellNode = (…)`, adding one attribute to it: `{...{ [COMMENT_ANCHOR_ATTR]: '' }}`; (b) make the three child edits listed after this code block; (c) return it wrapped, as below (the `{/* … */}` lines stand for that existing div, not for new code):

```tsx
          const target = { el, month: m, monthIndex: idx }
          const cellNode = (
            <div
              key={m}
              {...{ [COMMENT_ANCHOR_ATTR]: '' }}
              role="gridcell"
              /* …every existing attribute and handler of the gridcell div, unchanged… */
            >
              {/* …existing children, with the three edits below… */}
            </div>
          )
          return (
            <CellShell
              key={m}
              comments={isUncategorized ? [] : cellComments}
              previewDisabled={ctx.isCompact || ctx.commentsOpen}
              menuDisabled={ctx.isPhone || ctx.editMode}
              onSetBudget={editable ? (anchor) => (ctx.isCompact ? ctx.openDialog(target) : openLimitEditorIn(anchor)) : undefined}
              onOpenComments={isUncategorized ? undefined : (anchor) => ctx.openComments(target, { anchor })}
              onShowTransactions={
                !isUncategorized && !isIncomeType(el.type) && el.type !== BudgetElementType.SAVINGS
                  ? () => ctx.openTransactions(el, m)
                  : undefined
              }
            >
              {cellNode}
            </CellShell>
          )
```

(Copy the gridcell `div`'s existing attributes and children verbatim into `cellNode`; `isIncomeType` comes from `@/api/dto/budget` — import it if not already imported.) The three edits inside the children:
- `LimitEditor`: delete its `footer={…}` prop.
- non-editable button: `onClick={(e) => { e.stopPropagation(); ctx.openComments(target, { anchor: commentAnchorOf(e.currentTarget) }) }}` and replace its long comment with: `// a non-editable cell (guest role, archived element, month outside the budget) still opens its thread, so a guest can start one`.
- marker: `<CommentMarker count={commentCount} onOpen={(anchor) => ctx.openComments(target, { anchor })} />`.

10. The panel at the bottom:

```tsx
      <CommentsPanel
        open={commentsDialogTarget !== null}
        onClose={() => setCommentsDialogTarget(null)}
        title={commentsDialogTarget ? elementDisplayName(commentsDialogTarget.el.id, commentsDialogTarget.el.name, t) : ''}
        anchor={commentsDialogTarget?.anchor ?? null}
        budgetId={budget.meta.id}
        elementId={commentsDialogTarget?.el.id ?? ''}
        period={commentsDialogTarget?.month ?? ''}
        comments={commentsDialogTarget ? commentsByCell.get(commentCellKey(commentsDialogTarget.el.id, commentsDialogTarget.month)) ?? [] : []}
        currentUserId={userId}
        canModerate={canConfigureBudget(budget.meta, userId)}
        readOnly={commentsDialogTarget ? commentsReadOnly(budget.meta, commentsDialogTarget.month) : true}
        truncated={commentsTruncated}
      />
```

- [ ] **Step 4: Run the budgets suite**

Run: `cd web && pnpm exec vitest run src/features/budgets && pnpm exec tsc -b --noEmit && pnpm lint`
Expected: PASS, lint 0 errors. If `PlanSheet.test.tsx` has an assertion on the removed "Comments (" disclosure, change it to open the thread via the marker and expect `comments-popover`.

- [ ] **Step 5: Commit**

```bash
git add -A web/src/features/budgets
git commit -m "feat: Plan cells open comments on their own; Shift+F2"
```

---

### Task 6: Regression plan, full gates, PR

**Files:**
- Modify: `docs/regression-test-plan.md`

**Interfaces:** none.

- [ ] **Step 1: Update the regression plan**

In `docs/regression-test-plan.md`:

- In the savings item `Edit a savings row's Planned amount exactly like a budgeted cell:` replace `on desktop a click opens the inline editor popover (with its comments` / `disclosure), on a phone a tap opens the set-limit dialog with a` / `"Comments (N)" button;` with `on desktop a click opens the inline amount popover (no comments in it), on a phone a tap opens the set-limit dialog with a "Comments (N)" button;`.
- Replace the item `On the plan grid, select a cell and press Shift+Enter: its comment` … `opens the amount editor, unaffected.` with:

```markdown
- [ ] On the plan grid, select a cell and press Shift+F2 (or Shift+Enter):
      its thread opens in a popover beside the cell; Esc closes it and the
      arrow keys keep moving the selection. Plain Enter on the same cell opens
      the amount editor, which has no comments section.
```

- Add after the `**Budget cell comments**` group's marker item:

```markdown
- [ ] Desktop: click a cell's corner marker (Budget view, Plan view, a
      savings Planned cell): the thread opens in a popover beside that cell.
      Clicking another cell's marker switches to that cell's thread.
- [ ] Desktop: rest the pointer on a commented cell: after a moment a card
      previews its latest two comments (plus "+N more"); it disappears when
      the pointer leaves, and never shows while the thread or the amount
      editor is open.
- [ ] Desktop: right-click a cell: Set budget / Comments (N) or Add comment /
      Show transactions. A guest sees no Set budget; the uncategorized row
      offers no comment item; savings cells offer no Show transactions.
- [ ] Tablet (640–1023px): tapping a marker opens the thread popover;
      long-pressing a cell opens the same menu as right-click, and lifting the
      finger does not also open the set-limit dialog.
- [ ] 📱 Phone: the corner marker, tap-Available and long-press still reach
      comments through the set-limit sheet's "Comments (N)" button or the
      comments sheet; the comments sheet keeps its composer pinned above the
      keyboard.
```

- [ ] **Step 2: Run every gate**

```bash
cd web && pnpm exec tsc -b --noEmit && pnpm lint && pnpm exec vitest run
cd .. && PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1 go test ./internal/test/i18ntest/
```

Expected: tsc clean; lint 0 errors; vitest all pass except the known pre-existing `transaction.test.ts` Blob `instanceof` failure (it also fails on `main`; do not fix it here); i18n guard PASS.

- [ ] **Step 3: Commit and open the PR**

```bash
git add docs/regression-test-plan.md
git commit -m "docs: Regression items for the separated comments"
git push -u origin feature/budget-comments-separation
gh pr create --draft --base v1.6-dev --title "feat: Budget comments on their own (redesign stage 1)" --body-file /tmp/stage1-pr.md
```

Write `/tmp/stage1-pr.md` before that command, filling the Testing numbers from Step 2's actual output:

```markdown
## Summary
Stage 1 of the Budget & Plan UX redesign (spec: `docs/superpowers/specs/2026-09-29-budget-ux-redesign-design.md`, Part 2 + removals).

- The amount editor is amount-only in both views; comments have their own entry points.
- Corner marker (larger, 20px hit area) opens the thread in a popover beside the cell on desktop and tablet; a phone gets the bottom sheet with the composer pinned above the keyboard.
- Hover preview of the latest two comments on desktop.
- Right-click (desktop) / long-press (tablet) cell menu: Set budget, Comments (N) / Add comment, Show transactions.
- Shift+F2 opens a plan cell's thread (Shift+Enter still works).
- Removed the header currency chips and the "Spending progress" widget.
- Phones keep today's paths (set-limit sheet's "Comments (N)" button, tap-Available, long-press) until stage 2's item sheet.

## Testing
- tsc clean, oxlint 0 errors, vitest <passed>/<total> (only the pre-existing `transaction.test.ts` Blob failure).
- `go test ./internal/test/i18ntest/` passes.
- Regression plan updated.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```
