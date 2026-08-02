# Deleting a custom currency — design

Date: 2026-08-02
Status: approved design, pre-implementation
Complements: `2026-07-14-user-currencies-design.md`,
`2026-08-01-hidden-currencies-consolidation-design.md`.

## Problem

A user creates a custom currency, denominates an account in it, then deletes
the account. The currency can never be deleted again.

`CountCurrencyUsage`
(`internal/infra/storage/sqlc/query/{sqlite,pgsql}/currency_write.sql`)
counts every `accounts` row referencing the currency, with no `is_deleted`
filter. `DeleteAccount` (`internal/account/delete.go:31`) only sets
`is_deleted`, and there is no restore path anywhere — no REST route, no MCP
tool, no CLI command. So the account is invisible forever while pinning the
currency forever, and `DeleteCurrency` (`internal/currency/manage_lifecycle.go:52`)
returns `CodeCurrencyInUse` against data the user cannot see or act on.

Hiding is not a workaround: `CurrenciesPage.tsx:98` always renders own
customs under "My currencies", and a hidden one stays in that list with its
switch off.

Two constraints rule out simply relaxing the count:

- `accounts.currency_id` is `ON DELETE CASCADE`, and
  `transactions.account_id` cascades in turn. Deleting the currency row would
  silently destroy the account and its transactions.
- `transactions.account_recipient_id` is `ON DELETE SET NULL`. A transfer
  from a **live** account into the dead one would have its recipient nulled —
  corruption of live data.

## Decisions (from brainstorming)

- **Deleting a custom currency is a soft delete.** The row survives so every
  foreign key stays valid; the currency disappears from the owner's world.
- **The flag lives on `currencies`, not on `users_hidden_currencies`.** A
  custom currency has exactly one owner, so "deleted" is a property of the
  currency. Hiding is per-user only because globals are shared across users.
  Keeping deletion out of the join table means no read path needs a join to
  exclude deleted rows, and `users_hidden_currencies` keeps one meaning.
- **Delete is still refused while a LIVE entity uses the currency.** That
  error is legitimate and stays exactly as today.
- **Per-user code uniqueness stays a database guarantee.** Because the flag
  is a column on `currencies`, the existing partial unique index can filter
  on it, so re-creating a deleted code works without dropping the index.
- **No un-delete.** Deleted means gone from the user's perspective; there is
  no recycle bin and no UI affordance to bring one back.

### Rejected alternatives

- **Repoint dead accounts to the base currency, then hard-delete.** Works,
  but rewrites rows on the user's behalf to make an unrelated delete succeed.
- **Hard-purge the soft-deleted accounts.** Destroys transaction history and
  nulls transfer recipients on live accounts.
- **Overload `users_hidden_currencies` as the delete marker.** No new column,
  but a row would mean "hidden" for globals and "deleted" for own customs,
  and the unique index cannot see another table — so per-user code uniqueness
  would degrade to application-level validation, opening a concurrent-create
  hole.
- **`is_deleted` on `users_hidden_currencies`.** Same column count as the
  chosen design, but the table name becomes a lie, every read needs the join,
  and the index still cannot reference the flag.

## Semantics

A user-created currency has one lifecycle action, **Delete**:

| Referenced by | Result |
|---|---|
| a live account (`is_deleted = 0`), a budget, a budget element, or anyone's profile currency | 400 `CodeCurrencyInUse` — unchanged |
| only soft-deleted accounts, or nothing at all | success; `currencies.is_deleted = 1` |

The budget, budget-element and profile-currency terms of the usage count all
reference live data and must keep blocking; only the `accounts` term gains
the `is_deleted = 0` filter. That one predicate is the actual bug fix.

**Only the owner's own custom currencies are deletable.** A global currency
can never be deleted, by anyone — nor can another user's custom. This is
already enforced and does not change: `DeleteCurrency` calls `ownedRecord`
(`internal/currency/manage.go:111`) first, which rejects a row with
`user_id IS NULL` or a foreign owner with 403 `AccessDenied` before the usage
check ever runs. `is_deleted` therefore only ever becomes 1 on a row that has
an owner, and the rewritten partial unique index — predicated on
`user_id IS NOT NULL` — is unaffected on the global side.

Globals remain hideable per user through `users_hidden_currencies`, exactly
as today. Hide is the only lifecycle action a global has.

## Migration

`20260802000000`, both engines.

