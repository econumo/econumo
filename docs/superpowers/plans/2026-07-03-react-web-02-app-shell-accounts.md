# React Web Migration — Plan 2 of 6: App Shell + Accounts

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the application shell (sidebar with the account/folder tree, responsive one-pane mobile behavior, loading gate, sync) and the complete accounts feature: account page with the day-grouped transaction list, account create/edit modal, transaction create/edit modal (all three types), view/delete transaction flows — at behavior parity with the Vue app against the same Go backend.

**Architecture:** Extends the Plan-1 foundation in `web-react/`. All server data moves to TanStack Query (this fully replaces the Vue Pinia stores + `sync.ts` orchestration + localStorage persistence of server data). Feature folders: `features/accounts`, `features/transactions`, plus data-layer-only `features/{categories,payees,tags,currencies}` (their pages arrive in Plan 4). New shared libs: money formatting, safe calculator, datetime helpers, currency exchange.

**Tech stack additions:** none beyond Plan 1 except `zustand` (one tiny UI-state store) — dnd-kit is NOT needed yet (see scope note).

## Scope notes (deviations from the spec's phase list — decided against the actual Vue code)

1. **Drag-reorder moves to Plan 4.** The spec listed "drag-reorder" under this phase, but in the Vue app the account/folder reorder UI lives exclusively in `pages/Settings/Accounts.vue` (vuedraggable, handle `.sortable-control`) — the sidebar tree is read-only. This plan ships the **data layer** for reordering (`useOrderAccounts`, `useOrderFolders` mutations against `order-account-list` / `order-folder-list`); the dnd-kit UI ships with the Settings→Accounts page in Plan 4 where it belongs.
2. **Home page `/` is a placeholder.** Vue's `Home.vue` renders Budget (onboarded) or Onboarding. Both pages are later plans; until then `/` renders the shell with an empty workspace (desktop) / the sidebar pane (mobile).
3. **CSV import/export stays in Plan 6** (spec already says so).

## Approved divergences introduced by this plan (beyond Plan 1's)

- **"All accounts" renders translated.** Vue's sidebar stores the raw i18n key string `'elements.folders.default_folder'` as the synthetic folder name and renders it untranslated (a live bug). React renders `t('elements.folders.default_folder')` = "All accounts".
- **Sidebar folder collapse state persists.** Vue persists `accountFoldersOpened` but the sidebar's tree builder overwrites `opened: true`, so folders always render open (a live bug — the persisted state is only honored in Settings). React honors the persisted open/closed state in the sidebar (default open).
- **No `eval()`.** Vue's calculator validates/evaluates formulas with raw `eval`. React ships a small tokenizer + shunting-yard evaluator with identical observable results for valid inputs.
- **Server data is not persisted to localStorage.** Vue mirrors every store into localStorage (`store/*` keys) as an offline-ish cache. React keeps server state in TanStack Query memory only; a full reload refetches. (localStorage keeps only: token, `selfHosted`, `backendHost`, `locale`, and the new `sidebarFolders` open-state map.)
- Everything else from Plan 1 still applies (UUIDv7, `X-Request-Id`, token in localStorage, auth-expiry redirect).

## Global constraints (wire contract — verified against the Go source)

- **Envelope:** every endpoint in this plan wraps success as `{"success":true,"message":"","data":<payload>}`. The API layer unwraps `response.data.data` exactly once (login-user was the only bare exception, handled in Plan 1). Validation failure → 400 `{"success":false,"message":"Form validation error","code":400,"errors":{field:[msgs]}}`. Access denied → 403 with `errors: []` (array). Domain not-found → 400, not 404.
- **Money on the wire:** request `amount`/`amountRecipient`/`balance` may be sent as JSON number or string (backend accepts both — send numbers like Vue does). Responses ALWAYS return them as **decimal strings** (`"balance":"100.5"`, `"amount":"9.99"`). The API layer coerces them to `number` (Vue does `Number(...)`; max 8 fraction digits so float is safe here).
- **Datetimes:** `"2006-01-02 15:04:05"` — transaction `date`, `createdAt`/`updatedAt` everywhere. `X-Timezone` header (already sent by the Plan-1 axios client) drives the account-balance day boundary server-side.
- **Int-typed flags:** folder `isVisible` is `0|1`; category/payee/tag `isArchived` is `0|1`. Transaction/category `type` are alias strings (`"expense"|"income"|"transfer"`).
- **Create `id` is an idempotency/operation key, not the entity id.** For `create-account` and `create-transaction` the backend mints a fresh UUIDv7 for the entity and uses the request `id` only as an operation lock (duplicate → error message `"Operation is locked"`). Always take ids from `response.item`, never assume the sent id. `update-*` requests DO address entities by `id`.
- **`create-account`/`update-account` respond `{item, transaction}`** where `transaction` is the opening-balance/correction transaction (or `null`). `create/update/delete-transaction` respond `{item, accounts}` where `accounts` is the full refreshed account list — merge it into the accounts cache instead of refetching.
- **First-account folder auto-creation:** a user with zero folders may create an account with blank `folderId` (backend auto-creates a folder named "General"); once any folder exists, blank `folderId` → validation error. After a create that returns while the folder list was empty, invalidate the folders query.
- **`get-account-list` returns items reversed relative to `order-account-list`'s response** — always sort client-side by `position`, never trust response order. The multi-account transaction-list query has NO server ordering — always sort client-side by `date` desc.
- **`get-transaction-list`** accepts optional query params `accountId`, `periodStart`, `periodEnd` — this plan fetches the full list with no params (exactly like Vue) and filters client-side.
- **Update echo asymmetry:** `update-category`/`update-payee` return `{}` (no item); `update-tag` returns `{item}`. `create-*` all return `{item}`.
- **Entity icons are Material icon names** (`account.icon`, `category.icon` — e.g. `"account_balance"`, `"question_mark"`) stored in the production DB. The React app must render them. Decision: load the **Material Symbols/Icons web font** for entity icons only; lucide-react remains the toolkit for UI chrome. Never remap stored names.
- Validation messages shown must be the exact catalog strings (all keys verified to exist in the ported `en-US.ts` — see per-task lists).
- Same rules as Plan 1: pnpm, commit per task (`feat(web-react): …` + trailers), all UI copy through `t()`, vendored `components/ui/*` untouched, tests-first for logic.

