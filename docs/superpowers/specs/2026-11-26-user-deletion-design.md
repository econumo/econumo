# User deletion

Status: design, 2026-11-26. Revised the same day after a review against the code (currency
cascade, three-user disconnect, debts, lifetime re-grant, activity, guard test).

## Intent

A user can delete their Econumo data, and users nobody has used for a long time are marked for
deletion automatically. Deletion is a two-step process: the user is MARKED, and later WIPED.
Any sign-in between the two removes the mark. Wiping first unwinds everything the user shares
(the existing member-removal and grant-unwind code), then deletes the `users` row and lets the
database cascade remove everything the user owns or wrote. Every transaction removed from a
partner's account is replaced by a correction on the same date, so the partner's balances stay
right on every day. An audit table records the deletion history and lets a returning
lifetime-access customer get lifetime access back, once.

## Goals

- Three ways to mark: self-service in Settings, inactivity, CLI.
- Wiping after a configurable delay (`ECONUMO_USER_DELETION_DAYS`, 0 = never automatically),
  or by hand with the CLI.
- A sign-in on a marked user removes the mark.
- After a wipe nothing personal remains readable: no email, name or avatar, no financial data.
- Lifetime access survives deletion: registering again with the same email grants it again,
  one time per deletion and only to someone who proved the mailbox.
- Billing can learn about wiped users by polling, and cancel subscriptions on its side.
- Inactivity marking is off by default; specific users can be excluded; a user connected
  to an active user is never marked or wiped for inactivity.
- A wipe either completes or changes nothing, and a wipe that cannot complete is visible to the
  operator.

## Non-goals

- Cancelling subscriptions inside Econumo (billing does it from the poll; separate billing PR).
- Carrying trials or time-limited access over to a new account.
- Restoring data after a wipe. A wipe is final.
- Reminder emails, a "deleted" email, a "restored" email. The only deletion email is the
  inactivity notice (and the confirmation code of the self-service flow).
- Any new MCP tool. One additive field does show on `/mcp`: `list_connections` returns the
  same result as `get-connection-list`, so it carries `deletionMarkedAt` too.

## Anti-goals

- A partner losing data beyond what "stop sharing" and deleting things by hand would do.
- Two remaining users losing what they share with EACH OTHER because a third user was wiped.
- A self-hosted server silently marking family members (inactivity is off by default).
- A table, or a foreign key two hops away from `users`, added later and escaping or blocking
  the wipe unnoticed.
- A leaked database revealing who used to have an account. One accepted exception: an unkeyed
  email hash for deleted LIFETIME customers only (see `users_audit`).

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
| `last_active_at DATETIME NULL` | last credential mint or token use, to the day |

`last_active_at` backfill: the migration time, for every existing user. Token rows are no
evidence either way (expired and revoked ones are purged by `token:purge`), and any older
stamp would break the promise that on the day this ships nobody is closer to being marked
than a full threshold: with a 7-day threshold, a token last used 20 days ago would mark its
user on the first pass. A new user gets `last_active_at = created_at`, so an account that was
registered and never signed in ages from its registration; the job reads
`COALESCE(last_active_at, created_at)`.

None of the three columns is part of the generic `UpsertUser` save. Each is written only by its
own statement: `ClearActiveBudget` (`internal/server/glue_budget.go`) reads and saves a user row
without the row lock, and a stale whole-row save must not be able to erase a mark or move the
activity stamp back.

### `users_audit` (new)

`id`, `user_id` (TEXT, no foreign key), `event`, `email_hash` (NULL except below), `details`
(JSON text), `created_at`. Indexes on `(event, created_at)` and `(email_hash, created_at)`.
Events now:

| event | details | `email_hash` |
|---|---|---|
| `deletion_marked` | `{"reason": "self_initiated" \| "inactive" \| "cli"}` | NULL |
| `deletion_cancelled` | `{"reason": <the mark's reason>, "cause": "sign_in" \| "active_connection" \| "inactivity_off"}` | NULL |
| `deleted` | `{"reason": <the mark's reason>, "lifetimeAccess": true \| false}` | set only when `lifetimeAccess` is true |
| `lifetime_regranted` | `{"fromUserId": <the wiped user's id>, "deletedEventId": <id of the `deleted` row it consumes>}`; `user_id` is the NEW user | set |

