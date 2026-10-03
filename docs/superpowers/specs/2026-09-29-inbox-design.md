# Inbox — one place for everything waiting on the user

Date: 2026-09-29
Branch: `feature/inbox` (from `v1.6-dev`, PR targets `v1.6-dev`)

## Problem

`v1.6-dev` has two unrelated "needs your attention" surfaces:

| | Sharing invites | Imported transactions |
|---|---|---|
| Source | derived client-side from the account/budget list caches (`isAccepted === 0` for the caller) — `web/src/features/connections/pendingInvites.ts` | `get-import-queue` (`queued` rows only) |
| Surface | "Sharing requests (N)" sidebar item (+ `UserPlus` rail icon), only while N > 0 | full-width top banner `ImportQueueBanner` |
| Action | opens `SharingRequestsDialog` | links to the `/imports/queue` page |

Different places, different interactions, different counting rules — users do not
know where to look. Failed import events are never counted anywhere, and a failing
SimpleFIN sync is only visible on the SimpleFIN settings page. The top banner slot
is also shared with `SubscriptionBanner` and `ServerVersionNotice`.

## Goal

One **Inbox**: a single, always-present entry point with one badge, opening one page
where every item that waits on the user is shown and acted on.

Success criteria:

- The Inbox button is always in the same spot (visible even with nothing pending).
- The badge count and the page content come from one source and cannot disagree.
- Pending invites, queued imports, failed imports and failing syncs all appear there;
  nothing of that kind is surfaced anywhere else.
- The top banner slot holds only `SubscriptionBanner` and `ServerVersionNotice`.

## Decisions

- **Action inbox, not an activity feed.** Items exist only while the underlying state
  needs the user; they clear themselves when that state resolves. No read/unread, no
  dismiss, nothing persisted. Informational events ("Anna accepted your invite") are
  out of scope.
- **In-app only.** No email or push.
- **Included kinds:** sharing invites (account + budget), failing import syncs, failed
  import events, queued imports. **Excluded:** the app-update notice (keeps its dot on
  the settings gear), skipped import rows (shown, but not counted).
- **Everything inline.** The import review lives *inside* the Inbox. There is no
  `/imports/queue` page and no redirect — the import feature has never shipped (it
  exists only on `v1.6-dev`; `main` and `v1.5.2` have no `web/src/features/imports`),
  so nothing can link to the old path.
- **Client-derived list.** No new endpoint; one backend addition (`lastRun*` fields on
  `get-source-list`) so a failing sync is visible without a per-source run fetch.

## User-facing design

### Sidebar identity row (full sidebar and the phone home screen)

```
[avatar 32px] Name ...................... [Inbox (N)] [⚙︎•]
```

- The email line is removed from the sidebar block (the Profile page keeps
  `UserCard` with email unchanged).
- Avatar + name link to Profile (`/settings/profile`), as today.
- **Inbox button** (lucide `Inbox` icon) opens `/inbox`. Always rendered; shows a
  count badge only when N > 0; N above 99 renders as `99+`.
- **Settings gear** opens Settings and carries the update dot (moved from the footer
  link). The sidebar footer keeps the logo, version label and sync button, and loses
  its "Settings" link. The `UpdateNotice` card is unchanged.

### Collapsed desktop rail

Avatar → Inbox (badge) → Budget. The rail footer keeps its gear + sync buttons
unchanged.

### Removed

- "Sharing requests" sidebar item and the `UserPlus` rail icon.
- `SharingRequestsDialog`.
- `ImportQueueBanner`.
- The `/imports/queue` route, `RouterPage.IMPORT_QUEUE` and `ImportQueuePage`.
- `AppleWalletPage`'s "Import queue" link now points to `/inbox`.

### `/inbox` page

Title "Inbox", rendered in the workspace pane on desktop and as a full page on
phones. Sections, top to bottom (shortest/most urgent first so invites never sink
under a long import list); a section with no items is not rendered:

1. **Sharing** — one row per pending account/budget invite: owner avatar + name,
   what was shared (account or budget name), the role, **Accept** / **Decline**.
   Accept keeps today's behavior, including the folder choice for accounts.
2. **Sync problems** — one row per import source whose latest run ended `failed` or
   `partial`: source name, run time, first error message. Tapping opens the source's
   settings page (`/settings/simplefin`).
3. **Failed imports** — today's queue "Needs attention" rows (received date,
   **Retry** / **Discard**).
4. **To review** — queued rows grouped by card, today's behavior: tap opens the
   transaction dialog to import, **Skip**; a card group whose card is unmapped offers
   "Ignore card".
5. **Skipped (N)** — collapsed by default; **Restore** (unskip). Not counted.

Empty state (no items in sections 1–4 and no skipped rows): "All caught up".
With only skipped rows, the collapsed Skipped section is shown on its own.

**Badge count** = invites + sources with a failing sync + failed events + queued rows.

## Data flow

### Backend — `internal/imports` only

`GET /api/v1/import/get-source-list`: each `ImportSourceResult` gains

| field | value |
|---|---|
| `lastRunStatus` | status of the source's latest run by `started_at`: `running` / `completed` / `partial` / `failed`; `""` when the source has never run (every Apple Wallet push source) |
| `lastRunAt` | that run's `finished_at`, or `started_at` while running; `""` when none |
| `lastRunError` | first entry of that run's `errors` message list; `""` when none |

