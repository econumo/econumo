# ECONUMO_TRIAL as a day count

## Problem

`ECONUMO_TRIAL` today accepts two literals: `none` (default) and
`end-of-next-month`. `none` sets no access expiry, so a newly registered user
keeps their default `AccessLevelFull` forever — permanent full access.
`end-of-next-month` time-boxes the grant: it calls `SetAccess(full, until)` with
`until` = the first instant of the month after next, after which
`EffectiveAccessLevel` collapses the user to `readonly` (402 on writes).

We want the trial length to be configurable as a **number of days** instead of a
fixed calendar span, while keeping the self-hosted default of permanent full
access.

## Semantics

`ECONUMO_TRIAL` becomes an integer day count:

- unset / empty / `0` / `none` (case-insensitive) → `0` → **no grant**. The new
  user keeps the default `AccessLevelFull` with no expiry = permanent full
  access. This is the default, so self-hosters get full access out of the box.
- positive integer `N` → an **N-day trial**: full access from registration,
  lapsing to `readonly` `N` days later.
- anything else — a negative integer, a non-numeric string, or the removed
  `end-of-next-month` literal — **fails at boot** with a clear message.

`end-of-next-month` is dropped, not aliased. A deployment that still sets it must
switch to a day count (e.g. `ECONUMO_TRIAL=30`); the boot error names the
accepted forms.

Unchanged: CLI-created users get no trial regardless of this setting
(`selfService == false`); the readonly/402 enforcement and the apiparity
lapsed-trial fixtures set access explicitly and are untouched.

## Changes

### `internal/config/config.go`
- Replace the `Trial string` field with `TrialDays int`.
- Parse `ECONUMO_TRIAL`: empty or `none` (case-insensitive) → `0`; otherwise
  `strconv.Atoi`, rejecting parse errors and negative values. On rejection return
  a boot error, e.g. `ECONUMO_TRIAL: invalid value %q (want a non-negative number
  of days, 0, or none)`.
- Update the field comment.

### `internal/model/trial.go`
- Change the signature to `TrialEnd(registeredAt time.Time, days int) time.Time`
  returning `registeredAt.UTC().AddDate(0, 0, days)`.
- Replace the calendar-month rationale in the doc comment: the fixed day count is
  now the deliberate choice.

### `internal/user/usecase.go`
- Rename the `trial string` field to `trialDays int` and change the matching
  constructor parameter.

### `internal/user/register.go`
- Replace the grant block:
  ```go
  if selfService && s.trialDays > 0 {
      until := model.TrialEnd(now, s.trialDays)
      u.SetAccess(model.AccessLevelFull, &until, now)
  }
  ```

### Wiring
- `internal/server/server.go` and `internal/cli/container.go` pass
  `cfg.TrialDays` (int) instead of `cfg.Trial` (string).

### Tests
- `internal/config/config_test.go` — default `TrialDays == 0`; `ECONUMO_TRIAL=30`
  → `30`; `none` → `0`; `end-of-next-month` and a negative/garbage value → boot
  error.
- `internal/user/trial_integration_test.go` — `newTrialSvc` takes an `int`; the
  granted `AccessUntil` equals `now + N days`; a `0`-day service grants no expiry.
- `internal/model/trial_test.go` — rewrite for the day count (`TrialEnd(t, N)` ==
  `t.UTC() + N*24h`); drop the calendar-month-span test.

### Docs
- Update the `ECONUMO_TRIAL` paragraph in `CLAUDE.md`.
- `.env.example` has no `ECONUMO_TRIAL` entry today; leave it as-is.

## Out of scope

- The billing/subscription banner, admin `set-access`, and CLI `user:set-access`
  paths — they set access directly and don't read this config.
- Any change to `EffectiveAccessLevel`, the 402 middleware, or trial fixtures.