`email_hash` = lowercase hex SHA-256 of `model.NormalizeEmail(address)`. It is stored only where
the re-grant needs it: a user who was not a lifetime customer leaves no trace of their address.
More event types can be added later without a migration. Accepted limit: the hash is unkeyed,
so someone with a list of addresses can test it against the lifetime rows; plaintext never
leaks. A keyed hash was rejected because a lost key would silently break lifetime re-grants on
self-hosted servers.

## Marking

All three paths: under the user row lock, set `deletion_marked_at = now`, `deletion_reason`,
bump `credentials_generation`, revoke every access token of all three kinds (session, PAT,
OAuth) AND every OAuth grant, write a `deletion_marked` audit row. Both sweeps are needed, as
in `AdminDeactivate`: `Authenticate` reads the `access_tokens` row, not the grant, so revoking
only the grant would stop refreshes while an issued OAuth bearer kept working for up to an
hour. A test presents the old bearer right after the mark and expects the frozen 401. The
generation bump is also what `AdminDeactivate` does, for this reason: a
password login, a provider handoff or an OAuth code exchange that read its evidence before the
mark must not be able to mint a credential after it (the inserts are conditional on the
generation). The invariant: a marked user has no live credential, and the only mint that can
reach one is a fresh session, which clears the mark (below). Marking an already-marked user is
an error for the CLI and impossible from the API (no session survives the mark).

### 1. Self-service (Settings)

- `POST /api/v1/user/request-user-deletion` `{password}`: verifies the password (rate limited
  like other password checks), then emails a confirmation code with the email-change code
  machinery (hashing, expiry, attempt caps). A passwordless user (provider sign-in only) sends no
  password; the code alone confirms.
- `POST /api/v1/user/confirm-user-deletion` `{code}`: validates the code and marks the user with
  reason `self_initiated`. No email is sent.
- Both are on the readonly allowlist.
- SPA: Settings danger zone, "Delete my data". Dialog: what gets deleted, that signing in within
  the period cancels it, that a subscription keeps running until the data is actually deleted
  and can be cancelled sooner in billing (with the billing link when `ECONUMO_BILLING_URL` is
  set), and that people the user shares with keep their own data but lose the shared accounts
  and budgets. Steps: password -> code -> before confirming, the browser
  downloads the CSV export of all the user's transactions (existing
  `GET /api/v1/transaction/export-transaction-list`) -> confirm -> a final screen "Your data
  will be deleted within a month. Sign in before then to keep it." -> local sign-out.
  Offline-disabled via `useOnline`. Analytics event on confirm.
- The CSV download failing does not block deletion, but the UI says so and offers a retry.
- The existing export lists live accounts only (`ListExportAccountsForUser` skips deleted
  ones). The dialog says so in plain words: "The file contains the transactions of your
  current accounts. Accounts you deleted earlier are not in it."

### 2. Inactivity

- Daily job (below). When `ECONUMO_INACTIVE_USER_DAYS` > 0, every user with
  `last_active_at` older than that PLUS ONE DAY, not marked, `is_active = 1`, not excluded,
  and with no
  ACTIVE connected user is marked with reason `inactive` and receives ONE email: "Under our
  policy we delete the data of users inactive for <period>. If you want to keep your data,
  sign in." (11 catalogues; period rendered from the setting; includes `ECONUMO_URL` when set).
- Household rule (agreed 2026-11-26): a user is neither marked nor wiped for inactivity while
  a connected user is active. "Active connection" has one definition, used everywhere below:
  a row in `users_connections` to a user who is NOT marked and whose own `last_active_at` is
  within `ECONUMO_INACTIVE_USER_DAYS` plus one day. The extra day, here and in the first
  bullet, covers the stamp's precision: real use can be up to 24 hours newer than the stamp,
  and both errors must fall on the side of keeping data. A marked partner does not count:
  someone who just asked
  for their own deletion must not keep the other alive. Direct connections only, shared data
  or not. The owner of a budget or account may never sign in while their partner uses that
  data every day; inactivity is a property of the household, not of one login.
