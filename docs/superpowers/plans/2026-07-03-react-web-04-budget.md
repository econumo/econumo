# React Web Migration — Plan 4 of 6: Budget Page

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Budget page at parity with `pages/Budget/Budget.vue` (1,241 lines, split per the spec): month strip, expense widget, the folder/element/children table with fold state, edit mode (budget-folder CRUD, envelope CRUD, drag move/reorder), set-limit (desktop inline + mobile long-press modal), change-element-currency, budget update modal, per-element transaction list, and the no-budget onboarding empty state. `/` renders the Budget page for onboarded users (Vue's `Home.vue` behavior).

**Architecture:** Extends Plans 1–3. Everything lands in `features/budgets/` (queries exist for the list; this plan adds the detail). Pure math (bucketing/stats/period range) goes in plain modules for TDD. There are **no charts** — the only visual is a progress bar (the spec's chart.js mapping is unused here).

## Scope notes (decided against the actual Vue code)

1. **Access/share modals stay in Plan 5** (`BudgetAccessModal`/`BudgetAccessLevelModal` are not wired into Budget.vue — they belong to the connections flow). `grant/revoke/accept/decline-access` and `exclude/include-account` API functions are included in the API module for Plan 5, no UI.
2. **Dead Vue code is not ported:** the no-op HTML5 child-row drag handlers, `transferEnvelopeBudget`, `copyEnvelopesBudget`, `resetBudget` (destructive on the backend — wipes all limits; the SPA never calls it), and the folder fold map (`BUDGET_FOLDED_FOLDERS` — written but never consumed by the table; only the ELEMENT unfold map affects rendering).
3. **Onboarding page itself is Plan 5** — `/` shows the Budget page when the user's `onboarding` option is `'completed'`, otherwise it keeps the empty placeholder until Plan 5 wires the real Onboarding page. The Budget page's own no-budget empty state (create budget / create account prompts) IS built now.

## Approved divergences introduced by this plan

- **Set-limit amount semantics unified.** Vue has two paths that disagree: the desktop inline editor sends `null` to clear when the value is 0/empty, while the mobile modal sends `Number(amount)` (so an empty modal sets a real `0` limit — the backend treats `"0"` as a zero limit, NOT a clear). React uses ONE rule everywhere: empty/0/NaN → `amount: null` (clear), else the normalized decimal string.
- **No `eval()`** — the inline limit editor and the modal both use the Plan-2 safe calculator.
- **Mutations refetch instead of hand-rolled rollback** (except set-limit): Vue optimistically mutates the store and rolls back on failure for envelope/folder/move/currency operations, then usually refetches anyway. React invalidates `['budget']` after each mutation (the page shows the refreshed truth). **Set-limit keeps the optimistic update** (instant feedback while typing limits) with rollback on error.
- **No localStorage mirror of the budget** (consistent with Plans 2–3); the persisted bits are only `budgetSelectedDate` and the unfolded-elements map.
- Everything else from Plans 1–3 still applies.

## Wire contract for this plan (verified against the Go source; all under `/api/v1/budget/`)

- **`GET get-budget?id=<id>&date=<Y-m-d>`** → `{item: BudgetResult}`:
  - `meta` (as Plan 3) — `startedAt` is a full datetime `"Y-m-d H:i:s"`.
  - `filters: {periodStart, periodEnd (datetimes), excludedAccountsIds: Id[]}`.
  - `balances: [{currencyId, startBalance, endBalance, income, expenses, exchanges, holdings}]` — all money fields **nullable decimal strings**: future month → everything null except `holdings`; current month → `endBalance` null; past month → all set.
  - `currencyRates: [{currencyId, baseCurrencyId, rate (string), periodStart, periodEnd (date-only Y-m-d)}]` — **period-scoped rates; ALL budget math uses these**, not the global rate list.
  - `structure: {folders: [{id, name, position}], elements: [ParentElement]}` where ParentElement = `{id, type: 0|1|2 (envelope|category|tag), name, icon, currencyId, isArchived: 0|1, folderId: string|null, position, budgeted, available, spent, budgetSpent (decimal strings), ownerUserId: string|null, children: [{id, type, name, icon, isArchived, spent, budgetSpent, ownerUserId (non-null string)}]}`.
  - `date` parsing is lenient (Y-m-d, datetime, or RFC3339; empty/garbage silently falls back to the caller-local current month via `X-Timezone`). Always snapped to first-of-month.
  - Element `id`s are the EXTERNAL ids (category/tag/envelope ids) — they are what set-limit/move/change-currency take.
- **Coercions** (exact Vue list): parent `spent, budgetSpent, budgeted, available` → Number; child `spent, budgetSpent` → Number; balances null-preserving Number; `currencyRates[].rate` → Number.
- **`POST set-limit`** `{budgetId, elementId, period: "Y-m-d" (strict), amount: string|number|null}` → `{}`. Amount null/absent = clear (no-op-clear still 200); `"0"` sets a real zero limit. Period must be ≥ first-of-month of `startedAt`, else 400.
- **`POST create-envelope`** `{budgetId, id: v7() (client id IS the entity id), name, icon, currencyId, folderId: Id|null, categories: Id[]}` → `{item: ParentElement}` (placed at position 0, money fields "0", `ownerUserId: null`). Name 3–64 → `"Envelope name must be 3-64 characters"`.
- **`POST update-envelope`** `{budgetId, id, name, icon, currencyId, isArchived: 0|1, categories}` → `{item}` (categories fully replace). **`POST delete-envelope`** `{budgetId, id}` → `{}` (owner/admin only — hide Delete for role `user`).
- **Budget folders:** `create-folder` `{budgetId, id: v7(), name}` → `{item: {id,name,position}}` (inserted at position 0); `update-folder` `{budgetId, id, name}`; `delete-folder` `{budgetId, id}`; `order-folder-list` `{budgetId, items: [{id, position}]}` → `{}`. Folder name 3–64 → `"Folder name must be 3-64 characters"`.
- **`POST move-element-list`** `{budgetId, items: [{id, folderId: Id|null, position}]}` → `{}` — handles BOTH reorder and folder moves; no type discriminator; contiguous renumber server-side.
- **`POST change-element-currency`** `{budgetId, elementId, currencyId}` → `{}`.
- **`POST update-budget`** (Plan-3 module) `{id, name, currencyId, excludedAccounts}` → `{item: MetaResult}`.
- **`GET get-transaction-list?budgetId=&periodStart=&categoryId?=&tagId?=&envelopeId?=`** → `{items: [transaction]}` (transaction shape ≈ the Plan-2 TransactionDto).
- All mutation validation failures use the standard 400 envelope; access failures 403.

---

### Task 1: Budget detail API + DTOs (TDD)

**Files:** Extend `web-react/src/api/dto/budget.ts` and `web-react/src/api/budget.ts`; test `web-react/src/api/budget.test.ts`.

DTO additions:
```ts
export const BudgetElementType = { ENVELOPE: 0, CATEGORY: 1, TAG: 2 } as const
export type BudgetElementType = (typeof BudgetElementType)[keyof typeof BudgetElementType]
export interface BudgetChildElementDto { id: Id; type: BudgetElementType; name: string; icon: string; isArchived: 0|1; spent: number; budgetSpent: number; ownerUserId: Id }
export interface BudgetElementDto extends Omit<BudgetChildElementDto, 'ownerUserId'> {
  currencyId: Id | null; folderId: Id | null; position: number
  budgeted: number; available: number; ownerUserId: Id | null; children: BudgetChildElementDto[]
}
export interface BudgetFolderDto { id: Id; name: string; position: number }
export interface BudgetBalanceDto { currencyId: Id; startBalance: number|null; endBalance: number|null; income: number|null; expenses: number|null; exchanges: number|null; holdings: number|null }
export interface BudgetRateDto { currencyId: Id; baseCurrencyId: Id; rate: number; periodStart: string; periodEnd: string }
export interface BudgetDto { meta: BudgetMetaDto; filters: {periodStart: string; periodEnd: string; excludedAccountsIds: Id[]}; balances: BudgetBalanceDto[]; currencyRates: BudgetRateDto[]; structure: {folders: BudgetFolderDto[]; elements: BudgetElementDto[]} }
```
API functions (coercing per the exact list): `getBudget(id, date): Promise<BudgetDto>`; `setLimit({budgetId, elementId, period, amount: string|null})`; `createEnvelope(form)` / `updateEnvelope(form)` / `deleteEnvelope(budgetId, id)`; `createBudgetFolder({budgetId, id, name})` / `updateBudgetFolder` / `deleteBudgetFolder` / `orderBudgetFolders({budgetId, items})`; `moveElements({budgetId, items})`; `changeElementCurrency({budgetId, elementId, currencyId})`; `getBudgetTransactions(params): Promise<TransactionDto[]>` (reuse `coerceTransaction`).

- [x] **Step 1: failing tests** with a wire-exact `get-budget` fixture (decimal strings, nullable balance fields incl. an all-null future month, `folderId: null`, children, rate strings) asserting every coercion; set-limit posts `amount: null` verbatim; move-element payload shape.
- [x] **Step 2: implement.** **Step 3:** PASS + build.
- [x] **Step 4: commit** `feat(web-react): budget detail API with wire-exact coercions`.

---

### Task 2: Budget period + fold state store, detail query, mutations (TDD)

**Files:** Extend `web-react/src/app/uiStore.ts` (or a new `features/budgets/budgetStore.ts`), `web-react/src/features/budgets/queries.ts`; tests alongside.

- **`useBudgetPeriodStore`** (zustand, persisted `budgetSelectedDate`): `selectedDate` normalized `YYYY-MM-01` (default = first of the current LOCAL month), `setPeriod(date)`. **`unfoldedElements: Record<Id, true>`** + `toggleElement(id)` + `resetFolds()` persisted under a second key (element rows default folded; presence = unfolded — Vue semantics).
- **`useBudget()`**: resolves the default budget id from the user's `budget` option (no fallback — `null` id means "no budget" → the query returns `null` and the page shows the empty state, mirroring Vue's rejected `'Budget is not selected'`); fetches `['budget', id, selectedDate]`, staleTime 10min. **A budget-id change resets the fold map** (Vue parity; period change does not).
- **Mutations:** `useSetLimit` — optimistic `setQueryData` patch of the element's `budgeted` (+ rollback in `onError`), no refetch on success; `useCreateEnvelope`/`useUpdateEnvelope`/`useDeleteEnvelope`/`useCreateBudgetFolder`/`useUpdateBudgetFolder`/`useDeleteBudgetFolder`/`useOrderBudgetFolders`/`useMoveElements`/`useChangeElementCurrency`/`useUpdateBudgetDetail` (update-budget + also refresh the `['budgets']` meta) — all invalidate `['budget']` on success. Metrics: `BUDGET_UPDATE`, `BUDGET_SET_LIMIT`-family events per the Vue store (check `metrics.ts` for exact names; wire what exists).
- Role helpers: `canConfigureBudget(meta, userId)` (owner|admin), `canUpdateLimits(meta, userId, selectedDate)` (owner|admin|user AND `startedAt` month ≤ selected month), `canDeleteEnvelope` (owner|admin).

- [x] **Step 1: failing tests** — set-limit optimistic patch + rollback on a 400; budget-id change clears the fold map, period change keeps it; no default budget → `data: null` without a network call; envelope create invalidates `['budget']`.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): budget detail query, period store and mutations`.

---

### Task 3: Budget math — buckets, stats, period range (TDD)

**Files:** Create `web-react/src/features/budgets/budgetMath.ts`; test alongside.

Pure ports of the Vue page computeds:
- `bucketElements(budget)` → `{withFolder: [{folder, elements, stats}], withoutFolder: {elements, stats}, archive: {elements, stats}}` — folder buckets ordered by folder position, `folderId !== null && !isArchived`; `withoutFolder` = `folderId === null && !isArchived`; `archive` = archived, name-sorted. **Zero folders → all non-archived elements land in `withoutFolder`** even if they carry a folderId (Vue quirk: `budgetWithFolder` returns `[]` when there are no folders).
- `bucketStats(elements, budget, exchangeFn)`: `budgeted += exchange(el.currencyId ?? meta.currencyId → meta.currencyId, el.budgeted)`; `spent += el.budgetSpent` (NOT exchanged); `available += exchange(..., el.available + el.budgeted)`. `budgetTotals(buckets)` sums the three.
- Exchange uses **`budget.currencyRates`** with the Plan-2 `exchange()` (same arithmetic — rates through `baseCurrencyId`, rounded to target fractionDigits).
- `periodRange(selectedDate, startedAt)` → 47 items (±23 months): `{value: 'YYYY-MM-01', label (MMMM same-year / MMM YYYY), isActive, outsideBudget (month < startedAt month)}`.
- Display helpers: `displaySpent(el) = -el.spent`, `displayAvailable(el) = el.available + el.budgeted` (color by sign), expense-widget math: `spent = |expenses| + (exchanges<0 ? |exchanges| : 0) + (holdings<0 ? |holdings| : 0)`; `total = |startBalance + income| + (exchanges>0) + (holdings>0)`; `progress = clamp(spent/total, 0, 1)` (0 when total ≤ 0); `overspent = spent > total` (null balance fields count as 0).

- [x] **Step 1: failing tests** — bucketing (incl. zero-folder quirk and archived name-sort), stats with a cross-currency element (exchanged budgeted/available, unexchanged budgetSpent), totals, period range labels/flags at a year boundary, widget math with a future-month all-null balance.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): budget bucketing, stats and period math`.

