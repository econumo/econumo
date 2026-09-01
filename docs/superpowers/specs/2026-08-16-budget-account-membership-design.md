# Budget–account membership: explicit, permanent membership

**Date:** 2026-08-16 · **Branch:** `bug/budget-account-membership`

## Problem

Archived budget categories accumulate phantom "available" balances — rows
showing budgeted `0.00`, spent `0.00`, but a large positive available. In one
real budget the archived rows summed to **42,944.89** out of a **72,515.66**
grand total, inflating the headline available by ~59%.

### Root cause

`available = budgetedBefore − spentBefore − spent`
(`internal/budget/builder_structure_build.go:314`). The two sides are drawn
from **different account populations**:

| Side | Source | Account filter |
|---|---|---|
| `budgetedBefore` | `SummarizedLimits` (`internal/budget/repo/read.go:522`) | none — `budget_id` + period only |
| `spentBefore` | `CountSpending` (`internal/budget/builder_structure.go:107`) | `f.includedAccountIDs` |

`includedAccountIDs` shrinks in two ways:

1. **Excluding an account** — `internal/budget/builder.go:147` drops it from
   `included`.
2. **Deleting an account** — soft-delete sets `is_deleted`;
   `ListAvailable` filters `is_deleted = 0`, so the account vanishes from
   `AccountsForOwners` (`internal/server/glue_budget.go:57`).

Either way the account's spend leaves `spentBefore` while its limits remain
in `budgetedBefore`. The difference surfaces as phantom available,
accumulating one month at a time across the whole budget history.

Archiving is not the cause — archived rows are only where the drift is
*visible*, because nothing current masks it. Live categories carry the same
error inside real numbers.

## Design summary

Budget membership becomes an **explicit, append-mostly set**:

- A budget lists its member accounts in a new `budgets_accounts` table
  (replacing the `budgets_excluded_accounts` blacklist).
- The budget reads its member accounts **without an `is_deleted` filter**, so
  a soft-deleted member keeps contributing its history. `spentBefore` and
  `budgetedBefore` are then permanently drawn from the same population.
- Membership can be removed only while the account has **no transactions
  between the budget start and the start of the current month** — i.e. while
  removing it cannot change any closed month. A departing budget participant
  takes their accounts' membership with them (their limits already leave with
  them).
- Deleting an account **auto-zeroes** its balance with a correction
  transaction, so a permanently counted deleted account never carries a
  stale balance into the budget summary. The migration applies the same rule
  retroactively to every already-deleted account.
- Later transaction writes keep that invariant **self-healing** (§5a):
  creating a transaction against a deleted account is rejected; deleting or
  editing a transaction that touches one (e.g. a transfer to a live account)
  re-zeroes the deleted side with a dated correction in the same DB
  transaction.

## 1. Data model

New table, both engines (`internal/infra/storage/migrations/{sqlite,pgsql}`):

```sql
CREATE TABLE budgets_accounts (
    budget_id  TEXT     NOT NULL,
    account_id TEXT     NOT NULL,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (budget_id, account_id),
    FOREIGN KEY (budget_id)  REFERENCES budgets (id)  ON DELETE CASCADE,
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);
CREATE INDEX budgets_accounts_account_id_idx ON budgets_accounts (account_id);
```

`created_at` is bookkeeping only (the removal rule in §4 is activity-based).
The account FK is `ON DELETE CASCADE`,
which is correct: account deletion is *soft* (`is_deleted`), so the row
survives; only a hard delete drops membership.

`budgets_excluded_accounts` is dropped by the same migration.

## 2. Migration (three versions, run at boot in order)

### 2a. Command-backed migration steps (infra)

The boot runner (`internal/infra/storage/migrate`) today applies
`Migration{Version, SQL}` in version order, one transaction each, recorded in
`schema_migrations`. It gains **command steps** — a migration that names a
CLI command instead of carrying SQL:

- `migrate.Migration`, `backend.Migration` and `migrations.File` gain
  `Command string`, mutually exclusive with `SQL`. `migrations.SQLite()` /
  `Pgsql()` merge the `.sql` files with a small in-package registry
  (`{"20260817000001": "migration:zero-deleted-accounts"}`) and sort by
  version, so every consumer (`serve`, `data:import-sqlite`, `dbtest`, the
  migration tests) sees one ordered list.
