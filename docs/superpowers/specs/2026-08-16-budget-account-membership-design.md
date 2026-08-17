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
- Membership can only be removed within **1 month** of being added; after
  that it is permanent. A departing budget participant takes their accounts'
  membership with them (their limits already leave with them).
- Deleting an account **auto-zeroes** its balance with a correction
  transaction, so a permanently counted deleted account never carries a
  stale balance into the budget summary. The migration applies the same rule
  retroactively to every already-deleted account.

## 1. Data model

New table, both engines (`internal/infra/storage/migrations/{sqlite,pgsql}`):

```sql
CREATE TABLE budgets_accounts (
    budget_id  TEXT     NOT NULL,
    account_id TEXT     NOT NULL,
    added_at   DATETIME NOT NULL,
    PRIMARY KEY (budget_id, account_id),
    FOREIGN KEY (budget_id)  REFERENCES budgets (id)  ON DELETE CASCADE,
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);
CREATE INDEX budgets_accounts_account_id_idx ON budgets_accounts (account_id);
```

`added_at` drives the removal window. The account FK is `ON DELETE CASCADE`,
which is correct: account deletion is *soft* (`is_deleted`), so the row
survives; only a hard delete drops membership.

`budgets_excluded_accounts` is dropped by the same migration.

## 2. Migration `20260817000000` (one file per engine, one transaction)

1. **Create** `budgets_accounts` + index.
2. **Zero every deleted account** (all users — the invariant going forward is
   "a deleted account has zero balance", so it is established retroactively).
   For each `accounts.is_deleted = 1` whose balance is non-zero — the
   `account_balance.sql` formula (`incomes + transfer_incomes − expenses −
   transfer_expenses`), `ROUND(…, 8) <> 0` — insert one correction
   transaction:
   - `id`: `gen_random_uuid()` on pgsql; the hex-`randomblob` v4 recipe on
     sqlite (precedent: `20260730000000.sql`). A documented one-off exception
     to "new ids are UUIDv7".
   - `user_id = accounts.user_id`, `account_id = accounts.id`,
     `description = 'Balance adjustment'` (the account feature's
     `correctionComment`), `category_id/payee_id/tag_id/account_recipient_id
     = NULL`, `amount_recipient = NULL`.
   - `type = 0` (expense) and `amount = balance` when the balance is positive;
     `type = 1` (income) and `amount = −balance` when negative.
   - `spent_at = accounts.updated_at` — the soft-delete sets `UpdatedAt`
     (`model.Account.Delete`), and a deleted account is not editable
     afterwards, so this is the deletion time.
   - `created_at = updated_at = CURRENT_TIMESTAMP`.
3. **Seed membership.** For every budget, participants = the owner ∪
   `budgets_access` rows with `is_accepted = 1 AND role <> 2` (guest =
   `BudgetRoleGuest`). Insert `(budget_id, account_id, added_at)` for every
   account owned by a participant such that:
   - no `budgets_excluded_accounts (budget_id, account_id)` row exists, and
   - `accounts.is_deleted = 0`, **or** the account was deleted after the
     budget both started and existed:
     `accounts.updated_at >= budgets.started_at AND accounts.updated_at >
     budgets.created_at`.
   - `added_at = MAX(budgets.created_at, accounts.created_at)` — memberships
     older than a month are permanent from day one. (A fresh `added_at = now`
     would open a month-long removal window on every existing budget right
     after upgrading.)
4. **Drop** `budgets_excluded_accounts`.

Effect on existing numbers: a budget's figures move at migration only where a
deleted-after-start account rejoins the population — its spend closes the
phantom-available gap it opened, and its zeroing correction shows as
Uncategorized in the deletion month. Excluded-account drift is untouched
(that was a deliberate user choice). No other backfill.

## 3. Resolution — the actual fix

- `budgetAggregate.excludedAccountIDs` → `accounts []model.BudgetAccount`
  (`AccountID vo.Id`, `AddedAt time.Time`), loaded in `loadAggregate` via
  `Repository.MemberAccounts(ctx, budgetID)`.
