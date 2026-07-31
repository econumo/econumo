# In-process Currency-Rate Updater Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `serve` refresh exchange rates in-process on a configurable day cadence when an Open Exchange Rates token is present, removing the need for an external cron.

**Architecture:** A new opt-in env var `ECONUMO_CURRENCY_UPDATE_INTERVAL` (integer days) drives a background `RateUpdater` in `internal/currency` that mirrors the existing `system.Service.StartPolling` pattern (fetch on boot, then `time.Ticker`). Each run is DB-aware: it reads the newest stored rate date via a new `GetLatestRateDate` query and skips fetching when rates are already fresh, so restart loops don't burn API quota. Wired in `server.Build`, started only by `serve`; no new HTTP/MCP routes.

**Tech Stack:** Go stdlib (`net/http`, `time`, `log/slog`), sqlc (per-engine query codegen), `internal/test/dbtest` for repo integration tests.

## Global Constraints

- Env naming: app-owned config is prefixed `ECONUMO_`. The new var is `ECONUMO_CURRENCY_UPDATE_INTERVAL` (integer day count).
- Opt-in: unset/`0` = feature OFF (byte-identical to today's behavior). A negative or malformed value FAILS at boot.
- The updater is enabled only when the interval is `> 0` AND `OPEN_EXCHANGE_RATES_TOKEN` (`cfg.OpenExchangeRatesToken`) is non-empty.
- Background jobs never crash the server: every fetch error is logged and swallowed (mirror `system.Service.fetch`).
- Rates are keyed by `(published_at, currency_id, base_currency_id)`; `published_at` is midnight UTC. The clock is UTC.
- sqlc query `.sql` comments must stay ASCII-only (a multi-byte comment corrupts sqlite codegen in sqlc v1.30 — see the note atop `currency_write.sql`).
- No new routes → do NOT regenerate or edit apiparity/mcpparity goldens.
- Coverage gate: `make go-test` enforces `GO_COVER_MIN` (78). New code needs the tests in this plan.

---

### Task 1: Config — `ECONUMO_CURRENCY_UPDATE_INTERVAL`

**Files:**
- Modify: `internal/config/config.go` (struct field ~line 26; parse in `Load` after the `ECONUMO_TRIAL` block ~line 149)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config.CurrencyUpdateIntervalDays int` — days between in-process rate refreshes; `0` = disabled. Parsed with the existing `getIntStrict(key, def)` helper (fails at boot on negative/malformed).

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestLoad_CurrencyUpdateInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")

	// Default: unset -> 0 (disabled).
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.CurrencyUpdateIntervalDays != 0 {
		t.Errorf("default = %d, want 0", c.CurrencyUpdateIntervalDays)
	}

	// Positive value is parsed.
	t.Setenv("ECONUMO_CURRENCY_UPDATE_INTERVAL", "3")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.CurrencyUpdateIntervalDays != 3 {
		t.Errorf("interval = %d, want 3", c.CurrencyUpdateIntervalDays)
	}
}

func TestLoad_CurrencyUpdateIntervalBadValueFailsBoot(t *testing.T) {
	for _, bad := range []string{"-1", "abc", "1.5"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
			t.Setenv("ECONUMO_CURRENCY_UPDATE_INTERVAL", bad)
			if _, err := Load(); err == nil {
				t.Fatalf("Load: want error for %q, got nil", bad)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoad_CurrencyUpdateInterval -v`
Expected: FAIL — `c.CurrencyUpdateIntervalDays` undefined (compile error).

- [ ] **Step 3: Add the struct field**

In `internal/config/config.go`, in the "Econumo behavior" block (next to `CheckUpdates`, ~line 26):

```go
	CurrencyUpdateIntervalDays int // ECONUMO_CURRENCY_UPDATE_INTERVAL: days between in-process rate refreshes; 0 (default) = off (requires OPEN_EXCHANGE_RATES_TOKEN)
```

- [ ] **Step 4: Parse it in `Load`**

In `internal/config/config.go`, immediately after the `ECONUMO_TRIAL` block (after `c.TrialDays = days` closes, ~line 150):

```go
	// In-process rate refresh cadence in DAYS; 0 (default) disables it. Strict
	// parse (like the rate limits): a negative/typo'd value must fail at boot,
	// not silently disable the poller.
	interval, err := getIntStrict("ECONUMO_CURRENCY_UPDATE_INTERVAL", 0)
	if err != nil {
		return Config{}, err
	}
	c.CurrencyUpdateIntervalDays = interval
```

Note: `err` is already declared earlier in `Load` (used by `parseMailerDSN`), so use `=` not `:=` — the code above uses `interval, err :=` with a fresh `interval`, which is valid because `interval` is new. If the compiler complains `err` is unused/redeclared, switch to two lines: `var perr error; interval, perr := ...; if perr != nil { return Config{}, perr }`. Prefer the `interval, err :=` form first.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestLoad_CurrencyUpdateInterval -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): ECONUMO_CURRENCY_UPDATE_INTERVAL (days; opt-in, strict)"
```

---

### Task 2: Persistence — newest stored rate date

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/currency_write.sql`
- Modify: `internal/infra/storage/sqlc/query/pgsql/currency_write.sql`
- Generate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/currency_write.sql.go` + `querier.go` (via `go generate`)
- Modify: `internal/currency/admin.go` (add method to `WriteModel` interface + `WriteService`; add `time` import)
- Modify: `internal/currency/repo/write.go` (implement on `WriteRepo` + `writeQuerier` + both engine shims)
- Test: `internal/currency/repo/write_integration_test.go` (new)

**Interfaces:**
- Produces: `currency.WriteModel.LatestRateDate(ctx context.Context) (time.Time, bool, error)` — newest stored rate date; `ok == false` means "no rates yet". Delegated by `(*currency.WriteService).LatestRateDate(ctx) (time.Time, bool, error)`.
- Consumes: existing `seededUSD` const (`= "dffc2a06-6f29-4704-8575-31709adee926"`) from `internal/currency/repo/lookup_read_integration_test.go` (same `repo_test` package).

- [ ] **Step 1: Write the failing integration test**

Create `internal/currency/repo/write_integration_test.go`:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestWriteRepo_LatestRateDate(t *testing.T) {
	db := dbtest.New(t)
	w := currencyrepo.NewWriteRepo(db.Engine, db.TX)
	ctx := context.Background()

	// No rates yet -> ok=false.
	if _, ok, err := w.LatestRateDate(ctx); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	// Seed two rates on different days; the newest wins.
	for _, d := range []time.Time{
		time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
	} {
		if err := w.UpsertRate(ctx, model.RateRow{
			ID:             vo.NewId().String(),
			CurrencyID:     seededUSD,
			BaseCurrencyID: seededUSD,
			Date:           d,
			Rate:           "1.00000000",
		}); err != nil {
			t.Fatalf("UpsertRate: %v", err)
		}
	}

	got, ok, err := w.LatestRateDate(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestRateDate: ok=%v err=%v", ok, err)
	}
	if got.Format("2006-01-02") != "2026-07-22" {
		t.Errorf("LatestRateDate = %s, want 2026-07-22", got.Format("2006-01-02"))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/currency/repo/ -run TestWriteRepo_LatestRateDate -v`
