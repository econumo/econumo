# User-managed currencies & rates — design

Date: 2026-07-14
Status: approved design, pre-implementation

## Problem

Currencies and rates are instance-global and admin-only (CLI `currency:add`,
`currency:update-rates` / OpenExchangeRates). Users cannot introduce their own
currencies (e.g. a "Points" currency to teach kids budgeting, exchanged at
1 USD = 100 PTS) and cannot control which currencies clutter their dropdowns.

## Goals

- A user can create **custom currencies** with their own exchange rates.
- Custom currencies are **per-user**: private to the owner, except they render
  correctly for other users who see them through shared accounts.
- A user who sees a foreign custom currency via a shared account **cannot**
  denominate their own accounts/budgets in it.
- Users choose which **global** currencies appear in their dropdowns.
- Global currencies stay admin-managed (CLI + OXR), unchanged.

## Non-goals

- Editing global currencies or their rates from the UI.
- A per-currency rate-history editor (a single "set rate as of date" suffices).
- Roles/admin flags in the database — every authenticated user gets the same
  capabilities over their own currencies and their own visibility preferences.
- Sharing custom currencies through connections independently of accounts.

## Decisions (from brainstorming)

| Topic | Decision |
|---|---|
| Scoping | Per-user custom currencies; globals stay admin-managed |
| Rate editing | Owners set rates for their own custom currencies only |
| Rate dating | Default today (caller's timezone), optional backdate `date` field |
| Code format | 3 ASCII letters (ISO shape) + required display name + optional symbol |
| Code uniqueness | Unique per owner; must not collide with a global code at creation |
| Lifecycle | Own customs: edit / archive / delete-only-if-unused. Globals: per-user show/hide only |
| Dropdown default | All globals visible; users hide to taste (store the hidden set) |
| Permissions | Any authenticated user; no role system |

## Data model

One new migration per engine (`internal/infra/storage/migrations/{sqlite,pgsql}`):

1. `currencies.user_id TEXT NULL`, FK → `users(id) ON DELETE CASCADE`.
   NULL = global (all existing rows). Non-NULL = custom, owned by that user.
   Deleting a user cascades to their currencies and (via the existing
   `currencies_rates` FK) their rate rows.
2. `currencies.is_archived BOOLEAN NOT NULL DEFAULT 0`. Wire: `isArchived`
   int `0`/`1`. Only customs are ever archived in v1; the column is uniform.
3. Uniqueness split — replace `UNIQUE (code)` with:
   - `CREATE UNIQUE INDEX ... ON currencies(code) WHERE user_id IS NULL`
   - `CREATE UNIQUE INDEX ... ON currencies(user_id, code) WHERE user_id IS NOT NULL`
   Both engines support partial indexes. SQLite cannot drop a table-level
   UNIQUE, so the sqlite migration rebuilds the table (create-copy-rename).
   The rule "a custom code must not equal an existing global code" is enforced
   in the service at creation time only (cross-row; a later admin-added global
   with the same code coexists with pre-existing customs).
4. New table `users_hidden_currencies (user_id TEXT, currency_id TEXT,
   created_at DATETIME, PRIMARY KEY (user_id, currency_id))`, both FKs
   `ON DELETE CASCADE`. A row = "this user hid this global currency". Empty
   table reproduces today's behavior — no backfill.

`currencies_rates` is untouched: custom rates are ordinary dated rows against
the instance base currency. sqlc queries are added under
`internal/infra/storage/sqlc/query/{sqlite,pgsql}` and regenerated for both
engines.

## Backend API

All new endpoints: `POST /api/v1/currency/…`, authenticated, thin handlers via
`endpoint.Handle`, DTOs in `internal/model/currency_dto.go`, use cases on
`currency.WriteService` plus a visibility use case. Swagger annotations per
house pattern. (`/api/v1/user/update-currency` — the profile currency — is a
different module and is unaffected.)

### Custom-currency lifecycle (owner-only)

- `create-currency` — `{operationId?, code, name, symbol?, fractionDigits?, rate?}`.
  Code normalized to upper-case, 3 ASCII letters; rejected on collision with
  the caller's codes or any global code. `name` required (what pickers show).
  `symbol` defaults to the code. `fractionDigits` 0–8, default 2. `rate`, when
  present, writes today's rate row in the same transaction. Honors the
  `operationId` idempotency guard.
- `update-currency` — `{id, name, symbol, fractionDigits}`. Code is immutable.
- `archive-currency` / `unarchive-currency` — `{id}`. Archived customs leave
  the owner's pickers but keep rendering wherever referenced (shared accounts
  included).
- `delete-currency` — `{id}`. Refuses while any account, transaction, budget,
  budget element/envelope, or any user's profile-currency option references
  the currency — anyone's, including via shares. The reference check runs in
  the same transaction as the delete. Rate rows do not count as usage (the FK
  cascade removes them).

### Rates (owner-only)

- `set-currency-rate` — `{currencyId, rate, date?}`. `rate` is a positive
  decimal meaning "1 base unit = X of this currency" (the storage direction).
  `date` optional `YYYY-MM-DD`; default = today in the caller's timezone
  (`X-Timezone` header, matching the existing day-boundary convention).
  Upserts the `(date, currency, base)` row. Refused for global currencies,
  foreign customs, and the base currency itself.

### Global-currency visibility (per-user preference)

- `hide-currency` / `show-currency` — `{id}`. Insert/delete the
  `users_hidden_currencies` row; both idempotent. Only globals can be hidden.
  Guards: the instance base currency and the caller's current profile currency
  cannot be hidden.

### Ownership errors

