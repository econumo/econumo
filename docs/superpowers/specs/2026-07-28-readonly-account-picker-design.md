# Read-only accounts in transaction account pickers — design

**Date:** 2026-07-28
**Status:** Approved

## Problem

The transaction dialog's account pickers (transfer "from", transfer "to", and the
single income/expense picker) offer every account from `get-account-list`,
including shared accounts where the caller's role is `guest` (read-only). The
backend rejects any transaction write on such an account — including the
recipient leg of a transfer (`checkWriteAccess` runs on both legs) — so the UI
lets the user pick a doomed account and then errors out on save.

## Decision

Show write-denied accounts in the pickers but render them **visually disabled
and unselectable**, so the user can see the account exists and understand why it
can't be used. Applies to **all three** transaction account pickers, not just
the transfer recipient. UX-only change; no backend or API changes — the backend
write gate is already correct and stays the source of truth.

## Write-access rule (frontend mirror of the backend gate)

A user can write transactions to an account when they are the **owner**
(`account.owner.id === userId`) or hold an **accepted** `admin` or `user` grant
in `sharedAccess[]`. `guest`, unaccepted grants, and no grant are read-only.
This mirrors `internal/transaction/usecase.go` `checkWriteAccess` +
`HasWriteGrant` (`internal/connection/repo/adapters.go`).

## Changes

1. **Shared helper** — `canWriteToAccount(account: AccountDto, userId: Id | undefined): boolean`
   in `web/src/features/connections/shared.ts`, next to `hasAccountAdminAccess`.
   Refactor the two inline copies of the owner|admin|user rule to use it
   (`web/src/features/accounts/AccountPage.tsx` `canChangeTransaction`,
   `web/src/features/budgets/BudgetTransactionsDialog.tsx`) — no behavior change
   there. Note: the existing inline copies do not check `isAccepted`; the helper
   must (pending grants are inert placeholder rows in the list and are already
   filtered out of `useAccounts` for the invitee, but a grantee row can appear
   unaccepted in someone else's `sharedAccess[]`).

2. **`EntitySelect` per-row disabled** — extend `EntityOption` with optional
   `disabled?: boolean`; pass it through to the base-ui `Combobox.Item` so the
   row renders greyed out and is not selectable by click or keyboard. Call sites
   that don't set it are unaffected.

3. **`TransactionDialog` wiring** — `accountToOption` gains the current user id
   and sets `disabled: !canWriteToAccount(account, userId)`. All three pickers
   flow through it. Existing filters (dedupe of the opposite transfer leg,
   visible-folders filter for new transactions) are unchanged; `disabled` layers
   on top.

## Edge cases

- **Editing an existing transaction** on an account that is (or became)
  read-only: the account remains visible in the picker as the current value, so
  the field never renders blank. It is disabled like any other read-only row;
  the backend rejects a re-save regardless.
- **Swap button** on transfers: swapping can place a disabled account into
  either side's value; that is the same "existing value on a read-only account"
  state as above and needs no special handling.

## Out of scope

- No analytics event: this constrains an existing action, adds none.
- No backend changes, no new API fields — the caller's role is derivable from
  the existing `owner` + `sharedAccess[]` DTO fields.
- No changes to non-transaction account lists (accounts page, budget, etc.).

## Testing

- Unit tests for `canWriteToAccount`: owner, accepted admin, accepted user,
  guest, unaccepted admin/user, no grant.
- `TransactionDialog` test: a guest-role account renders as a disabled option
  and cannot be selected as transfer recipient; a writable account can.
- Gates: `pnpm exec tsc -b`, `pnpm lint`, `pnpm test`.