- The rule guards three moments, because a household can change after the mark:
  1. marking: the job skips a user with an active connection;
  2. release: every daily pass cancels `inactive` marks of users who now have an active
     connection (`deletion_cancelled`, cause `active_connection`, no email). Typical case:
     both partners were marked, one signed in. This step runs whatever
     `ECONUMO_USER_DELETION_DAYS` is, so the pending list and the partner's Inbox row stay
     true when wipes are manual;
  3. wipe: the same check runs inside the wipe transaction (step 1), for the job,
     `user:purge-deleted` and `user:delete` alike. An `inactive` mark with an active connection
     is cancelled instead of wiped.
- Inactivity switched off: with `ECONUMO_INACTIVE_USER_DAYS` back at 0 the policy that made
  these marks is gone, and "active connection" has no window left to measure. The daily pass
  then releases EVERY `inactive` mark (cause `inactivity_off`), and a wipe refuses an
  `inactive` mark the same way. Other reasons are untouched.
- When everyone in a household is inactive, each member is marked and emailed. If the household
  goes quiet again after a release, its members are marked and emailed again.
- The rule is about the `inactive` reason only. A user who asked for deletion themselves is
  wiped whatever their partners do, and so is a user marked from the CLI.
- A failed send logs a WARN and leaves the user unmarked, to retry on the next pass; nobody is
  marked without the notice going out. The console transport counts as sent.
- Paying users are not exempt.

### 3. CLI

```
user:mark-deleted <email>         # mark with reason cli; no email
user:list-pending-deletion        # marked users: id, email, reason, marked at, auto-wipe date (or "manual")
user:delete <email>               # wipe now; refuses a user who is not marked
user:purge-deleted                # one pass of the release + wipe job, by hand
```

