# Global Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `Ctrl/⌘+K` modal that searches accounts, own classifications and all transactions client-side, with drill-down and in-place edit/delete actions; plus a skip-tolerant ranked matcher shared with the transaction form's pickers.

**Architecture:** Pure frontend (`web/`). A matcher module (`web/src/lib/search.ts`), a data hook (`useGlobalSearch`) over the already-cached TanStack queries, and a `GlobalSearchDialog` built on `ResponsiveDialog` + the cmdk `Command` primitives. Account and classification actions are wrapped in reusable action components so the dialog never duplicates mutation logic.

**Tech Stack:** React 19, TypeScript, TanStack Query, Zustand, cmdk (`components/ui/command.tsx`), Radix dropdown, vitest + Testing Library + msw, react-i18next.

**Spec:** `docs/superpowers/specs/2026-10-02-global-search-design.md`

## Global Constraints

- Branch `feature/global-search`, PR base `v1.6-dev`. No Go/backend change.
- All commands run from `web/`: tests `pnpm test -- <path>`, lint `pnpm lint`, type-check `pnpm exec tsc -b`.
- Comments sparingly — only non-obvious *why* (CLAUDE.md "Comments — write sparingly").
- Every `t('…')` key must exist in ALL 11 `locales/<lang>.json` (`de en es fr it nl pl pt ru uk zh`) — real translations, not English copies. `{var}` placeholders identical across languages. New namespace: `search`.
- Plural strings: none needed (use `{count}` in a neutral phrasing, e.g. "Show all ({count})").
- Every new `METRICS` key must be fired (`web/src/lib/metrics-coverage.test.ts`).
- Classifications in search = own only (`ownerUserId === user.id`); archived included, ranked after active.
- Accounts in search = every account from `useAccounts()` (owned + shared, hidden folders included).
- Hotkey: `(metaKey || ctrlKey) && key.toLowerCase() === 'k'`; ignored when a `[role="dialog"]` is already in the document.
- Amounts/dates never fuzzy-match (`12.50` must not match `1250`).

## Review Focus

- A query that matches thousands of transactions (empty query = every transaction) must not freeze the dialog — rows render progressively (100 at a time). Test: Task 6 asserts only the first 100 transaction rows render for 250 transactions.
- Deleting / merging the classification you are drilled into must not leave an empty orphan header — view returns to Results. Test: Task 7.
- The hotkey pressed while the transaction form (or any dialog) is open must not open search on top. Test: Task 6.
- A transfer in the global list must not show a misleading "-"/"to X" from one side. Test: Task 3.
- `EntitySelect` must still offer "create «name»" when the typed name only fuzzy-matches existing options (e.g. typing "Fod" with "Food" existing). Test: Task 2.

---

## File Structure

| File | Responsibility |
|---|---|
| `web/src/lib/search.ts` (new) | `matchRank`, `rankByName`, `matchesTerms` |
| `web/src/features/transactions/EntitySelect.tsx` (mod) | use `rankByName` |
| `web/src/features/transactions/useAccountTransactions.ts` (mod) | export `enrichTransaction`, `groupByDay`, `buildLookups`; hook rewritten on top |
| `web/src/features/transactions/TransactionRow.tsx` (mod) | `pageAccount` optional → global mode |
| `web/src/features/search/useGlobalSearch.ts` (new) | data hook |
| `web/src/features/accounts/AccountActions.tsx` (new) | account `⋯` menu items + dialogs, shared with `AccountsSettingsPage` |
| `web/src/features/classifications/ClassificationActions.tsx` (new) | per-item edit/archive/merge/delete for category/payee/tag/label |
| `web/src/features/search/GlobalSearchDialog.tsx` (new) | the modal |
| `web/src/features/search/useWindowed.ts` (new) | progressive rendering hook |
| `web/src/app/uiStore.ts`, `web/src/app/layouts/ApplicationLayout.tsx` (mod) | open state + hotkey + mount |
| `web/src/lib/metrics.ts`, `locales/*.json` (mod) | analytics keys, i18n |
| `docs/regression-test-plan.md` (mod) | manual checklist |

---

### Task 1: Matcher module

**Files:**
- Create: `web/src/lib/search.ts`
- Test: `web/src/lib/search.test.ts`

**Interfaces:**
- Produces:
  ```ts
  export function matchRank(text: string, query: string): number | null // 0 exact,1 prefix,2 substring,3 subsequence
  export function rankByName<T>(items: T[], getName: (item: T) => string, query: string): T[]
  export interface TermFields { text: string[]; exact: string[] }
  export function matchesTerms(fields: TermFields, query: string): boolean
  export type ClassificationType = 'category' | 'payee' | 'tag' | 'label'
  ```
  (`ClassificationType` lives here so both `features/search` and `features/classifications` import ONE definition.)

- [ ] **Step 1: Write the failing test** — `web/src/lib/search.test.ts`

