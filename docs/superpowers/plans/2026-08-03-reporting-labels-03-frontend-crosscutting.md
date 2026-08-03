# Reporting Labels — Plan 3: Frontend + Cross-cutting

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface labels in the SPA — one unified "Tags" experience where budgeting tags and reporting labels are distinguished by a hardcoded per-kind icon and colour — and finish the MCP, analytics and i18n surfaces.

**Architecture:** Labels are a second entity with their own API client and TanStack Query hooks, mirroring tags. The UI aggregates both under the word "Tags": one flat chip list in the transaction editor (tags radio-like, labels free-toggle) and one settings list. The icon is **read from the stored row**, never re-derived from kind; the colour is a frontend constant keyed on kind.

**Tech Stack:** React 19, TypeScript (strict), TanStack Query v5, react-i18next, Vitest + RTL + MSW, oxlint, Tailwind 4 / shadcn.

**Spec:** `docs/superpowers/specs/2026-08-02-multi-tag-labels-design.md` — Delivery phases **6 and 7**.

**Depends on:** Plans 1 and 2 — the `/api/v1/label/*` routes, `labelIds` on transactions and recurring, the `get-budget` `structure.labels` block, the `labelId` drill-down param, and the CSV `labels` column must all exist.

## Global Constraints

- **No `enum`** — use an `as const` object + union type (the TS config uses `erasableSyntaxOnly`).
- **`pnpm exec tsc -b` is a required gate.** vitest and oxlint do **not** type-check; a TS error has shipped past task gates before. Run it before claiming any frontend task done.
- Verify each task with `cd web && pnpm vitest run <files>`; the final task runs `pnpm test`, `pnpm lint`, and `pnpm exec tsc -b`.
- **Every new `METRICS` key must be referenced literally** as `METRICS.LABEL_CREATE` in non-test `src/` code, or `metrics-coverage.test.ts` fails. The `NOT_WIRED` escape list is currently empty — keep it empty.
- i18n keys live in `locales/*.json` (shared by both stacks, **not** under `web/src`). A new key must be added to **every** shipped language or the `i18ntest` guard fails; `{var}` placeholder sets must match across languages.
- Plural strings are single pipe-joined values read via `pluralPick` (`web/src/lib/plural.ts`) — i18next plural suffixes are not used.
- Wire shapes: `isArchived` is `0 | 1`, not boolean.
- TDD: failing test → minimal code → green → commit. One commit per task.

## Decisions locked before coding

| Question | Answer |
|---|---|
| Icon source at render time | the `icon` **field on the row** (never re-derived from kind) |
| Colour source | frontend constant keyed on kind — tags one colour, labels another |
| Chip layout | one flat list under a single "Tags" heading |
| Tag selection | radio-like (picking one deselects the previous) |
| Label selection | free multi-toggle |
| Where chips show | same gate as tags today: non-transfer expense rows |

---

### Task 1: Label API client, DTO, and query hooks

**Files:**
- Create: `web/src/api/dto/label.ts`, `web/src/api/label.ts`
- Modify: `web/src/api/dto/tag.ts` (add `icon`)
- Modify: `web/src/features/classifications/queries.ts`
- Test: `web/src/features/classifications/queries.test.tsx`

**Interfaces:**
- Consumes: `/api/v1/label/*`.
- Produces: `LabelDto {id, ownerUserId, name, icon, position, isArchived: 0|1, createdAt, updatedAt}`; `labelApi.{getLabels,createLabel,updateLabel,archiveLabel,unarchiveLabel,deleteLabel,orderLabels}`; hooks `useLabels`, `useCreateLabel`, `useUpdateLabel`, `useArchiveLabel`, `useUnarchiveLabel`, `useDeleteLabel`, `useOrderLabels`.

- [ ] **Step 1: Write the failing test**

Add to `web/src/features/classifications/queries.test.tsx`:

```tsx
it('useLabels returns the labels from the API', async () => {
  server.use(
    http.get('*/api/v1/label/get-label-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [
        { id: 'l1', ownerUserId: 'u1', name: 'Kid A', icon: 'label', position: 0,
          isArchived: 0, createdAt: '2026-08-01 10:00:00', updatedAt: '2026-08-01 10:00:00' },
      ] } }),
    ),
  )
  const { result } = renderHook(() => useLabels(), { wrapper })
  await waitFor(() => expect(result.current.data).toHaveLength(1))
  expect(result.current.data?.[0].icon).toBe('label')
})

// Tags and labels have INDEPENDENT name namespaces on the backend, so the
// create-dedupe must not resolve a label to a same-named tag.
it('useCreateLabel does not dedupe against an existing tag of the same name', async () => {
  // seed the tags cache with { id: 't1', name: 'Travel' }
  // create a label named 'Travel' -> the API must be called, not short-circuited
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/classifications/queries.test.tsx`
Expected: FAIL — `useLabels` is not exported