---

### Task 1: `lib/datetime.ts` — datetime helpers (TDD)

**Files:** Create `web-react/src/lib/datetime.ts`, test `web-react/src/lib/datetime.test.ts`.

**Interfaces (mirrors the Quasar `date` usage):**
- `DATE_TIME_FORMAT`-shaped helpers, no date library:
  - `formatDateTime(d: Date): string` → `"2026-07-03 14:05:09"` (local wall clock, zero-padded).
  - `parseDateTime(s: string): Date` — parses `"YYYY-MM-DD HH:mm:ss"` (also accepts bare `"YYYY-MM-DD"`, time = 00:00:00) as LOCAL time.
  - `formatDate(d: Date): string` → `"2026-07-03"`.
  - `dayKey(s: string): string` — datetime string → its `"YYYY-MM-DD"` day.
  - `formatDayHeading(day: string): string` → `"3rd July 2026"` (Vue uses `Do MMMM YYYY`; implement English ordinal — 1st/2nd/3rd/4th…11th/12th/13th→th — plus full month name via `Intl.DateTimeFormat('en',{month:'long'})`).
  - `isToday(day: string): boolean`, `isYesterday(day: string): boolean`, `isFuture(dateTime: string): boolean` (>= tomorrow 00:00 local — Vue marks rows `isInFuture`).

- [ ] **Step 1: failing tests** covering: round-trip format/parse, zero padding, day extraction, ordinals (1st, 2nd, 3rd, 11th, 21st, 22nd, 23rd), today/yesterday against a mocked `Date` (use `vi.setSystemTime`), future detection at the day boundary.
- [ ] **Step 2: implement** (pure functions, no deps).
- [ ] **Step 3:** `pnpm vitest run src/lib/datetime.test.ts` → PASS.
- [ ] **Step 4: commit** `feat(web-react): datetime helpers for the frozen Y-m-d H:i:s contract`.

---

### Task 2: `lib/money.ts` — decimal + money formatting (TDD)

**Files:** Create `web-react/src/lib/money.ts`, test `web-react/src/lib/money.test.ts`.

Port of `web/src/composables/useDecimalNumber.ts` + `useMoney.ts`, decoupled from stores — the currency is passed in:

```ts
export interface CurrencyLike { symbol: string; fractionDigits: number }

export function normalizeNumber(amount: number | string): string
export function formatNumber(amount: number | string, digits: number, useFixedPrecision: boolean): string
export function addThousandSeparators(formatted: string): string   // integer part only, comma, regex \B(?=(\d{3})+(?!\d))
export function moneyFormat(
  amount: number | string,
  currency?: CurrencyLike,
  opts?: { showCurrency?: boolean; useNativePrecision?: boolean; useThousandSeparator?: boolean },
): string
```

Behavior parity (source of truth `useDecimalNumber.ts`/`useMoney.ts`):
- Defaults: `showCurrency=true, useNativePrecision=true, useThousandSeparator=true`.
- `normalizeNumber`: null/undefined/non-finite → `'0'`; `|n| < 0.000001` → `toFixed(8)` with trailing zeros stripped; else ≤8 decimals, trailing zeros stripped; returns string.
- Digits: native precision → `currency?.fractionDigits ?? 8`; non-native → integers use `fractionDigits ?? 0`, non-integers use `max(fractionDigits ?? 0, min(actualDecimals, 8))`.
- Symbol **appended** with a leading space (`"1,234.50 $"`); no symbol when `showCurrency=false` or no currency.
- Negative numbers keep their `-` inside the number (no parentheses); `+`/`-` signs for transaction rows are prepended by the caller, not here.
- Decimal separator `.`, thousands `,` — hard-coded (not locale-aware), same as Vue.

- [ ] **Step 1: failing tests** — normalize (0, tiny values, trailing zeros, strings), digits matrix (native/non-native × integer/decimal × with/without currency), thousand separators, symbol placement, negative passthrough, edge `"0"` balance string from the wire.
- [ ] **Step 2: implement.** **Step 3: run** → PASS.
- [ ] **Step 4: commit** `feat(web-react): money and decimal formatting helpers`.

---

### Task 3: `lib/calculator.ts` — safe formula engine (TDD) + deferred validators

**Files:** Create `web-react/src/lib/calculator.ts`, test `web-react/src/lib/calculator.test.ts`; modify `web-react/src/lib/validation.ts` (+ its test).

Port of `web/src/modules/helpers/calculator.ts` **without `eval`**:

```ts
export function sanitizeInput(value: string | number): string
export function validateFormula(formula: string | number): boolean
export function evaluateFormula(value: string | number): string
```

- `sanitizeInput`: strip all but `0-9 + - * / = . ,`; `,`→`.`; collapse runs of 2+ of `+ * / .` not preceded by a digit; strip leading zeros from non-decimal number parts. Parity examples (assert all): `"5+3*2"→"5+3*2"`, `"007"→"7"`, `"5,5+2"→"5.5+2"`, `"5abc+2"→"5+2"`, `"5++2"→"5+2"`.
- `validateFormula`: `''`→true; strip `=`; tokenize + parse (numbers with optional decimals, binary `+ - * /`, unary minus at the start); malformed (trailing operator, double operator, empty) → false. `"5+3*2"`→true, `"5+"`→false.
- `evaluateFormula`: only acts when the value contains `=`: strip `=`, evaluate with standard precedence (`* /` before `+ -`, left-assoc), round to 10 decimal places (`roundToPrecision(n,10)`), return as string; invalid → the value with `=` removed. No `=` → value returned unchanged. Parity: `"5+3*2="→"11"`, `"1/3="→"0.3333333333"`, `"5+3*2"→"5+3*2"`.
- Implementation: shunting-yard or recursive descent over tokens — ~60 lines, no dependency.

Also move the two Plan-1-deferred validators into `lib/validation.ts`:
```ts
export function isValidFormula(value: string): boolean      // validateFormula(sanitizeInput(value))
export function hasIncompleteFormula(value: string): boolean // /[+\-*/]$/.test(value)
```