```ts
import { matchRank, matchesTerms, rankByName } from './search'

describe('matchRank', () => {
  it('ranks exact < prefix < substring < subsequence and rejects non-matches', () => {
    expect(matchRank('Food', 'food')).toBe(0)
    expect(matchRank('Groceries', 'gro')).toBe(1)
    expect(matchRank('Groceries', 'cer')).toBe(2)
    expect(matchRank('Groceries', 'grcries')).toBe(3)
    expect(matchRank('Groceries', 'xyz')).toBeNull()
  })
  it('matches everything on an empty or blank query', () => {
    expect(matchRank('Anything', '')).toBe(0)
    expect(matchRank('Anything', '   ')).toBe(0)
  })
})

describe('rankByName', () => {
  const items = [
    { name: 'Tagged groceries' }, // substring "gro"
    { name: 'Gym' },              // subsequence of "gm"? no — not for "gro"
    { name: 'Groceries' },        // prefix
    { name: 'gRo' },              // exact (case-insensitive)
    { name: 'Garage repairs o' }, // subsequence g..r..o
  ]
  it('filters and orders by rank, keeping input order on ties', () => {
    expect(rankByName(items, (i) => i.name, 'gro').map((i) => i.name)).toEqual([
      'gRo', 'Groceries', 'Tagged groceries', 'Garage repairs o',
    ])
  })
  it('returns the input unchanged for an empty query', () => {
    expect(rankByName(items, (i) => i.name, '')).toEqual(items)
  })
})

describe('matchesTerms', () => {
  const fields = { text: ['Morning latte at Starbucks', 'Visa Gold', 'Coffee'], exact: ['12.50', '2026-10-01 08:00:00', '-'] }
  it('requires every term (AND)', () => {
    expect(matchesTerms(fields, 'visa coffee')).toBe(true)
    expect(matchesTerms(fields, 'visa rent')).toBe(false)
  })
  it('lets a term skip characters within one word', () => {
    expect(matchesTerms(fields, 'stbcks')).toBe(true)
    expect(matchesTerms(fields, 'cofee')).toBe(true)
  })
  it('never matches a subsequence that spans words', () => {
    expect(matchesTerms({ text: ['come for elections'], exact: [] }, 'cofe')).toBe(false)
  })
  it('matches exact fields by substring only', () => {
    expect(matchesTerms(fields, '12.5')).toBe(true)
    expect(matchesTerms({ text: [], exact: ['1250'] }, '12.50')).toBe(false)
    expect(matchesTerms({ text: [], exact: ['12.50'] }, '1250')).toBe(false)
  })
  it('matches everything for an empty query', () => {
    expect(matchesTerms(fields, '  ')).toBe(true)
  })
})
```

- [ ] **Step 2: Run** `pnpm test -- src/lib/search.test.ts` — expect FAIL (module not found).

- [ ] **Step 3: Implement** `web/src/lib/search.ts`

```ts
import { fuzzyMatch } from './fuzzy'

export type ClassificationType = 'category' | 'payee' | 'tag' | 'label'

export function matchRank(text: string, query: string): number | null {
  const q = query.trim().toLowerCase()
  if (q === '') {
    return 0
  }
  const s = text.toLowerCase()
  if (s === q) {
    return 0
  }
  if (s.startsWith(q)) {
    return 1
  }
  if (s.includes(q)) {
    return 2
  }
  return fuzzyMatch(s, q) ? 3 : null
}

export function rankByName<T>(items: T[], getName: (item: T) => string, query: string): T[] {
  if (query.trim() === '') {
    return items
  }
  return items
    .map((item, index) => ({ item, index, rank: matchRank(getName(item), query) }))
    .filter((entry): entry is { item: T; index: number; rank: number } => entry.rank !== null)
    .sort((a, b) => a.rank - b.rank || a.index - b.index)
    .map((entry) => entry.item)
}

export interface TermFields {
  /** names and free text: a term may skip characters, but only within one word */
  text: string[]
  /** amounts, dates, signs: substring only */
  exact: string[]
}

// Subsequence matching is confined to single words: across one joined
// haystack a short term like "cofe" would match "come for elections".
export function matchesTerms(fields: TermFields, query: string): boolean {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) {
    return true
  }
  const text = fields.text.map((f) => f.toLowerCase())
  const exact = fields.exact.map((f) => f.toLowerCase())
  const words = text.flatMap((f) => f.split(/\s+/).filter(Boolean))
  return terms.every(
    (term) =>
      text.some((f) => f.includes(term)) ||
      exact.some((f) => f.includes(term)) ||
      words.some((w) => fuzzyMatch(w, term)),
  )
}
```

- [ ] **Step 4: Run** `pnpm test -- src/lib/search.test.ts` — expect PASS. If the `rankByName` ordering test fails because one of the fixture names doesn't land in the rank you expected, fix the FIXTURE so each name exercises the rank its comment states (don't loosen the implementation).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/search.ts web/src/lib/search.test.ts
git commit -m "feat(web): Ranked skip-tolerant search matcher"
```

---

### Task 2: EntitySelect uses the ranked matcher

**Files:**
- Modify: `web/src/features/transactions/EntitySelect.tsx:61`
- Test: `web/src/features/transactions/EntitySelect.test.tsx`
- Modify: `docs/regression-test-plan.md` (transaction form section)

**Interfaces:**
- Consumes: `rankByName` from Task 1.

- [ ] **Step 1: Add failing tests** to `EntitySelect.test.tsx` (reuse its `OPTIONS`, `combobox()` helper and `beforeEach`):

```tsx
it('matches when a character is skipped', async () => {
  const user = userEvent.setup()
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={[...OPTIONS, { value: 'c4', label: 'Groceries' }]} />)
  await user.click(combobox())
  await user.keyboard('grcries')
  expect(await screen.findByRole('option', { name: 'Groceries' })).toBeInTheDocument()
})

it('lists a prefix match before a substring match', async () => {
  const user = userEvent.setup()
  const options = [{ value: 'x1', label: 'Car rent' }, { value: 'x2', label: 'Rent' }]
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={options} />)
  await user.click(combobox())
  await user.keyboard('rent')
  const names = (await screen.findAllByRole('option')).map((o) => o.textContent)
  expect(names).toEqual(['Rent', 'Car rent'])
})

