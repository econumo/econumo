# Mobile Share-Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make account sharing reachable on mobile: an access row inside the account edit modal (every surface) and a manageable access list inside the Settings→Accounts preview sheet.

**Architecture:** Pure frontend change in `web/` (React 19 + Vite SPA). Reuses the existing `ShareAccessDialog` → `AccessLevelDialog` flow, `useConnections`/`useSetAccountAccess`/`useRevokeAccountAccess` hooks, and `buildShareEntries`/`hasAccountAdminAccess` helpers. The person-row markup is extracted from `ShareAccessDialog` into a shared `ShareEntryList` component used by both the dialog and the preview sheet.

**Tech Stack:** React 19, TypeScript, TanStack Query, vitest + Testing Library + msw, oxlint. Spec: `docs/superpowers/specs/2026-07-11-mobile-share-access-design.md`.

## Global Constraints

- Work in the worktree at `/home/dmitry/dev/econumo/econumo/.claude/worktrees/fancy-marinating-pearl`, branch `feat/mobile-share-access`. First command of every session: `cd /home/dmitry/dev/econumo/econumo/.claude/worktrees/fancy-marinating-pearl/web && git branch --show-current` and verify it prints `feat/mobile-share-access`.
- No backend changes, no new API endpoints, no new i18n keys. Labels reuse: `pages.settings.accounts.list_actions.access` = "Access control", `modules.connections.accounts.roles.*` (Owner / Full control / Manage transactions / View only / No access), `modules.connections.modals.share_access.list_empty` = "No connections found".
- Comments sparingly: only non-obvious rationale (per repo CLAUDE.md). No comments that restate the code.
- Run tests with `pnpm vitest run <path>` from `web/`. Do NOT run `pnpm install` (broken under pnpm 11 on this machine); node_modules is already installed.
- The access-management guard is always `hasAccountAdminAccess(account, user.id)` (owner or `admin` role) — never invent a different predicate.

---

### Task 1: Extract `ShareEntryList` from `ShareAccessDialog`

**Files:**
- Create: `web/src/features/connections/ShareEntryList.tsx`
- Create: `web/src/features/connections/ShareEntryList.test.tsx`
- Modify: `web/src/features/connections/ShareAccessDialog.tsx`

**Interfaces:**
- Consumes: `ShareEntry` from `web/src/features/connections/shared.ts` (`{ user: UserDto; role: string | null; isAccepted?: boolean }`).
- Produces: `ShareEntryList({ kind, entries, onPick })` — `kind: 'accounts' | 'budgets'`, `entries: ShareEntry[]`, `onPick?: (entry: ShareEntry) => void`. When `onPick` is omitted the rows render as plain (non-button) read-only rows. Tasks 3 depends on this exact signature.

- [ ] **Step 1: Write the failing test**

Create `web/src/features/connections/ShareEntryList.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShareEntryList } from './ShareEntryList'
import type { ShareEntry } from './shared'

const entries: ShareEntry[] = [
  { user: { id: 'u2', name: 'Partner', avatar: 'pets:sky' }, role: 'admin' },
  { user: { id: 'u3', name: 'Newcomer', avatar: 'face:emerald' }, role: null },
]

it('renders role labels and fires onPick for tappable rows', async () => {
  const onPick = vi.fn()
  const user = userEvent.setup()
  render(<ShareEntryList kind="accounts" entries={entries} onPick={onPick} />)
  expect(screen.getByText('Full control')).toBeInTheDocument()
  expect(screen.getByText('No access')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /Partner/ }))
  expect(onPick).toHaveBeenCalledWith(entries[0])
})

it('renders read-only rows when onPick is omitted', () => {
  render(<ShareEntryList kind="accounts" entries={entries} />)
  expect(screen.getByText('Partner')).toBeInTheDocument()
  expect(screen.getByText('Newcomer')).toBeInTheDocument()
  expect(screen.queryByRole('button')).toBeNull()
})
```

