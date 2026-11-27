# User deletion

Status: design, 2026-11-26.

## Intent

A user can delete their Econumo data, and users nobody has used for a long time are marked for
deletion automatically. Deletion is a two-step process: the user is MARKED, and later WIPED.
Any sign-in between the two removes the mark. Wiping first disconnects the user from everyone
(the existing stop-sharing path), then deletes the `users` row and lets the database cascade
remove everything the user owns or wrote. Partners' account balances are kept right with one
correction per affected account, the way deleting an account already does. An audit table
records the deletion history and lets a returning lifetime-access customer get lifetime access
back.

## Goals

- Three ways to mark: self-service in Settings, inactivity, CLI.
- Wiping after a configurable delay (`ECONUMO_USER_DELETION_DAYS`, 0 = never automatically),
  or by hand with the CLI.
- A sign-in on a marked user removes the mark.
- After a wipe nothing personal remains readable: no email, name or avatar, no financial data.
- Lifetime access survives deletion: registering again with the same email grants it again.
- Billing can learn about wiped users by polling, and cancel subscriptions on its side.
- Inactivity marking is off by default; specific users can be excluded.

## Non-goals

- Cancelling subscriptions inside Econumo (billing does it from the poll; separate billing PR).
- Carrying trials or time-limited access over to a new account.
- Restoring data after a wipe. A wipe is final.
- Reminder emails, a "deleted" email, a "restored" email. The only deletion email is the
  inactivity notice (and the confirmation code of the self-service flow).
- Any MCP surface.

## Anti-goals

- A partner losing data beyond what "stop sharing" and deleting things by hand would do.
- A self-hosted server silently marking family members (inactivity is off by default).
- A table added later escaping the wipe unnoticed.
- A leaked database revealing who used to have an account.

## Configuration (documented in `docs/full.env.example`)

| variable | default | meaning |
|---|---|---|
| `ECONUMO_INACTIVE_USER_DAYS` | `0` | mark users with no activity for this many days; `0` = off |
| `ECONUMO_USER_DELETION_DAYS` | `0` | wipe marked users this many days after the mark; `0` = never automatically (CLI only) |
| `ECONUMO_INACTIVE_USER_EXCLUDE` | empty | comma-separated emails the inactivity job never marks |

Strict parsing like every other setting: negative numbers or malformed values fail at boot.
Exclude entries are compared with `model.NormalizeEmail`. Excluded users can still be marked by
themselves or the CLI.

## Data

### `users` (new columns, both engines, one migration above every released version)

| column | meaning |
|---|---|
| `deletion_marked_at DATETIME NULL` | when the user was marked; NULL = not marked |
| `deletion_reason TEXT NOT NULL DEFAULT ''` | `self_initiated`, `inactive` or `cli` while marked |
| `last_active_at DATETIME NULL` | last sign-in or token use |

`last_active_at` backfill: the newest `access_tokens.last_used_at` when it is within the last 30
days, else the migration time. Expired and revoked tokens are purged by `token:purge`, so old
token rows are no evidence of inactivity; on the day this ships nobody is closer to being marked
than a full threshold.

### `users_audit` (new)

`id`, `user_id` (TEXT, no foreign key), `event`, `email_hash`, `details` (JSON text), `created_at`.
Indexes on `(event, created_at)` and `(email_hash, event)`. Events now:

| event | details |
|---|---|
| `deletion_marked` | `{"reason": "self_initiated" \| "inactive" \| "cli"}` |
| `deletion_cancelled` | `{"reason": <the mark's reason>}` (sign-in removed the mark) |
| `deleted` | `{"reason": <the mark's reason>, "lifetimeAccess": true \| false}` |

`email_hash` = lowercase hex SHA-256 of `model.NormalizeEmail(address)`. More event types can be
added later without a migration. Accepted limit: the hash is unkeyed, so someone with a list of
addresses can test it; plaintext never leaks. A keyed hash was rejected because a lost key would
silently break lifetime re-grants on self-hosted servers.

## Marking

All three paths: under the user row lock, set `deletion_marked_at = now`, `deletion_reason`,
revoke every session, PAT and OAuth grant, write a `deletion_marked` audit row. Marking an
already-marked user is an error for the CLI and impossible from the API (no session survives
the mark).

### 1. Self-service (Settings)