Expected: FAIL — `w.LatestRateDate` undefined (compile error).

- [ ] **Step 3: Add the query to BOTH engine `.sql` files**

Append to `internal/infra/storage/sqlc/query/sqlite/currency_write.sql`:

```sql
-- name: GetLatestRateDate :one
-- Newest stored rate date, for the in-process rate updater's freshness check.
-- ORDER BY ... LIMIT 1 (not MAX) so the result types as the published_at column
-- (time.Time) instead of an aggregate interface{}. sql.ErrNoRows = no rates yet.
SELECT published_at FROM currencies_rates ORDER BY published_at DESC LIMIT 1;
```

Append to `internal/infra/storage/sqlc/query/pgsql/currency_write.sql` (identical SQL; keep the comment ASCII-only):

```sql
-- name: GetLatestRateDate :one
-- Newest stored rate date, for the in-process rate updater's freshness check.
-- ORDER BY ... LIMIT 1 (not MAX) so the result types as the published_at column
-- (time.Time) instead of an aggregate interface{}. sql.ErrNoRows = no rates yet.
SELECT published_at FROM currencies_rates ORDER BY published_at DESC LIMIT 1;
```

- [ ] **Step 4: Regenerate sqlc**

Run: `go generate ./internal/infra/storage/sqlc/...`
Then verify the generated signature in both engines is `GetLatestRateDate(ctx context.Context) (time.Time, error)`:

Run: `grep -rn "func (q \*Queries) GetLatestRateDate" internal/infra/storage/sqlc/gen/`
Expected: two matches, each returning `(time.Time, error)`.

- [ ] **Step 5: Add the method to `writeQuerier` + both engine shims**

In `internal/currency/repo/write.go`:

Add to the `writeQuerier` interface:

```go
	GetLatestRateDate(ctx context.Context, db backend.DBTX) (time.Time, error)
```

Add the `WriteRepo` method (place after `UpsertRate`):

```go
// LatestRateDate returns the newest stored rate date; ok=false when no rates
// exist yet (the in-process updater treats that as "must fetch").
func (r *WriteRepo) LatestRateDate(ctx context.Context) (time.Time, bool, error) {
	t, err := r.q.GetLatestRateDate(ctx, r.db(ctx))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	return t, true, nil
}
```

Add to `sqliteWriteQuerier`:

```go
func (sqliteWriteQuerier) GetLatestRateDate(ctx context.Context, db backend.DBTX) (time.Time, error) {
	return sqlitegen.New(db).GetLatestRateDate(ctx)
}
```

Add to `pgsqlWriteQuerier`:

```go
func (pgsqlWriteQuerier) GetLatestRateDate(ctx context.Context, db backend.DBTX) (time.Time, error) {
	return pgsqlgen.New(db).GetLatestRateDate(ctx)
}
```

(`time`, `errors`, and `database/sql` are already imported in `write.go`.)

- [ ] **Step 6: Extend `WriteModel` + `WriteService` in `admin.go`**

In `internal/currency/admin.go`, add `"time"` to the import block, then add to the `WriteModel` interface (after `UpsertRate`):

```go
	// LatestRateDate returns the newest stored rate date; ok=false when none
	// exist yet.
	LatestRateDate(ctx context.Context) (time.Time, bool, error)
```

Add the `WriteService` delegate (place after `AvailableCodes`):

```go
// LatestRateDate returns the newest stored rate date; ok=false when no rates
// exist yet. Used by the in-process rate updater's freshness check.
func (s *WriteService) LatestRateDate(ctx context.Context) (time.Time, bool, error) {
	return s.write.LatestRateDate(ctx)
}
```

- [ ] **Step 7: Run the integration test + build**

Run: `go build ./... && go test ./internal/currency/... -run TestWriteRepo_LatestRateDate -v`
Expected: build OK; test PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/infra/storage/sqlc internal/currency/admin.go internal/currency/repo/write.go internal/currency/repo/write_integration_test.go
git commit -m "feat(currency): GetLatestRateDate query + WriteService.LatestRateDate"
```

---

### Task 3: The `RateUpdater` background poller

**Files:**
- Create: `internal/currency/rateupdater.go`
- Test: `internal/currency/rateupdater_test.go` (package `currency`, white-box)

**Interfaces:**
- Consumes: `(*WriteService).LatestRateDate/AvailableCodes/UpdateRates` (Task 2 + existing), `port.Clock`, `model.RateInput`.
- Produces:
  - `type RateLoader interface { Load(ctx context.Context, date time.Time, base string, symbols []string) ([]model.RateInput, error) }` — satisfied by `repo.Loader`.
  - `func NewRateUpdater(enabled bool, intervalDays int, base string, loader RateLoader, svc rateStore, clock port.Clock) *RateUpdater`
  - `func (u *RateUpdater) StartPolling(ctx context.Context)` — no-op when disabled; else one goroutine (boot run + `time.Ticker`).

- [ ] **Step 1: Write the failing unit tests**

Create `internal/currency/rateupdater_test.go`:

```go
package currency

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
)

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeLoader struct {
	calls   int
	lastDay time.Time
	rates   []model.RateInput
}