- Repository (`internal/budget/repository.go`, sqlc queries in
  `budgets.sql` for both engines):
  - `MemberAccounts(ctx, budgetID) ([]model.BudgetAccount, error)` — ordered
    by `added_at, account_id`.
  - `AddAccount(ctx, budgetID, accountID, addedAt)` — `ON CONFLICT DO
    NOTHING`; a re-add never refreshes `added_at`.
  - `RemoveAccount(ctx, budgetID, accountID)`.
  - `RemoveAccountsOwnedBy(ctx, budgetID, ownerID)` — `DELETE … WHERE
    budget_id = ? AND account_id IN (SELECT id FROM accounts WHERE user_id =
    ?)`; a storage-layer join only.
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
  (validation error on `accountId`). Idempotent. `added_at = clock.Now()`.
- **Remove** — same authorization. Refused with coded error
  `budget.account_not_removable` (field `accountId`) when
  `!now.Before(addedAt.AddDate(0, 1, 0))` (UTC wall clock from `s.clock`).
  No-op when not a member.
- One helper `removable(addedAt, now time.Time) bool` is shared by remove,
  `update-budget` and the `filters` builder.
- **Rationale for 1 month.** Removing an account with transactions
  re-creates the asymmetry (its spend leaves `spentBefore`, its limits stay
  in `budgetedBefore`). The window bounds that drift to a single period,
  where it is visible in the month being viewed and correctable by adjusting
  that month's limits. Longer windows spread the drift across periods.
- **Leaving participants** — `removeMemberRecords` additionally calls
  `RemoveAccountsOwnedBy(budgetID, memberID)`, window ignored. Revoke,
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
the same transaction, exactly as `update-account` does: `correctionComment`
("Balance adjustment"), uncategorized, `type = 0`/`1` by sign, `amount =
|balance|`, `spent_at = localNow(ctx)`. Then `acct.Delete(now)`.

The response stays `{}` (frozen). Non-owner "delete" (dropping one's own
grant) is unchanged.

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
  a one-line hint appears (`budgets.modal.budget_form.accounts_locked_hint`).
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
`budgets.modal.budget_form.accounts_locked_hint`,
`budgets.form.budget.accounts.validation.required`. Both codes are
registered in `errs.AllCodes`.

## 9. Testing

1. **Migration** (Go, `migration_20260817_test.go` style): migrate to the
   previous version; seed a budget (owner + accepted user + guest), an
   excluded account, a live account, an account deleted after start with
   balance 12.5, an account deleted before the budget existed with balance
   −3; run the new migration; assert exact `budgets_accounts` rows and
   `added_at`, both zeroing corrections (type/amount/spent_at/description),
   the guest's accounts absent, and `budgets_excluded_accounts` gone.
2. **Core regression**: a category whose only spend sits on a since-deleted
   member account shows `available = 0` (spend still counted); the same
   through `get-budget` (apiparity scenario).
3. **Window boundary**: `now = added_at + 1 month − 1s` → removable;
   `+ 1 month` → locked; `remove-account` on a locked member → 400 with the
   code; `update-budget` omitting a locked member → 400.
4. **Leaving participant** drops their rows even when locked; other members
   untouched.
5. **Delete-account auto-zero**: positive balance → expense correction,
   negative → income, zero → none, `1e-10` residue → none; correction fields
   exact.
6. `create-budget` requires ≥ 1 owned account; `update-budget` with
   `accountIds` omitted leaves membership; present replaces.
7. `apiparity` + `mcpparity` goldens regenerated with `UPDATE_GOLDEN=1` and
   the diff **inspected** (filters shape, renamed routes/tools).
8. `make test`: sqlite/PostgreSQL byte parity for the new queries and the
   migration.
9. SPA vitest for `BudgetAccountsField`, both dialogs, and the API client.

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
| Balance formula | `internal/infra/storage/sqlc/query/sqlite/account_balance.sql` |
| Old blacklist table | `internal/infra/storage/migrations/sqlite/20241103192302.sql:101` |
| SPA dialogs | `web/src/features/budgets/{BudgetDialog,BudgetUpdateDialog,BudgetAccountsField}.tsx` |
| MCP tools | `internal/budget/mcp/mcp.go` |