Mutations on a currency the caller does not own (global or foreign custom)
fail with the standard access-denied error, identical to how other foreign
resources respond — no existence leak.

## Read path & conversion

### get-currency-list (per-user)

Returns, for the caller:

1. all global currencies,
2. all their own customs (archived included — the settings page needs them),
3. foreign customs referenced by accounts shared to them
   (`accounts_access` → `accounts.currency_id`, owner ≠ caller).

`CurrencyResult` gains three additive fields (existing fields frozen):

- `"scope"`: `"global"` | `"own"` | `"shared"`
- `"isArchived"`: int 0/1
- `"isHidden"`: int 0/1 (from `users_hidden_currencies`; 0 for non-globals)

The SPA derives all dropdowns from this one response. Custom names come from
the stored `name` (required at creation), so the existing `currencyName()`
Intl fallback is untouched.

### get-currency-rate-list

Same wire shape. The query changes from "all rates on the single most-recent
published date" to "the latest rate per currency", scoped like the list
(globals + own + shared-visible). Rationale: with backdated custom rates, the
old query would drop a custom rate the moment OXR writes newer rows for other
currencies. Parity goldens are regenerated and inspected for both reads.

### Convertor

No changes. Custom rates are ordinary rows against the base; period averaging
and fallbacks behave exactly as for a global currency with sparse rates. The
optional `rate` on create mitigates the empty-rate window.

### Denomination validation (account/budget/transaction writes)

"Currency id exists" tightens to "exists AND usable by the caller":
usable = global, or own non-archived custom. Foreign customs and archived own
customs are rejected when *setting* a currency; existing references are never
re-validated, so shared accounts keep working.

## Frontend (web/)

- **Settings → Currencies** joins the **classification** menu group in
  `SettingsPage.tsx` (after Payees), route `RouterPage.SETTINGS_CURRENCIES`.
- Page: `web/src/features/currencies/CurrenciesPage.tsx`, following the
  Categories/Tags/Payees pattern (`web/src/features/classifications/`,
  reusing `ClassificationList` conventions for list + archive actions).
  Two sections:
  1. **My currencies** — name, code, symbol, current rate ("1 USD = 100 PTS"),
     archived badge; actions: add, edit, set rate (rate + optional date),
     archive/unarchive, delete (confirm; server refusals surface as the error
     message). Empty state explains the feature.
  2. **Global currencies** — show/hide toggle per currency; the base currency
     and the user's profile currency render disabled with a tooltip.
- Dialogs follow the existing shadcn dialog patterns (AccountDialog as
  reference); create sends an `operationId`.
- API layer: new functions in `web/src/api/currency.ts`, DTO additions in
  `web/src/api/dto/currency.ts`; TanStack Query mutations invalidate the
  currency-list query (`web/src/features/currencies/` queries).
- Dropdown filtering: a shared `selectableCurrencies(items)` helper =
  globals with `isHidden === 0` + own with `isArchived === 0`; used by the
  account dialog, budget dialogs, and profile currency picker. Rendering
  paths resolve from the full list. Edge: when editing an entity whose current
  currency the filter would drop (hidden/archived/foreign), the picker
  includes that currency so the form cannot self-corrupt.
- i18n: strings under `modules.classifications.currencies.*` in every
  `web/src/locales/*` file.

## Error handling

Standard `errs` taxonomy → the frozen error envelope. Exact strings (frozen
once shipped):

| Case | Field | Message |
|---|---|---|
| Bad code shape | `code` | `CurrencyCode is incorrect` |
| Duplicate code (own or global collision) | `code` | `Currency already exists` |
| Bad name | `name` | `Currency name must be 1-64 characters` |
| Bad symbol | `symbol` | `Currency symbol must be 1-12 characters` |
| Bad fraction digits | `fractionDigits` | `Fraction digits must be between 0 and 8` |
| Bad rate | `rate` | `Rate must be a positive number` |
| Bad date | `date` | `Date is not valid` |
| Delete while referenced | — (message-level) | `Currency is in use and cannot be deleted` |
| Base-currency guard (rate/hide/archive/delete) | — (message-level) | `The base currency cannot be modified` |
| Hide own profile currency | — (message-level) | `This currency cannot be hidden` |
| Hide a non-global currency | — (message-level) | `This currency cannot be hidden` |
| Not owner (update/archive/delete/set-rate) | — | standard access-denied envelope |

All writes log the standard operation-result line and add `currency_id` via
`reqctx.AddLogAttr`.

Concurrency: `create-currency` honors `operationId`; `set-currency-rate` is a
retry-safe upsert; hide/show are idempotent. The partial unique indexes and
the in-transaction delete check backstop races the service checks cannot
close.

## Testing

- **Unit/integration (per package)**: WriteService use cases — ownership,
  code collisions (own + global), base-currency guards, delete-in-use across
  every referencing table, rate upsert incl. backdating, hide/show guards.
  Read scoping — globals + own + shared-visible via `accounts_access`
  fixtures. Denomination tightening — foreign custom rejected, archived own
  rejected, existing references untouched.
- **Repo tests, both engines**: partial-index semantics and the
  per-currency-latest rate query, via the default sqlite run and
  `make test-repo-pgsql`.
- **apiparity**: one scenario per new route (guard-enforced) + regenerated,
  inspected goldens for `get-currency-list` / `get-currency-rate-list`.
- **enginecompare**: inherits the new scenarios; responses must stay
  byte-identical across engines.
- **Frontend vitest**: CurrenciesPage (render, dialogs, toggles),
  `selectableCurrencies` (hidden/archived/foreign filtering + keep-current-
  value edge), API client functions.
- Coverage gate (`GO_COVER_MIN`, currently 72) stays green.
