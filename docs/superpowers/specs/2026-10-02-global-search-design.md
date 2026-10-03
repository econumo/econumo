# Global search — design

Date: 2026-10-02 · Branch: `feature/global-search` (base `v1.6-dev`)

## Goal

One keyboard shortcut, from anywhere in the app, opens a modal that finds a
transaction, account, category, payee, tag or label in a few keystrokes and
lets the user fix it on the spot — edit or delete a transaction, rename /
archive / merge / delete a classification, edit / share / delete an account —
without navigating to the account page or the settings screens first.

Success: finding and correcting a mis-categorised transaction, a stray
duplicate payee, or an account parked in a hidden folder takes seconds.

## Decisions (agreed in brainstorming)

| Topic | Decision |
|---|---|
| Entry point | Hotkey only: `Ctrl+K` (`⌘K` on macOS). No icon yet — opening goes through one `uiStore.openSearch()` action so a later icon is a one-line change. |
| Result kinds | Accounts, categories, payees, tags, labels, transactions. Not budgets, not recurring templates. |
| Empty query | Recent transactions across all available accounts (day-grouped); no entity groups. |
| Classifications | The current user's own only (`ownerUserId === user.id`). Archived ones included, ranked after active ones, dimmed with the archived badge. |
| Classification tap | Drill-down: the modal narrows to that item's transactions. A `⋯` menu on the row (and in the drill-down header) holds the direct actions. |
| Accounts | Every account available to the user (owned + shared), including accounts in hidden folders. Tap navigates to the account page; `⋯` holds the account actions. |
| Transaction rows | Visible account name on every row; transfers show `From → To`. |
| Matching | Skip-tolerant (subsequence) fuzzy, ranked. The same matcher replaces the substring filter in the transaction form's `EntitySelect`. |
| Backend | None. Everything needed is already in the client query cache. |

## Approach

Client-side search. `useTransactions()` already loads every transaction the
user can see (`GET /api/v1/transaction/get-transaction-list`, no filter), and
accounts, categories, payees, tags and labels are all cached app-wide. The
account page already filters its full dataset in memory. A server endpoint
would add API, golden, MCP and engine-parity surface for no gain.

## Components

All new code lives in `web/src/features/search/` unless noted.

### 1. Matcher — `web/src/lib/search.ts`

One module, used by global search and by `EntitySelect`.

- `matchRank(text, query): number | null` — rank of `query` against a short
  name, `null` when it does not match. Case-insensitive (`toLowerCase`, same
  as the existing code — no locale folding). Ranks, best first:
  `0` exact, `1` prefix, `2` substring, `3` subsequence (`fuzzyMatch` from
  `web/src/lib/fuzzy.ts`). An empty query returns `0` for every text.