- `migrate.Run(ctx, db, migs, migrate.WithCommandRunner(r))`, with
  `r func(ctx context.Context, name string) error`. A due command step calls
  the runner — **no surrounding migration transaction**; the command manages
  its own — and on success records the version. Failure → boot fails with
  `migrate: apply "<version>": …`, nothing is recorded, and it is retried at
  the next boot. A missing runner or an unknown command name is also a boot
  error: a command step is never silently skipped.
- The dependency rule shapes the dispatch: `migrate` (infra) cannot import
  `internal/cli` (which imports every feature). So `cmd/econumo/main.go`
  (`serve`) and `data:import-sqlite` — which already hold the CLI container —
  pass a runner that dispatches into the CLI command registry. `dbtest`
  passes `migrate.NoCommands` (a fresh database has no data for these
  commands to act on); the commands themselves are tested where they live,
  on seeded data, on both engines.
- Command names for such steps use the `migration:` prefix
  (`migration:<slug>`). They are ordinary CLI commands, listed with the
  others, and safe to run by hand at any time — every migration command must
  be **idempotent**.

### 2b. `20260817000000.sql` — schema

Create `budgets_accounts` + index (§1).

### 2c. `20260817000001` (command `migration:zero-deleted-accounts`)

The command is a thin CLI wrapper over a new account-feature use case
`Service.ZeroDeletedAccounts(ctx)`. All users, not only budget-relevant
ones: the invariant going forward is "a deleted account has zero balance",
so it is established retroactively. For each `accounts.is_deleted = 1`:
balance via `GetAccountBalance` (all transactions), rounded to 8 decimals
(the `NUMERIC(19,8)` scale); when non-zero, write one correction through the
**same `AccountCorrection`/`SaveCorrection` path** that `update-account` and
the delete-account auto-zero (§5) use, in its own transaction:

- `id`: `NextIdentity()` (UUIDv7); `user_id = account.UserID`;
- `description = "Balance adjustment (account deleted)"` (the same new
  constant §5 uses — NOT the plain `correctionComment`), uncategorized,
  no payee/tag/recipient;
- `type = 0` (expense) and `amount = balance` when positive; `type = 1`
  (income) and `amount = −balance` when negative;
- `spent_at = account.UpdatedAt` — the soft-delete sets `UpdatedAt`
  (`model.Account.Delete`) and a deleted account is not editable afterwards,
  so this is the deletion time;
- `created_at = updated_at = clock.Now()`.

Idempotent by construction (a zeroed account is skipped), so it is safe to
run from the CLI at any time. It logs one operation line per correction
(`account_id`, `amount`, `type` — ids and amounts only).

### 2d. `20260817000002.sql` — seed membership, drop the blacklist

For every budget, participants = the owner ∪ `budgets_access` rows with
`is_accepted = 1 AND role <> 2` (guest = `BudgetRoleGuest`). Insert
`(budget_id, account_id, created_at)` for every account owned by a
participant such that:

- no `budgets_excluded_accounts (budget_id, account_id)` row exists, and
- `accounts.is_deleted = 0`, **or** the account was deleted after the
  budget both started and existed:
  `accounts.updated_at >= budgets.started_at AND accounts.updated_at >
  budgets.created_at`;
- `created_at = CURRENT_TIMESTAMP`.

Then `DROP TABLE budgets_excluded_accounts`.

**SQL vs command step.** First preference is plain SQL: one
`INSERT … SELECT` per engine (participants via `UNION` of the owner and the
accepted non-guest access rows, `NOT EXISTS` against the blacklist, the
deleted-after-start predicate), then the `DROP`. If the dialect pair proves
too complex in practice, fall back to the §2a infrastructure:
`20260817000002` becomes command `migration:seed-budget-accounts` and the
`DROP TABLE` moves to a small `20260817000003.sql`. Two constraints shape
that command: it must use hand-written SQL (`db.Rebind` style — sqlc cannot
generate queries against `budgets_excluded_accounts`, since the final schema
no longer contains it), and it must treat a **missing blacklist table as a
no-op** — that is what keeps it idempotent and safe to run by hand after the
migration era, when re-seeding would otherwise resurrect memberships users
had removed.

