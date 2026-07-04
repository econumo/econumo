# React Web Migration — Plan 5: Connections, Access Control, Onboarding, CSV Import/Export

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the last functional surfaces of the React SPA — shared-access connections (invites, per-account and per-budget roles, accept/decline), the onboarding page, and CSV import/export — so the React app at `web-react/` is feature-complete and testable side-by-side with the Vue app.

**Architecture:** Same layering as Plans 1–4: `api/` thin typed clients over the frozen wire contract, `features/<domain>/queries.ts` TanStack Query hooks with metric tracking, feature components on shadcn primitives, pure logic in plain modules with unit tests. Connections get a new feature folder; access dialogs are shared between the accounts and budgets settings pages; CSV parsing/chunking is a pure `lib/csv.ts` module.

**Tech Stack:** React 19, TypeScript 6 strict (`erasableSyntaxOnly` — NO enums), TanStack Query v5, react-i18next (single-brace `{name}` interpolation), MSW 2 + Vitest 4 + RTL, axios.

## Scope note — the `web/` swap is EXCLUDED

Per explicit user instruction: **do NOT delete or replace `web/`, do NOT repoint the Makefile `web-*` targets or the Dockerfile.** Both apps must stay runnable side-by-side (Vue served by the Go binary at :8181, React dev server at :9000) so the user can test and give feedback. The swap becomes its own future commit after that feedback.

## Global Constraints

- No `enum` — `as const` object + union type (TS6 `erasableSyntaxOnly`).
- i18n keys already exist in `web-react/src/locales/en-US.ts` (catalog ported verbatim from Vue). Use them exactly; do not invent keys. Onboarding step titles/bodies are hardcoded English in the Vue template — port that copy verbatim as JSX text, not new i18n keys.
- Wire contract is frozen: envelope `{success, message, data}`; datetimes `Y-m-d H:i:s`; `isAccepted` is int `0/1`; roles travel as strings `admin|user|guest` (accounts) and `owner|admin|user|guest` (budgets).
- New client-side ids: `v7()` from `uuid`. (No new ids in this plan — all connection/access ops reference existing ids.)
- TDD: failing test → minimal code → green → commit. One commit per task.
- Test env: `src/test/setup.ts` handles the Node-25 localStorage rebind; every test `beforeEach` sets `window.econumoConfig = {}` and clears localStorage (follow existing test files).
- Verify each task with `cd web-react && pnpm vitest run <files>`; the final task runs the full suite + `pnpm lint` + `pnpm exec tsc -b`.
- Vue reference sources: `web/src/pages/Settings/Connections.vue`, `web/src/components/Connections/*.vue`, `web/src/components/{AccessDialogModal,AccessLevelDialogModal}.vue`, `web/src/components/Budget/{BudgetAccessModal,BudgetAccessLevelModal}.vue`, `web/src/pages/Onboarding.vue`, `web/src/components/{ImportCsvModal,ImportResultModal,ExportCsvModal,ExportCsvForm}.vue`, `web/src/stores/connections.ts`, `web/src/modules/api/v1/{connection,transaction}.ts`.
- Go wire source of truth: `internal/ui/handler/connection/*`, `internal/app/connection/*`, `internal/ui/handler/transaction/{export,import}.go`, `internal/app/transaction/{export,import,import_parse}.go`.

## Wire contract (verified against the Go source)

### Connection module — all under `/api/v1/connection/`, JWT

| Method | Path | Request | Response `data` |
|---|---|---|---|
| GET | `get-connection-list` | — | `{items: ConnectionDto[]}` |
| POST | `generate-invite` | `{}` | `{item: {code, expiredAt}}` — code is 5 chars `[0-9a-fA-F]` mixed case, expires in 5 min, one per user (regenerating refreshes) |
| POST | `accept-invite` | `{code}` | `{items: ConnectionDto[]}` (the redeemer's refreshed list). 400 on bad/expired/own code (`"ConnectionCode is incorrect"`, `"Inviting yourself?"`) |
| POST | `delete-invite` | `{}` | `{}` (exists but UNUSED by the Vue UI — do not build) |
| POST | `delete-connection` | `{id: <connectedUserId>}` | `{}` — also revokes all account+budget access both directions server-side |
| POST | `set-account-access` | `{accountId, userId, role}` role ∈ `admin\|user\|guest` | `{}` — requires caller owner-or-admin on the account, else 403 `"Access denied"` |
| POST | `revoke-account-access` | `{accountId, userId}` | `{}` (missing grant → 400) |

`ConnectionDto`: `{user: {id, avatar, name}, sharedAccounts: [{id, ownerUserId, role}]}`. `sharedAccounts` is account-centric and **not used by the UI** — the Vue UI derives shared accounts/budgets client-side from the accounts/budgets stores; we do the same from the query caches.

Budget access lives in the **budget** API (functions already exist in `src/api/budget.ts`): `grant-access {budgetId, userId, role}` → `{items}`, `revoke-access {budgetId, userId}`, `accept-access {budgetId}` → `{items}`, `decline-access {budgetId}`. Budget roles include `owner`; `BudgetAccessDto` has `isAccepted: 0|1`. Account access has NO accept step. **Declining a shared account = `delete-account`** (no dedicated endpoint; matches Vue).

### complete-onboarding

`POST /api/v1/user/complete-onboarding` (empty body) → `data: {user: CurrentUserDto}` — same refreshed-user echo as update-name. Vue quirk to replicate: **an absent `onboarding` option means COMPLETED** (only an explicit non-`completed` value shows onboarding).

### Transaction export

`GET /api/v1/transaction/export-transaction-list?accountId=<id,id,...>` (single comma-joined param; blank = all accessible). Response is **raw CSV**, not the JSON envelope: `Content-Type: text/csv; charset=UTF-8`. Header row: `transaction_id,account_name,account_currency,category,description,tag,payee,amount,date`. Expense/transfer-out amounts are negative. Client downloads as `transactions-YYYY-MM-DD.csv` (local date).

### Transaction import

`POST /api/v1/transaction/import-transaction-list`, multipart, 10 MiB cap. Fields: `file` (CSV), `mapping` (JSON string, keys `account,date,amount,amountInflow,amountOutflow,category,description,payee,tag`, values = CSV **column header names** or `null`), plus optional fixed-value overrides as separate fields: `accountId`, `date`, `categoryId`, `description`, `payeeId`, `tagId`. Response (HTTP 200 even for total failures): `data: {imported: n, skipped: n, errors: {"<message>": [rowNumbers...]}}` — rows are 1-based counting the header (data row N = N+1), row `0` = top-level error. Single-amount mode: negative = expense, non-negative = income. Dual mode: inflow column → income, outflow column → expense. The Vue client chunks files at 500 data rows, posts chunks sequentially, and remaps chunk row numbers back to original file rows: `originalRow = chunkRow + chunkIndex*500`.

## Approved divergences (log here, mirror Vue everywhere else)

1. **Accept-invite failure stays open**: Vue closes the dialog regardless of outcome; React keeps it open and shows the server message (e.g. `ConnectionCode is incorrect`) inline.
2. **No localStorage mirror** of connections (established pattern — query cache only). Polling every 5 s happens only while the Connections page is mounted, same as Vue.
3. **`pluralPick` helper** replaces vue-i18n's `|` pipe pluralization (i18next doesn't parse it); the catalog strings stay verbatim.
4. Onboarding "active area" store calls (Quasar mobile layout bookkeeping) are dropped; React's responsive layout doesn't need them.

## File structure

```
web-react/src/
├── api/
│   ├── connection.ts                    NEW  connection endpoints
│   ├── dto/connection.ts                NEW  ConnectionDto
│   ├── user.ts                          MOD  completeOnboarding returns the user
│   ├── transaction.ts                   MOD  + exportTransactionList / importTransactionList
│   └── dto/transaction.ts               MOD  + ImportResultDto
├── app/
│   ├── queryKeys.ts                     MOD  + connections
│   ├── routes.tsx                       MOD  wire /settings/connections + /onboarding
│   └── layouts/ApplicationLayout.tsx    MOD  sidebar Onboarding link
├── lib/
│   ├── csv.ts                           NEW  parse/serialize/chunk (pure)
│   ├── plural.ts                        NEW  pluralPick (pure)
│   └── download.ts                      NEW  downloadBlob + localDateStamp
├── features/connections/
│   ├── queries.ts                       NEW  useConnections + invite/access mutations
│   ├── shared.ts                        NEW  pure: derive shared accounts/budgets, access edits, hasAdminAccess
│   ├── ConnectionsPage.tsx              NEW
│   ├── GenerateInviteDialog.tsx         NEW
│   ├── AcceptInviteDialog.tsx           NEW
│   ├── PreviewConnectionDialog.tsx      NEW
│   ├── AccessLevelDialog.tsx            NEW  role picker (accounts|budgets)
│   ├── DeclineAccessDialog.tsx          NEW
│   └── ShareAccessDialog.tsx            NEW  user list w/ role labels (accounts|budgets)
├── features/accounts/AccountsSettingsPage.tsx   MOD  Access control + shared avatars
├── features/budgets/
│   ├── queries.ts                       MOD  + grant/revoke/accept/decline mutations
│   └── BudgetsPage.tsx                  MOD  Accept/Decline/Access control
├── features/transactions/
│   ├── importCsv.ts                     NEW  pure: analyze, auto-detect, form entries, upload pipeline
│   ├── ImportCsvDialog.tsx              NEW
│   ├── ImportResultDialog.tsx           NEW
│   └── ExportCsvDialog.tsx              NEW
├── features/settings/SettingsPage.tsx   MOD  Import CSV / Export CSV rows
├── features/user/queries.ts             MOD  isOnboardingCompleted default + useCompleteOnboarding
├── features/onboarding/OnboardingPage.tsx  NEW
├── features/home/HomePage.tsx           MOD  Onboarding branch
└── test/fixtures.ts                     MOD  fixtureConnections + get-connection-list handler
```

