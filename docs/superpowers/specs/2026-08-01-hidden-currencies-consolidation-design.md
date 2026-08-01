# Hidden currencies + migration consolidation — design

Date: 2026-08-01
Status: approved design, pre-implementation
Supersedes: the archival lifecycle of `2026-07-14-user-currencies-design.md`;
the migration layout of `2026-07-31-fixed-custom-currency-rates-design.md`
and `2026-08-01-profile-currency-id-design.md` (their semantics stand).

## Decisions (from brainstorming)

1. **`users_hidden_currencies` replaces `is_archived`.** A custom currency is
   only ever usable by its owner, so "archived" was owner-scoped in effect —
   it collapses into "the owner hid it". One mechanism, one switch, for
   globals and customs alike.
2. **Hidden is a pure picker preference** — no server-side usability block.
   `EnsureUsable` simplifies to "global, or the caller's own custom". An API
   caller may denominate in their own hidden custom; hiding is a preference,
   not a lock.
3. **No read-time currency fallbacks.** The migration establishes the
   invariant "every user's currency option holds a live currency id"
   (unresolvable codes normalize to the global USD row — baseline-seeded and
   undeletable — instead of being deleted). Reads then treat a violated
   invariant as an error instead of silently substituting USD.
4. **The branch's four currency migrations squash into one**
   (`20260730000000`, replacing the old closure rebuild and all three
   `202608xx` steps — nothing is deployed). On released databases no custom
   currencies exist, so the fixed-rate backfill and archived→hidden copy
   have nothing to do and disappear entirely.
5. **The migration runner gains `-- migrate:fk-off`** (first line of a
   migration file): on SQLite the runner disables `foreign_keys` BEFORE the
   transaction, applies the migration, re-enables, and fails unless
   `PRAGMA foreign_key_check` is clean. This makes small single-table
   rebuilds safe and retires the 317-line FK-closure dance as a pattern.
   On PostgreSQL the directive is an inert comment.

## The squashed migration `20260730000000`

sqlite (`-- migrate:fk-off`):
- `CREATE TABLE currencies_new` in the FINAL shape: `id, code, symbol,
  created_at, name, fraction_digits, user_id (FK users, CASCADE), rate` —
  no table-level `UNIQUE(code)`, no `is_archived` (it never exists in this
  history).
- Copy all rows (existing globals: `user_id`/`rate` NULL), drop the old
  table, rename. Recreate the currencies indexes: the two partial uniques
  (`UNIQ_currencies_code_global`, `UNIQ_currencies_user_code`),
  `IDX_currencies_user_id`; the legacy `UNIQ_37C4469377153098` dies with
  the old table.
- `users_hidden_currencies` + its index (unchanged DDL).
- The profile-option rewrite: code → id, own-first then global, values that
  already match a `currencies.id` untouched, unresolvable → the global USD
  id (NOT deleted — invariant, decision 3). Users with NO currency option
  row at all get one seeded with the global USD id, so the invariant holds
  for every user, not just those the registration seeding reached.

pgsql (no directive): `ADD COLUMN user_id`, `ADD COLUMN rate`, drop the
`UNIQUE(code)` constraint + legacy index, partial uniques + user_id index,
hidden table, option rewrite with the `::text` casts.

Deleted: the old `20260730000000` closure rebuild, `20260801000000`,
`20260801100000`, and their three test files → ONE consolidated migration
test (rebuild preserves rows + FK check clean; partial uniques enforce;
option rewrite matrix incl. USD-normalization; hidden-table cascade).

Local databases that ran the old steps (demo, beta) are re-aligned once by
hand (drop `is_archived`, prune stale version bookkeeping) — no released
database exists.

## Backend changes

- Delete `archive-currency` + `unarchive-currency`: routes (guard floor
  101 → 99), handlers, use cases, DTOs, swagger, the `SetCurrencyArchived`
  query, `IsArchived` on `CurrencyRecord`/view rows.
- Delete the `currency.cannot_archive` code/message/locales added earlier
  today — the hide guard is the guard now.
- `HideCurrency`/`ShowCurrency` accept the caller's OWN customs; foreign
  customs keep the frozen `"This currency cannot be hidden"` refusal; the
  base guard and the by-id profile-default guard are unchanged.
- `EnsureUsable`: drop the archived branch.
- Read side: `isHidden` is computed for own rows too (shared rows stay 0);
  `isArchived` leaves the wire (pre-release).
- Strict reads (decision 3): remove the USD fallbacks from the user read
  service, `toCurrentUserWithEmail`, the glue `CurrencyID` adapter, and the
  budget-create default path (`DefaultCurrencyID` documents that "" cannot
  happen; a dangling id errors). Registration's defensive
  ignore-resolution-error branch becomes a propagated error.

## Frontend

- One switch semantic for every row: visible/hidden. Own rows keep the
  Edit/Delete kebab. `selectableCurrencies` filters own customs by
  `isHidden`; the archived caption/greyed styling for currencies goes; the
  template adapter feeds the constant `isArchived: 0` (the
  `ClassificationItem` shape keeps the field for categories).
- Locked switches unchanged (base + profile default, disabled + tooltip) —
  and now equally applicable when a custom is the profile default.

## Accepted trade-offs (recorded)

- Hiding is not a lock (decision 2).
- A shared viewer loses the "retired" signal on a foreign custom (they
  could never act on it).
- If custom currencies ever become shareable-for-use, the owner-state vs
  viewer-preference distinction must be reintroduced.

## Testing

- Consolidated migration test (matrix above) on both engines; the fk-off
  runner behavior gets its own runner-level test (fk-off migration that
  would cascade under FK=ON preserves children; a migration that leaves a
  dangling FK fails the post-check).
- Hide-own-custom happy path in apiparity (previously impossible to cover),
  `err:hide-default` for a custom default, `err:hide-foreign`; archive
  scenario calls removed; goldens regenerated once and inspected;
  enginecompare byte-identical.
- Strict-read tests: dangling option id errors (no silent USD); the
  invariant tests from the option rewrite.
- SPA: unified switch behavior for own rows, selectable filtering by
  isHidden, no archive UI remnants.
- Full tier green; coverage gate 80.