- `rankByName(items, getName, query)` — filters to matches and sorts by rank,
  then by the input order (callers pass already position-ordered lists, so
  ties keep the user's ordering). Stable.
- `matchesTerms(fields, query): boolean` — the transaction matcher. The query
  is split on whitespace into terms; **every** term must match (AND). A term
  matches when it is a substring of any field, or a subsequence of any single
  **word** of a *text* field (words = the field split on whitespace).
  Fields are passed as `{ text: string[]; exact: string[] }`:
  - `text` (word-level fuzzy allowed): description, account name(s),
    category, payee, tag, label names, `@author`, type.
  - `exact` (substring only): amount, amountRecipient, date, sign.
  Word-level subsequence is deliberate: subsequence over one joined haystack
  would let `cofe` match "come for elections" and turn a search over
  thousands of rows into noise. Amounts stay exact so `12.50` never matches
  `1250`.

`fuzzyMatch` stays where it is; `ClassificationList`'s settings-page filter
keeps using it unchanged.

### 2. Shared transaction enrichment

Extract from `useAccountTransactions.ts` into exported helpers in the same
file (no behaviour change for the account page):

- `enrichTransaction(tx, lookups): ViewTransaction` — resolves account,
  recipient, category, payee, tag, labels, `isInFuture` (today inline in the
  `.map`).
- `groupByDay(items): DailyListEntry[]` — emits the separator/transaction
  entries from an already-sorted list (today the loop at the end of the
  hook). Takes `{ tx, groupDay }` pairs so the account page's recurring
  pinning keeps working.

`useAccountTransactions` is rewritten on top of these and its existing tests
must pass untouched. The account page's own search keeps its current
substring semantics — switching it to the new matcher is out of scope.

### 3. `useGlobalSearch(query, scope)`

Pure data hook, no UI.

- `scope`: `{ kind: 'all' } | { kind: 'category' | 'payee' | 'tag' | 'label'; id: Id }`.
- Returns:
  ```ts
  interface GlobalSearchResult {
    accounts: AccountDto[]          // empty when query is empty or scope != all
    categories: CategoryDto[]       // own only; active then archived, each by rank
    payees: PayeeDto[]
    tags: TagDto[]
    labels: LabelDto[]
    transactions: DailyListEntry[]  // day-grouped, newest first
    transactionCount: number
  }
  ```
- Accounts: all of `useAccounts()` — no folder visibility filter, owned and
  shared alike — ranked by `rankByName` on `name`.
- Classifications: `ownerUserId === user.id`, ranked by `rankByName`, then
  stably partitioned so every active match precedes every archived match.
- Transactions: every transaction in `useTransactions()` enriched with
  `enrichTransaction`; no recurring virtual rows. When `scope` is a
  classification, first filter to `categoryId` / `payeeId` / `tagId` /
  `labelIds.includes(id)`. Then filter with `matchesTerms`, sort by `date`
  descending (ties: keep input order), group with `groupByDay` using
  `dayKey(tx.date)`.
- Empty query + scope `all`: no entity groups, all transactions (the
  "recent" feed — the dialog renders it progressively).

### 4. `GlobalSearchDialog`

Mounted once in `ApplicationLayout` next to `TransactionDialog`; open state in
`uiStore` (`searchOpen`, `openSearch()`, `closeSearch()`).

Shell: `ResponsiveDialog` (bottom sheet on compact, centred modal otherwise)
containing the `components/ui/command.tsx` (cmdk) primitives with
`shouldFilter={false}` — our hook does matching and ranking; cmdk provides
roving highlight, ↑/↓/Enter and listbox ARIA roles.

Views (local state, reset on close):

- **Results** (`scope = all`)
  - Search input, auto-focused, empty on every open.
  - Groups in order: Accounts, Categories, Payees, Tags, Labels — each
    hidden when empty, capped at 5 rows with a "Show all {count}" row that
    lifts the cap for that group.
  - Then Transactions, day-grouped, rendered progressively: first 100
    entries, growing by 100 as a scroll sentinel comes into view (same
    technique as the account page's `WindowedEntries`; extract it to a
    shared component if the extraction is clean, otherwise replicate the
    small hook).
  - Empty state (query typed, nothing matched): one line, "Nothing found".
- **Drill-down** (`scope = classification`)
  - Header: back arrow, the item's icon + name, and its `⋯` menu.
  - The same input keeps working and narrows within the item's
    transactions; the query is cleared on entering the drill-down.
  - Back (button, or Backspace on an empty input) returns to Results with
    the previous query restored.
  - If the item disappears (deleted / merged away) the view returns to
    Results.

### 5. Rows

- **Account row**: `EntityIcon` (account icon), name, balance
  (`moneyFormat`), folder name in muted text (so a hidden-folder account is
  identifiable), shared avatars when shared. Select → `navigate` to
  `/account/:id` and close the dialog. `⋯` → account actions (§6).
- **Classification row**: icon (categories; tags/labels/payees use their
  existing glyphs as on the settings pages), name, archived badge + dimmed
  when archived. Select → drill-down. `⋯` → classification actions (§6).
- **Transaction row**: `TransactionRow` with `pageAccount = tx.account`
  (the source account), plus a visible account line: the account name, or
  `From → To` for transfers. Amount sign for the global list: `-` expense,
  `+` income, no sign for transfers (no page account to be in or out of).
  This needs a small `TransactionRow` option (e.g. `context: 'global'`)
  that, for transfers only, titles the row with the description (falling
  back to the existing transfer type label) instead of "to X"/"from Y", and
  drops the transfer sign; the account page's rendering is unchanged. Select → `ViewTransactionDialog` with Edit (via
  `openTransactionModal`) and Delete (`ConfirmDialog` →
  `useDeleteTransaction`), `canChange = canWriteToAccount(...)`, identical to
  `AccountPage`.

### 6. Actions

- **Accounts** — the same items and rules as the account row on Settings →
  Accounts: Edit (`openAccountModal`), Access (only when
  `hasAccountAdminAccess`; refreshes accounts then opens
  `ShareAccessDialog` → `AccessLevelDialog`), Delete (owner) or Decline
  (shared). Extract the handlers and the delete/decline/access dialogs from
  `AccountsSettingsPage` into `useAccountActions()` +
  `<AccountActionDialogs />` in `features/accounts/`, used by both screens.