Note: the `UserDto` fields used are `{ id, name, avatar }`; avatar strings must be `"<icon>:<color>"` with a valid color slug (`emerald`, `sky`, … — see `web/src/lib/avatars.ts`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm vitest run src/features/connections/ShareEntryList.test.tsx`
Expected: FAIL — cannot resolve `./ShareEntryList`.

- [ ] **Step 3: Create the component and refactor the dialog**

Create `web/src/features/connections/ShareEntryList.tsx` (the row markup and `roleText` are moved verbatim from `ShareAccessDialog`):

```tsx
import { useTranslation } from 'react-i18next'
import { UserAvatar } from '@/components/UserAvatar'
import type { ShareEntry } from './shared'

interface ShareEntryListProps {
  kind: 'accounts' | 'budgets'
  entries: ShareEntry[]
  /** omitted = read-only rows (plain text, no buttons) */
  onPick?: (entry: ShareEntry) => void
}

export function ShareEntryList({ kind, entries, onPick }: ShareEntryListProps) {
  const { t } = useTranslation()

  const roleText = (entry: ShareEntry): string => {
    if (!entry.role) {
      return t(`modules.connections.${kind}.roles.no_access`)
    }
    const label = t(`modules.connections.${kind}.roles.${entry.role}`)
    if (kind === 'budgets' && entry.isAccepted === false) {
      return `${label} – ${t('modules.connections.modals.share_access.not_accepted')}`
    }
    return label
  }

  const row = (entry: ShareEntry) => (
    <>
      <UserAvatar avatar={entry.user.avatar} size="sm" />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm">{entry.user.name}</span>
        <span className="text-xs text-muted-foreground">{roleText(entry)}</span>
      </span>
    </>
  )

  return (
    <ul className="flex flex-col">
      {entries.map((entry) => (
        <li key={entry.user.id}>
          {onPick ? (
            <button
              type="button"
              className="flex w-full items-center gap-3 rounded-md px-2 py-2 text-left hover:bg-accent"
              onClick={() => onPick(entry)}
            >
              {row(entry)}
            </button>
          ) : (
            <span className="flex w-full items-center gap-3 rounded-md px-2 py-2">{row(entry)}</span>
          )}
        </li>
      ))}
    </ul>
  )
}
```

Rewrite `web/src/features/connections/ShareAccessDialog.tsx` to use it (full new file content):

```tsx
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { ShareEntryList } from './ShareEntryList'
import type { ShareEntry } from './shared'

interface ShareAccessDialogProps {
  open: boolean
  title: string
  kind: 'accounts' | 'budgets'
  entries: ShareEntry[]
  onPick: (entry: ShareEntry) => void
  onClose: () => void
}

export function ShareAccessDialog({ open, title, kind, entries, onPick, onClose }: ShareAccessDialogProps) {
  const { t } = useTranslation()

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title}>
      <div className="flex flex-col gap-2">
        {entries.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.list_empty')}</p>
        ) : (
          <>
            <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.tap_to_share')}</p>
            <ShareEntryList kind={kind} entries={entries} onPick={onPick} />
          </>
        )}
        <Button type="button" className="w-full h-11" onClick={onClose}>
          {t('elements.button.ok.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
```

(`UserAvatar` import moves to `ShareEntryList`; remove it from `ShareAccessDialog`.)

- [ ] **Step 4: Run the new test and the existing consumers' tests**

Run: `pnpm vitest run src/features/connections/`
Expected: ALL PASS, including the untouched `ShareAccessDialog.test.tsx`, `PreviewConnectionDialog.test.tsx`, `ConnectionsPage.test.tsx`.

- [ ] **Step 5: Commit**

```bash
git add src/features/connections/ShareEntryList.tsx src/features/connections/ShareEntryList.test.tsx src/features/connections/ShareAccessDialog.tsx
git commit -m "refactor(connections): extract ShareEntryList from ShareAccessDialog"
```

---

### Task 2: Access row in the account edit modal (`AccountDialog`)

**Files:**
- Modify: `web/src/features/accounts/AccountDialog.tsx`
- Modify: `web/src/features/accounts/AccountDialog.test.tsx`

**Interfaces:**
- Consumes: `ShareAccessDialog`, `AccessLevelDialog`, `buildShareEntries`, `hasAccountAdminAccess`, `ShareEntry` (features/connections); `useConnections`, `useSetAccountAccess`, `useRevokeAccountAccess` (features/connections/queries); `useAccounts` (./queries).
- Produces: nothing consumed by later tasks (Task 3 is independent).

- [ ] **Step 1: Write the failing tests**

Append to `web/src/features/accounts/AccountDialog.test.tsx`. Extend the fixtures import on line 6 to `import { coreHandlers, fixtureAccounts, fixtureConnections, fixtureOwner } from '@/test/fixtures'`.

```tsx
it('edit mode shows the access row for the owner; the share flow posts the grant', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    ...coreHandlers({ connections: fixtureConnections }),
    http.post('*/api/v1/connection/set-account-access', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openAccountModal({ account: fixtureAccounts[0] as unknown as AccountDto })

  await screen.findByText('Edit account')
  await user.click(await screen.findByRole('button', { name: /Access control/ }))
  // ShareAccessDialog: the connection appears with its current (no) access
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  // AccessLevelDialog: grant admin
  await user.click(await screen.findByRole('button', { name: 'Full control' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ accountId: 'a1', userId: 'u2', role: 'admin' })
})

it('create mode has no access row', async () => {
  renderDialog()
  useUiStore.getState().openAccountModal({ folderId: 'f1' })
  await screen.findByText('New account')
  await screen.findByRole('button', { name: /Currency/ })
  expect(screen.queryByRole('button', { name: /Access control/ })).toBeNull()
})

it('edit mode hides the access row from a non-admin member', async () => {
  const foreign = {
    ...fixtureAccounts[0],
    id: 'a-foreign',
    owner: { id: 'u2', avatar: 'pets:sky', name: 'Partner' },
    sharedAccess: [{ user: fixtureOwner, role: 'user' }],
  }
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, foreign], connections: fixtureConnections }))
  renderDialog()
  useUiStore.getState().openAccountModal({ account: foreign as unknown as AccountDto })
  await screen.findByText('Edit account')
  await screen.findByRole('button', { name: /Currency/ })
  expect(screen.queryByRole('button', { name: /Access control/ })).toBeNull()
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm vitest run src/features/accounts/AccountDialog.test.tsx`
Expected: the first and (only the row assertion of the) third test FAIL — no `Access control` button exists; `create mode has no access row` may already pass. The four pre-existing tests must still pass.

- [ ] **Step 3: Implement the access row**

In `web/src/features/accounts/AccountDialog.tsx`:

a) Add imports (keep existing ones):

```tsx
import { AccessLevelDialog } from '@/features/connections/AccessLevelDialog'
import { ShareAccessDialog } from '@/features/connections/ShareAccessDialog'
import type { ShareEntry } from '@/features/connections/shared'
import { buildShareEntries, hasAccountAdminAccess } from '@/features/connections/shared'
import { useConnections, useRevokeAccountAccess, useSetAccountAccess } from '@/features/connections/queries'
import { UserAvatar } from '@/components/UserAvatar'
```

and extend the `./queries` import to include `useAccounts`:

```tsx
import { useAccounts, useCreateAccount, useUpdateAccount } from './queries'
```

b) Inside the component, after the existing `updateAccount` line, add hooks and state:

```tsx
  const { data: accounts } = useAccounts()
  const { data: connections = [] } = useConnections()
  const setAccountAccess = useSetAccountAccess()
  const revokeAccountAccess = useRevokeAccountAccess()
```

and after the `errors` state line:

```tsx
  const [shareOpen, setShareOpen] = useState(false)
  const [levelEntry, setLevelEntry] = useState<ShareEntry | null>(null)
```

c) In the existing `useEffect` that re-seeds on open (right before `setErrors({})`), add:

```tsx
    setShareOpen(false)
    setLevelEntry(null)
```

d) After the `const pending = …` line, add the derived values:

```tsx
  // grant/revoke updates the accounts cache optimistically; read the live copy, not the open-time snapshot
  const liveAccount = account ? accounts?.find((a) => a.id === account.id) ?? account : undefined
  const canShare = !isNew && !!user && !!liveAccount && hasAccountAdminAccess(liveAccount, user.id)
```

e) In the JSX, insert the access row between the currency picker `<button>` and the icon-picker `<div>`:

```tsx
        {canShare && liveAccount ? (
          <button
            type="button"
            className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover"
            title={t('pages.settings.accounts.list_actions.access')}
            onClick={() => setShareOpen(true)}
          >
            <span className="flex min-w-0 flex-col gap-0.5">
              <span className="text-[11px] text-muted-foreground">{t('pages.settings.accounts.list_actions.access')}</span>
              {liveAccount.sharedAccess.length > 0 ? (
                <span className="flex items-center -space-x-2 pt-0.5">
                  <UserAvatar avatar={liveAccount.owner.avatar} size="sm" className="size-7" />
                  {liveAccount.sharedAccess.map((entry) => (
                    <UserAvatar key={entry.user.id} avatar={entry.user.avatar} size="sm" className="size-7" />
                  ))}
                </span>
              ) : null}
            </span>
            <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
          </button>
        ) : null}
```

f) After the closing `<CurrencyPickerDialog … />` element, add the stacked sharing dialogs:

```tsx
      {canShare && liveAccount && user ? (
        <>
          <ShareAccessDialog
            open={shareOpen && levelEntry === null}
            title={liveAccount.name}
            kind="accounts"
            entries={buildShareEntries(connections, liveAccount.sharedAccess, user.id, liveAccount.owner.id)}
            onPick={(entry) => {
              if (entry.role !== 'owner') {
                setLevelEntry(entry)
              }
            }}
            onClose={() => setShareOpen(false)}
          />
          <AccessLevelDialog
            open={levelEntry !== null}
            kind="accounts"
            user={levelEntry?.user ?? null}
            role={levelEntry?.role ?? null}
            onSelect={(role) => {
              if (levelEntry) {
                setAccountAccess.mutate({ accountId: liveAccount.id, userId: levelEntry.user.id, role })
              }
              setLevelEntry(null)
            }}
            onRevoke={() => {
              if (levelEntry) {
                revokeAccountAccess.mutate({ accountId: liveAccount.id, userId: levelEntry.user.id })
              }
              setLevelEntry(null)
            }}
            onClose={() => setLevelEntry(null)}
          />
        </>
      ) : null}
```

(Closing `AccessLevelDialog` drops back to `ShareAccessDialog` — same return-to-list behavior as the settings page flow.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm vitest run src/features/accounts/AccountDialog.test.tsx`
Expected: ALL PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
git add src/features/accounts/AccountDialog.tsx src/features/accounts/AccountDialog.test.tsx
git commit -m "feat(accounts): access-control row in the account edit dialog"
```

---

### Task 3: Manageable access list in the Settings→Accounts preview sheet

**Files:**
- Modify: `web/src/features/accounts/AccountsSettingsPage.tsx`
- Modify: `web/src/features/accounts/AccountsSettingsPage.test.tsx`

**Interfaces:**
- Consumes: `ShareEntryList({ kind, entries, onPick? })` from Task 1; `buildShareEntries`, `hasAccountAdminAccess`, `ShareEntry` (already imported in this file).
- Produces: nothing downstream.

- [ ] **Step 1: Write the failing tests**

In `web/src/features/accounts/AccountsSettingsPage.test.tsx`:

a) Make the viewport helper switchable (replace the existing `mockViewport`):

```tsx
function mockViewport(compact = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}
```

(The `beforeEach` keeps calling `mockViewport()` — desktop default; compact tests re-call it with `true` before `renderPage()`.)

b) Extend the fixtures import (line 7) to also pull `fixtureUsd`:

```tsx
import { coreHandlers, fixtureAccounts as fixtureAccountsForAccess, fixtureFolders, fixtureUsd } from '@/test/fixtures'
```

c) Append three tests:

```tsx
it('compact: the preview sheet lists connections and grants access in place', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  let granted: unknown
  server.use(
    ...coreHandlers({ connections: [{ user: partner, sharedAccounts: [] }] }),
    http.post('*/api/v1/connection/set-account-access', async ({ request }) => {
      granted = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByText('Cash'))
  await screen.findByText('Account details')
  await user.click(await screen.findByRole('button', { name: /Partner/ }))
  await user.click(await screen.findByRole('button', { name: 'Full control' }))
  await waitFor(() => expect(granted).toEqual({ accountId: 'a1', userId: 'u2', role: 'admin' }))
})

it('compact: the preview sheet shows the empty hint when the owner has no connections', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByText('Cash'))
  await screen.findByText('Account details')
  expect(await screen.findByText('No connections found')).toBeInTheDocument()
})

