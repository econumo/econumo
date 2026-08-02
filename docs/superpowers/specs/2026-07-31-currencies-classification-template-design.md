# Currencies page on the classification template — design

Date: 2026-07-31
Status: approved design, pre-implementation

## Problem

Settings → Currencies is hand-rolled and drifts from the classification
template used by Categories/Tags/Payees (the shared `ClassificationList`
component) and Recurring (a visual copy of the same template): no compact
bottom-sheet actions (the kebab stays tiny on mobile), no persisted
"Active only" filter, archived state shown as a Badge instead of the
template's greyed caption, and hand-built section headers.

## Decisions (from brainstorming)

- Currencies adopts: **full row behavior** (tap-row → bottom sheet on
  compact / kebab on desktop, archived caption under the name),
  **active-only filter**, **section captions** ("My currencies" /
  "Global currencies" with the divider line).
- **No reordering** — currencies have no `position`; the server's by-code
  order stays. The template's drag/reorder UI must therefore be optional.
- Mechanism: **extend `ClassificationList` with optional props whose
  defaults preserve today's behavior exactly** (approach A). Categories/
  Tags/Payees pages don't change; Recurring is untouched.

## Design

### 1. `ClassificationList` extensions (all optional, defaults = today)

```ts
interface RowAction {
  label: string
  destructive?: boolean
  onSelect: () => void
}

interface RowSwitch {
  checked: boolean
  disabled?: boolean
  title?: string
  ariaLabel: string
  onToggle: () => void
}

// new optional props on ClassificationListProps<T>:
onOrder?: (changes: { id: string; position: number }[]) => void
//   was required; when absent: no drag grips, no reorder button, no SortDialog.
meta?: (item: T) => ReactNode
//   extra muted lines under the name (code · symbol, rate caption).
rowSwitch?: (item: T) => RowSwitch
//   default: the archive semantics (checked = !isArchived, onToggle =
//   onToggleArchive with the sticky-archived behavior). Custom values render
//   verbatim; the sticky-set bookkeeping still applies when an item flips
//   from isArchived 0 while the active-only filter is on.
hasActions?: (item: T) => boolean
//   default () => true. false: no kebab, row not clickable, no sheet.
extraActions?: (item: T) => RowAction[]
//   default []. Rendered between Edit and Delete in both the kebab menu and
//   the compact sheet ("Set rate").
alert?: ReactNode
//   rendered above the list (below info) — the page's inline error banner.
```

Edit/Delete (with the component-owned ConfirmDialog) stay exactly as they
are; `extraActions` inserts between them. This deliberately does NOT allow
replacing the whole menu — suppression (`hasActions`) and insertion
(`extraActions`) cover the known needs with less API surface.

### 2. `CurrenciesPage` recomposition

Becomes a thin composition like `CategoriesPage`:

- `items`: own + global currencies (scope `shared` stays excluded, as
  today), adapted to `ClassificationItem` (`position` = array index; no
  icon, `showIcon` unset).
- `sections`: `[{label: my_currencies, match: scope === 'own'},
  {label: global_currencies, match: scope === 'global'}]`. The template
  shows captions when >1 section is visible and drops empty sections.
- `info`: when the user has no own currencies, pass the existing
  `empty_state` copy so the educational hint survives the template's
  section-dropping (categories use `info` the same way).
- `rowSwitch`:
  - own → archive/unarchive mutations (errors → the `alert` banner);
  - global → show/hide mutations; the base and profile currencies render
    disabled with the existing locked tooltips (disable the entry point,
    don't hide it — the house gating pattern).
- `hasActions`: `scope === 'own'` (globals get no kebab and no tap-sheet).
- `extraActions` (own): one entry — "Set rate" (opens `RateDialog`).
- `meta`: own → `code · symbol` line + the "1 USD = 10 PTS" rate caption;
  global → `code` line.
- `alert`: the page's inline server-error text (delete-in-use,
  cannot-hide, etc.) — kept, because these mutations have real backend
  refusals the UI must surface.
- Archived badge is dropped in favor of the template's greyed name +
  archived caption. `CurrencyDialog`/`RateDialog`/delete confirmation keep
  working; the delete confirmation now uses the template's title+name shape
  (the bespoke `modals.delete.question` copy is retired from both locale
  catalogues).

### 3. Out of scope

Reordering/positions for currencies, showing `shared`-scope currencies,
any backend change, changes to Recurring or the categories/tags/payees
pages (they must render byte-identically after the refactor).

## Testing

- Existing `CategoriesPage.test.tsx` / `PayeesTagsPages.test.tsx` pass
  unchanged — that is the proof the new props' defaults are invisible.
- `CurrenciesPage.test.tsx` reworked to the template interactions:
  - desktop: kebab menu on own rows (Edit / Set rate / Delete), NO kebab on
    global rows;
  - compact: tapping an own row opens the bottom sheet with the same three
    actions; tapping a global row does nothing;
  - active-only filter hides archived own currencies, never globals, and
    persists to localStorage;
  - locked base/profile switches render disabled with their tooltips;
  - archive/hide/show/delete mutations still fire, server refusals surface
    in the alert banner;
  - `vi`/msw fixtures unchanged (`coreHandlers`).
- `web-lint` + full vitest suite green.
