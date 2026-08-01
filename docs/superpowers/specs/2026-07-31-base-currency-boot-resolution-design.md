# Base currency: resolve once at boot — design

Date: 2026-07-31
Status: approved design, pre-implementation

## Problem

The instance base currency (`ECONUMO_CURRENCY_BASE`, default `USD`) lives as a
code string that services re-resolve to a `currencies` row at runtime, over and
over: `ManageService.SetCurrencyRate` calls `GetGlobalIDByCode` per request,
`RateProvider` resolves the pivot per conversion batch, `ReadService`'s
synthetic-rate row scans the visible list for a code match, and the base
guards compare codes. A typo'd base code fails at runtime as lookup errors,
not at boot. There is also no visibility when an admin changes the base on an
instance that already holds rates.

## Decisions (from brainstorming)

- The base currency is **instance-global and admin-only**. Per-user "default
  currency" already exists (the `users_options` `currency` profile option) and
  is out of scope.
- `ECONUMO_CURRENCY_BASE` **stays the source of truth** — an env var is
  already admin-only. No settings table, no admin API surface.
- An admin **may** change the base on a live instance. Nothing is rewritten
  or deleted: `currencies_rates` rows carry their own `base_currency_id`, so
  old rows simply become inert (they stop matching the current pivot). The
  system only makes the drift visible (boot WARN).
- No rate rebase/wipe tooling. Can be added later if anyone asks.

## Design

### 1. The value

New type in `internal/model` (shared type universe):

```go
// BaseCurrency is the instance base currency, resolved at boot: every stored
// rate is denominated "X units per 1 base unit". Constructing one is the
// composition root's job, so consumers can trust ID references an existing
// global currency row.
type BaseCurrency struct {
    ID   string
    Code string
}
```

### 2. Boot resolution (serve path only)

`server.Build`/`BuildAPI` resolves `cfg.CurrencyBase` to the global currency
row once, right after the currency repo is wired (migrations have already
run; the baseline seeds USD, so default setups cannot fail). Unresolvable
code → boot error:

```
ECONUMO_CURRENCY_BASE="XXX" does not match any global currency; add it first with currency:add
```

The **CLI container is untouched**: `currency:add` must work before the base
resolves (chicken-and-egg), and the CLI's only base use is passing the code
to the OXR loader.

### 3. Consumers

| Consumer | Change |
|---|---|
| `currency.NewReadService` | takes `model.BaseCurrency`; the synthetic base-rate row uses `base.ID`; the visible-list code scan is deleted |
| `currency.NewManageService` | takes `model.BaseCurrency`; `SetCurrencyRate` drops its per-request `GetGlobalIDByCode`; the base guards ("cannot be modified/hidden") compare `rec.ID == base.ID` |
| `currencyrepo.NewRateProvider` | takes the resolved base id; stops re-resolving the pivot |
| `currency.NewRateUpdater` + CLI OXR fetch | keep the **code** (the OXR API wants a code) |
| CSV import (`NewTransactionImportAccounts`) | **keeps the code, on purpose**: new imported accounts resolve the base code user-aware (an own custom with the base's code wins over the global — pinned by `TestImport_NewAccount_PrefersOwnCustomCurrencyOverGlobalCode`); pre-resolving to the global id would be a behavior regression |

### 4. Drift visibility

At boot (serve), one query: newest `currencies_rates` row's
`base_currency_id`. If it differs from the resolved base id, log WARN:
stored rates were written against a different base currency; conversions use
only rates written against the current base. Nothing is rewritten or deleted.
No WARN when there are no rates.

### 5. Error handling

- Unresolvable base code: `serve` refuses to start with the message above
  (config-error class, like a malformed `MAILER_DSN`).
- Everything else is behavior-identical for valid configurations.

## Testing

- Boot failure: unknown base code refuses to serve; message asserted.
- Drift WARN: seeded rate against a different base id → WARN emitted; no
  rates → no WARN.
- Existing currency read/manage tests construct
  `model.BaseCurrency{ID: usdID, Code: "USD"}` in their harnesses.
- apiparity/mcpparity goldens must be **byte-identical** — the change is
  observable only in boot behavior, and an unchanged golden set is itself the
  verification.

## Out of scope

Settings table, per-user base, rate rebase or wipe on base change, admin
API/UI for the base, changes to the profile-currency option.