Effect on existing numbers: a budget's figures move at migration only where a
deleted-after-start account rejoins the population — its spend closes the
phantom-available gap it opened, and its zeroing correction shows as
Uncategorized in the deletion month. Excluded-account drift is untouched
(that was a deliberate user choice). No other backfill.

## 3. Resolution — the actual fix

- `budgetAggregate.excludedAccountIDs` → `accounts []model.BudgetAccount`
  (`AccountID vo.Id`, `CreatedAt time.Time`), loaded in `loadAggregate` via
  `Repository.MemberAccounts(ctx, budgetID)`.
- Repository (`internal/budget/repository.go`, sqlc queries in
  `budgets.sql` for both engines):
  - `MemberAccounts(ctx, budgetID) ([]model.BudgetAccount, error)` — ordered
    by `created_at, account_id`.
  - `AddAccount(ctx, budgetID, accountID, now)` — `ON CONFLICT DO NOTHING`.
  - `RemoveAccount(ctx, budgetID, accountID)`.
  - `RemoveAccountsOwnedBy(ctx, budgetID, ownerID)` — `DELETE … WHERE
    budget_id = ? AND account_id IN (SELECT id FROM accounts WHERE user_id =
    ?)`; a storage-layer join only.
- Read model (`internal/budget/readmodel.go`, `repo/read.go`, both engines):
  `AccountsWithTransactions(ctx, accountIDs []vo.Id, start, end time.Time)
  ([]vo.Id, error)` — the subset of the given accounts that appear as
  `account_id` or `account_recipient_id` on any transaction with
  `start <= spent_at < end`. One query for all of the requester's members.
- Port (`internal/budget/ports.go`): `AccountLookup.AccountsForOwners` →
  `AccountsByIDs(ctx, ids []vo.Id) ([]model.AccountView, error)`, resolved by
  the server glue over the account repository **without an `is_deleted`
  filter** (`GetAccount` has none; a by-ids list query is added).
  `model.AccountView` gains `IsDeleted bool`. `AccountOwner` is unchanged.
- `buildFilters`: `included` = every membership row, in repository order;
  `currencyIDs` from the resolved views; the requester's own members (owner
  == requester) feed `filters.accounts` on the wire. **No participant
  re-filter at read time** — the invariant "a member row's owner is a
  current participant" is maintained on write (§4), so no read-side rule can
  shrink the population again.
- `ListAvailable`'s other consumers (account list, transactions, folders) are
  unchanged; they legitimately want the non-deleted set.

## 4. Membership rules (`internal/budget/accounts.go`, replacing `toggleAccount`)

- **Add** — the caller must own the account (`AccountOwner`) and hold
  `canUpdate` on the budget; a soft-deleted account cannot be added
  (validation error on `accountId`). Idempotent.
- **Remove** — same authorization. Refused with coded error
  `budget.account_not_removable` (field `accountId`) when the account has
  any transaction (as source or transfer recipient) with
  `budget.started_at <= spent_at < localMonth(now)` — the first of the
  current month in the caller's timezone (`localMonth`, the boundary every
  other budget month computation uses). No-op when not a member.
- One helper `removableAccounts(ctx, b, accountIDs, now) (map[string]bool,
  error)` (one `AccountsWithTransactions` call) is shared by remove,
  `update-budget` and the `filters` builder.
- **Rationale.** Removing an account with transactions in a closed month
  re-creates the asymmetry: its spend leaves `spentBefore` while its limits
  stay in `budgetedBefore`, and nothing in the current view shows it. Removing
  an account whose only budget-period transactions are in the current month
  changes only that month's own `spent`, in the view being looked at, where it
  is visible and correctable by adjusting that month's limits. So membership
  is freely editable until history accrues, then permanent — a freshly created
  or cloned budget starts fully editable.
- **Joining participants** — `AcceptAccess` seeds membership the way it seeds
  elements: every LIVE account owned by the accepting user joins the budget,
  in the same transaction, unless the accepted role is guest (mirroring the
  §2d seed rule `is_accepted AND role <> 2`). Without it a new participant's
  limits would land in `budgetedBefore` while their spending reached no member
  account — the asymmetry this design exists to end. Deleted accounts do not
  join: §2d's deleted-after-start branch is about pre-existing history, not a
  fresh join. `AddAccount` is `ON CONFLICT DO NOTHING`, so re-accepting after a
  revoke is idempotent.
