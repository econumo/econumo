# Profile currency by ID + custom-currency archival removed — design

Date: 2026-08-01
Status: approved design, pre-implementation
Amends: `2026-07-14-user-currencies-design.md` (archival lifecycle removed)
and complements `2026-07-31-fixed-custom-currency-rates-design.md`.

## Problems

1. The profile default currency is stored as a **code** (`users_options`,
   `name='currency'`, `value="USD"`), and every read re-infers which currency
   row that means (own-custom-first, then global). With custom currencies this
   inference has real failure modes: a later same-code currency can silently
   **rebind** an existing profile, and the delete-protection census matches by
   code, so Bob's own `PTS` profile blocks deleting Alice's unrelated `PTS`
   custom (false positive).
2. **Archival of custom currencies earns nothing.** Delete is already
   hard-blocked while referenced; archive only removed the currency from the
   owner's pickers and could leave incoherent states (an archived profile
   default). Decision: the lifecycle is delete-when-unreferenced, keep
   otherwise. Global hide/show is untouched.

## Decisions (from brainstorming)

- The profile currency ID lives in the existing `users_options` row — its
  value becomes the **currency ID** (precedent: the `budget` option already
  stores an id). No dedicated `users.currency_id` column.
- The wire stays frozen: clients keep receiving the CODE in the options list;
  `update-currency` (profile) keeps accepting a code.
- Archive/unarchive for custom currencies is **removed entirely**, including
  the `is_archived` column (a follow-up migration on the same date, later
  time, drops it — SQLite's in-place `DROP COLUMN` applies, no closure
  rebuild; the column is not indexed or referenced by any constraint).
- Known trade-off, accepted: a custom currency with history (undeletable)
  stays in pickers forever. Extending global hide to own customs is the
  natural follow-up if that ever itches — out of scope here.

## Part 1: profile currency stored by ID

### Migration (data-only, both engines)

`20260801200000` (after the drop-column migration; order between the two is
immaterial, the versions just need distinct slots on the same day). For every
`users_options` row with `name='currency'`: resolve the stored code
exactly as reads do today — the owner's own custom first, then global — and
write the **id** back as the value. Unresolvable codes: **delete the row**
(a missing option already has the well-defined USD read fallback, so behavior
is unchanged rather than inventing a reference).

### Reads

- The options-list rendering maps id→code for the `currency` option, so the
  frozen wire `{"name":"currency","value":"USD"}` is byte-identical.
- `currency_id` in `CurrentUserResult` stops being synthetic inference and is
  the stored value (still falling back to USD when the option is absent).
- The deprecated `currency` field keeps deriving from the same mapping.

### Writes

`POST /api/v1/user/update-currency` keeps its request shape (code). It
already resolves the code user-aware for validation; it now persists the
resolved **id**. Login/registration defaults unchanged (they write the
default code today — they now write the resolved default id).

### Exactness fixes that fall out

- Delete-protection census: the `users_options` term becomes
  `name='currency' AND value = <currency id>` — exact, and the pgsql query's
  id/code param asymmetry disappears (every position is the same id).
- Hide-currency profile guard: the `ProfileCurrency` port becomes
  `CurrencyID(ctx, userID) (string, error)`; the glue adapter returns the
  stored value with no resolution; the guard compares `rec.ID`.
- Rebinding is impossible: the profile references a row, not a code.

## Part 2: custom-currency archival removed

### Migration

`20260801100000` (same day as the fixed-rate migration, later time), both
engines: `ALTER TABLE currencies DROP COLUMN is_archived;`. The sqlc schema
replay then generates models without the column, so every stale reference
fails the build — that is the deletion checklist.

### Deletions

- Endpoints `POST /api/v1/currency/archive-currency` and
  `/unarchive-currency`: routes, handlers, use cases, DTOs, swagger,
  apiparity scenario calls. Route-guard floor 101 → 99 (commented as a
  deliberate pre-release removal).
- `EnsureUsable` simplifies to "global, or the caller's own custom".
- sqlc: `SetCurrencyArchived` query removed; `is_archived` leaves
  `GetCurrencyRecord`, `InsertUserCurrency`, `GetUserCurrencyListView`.
- Wire (pre-release, never shipped): `isArchived` is removed from the
  currency **list item**; `scope` and `isHidden` stay. Goldens change once.
- Frontend: the archive switch on own rows goes away — own rows render **no
  switch** (globals keep hide/show). `ClassificationList.rowSwitch` may
  return `null` for "no switch on this row" (defaults for categories/tags/
  payees untouched). `selectableCurrencies` drops the archived filter; the
  archived caption/greyed style no longer applies to currencies.
- `errs` codes and locale entries that existed only for archival are retired
  from the registry and BOTH catalogues (two-way guard).

## Out of scope

- `users.currency_id` FK column; changes to `budget`/`report_period` options.
- Extending hide/show to custom currencies.
- Any change to global-currency management or the fixed-rate model.

## Testing

- Part 1: migration test (own-first resolution, global fallback,
  unresolvable → row deleted); census-by-id regression test (Bob's own `PTS`
  profile no longer blocks deleting Alice's `PTS`); hide-guard id
  comparison; profile update/read round-trip persists the id while the wire
  keeps showing the code; **apiparity goldens byte-identical for Part 1**.
- Part 2: goldens regenerated once (isArchived gone, archive scenario calls
  gone) and inspected; route floor bump; SPA tests for switchless own rows
  and the removed filter behavior.
- Both engines: repo suite on PostgreSQL + enginecompare byte-identical.
- Coverage gate (80%) stays green.
