# In-process currency-rate updater

**Date:** 2026-07-23
**Status:** Approved design, pending implementation

## Problem

Exchange-rate updates today run only through the `currency:update-rates` CLI
command, which self-hosters must drive from an external cron. We want the
server itself to refresh rates on a schedule when it holds an Open Exchange
Rates token, removing the need for an outside process.

## Goals

- A new env var sets how often (in days) `serve` refreshes rates in-process.
- The feature is **opt-in**: unset/`0` keeps today's behavior (no in-process
  updates), so existing external-cron deployments are unaffected.
- Restart-safe: a frequently-restarting server must not burn OER API quota by
  re-fetching on every boot.
- No new HTTP/MCP surface; the CLI command stays as-is.

## Non-goals

- No change to `currency:update-rates` or the OER `Loader`.
- No per-currency or per-base scheduling; one instance-wide cadence.
- No backfill of historical dates; the poller fetches "today" only.

## Configuration

New field on `config.Config`:

```go
CurrencyUpdateIntervalDays int // ECONUMO_CURRENCY_UPDATE_INTERVAL: days between
                               // in-process rate refreshes; 0 (default) = off
```

- Parsed with `getInt("ECONUMO_CURRENCY_UPDATE_INTERVAL", 0)`.
- Validation in `config.Load` (with the other strict validators): a **negative**
  value fails at boot with a clear message. `0` is valid (disabled).
- The updater is enabled only when `CurrencyUpdateIntervalDays > 0` **and**
  `OpenExchangeRatesToken != ""`. If the interval is set (`> 0`) but the token is
  empty, `serve` logs a WARN at boot and the updater stays disabled — never a
  silent no-op. (The WARN lives in `serve`/wiring, not `config.Load`, matching how
  other "configured-but-inert" combinations are surfaced.)

## Poller

New type in a new file `internal/currency/rateupdater.go`, mirroring
`system.Service.StartPolling`:

```go
type RateUpdater struct {
    enabled  bool
    interval time.Duration // intervalDays * 24h
    days     int
    base     string
    loader   RateLoader
    svc      *WriteService
    clock    port.Clock
}
```

- `RateLoader` is a small interface declared in the `currency` package so the
  package does not import its own `repo` subpackage (keeps archtest clean):

  ```go
  type RateLoader interface {
      Load(ctx context.Context, date time.Time, base string, symbols []string) ([]model.RateInput, error)
  }
  ```

  The concrete `repo.Loader` already satisfies it and is injected at wiring time.

- `StartPolling(ctx)` — no-op when `!enabled`; otherwise launches ONE goroutine
  that runs `updateOnce` immediately (boot), then on a `time.Ticker` of
  `intervalDays * 24h`, exiting on `ctx.Done()`. Only `serve` calls it; `BuildAPI`
  (tests, CLI) never does, and a disabled updater no-ops regardless — so goldens
  and parity stay hermetic.

- `updateOnce(ctx)` (DB-aware skip):
  1. `latest, ok := svc.LatestRateDate(ctx)` — newest stored rate date.
  2. If `ok` and `today - latest < intervalDays`, skip with a DEBUG log
     (this is what makes restart loops cheap — no fetch when rates are fresh).
  3. Else `codes := svc.AvailableCodes(ctx)`, `rates := loader.Load(ctx, today, base, codes)`,
     `n := svc.UpdateRates(ctx, rates)`, then an INFO line with loaded/updated counts.
  - Any error is logged (WARN/DEBUG) and swallowed; a background job must never
    crash the server, matching the release poller.

"today" is `clock.Now()` truncated to the UTC date, consistent with how the CLI
passes its date to the loader and how rates are keyed (`published_at` at midnight
UTC).

## Persistence

New write query (both engines), regenerated with sqlc:

```sql
-- name: GetLatestRateDate :one
-- Newest published_at across all stored rates; NULL when none exist yet.
SELECT MAX(published_at) FROM currencies_rates;
```

- Implemented in `internal/currency/repo/write.go`; the nullable result maps to
  `(time.Time, bool, error)` where `ok == false` means "no rates yet".
- Added to the `currency.WriteModel` interface; `WriteService` gains a thin
  `LatestRateDate(ctx) (time.Time, bool, error)` delegating to it.
- The two engines differ in how `MAX(published_at)` comes back (sqlite text vs
  pgsql), handled by the existing per-engine adapter shims — no branching in the
  method body. `enginecompare` is not exercised here (no wire surface), but
  `make test-repo-pgsql` covers the pgsql adapter.

## Wiring

In `server.Build`:

- Build `currencyWriteRepo := currencyrepo.NewWriteRepo(...)` and
  `currencyWriteSvc := appcurrency.NewWriteService(currencyWriteRepo, txm, clk)`
  (today built only in the CLI container).
- Build `loader := currencyrepo.NewLoader(cfg.OpenExchangeRatesToken, clk.Now)`.
- Build the updater:
  `rateUpdater := appcurrency.NewRateUpdater(enabled, cfg.CurrencyUpdateIntervalDays, cfg.CurrencyBase, loader, currencyWriteSvc, clk)`
  where `enabled = cfg.CurrencyUpdateIntervalDays > 0 && cfg.OpenExchangeRatesToken != ""`.
  Emit the token-missing WARN here when `interval > 0 && token == ""`.
- `Build` returns the updater as an additional value; `BuildAPI` discards it.
- `serve` (`cmd/econumo/main.go`) calls `rateUpdater.StartPolling(ctx)` right
  after `updates.StartPolling(ctx)`.

## Docs

- `.env.example`: a commented `# ECONUMO_CURRENCY_UPDATE_INTERVAL=` line next to
  `OPEN_EXCHANGE_RATES_TOKEN`, explaining opt-in + token requirement.
- `CLAUDE.md`: an entry in the configuration list describing the var, its opt-in
  default, the token dependency, and the DB-aware restart behavior.

## Testing

- `config_test.go`: parses a positive value; `0`/unset → disabled; a negative
  value fails `Load`.
- `internal/currency/rateupdater_test.go`: a fake `RateLoader` + a fake service
  (implementing `LatestRateDate`/`AvailableCodes`/`UpdateRates`) covering:
  - disabled (interval 0, or token empty) → `updateOnce` never fetches;
  - fresh rates (latest within interval) → skip, no `Load` call;
  - stale/absent rates → `Load` + `UpdateRates` called with today's date.
- No new routes → apiparity/mcpparity goldens untouched.

## Rollout / compatibility

Purely additive and opt-in. Unset var = byte-identical behavior to today.
Setting the var on an instance that also runs external cron is harmless: rate
upserts are idempotent per `(date, currency, base)`.