- **Classifications** — Edit (`CategoryDialog` / payee dialog /
  `TagDialog`), Archive or Unarchive, Merge (`MergeDialog`, candidates =
  own items of the same type, and same category type for categories),
  Delete (`ConfirmDialog`). Reuse the existing mutations in
  `classifications/queries.ts` and the dialogs as-is; the per-type wiring
  currently inline in `CategoriesPage` / `PayeesPage` / `TagsPage` is
  lifted only as far as needed to call it from the dialog without
  duplicating mutation logic.

All writes go through existing mutations, which already invalidate the
shared queries, so an open search updates in place. A read-only (402) user
sees the same refusals as elsewhere via the existing error handling.

### 7. Hotkey

A `keydown` listener registered by `ApplicationLayout` (authenticated shell
only): `(e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k'` →
`preventDefault()` and `openSearch()`. Ignored when the search is already
open, and when any other dialog is open (a `[role="dialog"]` element in the
document), so it never stacks over a transaction form. Works from inside
text inputs (it is a modifier chord, not a bare key).

### 8. `EntitySelect` matcher switch

`EntitySelect.tsx` replaces its `label.toLowerCase().includes(search)` filter
with `rankByName(options, o => o.label, search)` — skip-tolerant and ranked
(exact → prefix → substring → subsequence). The exact-match check that
decides whether to offer "create {name}" is unchanged (exact,
case-insensitive), so a fuzzy hit never suppresses creating a new item.

## Analytics

New `METRICS` keys (`web/src/lib/metrics.ts`):

- `GLOBAL_SEARCH_OPEN: 'appGlobalSearchOpen'` — fired in `openSearch()`.
- `GLOBAL_SEARCH_SELECT: 'appGlobalSearchSelect'` — `{ type }` where type ∈
  `account | category | payee | tag | label | transaction`, fired when a
  result is opened (navigate / drill-down / view dialog).

Edits, deletes, archives and merges already fire their own events through
the shared mutations.

## i18n

New `search.*` namespace in all 11 `locales/<lang>.json`: input placeholder,
group headings (accounts, categories, payees, tags, labels, transactions),
`show_all` with `{count}`, `back`, `nothing_found`, and the dialog title (for
the accessible name). Existing keys are reused for action labels and
archived badges.

## Testing

- `lib/search.test.ts`: rank order; subsequence across a skipped
  character (`grcries` → Groceries); `matchesTerms` AND semantics, word-level
  fuzzy, amounts exact (`12.50` ≠ `1250`), no cross-word subsequence
  (`cofe` ∌ "come for elections").
- `useAccountTransactions` existing tests pass unchanged after the
  extraction.
- `useGlobalSearch.test.tsx`: empty query → all transactions, no groups;
  own-only classifications; archived after active; hidden-folder and shared
  accounts included; account-name match finds transactions; drill-down scope
  per kind (including `labelIds`).
- `GlobalSearchDialog.test.tsx`: Ctrl+K and ⌘K open it; ignored while
  another dialog is open; typing narrows; Show all lifts the cap;
  classification tap drills down, back restores the query; deleting the
  drilled item returns to Results; account select navigates and closes;
  account `⋯` shows Decline for a shared account and Access only with admin
  rights; archived classification `⋯` shows Unarchive; transaction select
  opens the view dialog with Edit/Delete; ↑/↓/Enter navigation; analytics
  events fired.
- `EntitySelect.test.tsx`: a skipped character still matches; ranking puts
  the prefix match first; "create" is still offered when only fuzzy matches
  exist.
- `AccountsSettingsPage` tests guard the account-actions extraction.
- `metrics-coverage.test.ts` and the `i18ntest` guards cover the new keys.

Gates: `make web-lint`, `make web-test`, type-check. No Go change, so the Go
suites are unaffected.

## Docs

`docs/regression-test-plan.md`: a new "Global search" section (desktop and
tablet-with-keyboard: open via hotkey, empty-state feed, each result kind,
drill-down + back, each `⋯` action, hidden-folder account, archived
classification, shared account Decline, fuzzy match) and an item under the
transaction form for the skip-tolerant category/payee/tag pickers. No 📱
phone item until the icon lands.

## Out of scope

- The search icon / any visible entry point (follow-up).
- Budgets and recurring templates as results.
- Changing the account page's in-page search semantics.
- Server-side search.
