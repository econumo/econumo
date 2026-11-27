# Account deletion

Status: design, 2026-11-24.

## Intent

Users can delete their own Econumo account, and accounts nobody has used for a long time are
deleted automatically. Deletion is forgiving: a user-requested deletion can be undone by
signing in within 30 days, and an inactive user is warned three times before anything goes.
Deletion removes the user's financial data but keeps the `users` row, so billing state (paid
access) survives and comes back if the same email registers again.

## Goals

- Self-service deletion from Settings, confirmed with a code emailed to the account address.
- A 30-day grace window: any sign-in within it restores the account as it was.
- After the window, every piece of the user's data is wiped except the `users` row, which is
  marked with the reason.
- A separate inactivity mechanism: users with no activity for N days (configurable, off by
  default) get warning emails 30, 7 and 1 day before, then are wiped with reason `inactive`.
- Paid users can delete too; their `access_level` / `access_until` stay on the row.
- Re-registering with a wiped account's email revives the same row (same id) and keeps its
  billing status. The wiped row stores only a hash of the email, never the address.
- CLI: mark deleted, delete now, list pending, run the purge pass.

## Non-goals

- Exporting data before deletion (separate feature).
- Cancelling a billing subscription (billing owns it; the delete dialog points to it).
- Restoring data after the wipe. A wipe is final.
- Any MCP surface.

## Anti-goals

- Data from another user disappearing because their partner deleted an account, beyond what
  "stop sharing" already does today.
- A self-hosted server silently deleting a family member (hence inactivity off by default).
- A table added later escaping the wipe unnoticed.
- Deleting the `users` row, or its id, access level or billing options.
- A leaked database revealing who used to have an account (no plaintext email, name or avatar
  on a wiped row).

## Decided behaviour

### States on the `users` row

New columns (both engines, one migration version above every released one):

| column | meaning |
|---|---|
| `deleted_at DATETIME NULL` | set when deletion is requested/marked; NULL = live account |
| `deletion_reason TEXT NOT NULL DEFAULT ''` | `user_requested` or `inactive` |
| `purged_at DATETIME NULL` | set when the data was wiped |
| `last_active_at DATETIME NULL` | last time any of the user's tokens was used, or they signed in |
| `inactivity_warnings INTEGER NOT NULL DEFAULT 0` | how many of the 3 inactivity warnings were sent (0-3) |
| `inactivity_wipe_at DATETIME NULL` | the inactivity deadline, fixed when the first warning is sent |

States: live (`deleted_at` NULL), pending (`deleted_at` set, `purged_at` NULL), wiped
(`purged_at` set). `last_active_at` is backfilled by the migration from the newest
`access_tokens.last_used_at` when it is newer than the migration minus 30 days, else the
migration time. `token:purge` deletes expired and revoked tokens, so older token rows are not
evidence of inactivity; anchoring to the migration means nobody is closer to deletion on the
day the feature ships than a full threshold.

### Self-service request (user-requested)

- `POST /api/v1/user/request-account-deletion` (no body fields): emails a 6-digit-style code
  to the account address, same code machinery, hashing, expiry and rate limiting as the
  email-change flow. No password required (passwordless accounts exist).
- `POST /api/v1/user/confirm-account-deletion` `{code}`: validates the code under the user row
  lock, sets `deleted_at = now`, `deletion_reason = user_requested`, revokes every session, PAT
  and OAuth grant, and emails a notice "your account will be deleted on <date>; sign in before
  then to keep it". The SPA then signs out locally.
- Both routes are on the readonly allowlist (a readonly user must be able to delete).
- SPA: a "Delete account" entry in Settings (danger zone), a dialog explaining the 30 days,
  then the code step. Offline-disabled via `useOnline`. Analytics event fired on confirm.

### Restore

Any successful sign-in (password login, provider sign-in completing a handoff) on a pending
account clears `deleted_at`, `deletion_reason`, resets `inactivity_warnings`, sets
`last_active_at`, and proceeds as a normal login. The data was never touched, so the account is
as it was, minus the revoked tokens (PATs and OAuth apps must be re-created; Apple Wallet
shortcut needs a new token). A password reset on a pending account is allowed and is followed
by a login, which restores.

### Inactivity

- Config `ECONUMO_INACTIVE_ACCOUNT_DAYS` (integer, `0` = off, default `0`; cloud sets 365).
  Documented in `docs/full.env.example`. Values 1..30 are refused at boot (no room for the
  30-day warning) — builder may pick the exact minimum.
- Activity = `last_active_at`, refreshed when a token is touched (already throttled to once per
  5 minutes in `Touch`, so the extra write rides the same throttle) and on every sign-in.