---

### Task 4: Month strip component (TDD)

**Files:** Create `web-react/src/features/budgets/PeriodStrip.tsx`; test alongside.

Horizontal scroll strip of the 47 `periodRange` items (no arrows/dropdown — Vue parity): active item styled, `outsideBudget` items dimmed but clickable; click → `setPeriod(value)` (the query refetches via the key). Center the active chip with `scrollIntoView({inline:'center'})` in an effect (no setTimeout chains).

- [x] **Step 1: failing tests** — renders labels, marks active, click fires setPeriod with `YYYY-MM-01`. **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): budget period strip`.

---

### Task 5: Expense widget (TDD)

**Files:** Create `web-react/src/features/budgets/ExpenseWidget.tsx`; test alongside.

Parity with `BudgetExpenseWidget.vue`: rendered only when a currency chip is selected; header `modules.budget.modal.expense_widget.header` ("Outflow vs. Total", `{period}` = "Mon YYYY" via `elements.date.month_short.*`); spent vs total amounts, progress bar (shadcn `Progress`), `-overbudget` styling when overspent; conversion-rate hint (`...expense_widget.conversion_rate`) when the selected currency ≠ budget currency, rate = `exchange(budgetCurrency → selected, 1)` over `budget.currencyRates`.

- [x] **Step 1: failing tests** — spent/total/progress values from a fixture balance, overspent class, conversion hint text with the rate. **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): budget expense widget`.