---

### Task 1: Connection API layer + complete-onboarding echo

**Files:**
- Create: `web-react/src/api/dto/connection.ts`
- Create: `web-react/src/api/connection.ts`
- Test: `web-react/src/api/connection.test.ts`
- Modify: `web-react/src/api/user.ts` (completeOnboarding), `web-react/src/api/user.test.ts`
- Modify: `web-react/src/app/queryKeys.ts` (+ `connections`)

**Interfaces:**
- Produces: `ConnectionDto { user: UserDto; sharedAccounts: {id, ownerUserId, role: AccountRole}[] }`; `InviteDto { code: string; expiredAt: string }`; functions `getConnectionList(): Promise<ConnectionDto[]>`, `generateInvite(): Promise<InviteDto>`, `acceptInvite(code: string): Promise<ConnectionDto[]>`, `deleteConnection(userId: Id): Promise<void>`, `setAccountAccess(form: {accountId: Id; userId: Id; role: AccountRole}): Promise<void>`, `revokeAccountAccess(form: {accountId: Id; userId: Id}): Promise<void>`; `completeOnboarding(): Promise<CurrentUserDto>`; `queryKeys.connections`.

- [ ] **Step 1: Write the failing test** — `src/api/connection.test.ts`, same MSW pattern as `api/budget.test.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { acceptInvite, deleteConnection, generateInvite, getConnectionList, revokeAccountAccess, setAccountAccess } from './connection'

beforeEach(() => {
  window.econumoConfig = {}
  localStorage.setItem('token', 't')
})

const wireConnection = {
  user: { id: 'u2', avatar: 'https://a/u2', name: 'Partner' },
  sharedAccounts: [{ id: 'a1', ownerUserId: 'u1', role: 'user' }],
}

it('getConnectionList unwraps items', async () => {
  server.use(http.get('*/api/v1/connection/get-connection-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [wireConnection] } })))
  expect(await getConnectionList()).toEqual([wireConnection])
})

it('generateInvite posts an empty body and unwraps item', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/connection/generate-invite', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { item: { code: 'aB3f9', expiredAt: '2026-07-03 12:05:00' } } })
  }))
  expect(await generateInvite()).toEqual({ code: 'aB3f9', expiredAt: '2026-07-03 12:05:00' })
  expect(body).toEqual({})
})

it('acceptInvite posts the code and returns the refreshed list', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/connection/accept-invite', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { items: [wireConnection] } })
  }))
  expect(await acceptInvite('aB3f9')).toEqual([wireConnection])
  expect(body).toEqual({ code: 'aB3f9' })
})

it('deleteConnection posts the user id under "id"', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/connection/delete-connection', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  await deleteConnection('u2')
  expect(body).toEqual({ id: 'u2' })
})

it('set/revoke account access post the exact payloads', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/connection/set-account-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/connection/revoke-account-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await setAccountAccess({ accountId: 'a1', userId: 'u2', role: 'user' })
  await revokeAccountAccess({ accountId: 'a1', userId: 'u2' })
  expect(bodies).toEqual([
    { accountId: 'a1', userId: 'u2', role: 'user' },
    { accountId: 'a1', userId: 'u2' },
  ])
})
```

Add to `src/api/user.test.ts`:

```ts
it('completeOnboarding returns the refreshed user', async () => {
  server.use(http.post('*/api/v1/user/complete-onboarding', () =>
    HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, options: [{ name: 'onboarding', value: 'completed' }] } } })))
  const user = await completeOnboarding()
  expect(user.options).toEqual([{ name: 'onboarding', value: 'completed' }])
})
```

- [ ] **Step 2: Run to verify failure** — `pnpm vitest run src/api/connection.test.ts src/api/user.test.ts` → FAIL (module not found / void return).

- [ ] **Step 3: Implement.** `src/api/dto/connection.ts`:

```ts
import type { Id } from '../types'
import type { UserDto } from './user'
import type { AccountRole } from './account'

// account-centric; the UI derives shared items from the accounts/budgets caches instead
export interface SharedAccountRefDto {
  id: Id
  ownerUserId: Id
  role: AccountRole
}

export interface ConnectionDto {
  user: UserDto
  sharedAccounts: SharedAccountRefDto[]
}

export interface InviteDto {
  code: string
  /** Y-m-d H:i:s; codes live 5 minutes */
  expiredAt: string
}
```

`src/api/connection.ts`:

```ts
import { api, apiUrl } from './client'
import type { Id } from './types'
import type { AccountRole } from './dto/account'
import type { ConnectionDto, InviteDto } from './dto/connection'

interface Envelope<T> {
  data: T
}

export async function getConnectionList(): Promise<ConnectionDto[]> {
  const response = await api.get<Envelope<{ items: ConnectionDto[] }>>(apiUrl('/api/v1/connection/get-connection-list'))
  return response.data.data.items
}

export async function generateInvite(): Promise<InviteDto> {
  const response = await api.post<Envelope<{ item: InviteDto }>>(apiUrl('/api/v1/connection/generate-invite'), {})
  return response.data.data.item
}

export async function acceptInvite(code: string): Promise<ConnectionDto[]> {
  const response = await api.post<Envelope<{ items: ConnectionDto[] }>>(apiUrl('/api/v1/connection/accept-invite'), { code })
  return response.data.data.items
}

// the wire field is "id" (the connected user's id)
export async function deleteConnection(userId: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/connection/delete-connection'), { id: userId })
}

export async function setAccountAccess(form: { accountId: Id; userId: Id; role: AccountRole }): Promise<void> {
  await api.post(apiUrl('/api/v1/connection/set-account-access'), form)
}

export async function revokeAccountAccess(form: { accountId: Id; userId: Id }): Promise<void> {
  await api.post(apiUrl('/api/v1/connection/revoke-account-access'), form)
}
```

In `src/api/user.ts` replace `completeOnboarding`:

```ts
export async function completeOnboarding(): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/complete-onboarding'))
  return response.data.data.user
}
```

In `src/app/queryKeys.ts` add `connections: ['connections'] as const,` after `user`.

- [ ] **Step 4: Run to verify pass** — same command → PASS.

- [ ] **Step 5: Commit** — `git add web-react/src/api web-react/src/app/queryKeys.ts && git commit -m "feat(react/connections): connection api client + complete-onboarding user echo"`

---

### Task 2: Connections queries + pure shared-access helpers

**Files:**
- Create: `web-react/src/features/connections/shared.ts`
- Create: `web-react/src/features/connections/queries.ts`
- Test: `web-react/src/features/connections/shared.test.ts`, `web-react/src/features/connections/queries.test.tsx`
- Modify: `web-react/src/test/fixtures.ts` — add `fixtureConnections` + a `get-connection-list` handler to `coreHandlers` (returns `{items: overrides.connections ?? []}`) so pages mounting `useConnections` don't hit unhandled requests.

**Interfaces:**
- Produces (`shared.ts`): `SharedItem { id: Id; name: string; icon?: string; role: string; ownedByMe: boolean; owner: UserDto }`; `sharedAccountsFor(accounts: AccountDto[], meId: Id, otherId: Id): SharedItem[]`; `sharedBudgetsFor(budgets: BudgetMetaDto[], meId: Id, otherId: Id): SharedItem[]`; `applyAccountAccess(accounts: AccountDto[], accountId: Id, user: UserDto, role: AccountRole): AccountDto[]`; `removeAccountAccess(accounts: AccountDto[], accountId: Id, userId: Id): AccountDto[]`; `hasAccountAdminAccess(account: AccountDto, meId: Id): boolean`; `hasBudgetAdminAccess(budget: BudgetMetaDto, meId: Id): boolean`.
- Produces (`queries.ts`): `useConnections(options?: {poll?: boolean})`, `useGenerateInvite()`, `useAcceptInvite()`, `useDeleteConnection()`, `useSetAccountAccess()`, `useRevokeAccountAccess()`.