- `POST /api/v1/user/request-user-deletion` `{password}`: verifies the password (rate limited
  like other password checks), then emails a confirmation code with the email-change code
  machinery (hashing, expiry, attempt caps). A passwordless user (provider sign-in only) sends no
  password; the code alone confirms.
- `POST /api/v1/user/confirm-user-deletion` `{code}`: validates the code and marks the user with
  reason `self_initiated`. No email is sent.
- Both are on the readonly allowlist.
- SPA: Settings danger zone, "Delete my data". Dialog: what gets deleted, that signing in within
  the period cancels it, that deleting does not cancel a subscription (with the billing link
  when `ECONUMO_BILLING_URL` is set). Steps: password -> code -> before confirming, the browser
  downloads the CSV export of all the user's transactions (existing
  `GET /api/v1/transaction/export-transaction-list`) -> confirm -> a final screen "Your data
  will be deleted within a month. Sign in before then to keep it." -> local sign-out.
  Offline-disabled via `useOnline`. Analytics event on confirm.
- The CSV download failing does not block deletion, but the UI says so and offers a retry.

### 2. Inactivity

- Daily job (below). When `ECONUMO_INACTIVE_USER_DAYS` > 0, every user with
  `last_active_at` older than that, not marked, `is_active = 1`, and not excluded is
  marked with reason `inactive` and receives ONE email: "Under our policy we delete the data of
  users inactive for <period>. If you want to keep your data, sign in." (11 catalogues; period
  rendered from the setting; includes `ECONUMO_URL` when set).
- A failed send logs a WARN and leaves the user unmarked, to retry on the next pass; nobody is
  marked without the notice going out. The console transport counts as sent.
- Paying users are not exempt.

### 3. CLI

```
user:mark-deleted <email>         # mark with reason cli; no email
user:list-pending-deletion        # marked users: id, email, reason, marked at, auto-wipe date (or "manual")
user:delete <email>               # wipe now; refuses a user who is not marked
user:purge-deleted                # one pass of the wipe job, by hand
```

## Activity and cancelling a mark

`last_active_at` moves on every sign-in and when a token is touched (the existing 5-minute
throttle in `Touch`, so no extra writes). Any of the user's own tokens counts, including
Apple Wallet ingest PATs and MCP OAuth tokens.

A successful sign-in (password login; provider sign-in completing its handoff; the reset-password
flow followed by login) on a marked user clears `deletion_marked_at` and `deletion_reason` and
writes `deletion_cancelled`. Tokens revoked at marking are not restored. Data was never touched.

## Wiping

Runs from the daily job for users marked at least `ECONUMO_USER_DELETION_DAYS` ago (skipped when
0), and from `user:delete`. One database transaction per user:

1. Lock the user row, re-check it is still marked (a sign-in may have cancelled it).
2. Read `lifetimeAccess` = `access_level = full AND access_until IS NULL`, and the email.
3. Disconnect the user from every connected user through the existing delete-connection path
   (account access both directions; budget membership both directions, whose
   `removeMemberRecords` deletes the budget elements, savings rows and account memberships a
   departing member leaves behind - `budgets_elements.external_id` has no foreign key, so the
   cascade alone would leave orphan elements with plans, debts and comments); delete invites.
4. Find the partner accounts the wipe will change: accounts NOT owned by the user that hold
   rows the user wrote (`transactions.user_id`), or that are either side of a transfer with one
   of the user's accounts. Record each one's all-time balance.
5. Delete the transfers from a partner account into one of the user's accounts (the cascade
   would only NULL their recipient, leaving a transfer to nowhere).
6. Debts on a partner's budget whose currency is one of the user's custom currencies switch to
   the budget's currency (`debts.currency_id` is `NO ACTION` and would block the delete).
   Budget elements in that situation fall back by the existing `SET NULL`.