- [ ] **Step 1: failing tests** (the parity table above + operator precedence, unary minus `-5+2`, division rounding, garbage input).
- [ ] **Step 2: implement.** **Step 3: run** calculator + validation tests → PASS.
- [ ] **Step 4: commit** `feat(web-react): safe calculator engine replacing the Vue eval-based one`.

---

### Task 4: API DTOs + modules — account, folder, transaction, category, payee, tag, currency (TDD)

**Files:**
- Create: `web-react/src/api/dto/{account,folder,transaction,category,payee,tag,currency}.ts`
- Create: `web-react/src/api/{account,transaction,category,payee,tag,currency}.ts` (folders live in `account.ts`, mirroring the backend's `/api/v1/account/...-folder` paths)
- Test: `web-react/src/api/account.test.ts`, `web-react/src/api/transaction.test.ts` (cover the tricky unwrap/coercion paths; category/payee/tag/currency are same-shaped and get one smoke test each inside those two files or a third)

**DTO shapes (exact wire fields; coercions noted):**

```ts
// dto/currency.ts
export interface CurrencyDto { id: Id; code: string; name: string; symbol: string; fractionDigits: number }
export interface CurrencyRateDto { currencyId: Id; baseCurrencyId: Id; rate: number /* wire: string */; updatedAt: string }

// dto/folder.ts
export interface FolderDto { id: Id; name: string; position: number; isVisible: 0 | 1 }

// dto/account.ts
export const AccountType = { CASH: 1, CREDIT_CARD: 2 } as const
export type AccountType = (typeof AccountType)[keyof typeof AccountType]
export interface AccountAccessDto { user: UserDto; role: 'admin' | 'user' | 'guest' }
export interface AccountDto {
  id: Id; owner: UserDto; folderId: Id | null; name: string; position: number
  currency: CurrencyDto; balance: number /* wire: decimal string */
  type: AccountType; icon: string; sharedAccess: AccountAccessDto[]
}
export interface AccountItemDto { item: AccountDto; transaction: TransactionDto | null }

// dto/transaction.ts
export type TransactionType = 'expense' | 'income' | 'transfer'
export interface CreateTransactionDto {
  id: Id; type: TransactionType; accountId: Id; accountRecipientId: Id | null
  amount: number; amountRecipient: number | null
  categoryId: Id | null; description: string; payeeId: Id | null; tagId: Id | null; date: string
}
export interface TransactionDto extends CreateTransactionDto { author: UserDto }
export interface TransactionItemDto { item: TransactionDto; accounts: AccountDto[] }

// dto/category.ts
export interface CategoryDto {
  id: Id; ownerUserId: Id; name: string; position: number
  type: 'expense' | 'income'; icon: string; isArchived: 0 | 1; createdAt: string; updatedAt: string
}
// dto/payee.ts + dto/tag.ts — identical minus type/icon
export interface PayeeDto { id: Id; ownerUserId: Id; name: string; position: number; isArchived: 0 | 1; createdAt: string; updatedAt: string }
```

**API functions** (all unwrap `.data.data`; coerce decimal strings with `Number(...)`):

```ts
// api/account.ts
getAccountList(): Promise<AccountDto[]>                    // GET  /api/v1/account/get-account-list → data.items, coerce balance
createAccount(form: { id: Id; name: string; currencyId: Id; balance: number; icon: string; folderId: Id | null }): Promise<AccountItemDto>
updateAccount(form: { id: Id; name: string; balance: number; icon: string; currencyId: Id; updatedAt: string }): Promise<AccountItemDto>
deleteAccount(id: Id): Promise<void>
orderAccountList(changes: { id: Id; folderId: Id | null; position: number }[]): Promise<AccountDto[]>
getFolderList(): Promise<FolderDto[]>                      // GET /api/v1/account/get-folder-list
createFolder(name: string): Promise<FolderDto>             // → data.item
updateFolder(id: Id, name: string): Promise<FolderDto>
replaceFolder(id: Id, replaceId: Id): Promise<void>
hideFolder(id: Id): Promise<void>; showFolder(id: Id): Promise<void>
orderFolderList(changes: { id: Id; position: number }[]): Promise<FolderDto[]>

// api/transaction.ts
getTransactionList(): Promise<TransactionDto[]>            // GET, no params; coerce amount/amountRecipient
createTransaction(form: CreateTransactionDto): Promise<TransactionItemDto>
updateTransaction(form: CreateTransactionDto): Promise<TransactionItemDto>   // update sends the same field set; id addresses the tx
deleteTransaction(id: Id): Promise<TransactionItemDto>     // also returns {item, accounts}

// api/category.ts | payee.ts | tag.ts (same skeleton)
getCategoryList(): Promise<CategoryDto[]>
createCategory(form: { id: Id; name: string; type: 'expense' | 'income'; accountId?: Id; icon?: string }): Promise<CategoryDto>
createPayee(form: { id: Id; name: string; accountId?: Id }): Promise<PayeeDto>
createTag(form: { id: Id; name: string; accountId?: Id }): Promise<TagDto>
// (update/archive/delete/order functions included for completeness — Plan 4 wires their UI)

// api/currency.ts
getCurrencyList(): Promise<CurrencyDto[]>
getCurrencyRateList(): Promise<CurrencyRateDto[]>          // coerce rate
```

- [ ] **Step 1: failing tests** with MSW fixtures using EXACT wire shapes: balance/amount as strings (`"balance":"100.5"`, `"amount":"9.99"`, `"rate":"1.08"`), `isVisible: 1`, `isArchived: 0`, envelope wrapping, `{item, transaction:null}`, `{item, accounts:[...]}`. Assert numeric coercion and id passthrough.
- [ ] **Step 2: implement.** **Step 3: run** → PASS. `pnpm build` → clean.
- [ ] **Step 4: commit** `feat(web-react): account, transaction, classification and currency API modules`.

---

### Task 5: Query layer — hooks, keys, invalidation map (TDD)

**Files:**
- Create: `web-react/src/features/accounts/queries.ts`, `web-react/src/features/transactions/queries.ts`, `web-react/src/features/classifications/queries.ts` (categories+payees+tags), `web-react/src/features/currencies/queries.ts`, `web-react/src/features/user/queries.ts`
- Create: `web-react/src/app/queryKeys.ts`
- Test: `web-react/src/features/accounts/queries.test.tsx`, `web-react/src/features/transactions/queries.test.tsx`

**Query keys (one convention):** `['accounts']`, `['folders']`, `['transactions']`, `['categories']`, `['payees']`, `['tags']`, `['currencies']`, `['currencyRates']`, `['user']`.

**Queries** (staleTime mirrors Vue's `fetchUpdates` windows): accounts/folders/transactions/categories/payees/tags/user → `staleTime: 10 * 60_000`; currencies/currencyRates → `staleTime: 24 * 3600_000`. `refetchOnWindowFocus: true` globally (replaces the Vue `visibilitychange` → `fetchUpdates`). Sorting is done in selectors: `useAccounts()` returns position-sorted, `useTransactions()` date-desc-sorted, etc.

**Mutations + cache effects (the invalidation map — replaces Vue store side effects 1:1):**

| Mutation | Endpoint | On success |
|---|---|---|
| `useCreateAccount` | create-account | upsert `item` into `['accounts']`; if `transaction` → prepend to `['transactions']` + invalidate `['budget'…]` (no-op until Plan 5); if the folders cache was empty → invalidate `['folders']` |
| `useUpdateAccount` | update-account | upsert `item`; if `transaction` → prepend + budget invalidate |
| `useDeleteAccount` | delete-account | remove from `['accounts']`; drop its transactions from `['transactions']` |
| `useOrderAccounts` | order-account-list | replace `['accounts']` with response items |
| `useCreateFolder` / `useUpdateFolder` / `useOrderFolders` | …-folder | upsert/replace `['folders']` |
| `useReplaceFolder` / `useHideFolder` / `useShowFolder` | …-folder | invalidate `['folders']` + `['accounts']` |
| `useCreateTransaction` | create-transaction | set `['accounts']` = response `accounts`; prepend `item` to `['transactions']` |
| `useUpdateTransaction` | update-transaction | set `['accounts']`; replace item in `['transactions']` |
| `useDeleteTransaction` | delete-transaction | set `['accounts']`; remove from `['transactions']` |
| `useCreateCategory` / `useCreatePayee` / `useCreateTag` | create-* | append `item` to its list cache |

Client-generated ids: every `create*` call sends `id: v7()` generated at call time (the caller passes it or the hook generates it — generate in the hook, return the response item). Remember: response `item.id` is authoritative.

- [ ] **Step 1: failing tests**: create-transaction updates BOTH caches from `{item, accounts}` (assert the accounts cache equals the response accounts, coerced); create-account with `transaction` non-null prepends to transactions; create-account while `['folders']` cache is `[]` triggers a folders refetch (spy via MSW hit count); delete-transaction removes and updates accounts.
- [ ] **Step 2: implement.** **Step 3: run** → PASS.
- [ ] **Step 4: commit** `feat(web-react): TanStack Query data layer with the Vue store invalidation semantics`.

---

### Task 6: `lib/exchange.ts` + `useExchange` — currency conversion (TDD)

**Files:** Create `web-react/src/lib/exchange.ts`, `web-react/src/features/currencies/useExchange.ts`; test `web-react/src/lib/exchange.test.ts`.

Port of `composables/useCurrency.ts`: `exchange(fromCurrencyId, toCurrencyId, amount, rates: CurrencyRateDto[]): number` — same-currency passthrough; convert via base-currency rates (`amount / rate(from) * rate(to)` per the Vue implementation — port the exact arithmetic from `useCurrency.ts`, verify against it while implementing); missing rate → passthrough (Vue behavior). `useExchange()` binds it to the `['currencyRates']` query. Used by: sidebar folder totals, transfer recipient-amount prefill.

- [ ] **Step 1: failing tests** (same currency, through-base conversion both directions, missing rate fallback). **Step 2: implement.** **Step 3: run** → PASS.
- [ ] **Step 4: commit** `feat(web-react): currency exchange helper`.

---

### Task 7: Material entity icons + `EntityIcon` component

**Files:** Create `web-react/src/components/EntityIcon.tsx`; modify `web-react/index.html` (font link) or vendor the font into `public/fonts/` + `src/index.css` `@font-face`.

Stored `account.icon` / `category.icon` values are **Material icon ligature names** (production data — frozen). Vendor the Material Symbols Outlined woff2 locally (self-hosted app must not depend on Google CDN): download once into `public/fonts/`, add `@font-face` + a `.material-icon` ligature class in `src/index.css` (font-feature ligatures on). `EntityIcon({ name, className })` renders `<span className="material-icon">{name}</span>`; empty/unknown name renders the `question_mark` glyph like Vue's fallbacks.

- [ ] **Step 1:** vendor font + CSS. **Step 2:** `EntityIcon` + a render test asserting the ligature text content. **Step 3:** `pnpm test`/`build` pass.
- [ ] **Step 4: commit** `feat(web-react): material entity icons for frozen account/category icon names`.

---

### Task 8: UI-state store + shell scaffolding (zustand)

**Files:** Create `web-react/src/app/uiStore.ts`; `pnpm add zustand`.

One small store (the design's "one small zustand store"):
- `sidebarFolderOpen: Record<Id, boolean>` (+ `toggleFolder(id)`) — persisted to localStorage key `sidebarFolders` via zustand `persist` (replaces Vue's `ACCOUNT_FOLDERS_OPENED`; default open).
- `transactionModal: { open: boolean; params: OpenTransactionParams | null }` + open/close actions (replaces Vue `transaction-modal` store — the modal is opened from both the account page and, later, other pages).
- `accountModal: { open: boolean; params: OpenAccountParams | null }` + actions.
- `switchAccountPrompt: Id | null` + actions (the post-transfer "Switch to" bottom prompt).

No tests needed beyond usage in later tasks (pure state).

- [ ] **Step 1:** implement + `pnpm build`. **Step 2: commit** `feat(web-react): ui-state store for modals and sidebar folders`.

---

### Task 9: Application shell — sidebar, loading gate, responsive panes (TDD)

**Files:**
- Rewrite: `web-react/src/app/layouts/ApplicationLayout.tsx`
- Create: `web-react/src/features/accounts/SidebarAccountTree.tsx`, `web-react/src/components/LoadingDialog.tsx`
- Copy: `web-react/src/assets/econumo-gray.svg` (from `../web/src/assets/`)
- Test: `web-react/src/app/layouts/ApplicationLayout.test.tsx`, `web-react/src/features/accounts/SidebarAccountTree.test.tsx`

**Structure (parity with `ApplicationLayout.vue`, which is a manual two-pane layout — NOT a Quasar drawer):**

- Desktop (`lg`+, ≥1024px): fixed sidebar (logo + edition badge, user block, nav, account tree, footer) + workspace (`<Outlet/>`).
- Mobile/tablet (<1024px): ONE pane at a time, derived from the route (replaces Vue's `activeArea` store): `/` shows the sidebar as the page; any other route shows the workspace (pages render their own back button → `navigate('/')`). Use the Plan-1 `useIsMobile` pattern generalized to a `useBreakpoint('lg')` hook if needed.
- **Loading gate:** until the core queries (accounts, folders, transactions, categories, payees, tags, currencies, currencyRates, user) all have data → render `LoadingDialog` (blocking, non-dismissible ResponsiveDialog wrapping a spinner) with `t('modules.app.modal.loading.data_loading')` = "Loading details". This is Vue's `isFullyLoaded` gate.
- **Header/user block:** avatar (`avatar + '?s=100'`), name, email — links to `/settings/profile`.
- **Nav:** Onboarding link (`blocks.main.onboarding`, only when the user's `onboarding` option ≠ `'completed'`), Budget link (`blocks.main.budget` → `/budget`).
- **Footer:** gray logo + edition badge (`econumoPackage().label`), sync button (invalidates ALL query keys → refetch; replaces `fetchAll`), Settings (`pages.settings.settings.menu_item` → `/settings`), website link (`blocks.website.label`, href `getWebsiteUrl()`, `target="_blank"`).

**SidebarAccountTree** (parity with the `accountsTree` computed):
- Groups position-sorted accounts into position-sorted VISIBLE folders (`isVisible === 1`); accounts with `folderId` not matching any visible folder OR `null` go to a synthetic trailing "All accounts" folder (`t('elements.folders.default_folder')` — approved fix).
- Folder row: name + per-folder total — if all its accounts share one currency: raw sum formatted in that currency (native precision); mixed: sum converted to the user's currency via `useExchange`. `moneyFormat(amount, currency)`.
- Collapsible (shadcn `Collapsible`), open state from `uiStore.sidebarFolderOpen` (default open), chevron.
- Account row: `EntityIcon(account.icon)`, truncated name (`title` attr), balance `moneyFormat(balance, currency)`; active when the route is `/account/:id`; click → `navigate(RouterPage.ACCOUNT(id))`. Shared badge: when `sharedAccess.length > 0`, a people glyph + stacked owner/user avatars (`?s=30`).
- Empty state (no accounts, folders exist or not): the folder header with `0` total and an "Add an account" row (`blocks.main.create_account`) → opens the account modal (`uiStore.accountModal`).
- Folders with zero accounts are omitted (Vue behavior).

**Home page:** create `web-react/src/features/home/HomePage.tsx` — desktop: empty workspace placeholder (`<div/>`; Budget lands here in Plan 5, onboarding branch in Plan 6); mobile: nothing (the shell already shows the sidebar pane at `/`). Swap into `routes.tsx` for `/`.

- [ ] **Step 1: failing tests**: (a) tree grouping — folders sorted, hidden folder excluded, synthetic "All accounts" holds folderless accounts and renders translated, empty folders dropped; (b) single-currency folder total vs mixed-currency converted total; (c) loading gate shows "Loading details" until MSW answers all core queries, then the tree renders; (d) mobile: at `/` sidebar visible + workspace hidden, at `/account/:id` reversed.
- [ ] **Step 2: implement.** **Step 3:** targeted tests + `pnpm test` + `pnpm build` → PASS.
- [ ] **Step 4: commit** `feat(web-react): application shell with sidebar account tree and loading gate`.

---

### Task 10: `CurrencySelect` component (TDD)

**Files:** Create `web-react/src/components/CurrencySelect.tsx`; test alongside.

Props: `{ value: Id | null; onChange: (id: Id) => void; disabled?: boolean }`. shadcn `Select` (or `Combobox` if the list warrants search — Vue uses a plain select) listing `useCurrencies()` sorted by code; option label `"USD — US Dollar"` style matching the Vue `CurrencySelect.vue` (check `web/src/components/CurrencySelect.vue` for the exact label format and mirror it).

- [ ] **Step 1: failing test** (options render from MSW currencies; change fires with the id). **Step 2: implement + verify against the Vue component's label format.** **Step 3: run** → PASS.
- [ ] **Step 4: commit** `feat(web-react): currency select`.

---

### Task 11: `CalculatorInput` component (TDD)

**Files:** Create `web-react/src/components/CalculatorInput.tsx`; test `web-react/src/components/CalculatorInput.test.tsx`.

Wraps shadcn `Input` (`inputMode="decimal"`, `pattern="[0-9+\-\*\.=,]*"`). Props: `{ value: string; onChange: (v: string) => void }` + passthrough props (id, placeholder, aria).

Behavior parity with `Calculator/CalculatorInput.vue`:
- On change: if the text ends with `=` → `sanitizeInput(text.slice(0,-1))`; if `validateFormula` passes → `onChange(evaluateFormula(sanitized + '='))`, else `onChange(raw)`.
- On **Enter**: if the sanitized value contains an operator and validates → `preventDefault()` (block form submit) + `onChange(evaluateFormula(sanitized + '='))`. Plain number → let the form submit.
- Mobile-only operator keypad (Vue's `CalculatorWidget`, hidden ≥1024px): a simple fixed row of `+ − × ÷ =` buttons rendered below the focused input on mobile (`useIsMobile`), mapping `×→*`, `÷→/`, `−→-`, appending via `sanitizeInput`, keeping focus on the input. The Vue draggable-floating-card behavior is simplified to a static row (same functions, simpler plumbing — allowed by "cleaner internals"); position persistence is dropped.

- [ ] **Step 1: failing tests**: typing `5+3*2` then `=` → value `11`; Enter with operators evaluates and does NOT submit the form; Enter on plain number submits; invalid trailing-operator input left as-is; keypad buttons append and evaluate on `=`.
- [ ] **Step 2: implement.** **Step 3: run** → PASS.
- [ ] **Step 4: commit** `feat(web-react): calculator amount input`.

---

### Task 12: Shared `ConfirmDialog` (TDD)

**Files:** Create `web-react/src/components/ConfirmDialog.tsx`; test alongside.

`{ open, onClose, onConfirm, title?, question, confirmLabel, cancelLabel }` on `ResponsiveDialog`. Default labels `t('elements.button.delete.label')` / `t('elements.button.cancel.label')` are passed by callers (keep the component copy-free). Used by: delete transaction (this plan), folder/account/category deletes (Plan 4).

- [ ] **Step 1: failing test** (renders question, confirm fires + closes, cancel closes). **Step 2: implement.** **Step 3: run** → PASS.
- [ ] **Step 4: commit** `feat(web-react): confirm dialog`.

---

### Task 13: Account modal — create/edit (TDD)

**Files:** Create `web-react/src/features/accounts/AccountDialog.tsx`; test `web-react/src/features/accounts/AccountDialog.test.tsx`; mount it in `ApplicationLayout` driven by `uiStore.accountModal`.

Parity with `components/AccountModal.vue` + `stores/account-modal.ts`:

- **Open params:** `{ account?: AccountDto; folderId?: Id | null }` — edit when `account` present.
- **Fields:** name (Input, maxLength 64), balance (`CalculatorInput`), currency (`CurrencySelect`), icon (grid picker of the same Material icon set the Vue `ResponsiveModalIcons` offers — copy the icon-name list from `web/src/components/ResponsiveModalIcons.vue` verbatim; render with `EntityIcon`; label `modals.account.form.icon.label` = "Icon").
- **Defaults:** create → balance `"0"`, currency = user's currency (from `['user']` options `currency_id`), icon = the Vue `defaultAccountIcon` (copy the constant), folderId from params. Edit → balance seeded `moneyFormat(balance, currency, {showCurrency:false, useNativePrecision:false, useThousandSeparator:false})`.
- **Validation:** name — `isNotEmpty` → `elements.validation.required_field`, `isValidAccountName` → `elements.form.account.name.validation.invalid_name`; balance — `isNotEmpty` → required_field, `isValidNumber` → `elements.validation.invalid_number`, `isValidDecimalNumber` → `elements.validation.invalid_decimal_number`, `isValidFormula` → `elements.validation.invalid_formula`.
- **Submit:** create → `useCreateAccount` with `{ id: v7(), name, balance: Number(evaluated), icon, folderId, currencyId }`; edit → `useUpdateAccount` with `{ id, name, balance, icon, currencyId, updatedAt: formatDateTime(new Date()) }`. Close on success. (The `{item, transaction}` cache handling is already in the Task-5 hooks.)
- **Headers/buttons:** `modals.account.create_form.header` "New account" / `modals.account.update_form.header` "Update account"; `elements.button.add.label` / `elements.button.update.label` / `elements.button.cancel.label`.

- [ ] **Step 1: failing tests**: create posts the exact payload (id UUIDv7, numeric balance) and the new account appears in the accounts cache; a non-zero balance response with `transaction` lands in the transactions cache; validation messages exact; edit seeds the unformatted balance and posts `updatedAt` in `Y-m-d H:i:s`.
- [ ] **Step 2: implement (check `ResponsiveModalIcons.vue` for the icon list + `defaultAccountIcon` constant first).**
- [ ] **Step 3: run** targeted + full tests → PASS.
- [ ] **Step 4: commit** `feat(web-react): account create/edit dialog`.

---

### Task 14: Transaction modal (TDD — the big one, built as split files)

**Files:**
- Create: `web-react/src/features/transactions/TransactionDialog.tsx` (shell: type toggle, date, submit)
- Create: `web-react/src/features/transactions/TransactionFormFields.tsx` (account/category/payee selects + tags row for expense/income), `web-react/src/features/transactions/TransferFields.tsx` (from/to/swap/recipient amount)
- Create: `web-react/src/features/transactions/AddTagDialog.tsx`
- Create: `web-react/src/features/transactions/useTransactionForm.ts` (state, defaults, derived options, submit — the logic hook, unit-testable)
- Test: `web-react/src/features/transactions/TransactionDialog.test.tsx`, `web-react/src/features/transactions/useTransactionForm.test.tsx`
- Mount in `ApplicationLayout` driven by `uiStore.transactionModal`.

**Parity spec (source: `TransactionModal.vue` + `stores/transaction-modal.ts`):**

- **Type toggle** order Income / Transfer / Expense (`modals.transaction.transaction_type.*`). Switching type clears the category, recomputes the transfer recipient amount, keeps everything else.
- **Defaults (create):** `id: v7()` at open; `date` = now `Y-m-d H:i:s`; `type` = param or `'expense'`; account = param `accountId` or the currently selected account (route account id); amounts null/empty. **Edit:** all fields from the transaction; amount seeded via `moneyFormat(amount, currency, {false,false,false})`.
- **Date field:** shadcn `Calendar` in a `Popover` (Monday first, today button), display `YYYY-MM-DD`; picking a date writes `YYYY-MM-DD 00:00:00`; create keeps the opening "now" time until a date is picked. A "previous day" quick button subtracts one day. No time UI.
- **Expense/Income fields:** account select (all accounts on edit; only accounts in **visible** folders on create; shows `EntityIcon` + balance), amount (`CalculatorInput` — rules: `isNotEmpty`→`elements.validation.required_field`, `isValidNumber`→`invalid_number`, `isValidDecimalNumber`→`invalid_decimal_number`, `isValidFormula`→`invalid_formula`), **category (required:** `modals.transaction.form.category.validation.required_field`; options filtered by current type + account owner, non-archived, position-sorted; create-on-the-fly allowed when the name is 3-64 chars and the user can change the account's data), payee (clearable; label `modals.transaction.form.payee.expense` "Recipient" / `.income` "Sender"; create-on-the-fly), tags row **expense only** (non-archived owner tags as toggle pills + an add-tag button opening `AddTagDialog` → `useCreateTag`), description (`modals.transaction.form.description.*`, textarea, no validation) under an "Advanced" section header (`modals.transaction.form.options.header`).
- **Transfer fields:** amount (CalculatorInput), From (`modals.transaction.form.from.label`; disabled when editing) + To selects (each disables the other's selection; swap button swaps them), recipient amount — plain Input, shown ONLY when both accounts set and currencies differ (label `modals.transaction.form.amount_recipient.label` "Will be exchanged"; rules: required/number/decimal — no formula), prefilled via `exchange(from.currency, to.currency, amount)` whenever amount/accounts change during creation. No category/payee/tags.
- **Create-on-the-fly gating:** `canChangeAccountData` = account owner is me OR my `sharedAccess` role is `admin`. Category/payee/tag create posts `{id: v7(), name, accountId}` (+ `type` for category) and selects the created item; dedupe by lowercased name against the cache first (Vue behavior).
- **Submit:** build `{ id, type, amount: Number(evaluated), amountRecipient, accountId, accountRecipientId, categoryId, description: description || '', payeeId, tagId, date }`; transfer sends recipient fields + null category/payee/tag; non-transfer the reverse. Create → `useCreateTransaction`; edit → `useUpdateTransaction`. On successful **transfer create** → `uiStore.switchAccountPrompt = accountRecipientId` (see Task 15). Close on success.
- **Headers/buttons:** `modals.transaction.create_form.header` "Add transaction" / `.update_form.header` "Update transaction"; Add/Update/Cancel buttons.
- The Vue hand-rolled q-select typing quirks (typing replaces selection, blur restores) are covered by shadcn `Combobox` defaults — match the *outcomes* (filter as you type, restore selection on blur, create option visible when no match), not the implementation.

- [ ] **Step 1: failing hook tests** (`useTransactionForm`): defaults on create (uuid v7 id, now-date, expense, selected account), type switch clears category, transfer recipient-amount computed only on create and only cross-currency, payload building for all three types (nulls in the right places).
- [ ] **Step 2: failing dialog tests**: expense create happy path posts the exact payload and MSW's `{item, accounts}` updates the accounts cache; category required message; transfer with different currencies shows the recipient amount prefilled (assert `exchange` applied); tags row present on expense, absent on income; create-on-fly category appears and is selected.
- [ ] **Step 3: implement** (hook → shell → fields → transfer → add-tag).
- [ ] **Step 4: run** targeted + `pnpm test` + `pnpm build` → PASS.
- [ ] **Step 5: commit** `feat(web-react): transaction dialog for expense, income and transfer`.

---

### Task 15: Switch-account prompt (post-transfer)

**Files:** Create `web-react/src/features/accounts/SwitchAccountPrompt.tsx`; mount in `ApplicationLayout`; test alongside.

Parity with the layout's bottom `q-dialog`: after a transfer create, a bottom bar/sheet appears — `t('elements.switch_to_account')` ("Switch to") + the recipient account name; clicking navigates to `/account/:id` and clears the prompt; a close button clears it. Render as a fixed bottom bar (shadcn-styled), non-modal.

- [ ] **Step 1: failing test** (renders name, click navigates + clears). **Step 2: implement.** **Step 3: run** → PASS.
- [ ] **Step 4: commit** `feat(web-react): switch-to-account prompt after transfers`.

---

### Task 16: Account page — transaction list (TDD)

**Files:**
- Create: `web-react/src/features/accounts/AccountPage.tsx`, `web-react/src/features/transactions/TransactionList.tsx`, `web-react/src/features/transactions/TransactionRow.tsx`, `web-react/src/features/transactions/ViewTransactionDialog.tsx`, `web-react/src/features/transactions/useAccountTransactions.ts`
- Modify: `web-react/src/app/routes.tsx` (`/account/:id` → `AccountPage`)
- Test: `web-react/src/features/accounts/AccountPage.test.tsx`, `web-react/src/features/transactions/useAccountTransactions.test.ts`

**Parity spec (source: `pages/Account.vue` + `mixins/transactionMixin.js` + `ViewTransactionModal.vue`):**

- **Resolution:** account = accounts query item matching `useParams().id`; render nothing (or the loading gate) until found.
- **Header:** `EntityIcon` + truncated name; balance `moneyFormat(balance, currency, {showCurrency:true, useNativePrecision:false})`; shared avatars when `sharedAccess.length > 0` (owner + users, `?s=30`); mobile back button → `/`. Actions: settings/edit (visible when I'm owner or role `admin`) → account dialog; add transaction (owner or role `admin`/`user`) → transaction dialog `{type:'expense'}`; search input (`pages.account.toolbar.search`).
- **`useAccountTransactions(accountId, search)`:** filter the `['transactions']` cache to `accountId === id || accountRecipientId === id`; search filters by a lowercased haystack of amount, author name (`@name`), category/payee/tag names, description, sign, type, date (the Vue `search` string — build it on the fly, no precompute needed); order date desc; group into day buckets emitting separators `{day, label}` where label = `t('pages.account.transaction_list.today')` / `.yesterday` / `formatDayHeading(day)`; rows dated ≥ tomorrow get an `isInFuture` flag (dimmed styling).
- **Row (`TransactionRow`)**: 
  - Title logic (exact port): transfer → `t('pages.account.transaction_list.item.transfer_from'|'transfer_to', {account})` (incoming when `accountId !== pageAccountId`; counterparty name falls back to `t('elements.account.name_hidden')`); otherwise first of category name → description → tag name → payee name; remember which source was used and suppress that field from the secondary line (description/tag badge/payee text render only when not the title source).
  - Icon: transfer → a transfer glyph (lucide `ArrowLeftRight` is the chrome exception here — acceptable, or Material `sync_alt` via EntityIcon for exactness: use `EntityIcon('sync_alt')`); else `EntityIcon(category?.icon || 'question_mark')`.
  - Amount: transfer incoming → `'+' + moneyFormat(amountRecipient, recipientAccount.currency, {false,false})`, transfer outgoing → `'-' + moneyFormat(amount, account.currency, {false,false})`; expense → `'-'…`, income → `'+'…`. Color: income-green / expense-red classes. Currency symbol rendered as a separate muted span using the PAGE account's `currency.symbol` (Vue quirk — keep it).
  - Author avatar on shared accounts.
- **Interactions:** desktop row click → inline menu (Edit / Delete — `elements.button.edit.label`/`delete.label`), shown only when I can change transactions and (for transfers) both legs are visible; mobile row click → `ViewTransactionDialog`.
- **`ViewTransactionDialog`:** fields per `pages.account.preview_transaction_modal.*` keys — header "Details", type badge (`type.expense|income|transfer`), sender/recipient account rows (icon, name with hidden fallback, formatted amount), Category, Notes, Payee, Tags, Author, Date (raw `transaction.date`); Edit/Delete buttons (disabled per the same permission rule) + Cancel.
- **Delete flow:** `ConfirmDialog` — question `t('pages.account.delete_transaction_modal.question')`, Delete/Cancel → `useDeleteTransaction`.
- **Empty list renders nothing** (no empty-state UI — Vue parity).

- [ ] **Step 1: failing hook tests** (`useAccountTransactions`): account filtering incl. recipient leg, day grouping with Today/Yesterday/ordinal headings, desc order, search across category/payee/description/amount, future flag.
- [ ] **Step 2: failing page tests**: renders header balance + rows from MSW data; transfer row incoming/outgoing titles and signs; title-source suppression (a category-titled row still shows description, a description-titled row doesn't duplicate it); delete confirm → MSW delete hit → row disappears and accounts cache updates; edit opens the transaction dialog prefilled.
- [ ] **Step 3: implement.** **Step 4: run** targeted + full suite + build → PASS.
- [ ] **Step 5: commit** `feat(web-react): account page with day-grouped transaction list`.

---

### Task 17: Metrics port

**Files:** Create `web-react/src/lib/metrics.ts`; wire calls.

Port `web/src/modules/metrics.ts` as-is (dataLayer push + the `METRICS` event-name constants). Wire the same events this plan's surfaces fire in Vue: `TRANSACTION_CREATE/UPDATE/DELETE`, `ACCOUNT_CREATE/UPDATE/DELETE`, `ACCOUNT_FOLDER_EXPAND/COLLAPSE`, `UI_MODAL_TRANSACTION_OPEN/CLOSE`, `UI_MODAL_ACCOUNT_OPEN/CLOSE`, `USER_LOGIN` (retro-wire into the Plan-1 login mutation). Fire from the mutation `onSuccess`/store actions — keep call sites minimal.

- [ ] **Step 1:** port + wire + one unit test (dataLayer receives the event name). **Step 2:** `pnpm test`/`build` → PASS.
- [ ] **Step 3: commit** `feat(web-react): dataLayer metrics events`.

---

### Task 18: Parity check — shell + accounts (manual, gate for Plan 2)

**Files:** none (verification only; fix divergences with a test before closing).

- [ ] **Step 1: run everything**

```bash
# backend on a scratch DB (no .env in this repo — use inline env)
PORT=8181 DATABASE_URL="sqlite://$(mktemp -d)/parity.sqlite" go run ./cmd/econumo serve
# Vue reference is served by the backend itself at :8181 (web/dist/spa)
make web-react-dev   # React on :9000 (proxies /api)
go run ./cmd/econumo user:create "Parity Tester" parity@example.test secret123
```

- [ ] **Step 2: walk the flows in BOTH apps at 1280px / 820px / 375px**

1. First login → loading gate → sidebar renders; create the FIRST account with a non-zero balance → a "General" folder appears, the account lands in it, and the opening-balance transaction is visible on the account page immediately (no reload).
2. Sidebar: folder totals (single-currency native vs mixed-currency converted), collapse/expand persists across reloads (React divergence: state honored — note it), hidden folder absent, "All accounts" label translated (divergence vs Vue's raw key — expected), shared-account avatar badges.
3. Account page: balance matches the Vue app for the same data (incl. a future-dated transaction NOT counted in balance but shown dimmed in the list — verify with `X-Timezone` by setting a non-UTC browser timezone), Today/Yesterday/date group headers identical, search finds by category/payee/description/amount.
4. Expense create: defaults (today, page account), category required message, calculator `5+3*2=` → 11 in both apps, create-on-the-fly category and payee, tags only on expense, saved row identical in both apps.
5. Income create: payee label "Sender", no tags row.
6. Transfer: same-currency (no recipient amount field) and cross-currency (prefilled converted amount, editable); From disabled on edit; swap button; after create the "Switch to" prompt appears and navigates; both account balances update in both apps without reload.
7. Edit + delete a transaction (desktop menu and mobile view-dialog paths); confirm dialog copy identical.
8. Account edit: rename, change balance (correction transaction appears, description "Balance adjustment" in the list), change currency and icon.
9. Sync button refetches; changing data via the VUE app then focusing the React window refetches (refetchOnWindowFocus) within staleTime rules.
10. Mobile: `/` shows the sidebar pane; opening an account shows only the workspace with a working back button; dialogs are bottom sheets.

- [ ] **Step 3: record** divergences found/fixed, then:

```bash
git commit --allow-empty -m "chore(web-react): shell+accounts parity check vs Vue app passed (desktop/tablet/mobile)"
```

---

## Plan sequence

This is Plan 2 of 6. Remaining (each written after the prior phase completes):

3. *(folded into 2/4 as needed — calculator/currency-select shipped here; month picker ships with Budget)*
4. Settings cluster (profile, change-password, accounts incl. dnd-kit drag-reorder + folder CRUD UI, categories, payees, tags, budgets list)
5. Budget (page split, envelopes, limits, widgets, modals, month picker)
6. Connections + onboarding + CSV import/export + the `web/` swap commit (Makefile, Dockerfile, delete Vue app)