- [ ] **Step 1: Write the failing tests.** `shared.test.ts` (pure, no rendering):

```ts
import { applyAccountAccess, hasAccountAdminAccess, hasBudgetAdminAccess, removeAccountAccess, sharedAccountsFor, sharedBudgetsFor } from './shared'
import { fixtureAccounts } from '@/test/fixtures'

const me = { id: 'u1', avatar: '', name: 'Me' }
const partner = { id: 'u2', avatar: '', name: 'Partner' }

const mine = { ...fixtureAccounts[0], id: 'a1', name: 'Wallet', owner: me, sharedAccess: [{ user: partner, role: 'user' as const }] }
const theirs = { ...fixtureAccounts[0], id: 'a2', name: 'Their cash', owner: partner, sharedAccess: [{ user: me, role: 'guest' as const }] }
const unshared = { ...fixtureAccounts[0], id: 'a3', owner: me, sharedAccess: [] }

it('sharedAccountsFor picks both directions with the counterparty role', () => {
  const items = sharedAccountsFor([mine, theirs, unshared], 'u1', 'u2')
  expect(items).toEqual([
    { id: 'a1', name: 'Wallet', icon: mine.icon, role: 'user', ownedByMe: true, owner: me },
    { id: 'a2', name: 'Their cash', icon: theirs.icon, role: 'guest', ownedByMe: false, owner: partner },
  ])
})

it('sharedBudgetsFor mirrors the logic over budget access', () => {
  const myBudget = { id: 'b1', ownerUserId: 'u1', name: 'Household', startedAt: '2026-01-01 00:00:00', currencyId: 'c1',
    access: [{ user: me, role: 'owner' as const, isAccepted: 1 as const }, { user: partner, role: 'user' as const, isAccepted: 0 as const }] }
  const theirBudget = { ...myBudget, id: 'b2', ownerUserId: 'u2',
    access: [{ user: partner, role: 'owner' as const, isAccepted: 1 as const }, { user: me, role: 'guest' as const, isAccepted: 1 as const }] }
  expect(sharedBudgetsFor([myBudget, theirBudget], 'u1', 'u2')).toEqual([
    { id: 'b1', name: 'Household', icon: undefined, role: 'user', ownedByMe: true, owner: me },
    { id: 'b2', name: 'Household', icon: undefined, role: 'guest', ownedByMe: false, owner: partner },
  ])
})

it('applyAccountAccess upserts, removeAccountAccess drops', () => {
  const granted = applyAccountAccess([unshared], 'a3', partner, 'admin')
  expect(granted[0].sharedAccess).toEqual([{ user: partner, role: 'admin' }])
  const changed = applyAccountAccess(granted, 'a3', partner, 'guest')
  expect(changed[0].sharedAccess).toEqual([{ user: partner, role: 'guest' }])
  expect(removeAccountAccess(changed, 'a3', 'u2')[0].sharedAccess).toEqual([])
})

it('admin access = owner or admin grant', () => {
  expect(hasAccountAdminAccess(mine, 'u1')).toBe(true)
  expect(hasAccountAdminAccess(theirs, 'u1')).toBe(false)
  expect(hasAccountAdminAccess({ ...theirs, sharedAccess: [{ user: me, role: 'admin' }] }, 'u1')).toBe(true)
  const budget = { id: 'b1', ownerUserId: 'u2', name: 'B', startedAt: '', currencyId: 'c1',
    access: [{ user: me, role: 'admin' as const, isAccepted: 1 as const }] }
  expect(hasBudgetAdminAccess(budget, 'u1')).toBe(true)
  expect(hasBudgetAdminAccess({ ...budget, access: [{ user: me, role: 'admin', isAccepted: 0 }] }, 'u1')).toBe(false)
})
```

`queries.test.tsx` (renderHook + QueryClientProvider, pattern from `features/budgets/queries.test.tsx`): assert (a) `useAcceptInvite` success replaces the `connections` cache with the returned items; (b) `useSetAccountAccess` success rewrites the `accounts` cache via `applyAccountAccess` (seed the accounts cache and the connections cache first, then check `sharedAccess` on the cached account); (c) `useRevokeAccountAccess` drops the entry; (d) `useDeleteConnection` removes the user from the connections cache.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.** `shared.ts`:

```ts
import type { AccountDto, AccountRole } from '@/api/dto/account'
import type { BudgetMetaDto } from '@/api/dto/budget'
import type { UserDto } from '@/api/dto/user'
import type { Id } from '@/api/types'

export interface SharedItem {
  id: Id
  name: string
  icon?: string
  role: string
  ownedByMe: boolean
  owner: UserDto
}

// Vue derives a connection's shared items from the accounts/budgets stores, not
// from the connection payload: my item -> the partner's role, their item -> my role.
export function sharedAccountsFor(accounts: AccountDto[], meId: Id, otherId: Id): SharedItem[] {
  const items: SharedItem[] = []
  for (const account of accounts) {
    if (account.owner.id === meId) {
      const entry = account.sharedAccess.find((a) => a.user.id === otherId)
      if (entry) items.push({ id: account.id, name: account.name, icon: account.icon, role: entry.role, ownedByMe: true, owner: account.owner })
    } else if (account.owner.id === otherId) {
      const entry = account.sharedAccess.find((a) => a.user.id === meId)
      if (entry) items.push({ id: account.id, name: account.name, icon: account.icon, role: entry.role, ownedByMe: false, owner: account.owner })
    }
  }
  return items
}

export function sharedBudgetsFor(budgets: BudgetMetaDto[], meId: Id, otherId: Id): SharedItem[] {
  const items: SharedItem[] = []
  for (const budget of budgets) {
    const owner = budget.access.find((a) => a.user.id === budget.ownerUserId)?.user
    if (!owner) continue
    if (budget.ownerUserId === meId) {
      const entry = budget.access.find((a) => a.user.id === otherId)
      if (entry) items.push({ id: budget.id, name: budget.name, icon: undefined, role: entry.role, ownedByMe: true, owner })
    } else if (budget.ownerUserId === otherId) {
      const entry = budget.access.find((a) => a.user.id === meId)
      if (entry) items.push({ id: budget.id, name: budget.name, icon: undefined, role: entry.role, ownedByMe: false, owner })
    }
  }
  return items
}

export function applyAccountAccess(accounts: AccountDto[], accountId: Id, user: UserDto, role: AccountRole): AccountDto[] {
  return accounts.map((account) => {
    if (account.id !== accountId) return account
    const rest = account.sharedAccess.filter((a) => a.user.id !== user.id)
    return { ...account, sharedAccess: [...rest, { user, role }] }
  })
}

export function removeAccountAccess(accounts: AccountDto[], accountId: Id, userId: Id): AccountDto[] {
  return accounts.map((account) =>
    account.id === accountId ? { ...account, sharedAccess: account.sharedAccess.filter((a) => a.user.id !== userId) } : account,
  )
}

export function hasAccountAdminAccess(account: AccountDto, meId: Id): boolean {
  return account.owner.id === meId || account.sharedAccess.some((a) => a.user.id === meId && a.role === 'admin')
}

export function hasBudgetAdminAccess(budget: BudgetMetaDto, meId: Id): boolean {
  if (budget.ownerUserId === meId) return true
  const entry = budget.access.find((a) => a.user.id === meId)
  return entry?.role === 'admin' && entry.isAccepted === 1
}
```