func (l *fakeLoader) Load(_ context.Context, date time.Time, _ string, _ []string) ([]model.RateInput, error) {
	l.calls++
	l.lastDay = date
	return l.rates, nil
}

type fakeStore struct {
	latest      time.Time
	latestOK    bool
	codes       []string
	updated     int
	updateCalls int
}

func (s *fakeStore) LatestRateDate(context.Context) (time.Time, bool, error) {
	return s.latest, s.latestOK, nil
}
func (s *fakeStore) AvailableCodes(context.Context) ([]string, error) { return s.codes, nil }
func (s *fakeStore) UpdateRates(_ context.Context, rates []model.RateInput) (int, error) {
	s.updateCalls++
	s.updated = len(rates)
	return len(rates), nil
}

var today = time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)

func TestRateUpdater_DisabledNeverFetches(t *testing.T) {
	loader := &fakeLoader{}
	store := &fakeStore{}
	u := NewRateUpdater(false, 1, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 0 || store.updateCalls != 0 {
		t.Fatalf("disabled updater fetched: load=%d update=%d", loader.calls, store.updateCalls)
	}
}

func TestRateUpdater_FreshSkips(t *testing.T) {
	loader := &fakeLoader{}
	// Newest rate is today; interval 1 day -> fresh -> skip.
	store := &fakeStore{latest: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), latestOK: true}
	u := NewRateUpdater(true, 1, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 0 || store.updateCalls != 0 {
		t.Fatalf("fresh rates should skip: load=%d update=%d", loader.calls, store.updateCalls)
	}
}

func TestRateUpdater_StaleFetches(t *testing.T) {
	loader := &fakeLoader{rates: []model.RateInput{{Code: "EUR", Base: "USD", Rate: "0.9", Date: today}}}
	// Newest rate is 3 days old; interval 1 day -> stale -> fetch.
	store := &fakeStore{latest: time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC), latestOK: true, codes: []string{"EUR"}}
	u := NewRateUpdater(true, 1, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 1 || store.updateCalls != 1 {
		t.Fatalf("stale rates should fetch: load=%d update=%d", loader.calls, store.updateCalls)
	}
	if loader.lastDay.Format("2006-01-02") != "2026-07-22" {
		t.Errorf("loader called for %s, want 2026-07-22", loader.lastDay.Format("2006-01-02"))
	}
	if store.updated != 1 {
		t.Errorf("updated %d rates, want 1", store.updated)
	}
}

func TestRateUpdater_NoRatesFetches(t *testing.T) {
	loader := &fakeLoader{rates: []model.RateInput{{Code: "EUR", Base: "USD", Rate: "0.9", Date: today}}}
	store := &fakeStore{latestOK: false, codes: []string{"EUR"}} // empty DB
	u := NewRateUpdater(true, 7, "USD", loader, store, fakeClock{today})
	u.updateOnce(context.Background())
	if loader.calls != 1 || store.updateCalls != 1 {
		t.Fatalf("empty DB should fetch: load=%d update=%d", loader.calls, store.updateCalls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/currency/ -run TestRateUpdater -v`
Expected: FAIL — `NewRateUpdater`/`updateOnce`/`rateStore` undefined (compile error).

- [ ] **Step 3: Implement `internal/currency/rateupdater.go`**

```go
package currency

import (
	"context"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
)

// RateLoader fetches exchange rates for a date/base/symbol set. The Open
// Exchange Rates adapter (currency/repo.Loader) satisfies it; declared here so
// this package does not import its own repo subpackage.
type RateLoader interface {
	Load(ctx context.Context, date time.Time, base string, symbols []string) ([]model.RateInput, error)
}

// rateStore is the write-side surface the updater needs. *WriteService satisfies it.
type rateStore interface {
	LatestRateDate(ctx context.Context) (time.Time, bool, error)
	AvailableCodes(ctx context.Context) ([]string, error)
	UpdateRates(ctx context.Context, rates []model.RateInput) (int, error)
}

// RateUpdater periodically refreshes exchange rates in-process. Only serve
// starts it; when disabled (no interval, or no OER token) StartPolling is a
// no-op, so tests and the CLI stay hermetic.
type RateUpdater struct {
	enabled  bool
	interval time.Duration
	base     string
	loader   RateLoader
	svc      rateStore
	clock    port.Clock
}

// NewRateUpdater wires the updater. intervalDays is the refresh cadence in days;
// enabled must already fold in the OER-token presence.
func NewRateUpdater(enabled bool, intervalDays int, base string, loader RateLoader, svc rateStore, clock port.Clock) *RateUpdater {
	return &RateUpdater{
		enabled:  enabled,
		interval: time.Duration(intervalDays) * 24 * time.Hour,
		base:     base,
		loader:   loader,
		svc:      svc,
		clock:    clock,
	}
}

// StartPolling launches the background refresh (boot + every interval). No-op
// when disabled. Only serve calls this; it exits on ctx cancellation.
func (u *RateUpdater) StartPolling(ctx context.Context) {
	if !u.enabled {
		return
	}
	go func() {
		u.updateOnce(ctx)
		ticker := time.NewTicker(u.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				u.updateOnce(ctx)
			}
		}
	}()
}

// updateOnce refreshes rates unless the newest stored rate is already within the
// interval (DB-aware skip, so restart loops don't burn OER quota). All errors
// are logged and swallowed: a background job must never crash the server.
func (u *RateUpdater) updateOnce(ctx context.Context) {
	if !u.enabled {
		return
	}
	now := u.clock.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	if latest, ok, err := u.svc.LatestRateDate(ctx); err != nil {
		slog.Debug("rate updater: latest-date check failed", "err", err)
		// Fall through and try to fetch; a transient read error should not wedge
		// the schedule.
	} else if ok && today.Sub(latest) < u.interval {
		slog.Debug("rate updater: rates fresh, skipping", "latest", latest.Format("2006-01-02"))
		return
	}

	codes, err := u.svc.AvailableCodes(ctx)
	if err != nil {
		slog.Warn("rate updater: available-codes failed", "err", err)
		return
	}
	rates, err := u.loader.Load(ctx, now, u.base, codes)
	if err != nil {
		slog.Warn("rate updater: load failed", "err", err)
		return
	}
	n, err := u.svc.UpdateRates(ctx, rates)
	if err != nil {
		slog.Warn("rate updater: update failed", "err", err)
		return
	}
	slog.Info("rate updater: rates refreshed", "loaded", len(rates), "updated", n)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/currency/ -run TestRateUpdater -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/currency/rateupdater.go internal/currency/rateupdater_test.go
git commit -m "feat(currency): in-process RateUpdater (DB-aware, opt-in poller)"
```

---

### Task 4: Wiring — build the updater in `server.Build`, start it in `serve`

**Files:**
- Modify: `internal/server/server.go` (`Build` signature + return; construct write repo/service, loader, updater)
- Modify: `internal/server/server.go` (`BuildAPI` — discard the new return)
- Modify: `cmd/econumo/main.go` (`serve` — start the updater)
- Modify: `internal/server/glue_admin_test.go:37` (discard the new return)

**Interfaces:**
- Consumes: `config.Config.CurrencyUpdateIntervalDays`, `config.Config.OpenExchangeRatesToken` (Task 1), `appcurrency.NewRateUpdater` (Task 3), `appcurrency.NewWriteService`, `currencyrepo.NewWriteRepo`, `currencyrepo.NewLoader` (existing).
- Produces: `func Build(cfg config.Config, db *sql.DB, seams Seams) (http.Handler, http.Handler, *appcurrency.RateUpdater)`.

- [ ] **Step 1: Change `Build` to construct + return the updater**

In `internal/server/server.go`, change the `Build` signature:

```go
func Build(cfg config.Config, db *sql.DB, seams Seams) (http.Handler, http.Handler, *appcurrency.RateUpdater) {
```

Near the existing currency read wiring (after line ~186, `currencyHandlers := handlercurrency.NewHandlers(currencyReadSvc)`), add the write-side + updater construction:

```go
	// In-process exchange-rate updater. Enabled only when a cadence is set AND an
	// OER token is present; serve starts its poller (Build's third return),
	// BuildAPI and tests discard it (a disabled updater never polls).
	currencyWriteRepo := currencyrepo.NewWriteRepo(cfg.DatabaseDriver, txm)
	currencyWriteSvc := appcurrency.NewWriteService(currencyWriteRepo, txm, clk)
	rateLoader := currencyrepo.NewLoader(cfg.OpenExchangeRatesToken, clk.Now)
	rateUpdaterEnabled := cfg.CurrencyUpdateIntervalDays > 0 && cfg.OpenExchangeRatesToken != ""
	if cfg.CurrencyUpdateIntervalDays > 0 && cfg.OpenExchangeRatesToken == "" {
		slog.Warn("ECONUMO_CURRENCY_UPDATE_INTERVAL is set but OPEN_EXCHANGE_RATES_TOKEN is empty; in-process rate updates are disabled")
	}
	rateUpdater := appcurrency.NewRateUpdater(
		rateUpdaterEnabled, cfg.CurrencyUpdateIntervalDays, cfg.CurrencyBase,
		rateLoader, currencyWriteSvc, clk,
	)
```

`Build` currently ends (~line 311) with an inline first value:

```go
	return router.New(router.Deps{
		Cfg:                cfg,
		DB:                 pinger{db},
		RegisterAPI:        registerAPI,
		SupportedLanguages: i18n.Supported,
		MCP:                mcpHandler,
		SPA:                spaFS,
		SPAVersion:         spaVersion,
	}), adminHandler
```

Add `rateUpdater` as the third value — change the final line `}), adminHandler` to:

```go
	}), adminHandler, rateUpdater
```

Ensure `"log/slog"` is imported in `server.go` (add it if absent).

- [ ] **Step 2: Update `BuildAPI` to discard the third value**

In `internal/server/server.go`, `BuildAPI`:

```go
func BuildAPI(cfg config.Config, db *sql.DB, seams Seams) http.Handler {
	api, _, _ := Build(cfg, db, seams)
	return api
}
```

- [ ] **Step 3: Update the admin glue test caller**

In `internal/server/glue_admin_test.go:37`:

```go
	_, adminHandler, _ := server.Build(cfg, db.Raw, server.Seams{})
```

- [ ] **Step 4: Start the poller in `serve`**

In `cmd/econumo/main.go`, at the `server.Build` call (~line 222) and the poller start (~line 223):

```go
	handler, adminHandler, rateUpdater := server.Build(cfg, db, server.Seams{Updates: updates})
	updates.StartPolling(ctx)
	rateUpdater.StartPolling(ctx)
```

- [ ] **Step 5: Build the whole tree + run the server + config + currency suites**

Run: `go build ./... && go test ./internal/server/... ./internal/config/... ./internal/currency/...`
Expected: build OK; all PASS. (No golden/parity changes — those suites must still pass untouched.)

- [ ] **Step 6: Commit**

```bash
git add internal/server/server.go internal/server/glue_admin_test.go cmd/econumo/main.go
git commit -m "feat(server): wire + start the in-process currency-rate updater"
```

---

### Task 5: Documentation

**Files:**
- Modify: `.env.example` (near `OPEN_EXCHANGE_RATES_TOKEN`, ~line 68)
- Modify: `CLAUDE.md` (configuration list)

**Interfaces:** none (docs only).

- [ ] **Step 1: Add the `.env.example` entry**

In `.env.example`, after the `# OPEN_EXCHANGE_RATES_TOKEN=` line (~line 68):

```bash
# Optional: refresh exchange rates in-process every N days (requires
# OPEN_EXCHANGE_RATES_TOKEN above). Unset or 0 = off; the server never fetches
# rates on its own and you drive currency:update-rates from your own cron. When
# set, serve refreshes on boot and every N days, skipping the fetch while stored
# rates are still within N days (so restarts don't waste API quota).
# ECONUMO_CURRENCY_UPDATE_INTERVAL=1
```

- [ ] **Step 2: Add the CLAUDE.md config entry**

In `CLAUDE.md`, in the configuration bullet list, add an entry after the `OPEN_EXCHANGE_RATES_TOKEN` line (search for `OPEN_EXCHANGE_RATES_TOKEN — currency-rate updates.`):

```markdown
- `ECONUMO_CURRENCY_UPDATE_INTERVAL` — refresh exchange rates in-process every N
  DAYS. `0` (default/unset) = off (drive `currency:update-rates` from an external
  cron as before). A positive `N` starts a background poller in `serve` **only
  when `OPEN_EXCHANGE_RATES_TOKEN` is also set** (interval-without-token logs a
  WARN at boot and stays off). Negative/malformed fails at boot. The poller
  refreshes on boot then every N days and is DB-aware: it skips the fetch while
  the newest stored rate is within N days, so a restart loop never burns API
  quota. Idempotent per `(date, currency, base)`, so it is safe alongside an
  existing external cron.
```

- [ ] **Step 3: Verify docs render / no stray tokens**

Run: `grep -n "ECONUMO_CURRENCY_UPDATE_INTERVAL" .env.example CLAUDE.md`
Expected: one match in each file (plus the commented example line in `.env.example`).

- [ ] **Step 4: Commit**

```bash
git add .env.example CLAUDE.md
git commit -m "docs: ECONUMO_CURRENCY_UPDATE_INTERVAL (.env.example + CLAUDE.md)"
```

---

### Task 6: Full-suite verification

**Files:** none (verification only).

- [ ] **Step 1: Run the smoke suite**

Run: `make go-test`
Expected: PASS — build + vet + gofmt + OpenAPI-fresh + sqlite unit/integration + coverage ≥ 78. In particular apiparity/mcpparity goldens pass UNCHANGED (no new routes).

- [ ] **Step 2: (If Postgres available) run the pgsql repo suite**

Run: `make test-repo-pgsql`
Expected: PASS — exercises the pgsql `GetLatestRateDate` adapter. If no Postgres is provisioned locally, note it as skipped; CI covers it.

- [ ] **Step 3: Final gofmt/vet sweep**

Run: `gofmt -l internal cmd && go vet ./...`
Expected: no gofmt output; vet clean.

---

## Self-Review Notes

- **Spec coverage:** config var (Task 1), DB-aware freshness query (Task 2), poller with boot+ticker and error-swallowing (Task 3), enablement gate + token WARN + wiring + serve start (Task 4), `.env.example`/CLAUDE.md docs (Task 5), no-new-routes verification (Task 6). All spec sections mapped.
- **Type consistency:** `LatestRateDate(ctx) (time.Time, bool, error)` is identical across `WriteModel`, `WriteRepo`, `WriteService`, and the test `fakeStore`. `RateLoader.Load` matches `repo.Loader.Load` exactly. `NewRateUpdater(enabled, intervalDays, base, loader, svc, clock)` matches every call site (test + wiring). `Build` returns 3 values; all three callers (`BuildAPI`, `main.go`, `glue_admin_test.go`) updated.
- **Freshness math:** `today.Sub(latest) < interval` with `interval = days*24h` and both dates at midnight UTC → with `days=1`, same-day skips, previous-day fetches (once per calendar day). Matches intent.