it('compact: the preview sheet shows a read-only access list to a non-admin member', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  const foreign = {
    id: 'a-foreign', owner: partner, folderId: 'f1', name: 'Shared wallet', position: 5,
    currency: fixtureUsd, balance: '10', type: 1, icon: 'wallet',
    sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'user' }],
  }
  server.use(...coreHandlers({
    accounts: [...fixtureAccountsForAccess, foreign],
    connections: [{ user: partner, sharedAccounts: [] }],
  }))
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('folder-General')
  await user.click(screen.getByText('Shared wallet'))
  await screen.findByText('Account details')
  expect(await screen.findByText('Owner')).toBeInTheDocument()
  expect(screen.getByText('Manage transactions')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Partner/ })).toBeNull()
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm vitest run src/features/accounts/AccountsSettingsPage.test.tsx`
Expected: the three new tests FAIL (no `Partner` button / hint / role labels in the preview). All 16 pre-existing tests must still pass.

- [ ] **Step 3: Implement the preview access list**

In `web/src/features/accounts/AccountsSettingsPage.tsx`:

a) Add the import:

```tsx
import { ShareEntryList } from '@/features/connections/ShareEntryList'
```

b) Replace the `levelEntry` state (line 293) — the level dialog must know WHICH account it is editing, because the preview flow must not set `accessAccountId` (that would pop `ShareAccessDialog` open when the level dialog closes):

```tsx
  const [levelTarget, setLevelTarget] = useState<{ accountId: string; entry: ShareEntry } | null>(null)