`queries.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as connectionApi from '@/api/connection'
import type { ConnectionDto } from '@/api/dto/connection'
import type { AccountDto, AccountRole } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { applyAccountAccess, removeAccountAccess } from './shared'

// poll matches Vue's 5s setInterval on the Connections page; other callers read the cache
export function useConnections(options: { poll?: boolean } = {}) {
  return useQuery({
    queryKey: queryKeys.connections,
    queryFn: connectionApi.getConnectionList,
    staleTime: TEN_MINUTES,
    refetchInterval: options.poll ? 5_000 : undefined,
  })
}

export function useGenerateInvite() {
  return useMutation({
    mutationFn: connectionApi.generateInvite,
    onSuccess: () => trackEvent(METRICS.CONNECTION_GENERATE_INVITE),
  })
}

export function useAcceptInvite() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (code: string) => connectionApi.acceptInvite(code),
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.connections, items)
      trackEvent(METRICS.CONNECTION_ACCEPT_INVITE)
    },
  })
}

export function useDeleteConnection() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (userId: Id) => connectionApi.deleteConnection(userId),
    onSuccess: (_data, userId) => {
      queryClient.setQueryData<ConnectionDto[]>(queryKeys.connections, (old) => old?.filter((c) => c.user.id !== userId))
      // the server revoked shared access both ways; refetch everything (Vue: syncStore.fetchAll)
      void queryClient.invalidateQueries()
      trackEvent(METRICS.CONNECTION_DELETE)
    },
  })
}

export function useSetAccountAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { accountId: Id; userId: Id; role: AccountRole }) => connectionApi.setAccountAccess(form),
    onSuccess: (_data, form) => {
      const connections = queryClient.getQueryData<ConnectionDto[]>(queryKeys.connections)
      const user = connections?.find((c) => c.user.id === form.userId)?.user
      if (user) {
        queryClient.setQueryData<AccountDto[]>(queryKeys.accounts, (old) =>
          old ? applyAccountAccess(old, form.accountId, user, form.role) : old)
      }
      trackEvent(METRICS.CONNECTION_UPDATE_ACCOUNT_ACCESS)
    },
  })
}

export function useRevokeAccountAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { accountId: Id; userId: Id }) => connectionApi.revokeAccountAccess(form),
    onSuccess: (_data, form) => {
      queryClient.setQueryData<AccountDto[]>(queryKeys.accounts, (old) =>
        old ? removeAccountAccess(old, form.accountId, form.userId) : old)
      trackEvent(METRICS.CONNECTION_REVOKE_ACCOUNT_ACCESS)
    },
  })
}
```

In `test/fixtures.ts` add `export const fixtureConnections = [{ user: { id: 'u2', avatar: 'https://gravatar/u2', name: 'Partner' }, sharedAccounts: [] }]` and register in `coreHandlers`: `http.get('*/api/v1/connection/get-connection-list', () => HttpResponse.json({ success: true, message: '', data: { items: overrides.connections ?? [] } }))`.

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/connections): queries + pure shared-access derivation"`

---

### Task 3: ConnectionsPage — list, generate invite, accept invite, delete

**Files:**
- Create: `web-react/src/features/connections/ConnectionsPage.tsx`, `GenerateInviteDialog.tsx`, `AcceptInviteDialog.tsx`
- Test: `web-react/src/features/connections/ConnectionsPage.test.tsx`
- Modify: `web-react/src/app/routes.tsx` — `/settings/connections` → `<ConnectionsPage />`

**Interfaces:**
- Consumes: `useConnections({poll: true})`, `useGenerateInvite`, `useAcceptInvite`, `useDeleteConnection`; `SettingsShell` (`{title, backTo, actions, children}`); `ConfirmDialog`, `ResponsiveDialog`.
- Produces: `GenerateInviteDialog({open, code, onClose})`, `AcceptInviteDialog({open, onSubmit, error, pending, onClose})` — reused nowhere else, but the page's row click / View menu wiring is extended by Task 5.

Page anatomy (Vue `Connections.vue`): title `modules.connections.pages.settings.header`, two buttons `…generate_invite` ("Create an invitation") and `…accept_invite` ("Accept an invitation"); empty state `blocks.list.list_empty`; one row per connection: avatar (`?s=50`), name, `MoreVertical` menu with View (`elements.button.view.label`) and Delete (`elements.button.delete.label`); delete → ConfirmDialog with question `modules.connections.modals.delete_connection.question` (`{name}`), confirm `elements.button.delete.label`. Generate → calls the mutation, then opens `GenerateInviteDialog` showing label `modules.connections.modals.generate_invite.code.label`, instruction `…generate_invite.instruction`, the code in large mono text, single OK button (`elements.button.ok.label`). No copy button, no expiry countdown (Vue shows none). Accept → `AcceptInviteDialog` with input labeled `modules.connections.modals.accept_invite.code.label`, instruction `…accept_invite.instruction`, required-field message `modules.connections.forms.invitation_code.validation.required_field`, Cancel + Accept (`elements.button.accept.label`); on success closes; on 400 stays open showing the server `message` (approved divergence #1).

- [ ] **Step 1: Write the failing tests** — render via `createMemoryRouter` at `/settings/connections` with `coreHandlers({connections: fixtureConnections})` (pattern: `BudgetsPage.test.tsx`). Cases:
  1. renders the row `Partner` with its avatar; empty override → `No connections found`.
  2. "Create an invitation" click → POST `generate-invite` (assert body `{}`), dialog shows `aB3f9` and the instruction text `The code is valid for 5 minutes.` (substring).
  3. "Accept an invitation" → type `aB3f9`, submit → POST body `{code: 'aB3f9'}`; on MSW success returning two items the dialog closes and the new user's name appears.
  4. Accept failure: MSW returns `HttpResponse.json({success:false, message:'ConnectionCode is incorrect', code:400, errors:{}}, {status:400})` → dialog stays open, text `ConnectionCode is incorrect` visible.
  5. Delete: open the row menu → Delete → confirm question contains `Partner` → confirm → POST `delete-connection` `{id:'u2'}`.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.** `GenerateInviteDialog` — `ResponsiveDialog` with `title={t('modules.connections.modals.generate_invite.code.label')}`, body: `<p className="text-sm text-muted-foreground">{t('modules.connections.modals.generate_invite.instruction')}</p>` + `<p className="py-3 text-center font-mono text-3xl tracking-widest" data-testid="invite-code">{code}</p>` + full-width OK button. `AcceptInviteDialog` — form with a single `Input` (label via `<Label>`), client `isNotEmpty` check surfacing the required_field message, server error paragraph (`role="alert"`), Cancel + Accept buttons (Accept `disabled={pending}`). `ConnectionsPage`:

```tsx
export function ConnectionsPage() {
  const { t } = useTranslation()
  const { data: connections = [] } = useConnections({ poll: true })
  const generateInvite = useGenerateInvite()
  const acceptInvite = useAcceptInvite()
  const deleteConnection = useDeleteConnection()
  const [invite, setInvite] = useState<InviteDto | null>(null)
  const [acceptOpen, setAcceptOpen] = useState(false)
  const [acceptError, setAcceptError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ConnectionDto | null>(null)
  // Task 5 adds: const [preview, setPreview] = useState<ConnectionDto | null>(null)
  ...
}
```

Generate handler: `generateInvite.mutate(undefined, { onSuccess: setInvite })`. Accept submit: `acceptInvite.mutate(code, { onSuccess: () => { setAcceptOpen(false); setAcceptError(null) }, onError: (e) => setAcceptError(axiosMessage(e)) })` where `axiosMessage` mirrors the extraction used in `LoginPage` (axios error → `response.data.message`, fallback generic). Rows inside `SettingsShell title={t('modules.connections.pages.settings.header')} backTo={RouterPage.SETTINGS}` with the two action buttons. Route: replace the `EmptyPage` at `/settings/connections` with `<ConnectionsPage />`.

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/connections): connections page with invite generate/accept and delete"`

---

### Task 4: AccessLevelDialog + DeclineAccessDialog

**Files:**
- Create: `web-react/src/features/connections/AccessLevelDialog.tsx`, `DeclineAccessDialog.tsx`
- Test: `web-react/src/features/connections/AccessLevelDialog.test.tsx`

**Interfaces:**
- Produces: `AccessLevelDialog({ open, kind, user, role, onSelect, onRevoke, onClose }: { open: boolean; kind: 'accounts' | 'budgets'; user: UserDto | null; role: string | null; onSelect: (role: 'guest' | 'user' | 'admin') => void; onRevoke: () => void; onClose: () => void })` and `DeclineAccessDialog({ open, owner, itemName, onDecline, onClose }: { open: boolean; owner: UserDto | null; itemName: string; onDecline: () => void; onClose: () => void })`. Consumed by Tasks 5–7.

Vue behavior: header = user avatar + name; hint `modules.connections.modals.share_access.choose_access_level`; three option rows labeled `modules.connections.{kind}.roles.{guest|user|admin}` (accounts: "View only" / "Manage transactions" / "Full control"; budgets: "View only" / "Manage budget" / "Full control"), current role highlighted; a destructive "Revoke access" row (`modules.connections.modals.share_access.revoke_access`) **only when `role` is set and not `owner`**; Cancel. DeclineAccessDialog: owner avatar + name, item name as hint, one destructive action `modules.connections.modals.decline_access.decline_access`, Cancel.

