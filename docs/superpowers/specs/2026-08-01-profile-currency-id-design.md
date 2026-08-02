# Profile currency stored by ID — design

Date: 2026-08-01
Status: approved design, pre-implementation
Complements: `2026-07-14-user-currencies-design.md`,
`2026-07-31-fixed-custom-currency-rates-design.md`.

## Problem

The profile default currency is stored as a **code** (`users_options`,
`name='currency'`, `value="USD"`), and every read re-infers which currency
row that means (own-custom-first, then global). With custom currencies this
inference has real failure modes:

- **Silent rebinding**: a later same-code currency (e.g. an admin-added
  global matching an existing custom's code) changes what an existing
  profile resolves to, without the user doing anything.
- **Census false positives**: delete protection matches profiles by code, so
  Bob's own `PTS` profile blocks deleting Alice's unrelated `PTS` custom.
- **Incoherent archival**: a user can archive their own default currency
  (it vanishes from their pickers while remaining their default).

## Decisions (from brainstorming)

- The profile currency ID lives in the existing `users_options` row — its
  value becomes the **currency ID** (precedent: the `budget` option already
  stores an id). No dedicated `users.currency_id` column.
- The wire stays frozen: clients keep receiving the CODE in the options
  list; profile `update-currency` keeps accepting a code.
- **Archive stays** (rare but useful, and cheap). The one archival change:
  archiving is **refused while the currency is anyone's profile default** —
  now an exact by-id check.

## Migration (data-only, both engines)

`20260801100000`. For every `users_options` row with `name='currency'`:
resolve the stored code exactly as reads do today — the owner's own custom
first, then global — and write the **id** back as the value. Rows whose
value already matches an existing `currencies.id` are left untouched (guards
against a double-applied pre-release database; an id must never be treated
as an unresolvable code). Truly unresolvable codes: **delete the row** (a
missing option already has the well-defined USD read fallback, so behavior
is unchanged rather than inventing a reference).

## Reads

- The options-list rendering maps id→code for the `currency` option, so the
  frozen wire `{"name":"currency","value":"USD"}` is byte-identical.
- `currency_id` in `CurrentUserResult` stops being synthetic inference and
  is the stored value (still falling back to USD when the option is absent).
- The deprecated `currency` field keeps deriving from the same mapping.

## Writes

`POST /api/v1/user/update-currency` keeps its request shape (code). It
already resolves the code user-aware for validation; it now persists the
resolved **id**. Login/registration defaults: wherever the default code is
written today, the resolved default id is written instead.

## Exactness fixes that fall out

- Delete-protection census: the `users_options` term becomes
  `name='currency' AND value = <currency id>` — exact, and the pgsql query's
  id/code param asymmetry disappears (every position is the same id).
- Hide-currency profile guard: the `ProfileCurrency` port becomes
  `CurrencyID(ctx, userID) (string, error)`; the glue adapter returns the
  stored value with no resolution; the guard compares `rec.ID`.
- **New archive guard**: `archive-currency` is refused while any user's
  profile default references the currency (small dedicated count query,
  `name='currency' AND value = <id>`). Frozen message (fieldless):
  `"Currency is in use and cannot be archived"`, catalogue key
  `errors.currency.cannot_archive` (registered in `errs.AllCodes`, entries
  in BOTH locale catalogues — the two-way i18n guard enforces the pairing).
  Unarchive needs no guard.
- Rebinding is impossible: the profile references a row, not a code.

## Out of scope

- Removing or restricting archive beyond the default-currency guard
  (explicitly reconsidered and kept).
- `users.currency_id` FK column; changes to `budget`/`report_period`
  options.
- Extending global hide/show to custom currencies.

## Testing

- Migration: own-first resolution, global fallback, unresolvable → row
  deleted, already-an-id rows untouched (idempotence).
- Census by id: Bob's own `PTS` profile no longer blocks deleting Alice's
  `PTS` custom (regression test for the false positive).
- Archive guard: archiving your own default is refused with the frozen
  message; archiving after switching the default away succeeds. (The guard
  counts ANY profile holding the id, but in practice only the owner can —
  foreign users cannot resolve someone else's custom as their profile.)
- Hide guard compares ids; profile update/read round-trip persists the id
  while the wire keeps showing the code.
- apiparity goldens: byte-identical EXCEPT the new archive-guard scenario
  call (additive); enginecompare byte-identical; repo suite on PostgreSQL;
  coverage gate (80%) green.