- **Membership entry points** — rows are created by exactly four paths:
  (1) `create-budget`'s `accountIds`, (2) `add-account` / `update-budget`'s
  replace-set, (3) `accept-access` seeding the joining non-guest participant's
  live accounts, and (4) the one-time §2d migration seed. A newly created
  account joins **no** budget automatically — that is the deliberate
  explicit-membership design, not an oversight: adding an account to a budget
  is an act the user takes, and an auto-join would silently move figures in
  every budget its owner participates in.
- **Leaving participants** — `removeMemberRecords` additionally calls
  `RemoveAccountsOwnedBy(budgetID, memberID)`, removal rule ignored. Revoke,
  decline and delete-connection (`RemoveMember`) all route through it, and
  their limits already leave with them, so both sides of `available` shrink
  together. It also stops a former participant's future transactions leaking
  into a budget they no longer share.
- **Soft-delete / archive** of a member account: the row is untouched; the
  account keeps counting.
- **Budget delete**: cascade.

## 5. Delete-account auto-zero (`internal/account/delete.go`, owner branch)

Before soft-deleting, compute the account's balance over all its
transactions (`GetAccountBalance` with a far-future bound), round to 8
decimals (the `NUMERIC(19,8)` scale — a sub-representable float residue must
not produce a correction). When non-zero, write a correction transaction in
the same transaction, exactly as `update-account` does except the
description: `"Balance adjustment (account deleted)"` (a new constant
alongside `correctionComment`, so a deletion-driven zeroing is
distinguishable from a user's manual balance edit), uncategorized,
`type = 0`/`1` by sign, `amount = |balance|`, `spent_at = localNow(ctx)`.
Then `acct.Delete(now)`. The §2c migration stamps the same description on
its retroactive corrections.

The response stays `{}` (frozen). Non-owner "delete" (dropping one's own
grant) is unchanged.

## 5a. Transaction writes touching deleted accounts (self-healing zero)

The §5 invariant — a deleted account has zero balance — must survive later
transaction writes. A transfer between a deleted account and a live one is a
single row (`type = 2`, `account_id` + `account_recipient_id`) that stays
visible and editable from the live side; deleting or editing it moves the
deleted side's balance. Rules, enforced in `internal/transaction`:

- **Create is rejected** (`create.go`) when `accountId` or
  `accountRecipientId` resolves to a deleted account: coded error
  `transaction.account_deleted` (400, on the offending field). The guard in
  `createTransaction` covers manual create, MCP, and recurring posting
  (`CreateTransactionFromRecurring`). Import bypasses `createTransaction`
  (`Importer.SaveTransaction`) but its account resolution
  (`AvailableAccounts` / `AccountByID`) already returns only live accounts —
  asserted by test rather than re-guarded.
- **Delete is allowed** (`delete.go`). After the delete, in the same DB
  transaction, every deleted account among
  `{account_id, account_recipient_id}` is re-zeroed.
- **Update is allowed** (`update.go`) — amount, type, sides, date. An edit
  may remove a deleted account from a transaction but not **introduce** one:
  the create rule applies to any deleted account newly appearing in the
  account set (`transaction.account_deleted`). After the write, the same
  re-zero runs for every deleted account in the union of the old and new
  account sets.
- **Bulk update** (`bulk.go`): exempt — confirmed classification-only
  (category/payee/tag over non-transfers; amounts, accounts and dates are
  copied from the stored row, and transfers are rejected outright), so it
  has no balance effect.

**Re-zero** is the §2c/§5 zeroing primitive with a caller-supplied `spent_at`
and description: recompute the account balance (8-decimal rounding); zero or
sub-representable residue → nothing; positive → expense correction, negative
→ income; uncategorized, no payee/tag/recipient. Descriptions (literal
English constants, same convention as `correctionComment`):
`"Balance adjustment (deleted transaction)"` for delete-triggered
corrections, `"Balance adjustment (edited transaction)"` for
update-triggered ones.

**Correction `spent_at`**, per affected deleted account D:

- the transaction was deleted, or the edit removed D from it → the
  transaction's **old** `spent_at` (the offset lands in the month the
  removed flow left, so closed-month budget rows stay consistent);