- With threshold T days and `last_active_at` = L: once now >= L+T-30 and no warning was sent,
  the first warning ("will be deleted in 30 days") is sent and `inactivity_wipe_at` is fixed at
  send time + 30 days. The deadline is anchored to the first warning ACTUALLY sent, never to
  L+T, so a user found long past the threshold (feature just enabled, job down for a week) still
  gets the full 30 days.
- The second ("in 7 days") goes out at `inactivity_wipe_at - 7d`, the third ("tomorrow") at
  `inactivity_wipe_at - 1d`. A late pass sends only the latest due warning, never a burst, and
  records it in `inactivity_warnings`.
- A failed send does not advance `inactivity_warnings`. The wipe happens at
  `inactivity_wipe_at` only when the third warning was sent at least 20 hours earlier;
  otherwise `inactivity_wipe_at` moves to the third warning's send time + 1 day. The console
  transport counts as sent.
- Any activity (token touch, sign-in) clears `inactivity_warnings` and `inactivity_wipe_at`.
- At the deadline the account is wiped directly with `deletion_reason = inactive` (`deleted_at`
  and `purged_at` both set to now). No extra grace window: the warnings were the grace.
- Users already pending (user-requested) are skipped by the inactivity pass.
- Users with `is_active = 0` (deactivated by an admin) are skipped too.
- Paying users: OPEN, see Open questions.

### Wipe

Runs for: pending users whose `deleted_at` is 30+ days old, inactive users at their deadline,
and `user:delete`. One database transaction per user:

1. Lock the user row; re-check the state (a sign-in may have restored it).
2. Stop-sharing unwind with every connected user: the existing delete-connection path
   (account access both directions, budget access both directions, the connection link), and
   delete pending invites.
3. Bump the transaction revision of every account NOT owned by the user whose transactions
   reference the user's categories, payees, tags or labels (they become NULL / unlinked via FK),
   or that is either side of a transfer with one of the user's accounts, using
   `txrevision.BumpWhere` before the rows change.
3a. Cross-owner transfers keep the partner's balance intact (nothing in the code handles a
   missing transfer side today, so the wipe resolves both cases explicitly instead of leaving
   it to the FKs):
   - partner's account -> user's account: the row becomes an expense on the partner's account
     (same amount, recipient columns cleared, no category);
   - user's account -> partner's account: the row moves to the partner's account as an income
     (`account_id` = the partner's account, `amount` = the old `amount_recipient`,
     `sort_key` = the old `recipient_sort_key`, recipient columns cleared, no category).
   Both keep date, description, payee/tag only if they belong to the partner (else NULL).
4. Delete everything the user owns from an explicit ordered table list: accounts (and their
   transactions, including ones others wrote into them), budgets, folders, categories, payees,
   tags, labels, recurring transactions, imports (sources, runs, events, links, rules,
   suggestions, credential keys), debts, comments, options except the kept ones, identities,
   access tokens, OAuth grants/codes, password/email/verification requests, hidden currencies,
   idempotency keys, operation ids.
5. Transactions the user WROTE into someone else's account stay (as with stop sharing today).
6. Set `purged_at = now` (and `deleted_at`, `deletion_reason` when wiping directly).

The user's own accounts and every transaction in them are hard-deleted, the same end state as
the user disconnecting everyone and then deleting everything by hand. A partner's budget that
counted those accounts, or had elements on the user's categories, recalculates without them;
that is intended.

Kept on the row: id, `access_level`, `access_until`, `created_at`, the deletion columns, and
the user options `affiliate`, `affiliate_code`. Scrubbed: `email` becomes
`deleted:` + lowercase hex SHA-256 of `model.NormalizeEmail(address)` (not a valid address, so
it never matches a login and stays unique), `name` and `avatar` blanked, `password` and `salt`
emptied with `algorithm = none`, `timezone` and `language` reset to defaults, `email_verified`
0, `credentials_generation` bumped. Accepted limit: the hash is unkeyed, so someone holding a
list of addresses can test it; plaintext never leaks. A keyed hash was rejected because a lost
key would silently break revival on self-hosted servers.

**Guard test**: reads the live schema (SQLite and PostgreSQL) and fails if a table with a
foreign key to `users(id)` (directly, or a column named `user_id`) is neither in the wipe list
nor in an explicit "kept / reached by cascade" list with a reason. `revisionguard` must pass
for the wipe.

### Re-registration of a wiped email

A wiped row is found by `email = 'deleted:' || sha256hex(normalized address)`, checked only
when no live row has the address.