`user:delete` on an `inactive` mark that the household rule cancels says so ("not deleted: a
connected user is active; the mark was removed") and exits non-zero. An operator who wants
that user gone anyway runs `user:mark-deleted` (reason `cli`, outside the rule) and then
`user:delete`: the override is a deliberate second command, not a flag.

`user:delete` prints why a wipe failed and exits non-zero. `user:purge-deleted` continues past
a failed user, prints one line per failure (user id and the reason) and exits non-zero when any
failed. With `ECONUMO_USER_DELETION_DAYS=0` nothing is wiped by age, so `user:delete` is the
only way a wipe runs and its exit code is how a stuck wipe is noticed.

## Activity and cancelling a mark

`last_active_at` moves on every credential mint (a new session, an OAuth code exchange, an
OAuth refresh) and when a token is touched. Any of the user's own tokens counts, including
Apple Wallet ingest PATs and MCP OAuth tokens. The refresh matters: an OAuth access token is
minted with `last_used_at = now` and the touch only fires five minutes later, so an MCP client
that works in short bursts would otherwise never register as active.

The stamp has day precision. It is one conditional statement,
`UPDATE users SET last_active_at = :now WHERE id = :id AND (last_active_at IS NULL OR
last_active_at < :now - 24h)`, issued next to the token write (the NULL arm matters: without
it a row that was never stamped could not be stamped, and an active user would age from
`created_at`): at most one extra write per user per day (the earlier "no extra
writes" was wrong: `Touch` writes `access_tokens`, not `users`). It is a blind, monotonic
update, so it is the one write to a user row that does not take the row lock first; on
PostgreSQL it waits behind a holder of that lock, which is correct.

Session creation (`createSession`, the one place sessions are minted: password login, provider
handoff, login after a password reset) clears `deletion_marked_at` and `deletion_reason` on a
marked user and writes `deletion_cancelled` (cause `sign_in`), inside the row lock it already
takes. No sign-in flow is listed by hand, so a flow added later cancels the mark too. The
other way a mark goes is the household rule (cause `active_connection`, see Inactivity).
Either way tokens revoked at marking are not restored, and data was never touched.

## What connected users see while a user is marked

`get-connection-list` carries an additive `deletionMarkedAt` on each connected user (empty when
not marked). The Inbox "Sharing" section shows one row per marked connection: "<name>'s data is
scheduled for deletion. The accounts and budgets they share with you will disappear, and
transfers with their accounts will become corrections." No date in the copy: the wipe may be
manual. The row disappears when the mark is cancelled or the user is wiped. It is the only
notice a partner gets; without it the first sign is missing accounts. 11 catalogues; no email.

## Wiping

Runs from the daily job for users marked at least `ECONUMO_USER_DELETION_DAYS` ago (skipped when
0), and from `user:delete`. One database transaction per user: it commits whole or rolls back
whole.

### Prerequisite fixes (own commits, each with its regression test)

Both are bugs today, independent of deletion, that the wipe would spread.

**1.** Today's delete-connection removes a third user. `ConnectionBudgetRevoker.revokeDirection`
(`internal/server/glue_connection.go`) walks `ListForUser`, which returns budgets the user owns
AND budgets shared with them. So when A and B are both members of C's budget, disconnecting A
from B removes B from C's budget and deletes B's rows there (reproduced with a throwaway test
during the review). The fix: only budgets OWNED by one of the two users are touched.

**2.** `ClearActiveBudget` (`internal/server/glue_budget.go`) reads a user and saves the whole
row through `UpsertUser` without the row lock. On PostgreSQL a password or email change that
commits between that read and that save is written back to its old value. The wipe calls this
for every member of every budget it unwinds. The fix: clear the `budget` option with one
conditional statement on `users_options` (`... WHERE user_id = ? AND name = 'budget' AND value
= ?`), never through the user aggregate.

### Lock order (a requirement; the plan proves it with a two-connection PostgreSQL test)

1. Every budget the wipe will touch, in id order, through the debt feature's budget lock. Its
   `Prepare` must be first in a transaction (`internal/category/ports.go`), so budgets come
   before the user row.
2. The user row. The mark is re-checked after this lock, never before.
3. Every account the user owns, in id order (`FOR UPDATE` on PostgreSQL). A transaction
   write on an account bumps that account's revision and takes foreign-key locks on its row,
   so it now waits for the wipe. Without this, a partner whose access the wipe has unwound
   but not yet committed could still commit a transfer from or into the user's account
   after step 6 scanned it; the cascade would then delete that transfer with no correction
   on the partner's side. After the wipe commits, the waiting write fails on the missing
   account. The two-connection test covers exactly this interleaving.
4. Everything else. Partner user rows are never locked first; the wipe only writes them late
   (the active-budget pointer), the same order `RevokeAccess` already uses.

The set of budgets is read before it is locked. A budget that joins the set in that window, a
deadlock and a serialization failure all end the same way: the transaction rolls back and the
user stays marked for the next pass or the next `user:delete`.

### Steps

1. Take the locks above; re-check the user is still marked (a sign-in may have cancelled it).
   An age-based wipe (the job, `user:purge-deleted`) also requires `deletion_marked_at` to be
   the value it selected: a user who signed in and was marked again while the wipe waited
   has a new mark whose period has just started. `user:delete` has no such condition.
   For reason `inactive`, check the household rule here, under the lock: with an active
   connection the mark is cancelled, the transaction commits that and nothing else.
2. Read `lifetimeAccess` = `access_level = full AND access_until IS NULL`, and the email.
3. Unwind sharing, selected by ACCESS ROWS, not by `users_connections` (an access row without a
   connection must not survive as an orphan):
   - every budget the user is a member of: the existing `RemoveMember`, whose
     `removeMemberRecords` deletes the elements, savings rows, account memberships and envelope
     links the member leaves behind (`budgets_elements.external_id` has no foreign key, so the
     cascade alone would orphan them with their plans and comments);
   - every budget the user owns: each member's active-budget pointer is cleared (the budget
     itself goes in step 9);
   - every account grant in either direction: the existing `unwindGrant` (folder placement,
     per-account options, the grant row);
   - invites and connection rows go with the cascade.
   The pairwise `DeleteConnection` is not called: the wipe needs "everything of this user",
   never "everything between these two".
4. Debts on the user's categories in OTHER users' budgets are deleted through the debt
   feature's `Release`, exactly as deleting the category does. `removeMemberRecords` keeps such
   a debt on purpose ("shows again if the category returns"); here the category never returns,
   and `debts.element_external_id` has no foreign key, so without this step the rows stay
   forever.
5. Rows the user wrote into another user's account (`transactions.user_id` = the user,
   `account_id` owned by someone else) stay as they are: a row written into another user's
   account already uses that account owner's categories, payees, tags and labels
   (`checkReferences`, `internal/transaction/usecase.go`). The wipe only reassigns `user_id` to
   the owner of `account_id`, the SOURCE account; everything else (description, date, order,
   amounts, category, payee, tag, labels) is untouched. A transfer the user wrote between two
   other users' accounts follows the same rule. Selection is by these columns alone, so an
   EX-partner's account is covered: a row left with `user_id` = the user would be deleted by
   the cascade.