- D is still a party after the edit → the **new** `spent_at` (equal to the
  old one for a pure amount edit). A date-only move changes no total
  balance, so no correction fires.

**Seam**: `internal/transaction/ports.go` gains
`AccountZeroer { ZeroDeleted(ctx, accountID vo.Id, spentAt time.Time, description string) error }`,
wired in `internal/server/glue_transaction_zeroer.go` over the account
feature's zeroing use case (shared with §2c/§5). It runs inside the caller's
`TxRunner` context, so the write and its correction commit atomically.

Consequences that need no special code: the correction is itself a
transaction on the deleted account, so deleting it re-fires the rule and
restores it (converges — after re-zero the balance is zero); a transfer
between **two** deleted accounts (API-reachable, invisible in the UI)
re-zeroes both sides. No endpoint or response-shape changes: the correction
sits on a hidden account and never appears in the refreshed account embed.

## 6. Wire contract

REST (`internal/budget/api`, DTOs in `internal/model/budget_dto.go`):

| Endpoint | Before | After |
|---|---|---|
| `POST create-budget` | `excludedAccounts: []` | **`accountIds: []`, required, ≥ 1**, each owned by the caller. Empty/missing → coded `budget.accounts_required` (field `accountIds`); a foreign or deleted id → blank-validation on `accountIds`. |
| `POST update-budget` | `excludedAccounts: []` (replace-set over own) | `accountIds: []`, **optional**: omitted (JSON absent → nil) → membership untouched; present → replace-set over the caller's OWN accounts: add missing, remove absent-and-removable; an absent-but-locked member → `budget.account_not_removable`, whole request 400. Foreign ids ignored (`ownsAccount`), as today. |
| `POST exclude-account` / `include-account` | `{id, accountId}` → `{item: meta}` | **`POST add-account` / `remove-account`**, same body and result. |
| `GET get-budget` `filters` | `excludedAccountsIds: []` | **`accounts: [{id, removable}]`** — the requester's own member accounts (deleted ones included), `removable` computed server-side. `periodStart`/`periodEnd` unchanged. |

MCP (`internal/budget/mcp`): `create_budget` gains required `account_ids`;
`update_budget` no longer round-trips the excluded set (membership
untouched); `set_budget_account_included` → `add_budget_account` /
`remove_budget_account`; `get_budget` follows the DTO.

Compatibility: no `minAppVersion` bump. An outdated app gets a translated 400
on create-budget and cannot change membership from the update dialog;
everything else works.

## 7. Frontend (`web/src/features/budgets`)

