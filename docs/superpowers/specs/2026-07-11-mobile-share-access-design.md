# Mobile "Share access" on Settings→Accounts — Design

Date: 2026-07-11
Status: approved

## Problem

On desktop (viewport ≥1024px), the Settings→Accounts page exposes account
sharing through a per-row dropdown menu whose "Access" item opens
`ShareAccessDialog` → `AccessLevelDialog`. That dropdown is rendered only when
`!isCompact` (`web/src/features/accounts/AccountsSettingsPage.tsx`, the
`AccountRow` component). On compact viewports, tapping a row opens a preview
sheet (`ResponsiveDialog`) offering only Edit and Delete — so an owner or admin
on a phone has no way to share an account from this page. Both sharing dialogs
are already `ResponsiveDialog`-based and render correctly as bottom sheets on
phones; only the entry point is missing.

## Change

All in `web/src/features/accounts/AccountsSettingsPage.tsx`:

1. Add a full-width **"Share access"** button to the existing mobile preview
   sheet, above the Delete/Edit action row.
   - Label: reuse the existing i18n key
     `pages.settings.accounts.list_actions.access` (same label as the desktop
     menu item). No new i18n keys.
2. Render it only when the current user may manage access to the previewed
   account: `user && hasAccountAdminAccess(previewAccount, user.id)` — the
   same guard the desktop menu item uses (owner or `admin` role).
3. On tap: `setAccessAccountId(previewAccount.id)` and
   `setPreviewAccount(null)` — closes the preview and opens the existing
   `ShareAccessDialog`, from which the whole existing flow works unchanged
   (pick person → `AccessLevelDialog` → `set-account-access` /
   `revoke-account-access` mutations).

No backend changes, no API changes, no new components or dialogs.

## Testing

In `web/src/features/accounts/AccountsSettingsPage.test.tsx`:

- Extend the file-local `mockViewport()` helper to accept a `compact` flag
  (`matches: q.includes('1023') ? compact : false`) — the exact pattern
  already used by `CategoriesPage.test.tsx` and `AccountPage.test.tsx`.
- New compact-mode tests:
  1. Tapping an account row the user owns (or admins) opens the preview sheet
     containing a "Share access" button; tapping it opens `ShareAccessDialog`.
  2. For an account where the user lacks admin access, the preview sheet has
     no "Share access" button (Edit/Delete still present).

## Error handling

Nothing new — grant/revoke error handling lives in the existing
`useSetAccountAccess` / `useRevokeAccountAccess` hooks, which are unchanged.

## Out of scope

- The Settings→Connections page (already fully mobile-usable).
- Budget sharing (separate surface, already reachable on mobile).
- Any redesign of the preview sheet into a pure action sheet.
