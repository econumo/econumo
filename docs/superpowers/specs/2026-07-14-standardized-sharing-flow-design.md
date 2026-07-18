# Standardized Sharing Flow — Design

**Date:** 2026-07-14
**Status:** Approved

## Problem

Econumo has two shareable entity types — accounts and budgets — with two different
sharing flows:

- **Budgets** follow a grant → pending → accept/decline handshake
  (`budgets_access.is_accepted`). The recipient must accept before the budget
  becomes usable.
- **Accounts** are shared with immediate effect (`set-account-access`): no accept
  step, and the account is silently auto-placed into the recipient's *last*
  folder. The recipient is never asked and may not notice.

There is no recipient-facing surface that collects pending shares, and the two
API contracts differ in naming, module placement, and DTO shape.

## Goal

One sharing flow for every shareable entity:

1. The sharer selects a connected user and grants access → the grant is **pending**.
2. The recipient sees a pending sharing request in-app and **accepts or declines**.
3. Only after accepting does the shared entity become visible/usable.
4. When accepting an **account**, the recipient chooses the folder it lands in.

Budget sharing already implements the handshake; its contract becomes the
standard. Account sharing is brought up to it.

## Non-goals

- No generic notification subsystem (no `notifications` table, no mark-read).
  The pending access rows *are* the notification.
- No push channel (SSE/websocket). Freshness rides on the existing TanStack
  Query staleTime/invalidation/manual-refresh model.
- No new shareable entity types. Categories/tags/payees stay per-user.
- No changes to the connection (user-linking) invite-code flow.

## Data model

One migration per engine (`internal/infra/storage/migrations/{sqlite,pgsql}`):

```sql
ALTER TABLE accounts_access ADD COLUMN is_accepted BOOLEAN NOT NULL DEFAULT '0';
UPDATE accounts_access SET is_accepted = '1';
```

Existing shares are grandfathered as accepted — no visible change for current
users. The column mirrors `budgets_access.is_accepted`. sqlc queries for both
dialects are updated and regenerated.

## Access semantics (backend)

### Grant (`grant-access`)

Gate: caller is the account owner or an admin grantee; target must be a
connected user (both as today). Creates the `accounts_access` row with
`is_accepted = 0` and **nothing else** — no `accounts_options` row, no folder
membership. Re-granting while pending updates the role and stays pending.
Changing the role of an accepted share does not reset acceptance (same as
budgets).

### Accept (`accept-access`)

Gate: caller has an unaccepted row for the account. In one transaction:

1. Mark the row accepted.
2. Seed `accounts_options` at `position = max + 1`.
3. Add the account to the folder the recipient chose (`folderId`), validated
   as one of the recipient's own folders.

`folderId` is optional on the wire: when omitted (recipient has no folders),
the backend auto-creates a "General" folder — the same fallback
`create-account` already uses (`internal/account/create.go`).

### Decline (`decline-access`)

Recipient deletes their own **pending** row. Accepted access continues to be
dropped via the existing own-revoke path (e.g. account delete for non-owners).

### Revoke (`revoke-access`)

Owner/admin deletes the row, pending or accepted, with today's cleanup
(folder membership + `accounts_options`). Revoking a pending row is how the
sharer cancels an invite.

For **budgets**, revoke (and decline, and the delete-connection unwind) also
removes the member's seeded records from the budget: their category/tag
elements (limits cascade) and their categories' envelope assignments. Revoke
is deliberate, so the member's limit history goes with them; re-accepting a
later invite re-seeds fresh elements. Budget accept stays idempotent
regardless (it skips already-present `external_id`s) to tolerate
pre-handshake budgets whose pending members carry grandfathered elements.

### Unaccepted row = no access

Everywhere the backend currently asks "does this user have an
`accounts_access` row?", an unaccepted row now counts as **no access** —
exactly like budget's `budgetRole` treats unaccepted rows as `AccessDenied`.
This covers transaction visibility, transaction writes, balance math, and
budget spending aggregation over shared accounts. The implementation plan must
audit every consumer of the access table.

**Deliberate exception:** `get-account-list` includes the pending account for
the recipient (so they can see and accept it), marked via `isAccepted` in the
access data, with `folderId: null` and no position.

`delete-connection` keeps unwinding all access between the two users in both
directions, now including pending rows.

## API surface

New endpoints in the **account** feature (`internal/account/api/routes.go`),
mirroring budget's names exactly:

| Endpoint | Body |
|---|---|
| `POST /api/v1/account/grant-access` | `{accountId, userId, role}` (role: `admin\|user\|guest`) |
| `POST /api/v1/account/accept-access` | `{accountId, folderId?}` |
| `POST /api/v1/account/decline-access` | `{accountId}` |
| `POST /api/v1/account/revoke-access` | `{accountId, userId}` |

**Removed:** `POST /api/v1/connection/set-account-access` and
`POST /api/v1/connection/revoke-account-access`. The SPA ships with the binary,
so both sides change in one release. Route count is net +2, so the apiparity
never-shrink guards hold.

**Budget endpoints are untouched** — `/api/v1/budget/grant-access`,
`accept-access`, `decline-access`, `revoke-access` already are the standard.
After this change both features expose an identical sharing contract, which is
the template for any future shareable entity.