it('still offers create when the typed name only fuzzy-matches', async () => {
  const user = userEvent.setup()
  render(<EntitySelect aria-label="Category" value={null} onChange={() => {}} options={OPTIONS} onCreate={() => {}} />)
  await user.click(combobox())
  await user.keyboard('Fod')
  expect(await screen.findByRole('option', { name: 'Food' })).toBeInTheDocument()
  expect(screen.getByRole('option', { name: /Fod/ })).toBeInTheDocument()
})
```

(The create row's accessible name contains `«Fod»`; adjust the regex to the existing create-row test in this file if one exists.)

- [ ] **Step 2: Run** `pnpm test -- src/features/transactions/EntitySelect.test.tsx` — expect the three new tests FAIL.

- [ ] **Step 3: Implement** — in `EntitySelect.tsx` add `import { rankByName } from '@/lib/search'` and replace line 61:

```tsx
const filtered = rankByName(options, (o) => o.label, search)
```

Leave `exactMatch` / `canCreate` untouched.

- [ ] **Step 4: Run** the file again — expect all PASS, including the pre-existing tests.

- [ ] **Step 5: Regression plan** — in `docs/regression-test-plan.md`, in the transaction create/edit section, add an item (match surrounding style and 📱 markers):
  `- [ ] 📱 Category / payee / tag pickers find an item when a letter is skipped ("grcries" → Groceries); a prefix match is listed before a mid-word match; typing a new name that only resembles an existing one still offers "Add «name»".`

- [ ] **Step 6: Commit**

```bash
git add web/src/features/transactions/EntitySelect.tsx web/src/features/transactions/EntitySelect.test.tsx docs/regression-test-plan.md
git commit -m "feat(web): Skip-tolerant, ranked matching in the transaction form pickers"
```

---

### Task 3: Shared transaction enrichment + global TransactionRow

**Files:**
- Modify: `web/src/features/transactions/useAccountTransactions.ts`
- Modify: `web/src/features/transactions/TransactionRow.tsx`
- Test: `web/src/features/transactions/useAccountTransactions.test.tsx` (must pass UNCHANGED), `web/src/features/transactions/TransactionRow.test.tsx` (add cases)

**Interfaces:**
- Produces (exported from `useAccountTransactions.ts`):
  ```ts
  export interface TransactionLookups {
    accounts?: AccountDto[]; categories?: CategoryDto[]; payees?: PayeeDto[]; tags?: TagDto[]; labels?: LabelDto[]
  }
  export function enrichTransaction(tx: TransactionDto, l: TransactionLookups): ViewTransaction
  export function groupByDay(placed: { tx: ViewTransaction; groupDay: string }[]): DailyListEntry[]
  export function useTransactionLookups(): TransactionLookups
  ```
- Produces (`TransactionRow`): `pageAccount?: AccountDto` — when omitted the row is in **global** mode.

- [ ] **Step 1: Refactor** `useAccountTransactions.ts` — extract, with no behaviour change:
  - `enrichTransaction(tx, l)`: the body of the current `.map((tx) => ({...}))` for real transactions (account, accountRecipient, category, payee, tag, labels via `resolveLabels`, `isInFuture`).
  - `groupByDay(placed)`: the final loop that emits separators (`label` via `isToday`/`isYesterday`).
  - `useTransactionLookups()`: calls `useAccounts/useCategories/usePayees/useTags/useLabels` and returns `useMemo(() => ({ accounts, categories, payees, tags, labels }), [...])`.
  - Rewrite the hook body to use them (the recurring virtual rows and pinning stay inline in the hook).

- [ ] **Step 2: Run** `pnpm test -- src/features/transactions src/features/accounts` — expect PASS with the test files untouched.

- [ ] **Step 3: Write failing TransactionRow tests** (follow the existing render setup in `TransactionRow.test.tsx`; build `ViewTransaction` objects the same way it does):

```tsx
it('global mode: shows the account name line and signs expense/income', () => {
  // expense on account "Cash" (currency $), no pageAccount
  render(<TransactionRow transaction={expenseOnCash} />)
  expect(screen.getByTestId(`tx-account-${expenseOnCash.id}`)).toHaveTextContent('Cash')
  expect(screen.getByText(/^-9\.99/)).toBeInTheDocument()
})

