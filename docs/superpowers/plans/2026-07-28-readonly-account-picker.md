# Read-only Accounts Disabled in Transaction Pickers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render write-denied (guest / read-only) accounts as visually disabled, unselectable options in all three transaction account pickers (transfer "from", transfer "to", income/expense account).

**Architecture:** UX-only frontend change (no backend/API changes). One new shared helper `canWriteToAccount` mirrors the backend write gate (owner OR accepted admin/user grant); `EntitySelect` gains per-row `disabled` support passed through to the base-ui `Combobox.Item`; `TransactionDialog.accountToOption` sets the flag. Two existing inline copies of the write rule are refactored onto the helper.

**Tech Stack:** React 19, TypeScript, base-ui Combobox, vitest + testing-library + msw. All commands run from `web/` via pnpm.

**Spec:** `docs/superpowers/specs/2026-07-28-readonly-account-picker-design.md`

## Global Constraints

- Work on branch `feature/readonly-account-picker` in this worktree. Verify with `git branch --show-current` before the first commit.
- Write rule (frozen, mirrors backend `checkWriteAccess`): writable = `account.owner.id === userId` OR an entry in `sharedAccess[]` with `user.id === userId && isAccepted === 1 && (role === 'admin' || role === 'user')`. Guest, unaccepted, no grant, or `userId === undefined` ⇒ not writable.
- No analytics event (constrains an existing action; adds none).
- No behavior change to `AccountPage` / `BudgetTransactionsDialog` beyond routing their existing rule through the helper.
- Final gates (all from `web/`): `pnpm exec tsc -b`, `pnpm lint`, `pnpm test`.
- Tests may hit load-dependent 5s vitest timeouts (known pre-existing issue); rerun the file before diagnosing.

---

### Task 1: `canWriteToAccount` helper + refactor the two inline copies

**Files:**
- Modify: `web/src/features/connections/shared.ts` (add helper after `hasAccountAdminAccess`, ~line 82)
- Modify: `web/src/features/accounts/AccountPage.tsx:88-94`
- Modify: `web/src/features/budgets/BudgetTransactionsDialog.tsx:120-134`
- Test: `web/src/features/connections/shared.test.ts`