**DTO change:** account `sharedAccess[]` entries gain `isAccepted` (int `0`/`1`,
matching budget's `access[].isAccepted` and the frozen int-flag convention).
Both the sharer's "awaiting acceptance" indicator and the recipient's pending
detection read this field.

## Module placement

The sharing logic in `internal/connection/setaccess.go` / `revoke.go` moves
into the **account** feature, which already owns folders and options — the
`ConnectionFolderPort` glue disappears. The connection feature keeps only what
it genuinely owns: invite codes, the user link, and `delete-connection`'s
unwind. For the unwind it declares a consumer port:

```go
// internal/connection/ports.go
RevokeAccessBetween(ctx, userA, userB vo.Id) error
```

implemented by the account feature and wired in `internal/server` via
`glue_connection_accountaccess.go` — exactly parallel to the existing
`budgetAccess.RevokeBetween` port.

## Frontend

### Sidebar button

A "Sharing requests" item in the left sidebar, directly **above the Budgets
link**, shown **only when there are pending invites**, with a count badge.
When nothing is pending it disappears entirely. The count comes from a
`usePendingInvites()` hook deriving from cached lists: accounts where my
access entry has `isAccepted === 0` plus budgets where
`myAccess.isAccepted === 0`. No new endpoint, no poll.

### Requests modal

Clicking the button opens a modal listing all pending requests across both
entity types. Each row: owner avatar + name, entity-type icon, entity name,
granted role, Accept / Decline buttons. Decline asks a one-step confirmation.
When the last request is resolved the modal closes and the sidebar button
disappears.

- **Account rows** show the folder picker immediately, inline in the row (no
  expand step): recipient's folders from `get-folder-list`, defaulting to the
  first one. Hidden folders (`isVisible: 0`) stay listed and selectable but
  are marked (EyeOff icon + muted text, the accounts-settings convention) —
  accepting into one means the account won't show in the accounts tree.
  Accept commits directly with the selected folder. With zero folders the
  picker shows "General (will be created)" and omits `folderId`. On success
  the dialog closes and the app navigates to the account's page (its
  transaction list); the backend already positions the account last in the
  chosen folder (`position = max + 1`).
- **Accepting a budget** applies immediately, no extra step. On success it
  becomes the default budget (`update-default-budget`, same as tapping a
  budget in settings), the dialog closes, and the app navigates to `/budget`.

### Entity lists stay clean

Pending accounts and budgets do **not** appear in the Accounts page or Budgets
list — the modal is the only recipient-side surface. The current inline
pending-budget affordance in `BudgetsPage.tsx` is removed. The list endpoints
still carry pending entries with `isAccepted: 0`; the frontend routes them to
the modal instead of the lists. Pending accounts show no balance and never
count in totals.

### Sharer side

`ShareAccessDialog` / `ShareEntryList` (already shared by both domains) gain a
per-entry pending indicator ("invited, awaiting acceptance"); the existing
revoke affordance doubles as cancel-invite on pending entries.

### API clients

`web/src/api/account.ts` gains `grantAccess` / `acceptAccess` /
`declineAccess` / `revokeAccess`; the two removed functions leave
`connection.ts`. Mutation hooks move to `features/accounts/queries.ts`,
invalidating the account list (and connections where relevant) on settle. The
modal and sidebar button live in `features/connections/`.

## Testing

- **Backend unit/integration** (`internal/account`): grant→pending; accept
  with/without folder; accept rejects a foreign folder; decline is
  pending-only; revoke works pending+accepted; role update doesn't reset
  acceptance; a pending recipient cannot read the account's transactions or
  write to it (the access-consumer audit gets explicit tests). Connection
  tests cover the new `RevokeAccessBetween` port.
- **API parity:** scenarios for the four new account routes replace the two
  removed connection ones (net +2 routes). Goldens regenerate via
  `UPDATE_GOLDEN=1` with the diff inspected: expected changes are the new
  routes, `sharedAccess[].isAccepted` in account payloads, and the removed
  routes' goldens.
- **Engine parity:** the enginecompare suite replays the catalogue on
  PostgreSQL; `make test-repo-pgsql` exercises the new pgsql queries.
- **Frontend (vitest):** `usePendingInvites` count derivation; requests modal
  (account row shows inline folder picker defaulting to the first folder,
  hidden folders marked; accept account → mutation; accept budget direct; decline
  confirm); sidebar button visibility.

## Migration & rollout

Single release. The SQL migration (add `is_accepted`, backfill `1`) runs on
boot before the new code serves traffic, so existing shares are accepted
before the stricter access checks apply — no user-visible regression. Grants
created after the release start pending. Budgets need no data migration.

## Edge cases

- **Disconnect while pending:** `delete-connection` removes pending and
  accepted rows both ways (already its job).
- **Recipient deletes the chosen folder later:** same as today for any account
  whose folder vanishes — membership row goes; account shows `folderId: null`.
- **Owner deletes the account while an invite is pending:** access rows are
  cleaned up as today.
- **Double-accept / accept-after-revoke:** validation errors with the standard
  error envelope.