it('global mode: a transfer shows From → To, its description as title, and no sign', () => {
  // transfer Cash -> Card, description "move money"
  render(<TransactionRow transaction={transferCashToCard} />)
  expect(screen.getByTestId(`tx-account-${transferCashToCard.id}`)).toHaveTextContent('Cash → Card')
  expect(screen.getByText('move money')).toBeInTheDocument()
  expect(screen.queryByText(/Transfer to/)).not.toBeInTheDocument()
  expect(screen.getByText(/^100(\.00)?/)).toBeInTheDocument() // no leading - or +
})
```

- [ ] **Step 4: Run** — expect FAIL (`pageAccount` required / no account line).

- [ ] **Step 5: Implement global mode** in `TransactionRow.tsx`:
  - `pageAccount?: AccountDto`. `const global = !pageAccount`.
  - Title: when `global && tx.type === 'transfer'` → `{ text: tx.description || t('transactions.<existing transfer type label key>'), source: tx.description ? 'description' : 'transfer' }` (find the existing "Transfer" type label key in `locales/en.json` under `transactions` — reuse it, do not add a new one). Otherwise `transactionTitleInfo(tx, pageAccount?.id ?? tx.accountId, t)`.
  - Amount: `displayAmount` gets a global branch — transfer → no sign, formatted with `tx.account?.currency`; expense `-`, income `+`. Currency symbol: `(pageAccount ?? tx.account)?.currency.symbol`. Colour: global transfer uses `text-muted-foreground`; otherwise the existing income/expense rule via `isIncomeForAccount(tx, tx.accountId)`.
  - Author avatar: in global mode show it when `tx.account?.sharedAccess.length`.
  - Account line (global only), rendered as the first muted line under the title:
    ```tsx
    <span data-testid={`tx-account-${tx.id}`} className="truncate text-[13px] text-muted-foreground">
      {tx.type === 'transfer'
        ? `${tx.account?.name ?? t('accounts.account.name_hidden')} → ${tx.accountRecipient?.name ?? t('accounts.account.name_hidden')}`
        : (tx.account?.name ?? t('accounts.account.name_hidden'))}
    </span>
    ```
  - Account-page rendering (pageAccount given) must be byte-for-byte unchanged.

- [ ] **Step 6: Run** `pnpm test -- src/features/transactions src/features/accounts src/features/recurring` — expect PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/transactions
git commit -m "refactor(web): Share transaction enrichment; TransactionRow global mode"
```

---

### Task 4: `useGlobalSearch` data hook

**Files:**
- Create: `web/src/features/search/useGlobalSearch.ts`
- Test: `web/src/features/search/useGlobalSearch.test.tsx`

**Interfaces:**
- Consumes: `matchesTerms`, `rankByName` (Task 1); `enrichTransaction`, `groupByDay`, `useTransactionLookups`, `DailyListEntry`, `ViewTransaction` (Task 3); `useTransactions` (`features/transactions/queries`), `useUserData` (`features/user/queries`), `dayKey` (`lib/datetime`).
- Produces:
  ```ts
  // ClassificationType is imported from '@/lib/search' (Task 1)
  export type SearchScope = { kind: 'all' } | { kind: ClassificationType; id: Id }
  export interface GlobalSearchResult {
    accounts: AccountDto[]
    categories: CategoryDto[]
    payees: PayeeDto[]
    tags: TagDto[]
    labels: LabelDto[]
    transactions: DailyListEntry[]
    transactionCount: number
  }
  export function useGlobalSearch(query: string, scope: SearchScope): GlobalSearchResult
  export function transactionFields(tx: ViewTransaction): TermFields
  ```

- [ ] **Step 1: Write failing tests** (pattern from `useAccountTransactions.test.tsx`: `renderHook` + `QueryClientProvider` + `server.use(...coreHandlers({...}))`). Build overrides from the fixtures in `web/src/test/fixtures.ts`:
  - `fixtureUser` is the current user (id — read it from the fixture); give one category/payee/tag/label an `ownerUserId` of another user `'u-other'`, and mark one own category `isArchived: 1`.
  - Put one account in a folder with `isVisible: 0`, and one shared account (`owner` ≠ current user).

```tsx
it('empty query: every transaction, no entity groups', async () => {
  const { result } = renderHook(() => useGlobalSearch('', { kind: 'all' }), { wrapper })
  await waitFor(() => expect(result.current.transactionCount).toBe(TOTAL_TX))
  expect(result.current.accounts).toEqual([])
  expect(result.current.categories).toEqual([])
})

it('classifications are own-only and archived ones come last', async () => {
  const { result } = renderHook(() => useGlobalSearch('f', { kind: 'all' }), { wrapper })
  await waitFor(() => expect(result.current.categories.length).toBeGreaterThan(0))
  expect(result.current.categories.every((c) => c.ownerUserId === ME)).toBe(true)
  const archivedIdx = result.current.categories.findIndex((c) => c.isArchived === 1)
  expect(result.current.categories.slice(archivedIdx).every((c) => c.isArchived === 1)).toBe(true)
})

it('accounts include hidden-folder and shared accounts', async () => { /* query that matches both by name */ })
it('an account name finds its transactions', async () => { /* query = account name; every tx belongs to it */ })
it('a skipped character still finds a transaction by description', async () => { /* 'cofee' -> 'coffee beans' */ })
it.each(['category', 'payee', 'tag', 'label'] as const)('scope %s narrows to that item', async (kind) => {
  /* every returned tx has categoryId/payeeId/tagId === id, or labelIds includes id; entity groups empty */
})
it('orders transactions newest first and groups by day', async () => {
  /* first entry is a separator, the next transaction has the max date */
})
```

Fill in each body completely with concrete fixture ids — no empty bodies.

- [ ] **Step 2: Run** `pnpm test -- src/features/search/useGlobalSearch.test.tsx` — expect FAIL.

- [ ] **Step 3: Implement** `useGlobalSearch.ts`:

```ts
import { useMemo } from 'react'
import { dayKey } from '@/lib/datetime'
import { matchesTerms, rankByName, type ClassificationType, type TermFields } from '@/lib/search'
import { useTransactions } from '@/features/transactions/queries'
import {
  enrichTransaction, groupByDay, useTransactionLookups,
  type DailyListEntry, type ViewTransaction,
} from '@/features/transactions/useAccountTransactions'
import { useUserData } from '@/features/user/queries'
// + DTO type imports

export function transactionFields(tx: ViewTransaction): TermFields {
  return {
    text: [
      tx.description,
      tx.account?.name ?? '',
      tx.accountRecipient?.name ?? '',
      tx.category?.name ?? '',
      tx.payee?.name ?? '',
      tx.tag?.name ?? '',
      ...(tx.labels ?? []).map((l) => l.name),
      `@${tx.author?.name ?? ''}`,
      tx.type,
    ],
    exact: [tx.amount, tx.amountRecipient ?? '', tx.date, tx.type === 'expense' ? '-' : '+'],
  }
}

function ownActiveFirst<T extends { ownerUserId: string; isArchived: 0 | 1; name: string }>(
  items: T[] | undefined, me: string | undefined, query: string,
): T[] {
  if (!query.trim() || !items || !me) return []
  const ranked = rankByName(items.filter((i) => i.ownerUserId === me), (i) => i.name, query)
  return [...ranked.filter((i) => i.isArchived === 0), ...ranked.filter((i) => i.isArchived === 1)]
}

function inScope(tx: ViewTransaction, scope: SearchScope): boolean {
  switch (scope.kind) {
    case 'all': return true
    case 'category': return tx.categoryId === scope.id
    case 'payee': return tx.payeeId === scope.id
    case 'tag': return tx.tagId === scope.id
    case 'label': return (tx.labelIds ?? []).includes(scope.id)
  }
}

export function useGlobalSearch(query: string, scope: SearchScope): GlobalSearchResult {
  const { data: transactions } = useTransactions()
  const { data: user } = useUserData()
  const lookups = useTransactionLookups()

  const enriched = useMemo(
    () => (transactions ?? []).map((tx) => enrichTransaction(tx, lookups)),
    [transactions, lookups],
  )

  return useMemo(() => {
    const groups = scope.kind === 'all'
    const me = user?.id
    const matched = enriched
      .filter((tx) => inScope(tx, scope) && matchesTerms(transactionFields(tx), query))
      .map((tx, index) => ({ tx, index }))
      .sort((a, b) => (a.tx.date < b.tx.date ? 1 : a.tx.date > b.tx.date ? -1 : a.index - b.index))
      .map(({ tx }) => ({ tx, groupDay: dayKey(tx.date) }))
    return {
      accounts: groups && query.trim() ? rankByName(lookups.accounts ?? [], (a) => a.name, query) : [],
      categories: groups ? ownActiveFirst(lookups.categories, me, query) : [],
      payees: groups ? ownActiveFirst(lookups.payees, me, query) : [],
      tags: groups ? ownActiveFirst(lookups.tags, me, query) : [],
      labels: groups ? ownActiveFirst(lookups.labels, me, query) : [],
      transactions: groupByDay(matched),
      transactionCount: matched.length,
    }
  }, [enriched, lookups, user?.id, query, scope])
}
```

(`scope` must be memo-stable in callers — the dialog keeps it in `useState`. Verify the DTO field names (`ownerUserId`, `isArchived`) against `web/src/api/dto/*`; `LabelDto` may differ — adapt the generic constraint if so.)