- [ ] **Step 3: Add the DTO and API client**

Create `web/src/api/dto/label.ts`:

```ts
export interface LabelDto {
  id: Id
  ownerUserId: Id
  name: string
  /** Material Symbols ligature, stored per row. Always rendered from this value
   *  rather than derived from the kind, so user-picked icons need no UI change. */
  icon: string
  position: number
  isArchived: 0 | 1
  createdAt: string
  updatedAt: string
}
```

Create `web/src/api/label.ts` mirroring `web/src/api/tag.ts` one-for-one against the seven `/api/v1/label/*` routes. Add `icon: string` to `TagDto` in `web/src/api/dto/tag.ts`.

- [ ] **Step 4: Add the hooks**

In `web/src/features/classifications/queries.ts`, add the label hooks mirroring the tag ones with query key `['labels']`.

**Critical:** `useCreateTag` short-circuits on a lowercased-name lookup (`findByName`). Labels must use their **own** cache and their own dedupe — a label named "Travel" must not resolve to a tag named "Travel", because the backend keeps independent namespaces. Give `useCreateLabel` a `findByName` scoped to the `labels` cache only.

- [ ] **Step 5: Run tests and type-check**

Run: `cd web && pnpm vitest run src/features/classifications/queries.test.tsx && pnpm exec tsc -b`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/api web/src/features/classifications
git commit -m "feat(web): add the label API client and query hooks"
```

---

### Task 2: Per-kind icon and colour rendering

**Files:**
- Create: `web/src/lib/classificationKind.ts`
- Modify: the shared icon component (`EntityIcon`) to accept a colour class
- Test: `web/src/lib/classificationKind.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `ClassificationKind = 'tag' | 'label'`; `DEFAULT_ICON: Record<ClassificationKind, string>` (`{tag: 'tag', label: 'label'}`); `kindAccentClass(kind): string`.

- [ ] **Step 1: Write the failing test**

Create `web/src/lib/classificationKind.test.ts`:

```ts
import { DEFAULT_ICON, kindAccentClass } from './classificationKind'

it('maps each kind to its default Material ligature', () => {
  expect(DEFAULT_ICON.tag).toBe('tag')     // renders as a hashtag
  expect(DEFAULT_ICON.label).toBe('label')
})

it('gives the two kinds visually distinct accents', () => {
  expect(kindAccentClass('tag')).not.toBe(kindAccentClass('label'))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/lib/classificationKind.test.ts`
Expected: FAIL — module not found

- [ ] **Step 3: Write the module**

Create `web/src/lib/classificationKind.ts`. No `enum` — `as const` + union:

```ts
export const CLASSIFICATION_KINDS = ['tag', 'label'] as const
export type ClassificationKind = (typeof CLASSIFICATION_KINDS)[number]

/** Seeded server-side at create; kept here only to preview a not-yet-saved row
 *  in the create dialog. Saved rows always render their stored icon. */
export const DEFAULT_ICON: Record<ClassificationKind, string> = {
  tag: 'tag',
  label: 'label',
}

/** Colour encodes the KIND (budgeting vs reporting), not identity, so it is a
 *  constant rather than a stored field. */
export function kindAccentClass(kind: ClassificationKind): string {
  return kind === 'tag' ? 'text-sky-600 dark:text-sky-400' : 'text-violet-600 dark:text-violet-400'
}
```

Extend `EntityIcon` with an optional `className` (or `accentClass`) prop so the caller can tint the glyph — it currently has no colour prop; colours exist only for avatars via `avatarColorAccents` in `web/src/lib/avatars.ts`. Follow that file's class-map idiom.

- [ ] **Step 4: Run tests and type-check**