- [ ] **Step 1: Write the failing tests:** (a) accounts kind renders the three labels + hint, no revoke row when `role={null}`; (b) `role="user"` → revoke row present, clicking it fires `onRevoke`; clicking "Full control" fires `onSelect('admin')`; (c) budgets kind renders "Manage budget"; `role="owner"` → no revoke row; (d) DeclineAccessDialog shows owner name + item name, clicking "Decline access" fires `onDecline`.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement** both as `ResponsiveDialog`s. Option row: full-width button, `aria-pressed={role === value}` + highlight class `bg-accent` when active; order guest → user → admin (Vue order). Revoke row: `variant="destructive"`-styled button. `DeclineAccessDialog` body: `<img src={`${owner.avatar}?s=50`}/>`, owner name, `<p className="text-sm text-muted-foreground">{itemName}</p>`, destructive Decline button + Cancel.

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/connections): access level + decline access dialogs"`

---

### Task 5: PreviewConnectionDialog

**Files:**
- Create: `web-react/src/features/connections/PreviewConnectionDialog.tsx`
- Test: `web-react/src/features/connections/PreviewConnectionDialog.test.tsx`
- Modify: `web-react/src/features/connections/ConnectionsPage.tsx` — row click + View menu open it

**Interfaces:**
- Consumes: `sharedAccountsFor/sharedBudgetsFor`, `useAccounts`, `useBudgets`, `useUserData`, `useSetAccountAccess`, `useRevokeAccountAccess`, `useGrantBudgetAccess`/`useRevokeBudgetAccess`/`useDeclineBudgetAccess` (Task 7 — for THIS task call the api-level `grantAccess`/`revokeAccess`/`declineAccess` via the budget mutations **created here** in minimal form; see note), `useDeleteAccount` (existing, accounts decline), `AccessLevelDialog`, `DeclineAccessDialog`.
- Produces: `PreviewConnectionDialog({ open, connection, onDelete, onClose }: { open: boolean; connection: ConnectionDto | null; onDelete: (userId: Id) => void; onClose: () => void })`.

**Note on ordering:** to keep tasks self-contained, this task adds the four budget-access mutations to `features/budgets/queries.ts` (they are also what Task 7 consumes — implement them HERE, Task 7 only consumes):

```ts
export function useGrantBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; userId: Id; role: string }) => budgetApi.grantAccess(form),
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.budgets, items)
      trackEvent(METRICS.BUDGET_GRANT_ACCESS)
    },
  })
}

export function useRevokeBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; userId: Id }) => budgetApi.revokeAccess(form),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
      trackEvent(METRICS.BUDGET_REVOKE_ACCESS)
    },
  })
}

export function useAcceptBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (budgetId: Id) => budgetApi.acceptAccess(budgetId),
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.budgets, items)
      // accepting can change the default budget option + budget visibility
      void queryClient.invalidateQueries({ queryKey: queryKeys.user })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.BUDGET_ACCEPT_ACCESS)
    },
  })
}

export function useDeclineBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (budgetId: Id) => budgetApi.declineAccess(budgetId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
      trackEvent(METRICS.BUDGET_DECLINE_ACCESS)
    },
  })
}
```

Dialog anatomy (Vue `PreviewConnectionModal.vue`): title = connection user name + avatar. Section `modules.connections.modals.preview_connection.budgets` ("Shared budgets") then `.accounts` ("Shared accounts"); empty states `.budgets_empty` / `.accounts_empty`; when non-empty, hint `.tap_to_manage` ("Click to manage access"). Each row: icon (`menu_book` equivalent → lucide `BookOpen` for budgets, `EntityIcon` for accounts), name, badge `.your_budget`/`.your_account` ("Your budget"/"Your account") when `ownedByMe` else `.shared_with_you` ("Shared with you"), role label `modules.connections.{budgets|accounts}.roles.{role}`. Row click: `ownedByMe` → `AccessLevelDialog` (skip when role is `owner`); not mine → `DeclineAccessDialog`. Account decline calls `useDeleteAccount` (matches Vue — no decline endpoint); budget decline calls `useDeclineBudgetAccess`. Footer: destructive Delete (`elements.button.delete.label`) firing `onDelete(connection.user.id)` + OK.

- [ ] **Step 1: Write the failing tests** — seed `coreHandlers` with accounts/budgets fixtures that include shared entries for `u2`; render the dialog directly (QueryClientProvider + MemoryRouter). Cases:
  1. lists a "Your account" row (Wallet, role "Manage transactions") and a "Shared with you" budget row; empty caches → both `_empty` strings.
  2. clicking the owned account row opens `AccessLevelDialog`; choosing "View only" → POST `set-account-access` `{accountId:'a1', userId:'u2', role:'guest'}`.
  3. clicking a shared-with-me budget row opens `DeclineAccessDialog`; Decline → POST `decline-access` `{budgetId:'b2'}`.
  4. clicking a shared-with-me account row → Decline posts `delete-account` for that id.
  5. Delete button fires `onDelete('u2')`.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement** the dialog + wire into `ConnectionsPage` (`View` menu item and row click set `preview`; `onDelete` closes preview and opens the existing delete confirm).

- [ ] **Step 4: Run to verify pass** (page test file re-run too).

- [ ] **Step 5: Commit** — `git commit -m "feat(react/connections): preview connection dialog with per-item access management"`

---

### Task 6: Account access control in Settings → Accounts

**Files:**
- Create: `web-react/src/features/connections/ShareAccessDialog.tsx`
- Test: `web-react/src/features/connections/ShareAccessDialog.test.tsx`
- Modify: `web-react/src/features/accounts/AccountsSettingsPage.tsx` (+ its test file)

**Interfaces:**
- Produces: `ShareAccessDialog({ open, title, kind, entries, onPick, onClose }: { open: boolean; title: string; kind: 'accounts' | 'budgets'; entries: ShareEntry[]; onPick: (entry: ShareEntry) => void; onClose: () => void })` where `ShareEntry = { user: UserDto; role: string | null; isAccepted?: boolean }`. Also a pure builder in `shared.ts`: `buildShareEntries(connections: ConnectionDto[], access: {user: UserDto; role: string; isAccepted?: 0|1}[], meId: Id, ownerUserId: Id): ShareEntry[]` — seeds every connection user (role `'owner'` if they own the item, else `null`), overlays access entries (role + `isAccepted === 1`), excludes `meId`.
- Consumed by: this task (accounts) and Task 7 (budgets).

Dialog copy: note `modules.connections.modals.share_access.tap_to_share` ("Click on a user to share access"), empty `…share_access.list_empty`, row role text `modules.connections.{kind}.roles.{role}` or `…roles.no_access` when null; budgets kind appends ` – ` + `modules.connections.modals.share_access.not_accepted` when the entry has a role and `isAccepted === false`. OK button closes.

AccountsSettingsPage additions (desktop menu): a new item **Access control** (`pages.settings.accounts.list_actions.access`) between Edit and Delete, rendered only when `hasAccountAdminAccess(account, user.id)` (import from `features/connections/shared.ts`; `econumoPackage.includesSharedAccess` is hardcoded `true` in Vue — skip the gate). Opens `ShareAccessDialog` with `buildShareEntries(connections, account.sharedAccess, meId, account.owner.id)`; `onPick` (bail when `role === 'owner'`) opens `AccessLevelDialog kind="accounts"`; select → `useSetAccountAccess`, revoke → `useRevokeAccountAccess`. Also add the shared-avatar cluster to `AccountRow`: when `account.sharedAccess.length > 0`, render owner avatar + each grantee avatar (`?s=30`, `size-4 rounded-full`) before the balance.

- [ ] **Step 1: Write the failing tests.** `ShareAccessDialog.test.tsx`: (a) `buildShareEntries` seeds connections at role null, overlays access, excludes me, marks the owner; (b) dialog renders "No access" for null role and "Manage transactions – not accepted" style suffix only for budgets kind with `isAccepted: false`; (c) clicking a row fires `onPick`. Page test additions: with a shared account fixture, the row shows the partner avatar; menu shows "Access control"; picking the partner then "Full control" posts `set-account-access` with `role:'admin'`; "Revoke access" posts `revoke-account-access`.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement** (dialog + `buildShareEntries` + page wiring).

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/accounts): access control dialog + shared avatars in accounts settings"`

---

### Task 7: Budget access control in Settings → Budgets

**Files:**
- Modify: `web-react/src/features/budgets/BudgetsPage.tsx`, `web-react/src/features/budgets/BudgetsPage.test.tsx`

**Interfaces:**
- Consumes: `useAcceptBudgetAccess`, `useDeclineBudgetAccess`, `useGrantBudgetAccess`, `useRevokeBudgetAccess` (Task 5), `ShareAccessDialog` + `buildShareEntries` (Task 6), `AccessLevelDialog` (Task 4), `useConnections` (no poll), `hasBudgetAdminAccess` (Task 2), existing `ConfirmDialog`/error dialog.

