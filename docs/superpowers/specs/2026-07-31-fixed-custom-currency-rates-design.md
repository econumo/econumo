# Fixed exchange rates for custom currencies — design

Date: 2026-07-31
Status: approved design, pre-implementation
Supersedes: the rate-related parts of `2026-07-14-user-currencies-design.md`
(dated custom rates, backdating, `set-currency-rate`).

## Problem

Custom currencies currently store dated rate history rows in
`currencies_rates`, like OXR-fed globals. The budget convertor is
period-aware (it snaps to the latest-rate month at-or-before the budget
month), so a "Points" rate set today does NOT convert April's Points
spending — the user would have to backdate rates month by month. That is
the right model for market currencies and the wrong one for user-invented
currencies: nobody maintains a rate history for a currency they made up.

## Decisions (from brainstorming)

- A custom currency has **one fixed rate, no history**. It is a property of
  the currency, denominated as today: "X units per 1 base unit".
- Changing the rate **re-values everything retroactively** — all periods,
  past budgets included, convert at the current fixed rate.
- The rate **folds into the currency's own lifecycle**: a field of
  `create-currency` and `update-currency`; the separate `set-currency-rate`
  endpoint, `RateDialog`, and "Set rate" menu action are deleted.
- `rate` is **mandatory** on create and on update (full replace) — a custom
  currency always has a rate; "clearing" does not exist. Legacy exception
  below.
- `get-currency-rate-list` remains every client's single conversion source:
  custom fixed rates are served as synthesized rows. The API's contract is
  "current rate per visible pair", not "rows of currencies_rates" (the
  synthetic base/base=1 row is precedent). Storage is an implementation
  detail behind the read model.

## Data model & migration

- `currencies` gains `rate NUMERIC NULL` (sqlite: `NUMERIC(19, 8)` text
  affinity like other decimals; pgsql: `NUMERIC(19, 8)`). Always NULL for
  global currencies; holds the fixed rate for customs.
- New migration (both engines, plain `ALTER TABLE ADD COLUMN` — no rebuild):
  1. add the column;
  2. backfill each custom currency's **latest** `currencies_rates` row
     (by `published_at`, per currency) into it;
  3. delete all rows in `currencies_rates` whose `currency_id` belongs to a
     custom currency — the table returns to globals-only OXR history.
- **Legacy NULL customs**: customs created while `rate` was optional and
  never given a rate migrate with NULL. They behave exactly as such
  currencies do today (no rate served; clients fall back unconverted); the
  first edit forces a rate in. No data is invented.

## Conversion semantics

- `RateProvider.AverageRates` overlays fixed rates on the period-averaged
  globals: customs no longer appear in the AVG query at all (their rows are
  gone); their rates come from the column, independent of the requested
  period. April's budget converts Points at the rate as it is NOW.
- `SnappedRatePeriod` and the AVG path for globals are unchanged.
- The boot-resolved base and the synthetic base/base=1 row are unchanged.

## API

- `create-currency`: `rate` becomes **required**. Blank/absent → field error
  `rate`: `"This value should not be blank."` (code `common.is_blank`);
  shape still validated by `"Rate must be a positive number"`. The rate is
  written to the column in the same transaction (no rate row).
- `update-currency` (full replace): gains **required** `rate`, same
  validation. Editing re-values history by design.
- `set-currency-rate`: **deleted** — route, handler, use case, DTOs,
  apiparity scenario + golden. The apiparity guard floors (`minRoutes` 102,
  and the scenario floor) drop by one with a comment recording the
  deliberate pre-release removal; the "never lower" rule guards against
  accidental scan breakage, not intentional deletions.
- `get-currency-rate-list`: global rows exactly as today (latest per pair +
  synthetic base row); for every **visible** custom currency with a non-NULL
  rate, a synthesized row `{currencyId, baseCurrencyId: base.ID, rate,
  updatedAt: <currency created_at, wire datetime format>}` is appended.
  `updatedAt` on synthesized rows is cosmetic and documented as such.
  Visibility scoping (own + shared-reachable) is identical to
  `get-currency-list`.
- Error-code registry: `currency.date_invalid` is retired from
  `errs.AllCodes` and both locale catalogues (the two-way i18n guard forces
  the pairing). `"Rate must be a positive number"` and its code stay.

## Frontend

- `RateDialog.tsx` and the "Set rate" `extraActions` entry are deleted.
- `CurrencyDialog` shows the rate field in **edit** mode too (create
  already has it), pre-filled from the rate list; required in both modes.
- The row caption ("1 USD = 10 PTS") keeps reading the rates query —
  unchanged.
- `web/src/api/currency.ts`: `setCurrencyRate` + its mutation hook are
  deleted; `updateCurrency` gains `rate`.

## Out of scope

- Rate history/backdating UI of any kind.
- Effective-dated custom rates ("keep old months as-is" was explicitly
  rejected).
- Changes to global-currency rate handling (OXR, CLI, in-process updater).
- Cross rates between two custom currencies beyond the existing
  through-the-base math.

## Testing

- Migration: latest-row-wins backfill, custom rows purged, global rows and
  their history untouched, NULL-rate legacy customs stay NULL.
- Provider: a historical period converts a custom at the fixed rate
  (the case that fails today); globals still period-averaged; both engines
  via the repo suites.
- Manage service: create requires rate (blank + shape errors), update
  requires rate, delete/archive/hide flows unchanged; the SetCurrencyRate
  use case and its tests are deleted.
- apiparity: `create-currency`/`update-currency` scenarios updated,
  `set-currency-rate` scenario removed, goldens regenerated and inspected
  (rate-list goldens gain the synthesized custom rows), guard floors
  adjusted once, enginecompare byte-identical.
- SPA: CurrencyDialog requires rate in both modes; the Set rate menu entry
  is gone; conversion still works end-to-end off the rates query.