SQLite — a plain `ADD COLUMN` with a constant default, no table rebuild:

```sql
ALTER TABLE currencies ADD COLUMN is_deleted BOOLEAN DEFAULT '0' NOT NULL;
DROP INDEX UNIQ_currencies_user_code;
CREATE UNIQUE INDEX UNIQ_currencies_user_code
  ON currencies (user_id, code) WHERE user_id IS NOT NULL AND is_deleted = 0;
```

PostgreSQL is the same shape with `BOOLEAN NOT NULL DEFAULT false`.

`UNIQ_currencies_code_global` is untouched. Existing rows default to not
deleted, so the migration is behaviour-neutral on existing data.

## Backend changes

1. **`CountCurrencyUsage`** — add `AND is_deleted = 0` to the `accounts`
   subquery in both engines' `currency_write.sql`, regenerate sqlc. Leave the
   `budgets`, `budgets_elements` and `users_options` terms alone.
2. **`DeleteCurrency`** (`manage_lifecycle.go:52`) — keep the ownership check
   and the usage guard; replace the row delete with a soft delete. The
   hard-delete query and its repo method become unreachable and are removed,
   leaving one delete path and one behaviour to test.
3. **`UserCurrencyListView`** — `AND is_deleted = 0`, so deleted customs
   leave every client's list at the source. `GetCurrencyList`
   (`internal/currency/read.go:56`) needs no filtering of its own, and REST,
   MCP and the SPA agree by construction.
4. **`EnsureUsable`** (`internal/currency/repo/lookup.go:86`) currently
   documents *"Hidden currencies stay usable: hiding is a picker preference,
   not a lock."* A deleted currency **is** a lock. `GetCurrencyRecord`
   already returns the row, so this reads the new column and rejects with the
   existing `CodeCurrencyNotAvailable` — no extra query. Without this the
   REST and MCP edges would still accept a deleted currency when creating an
   account.
5. **`OwnerCurrencyCodeExists`** — `AND is_deleted = 0`, so the create-time
   duplicate check only sees live currencies and re-creating a deleted code
   succeeds.
6. **`GetCurrencyIDByCodeForUser`** (`currencies.sql:4`) — `AND is_deleted = 0`.
   This query is `LIMIT 1` over `(user_id IS NULL OR user_id = ?)` and backs
   setting the profile currency by code (`internal/server/glue_user.go:43`).
   Excluding deleted rows keeps its result unambiguous once a user can hold a
   live and a deleted currency with the same code.

`GetCurrencyRecord`, `GetCurrencyByID` and the other by-id reads stay
unfiltered: historical rows still need to resolve their currency for display.

## Frontend changes

`CurrenciesPage.tsx:140` builds one enable/disable switch for every row, own
customs included. Restrict it to globals — Delete is the action people want
on their own currencies, and one control beats two. Own customs keep the
Edit/Delete menu (`:119`), and Delete now succeeds instead of erroring.

Deleted customs never reach the client, so no "deleted" state renders
anywhere and `selectableCurrencies`
(`web/src/features/currencies/selectable.ts`) needs no change.

Hiding and deleting are independent states in this design, so restoring the
toggle on own customs later is a pure UI change requiring no data work.

## Testing

- Repo/unit: the live-vs-dead usage split — a currency used by a
  soft-deleted account deletes, one used by a live account, budget, budget
  element or profile currency does not.
- `EnsureUsable` rejects a deleted own custom (guards the REST and MCP
  create paths).
- Deleting a global currency, and deleting another user's custom, both still
  return 403 and leave `is_deleted` at 0. A regression here would let one
  user remove a currency out from under every other user on the instance.
- Creating a currency reusing a deleted code succeeds, and the rewritten
  partial unique index still rejects a duplicate among live rows.
- `apiparity`: a scenario for deleting a currency used only by a soft-deleted
  account. Existing goldens should not move — the delete response is `{}`
  either way — so any golden diff means something unintended changed.
- `enginecompare` picks all of the above up for free and is the check that
  the SQLite and PostgreSQL partial indexes behave identically.

## Out of scope

- Restoring a soft-deleted account, or purging one. The absence of a restore
  path is what makes this soft delete safe; changing it is a separate
  decision.
- `ShowCurrency` still exists as an endpoint and would technically clear a
  hidden flag on an own custom, but it never clears `is_deleted` and nothing
  in the UI calls it for customs. Left as-is rather than guarded.