```

c) Below the `accessAccount` derivation (line 296), derive the live preview copy:

```tsx
  const previewLive = previewAccount ? accounts.find((a) => a.id === previewAccount.id) ?? previewAccount : null
```

d) Update `ShareAccessDialog` (lines 515-526): `open` becomes `accessAccount !== null && levelTarget === null`; `onPick` becomes:

```tsx
        onPick={(entry) => {
          if (entry.role !== 'owner' && accessAccountId) {
            setLevelTarget({ accountId: accessAccountId, entry })
          }
        }}
```

e) Update `AccessLevelDialog` (lines 528-546) to read everything from `levelTarget`:

```tsx
      <AccessLevelDialog
        open={levelTarget !== null}
        kind="accounts"
        user={levelTarget?.entry.user ?? null}
        role={levelTarget?.entry.role ?? null}
        onSelect={(role) => {
          if (levelTarget) {
            setAccountAccess.mutate({ accountId: levelTarget.accountId, userId: levelTarget.entry.user.id, role })
          }
          setLevelTarget(null)
        }}
        onRevoke={() => {
          if (levelTarget) {
            revokeAccountAccess.mutate({ accountId: levelTarget.accountId, userId: levelTarget.entry.user.id })
          }
          setLevelTarget(null)
        }}
        onClose={() => setLevelTarget(null)}
      />