Menu items per row (Vue order, existing items kept):
1. **Accept** (`elements.button.accept.label`) when `!accepted` → `useAcceptBudgetAccess.mutate(budget.id)`.
2. **Go to the budget** (existing) when accepted.
3. **Access control** (`modules.budget.page.settings.list_actions.access`) when `hasBudgetAdminAccess(budget, user.id)` → `ShareAccessDialog kind="budgets"` with `buildShareEntries(connections, budget.access, user.id, budget.ownerUserId)`; pick → `AccessLevelDialog kind="budgets"`; select → `useGrantBudgetAccess {budgetId, userId, role}` (error → the existing generic-error ResponsiveDialog); revoke → `useRevokeBudgetAccess`.
4. **Decline** (`elements.button.decline.label`, destructive) when `budget.ownerUserId !== user.id` → ConfirmDialog title `modules.budget.page.settings.decline_access_modal.title`, question `…decline_access_modal.question` (`{name}`), confirm label `elements.button.decline.label` → `useDeclineBudgetAccess.mutate(budget.id)`.
5. **Delete** (existing) — change its gate from `ownerUserId === user.id` to `hasBudgetAdminAccess(budget, user.id)` (Vue gates delete on admin access, not ownership).

- [ ] **Step 1: Write the failing tests** (extend `BudgetsPage.test.tsx`; fixtures: one owned budget with a partner grant, one not-accepted incoming budget `{role:'user', isAccepted:0}` owned by `u2`):
  1. not-accepted row menu shows Accept; clicking posts `accept-access {budgetId}` and the returned items land in the list (subtitle "…not accepted" disappears).
  2. incoming row menu shows Decline; confirm question contains the budget name; confirm posts `decline-access`.
  3. owned row menu shows Access control; picking the partner + "Manage budget" posts `grant-access {budgetId, userId:'u2', role:'user'}`; Revoke access posts `revoke-access`.
  4. Delete is offered on an admin-shared (accepted) budget, not only owned.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/budgets): accept/decline + access control on the budgets settings page"`

---

### Task 8: CSV + plural pure helpers

**Files:**
- Create: `web-react/src/lib/csv.ts`, `web-react/src/lib/plural.ts`
- Test: `web-react/src/lib/csv.test.ts`, `web-react/src/lib/plural.test.ts`

**Interfaces:**
- Produces: `parseCsvLine(line: string): string[]` (quote-aware, `""` escape); `parseCsv(text: string): { header: string[]; rows: string[][] }` (strips UTF-8 BOM, skips blank lines, ragged rows tolerated); `buildCsvText(header: string[], rows: string[][]): string` (quotes cells containing `,`/`"`/newline, `"` → `""`, `\n` line ends); `chunkRows<T>(rows: T[], size: number): T[][]`; `remapRow(chunkRow: number, chunkIndex: number, chunkSize: number): number` = `chunkRow === 0 ? 0 : chunkRow + chunkIndex * chunkSize`; `pluralPick(catalogValue: string, count: number): string` — splits on `' | '`, picks index `count === 1 ? 0 : last`, replaces `{count}`.

- [ ] **Step 1: Write the failing tests:**

```ts
import { buildCsvText, chunkRows, parseCsv, parseCsvLine, remapRow } from './csv'
import { pluralPick } from './plural'

it('parseCsvLine handles quotes, escaped quotes, and commas in cells', () => {
  expect(parseCsvLine('a,b,c')).toEqual(['a', 'b', 'c'])
  expect(parseCsvLine('"a,x",b')).toEqual(['a,x', 'b'])
  expect(parseCsvLine('"he said ""hi""",2')).toEqual(['he said "hi"', '2'])
  expect(parseCsvLine('a,,c')).toEqual(['a', '', 'c'])
})

it('parseCsv strips BOM and blank lines', () => {
  const { header, rows } = parseCsv('﻿Date,Amount\n2026-01-02,-5\n\n2026-01-03,7\n')
  expect(header).toEqual(['Date', 'Amount'])
  expect(rows).toEqual([['2026-01-02', '-5'], ['2026-01-03', '7']])
})

it('buildCsvText round-trips through parseCsv', () => {
  const text = buildCsvText(['a', 'b'], [['x,1', 'he said "hi"'], ['plain', '2']])
  expect(parseCsv(text)).toEqual({ header: ['a', 'b'], rows: [['x,1', 'he said "hi"'], ['plain', '2']] })
})

it('chunkRows splits at the boundary; remapRow restores original file rows', () => {
  expect(chunkRows([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]])
  expect(remapRow(3, 0, 500)).toBe(3)   // chunk 0: unchanged
  expect(remapRow(2, 1, 500)).toBe(502) // chunk 1 data row 1 = file row 502 (header-inclusive numbering)
  expect(remapRow(0, 4, 500)).toBe(0)   // top-level errors stay 0
})

it('pluralPick picks the vue-i18n pipe variant', () => {
  const s = '{count} transaction(s) imported | {count} transactions imported'
  expect(pluralPick(s, 1)).toBe('1 transaction(s) imported')
  expect(pluralPick(s, 3)).toBe('3 transactions imported')
  expect(pluralPick('no pipes {count}', 5)).toBe('no pipes 5')
})
```

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement** — `parseCsvLine` as a char loop with an `inQuotes` flag (port of Vue's `parseCSVLine` in `ImportCsvModal.vue`); `parseCsv` splits on `/\r?\n/`; `pluralPick`:

```ts
export function pluralPick(catalogValue: string, count: number): string {
  const variants = catalogValue.split(' | ')
  const chosen = count === 1 ? variants[0] : variants[variants.length - 1]
  return chosen.replaceAll('{count}', String(count))
}
```

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/lib): csv parse/serialize/chunk + vue-i18n plural pick"`

---

### Task 9: CSV export — API, dialog, hub row

**Files:**
- Create: `web-react/src/lib/download.ts`, `web-react/src/features/transactions/ExportCsvDialog.tsx`
- Test: `web-react/src/features/transactions/ExportCsvDialog.test.tsx` (+ export case in `src/api/transaction.test.ts`)
- Modify: `web-react/src/api/transaction.ts`, `web-react/src/features/settings/SettingsPage.tsx`

**Interfaces:**
- Produces: `exportTransactionList(accountIds: Id[]): Promise<Blob>`; `downloadBlob(blob: Blob, filename: string): void`; `localDateStamp(d?: Date): string` ("YYYY-MM-DD", local); `ExportCsvDialog({ open, onClose }: { open: boolean; onClose: () => void })` (self-contained: reads accounts + user itself).

API (in `api/transaction.ts`):

```ts
export async function exportTransactionList(accountIds: Id[]): Promise<Blob> {
  const response = await api.get<Blob>(apiUrl('/api/v1/transaction/export-transaction-list'), {
    params: { accountId: accountIds.join(',') },
    responseType: 'blob',
  })
  return response.data
}
```

`lib/download.ts`:

```ts
export function localDateStamp(d: Date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
```

Dialog (Vue `ExportCsvModal` + `ExportCsvForm`): header `modules.export_csv.modal.export_csv_form.header`, section label `…accounts` ("Select Accounts"), select-all toggle text `…select_all`/`…deselect_all`, one row per account (EntityIcon, name, `moneyFormat` balance, owner name in muted italic when any account is shared, shadcn `Checkbox`), Cancel + Export (`elements.button.export.label`, disabled when none selected). Default selection = accounts owned by the current user. Submit: `exportTransactionList(selected)` → `downloadBlob(blob, \`transactions-${localDateStamp()}.csv\`)` → close. Failure → `FailDialog` with `pages.settings.export_csv.error`.

Settings hub: add two `MenuRow`s after the Sync row — `pages.settings.import_csv.menu_item` (opens `ImportCsvDialog`, wired in Task 11; for THIS task render the row with a `onClick` that sets `importOpen`, and mount nothing yet — add the row itself in Task 11 to keep this task green) — **so in this task add ONLY the Export row**: `MenuRow label={t('pages.settings.export_csv.menu_item')} onClick={() => setExportOpen(true)}` + `<ExportCsvDialog open={exportOpen} onClose={() => setExportOpen(false)} />`.

- [ ] **Step 1: Write the failing tests:** api test asserts the GET url contains `accountId=a1%2Ca2` (or use `request.url` param check) and `responseType` behavior (MSW returns `new HttpResponse('id,...\n', { headers: { 'Content-Type': 'text/csv' } })`; assert the resolved value is a Blob whose text starts with `id`). Dialog test: default selection = owned accounts only (shared account row unchecked); "Select All" checks all and flips to "Deselect All"; Export click fires the GET with the selected ids (spy via MSW) and calls `downloadBlob` (mock `URL.createObjectURL`/`revokeObjectURL`, assert an `<a download="transactions-...csv">` click — spy on `HTMLAnchorElement.prototype.click`). Hub test: the Export CSV row opens the dialog.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/transactions): csv export dialog + settings hub row"`

---

### Task 10: CSV import — analysis module + mapping dialog UI

