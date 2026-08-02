# Currencies Page on the Classification Template Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild Settings → Currencies on the shared `ClassificationList` template (tap-row/bottom-sheet actions, active-only filter, section captions) by extending the component with optional props whose defaults keep Categories/Tags/Payees byte-identical — per the approved spec `docs/superpowers/specs/2026-07-31-currencies-classification-template-design.md`.

**Architecture:** `ClassificationList` gains `onOrder?` (optional — no drag/reorder UI when absent), `meta?`, `rowSwitch?`, `hasActions?`, `extraActions?`, `alert?`; `onToggleArchive` becomes optional (the default `rowSwitch` uses it). `CurrenciesPage` becomes a thin composition with two sections (own/global), per-scope switch semantics, and a "Set rate" extra action.

**Tech Stack:** React 19, TanStack Query, vitest + msw + testing-library.

## Global Constraints

- Categories/Tags/Payees/Recurring pages must render byte-identically: their test files are NOT modified; `CategoriesPage.test.tsx` + `PayeesTagsPages.test.tsx` passing unchanged is the acceptance proof for the new props' defaults.
- No backend changes; `scope === 'shared'` currencies stay excluded from the page.
- Locked base/profile switches stay **disabled with tooltip** (house gating pattern — disable the entry point, never hide it). Server refusals (delete-in-use, cannot-hide) surface in the page's alert banner.
- The bespoke `classifications.currencies.modals.delete.question` key is retired from BOTH `locales/en.json` and `locales/ru.json` (en/ru key parity is guarded by i18ntest, which runs in `make go-test`).
- Lint: `cd web && pnpm lint`; tests: `cd web && pnpm test`; run Go i18n guards with `go test ./internal/test/i18ntest/` after locale edits.
- Commit after every task (conventional commits).

---

### Task 1: Extend `ClassificationList` (defaults preserve today's behavior)

**Files:**
- Modify: `web/src/features/classifications/ClassificationList.tsx`

**Interfaces:**
- Produces (consumed by Task 3):
  ```ts
  export interface RowAction { label: string; destructive?: boolean; onSelect: () => void }
  export interface RowSwitchState { checked: boolean; disabled?: boolean; title?: string; ariaLabel: string; onToggle: () => void }
  // props changes:
  onOrder?: (changes: { id: string; position: number }[]) => void   // was required
  onToggleArchive?: (item: T) => void                               // was required
  meta?: (item: T) => ReactNode
  rowSwitch?: (item: T) => RowSwitchState
  hasActions?: (item: T) => boolean
  extraActions?: (item: T) => RowAction[]
  alert?: ReactNode
  ```

- [ ] **Step 1: Apply the component changes**

In `web/src/features/classifications/ClassificationList.tsx`:

1. Export the two new interfaces (above) next to `ClassificationItem`; add the six new/loosened props to `ClassificationListProps<T>` and the destructuring.
2. Sticky-archival + switch resolution (replaces `handleToggleArchive` usage):

```tsx
  // Items archived from THIS screen stay in place (greyed, switch off) even
  // with the active-only filter on — they disappear only on the next visit.
  const [stickyArchivedIds] = useState(() => new Set<string>())
  const switchFor = (item: T): RowSwitchState => {
    const custom = rowSwitch?.(item)
    const base: RowSwitchState = custom ?? {
      checked: item.isArchived === 0,
      ariaLabel: `archive ${item.name}`,
      onToggle: () => onToggleArchive?.(item),
    }
    return {
      ...base,
      onToggle: () => {
        if (item.isArchived === 0 && activeOnly) {
          stickyArchivedIds.add(item.id)
        }
        base.onToggle()
      },
    }
  }
```