7. `DELETE FROM users WHERE id = ?`. The cascade removes the user's own budgets with everything
   inside them, their accounts with every transaction in them, every transaction they wrote
   into partners' accounts, their comments, categories, tags, payees, labels, folders,
   currencies, recurring templates (including ones on partners' accounts), imports, debts,
   options, identities, tokens, OAuth grants and the OAuth apps they own, and the code/request
   tables.
8. Record each partner account's balance again. Where it changed, insert one correction for
   the difference: an uncategorized income or expense dated today (the owner's local day),
   authored by the account's owner, described like the existing account-deleted correction
   (`internal/account/zero.go`). Partner rows that pointed at the user's categories, payees or
   tags are left with those fields empty by the existing `SET NULL`.
9. Bump the revision of every partner account changed by steps 3-8 once
   (`txrevision`, inside this transaction).
10. Write the `deleted` audit row (old user id, email hash, reason, `lifetimeAccess`).

What the partner gives up: their current balances are exact, but past balances (and the net
worth report) before the correction date shift by the removed amounts, and the user's entries
disappear from their history. A partner's budget recalculates without the user's accounts and
categories; intended - the same end state as the user disconnecting everyone and deleting
everything by hand.

**Guard test**: reads the schema of both engines and fails when a foreign key to `users(id)` is
not `ON DELETE CASCADE`, or when a column that names a user (`user_id`, `owner_user_id`,
`connected_user_id`...) has no foreign key to `users`. A wipe test on a
fixture with a partner (shared account, shared budget both ways, cross transfers, a comment, a
custom currency) asserts the partner's balances and rows; `revisionguard` must pass.

## Registering again

- The old address is free (the row is gone), so registration creates a NEW user with a new id, onboarding, and the normal trial rules.
- Password register, provider sign-up and `user:create`: after the user is created, if the
  latest `deleted` audit row for the address's hash has `lifetimeAccess: true`, the new user gets
  `access_level = full`, `access_until = NULL`. With the email-verification gate on, the grant
  happens when the address is verified, not at registration. Trials and time-limited access are
  never carried over.

## Billing / admin listener

- New `GET /admin/deleted-users?since=<datetime>` on the private admin listener, built like
  `/admin/affiliate-users`: the `deleted` audit rows since the date (`userId`, `deletedAt`,
  `reason`), oldest first. Billing polls it and cancels subscriptions (separate billing PR).
- `/admin/user-context` for a wiped user answers the existing 404; a marked user shows additive
  `deletionMarkedAt`, `deletionReason`.

## Daily job

In-process like `currency/rateupdater`, started by `serve`: shortly after boot, then every 24 h,
single-flight. Order: wipe expired marks, then inactivity marking. Each user is independent; a
failure logs a WARN with the user id (never the email) and continues. `user:purge-deleted` runs
the wipe step only.

## Rules this change must follow

Regression plan items, a `METRICS` key, `useOnline`, i18n parity in 11 catalogues (UI, the
inactivity email, error codes), `docs/full.env.example`, apiparity goldens for the new routes,
both migration engines, revisionguard, slog with ids only, comment rules.

## Approaches considered

- **Wipe**: disconnect, then delete the `users` row and let the database cascade (chosen; a new
  table that cascades from `users` is covered automatically) vs keeping the row as a tombstone
  and deleting root tables in code (an explicit list that must grow with the schema).
- **Partner rows**: one balance correction per changed partner account (chosen; same pattern as
  account deletion, no schema change) vs converting each cross-user row in place (keeps the
  partner's history but needs row-by-row transfer and currency handling) vs dropping the
  foreign key on `transactions.user_id` (SQLite table rebuild, "Deleted" author handling).
- **Disconnect first** (chosen) vs relying on the cascade alone: budget elements point at
  categories and tags without a foreign key, so only the existing member-removal code cleans
  them.
- **Lifetime memory**: generic `users_audit` table (chosen) vs a dedicated lifetime table. The audit table also records the history and takes future events.
- **Subscription cancel**: billing polls `deleted-users` (chosen) vs Econumo calling billing.
- **Scheduling**: in-process daily job + CLI (chosen) vs host cron; the image has no cron.

## Calls made by the designer, for review

- Passwordless users confirm with the code alone.
- The inactivity notice must be sent before the mark is written; a failed send retries daily.
- Sharing is unwound at wipe time, not at marking, so a cancelled mark loses nothing.
- Marking revokes PATs and OAuth grants too; cancelling the mark does not bring them back.
- `user:delete` refuses unmarked users (run `user:mark-deleted` first).
- With the verification gate on, lifetime is re-granted on verification.
- Recurring templates the user made on partners' accounts are deleted with the user.
- Corrections are dated on the partner owner's local today.
- Partner debts in the user's custom currency move to the budget currency.

## Left to the builder

Package layout of the wipe, code length/expiry reuse, job jitter, email copy, the exact
password rate-limit scope.