**Files:**
- Create: `web-react/src/features/transactions/importCsv.ts`, `web-react/src/features/transactions/ImportCsvDialog.tsx`
- Test: `web-react/src/features/transactions/importCsv.test.ts`, `web-react/src/features/transactions/ImportCsvDialog.test.tsx`
- Modify: `web-react/src/api/transaction.ts`, `web-react/src/api/dto/transaction.ts`

**Interfaces:**
- Produces (`importCsv.ts` — pure):
  - `analyzeCsv(text: string): CsvAnalysis` where `CsvAnalysis = { header: string[]; rows: string[][]; samples: Record<string, string> }` — first non-empty sample per column from ≤500 rows, truncated at 25 chars.
  - `FieldModes = { account: 'csv_column' | 'existing'; date: 'csv_column' | 'manual'; category: 'csv_column' | 'existing'; description: 'csv_column' | 'manual'; payee: 'csv_column' | 'existing'; tag: 'csv_column' | 'existing' }`, `AmountMode = 'single' | 'dual'`.
  - `ImportSelection` = `{ modes: FieldModes; amountMode: AmountMode; columns: { account: string|null; date: string|null; amount: string|null; amountInflow: string|null; amountOutflow: string|null; category: string|null; description: string|null; payee: string|null; tag: string|null }; fixed: { accountId: string|null; date: string; categoryId: string|null; description: string; payeeId: string|null; tagId: string|null } }`.
  - `autoDetect(header: string[], labels: Record<keyof ImportSelection['columns'], string>): Partial<ImportSelection>` — lowercase substring match of header names against translated labels; both inflow+outflow matched → `amountMode: 'dual'`.
  - `selectionValid(sel: ImportSelection): boolean` — account (column or fixed), date (column or manual matching `/^\d{4}-\d{2}-\d{2}$/`), and amount(s) per mode.
  - `buildImportPayload(sel: ImportSelection): { mapping: Record<string, string|null>; fields: Record<string, string> }` — mapping always has all 9 keys (column name or null; fields in existing/manual mode contribute `null` to mapping); `fields` holds only truthy fixed values keyed `accountId|date|categoryId|description|payeeId|tagId` (trimmed).
- Produces (`api/transaction.ts`): `importTransactionList(file: File, mapping: Record<string, string|null>, fields: Record<string, string>): Promise<ImportResultDto>` — builds `FormData` (`file`, `mapping` = `JSON.stringify(mapping)`, then each field), POST, returns `response.data.data`. `ImportResultDto = { imported: number; skipped: number; errors: Record<string, number[]> }` in `dto/transaction.ts`.
- Produces (`ImportCsvDialog.tsx`): `ImportCsvDialog({ open, onClose, onComplete }: { open: boolean; onClose: () => void; onComplete: (result: AggregatedImportResult) => void })` — `AggregatedImportResult` defined in Task 11's pipeline section of `importCsv.ts` (declare it in THIS task: `{ imported: number; failed: number; errors: { message: string; rows: number[] }[] }`).

Dialog UI (Vue `ImportCsvModal.vue`): header `modals.import_csv.header`; file input `accept=".csv"`, client-side max 10 MB (10485760) with hint `modals.import_csv.file.hint`; selected file shows name + Change button (`elements.button.change.label`) that clears state. Once parsed: description `modals.import_csv.mapping.description`; per-field rows with a mode-toggle icon button (tooltips `modals.import_csv.switch_to_manual` / `switch_to_csv`): Account* (existing-account `Select`), Date* (manual input placeholder `YYYY-MM-DD`), Amount* single/dual toggle (tooltips `modals.import_csv.amount_mode.switch_to_dual` / `switch_to_single`; dual shows Amount (Inflow)* + Amount (Outflow)* column selects), Category / Description / Payee / Tag optional with a "None" option (`modals.import_csv.none`). Column options labeled `Name ("sample")`. Existing-account options exclude accounts where my role is `guest`; label `` `${name} (${balance} ${code})` ``. `targetUserId` = selected fixed account's owner id, else me; the category/payee/tag existing-entity selects filter to `ownerUserId === targetUserId` and reset when it changes. Import button disabled until `selectionValid`.

- [ ] **Step 1: Write the failing tests.** `importCsv.test.ts` (pure): analyzeCsv samples/truncation; autoDetect maps `Account,Date,Amount,Category,Note` headers (label match) and flips to dual on `In`/`Out` columns matched via inflow/outflow labels; selectionValid false without account, true with fixed accountId + manual valid date + single amount column; buildImportPayload emits all 9 mapping keys with nulls for fixed-mode fields and only truthy `fields`. API test: MSW handler reads `await request.formData()`, asserts `file` name, `mapping` JSON, `accountId` field; returns `{imported: 2, skipped: 0, errors: {}}` envelope. Dialog test: select a file (`new File([csvText], 'import.csv', {type:'text/csv'})` via `userEvent.upload`), mapping section appears with auto-detected column labels; toggling account to fixed mode shows the account select; Import disabled until valid.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement** (`file.text()` needs no mock in jsdom29; parse via `lib/csv.ts`).

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/transactions): csv import analysis module + mapping dialog"`

---

### Task 11: CSV import — chunked upload pipeline, result dialog, hub wiring

**Files:**
- Modify: `web-react/src/features/transactions/importCsv.ts`, `ImportCsvDialog.tsx`, `web-react/src/features/settings/SettingsPage.tsx`
- Create: `web-react/src/features/transactions/ImportResultDialog.tsx`
- Test: extend `importCsv.test.ts`, `ImportCsvDialog.test.tsx`; create `ImportResultDialog.test.tsx`

**Interfaces:**
- Produces (`importCsv.ts`): `runImport(analysis: CsvAnalysis, selection: ImportSelection, post: (file: File, mapping: Record<string,string|null>, fields: Record<string,string>) => Promise<ImportResultDto>, onProgress?: (done: number, total: number) => void): Promise<AggregatedImportResult>` — chunks `analysis.rows` at `CHUNK_SIZE = 500`, serializes each chunk with the header via `buildCsvText`, wraps in `new File([text], \`chunk_${i}.csv\`, { type: 'text/csv' })`, posts **sequentially**; aggregates `imported`, `failed` (= skipped sum + full rows of failed chunks), and an error map keyed by message with row numbers remapped via `remapRow(row, i, CHUNK_SIZE)`; a rejected chunk adds `Chunk ${i + 1} failed: ${message}` with its row count added to failed. Returns `{ imported, failed, errors: [{message, rows}] }`.
- Produces: `ImportResultDialog({ open, result, onClose }: { open: boolean; result: AggregatedImportResult | null; onClose: () => void })`.
- Consumes: `queryClient.invalidateQueries()` after a finished import (Vue: `syncStore.fetchAll()`).

Result dialog copy (Vue `ImportResultModal.vue`): outcome by `failed === 0` → title `modals.import_result.success_title`; else `imported > 0` → `partial_success_title`; else `error_title`. Stats: `pluralPick(t('modals.import_result.imported'), imported)` when `imported > 0`; same for `failed`. Errors block titled `modals.import_result.errors_detail`, first **5** groups; row formatting: 1 row → `t('modals.import_result.row') + ' ' + n`; ≤10 → `rows` + comma list; >10 → first 10 + `+N ${t('modals.import_result.more')}`; >5 groups → `t('modals.import_result.and_more', {count: extra})`. OK closes.

Dialog wiring: Import button → `runImport(analysis, selection, importTransactionList, setProgress)`; progress bar (shadcn `Progress`) only when >1 chunk; on completion `queryClient.invalidateQueries()`, close self, then `onComplete(result)`. Settings hub: add the `pages.settings.import_csv.menu_item` row before Export; parent holds `importOpen` + `importResult` state and renders `<ImportResultDialog>` when `onComplete` delivers a result (no 100 ms setTimeout needed — sequential React state is fine; keep order: import dialog closes, result dialog opens).

- [ ] **Step 1: Write the failing tests.** `runImport` (pure, fake `post`): 1200-row analysis → 3 posts with files `chunk_0.csv…chunk_2.csv` each ≤500 rows + header; per-chunk results `{imported, skipped, errors: {'Invalid date format \'x\'': [3]}}` aggregate with the chunk-2 error surfacing as row `503`; a `post` rejection for chunk 2 yields a `Chunk 2 failed:` error entry and 500 failed rows; onProgress called (1,3)(2,3)(3,3). `ImportResultDialog`: success/partial/failure titles; "3 transactions imported"; row list formatting for 1, ≤10 and >10 rows; `and {count} more error(s)` when 6 groups. Dialog integration: happy path with a 2-row file → one POST, result dialog shows "Import Successful", transactions query invalidated (spy on `invalidateQueries`). Hub: Import CSV row opens the dialog.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run to verify pass.**