3. Reorder affordances only when ordering is wired: `reorderButton` renders only if `onOrder && items.length > 1` (both the desktop actions slot and the compact toolbar row use the same condition; when absent render the compact row only if the filter control still needs it — keep the row, replace the button with `<span />`). `commitOrder` guards `if (!onOrder) return`. The `SortDialog` renders only when `onOrder` is set.
4. Rows without ordering render without drag grips: when `onOrder` is undefined, replace the `SortableList` wrapper with a plain `sectionItems.map(...)` of the same row markup minus the grip button. Extract the row into a local `renderRow(item: T, handle?: SortableHandle)` so both paths share one body (grip only when `handle` is provided).
5. Row body changes inside `renderRow`:
   - `const actionable = hasActions?.(item) ?? true`; the row `onClick` and hover/active accent classes apply only when `actionable`; kebab renders only when `actionable && !isCompact`; the compact sheet opens only when `actionable`.
   - Switch uses `switchFor(item)`: `checked`, `disabled`, `title`, `aria-label`, `onCheckedChange={() => s.onToggle()}` (keep `onClick` stopPropagation).
   - After the archived caption, render `{meta?.(item)}`.
6. Menu/sheet actions: between the Edit item and the Delete item, render `extraActions?.(item)?.map((a) => ...)` — `DropdownMenuItem` (desktop) and outline `Button` (sheet), calling `a.onSelect` (and closing the sheet). Apply the destructive styling when `a.destructive`.
7. Render `{alert ?? null}` directly after the `info` InfoBox.

- [ ] **Step 2: Verify existing template consumers are untouched**

Run: `cd web && pnpm test -- ClassificationList CategoriesPage PayeesTagsPages RecurringSettingsPage 2>&1 | tail -5`
Expected: all PASS with zero test-file changes — the defaults are invisible.

- [ ] **Step 3: Lint + commit**

```bash
cd web && pnpm lint
git add web/src/features/classifications/ClassificationList.tsx
git commit -m "refactor(web): optional ordering/switch/menu extension points on ClassificationList"
```

---

### Task 2: Rewrite `CurrenciesPage.test.tsx` to the template interactions (RED)

**Files:**
- Modify: `web/src/features/currencies/CurrenciesPage.test.tsx`

**Interfaces:**
- Consumes: fixtures already in the file (`fixturePts`, `fixtureGbp`, `defaultRates`, `coreHandlers`); `mockViewport` gains a `compact` flag: `matches: q.includes('max-width') ? compact : false` (mirror the helper in `PayeesTagsPages.test.tsx` — copy its media-query handling verbatim).

- [ ] **Step 1: Adapt existing cases + add template-behavior cases**

Keep the existing create/update/set-rate/delete/hide/show mutation cases but drive them through the template UI (kebab menu instead of the old inline buttons where applicable). Add these cases:

```tsx
it('own rows get a kebab with Edit / Set rate / Delete; global rows get none', async () => {
  renderPage()
  await screen.findByText('Points')
  expect(screen.getByRole('button', { name: 'actions Points' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'actions Pound' })).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: 'actions Points' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Set exchange rate' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()
})

it('compact: tapping an own row opens the action sheet; global rows do not react', async () => {
  mockViewport(true)
  renderPage()
  await userEvent.click(await screen.findByText('Points'))
  const sheet = await screen.findByRole('dialog')
  expect(within(sheet).getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(within(sheet).getByRole('button', { name: 'Set exchange rate' })).toBeInTheDocument()
  expect(within(sheet).getByRole('button', { name: 'Delete' })).toBeInTheDocument()
  await userEvent.click(within(sheet).getByRole('button', { name: 'Cancel' }))
  await userEvent.click(screen.getByText('Pound'))
  expect(screen.queryByRole('dialog')).toBeNull()
})

it('active-only filter hides archived own currencies, never globals, and persists', async () => {
  server.use(...coreHandlers({
    currencies: [fixtureUsd, fixtureEur, { ...fixturePts, isArchived: 1 }, fixtureGbp],
    rates: defaultRates,
  }))
  renderPage()
  await screen.findByText('Global currencies')
  expect(screen.queryByText('Points')).toBeNull() // filter defaults ON
  expect(screen.getByText('Pound')).toBeInTheDocument() // hidden global still listed
  await userEvent.click(screen.getByRole('switch', { name: 'Active only' }))
  expect(await screen.findByText('Points')).toBeInTheDocument()
  expect(localStorage.getItem('settings.currencies.activeOnly')).toBe('false')
})

it('base and profile currency switches are disabled with tooltips', async () => {
  renderPage()
  const usdSwitch = await screen.findByRole('switch', { name: 'show US Dollar' })
  expect(usdSwitch).toBeDisabled()
  const eurSwitch = screen.getByRole('switch', { name: 'show Euro' })
  expect(eurSwitch).toBeEnabled()
})
```