---

### Task 6: Budget table — folders, elements, children, totals (TDD)

**Files:** Create `web-react/src/features/budgets/BudgetTable.tsx`, `BudgetElementRow.tsx`, `BudgetTotalsRow.tsx`; tests alongside.

Parity with `BudgetTableFolder.vue`/`BudgetTotal.vue` (read mode; edit-mode actions arrive in Task 8):
- Folder section header: name + per-folder stat line `Budget / Spent / Available` (`modules.budget.page.budget.structure.tab.{budgeted,spent,available}`) via `moneyFormat` in the budget currency.
- Synthetic sections: "Default folder" (`...structure.no_folder`) for folderless, "Archived" (`...structure.in_archive`) — archived section never enters edit mode.
- Element row: `EntityIcon` + name; budget column (`budgeted`, the set-limit target); spent column (`-spent`); available column (`available + budgeted`, `income-color`/`expense-color` by sign); currency symbol (element currency ?? budget currency). Row fold toggle (children or mobile-category) driven by the persisted unfolded map; children rows show icon + name + `-spent`. Empty folder note `...structure.empty_folder.note`.
- Totals row: `...structure.total.name` ("Total") with `budgetTotals` values; mobile adds `...total.expenses`.
- Currency chips row (budget currencies from `balances[].currencyId`): toggling selects/deselects the widget currency (local state, not persisted).