```

f) Rework the preview `ResponsiveDialog` (lines 609-646): gate on `previewLive` instead of `previewAccount`, render account fields from `previewLive`, and insert the access section between the summary `<div>` and the actions `<div>`:

```tsx
      {previewLive ? (
        <ResponsiveDialog
          open
          onOpenChange={(o) => !o && setPreviewAccount(null)}
          title={t('pages.settings.accounts.preview_account_modal.header')}
        >
          <div className="flex items-center gap-3">
            <EntityIcon name={previewLive.icon} className="text-2xl text-muted-foreground" />
            <span className="flex min-w-0 flex-col">
              <span className="truncate text-sm font-medium">{previewLive.name}</span>
              <span className="text-xs text-muted-foreground">
                {moneyFormat(previewLive.balance, previewLive.currency, { useNativePrecision: false })}
              </span>
            </span>
          </div>
          {user && hasAccountAdminAccess(previewLive, user.id) ? (
            <div className="mt-4 flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">{t('pages.settings.accounts.list_actions.access')}</span>
              {buildShareEntries(connections, previewLive.sharedAccess, user.id, previewLive.owner.id).length === 0 ? (
                <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.list_empty')}</p>
              ) : (
                <ShareEntryList
                  kind="accounts"
                  entries={buildShareEntries(connections, previewLive.sharedAccess, user.id, previewLive.owner.id)}
                  onPick={(entry) => {
                    if (entry.role !== 'owner') {
                      setLevelTarget({ accountId: previewLive.id, entry })
                    }
                  }}
                />
              )}
            </div>
          ) : previewLive.sharedAccess.length > 0 ? (
            <div className="mt-4 flex flex-col gap-1">
              <span className="text-[11px] text-muted-foreground">{t('pages.settings.accounts.list_actions.access')}</span>
              <ShareEntryList
                kind="accounts"
                entries={[
                  { user: previewLive.owner, role: 'owner' },
                  ...previewLive.sharedAccess.map((a) => ({ user: a.user, role: a.role })),
                ]}
              />
            </div>
          ) : null}
          <div className={`mt-4 ${dialogActionsClass}`}>
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                setDeleteAccountTarget(previewLive)
                setPreviewAccount(null)
              }}
            >
              {t('elements.button.delete.label')}
            </Button>
            <Button
              type="button"
              onClick={() => {
                openAccountModal({ account: previewLive })
                setPreviewAccount(null)
              }}
            >
              {t('elements.button.edit.label')}
            </Button>
          </div>
        </ResponsiveDialog>
      ) : null}