Adjust exact label strings to the real locale values (`Set exchange rate` = `classifications.currencies.modals.rate.header`; check `locales/en.json` and use what it says verbatim). The storage-key literal is `settings.currencies.activeOnly`. `getItem`/`setItem` JSON-encode values — assert with the encoding the categories test uses (check `PayeesTagsPages.test.tsx`; if it asserts `'false'` keep that form).

- [ ] **Step 2: Run to verify RED**

Run: `cd web && pnpm test -- CurrenciesPage 2>&1 | tail -8`
Expected: new cases FAIL (no kebab/sheet/filter yet); old cases may fail where they drove the retired inline UI. That is the correct red state.

- [ ] **Step 3: Commit the red tests? NO — proceed to Task 3 in the same working tree.**

(Web tests must be green at every commit; Tasks 2+3 land as one commit.)

---

### Task 3: Recompose `CurrenciesPage` on the template (GREEN)

**Files:**
- Modify: `web/src/features/currencies/CurrenciesPage.tsx`

**Interfaces:**
- Consumes: Task 1 props (`RowAction`, `RowSwitchState`, `meta`, `rowSwitch`, `hasActions`, `extraActions`, `alert`), existing hooks/dialogs.

- [ ] **Step 1: Rewrite the page**

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { CurrencyListItemDto } from '@/api/dto/currency'
import { apiErrorMessage } from '@/lib/apiError'
import { ClassificationList, type RowSwitchState } from '@/features/classifications/ClassificationList'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { CurrencyDialog } from './CurrencyDialog'
import { RateDialog } from './RateDialog'
import { useCurrencies, useCurrencyRates, useCreateCurrency, useUpdateCurrency, useSetCurrencyRate, useArchiveCurrency, useUnarchiveCurrency, useDeleteCurrency, useHideCurrency, useShowCurrency } from './queries'

type CurrencyRow = CurrencyListItemDto & { position: number }