- [ ] **Step 4: Run** the test file — expect PASS. Then `pnpm exec tsc -b`.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/search
git commit -m "feat(web): useGlobalSearch data hook"
```

---

### Task 5: Reusable account and classification actions

**Files:**
- Create: `web/src/features/accounts/AccountActions.tsx`
- Modify: `web/src/features/accounts/AccountsSettingsPage.tsx` (use it for the row menu's delete/decline/access flow)
- Create: `web/src/features/classifications/ClassificationActions.tsx`
- Test: `web/src/features/accounts/AccountActions.test.tsx`, `web/src/features/classifications/ClassificationActions.test.tsx`; existing `AccountsSettingsPage.test.tsx` and classification page tests must pass unchanged.

**Interfaces:**
- Produces:
  ```tsx
  // AccountActions.tsx
  export function AccountActionsMenu(props: { account: AccountDto; onDone?: () => void }): JSX.Element
  // renders a ghost icon Button (aria-label `account actions ${account.name}`, MoreVertical)
  // + DropdownMenu with Edit / Access (only hasAccountAdminAccess) / Delete (owner) | Decline (shared),
  // and owns the ConfirmDialogs + ShareAccessDialog + AccessLevelDialog those items open.

  // ClassificationActions.tsx
  // ClassificationType is imported from '@/lib/search' (Task 1)
  export function ClassificationActionsMenu(props: {
    type: ClassificationType
    item: CategoryDto | PayeeDto | TagDto | LabelDto
  }): JSX.Element
  // ghost icon Button (aria-label `actions ${item.name}`) + DropdownMenu:
  // Edit, Archive|Unarchive, Merge, Delete — plus the dialogs they open.
  ```

- [ ] **Step 1: Write failing tests.**
  - `AccountActions.test.tsx`: owned account → menu shows Edit, Access (owner is admin), Delete; Delete → confirm → `POST */api/v1/account/delete-account` called (msw spy). Shared account where the user is a plain `user` role → Edit, Decline, no Access. Edit → `useUiStore.getState().accountModal?.account?.id === account.id`.
  - `ClassificationActions.test.tsx`: for each type, menu shows Edit / Archive / Merge / Delete; archived item shows Unarchive; Archive calls the matching endpoint (`category/archive-category`, `payee/archive-payee`, `tag/archive-tag`, `label/archive-label` — confirm paths in `web/src/api/*.ts`); Delete confirms then calls delete; Merge opens `MergeDialog` whose candidates are own items of the same type (and same `type` for categories), excluding the source.

- [ ] **Step 2: Run** them — expect FAIL.

- [ ] **Step 3: Implement `AccountActions.tsx`** by MOVING the logic now inline in `AccountsSettingsPage` (state `deleteAccountTarget`, `declineAccountTarget`, `accessAccountId`, `levelTarget`; the `ShareAccessDialog`, `AccessLevelDialog`, delete and decline `ConfirmDialog`s at lines ~530–640; the `onMenu` branch at ~481–497 including the `invalidateQueries(queryKeys.accounts)` before opening Access). Then make `AccountsSettingsPage`'s `AccountRow` render `<AccountActionsMenu account={account} />` for its desktop menu and keep the compact `view` preview path as is. Remove the now-dead state from the page. Item labels: `common.button.edit.label`, `settings.accounts.list_actions.access`, `common.button.delete.label` / `common.button.decline.label`.

- [ ] **Step 4: Implement `ClassificationActions.tsx`.** One component, switching on `type` for hooks/dialogs:
  - Edit: category → `CategoryDialog` + `useUpdateCategory` (`{ id, name, icon }`); payee → `PromptDialog` with the same `validate` as `PayeesPage` (move that validator into an exported `validatePayeeName(t)` helper in `PayeesPage.tsx` or a small shared file, and use it from both); tag/label → `TagDialog` with `item={{ id, name, kind: type, icon }}`.
  - Archive/Unarchive: `useArchiveX` / `useUnarchiveX` per type.
  - Merge: `MergeDialog` with `candidates` = own items (via `useUserData`) of the same type, minus the source, same `type` for categories; `warning`/`info`/`showIcon` exactly as on the corresponding settings page; confirm → `useMergeX().mutate({ sourceId, targetId })`.
  - Delete: `ConfirmDialog` with `title` = the per-type delete title (`classifications.<categories|payees|tags|labels>.modals.delete.title`), `question` = item name, destructive → `useDeleteX().mutate(id)`.
  - Labels: Edit `common.button.edit.label`; Archive/Unarchive — reuse the labels `ClassificationList` uses for its row menu (find them in `ClassificationList.tsx`); Merge `classifications.common.merge.action`; Delete `common.button.delete.label`.
  - The menu's `DropdownMenuContent` must `stopPropagation` on click (portaled clicks bubble to the cmdk row otherwise — see the comment in `AccountsSettingsPage`).

- [ ] **Step 5: Run** `pnpm test -- src/features/accounts src/features/classifications` — expect PASS (new and existing).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/accounts web/src/features/classifications
git commit -m "refactor(web): Reusable account and classification action menus"
```

---

### Task 6: GlobalSearchDialog — results view, hotkey, analytics, i18n

**Files:**
- Modify: `web/src/app/uiStore.ts` — `searchOpen: boolean`, `openSearch()`, `closeSearch()`
- Create: `web/src/features/search/useWindowed.ts`
- Create: `web/src/features/search/GlobalSearchDialog.tsx`
- Create: `web/src/features/search/useSearchHotkey.ts`
- Modify: `web/src/app/layouts/ApplicationLayout.tsx` — call `useSearchHotkey()`, mount `<GlobalSearchDialog />` next to `<TransactionDialog />`
- Modify: `web/src/lib/metrics.ts` — `GLOBAL_SEARCH_OPEN: 'appGlobalSearchOpen'`, `GLOBAL_SEARCH_SELECT: 'appGlobalSearchSelect'`
- Modify: `locales/*.json` (all 11) — `search` namespace
- Test: `web/src/features/search/GlobalSearchDialog.test.tsx`, `web/src/features/search/useSearchHotkey.test.tsx`

**Interfaces:**
- Consumes: Tasks 3–5.
- Produces: `useUiStore` `searchOpen/openSearch/closeSearch`; `GlobalSearchDialog` (no props).

i18n keys (English values; translate for the other 10):
```json
"search": {
  "title": "Search",
  "placeholder": "Search transactions, accounts, categories…",
  "groups": { "accounts": "Accounts", "categories": "Categories", "payees": "Payees", "tags": "Tags", "labels": "Labels", "transactions": "Transactions" },
  "show_all": "Show all ({count})",
  "back": "Back",
  "nothing_found": "Nothing found"
}
```

- [ ] **Step 1: Write failing tests.**

`useSearchHotkey.test.tsx`:
```tsx
it('Ctrl+K and ⌘K open search', () => {
  renderHook(() => useSearchHotkey())
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  expect(useUiStore.getState().searchOpen).toBe(true)
  act(() => useUiStore.getState().closeSearch())
  fireEvent.keyDown(window, { key: 'K', metaKey: true })
  expect(useUiStore.getState().searchOpen).toBe(true)
})
it('is ignored while another dialog is open', () => {
  renderHook(() => useSearchHotkey())
  const d = document.createElement('div'); d.setAttribute('role', 'dialog'); document.body.append(d)
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  expect(useUiStore.getState().searchOpen).toBe(false)
  d.remove()
})
it('fires the open metric', () => { /* vi.mock metrics like BudgetTransactionsDialog.test.tsx; expect trackEvent(METRICS.GLOBAL_SEARCH_OPEN) */ })
```

`GlobalSearchDialog.test.tsx` (setup like `BudgetTransactionsDialog.test.tsx`: `QueryClientProvider`, `MemoryRouter`, `coreHandlers`, `matchMedia` stub, `useUiStore.setState({ searchOpen: true })`):
- empty query shows transaction rows and no group headings;
- typing an account name shows the Accounts group; clicking the account navigates to `/account/<id>` (render inside `MemoryRouter` with a `Routes` probe) and closes (`searchOpen === false`); `trackEvent(METRICS.GLOBAL_SEARCH_SELECT, { type: 'account' })`;
- a group with 7 matches shows 5 rows + "Show all (7)"; clicking it shows 7;
- typing gibberish shows "Nothing found";
- clicking a transaction opens the view dialog with Edit and Delete; Delete → confirm → `POST */api/v1/transaction/delete-transaction`; Edit → `useUiStore.getState().transactionModal` set;
- 250 transactions + empty query → exactly 100 `[data-testid^="tx-"]` rows (filter out `tx-account-` test ids) — `IntersectionObserver` must be stubbed as a no-op class in the test.
- input is focused on open and empty after reopen.

- [ ] **Step 2: Run** — expect FAIL.

- [ ] **Step 3: Implement `uiStore`** additions:
```ts
searchOpen: false,
openSearch: () => {
  trackEvent(METRICS.GLOBAL_SEARCH_OPEN)
  set({ searchOpen: true })
},
closeSearch: () => set({ searchOpen: false }),
```
(uiStore already imports `trackEvent`/`METRICS`.)

- [ ] **Step 4: Implement `useSearchHotkey.ts`:**
```ts
export function useSearchHotkey() {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'k') return
      const { searchOpen, openSearch } = useUiStore.getState()
      // never stack over a form or another dialog
      if (searchOpen || document.querySelector('[role="dialog"]')) return
      e.preventDefault()
      openSearch()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])
}
```

- [ ] **Step 5: Implement `useWindowed.ts`** — the sentinel half of `AccountPage`'s `WindowedEntries` (no anchoring):
```ts
export const LIST_CHUNK = 100
export function useWindowed(total: number, resetKey: unknown) {
  const [count, setCount] = useState(LIST_CHUNK)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => setCount(LIST_CHUNK), [resetKey])
  const hasMore = count < total
  useEffect(() => {
    const el = sentinelRef.current
    if (!el) return
    const observer = new IntersectionObserver(
      (hits) => { if (hits.some((h) => h.isIntersecting)) setCount((c) => c + LIST_CHUNK) },
      { root: el.closest('[cmdk-list]') ?? el.parentElement, rootMargin: '600px' },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [hasMore, count])
  return { count, hasMore, sentinelRef }
}
```

- [ ] **Step 6: Implement `GlobalSearchDialog.tsx` (Results view only; drill-down in Task 7).** Structure:
```tsx
export function GlobalSearchDialog() {
  const { t, i18n } = useTranslation()
  const open = useUiStore((s) => s.searchOpen)
  const close = useUiStore((s) => s.closeSearch)
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<SearchScope>({ kind: 'all' })
  const [expanded, setExpanded] = useState<Partial<Record<GroupKey, boolean>>>({})
  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)
  const result = useGlobalSearch(query, scope)
  const { count, hasMore, sentinelRef } = useWindowed(result.transactions.length, `${query}|${JSON.stringify(scope)}`)
  // reset on every open
  useEffect(() => { if (open) { setQuery(''); setScope({ kind: 'all' }); setExpanded({}) } }, [open])
  ...
  return (
    <>
      <ResponsiveDialog open={open} onOpenChange={(o) => !o && close()} title={t('search.title')} hideHeader fullScreen>
        <Command shouldFilter={false} loop>
          <CommandInput autoFocus value={query} onValueChange={setQuery} placeholder={t('search.placeholder')} />
          <CommandList className="max-h-[70vh]">
            {/* groups: accounts, categories, payees, tags, labels — each hidden when empty */}
            {/* transactions: day separators as non-selectable headings; rows as CommandItem */}
            {nothing ? <div className="py-6 text-center text-sm text-muted-foreground">{t('search.nothing_found')}</div> : null}
          </CommandList>
        </Command>
      </ResponsiveDialog>
      {preview ? <ViewTransactionDialog ... /> : null}
      <ConfirmDialog ... delete transaction ... />
    </>
  )
}
```
  Details:
  - `CommandItem value` must be unique per row (`account:<id>`, `category:<id>`, `tx:<id>`) since filtering is off.
  - Group rows: show `items.slice(0, expanded[g] ? undefined : 5)`; when `items.length > 5 && !expanded[g]` add a `CommandItem` "Show all ({count})" that sets `expanded[g] = true`.
  - Account row: `EntityIcon name={account.icon}`, name, balance `moneyFormat(account.balance, account.currency)`, folder name (look up via `useFolders()` from `features/accounts/queries` — confirm the hook name), and `<AccountActionsMenu account={account} />` trailing. `onSelect` → `trackEvent(METRICS.GLOBAL_SEARCH_SELECT, { type: 'account' })`, `close()`, `navigate(\`/account/${account.id}\`)` (use the existing `RouterPage`/path helper if one exists for account pages — grep `RouterPage.ACCOUNT`).
  - Classification row: icon (`item.icon` for category/tag/label, a payee glyph for payees — reuse what `PayeesPage`/`ClassificationList` shows), name, archived badge (`classifications.<x>.pages.settings.archived_item`) + `opacity-60` when archived, trailing `<ClassificationActionsMenu type item />`. `onSelect` → set in Task 7 (for now: no-op placeholder is NOT allowed — implement drill-down state setting here: `setScope({ kind: type, id })`, `setQuery('')`, track select with `{ type }`; the header UI comes in Task 7).
  - Transaction rows: `result.transactions.slice(0, count)`; separators render `separatorText(entry, t, i18n.language)` as a muted heading; transactions render `<TransactionRow transaction={tx} />` inside a `CommandItem` whose `onSelect` → `setPreview(tx)` + track `{ type: 'transaction' }`. Then `{hasMore ? <div ref={sentinelRef} className="h-px" /> : null}`.
  - Trailing menu buttons inside `CommandItem`: stop pointer/click propagation so they don't trigger `onSelect`.
  - Transaction preview/edit/delete: copy `AccountPage`'s wiring — `canChange = !!user && !!tx.account && canWriteToAccount(tx.account, user.id)` (check the exact signature in `features/connections/shared.ts`), `isShared = (tx.account?.sharedAccess.length ?? 0) > 0`, Edit → `openTransactionModal({ transaction: <same prefill AccountPage's editTransaction builds> })` — extract `editTransaction`'s prefill builder from `AccountPage` into an exported helper in `features/transactions/` if it is not already shared, and use it in both places. Opening the transaction form must close search first (`close()`), so the form isn't stacked under the search sheet.
  - `nothing` = query non-empty and every group and transactions empty.

- [ ] **Step 7: Wire** `ApplicationLayout`: `useSearchHotkey()` inside the component, `<GlobalSearchDialog />` after `<TransactionDialog />`.

- [ ] **Step 8: Metrics + i18n**: add the two keys to `METRICS`; add the `search` block to all 11 catalogues (sensible native translations).

- [ ] **Step 9: Run** `pnpm test -- src/features/search src/app src/lib` then `pnpm exec tsc -b` and `pnpm lint` — expect PASS. Also run the Go i18n guard: `cd .. && GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/` — expect PASS.

- [ ] **Step 10: Commit**

```bash
git add web/src locales
git commit -m "feat(web): Global search dialog on Ctrl/⌘+K"
```

---

### Task 7: Drill-down view

**Files:**
- Modify: `web/src/features/search/GlobalSearchDialog.tsx`
- Test: `web/src/features/search/GlobalSearchDialog.test.tsx`

**Interfaces:**
- Consumes: `SearchScope`, `ClassificationActionsMenu`, lookups from Task 3.

- [ ] **Step 1: Write failing tests:**
  - typing "foo" then selecting category "Food" shows a header with "Food", a Back button (`aria-label` = `search.back`), and only Food's transactions; the input is empty;
  - typing inside the drill-down narrows within Food's transactions;
  - Back restores Results with the previous query "foo";
  - Backspace on an empty input in drill-down = Back;
  - for a label: only transactions whose `labelIds` include it;
  - deleting the drilled category via the header `⋯` → confirm → after the categories query refetches without it, the view returns to Results (msw: after delete, `get-category-list` returns the list without it).

- [ ] **Step 2: Run** — expect FAIL.

- [ ] **Step 3: Implement:**
  - State `prevQuery` saved when drilling in; `goBack = () => { setScope({ kind: 'all' }); setQuery(prevQuery) }`.
  - Resolve the drilled item from the live lookups (`useTransactionLookups()` + `scope`); if `scope.kind !== 'all'` and the item is not found, call `goBack()` in an effect.
  - Header above `CommandInput` when drilled: `<Button variant="ghost" size="icon" aria-label={t('search.back')} onClick={goBack}><ArrowLeft/></Button>`, icon + name, `<ClassificationActionsMenu type={scope.kind} item={item} />`.
  - `CommandInput` `onKeyDown`: `if (e.key === 'Backspace' && query === '' && scope.kind !== 'all') { e.preventDefault(); goBack() }`.
  - Groups are already empty for non-`all` scopes (hook), so only transactions render.

- [ ] **Step 4: Run** `pnpm test -- src/features/search` — PASS; `pnpm exec tsc -b`; `pnpm lint`.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/search
git commit -m "feat(web): Drill into a classification from global search"
```