6. Transfers between one of the user's accounts and another user's account lose their other
   side, so the surviving side becomes a correction on that account:
   - other account -> user's account: an expense on the other account for `amount`;
   - user's account -> other account: an income on the other account for `amount_recipient`.
     A legacy row with a NULL `amount_recipient` gets NO correction: the balance query sums
     `amount_recipient`, so that transfer never counted on the recipient's balance, and an
     income for `amount` would add money that was never there.
   The correction keeps the original's `spent_at` and its position in the day on that side
   (`sort_key`, or `recipient_sort_key` for the recipient side), is authored by the account's
   owner, and its description is "Original transaction deleted: <original description>" (just
   "Original transaction deleted" when empty; truncated to 255). Transfers carry no category,
   payee or tag. Then the originals are deleted (the cascade would remove the second kind and
   only NULL the recipient of the first). Wording of the note is English, like the existing
   account-deleted correction (`internal/account/zero.go`).
7. Custom currencies someone else still uses are MOVED, not deleted. A currency of the user
   referenced by a row another user owns (an account, a budget, a budget element or a debt in
   their budget) gets `user_id` = that owner (the lowest user id when several). If the new
   owner already has a live custom currency with the same code, the moved one arrives
   soft-deleted (`is_deleted = 1`): it keeps resolving for the rows that use it, and the unique
   index on live `(user_id, code)` is not hit. Amounts, rates and rate history are untouched.
   The currency list query (`GetUserCurrencyListView`) shows a foreign custom currency only
   through access rows; it is extended to the currencies used by budgets the caller OWNS
   (budget, element and debt currency), so a second surviving owner still gets the currency
   and its rates from the API. That also closes the same gap for a budget owner today.
   Why: `currencies.user_id` cascades, `budgets.currency_id` is NOT NULL with `SET NULL` and
   `debts.currency_id` is `NO ACTION`, so the delete in step 9 fails on both engines; and
   `accounts.currency_id` cascades, so a partner's account in the user's currency (prevented
   today only by `EnsureUsable`, not by the schema) would be deleted with all its rows.
8. Rows the cascade cannot reach, deleted or cleared here: `idempotency_keys` and
   `oauth_states` of the user (no foreign key); `applied_category_id`, `applied_payee_id`,
   `applied_tag_id`, `applied_rule_id` on other users' import links that point at the user's
   entities. OAuth apps the user owns are deleted through the authserver's existing delete
   path, which revokes the access tokens of every grant: on an upgraded database an app can
   have grants of OTHER users (migration `20261108000000` gave each app to its earliest
   grantee), the cascade would drop those grants, and their issued bearers, which have no
   foreign key to the grant, would keep working until they expire.