**Interfaces:**
- Consumes: `AccountDto` (`web/src/api/dto/account.ts`), `Id` (`web/src/api/types`) — both already imported in `shared.ts`.
- Produces: `canWriteToAccount(account: AccountDto, meId: Id | undefined): boolean` exported from `web/src/features/connections/shared.ts`. Task 3 imports it.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/features/connections/shared.test.ts` (the file already defines `me` = `{ id: 'u1', … }`, `partner` = `{ id: 'u2', … }`, `mine` (owner me), and `theirs` (owner partner, my grant `guest` accepted)):

```ts
it('canWriteToAccount mirrors the backend gate: owner or accepted admin/user grant', () => {
  expect(canWriteToAccount(mine, 'u1')).toBe(true) // owner
  expect(canWriteToAccount({ ...theirs, sharedAccess: [{ user: me, role: 'admin', isAccepted: 1 }] }, 'u1')).toBe(true)
  expect(canWriteToAccount({ ...theirs, sharedAccess: [{ user: me, role: 'user', isAccepted: 1 }] }, 'u1')).toBe(true)
  expect(canWriteToAccount(theirs, 'u1')).toBe(false) // guest
  expect(canWriteToAccount({ ...theirs, sharedAccess: [{ user: me, role: 'admin', isAccepted: 0 }] }, 'u1')).toBe(false) // unaccepted
  expect(canWriteToAccount({ ...theirs, sharedAccess: [] }, 'u1')).toBe(false) // no grant
  expect(canWriteToAccount(mine, undefined)).toBe(false) // user not loaded yet
})
```

Add `canWriteToAccount` to the import list on line 1 of the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run (from `web/`): `pnpm test src/features/connections/shared.test.ts`
Expected: FAIL — `canWriteToAccount` is not exported.

- [ ] **Step 3: Implement the helper**

In `web/src/features/connections/shared.ts`, directly after `hasAccountAdminAccess` (line 82):

```ts
/** Mirrors the backend transaction write gate: owner, or an ACCEPTED admin/user grant. Guest is read-only. */
export function canWriteToAccount(account: AccountDto, meId: Id | undefined): boolean {
  if (!meId) return false
  if (account.owner.id === meId) return true
  const entry = account.sharedAccess.find((a) => a.user.id === meId)
  return !!entry && entry.isAccepted === 1 && (entry.role === 'admin' || entry.role === 'user')
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test src/features/connections/shared.test.ts`
Expected: PASS (all tests in the file, old and new).

- [ ] **Step 5: Refactor the two inline copies**

In `web/src/features/accounts/AccountPage.tsx`, replace only the `canChangeTransaction` line (keep `myRole`, `isOwner`, `canUpdateSettings`, and the comment above `canChangeTransaction` intact):

```ts
const canChangeTransaction = canWriteToAccount(account, user?.id)
```

Add to imports: `import { canWriteToAccount } from '@/features/connections/shared'` (merge into the existing import from that module if one exists).

In `web/src/features/budgets/BudgetTransactionsDialog.tsx`, inside `canChange` (lines 120-134), replace the four lines computing `isOwner`/`myRole` and the `if (!(isOwner || …))` check with:

```ts
if (!canWriteToAccount(account, user?.id)) {
  return false
}
```

Add the same import. Behavior note (from the spec): the helper additionally requires `isAccepted === 1`, but pending invites never appear in `useAccounts` for the invitee, so this is a no-op in practice.

- [ ] **Step 6: Run the two features' tests + type check**

Run: `pnpm test src/features/accounts src/features/budgets src/features/connections && pnpm exec tsc -b`
Expected: PASS / no type errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/connections/shared.ts web/src/features/connections/shared.test.ts web/src/features/accounts/AccountPage.tsx web/src/features/budgets/BudgetTransactionsDialog.tsx
git commit -m "refactor: extract canWriteToAccount write-gate helper"
```

---

### Task 2: Per-row `disabled` support in `EntitySelect`

**Files:**
- Modify: `web/src/features/transactions/EntitySelect.tsx`
- Test: `web/src/features/transactions/EntitySelect.test.tsx`

**Interfaces:**
- Produces: `EntityOption` gains optional `disabled?: boolean`. A disabled option renders greyed out and cannot be picked by click or keyboard. Call sites not setting it are unaffected. Task 3 relies on this field.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/features/transactions/EntitySelect.test.tsx`:

```tsx
it('a disabled option is marked disabled and cannot be clicked', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  const options = [...OPTIONS, { value: 'c4', label: 'Locked', disabled: true }]
  render(<EntitySelect aria-label="Category" value={null} onChange={onChange} options={options} />)

  await user.click(combobox())
  const locked = await screen.findByRole('option', { name: 'Locked' })
  expect(locked).toHaveAttribute('aria-disabled', 'true')
  await user.click(locked)
  expect(onChange).not.toHaveBeenCalled()
})

it('Enter on a filter that matches only a disabled option selects nothing', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  const options = [...OPTIONS, { value: 'c4', label: 'Locked', disabled: true }]
  render(<EntitySelect aria-label="Category" value={null} onChange={onChange} options={options} />)

  await user.click(combobox())
  await user.keyboard('lock')
  expect(await screen.findByRole('option', { name: 'Locked' })).toBeInTheDocument()
  await user.keyboard('{Enter}')
  expect(onChange).not.toHaveBeenCalled()
})
```

Contingency: if base-ui marks disabled items with `data-disabled` instead of `aria-disabled`, assert `toHaveAttribute('data-disabled')` — check the rendered DOM (`screen.debug(locked)`) before changing the implementation; the not-selectable assertions must hold either way.

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test src/features/transactions/EntitySelect.test.tsx`
Expected: the two new tests FAIL (option is selectable / not marked disabled). Existing tests still pass.

- [ ] **Step 3: Implement**

In `web/src/features/transactions/EntitySelect.tsx`:

Extend the option type (lines 8-12):

```ts
export interface EntityOption {
  value: string
  label: string
  icon?: string
  disabled?: boolean
}
```

Pass the flag through to the item (line 144-148) — add one prop to `ComboboxItem`:

```tsx
<ComboboxItem
  key={row.create ? '__create__' : row.value}
  value={row}
  disabled={row.disabled}
  className={row.create ? 'text-econumo-magenta' : undefined}
>
```

`ComboboxItem` (`web/src/components/ui/combobox.tsx:150`) already spreads props onto `ComboboxPrimitive.Item` and already styles `data-disabled:pointer-events-none data-disabled:opacity-50`, so no change there. Base-ui skips disabled items for highlight/selection.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test src/features/transactions/EntitySelect.test.tsx`
Expected: PASS (all tests in the file).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/transactions/EntitySelect.tsx web/src/features/transactions/EntitySelect.test.tsx
git commit -m "feat: per-option disabled support in EntitySelect"
```

---

### Task 3: Wire the write gate into the TransactionDialog account pickers

**Files:**
- Modify: `web/src/features/transactions/TransactionDialog.tsx` (imports ~line 28; `accountToOption` at lines 193-197)
- Test: `web/src/features/transactions/TransactionDialog.test.tsx`

**Interfaces:**
- Consumes: `canWriteToAccount(account, userId)` from `@/features/connections/shared` (Task 1); `EntityOption.disabled` (Task 2).
- Produces: user-visible behavior only — all three pickers already map through `accountToOption`, so no other call-site changes.

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/transactions/TransactionDialog.test.tsx` (the logged-in user is `fixtureUser`/`fixtureOwner`, id `u1`; `coreHandlers({ accounts })` overrides the account list; `fixtureUsd` needs adding to the import from `@/test/fixtures`):

```tsx
it('read-only shared accounts are disabled in the transfer account pickers', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  const readOnlyShared = {
    id: 'a-ro', owner: partner, folderId: null, name: 'Partner cash', position: 9,
    currency: fixtureUsd, balance: '50', type: 1, icon: 'wallet',
    sharedAccess: [{ user: fixtureOwner, role: 'guest', isAccepted: 1 }],
  }
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, readOnlyShared] }))
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'transfer' })
  await screen.findByRole('heading', { name: 'Add transaction' })

  const toPicker = () => screen.getByRole('combobox', { name: 'to account' }) as HTMLInputElement
  await user.click(toPicker())
  const locked = await screen.findByRole('option', { name: /Partner cash/ })
  expect(locked).toHaveAttribute('aria-disabled', 'true')
  await user.click(locked)
  expect(toPicker().value).not.toContain('Partner cash')

  // a writable account remains selectable
  await user.click(await screen.findByRole('option', { name: /Bank/ }))
  await waitFor(() => expect(toPicker().value).toContain('Bank'))
})
```

If Task 2 landed on the `data-disabled` contingency, use the same attribute here.

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm test src/features/transactions/TransactionDialog.test.tsx`
Expected: the new test FAILS at the `aria-disabled` assertion (option renders enabled). Existing tests still pass.

- [ ] **Step 3: Implement**

In `web/src/features/transactions/TransactionDialog.tsx`:

Add the import:

```ts
import { canWriteToAccount } from '@/features/connections/shared'
```

Change `accountToOption` (lines 193-197) to set the flag — `user` (from `useUserData()`, line 74) is in scope:

```ts
const accountToOption = (a: (typeof accounts)[number]) => ({
  value: a.id,
  label: `${a.name} (${moneyFormat(a.balance, a.currency)})`,
  icon: a.icon,
  disabled: !canWriteToAccount(a, user?.id),
})
```

No picker JSX changes: the income/expense picker (line 296), transfer "from" (line 335), and transfer "to" (line 355) all map through `accountToOption`; existing dedupe/visible-folder filters stay as they are.

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm test src/features/transactions/TransactionDialog.test.tsx`
Expected: PASS (all tests in the file).

- [ ] **Step 5: Full gates**

Run (from `web/`): `pnpm exec tsc -b && pnpm lint && pnpm test`
Expected: no type errors, no lint errors, full suite green.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/transactions/TransactionDialog.tsx web/src/features/transactions/TransactionDialog.test.tsx
git commit -m "feat: disable read-only accounts in transaction account pickers"
```
