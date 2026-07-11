# Mobile "Share access" for accounts — Design

Date: 2026-07-11
Status: approved (v2 — supersedes the preview-sheet-button-only v1)

## Problem

On desktop (viewport ≥1024px), the Settings→Accounts page exposes account
sharing through a per-row dropdown menu whose "Access" item opens
`ShareAccessDialog` → `AccessLevelDialog`. That dropdown is rendered only when
`!isCompact` (`web/src/features/accounts/AccountsSettingsPage.tsx`,
`AccountRow`). On compact viewports, tapping a row opens a preview sheet
offering only Edit and Delete — so an owner or admin on a phone has no way to
share an account. Both sharing dialogs are already `ResponsiveDialog`-based
and render correctly as bottom sheets; only the entry points are missing.

## Change

Two surfaces gain access management; both reuse the existing dialogs,
mutations, and i18n keys. No backend or API changes.

### 1. Account edit modal (`web/src/features/accounts/AccountDialog.tsx`)

In **edit mode only** (not create), when the current user may manage access
(`hasAccountAdminAccess(account, user.id)` — owner or `admin`; required
because Settings→Accounts offers Edit to every role):

- Add a picker-style row (same card shape as the currency row, placed after
  it), labeled with the existing `pages.settings.accounts.list_actions.access`
  key, showing the owner + shared-user avatar stack when the account is
  shared, with a chevron.
- Tap opens the existing `ShareAccessDialog` (stacked over the edit dialog)
  → `AccessLevelDialog`, wired to `useConnections` / `useSetAccountAccess` /
  `useRevokeAccountAccess` and `buildShareEntries`.
- The sharing UI reads the **live account from the `useAccounts()` cache**
  by id (not the `params.account` snapshot) so optimistic grant/revoke
  updates render immediately.

Because `AccountDialog` is mounted globally (`ApplicationLayout`), this makes
sharing reachable from every surface that opens account edit: Settings→
Accounts, the account page header (mobile and desktop), the sidebar tree,
the budget page, and onboarding.

### 2. Account preview modal on Settings→Accounts (compact viewports)

The preview sheet (`AccountsSettingsPage.tsx`) gains an inline access list
between the account summary and the Delete/Edit action row:

- **Owner/admin viewer:** the manageable list — all connections with their
  current role (`buildShareEntries`, same data as `ShareAccessDialog`),
  each row tappable (avatar + name + role + chevron); tapping a non-owner
  entry opens `AccessLevelDialog` stacked over the preview to grant, change,
  or revoke. When the user has no connections yet, show the existing
  `modules.connections.modals.share_access.list_empty` hint.
- **Non-admin viewer:** read-only — owner plus current `sharedAccess`
  entries with their roles, not tappable, and no empty-state hint when the
  account is not shared.

To avoid duplicating the row markup, extract the entry-list rendering from
`ShareAccessDialog` into a small shared component in
`web/src/features/connections/` (e.g. `ShareEntryList`), used by both the
dialog and the preview.

State care: `AccessLevelDialog` opened from the preview must not cause
`ShareAccessDialog` to appear when it closes (today its `open` condition is
`accessAccount !== null && levelEntry === null`); the preview-initiated flow
tracks its target account without setting `accessAccountId`.

### Unchanged

- The desktop per-row dropdown "Access" item and its flow.
- The Settings→Connections page.
- Budget sharing.

## Testing

- `AccountDialog.test.tsx`: access row visible when editing an account the
  user owns or admins, opens `ShareAccessDialog`; absent in create mode and
  for non-admin roles; grant flow posts `set-account-access`.
- `AccountsSettingsPage.test.tsx`: extend the file-local `mockViewport()` to
  accept a `compact` flag (`matches: q.includes('1023') ? compact : false` —
  the pattern of `CategoriesPage.test.tsx`). New compact tests: preview shows
  the manageable list for an owned account and tapping an entry opens
  `AccessLevelDialog` (grant posts `set-account-access`); preview shows the
  read-only list (no tap targets) for a non-admin account.
- `ShareAccessDialog.test.tsx` keeps passing after the list extraction.

## Error handling

Nothing new — grant/revoke error handling lives in the existing
`useSetAccountAccess` / `useRevokeAccountAccess` hooks, unchanged.