- [ ] **Step 5: Commit** — `git commit -m "feat(react/transactions): chunked csv import pipeline + result dialog + hub rows"`

---

### Task 12: Onboarding page

**Files:**
- Create: `web-react/src/features/onboarding/OnboardingPage.tsx`
- Test: `web-react/src/features/onboarding/OnboardingPage.test.tsx`
- Modify: `web-react/src/features/user/queries.ts` (isOnboardingCompleted default + `useCompleteOnboarding`), `web-react/src/features/user/queries.test.tsx`, `web-react/src/features/home/HomePage.tsx` (+ test), `web-react/src/app/routes.tsx` (`/onboarding`), `web-react/src/app/layouts/ApplicationLayout.tsx` (+ test — sidebar link)

**Interfaces:**
- Consumes: `useAccounts`, `useFolders`, `useTransactions`, `useCategories`, `usePayees`, `useTags`, `useConnections()` (no poll), `useBudgets`, `useUserData`, `useUiStore` (`openAccountModal({folderId})`), `ImportCsvDialog` + `ImportResultDialog` (Tasks 10–11), `RouterPage.BUDGET`.
- Produces: `useCompleteOnboarding()` — mutation calling `userApi.completeOnboarding()`, onSuccess `queryClient.setQueryData(queryKeys.user, user)` + `trackEvent(METRICS.USER_COMPLETE_ONBOARDING)`.

**Behavioral fix (Vue parity):** `isOnboardingCompleted` must treat an **absent** onboarding option as completed:

```ts
export function isOnboardingCompleted(user: CurrentUserDto | undefined): boolean {
  if (!user) return false
  const option = user.options.find((o) => o.name === UserOptions.ONBOARDING)
  return option === undefined || option.value === 'completed'
}
```

Page content — a vertical step list (styled as a timeline: left rail with a circle per step, `Check` icon in primary color when done, muted otherwise). Heading `modules.user.pages.onboarding.header` (title bar) and `…onboarding.title` ("Welcome to Econumo!") as the list heading. Steps, copy **verbatim from `web/src/pages/Onboarding.vue`** (router-links → `<Link to={RouterPage.*}>` with the same i18n link texts; external links `target="_blank"`):

1. **Add your accounts** — done when `accounts.length > 0`. Subtitle link `modules.user.pages.onboarding.user_guide.accounts` → `https://econumo.com/docs/user-guide/accounts`. Body: "To start, you can add an account by clicking the button below. Alternatively, you can always navigate to the {Settings} -> {Accounts} page to manage your accounts and arrange them into folders." Button `…onboarding.add_account` → `openAccountModal({ folderId: firstFolder?.id ?? null })`.
2. **Enter your first transaction** — done when `accounts.length > 0 && categories.length > 0 && transactions.length > 0`. Guide `…user_guide.transactions` → `…/user-guide/transactions`. Body: "You can enter transactions by selecting any account in the left sidebar and clicking the **Add Transaction** button.\nYou can create categories, tags, and payees directly from the transaction modal by entering their names and pressing Enter." Button `…onboarding.import_transactions` → opens `ImportCsvDialog`.
3. **Manage categories, tags, and payees** — done when `categories.length > 0 && (tags.length > 0 || payees.length > 0)`. Guide `…user_guide.classifications`. Body: "To manage categories, tags, and payees, navigate to {Settings} -> {Categories}, {Tags}, or {Payees}. You can also sort or archive them as necessary."
4. **Update your avatar** — never "done"; circle shows the user's Gravatar (`${user.avatar}?s=30`). Guide `…user_guide.user_profile`. Body: "Econumo pulls your avatar from [Gravatar](https://gravatar.com), linked to your email address. To change your avatar, please visit [Gravatar](https://gravatar.com)."
5. **Connect with your partner** — done when `connections.length > 0`. Guide `…user_guide.shared_access`. Body: "To connect with your partner and manage shared access to your budget or accounts, please visit {Settings} -> {Shared access}."
6. **Create your budget** — done when `budgets.length > 0`. Guide `…user_guide.budgets`. Body: "You can create your first budget on the {Budget} page.\nAdditionally, you can access the {Settings} -> {Budgets} page to manage your budgets, shared access, and more." Button `…onboarding.complete` ("Complete onboarding") → `useCompleteOnboarding().mutate(undefined, { onSuccess: () => navigate(RouterPage.BUDGET) })`.

HomePage: replace the placeholder branch with `<OnboardingPage />`. Route `/onboarding` → `<OnboardingPage />`. ApplicationLayout sidebar: add the Onboarding link (`blocks.main.onboarding`, `NavLink to={RouterPage.ONBOARDING}`) directly above the Budget link, rendered only when `!isOnboardingCompleted(user)`.

- [ ] **Step 1: Write the failing tests.**
  - `queries.test.tsx`: `isOnboardingCompleted` true for absent option, true for `completed`, false for any other value.
  - `OnboardingPage.test.tsx` (coreHandlers with a NOT-completed user: `options` onboarding value `''`): (a) all six step titles render + "Welcome to Econumo!"; with fixture data present, steps 1–3 and 6 show done checks; (b) "Add an account" opens the account modal (uiStore state asserted); (c) "Complete onboarding" posts to `complete-onboarding`, the user cache gets the completed option, and navigation lands on `/budget` (render a `/budget` stub route).
  - `HomePage.test.tsx` (or extend `BudgetPage.test.tsx`'s HomePage case): non-onboarded user at `/` renders "Welcome to Econumo!"; onboarded renders the budget (existing test keeps passing).
  - ApplicationLayout test: sidebar shows "Onboarding" for a non-onboarded user and hides it once completed.

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run to verify pass** — plus re-run the whole `features/home` + layout suites.

- [ ] **Step 5: Commit** — `git commit -m "feat(react/onboarding): onboarding page, home branch, sidebar link, complete flow"`

---

### Task 13: Full verification + browser parity walk (NO web/ swap)

**Files:** none created — verification, plan checkboxes, memory, push. **Do NOT touch `web/`, `Makefile` web-* targets, or `deployment/docker/Dockerfile`.**

- [ ] **Step 1:** `cd web-react && pnpm vitest run` → all green; `pnpm lint`; `pnpm exec tsc -b`.
- [ ] **Step 2:** Start the backend from the repo root: `PORT=8181 DATABASE_URL="sqlite://<scratchpad>/parity2.sqlite" go run ./cmd/econumo serve` (kill any stale listener on :8181 first: `lsof -ti :8181 | xargs kill`). Start React dev: `cd web-react && pnpm dev` (:9000). Existing user: `parity2@example.test` / `finalpass99`.
- [ ] **Step 3: Connections walk (needs a second user).** Register `parity5@example.test` / `parity5pass1` via the React register page (or `go run ./cmd/econumo user:create Parity5 parity5@example.test parity5pass1`). As parity2: Settings → Shared access → Create an invitation → note the 5-char code. As parity5 (second browser tab/profile or logout/login): Accept an invitation → enter code → connection appears both sides (poll within 5 s). As parity2: Accounts settings → Wallet → Access control → grant parity5 "Manage transactions" → avatars appear on the row; check the Vue app at :8181 shows the same sharedAccess. Budgets settings → Household → Access control → grant "Manage budget"; as parity5 the budget row shows "Regular access - not accepted" → Accept → row activates. Then as parity5 Decline one grant; as parity2 revoke the other; finally delete the connection and confirm cleanup.
- [ ] **Step 4: CSV walk.** As parity2: Settings → Export CSV → both accounts selected by default → file `transactions-<today>.csv` downloads and matches the Vue export byte-for-byte (same endpoint). Import: prepare a 3-row CSV (`Account,Date,Amount,Category,Note` incl. one bad date), import with auto-detected mapping → result dialog "Import Partially Successful", 2 imported / 1 failed with `Invalid date format` row detail; transaction list refreshes.
- [ ] **Step 5: Onboarding walk.** Register a fresh `parity6@example.test` → lands on onboarding at `/`; sidebar shows Onboarding; add an account via the step button; Complete onboarding → redirected to Budget, sidebar link gone; `/onboarding` still reachable directly.
- [ ] **Step 6:** Check off all plan checkboxes, update the `react-web-migration` memory file (Plan 5 done; note the swap remains deferred pending user feedback), `git push`.

---

## Plan sequence

- Plan 1 — foundation + auth ✅
- Plan 2 — app shell + accounts + transactions ✅
- Plan 3 — settings cluster ✅
- Plan 4 — budget page ✅
- **Plan 5 (this) — connections + access control + onboarding + CSV import/export. Swap EXCLUDED.**
- Plan 6 (future, after user testing/feedback) — the `web/` swap: delete the Vue app, repoint Makefile `web-*` targets + Dockerfile, final smoke.