- [x] **Step 1: failing tests** — sections render from a fixture (folder, default, archived), fold toggle persists and expands children, available color flips by sign, totals match `budgetTotals`, currency chip toggle mounts/unmounts the widget.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): budget table with fold state and totals`.

---

### Task 7: Set-limit — inline editor + mobile modal (TDD)

**Files:** Create `web-react/src/features/budgets/LimitEditor.tsx`, `SetLimitDialog.tsx`; extend the row; tests alongside.

- **Desktop inline:** the budget cell becomes a click-to-edit popover (shadcn Popover + `CalculatorInput`) when `canUpdateLimits`; Enter/blur commits: `''`/`0`/NaN → `amount: null`, else `normalizeNumber(evaluated)` string. Validation messages `elements.validation.{invalid_number,invalid_decimal_number,invalid_formula}`.
- **Mobile:** long-press on the row (a small `useLongPress` hook, ~500ms) opens `SetLimitDialog` (`modules.budget.modal.set_limit_form.header` "Set limit", label `modules.budget.form.budget_limit.limit.label`, `CalculatorInput`) — same unified amount rule (approved divergence).
- Both call `useSetLimit` with `period = selectedDate`; the optimistic patch makes the cell update instantly.

- [x] **Step 1: failing tests** — commit posts the normalized string; `5+5=` evaluates; clearing posts null; a 400 rolls the cell back; long-press opens the dialog (fire pointer events with fake timers).
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): set-limit inline editor and mobile dialog`.