---

### Task 8: Regression plan + full verification

**Files:**
- Modify: `docs/regression-test-plan.md`

- [ ] **Step 1:** Add a "Global search" section (no 📱 markers — hotkey only; desktop + tablet with keyboard):
  - `Ctrl+K` (Windows/Linux) / `⌘K` (macOS) opens search from any page; does nothing while another dialog (e.g. Add transaction) is open; input focused and empty on each open.
  - Empty query lists recent transactions across all accounts, newest first, grouped by day, each row naming its account; transfers read "From → To" with no +/− sign.
  - Typing filters accounts, own categories, payees, tags, labels and transactions; a skipped letter still matches ("grcries"); amounts match exactly only.
  - Accounts in a hidden folder and shared accounts appear; tapping opens the account page and closes search; `⋯` shows Edit / Access (admin only) / Delete (owner) or Decline (shared).
  - Connected users' classifications never appear; archived ones appear after active ones, dimmed with the archived badge, `⋯` offers Unarchive.
  - Tapping a classification shows only its transactions; typing narrows within it; Back (button or Backspace on empty input) returns with the previous query; deleting or merging it from the header `⋯` returns to results.
  - A group with more than 5 matches shows "Show all (N)".
  - Tapping a transaction opens its preview; Edit opens the form (search closes); Delete removes it and the list updates.
  - ↑/↓ move the highlight, Enter opens the highlighted row, Esc closes.
- [ ] **Step 2: Full gates** from `web/`: `pnpm test`, `pnpm lint`, `pnpm exec tsc -b`, `pnpm build`. All must pass; paste the summary lines into the task report.
- [ ] **Step 3: Commit**

```bash
git add docs/regression-test-plan.md
git commit -m "docs: Global search regression checklist"
```