export function CurrenciesPage() {
  // hooks exactly as today (user, currencies, rates, all 8 mutations)
  // dialog / rateDialog / error / rateError state exactly as today
  const own = currencies?.filter((c) => c.scope === 'own') ?? []
  const globals = currencies?.filter((c) => c.scope === 'global') ?? []
  const items: CurrencyRow[] = [...own, ...globals].map((c, i) => ({ ...c, position: i }))
  const baseId = rates?.[0]?.baseCurrencyId
  const profileId = userCurrencyId(user)
  const rateFor = (id: string) => rates?.find((r) => r.currencyId === id)
  const baseCurrency = currencies?.find((c) => c.id === baseId)

  const mutate = (fn: { mutate: (id: string, opts: object) => void }, id: string) => {
    setError(null)
    fn.mutate(id, { onError: (e: unknown) => setError(apiErrorMessage(e)) })
  }

  return (
    <>
      <ClassificationList<CurrencyRow>
        title={t('classifications.currencies.pages.settings.header')}
        heading={t('classifications.currencies.pages.settings.menu_item')}
        info={own.length === 0 ? t('classifications.currencies.pages.settings.empty_state') : undefined}
        alert={error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
        createLabel={t('classifications.currencies.pages.settings.create_currency')}
        deleteTitle={t('classifications.currencies.modals.delete.title')}
        items={items}
        storageKey="settings.currencies.activeOnly"
        sections={[
          { label: t('classifications.currencies.pages.settings.my_currencies'), match: (c) => c.scope === 'own' },
          { label: t('classifications.currencies.pages.settings.global_currencies'), match: (c) => c.scope === 'global' },
        ]}
        hasActions={(c) => c.scope === 'own'}
        extraActions={(c) => [{ label: t('classifications.currencies.modals.rate.header'), onSelect: () => setRateDialog({ open: true, currency: c }) }]}
        meta={(c) => {
          const rate = c.scope === 'own' ? rateFor(c.id) : undefined
          return (
            <>
              <span className="truncate text-xs text-muted-foreground">{c.scope === 'own' ? `${c.code} · ${c.symbol}` : c.code}</span>
              {rate ? (
                <span className="truncate text-xs text-muted-foreground">
                  {t('classifications.currencies.pages.settings.rate_caption', { base: baseCurrency?.code ?? '', rate: rate.rate, code: c.code })}
                </span>
              ) : null}
            </>
          )
        }}
        rowSwitch={(c): RowSwitchState => {
          if (c.scope === 'own') {
            return {
              checked: c.isArchived === 0,
              ariaLabel: `archive ${c.name}`,
              onToggle: () => mutate(c.isArchived === 0 ? archiveCurrency : unarchiveCurrency, c.id),
            }
          }
          const locked = c.id === baseId || c.id === profileId
          return {
            checked: c.isHidden === 0,
            disabled: locked,
            title: c.id === baseId
              ? t('classifications.currencies.pages.settings.locked_base')
              : c.id === profileId
                ? t('classifications.currencies.pages.settings.locked_profile')
                : undefined,
            ariaLabel: `show ${c.name}`,
            onToggle: () => mutate(c.isHidden === 0 ? hideCurrency : showCurrency, c.id),
          }
        }}
        onCreate={() => setDialog({ open: true, currency: null })}
        onEdit={(c) => setDialog({ open: true, currency: c })}
        onDelete={(id) => mutate(deleteCurrency, id)}
      />
      {/* CurrencyDialog / RateDialog exactly as today (same handlers) */}
    </>
  )
}
```

The old hand-rolled SettingsShell/sections/Badge/ConfirmDialog markup is deleted (the template owns shell, captions, sheet, confirm). Keep the `rateError` wiring on `RateDialog` unchanged.

- [ ] **Step 2: Run to verify GREEN**

Run: `cd web && pnpm test -- CurrenciesPage ClassificationList CategoriesPage PayeesTagsPages 2>&1 | tail -6`
Expected: all PASS.

- [ ] **Step 3: Lint + commit Tasks 2+3 together**

```bash
cd web && pnpm lint
git add web/src/features/currencies/ web/src/features/classifications/
git commit -m "feat(web): currencies page on the classification template"
```

---

### Task 4: Locale cleanup + full verification

**Files:**
- Modify: `locales/en.json`, `locales/ru.json` (remove `classifications.currencies.modals.delete.question`)

- [ ] **Step 1: Retire the unused key from both catalogues**

Remove the `"question"` entry under `classifications.currencies.modals.delete` in BOTH files (the template's ConfirmDialog shows the currency name, like categories). Keep `"title"`.

- [ ] **Step 2: Full verification**

Run: `go test ./internal/test/i18ntest/ && cd web && pnpm test 2>&1 | tail -4 && pnpm lint 2>&1 | tail -2`
Expected: i18n guards PASS (en/ru parity holds), full web suite PASS, lint clean.

- [ ] **Step 3: Visual verification in the browser**

Rebuild the SPA (`cd web && pnpm build`), boot the demo server (`DATABASE_URL="sqlite:///$PWD/var/db/demo.sqlite" PORT=8181 ECONUMO_ADMIN_PORT=8285 ./econumo serve`), log in as John, open Settings → Currencies. Verify: section captions, PTS row kebab (Edit / Set rate / Delete), active-only toggle, locked USD switch tooltip, mobile viewport sheet (resize_window preset mobile). Screenshot for the user. Revoke the verification session afterwards.

- [ ] **Step 4: Commit**

```bash
git add locales/
git commit -m "chore(i18n): retire the bespoke currency delete question"
```