---

### Task 8: Edit mode — folder CRUD, envelope CRUD, drag move (TDD)

**Files:** Create `web-react/src/features/budgets/EnvelopeDialog.tsx` (create+update, shared form: name, `CurrencySelect`, categories multiselect of non-archived EXPENSE categories (`modules.budget.form.budget_envelope.categories.{label,selected}`), icon grid, status select on edit (`...status.option.{active,archive}`)); extend `BudgetTable` with edit mode; tests alongside.

- Edit-mode toggle lives in the page toolbar (Task 10); when on: folder headers gain `[+ envelope]` and a `⋮` menu (Edit → `PromptDialog` rename `...modal.update_folder_form.header`; Up/Down → `orderBudgetFolders` position swap; Delete → enabled only for empty folders → `deleteBudgetFolder`); a "Create a folder" button (`...structure.action.create_folder`) opens `PromptDialog` (`...modal.create_folder_form.header`, folder-name validation keys `modules.budget.form.budget.folder_name.validation.*`).
- Element rows gain drag handles: dnd-kit cross-container drag (same pattern as the accounts settings page) → `moveElements({budgetId, items})` with the moved element's `{id, folderId, position}` (+ renumbered siblings). Element `⋮` menu: **Change currency** (categories/tags only, `...structure.element.action.change_currency`), **Edit**/**Delete** (envelopes only; delete → ConfirmDialog `...modal.delete_envelope.{header,question}`, hidden unless owner/admin).
- Envelope create posts `{budgetId, id: v7(), name, icon, currencyId, folderId, categories}`; update posts `isArchived` from the status select. Validation: envelope name keys `modules.budget.form.budget_envelope.name.validation.*`.