9. `DELETE FROM budgets WHERE user_id = ?`, then `DELETE FROM users WHERE id = ?`. The budgets
   go first on purpose: with the user's own budget in the user's own custom currency, SQLite
   fires the currency's `SET NULL` on the still-present budget and fails, while a fresh
   PostgreSQL happens to delete the budget first. Nothing may depend on that order. The cascade
   then removes the user's accounts with every transaction in them, their comments, categories,
   tags, payees, labels, folders, remaining currencies, recurring templates (including ones on
   other users' accounts, and other users' templates that transfer into the user's accounts),
   imports, options, identities, tokens, OAuth grants and the OAuth apps they own, and the
   code/request tables.
10. Rows of other users that pointed at the user's categories, payees or tags are left with
    those fields empty by the existing `SET NULL`. A test asserts every other user's account
    balance at every date is unchanged by the wipe.
11. Every other user's account changed by steps 3-10 gets its revision bumped once. The
    affected account ids are registered BEFORE the rows change or go (`txrevision.BumpWhere`
    in the step that changes them): after a reassignment or a cascade no predicate finds
    them any more. `txrevision` deduplicates and flushes once before COMMIT.
12. Write the `deleted` audit row (old user id, reason, `lifetimeAccess`, and the email hash
    only when `lifetimeAccess` is true).

What a partner sees: transfers with the user's accounts become corrections on the same dates
(balances on every day, net worth and account order unchanged; the original description kept).
Rows the user wrote into the partner's accounts stay whole, now authored by the partner, but
lose their import link (the user's import source is gone), so a later import of the same bank
data is matched by the matcher, not by the link. A custom currency of the user that the
partner's budget used is now the partner's. A partner's budget recalculates without the user's
accounts and categories, and a debt tracked on one of the user's categories is gone. A debt on
the partner's own element loses the payment splits that came from transactions on the user's
accounts (`debts_payments.transaction_id` cascades), because those payments no longer exist in
the budget; the plan checks how such a debt's balance reads afterwards. All intended -
the same end state as the user disconnecting everyone and deleting everything by hand. Two
partners who share with each other keep everything they share.

### When a wipe fails

Nothing is half-applied. The job logs one ERROR per failed user (user id and an error type,
never the email or the message of a constraint error) and a summary line with the counts of
wiped and failed users; `user:delete` and `user:purge-deleted` exit non-zero (see CLI). The user
stays marked and is tried again. No failure is expected in normal operation: a failing wipe is
a bug in this change or a schema change the guard test missed, which is why it is an ERROR.

### Guard tests

**Schema guard** (both engines). It walks the foreign-key graph from `users` and fails when:

- a foreign key to `users(id)` is not `ON DELETE CASCADE`;
- anywhere in the set of tables a user delete can reach, a foreign key is `NO ACTION`/`RESTRICT`,
  or `SET NULL` on a NOT NULL column, and is not on the handled list. The list today, computed
  from the schema at this revision, is exactly `budgets.currency_id` and `debts.currency_id`,
  each naming the step that clears it (7 and 9). The remaining `SET NULL` keys are on nullable
  columns and are intended;
- an id-like column has no foreign key and is not on the second list, which names for each
  what the wipe does with it: `users_audit.user_id` (kept on purpose), `idempotency_keys.user_id`
  and `.resource_id` (step 8), `oauth_states.link_user_id` (step 8),
  `budgets_elements.external_id` (step 3), `debts.element_external_id` (step 4),
  `import_transaction_links.applied_*` (step 8), `import_suggestions.category_id`/`payee_id`
  and `access_tokens.grant_id` (the row is the user's own and cascades), and the external ids
  of import providers (not Econumo ids).

A new table, key or id column therefore fails the build until someone decides what the wipe
does with it.

**Wipe test** (both engines, through `dbtest`). Fixture: the user; a partner with a shared
account and a shared budget each way; an EX-partner holding rows the user wrote; a third user
who shares a budget with both the user and the partner; cross transfers in both directions; a
comment; a debt on the user's category in the partner's budget; a legacy transfer with a NULL
`amount_recipient`; an OAuth app the user owns that another user also has a grant on; the
user's custom currency as
the currency of their own budget, of the partner's budget, of a partner budget element and of
a partner debt. It asserts: the delete succeeds; every other user's balance at every date; the
rows and authors of steps 5-6; the partner budget and debt still resolve their currency; the
partner and the third user still share their budget; no row anywhere names the wiped id except
`users_audit`; `revisionguard` passes.

## Registering again

- The old address is free (the row is gone), so registration creates a NEW user with a new id,
  onboarding, and the normal trial rules.
- Lifetime means `access_level = full` with `access_until = NULL`, as defined in
  `2026-07-19-cloud-monetization-trial-access-design.md` ("paid (one-off / lifetime core)").
  The same pair is the default on a self-hosted server and for CLI-created users, where the
  re-grant changes nothing. Assumption to confirm with billing before building: a SUBSCRIPTION
  always carries an `access_until`; if billing ever writes `full` + NULL for a subscriber, that
  subscriber would be remembered as lifetime.
- The re-grant: when the address's hash has a `deleted` row with `lifetimeAccess: true` that
  no `lifetime_regranted` row names in `deletedEventId`, the new user gets
  `access_level = full`, `access_until = NULL`, and a `lifetime_regranted` row naming that
  `deleted` row is written in the same transaction. The match is by that reference, never by
  which row is newer: `created_at` has one-second precision, and a wipe and a sign-up can
  share a second. So the same deletion never grants twice: without it, a customer could re-register,
  move the new account to another address, and register the old address again for a second
  lifetime account, without end. Deleting the re-granted account writes a new `deleted` row and
  re-arms it.
- The re-grant needs proof of the mailbox, because the hash is all that ties the new user to
  the old one. It happens at the moment the proof exists: a provider sign-up (the provider
  verified the address), the verification code being confirmed when
  `ECONUMO_EMAIL_VERIFICATION` is on, or `user:create` (the operator vouches). A password
  registration with the gate off proves nothing (`NewUser` starts with `email_verified = true`)
  and gets no automatic re-grant. The operator's `user:verify-email <email>` vouches for the
  mailbox and runs the same re-grant, consuming the deletion. `user:set-access` is NOT the
  way: it would leave the deletion unconsumed, ready to grant a second account.
- Trials and time-limited access are never carried over.

## Billing / admin listener

- New `GET /admin/deleted-users?since=<datetime>` on the private admin listener, built like
  `/admin/affiliate-users`: the `deleted` audit rows since the date (`userId`, `deletedAt`,
  `reason`, `lifetimeAccess`), oldest first, with the same composite cursor as
  `ListAffiliateUsers` (`since` plus an after-id): a batch of wipes shares one second, and a
  page that ends inside it must neither skip nor repeat rows. Billing polls it and cancels
  subscriptions
  (separate billing PR). `lifetimeAccess` is there so billing keeps a lifetime purchase instead
  of treating it as cancelled.
- `/admin/user-context` for a wiped user answers the existing 404; a marked user shows additive
  `deletionMarkedAt`, `deletionReason`.
- Open item for the billing PR, not decided here: a re-granted user has full access in Econumo
  under a new id that billing has never seen, so the billing view shows no purchase. Billing
  needs the link from the new id to the old one (the `lifetime_regranted` row has both); the
  field that carries it is designed with the billing change.

## Outside the database

The wipe covers Econumo's database. It does not reach, and the user-facing text must not
promise:

- analytics events (a hashed user id and a synthetic host; never an email or a name);
- what the user sent through the feedback form;
- the mail provider's delivery logs;
- billing's own records (billing learns of the wipe from the poll and applies its own rules);
- database backups;
- server logs. Econumo's own log lines carry ids only, with one exception: the console mail
  transport (the default when `MAILER_DSN` is empty) prints each message to stdout with its
  recipient and body, so the deletion code email and the inactivity notice leave the address
  in the container log.

Browser data: the device that confirms the deletion signs out locally. Other devices keep
their cached copy until their next request is refused; the plan verifies that the existing
refused-token path clears IndexedDB and the persisted query cache.

## Daily job

In-process like `currency/rateupdater`, started by `serve`: shortly after boot, then every 24 h,
single-flight. Order: release `inactive` marks the household rule no longer allows, wipe
expired marks (skipped when `ECONUMO_USER_DELETION_DAYS` is 0), then inactivity marking. The
release runs first so a partner who came back saves the other before any wipe is tried. Each
user is independent; a failed release or inactivity mark logs a WARN with the user id (never
the email) and continues, a failed wipe logs an ERROR (see "When a wipe fails").
`user:purge-deleted` runs the release and the wipe step by hand, with the same age rule: at 0
it releases and wipes nobody.

## Rules this change must follow

Regression plan items, a `METRICS` key, `useOnline`, i18n parity in 11 catalogues (UI, the
Inbox row, the inactivity email, error codes), `docs/full.env.example`, apiparity goldens for
the new routes and the additive `deletionMarkedAt`, mcpparity goldens for `list_connections`,
both migration engines, the wipe and guard
tests on both engines, the PostgreSQL lock-order test, revisionguard, slog with ids only,
comment rules.

## Approaches considered

- **Wipe**: unwind sharing, then delete the `users` row and let the database cascade (chosen; a
  new table that cascades from `users` is covered automatically, and the schema guard catches
  the keys that do not) vs keeping the row as a tombstone and deleting root tables in code (an
  explicit list that must grow with the schema).
- **Partner rows**: rows written into partner accounts reassigned to the account owner,
  cross-user transfers replaced by same-date corrections (chosen; balances right on every day,
  no schema change) vs one correction per account dated today (past balances shift) vs
  converting each cross-user row in place vs dropping the foreign key on `transactions.user_id`
  (SQLite table rebuild, "Deleted" author handling).
- **Unwind first** (chosen) vs relying on the cascade alone: budget elements point at
  categories and tags without a foreign key, so only the existing member-removal code cleans
  them. **By access rows** (chosen) vs calling delete-connection per connected user: the
  pairwise path is the one with the three-user bug, and it misses access rows that have no
  connection.
- **A custom currency others still use**: move it to that owner (chosen; one UPDATE, numbers
  and rates keep their meaning) vs switching the partner's budget, elements and debts to
  another currency (every plan and debt amount silently changes meaning) vs copying the
  currency per owner (rate history to copy, three tables to repoint).
- **Lifetime memory**: generic `users_audit` table (chosen) vs a dedicated lifetime table. The
  audit table also records the history and takes future events. **One-time re-grant** through a
  `lifetime_regranted` row (chosen; no mutable state, the newest row decides) vs a `used` flag
  on the `deleted` row.
- **Email hash**: only on lifetime rows (chosen) vs on every event (a deleted free user would
  stay testable against an address list forever, for no feature).
- **Inactivity and households**: never mark or wipe a user connected to an active user
  (chosen; one join, checked at marking, daily and inside the wipe) vs counting only partners
  who share an account or budget (more precise, a larger query, and wrong on the unsafe side
  when a share is missed) vs counting other users' writes into the user's accounts and budgets
  as activity (an activity write on every partner write) vs marking anyway and relying on the
  Inbox notice (the partner can only watch).
- **Subscription cancel**: billing polls `deleted-users` (chosen) vs Econumo calling billing.
- **Scheduling**: in-process daily job + CLI (chosen) vs host cron; the image has no cron.

## Calls made by the designer, for review

- Passwordless users confirm with the code alone.
- The inactivity notice must be sent before the mark is written; a failed send retries daily.
- Sharing is unwound at wipe time, not at marking, so a cancelled mark loses nothing.
- Marking revokes PATs and OAuth grants too; cancelling the mark does not bring them back.
- `user:delete` refuses unmarked users (run `user:mark-deleted` first).
- Recurring templates the user made on partners' accounts are deleted with the user, and so
  are partners' templates that transfer into the user's accounts.
- Correction description is English, like the existing account-deleted correction.
- AGREED 2026-11-26: a user connected to an active user is neither marked nor wiped for
  inactivity. Mine, on top of that: a marked partner does not count as active; direct
  connections only (A-B-C with only C active protects B, not A); `user:delete` obeys the rule
  too, and the operator's override is `user:mark-deleted` first.
- NEW: connected users see an Inbox row while a user is marked, without a date. It tells a
  partner that the user asked for deletion. Alternative: no notice at all.
- NEW: the email hash is kept only for deleted lifetime customers.
- NEW: the lifetime re-grant works once per deletion and needs proof of the mailbox; a
  password registration with the verification gate off gets none.
- NEW (replaces "partner debts move to the budget currency"): a custom currency someone else
  still uses moves to that owner.
- NEW: debts on the user's categories in partners' budgets are deleted, as a category delete
  does.
- NEW: the activity stamp has day precision (one conditional write per user per day).
- NEW: the delete-connection three-user bug is fixed in this change.
- NEW (second review): switching inactivity off releases every `inactive` mark.
- NEW (second review): eligibility adds one day to the threshold, for the stamp's precision.
- NEW (second review): existing users are backfilled with the migration time, not token use.
- NEW (second review): with the verification gate off, `user:verify-email` is the operator's
  way to re-grant lifetime access; `user:set-access` is not.
- NEW (second review): the deletion dialog says the CSV leaves out accounts deleted earlier.
  Alternative: an export that includes them.
- NEW (second review): `ClearActiveBudget` is fixed to write only the option.
- NEW: a failed wipe is an ERROR in the log and a non-zero CLI exit; there is no stored
  failure state.

## Left to the builder

Package layout of the wipe, code length/expiry reuse, job jitter, email and Inbox copy, the
exact password rate-limit scope, how the wipe reaches the member-removal, grant-unwind and
debt-release code across features (ports and glue, all on the wipe's transaction).