- No new SQL: the service reads each source's latest run through the existing
  `ListRunsByUser(ctx, userID, &sourceID, 1)` repo method (`ORDER BY started_at DESC,
  id DESC LIMIT 1`) and merges it into the source list; `lastSyncedAt` (last
  non-failed sync) is unchanged.
- No new endpoint, no cross-feature glue. Imports has no MCP surface, so no MCP
  change.
- Regenerate swagger (`make swagger`) and the `get-source-list` apiparity golden;
  the enginecompare suite must stay byte-identical across engines.

### Frontend — new `web/src/features/inbox/`

- `useInbox()` — the single source of truth, returning the items grouped by section
  plus `count`. It composes:
  - `usePendingInvites()` (stays in `features/connections/`);
  - `useImportQueue()` → `queued`, `failed`, `skipped`;
  - `useImportSources()` → sources with `lastRunStatus` of `failed` or `partial`.
- The sidebar/rail Inbox button and `InboxPage` both read `useInbox()`.
- `ImportQueuePage` is split into section components kept in `features/imports/`
  (`FailedImportsSection`, `ToReviewSection`, `SkippedSection`) with today's
  mutations, toasts and transaction-dialog flow; `InboxPage` composes them.
- `SharingSection` (in `features/inbox/`) takes the accept/decline logic from
  `SharingRequestsDialog`, which is then deleted.
- `useImportSources()` now also runs from the app shell (one small GET at boot,
  same `TEN_MINUTES` stale time and refetch-on-focus as today).

### How items clear

Only through existing cache invalidation — nothing new:

- Accept/Decline mutations already invalidate the account/budget lists.
- Import/skip/unskip/retry/discard already update or invalidate `importQueue`.
- `sync-source` already invalidates `importSources`, so a successful re-sync removes
  the Sync problems row.

Freshness is unchanged: another user's new invite, or a Wallet tap from the phone,
appears on the next refetch (window focus, the sync button, or 10-minute staleness).

## Analytics

- All existing action metrics keep firing from the same hooks, names unchanged:
  `BUDGET_ACCEPT_ACCESS` / `BUDGET_DECLINE_ACCESS`,
  `CONNECTION_ACCEPT_ACCOUNT_ACCESS` / `CONNECTION_DECLINE_ACCOUNT_ACCESS`,
  `IMPORT_QUEUE_IMPORT` / `IMPORT_QUEUE_SKIP`, `IMPORT_ACCOUNT_IGNORE`.
- `/inbox` page views arrive as `$page_view` with `$path=/inbox`.
- New `INBOX_OPEN` (`appInboxOpen`), fired when the sidebar or rail Inbox button is
  tapped, with `eventData` `{ invites, syncProblems, failed, queued }` — shows
  whether people open a full or an empty inbox, i.e. whether the badge works.

## i18n (all 11 catalogues)

- New `inbox.*` namespace: title, button label, section headings, the sync-problem
  row (`{source}`, `{date}`, error line), the empty state "All caught up", the
  "Skipped ({count})" toggle; plurals in the pipe-delimited format.
- `connections.sharing_requests.*` row strings move to `inbox.sharing.*`.
- Removed: `common.nav.sharing_requests`, `imports.banner.*`,
  `imports.queue.header`, `imports.queue.empty`.
- `imports.queue.*` row strings stay (the section components remain in
  `features/imports/`).

## Docs

- `docs/regression-test-plan.md` (📱 markers kept accurate):
  - replace the "Review banner" item and the queue-page items with Inbox items —
    badge count and what counts, section order, empty sections hidden, "All caught
    up", items clearing after Accept/Import/re-sync, sync-problem row opens the source;
  - replace the "Sharing requests" items with the Inbox Sharing section;
  - add identity-row items: no email, avatar → Profile, gear with update dot, rail
    order, Inbox visible at zero.
- `CLAUDE.md`: one line in *Frontend architecture* — the Inbox (`features/inbox`) is
  the single surface for anything waiting on the user; new features route their
  attention items there, never a new banner.

## Testing

Go:

- Service test for the latest run per source: several runs per source, a source
  with no runs, another user's runs ignored — on SQLite and, via
  `make test-repo-pgsql`, PostgreSQL.
- Service test: source list carries `lastRunStatus` / `lastRunAt` / `lastRunError`.
- apiparity golden for `get-source-list` regenerated and the diff inspected.

Vitest:

- `useInbox`: count sums the four kinds; skipped not counted; `partial` and `failed`
  count as sync problems, `running` / `completed` / `""` do not.
- `InboxPage`: section order, empty sections hidden, "All caught up", Skipped
  collapsed by default.
- Sidebar: no email; Inbox visible at 0 with no badge; `99+`; tap fires
  `INBOX_OPEN` with the counts; gear carries the update dot.
- Existing queue-page tests re-pointed at the section components.
- `metrics-coverage` passes with the new key.

Gates: `make go-test`, `make web-test`, `make web-lint`, `make test` (engine parity).

## Out of scope

- Activity feed / read state / dismissing items.
- Email or push delivery.
- Server-side aggregation endpoint (can replace `useInbox()` internals later without
  UI changes).
- Scheduled SimpleFIN syncs (only manual syncs exist today; the Sync problems row
  gains value once scheduled syncs land).