Run: `cd web && pnpm vitest run src/lib/classificationKind.test.ts && pnpm exec tsc -b`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/lib web/src/components
git commit -m "feat(web): add per-kind icon defaults and accent colours"
```

---

### Task 3: Create/edit dialog with a kind radio

**Files:**
- Modify: the existing tag create/edit dialog under `web/src/features/classifications/`
- Modify: the classifications settings page to list both kinds
- Test: the dialog's test file

**Interfaces:**
- Consumes: `useCreateTag`/`useCreateLabel`, `DEFAULT_ICON`, `kindAccentClass`.
- Produces: a dialog with a name input + kind radio that previews the kind's icon live.

- [ ] **Step 1: Write the failing test**

```tsx
it('previews the kind icon and swaps it live with the radio', async () => {
  render(<TagDialog open onClose={noop} />)
  expect(screen.getByTestId('kind-icon')).toHaveTextContent('tag')
  await userEvent.click(screen.getByRole('radio', { name: /label/i }))
  expect(screen.getByTestId('kind-icon')).toHaveTextContent('label')
})

it('creates a label when the label kind is selected', async () => {
  // fill the name, pick "label", submit -> createLabel called, createTag not
})

it('hides the kind radio when editing an existing row', async () => {
  // kind is immutable: converting between kinds is explicitly out of scope
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/classifications`
Expected: FAIL — no kind radio

- [ ] **Step 3: Build the dialog**

Add a `kind` radio (budgeting tag / reporting label) shown **only on create** — kind is immutable, since the spec rules out in-place conversion. Render the icon preview from `DEFAULT_ICON[kind]` tinted with `kindAccentClass(kind)`, swapping as the radio changes. On submit, dispatch to `useCreateTag` or `useCreateLabel` by kind. No icon picker.

For the settings list: `ClassificationList` already supports `showIcon` and `sections`, and `ClassificationItem` already accepts `icon?: string` — render saved rows with their **stored** `icon` plus the kind accent. Compose the two lists (tags and labels) into the one "Tags" screen.

- [ ] **Step 4: Run tests and type-check**

Run: `cd web && pnpm vitest run src/features/classifications && pnpm exec tsc -b`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/classifications
git commit -m "feat(web): pick tag vs label in the create dialog"
```

---

### Task 4: Transaction editor chips

**Files:**
- Modify: `web/src/features/transactions/useTransactionForm.ts` (add `labelIds`)
- Modify: `web/src/features/transactions/TransactionDialog.tsx`
- Modify: `web/src/features/transactions/AddTagDialog.tsx` (inline create picks kind)
- Modify: the recurring editor to mirror it
- Test: `useTransactionForm.test.ts`, `TransactionDialog.test.tsx`

**Interfaces:**
- Consumes: `useTags`, `useLabels`, `DEFAULT_ICON`, `kindAccentClass`.
- Produces: `TransactionFormState.labelIds: Id[]`; `buildPayload` emits `labelIds`.

- [ ] **Step 1: Write the failing test**

```ts
it('toggles labels freely but keeps tags single-select', () => {
  const s0 = initialFormState()
  const s1 = toggleTag(s0, 't1')
  const s2 = toggleTag(s1, 't2')
  expect(s2.tagId).toBe('t2')            // radio-like: the second replaces the first

  const s3 = toggleLabel(s2, 'l1')
  const s4 = toggleLabel(s3, 'l2')
  expect(s4.labelIds).toEqual(['l1', 'l2'])   // free multi-select
  expect(toggleLabel(s4, 'l1').labelIds).toEqual(['l2'])
})

it('clears labels for a transfer, as it does for the tag', () => {
  // buildPayload on a transfer sends labelIds: []
})

it('hydrates labelIds from an existing transaction', () => {
  // initialFormState(tx with labelIds) -> same ids
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/transactions/useTransactionForm.test.ts`
Expected: FAIL — `toggleLabel` is not exported

- [ ] **Step 3: Extend the form state**

Add `labelIds: Id[]` to `TransactionFormState`, hydrate it from `tx.labelIds` / `rt.labelIds`, add `toggleLabel`, and emit `labelIds` in `buildPayload` — nulled to `[]` for transfers, mirroring how `tagId` is nulled.

- [ ] **Step 4: Render one flat chip list**

In `TransactionDialog.tsx`, render tags and labels as chips in a **single flat list** under the existing "Tags" heading, keeping the current `isExpense` gate. Each chip shows its **stored** `icon` tinted by `kindAccentClass`, plus the name. Wire `aria-checked` per kind: tags behave radio-like (`form.tagId === tag.id`), labels toggle (`form.labelIds.includes(label.id)`).

Extend the inline create dialog to pick the kind and set the created id into the right slot.

- [ ] **Step 5: Run tests and type-check**

Run: `cd web && pnpm vitest run src/features/transactions && pnpm exec tsc -b`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/features/transactions web/src/features/recurring
git commit -m "feat(web): attach many labels to a transaction"
```

---

### Task 5: Budget labels block + drill-down

**Files:**
- Modify: `web/src/api/dto/budget.ts` (structure gains `labels`)
- Modify: `web/src/api/budget.ts` (`BudgetTransactionsParams` gains `labelId`)
- Modify: `web/src/features/budgets/BudgetTable.tsx`, `BudgetTransactionsDialog.tsx`
- Test: `BudgetTable.test.tsx`, `BudgetTransactionsDialog.test.tsx`

**Interfaces:**
- Consumes: `structure.labels: LabelSpendDto[]`, `labelId` param.
- Produces: a collapsible labels section near the uncategorized row, each chip clickable.

- [ ] **Step 1: Write the failing test**

```tsx
it('renders the labels block with per-label spend', async () => {
  // structure.labels: [{id:'l1', name:'Kid A', icon:'label', isArchived:0,
  //   spent:'50.00', ownerUserId:'u1'}] -> "Kid A" and "50.00" visible
})

it('omits the block entirely when there are no labels', async () => {
  // structure.labels: [] -> no section heading
})

it('opens the transactions dialog filtered by labelId', async () => {
  // clicking the chip issues get-transaction-list?labelId=l1
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/budgets`
Expected: FAIL — `labels` is not on the structure type

- [ ] **Step 3: Add the types and render the block**

Add `LabelSpendDto {id, name, icon, isArchived: 0|1, spent, ownerUserId}` and `labels: LabelSpendDto[]` to the budget structure DTO. Render a collapsible section near the uncategorized bucket, hidden entirely when the list is empty. Each row: stored icon tinted with the label accent, name, and spent amount.

Add a short explanatory line (i18n key) that these totals **overlap and do not sum to total spend** — without it the numbers look like a bug.

Add `labelId?: Id` to `BudgetTransactionsParams` and set it in the query string; extend `BudgetTransactionsTarget` so a label click routes to the `labelId` branch. Note the existing spread-order comment in `BudgetTransactionsDialog.tsx` — put the label branch alongside the tag branch so it cannot clobber another selector.

- [ ] **Step 4: Run tests and type-check**

Run: `cd web && pnpm vitest run src/features/budgets && pnpm exec tsc -b`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/api web/src/features/budgets
git commit -m "feat(web): show the budget labels block with drill-down"
```

---

### Task 6: CSV import — labels column, separator dialog, count preview

**Files:**
- Modify: `web/src/features/transactions/importCsv.ts`
- Modify: `web/src/features/transactions/ImportCsvDialog.tsx`
- Test: `importCsv.test.ts`, `ImportCsvDialog.test.tsx`

**Interfaces:**
- Consumes: `useLabels`, the backend `labels` mapping key + `labelsSeparator` + `labelIds`.
- Produces: a `labels` field in the mapping UI, a separator dialog, and a new-label count preview.

- [ ] **Step 1: Write the failing test**

```ts
it('includes labels in the mapping payload', () => {
  // sel.modes.labels === 'csv_column' -> mapping.labels = selected column
  // unmapped -> mapping.labels = null
})

it('sends the chosen separator, defaulting to ";"', () => {
  // default payload carries labelsSeparator: ';'
  // after choosing '|' -> labelsSeparator: '|'
})

it('counts how many NEW labels the import will create', () => {
  // cells "Kid A;Kid B" and "Kid A" with existing label "Kid A"
  // -> newLabelCount === 1 (case-insensitive, deduped)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/transactions/importCsv.test.ts`
Expected: FAIL — `labels` is not a field key

- [ ] **Step 3: Add the field and separator**

Add `'labels'` to the field keys and `autoDetect` (new i18n leaf `transactions.import_csv.fields.labels` — note field keys are camelCase while i18n leaves are snake_case, mapped by hand in the `labels` object; rename that local variable if it now collides with the new field name). `buildImportPayload` sets `mapping.labels` in `csv_column` mode and always sends `labelsSeparator`.

Add a **separator button** next to the mapped labels row opening a dialog with presets (`;` default, `,`, `|`, tab, newline) plus a custom free-text value. Show it only in `csv_column` mode.

In "existing" mode, add an owner-scoped label multi-select sending `labelIds` (reuse the `targetUserId` scoping and the effect that clears fixed picks when the owner changes).

- [ ] **Step 4: Add the count preview**

Compute, client-side from the parsed rows, the distinct label names that do not already exist for the target owner (case-insensitive), and show "this import will create N new labels" before the user confirms. The SPA already parses the CSV locally, so no backend call is needed.

- [ ] **Step 5: Run tests and type-check**

Run: `cd web && pnpm vitest run src/features/transactions && pnpm exec tsc -b`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/features/transactions
git commit -m "feat(web): map a labels column with a separator picker and preview"
```

---

### Task 7: Analytics keys

**Files:**
- Modify: `web/src/lib/metrics.ts`
- Modify: the label hooks in `web/src/features/classifications/queries.ts`
- Test: `web/src/lib/metrics-coverage.test.ts` (must stay green)

**Interfaces:**
- Consumes: `trackEvent`.
- Produces: `LABEL_CREATE`, `LABEL_UPDATE`, `LABEL_ORDER_LIST`, `LABEL_DELETE`, `LABEL_ARCHIVE`, `LABEL_UNARCHIVE`.

- [ ] **Step 1: Write the failing test**

```ts
it('fires LABEL_CREATE when a label is created', async () => {
  // spy on window.dataLayer; run useCreateLabel; expect event 'appLabelCreate'
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm vitest run src/features/classifications`
Expected: FAIL — `METRICS.LABEL_CREATE` undefined

- [ ] **Step 3: Add the keys and fire them**

In `web/src/lib/metrics.ts`, next to the `TAG_*` block (names are frozen `app`-prefixed camelCase; the PostHog snake_case name derives automatically):

```ts
  LABEL_CREATE: 'appLabelCreate',
  LABEL_UPDATE: 'appLabelUpdate',
  LABEL_ORDER_LIST: 'appLabelOrderList',
  LABEL_DELETE: 'appLabelDelete',
  LABEL_ARCHIVE: 'appLabelArchive',
  LABEL_UNARCHIVE: 'appLabelUnarchive',
```

Fire each at the corresponding hook's success point, matching the tag hooks. Every key must appear **literally** as `METRICS.LABEL_*` in non-test source — the coverage test scans with `/METRICS\.([A-Z_0-9]+)/`, so a computed lookup will not satisfy it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm vitest run src/lib/metrics-coverage.test.ts src/features/classifications`
Expected: PASS, with `NOT_WIRED` still empty

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/metrics.ts web/src/features/classifications
git commit -m "feat(web): track label lifecycle events"
```

---

### Task 8: i18n catalogues

**Files:**
- Modify: every `locales/*.json`
- Test: `internal/test/i18ntest` (Go guard)

**Interfaces:**
- Consumes: nothing.
- Produces: `classifications.labels.*`, the labels-block strings, the CSV `labels` field label and separator dialog strings.

- [ ] **Step 1: Run the guard to see what is missing**

Run: `go test ./internal/test/i18ntest/ -v`
Expected: FAIL listing frontend `t()` keys with no catalogue entry

- [ ] **Step 2: Add the English keys**

In `locales/en.json`, add a `classifications.labels` subtree mirroring `classifications.tags` (`pages.settings.{menu_item,header,info,create_label,archived_item}`, `modals.{edit,create,delete}`, `forms.label.name.{label,validation}`), plus:

- the kind radio labels (budgeting tag / reporting label) with a one-line explanation of the difference;
- the budget labels-block heading and the overlap note ("A transaction can carry several labels, so these totals overlap and do not add up to your total spending.");
- `transactions.import_csv.fields.labels`, the separator dialog title, its preset names, and the "will create N new labels" string — authored as a **single pipe-joined plural value** (`"one | many"`) read via `pluralPick`, not an i18next plural suffix.

- [ ] **Step 3: Translate into every other language**

Add the same keys to **every** other `locales/*.json`. Placeholder sets must match per key across languages (e.g. `{count}` present everywhere), and Russian plurals need three pipe-joined variants.

- [ ] **Step 4: Run the guards**

Run: `go test ./internal/test/i18ntest/ -v && cd web && pnpm vitest run`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add locales
git commit -m "feat(i18n): add label catalogue keys in every language"
```

---

### Task 9: MCP surface

**Files:**
- Create: `internal/label/mcp/mcp.go`
- Modify: `internal/server/server.go` (register), the transaction MCP tool (`labelIds`)
- Test: `internal/label/mcp/mcp_test.go`, `internal/test/mcpparity/` goldens

**Interfaces:**
- Consumes: `applabel.ReadService`, `applabel.Service`.
- Produces: tools `list_labels`, `create_label`, `update_label`, `set_label_archived`.

- [ ] **Step 1: Write the failing test**

Mirror `internal/tag/mcp/mcp_test.go`: `list_labels` returns the caller's labels; `create_label` mints its own operation id server-side; `set_label_archived` toggles both ways.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/label/mcp/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write the MCP edge**

Create `internal/label/mcp/mcp.go` mirroring `internal/tag/mcp/mcp.go`: input structs with `jsonschema` tags, `Register(read *applabel.ReadService, write *applabel.Service) webmcp.Register`, each handler calling `reqctx.AddLogAttr(ctx, "tool", ...)`, `webmcp.UserID(ctx)`, and mapping errors with `webmcp.MapErr`. `create_label` mints the operation id with `vo.NewId().String()`, as `create_tag` does.

Add `labelIds` to the transaction MCP tool's input/output. Register in `internal/server/server.go` inside `webmcp.Compose(...)`:

```go
		labelmcp.Register(labelReadSvc, labelSvc),
```

- [ ] **Step 4: Regenerate MCP goldens and inspect**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/ && git diff internal/test/mcpparity/testdata/`
Expected: `tools/list` gains the four label tools; transaction tools gain `labelIds`. **Inspect** the diff.

- [ ] **Step 5: Commit**

```bash
git add internal/label/mcp internal/server internal/transaction internal/test/mcpparity
git commit -m "feat(mcp): expose label tools"
```

---

### Task 10: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Frontend gates**

Run: `cd web && pnpm test && pnpm lint && pnpm exec tsc -b`
Expected: all PASS. `tsc -b` is not optional — vitest and oxlint do not type-check.

- [ ] **Step 2: Backend smoke tier**

Run: `make go-test`
Expected: PASS, coverage ≥ `GO_COVER_MIN`

- [ ] **Step 3: Full suite including PostgreSQL and engine comparison**

Run: `make test`
Expected: PASS

- [ ] **Step 4: Manual smoke against a real instance**

Boot the app, then confirm end to end:
1. Create a budgeting tag and a reporting label — both appear under "Tags" with distinct icons/colours.
2. Put one expense on a cafe with **two** labels; confirm the budget labels block shows the **full** amount under each, and that the envelope/available figures are unchanged.
3. Click a label chip → the transactions dialog lists that expense.
4. Export CSV → `labels` sits right after `tag`, semicolon-joined.
5. Re-import that CSV with the default separator → labels round-trip, and the preview reports 0 new labels.

- [ ] **Step 5: Commit any fixes**

```bash
git add -A
git commit -m "test(labels): full-suite verification"
```

---

## Self-Review

**Spec coverage (phases 6–7):** label API + hooks → Task 1; per-kind icon/colour → Task 2; create/edit dialog + settings list → Task 3; transaction + recurring chips → Task 4; budget block + drill-down → Task 5; CSV import UI incl. separator dialog and the count preview deferred from Plan 2 → Task 6; analytics → Task 7; i18n → Task 8; MCP → Task 9; verification → Task 10.

**Two traps called out explicitly, both found by reading the current source:**
1. `useCreateTag` dedupes by lowercased name. Because tags and labels have independent backend namespaces, sharing that cache would silently resolve a new label to a same-named tag. Task 1 scopes the dedupe per kind.
2. `EntityIcon` has no colour prop today — colours exist only for avatars. Task 2 extends it rather than assuming it works.

**Type consistency:** `LabelDto` (API) → `LabelSpendDto` (budget block) are distinct types for distinct payloads. `labelIds` is `Id[]` throughout the form state and `string[]` on the wire. `DEFAULT_ICON`/`kindAccentClass` are imported from `classificationKind.ts` in Tasks 3, 4 and 5 under those exact names.

**Note on `ImportCsvDialog.tsx`:** it already has a local variable named `labels` (the i18n field-label map). Adding a `labels` **field key** collides — rename the local to `fieldLabels` as part of Task 6 rather than shadowing it.