- `BudgetAccountsField`: props become `selected: Set<Id>` + `locked:
  Set<Id>`; a locked row renders a disabled `Switch`; when any row is locked
  a one-line hint appears (`budgets.modal.budget_form.accounts_locked_hint`: "Accounts with
  transactions in past months can't be removed").
  The "N of M included" counter stays.
- `BudgetDialog` (create): all toggles OFF; submit blocked with
  `budgets.form.budget.accounts.validation.required` until ≥ 1 is on; sends
  `accountIds`.
- `BudgetUpdateDialog`: members ON, locked ones disabled; sends `accountIds`.
- `api/budget.ts`: `addAccount`/`removeAccount`; DTO `filters.accounts`.
- Delete-account success also invalidates budget queries (the correction
  lands in the current month).
- `METRICS`: the exclude/include keys become add/remove-account, fired at the
  mutation success point (coverage test enforces).

## 8. i18n

New keys in all 11 catalogues (`locales/<lang>.json`, parity guards):
`errors.budget.account_not_removable`, `errors.budget.accounts_required`,
`errors.transaction.account_deleted`,
`budgets.modal.budget_form.accounts_locked_hint`,
`budgets.form.budget.accounts.validation.required`. All three codes are
registered in `errs.AllCodes`. The §5a correction descriptions are stored
transaction text, not catalogue keys (same convention as
`correctionComment`).

## 9. Testing

1. **Migration** (Go, `migration_20260817_test.go` style, both engines):
   migrate to the previous version; seed a budget (owner + accepted user + guest), an
   excluded account, a live account, an account deleted after start with
   balance 12.5, an account deleted before the budget existed with balance
   −3; run the new migration; assert exact `budgets_accounts` rows and
   the guest's accounts absent, and `budgets_excluded_accounts` gone.
   Runner test: a command step runs in order between SQL steps via the
   injected runner, is recorded once, is skipped on the next boot, a failing
   command records nothing, and a missing runner/unknown name fails.
   Use-case test (`ZeroDeletedAccounts`, both engines): positive → expense,
   negative → income, zero and `1e-10` residue → nothing, `spent_at ==
   UpdatedAt`, second run writes nothing.
2. **Core regression**: a category whose only spend sits on a since-deleted
   member account shows `available = 0` (spend still counted); the same
   through `get-budget` (apiparity scenario).
3. **Removal rule**: a member with a transaction last month → locked; only
   this-month transactions → removable; a received transfer last month →
   locked; a transaction before `started_at` → still removable; boundary is
   the caller's local first-of-month (`X-Timezone`); `remove-account` on a
   locked member → 400 with the code; `update-budget` omitting a locked
   member → 400.
4. **Leaving participant** drops their rows even when locked; other members
   untouched.
5. **Delete-account auto-zero**: positive balance → expense correction,
   negative → income, zero → none, `1e-10` residue → none; correction fields
   exact, description `"Balance adjustment (account deleted)"` (also asserted
   for the §2c migration corrections).
6. `create-budget` requires ≥ 1 owned account; `update-budget` with
   `accountIds` omitted leaves membership; present replaces.
7. `apiparity` + `mcpparity` goldens regenerated with `UPDATE_GOLDEN=1` and
   the diff **inspected** (filters shape, renamed routes/tools).
8. `make test`: sqlite/PostgreSQL byte parity for the new queries and the
   migration.
9. SPA vitest for `BudgetAccountsField`, both dialogs, and the API client.
10. **Self-healing zero (§5a)**: delete a live↔deleted transfer → correction
    on the deleted side at the transfer's `spent_at`, exact
    fields/description; edit its amount → delta correction at the (new)
    `spent_at` with the edit description; an edit that swaps the deleted
    side out → correction at the old `spent_at`; date-only edit → no
    correction; deleting the correction itself → restored; both-sides-deleted
    transfer → both corrected; create against a deleted account (direct,
    recurring posting, import) → 400 `transaction.account_deleted`; all of
    it atomic with the triggering write.

## 10. Out of scope

- The frontend total bug: `web/src/features/budgets/budgetMath.ts:96` folds
  the archive bucket into the grand total (its own comment at `:76` says
  archive is read-only history). Separate fix.
- Multi-currency summing in `budgetTotals` (raw amount strings across
  `currencyID`s). Separate investigation.
- A `restore-account` use case.
- Repairing excluded-account drift.

## Reference: key file locations

| What | Where |
|---|---|
| `available` computation | `internal/budget/builder_structure_build.go:314` |
| `budgetedBefore` / `spentBefore` | `internal/budget/builder_structure.go:58` / `:107` |
| `SummarizedLimits` (no account filter) | `internal/budget/repo/read.go:522` |
| Account resolution glue | `internal/server/glue_budget.go:57` |
| Exclusion filter | `internal/budget/builder.go:134` |
| Toggle use cases | `internal/budget/accounts.go` |
| Update/create budget | `internal/budget/crud.go`, `internal/budget/create.go` |
| Member removal | `internal/budget/accesssvc.go:117` |
| Account soft-delete / correction | `internal/account/delete.go`, `internal/account/update.go` |
| Transaction write paths (§5a) | `internal/transaction/{create,update,delete,bulk}.go`, `ports.go` |
| Balance formula | `internal/infra/storage/sqlc/query/sqlite/account_balance.sql` |
| Old blacklist table | `internal/infra/storage/migrations/sqlite/20241103192302.sql:101` |
| SPA dialogs | `web/src/features/budgets/{BudgetDialog,BudgetUpdateDialog,BudgetAccountsField}.tsx` |
| MCP tools | `internal/budget/mcp/mcp.go` |