- Password register (and CLI `user:create`): if the hash matches a wiped row, the same row is
  revived with the plaintext email restored: new name,
  password hash, avatar, language/timezone, `email_verified` per the normal register rules,
  `deleted_at`/`deletion_reason`/`purged_at` cleared, `last_active_at = now`, default options
  re-seeded (onboarding starts fresh), `credentials_generation` bumped. `access_level` and
  `access_until` are kept: no new trial. The register response stays byte-identical to today.
- The billing status carries over only through a verified address: when the instance has the
  email-verification gate on, the revived row keeps its access only once the address is
  verified (until then it behaves as a fresh unverified registration would); with the gate off
  it carries over at once, matching how registration works there today.
- Login, `remind-password` and `reset-password` on a wiped address behave exactly as for an
  unknown address.
- Another user may change their email to a wiped address (the unique key no longer blocks it).
  Confirming that change rewrites the wiped row's email to `orphaned:<row id>` in the same
  transaction: the address now belongs to a live account, so the wiped row can never be
  revived, and when that live account is later wiped its `deleted:<hash>` cannot collide. The
  wipe also checks defensively: if another row already holds the same `deleted:<hash>`, that
  older row is rewritten to `orphaned:<its id>` first. The same applies when the CLI
  `user:change-email` targets a wiped address.
- `set-access` (admin) on a wiped user works as usual.
- Provider sign-in with a wiped email: same revival, then the identity is linked.
- If the email belongs to a pending (not yet wiped) row, register answers "user already exists"
  as today; the user signs in to restore.

### CLI

```
user:mark-deleted <email>         # pending, reason user_requested, tokens revoked, notice emailed
user:delete <email>               # wipe now (live or pending)
user:list-pending-deletion        # pending users + inactive users with a warning sent, with dates
user:purge-deleted                # one pass of the daily job
```

### Daily job

In-process, like `currency/rateupdater`: starts with `serve`, runs shortly after boot then
every 24 h, single-flight. Pass order: purge expired pending users, then inactivity warnings and
inactivity wipes. Each user is processed independently; a failure logs a WARN with the user id
and continues. Never runs inside a request.

### Billing / admin surface

Billing identifies users by id, so a wiped row keeps working for billing. `/admin/user-context`
gains additive fields `deletedAt`, `deletionReason`, `purgedAt`; for a wiped row it returns
`email` and `name` as empty strings (never the `deleted:` hash). `/admin/affiliate-users`
keeps listing wiped users (it returns ids, not emails). A wiped user's access is unchanged.

### Emails (all 11 catalogues)

Deletion code, deletion scheduled notice (with date), inactivity warning (30 / 7 / 1 day
variants or one template with a plural), and optionally "account restored". No email after the
wipe.

## Approaches considered

1. **One purge in storage with a schema guard test (chosen).** Explicit ordered table list,
   single transaction, guard proves completeness. Picked for having one place that proves
   nothing was missed.
2. Per-feature purge methods wired through server glue. Cleaner on the dependency rule but ~15
   new ports and no single completeness proof.

Scheduling: in-process daily job plus CLI (chosen) vs host cron only — the image is distroless
with no cron, and self-hosters should not need extra setup.

## Rules this change must follow

Regression plan items, `METRICS` key, `useOnline`, i18n parity, `docs/full.env.example`,
apiparity goldens for the two new routes, both migration engines, revisionguard, logging with
ids only (never emails), comment rules.

## Left to the builder

Exact package layout of the purge (user repo vs an infra package), the job's jitter, code
length/expiry reuse, the minimum allowed inactivity threshold, email copy.

## Open questions

1. Are paying users exempt from the inactivity wipe? Econumo's own columns cannot tell a paid
   user from a trial (both are `full` with a future `access_until`); only billing's get-access
   (`paid`, `renewing`) can. Options: no exemption (matches "paid users can be deleted"), or
   exempt when billing reports `paid` or `renewing` (the job asks billing, only when
   `ECONUMO_BILLING_URL` is set; a billing failure postpones that user, never wipes).

Calls made by the designer (not discussed explicitly), for review:

- Any use of the user's OWN tokens counts as activity (Apple Wallet ingest, MCP included);
  partner activity on shared accounts does not.
- Sharing is unwound at wipe time, not at the request, so a restore is complete.
- The request revokes sessions, PATs and OAuth grants; a restore does not bring them back.
- The delete dialog tells users with full access that deleting does not cancel a
  subscription, with a link to billing when `ECONUMO_BILLING_URL` is set. No outbound billing
  call.
- Grace window fixed at 30 days (not configurable).
- No email after the wipe.
- Cross-owner transfers are converted so the partner's balance does not change (Wipe 3a).
- The inactivity deadline is anchored to the first warning sent; the backfill never puts anyone
  closer than a full threshold on the day the feature ships.
- A wiped row whose address is taken by a live account becomes `orphaned:<id>` and is never
  revived.