```

The duplicated `buildShareEntries(...)` call is intentional inline JSX; if the implementer prefers, hoist it to a `const previewEntries = user && previewLive ? buildShareEntries(connections, previewLive.sharedAccess, user.id, previewLive.owner.id) : []` next to `previewLive` and use it in both spots — either form is acceptable.

- [ ] **Step 4: Run the page tests to verify everything passes**

Run: `pnpm vitest run src/features/accounts/AccountsSettingsPage.test.tsx`
Expected: ALL PASS (19 tests) — the pre-existing desktop test `access control: shared avatars, grant and revoke through the dialogs` exercises the `levelTarget` refactor end-to-end (grant AND revoke).

- [ ] **Step 5: Commit**

```bash
git add src/features/accounts/AccountsSettingsPage.tsx src/features/accounts/AccountsSettingsPage.test.tsx
git commit -m "feat(accounts): manageable access list in the mobile account preview"
```

---

### Task 4: Full-suite verification

**Files:** none (verification only).

- [ ] **Step 1: Run the whole frontend suite**

Run from `web/`: `pnpm vitest run`
Expected: ALL PASS except possibly `src/features/import/ImportCsvDialog.test.tsx`, which is a KNOWN pre-existing failure on `main` (unrelated). Any other failure is a regression from this work — fix it before proceeding. If ImportCsvDialog fails, confirm the failure is identical on main (`git stash` is forbidden in this repo — instead check: `git log --oneline main -1` and note the failure is documented in the project memory) and ignore it.

- [ ] **Step 2: Lint and typecheck**

Run: `pnpm lint` — expected: no new warnings/errors.
Run: `pnpm exec tsc --noEmit` (if a `typecheck` script exists in `web/package.json`, use `pnpm typecheck` instead) — expected: clean.

- [ ] **Step 3: Commit any fixups**

If Steps 1-2 forced changes:

```bash
git add -A src/
git commit -m "test: fixups from full-suite verification"
```

Otherwise nothing to commit.