- [x] **Step 1: failing tests** — folder create/rename/delete payloads (delete blocked on non-empty), up/down order payload, envelope create/update/delete payloads (categories replace, status → isArchived), change-currency payload, move via a directly-driven reorder callback.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): budget edit mode with envelope and folder management`.

---

### Task 9: Budget update dialog + transaction list dialog (TDD)

**Files:** Create `web-react/src/features/budgets/BudgetUpdateDialog.tsx` (generalize the Plan-3 `BudgetDialog` form: initial values from `meta` + `filters.excludedAccountsIds`, name disabled unless `canConfigureBudget`, header `modules.budget.modal.update_budget_form.header`) and `web-react/src/features/budgets/BudgetTransactionsDialog.tsx` (row click in read mode → per-element transactions for the period: `getBudgetTransactions({budgetId, periodStart: selectedDate, categoryId|tagId|envelopeId by element type})`, day-grouped read-only list reusing the Plan-2 grouping helpers); tests alongside.

- [x] **Step 1: failing tests** — update dialog seeds and posts `{id, name, currencyId, excludedAccounts}` and invalidates budget+budgets; transactions dialog passes the right id param per element type and renders grouped rows.
- [x] **Step 2: implement.** **Step 3:** PASS.
- [x] **Step 4: commit** `feat(web-react): budget update and element transactions dialogs`.

---

### Task 10: Page assembly, onboarding empty state, routes (TDD)

**Files:** Create `web-react/src/features/budgets/BudgetPage.tsx`, `BudgetEmptyState.tsx`; rewrite `web-react/src/features/home/HomePage.tsx`; update `routes.tsx` (`/budget` → BudgetPage); tests alongside.

- **Toolbar:** budget name; currency chips; desktop "Configure" menu (`...page.budget.settings.button`): Budget details → update dialog, Edit structure (gated `canConfigureBudget`) → edit mode (menu shows "Done Editing" toggle), Open budget list → `/settings/budgets`. Mobile: back → `/`, gear → the same actions (ResponsiveDialog menu), check icon exits edit mode.
- **Empty state** (`isLoaded && !budget`): `...page.budget.empty.{header,no_budget,description,create_budget}`; when the user has no accounts or no categories, show `...empty.initial_setup` + `...empty.create_account` (opens the account dialog) — Vue's `BudgetOnboarding`. Create budget opens the Plan-3 create dialog.
- **HomePage:** onboarded → `<BudgetPage/>`; not onboarded → keep the placeholder (Plan 5 swaps in Onboarding).
- Assemble: PeriodStrip (hidden without a budget meta), widget, table, edit-mode create-folder button, all dialogs.

- [x] **Step 1: failing tests** — full-page MSW fixture renders strip + table + totals; empty state shows the right copy with/without accounts; configure menu toggles edit mode; `/` renders the budget for an onboarded fixture user.
- [x] **Step 2: implement.** **Step 3:** PASS + full suite + build + lint.
- [x] **Step 4: commit** `feat(web-react): budget page assembly with onboarding empty state`.

---

### Task 11: Budget parity check (manual, gate for Plan 4)

- [x] **Step 1: run** the stack (inline env, scratch DB with the Plan-3 data: user, accounts, categories, budget) + the Vue reference at `:8181`.
- [x] **Step 2: walk in BOTH apps at 1280px / 375px:**

1. `/` and `/budget` land on the budget; month strip centered on the current month, months before `startedAt` dimmed; switching periods refetches (check a future month: widget balances null-safe).
2. Category/tag elements appear with spent from real transactions; set a limit inline (formula `100+50=`), clear it (empty → null on the wire — verify no zero-limit row is left via the Vue app), long-press on mobile.
3. Currency chips toggle the expense widget; cross-currency element math matches Vue (budgeted/available exchanged, spent not).
4. Edit structure: create folder (position 0), rename, up/down, create envelope with categories (appears at position 0 with children), drag an element between folders (persists in both apps), archive an envelope via status (moves to the Archived section), delete an empty folder + an envelope (owner).
5. Change a category's currency; totals re-exchange.
6. Row click (read mode) shows the element's period transactions.
7. Budget details dialog: rename + exclude an account → balances change.
8. Empty state: fresh user with no default budget sees the no-budget copy; with no accounts sees initial-setup + create-account.
9. Fold state persists across reload; switching budgets resets it.

- [x] **Step 3: record:**

```bash
git commit --allow-empty -m "chore(web-react): budget parity check vs Vue app passed (desktop/mobile)"
```

---

## Plan sequence

This is Plan 4 of 6 (spec phase 5). Remaining: **Plan 5** — connections + onboarding + CSV import/export + deferred access-control (accounts + budgets: grant/revoke/accept/decline UI, exclude/include-account) + the `web/` swap commit (Makefile, Dockerfile, delete the Vue app).
