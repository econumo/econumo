# Base Currency Boot Resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve `ECONUMO_CURRENCY_BASE` to its global currency row exactly once at boot (fail fast on a bad code, WARN on rate-base drift) and thread the resolved `model.BaseCurrency{ID, Code}` through the currency services, deleting every runtime code→id re-resolution — per the approved spec `docs/superpowers/specs/2026-07-31-base-currency-boot-resolution-design.md`.

**Architecture:** `server.Build` gains an error return and resolves the base right after the currency lookup is wired; `BuildAPI` panics on the (test-impossible) error. `ReadService`, `ManageService`, and `RateProvider` switch from a base *code* to the resolved value; the OXR paths (rate updater, CLI) and CSV import keep the code on purpose. A new `WriteRepo.LatestRateBase` powers the drift WARN.

**Tech Stack:** Go stdlib, sqlc (engine-adapter pattern), dbtest/fixture, slog.

## Global Constraints

- Behavior for valid configurations is **identical**: apiparity/mcpparity goldens must not change (`git status internal/test/*/testdata/` stays clean); frozen error strings (`"The base currency cannot be modified"`, `"This currency cannot be hidden"`) unchanged.
- Boot-failure message (exact): `ECONUMO_CURRENCY_BASE=%q does not match any global currency; add it first with currency:add` .
- OXR paths (`NewRateUpdater`, `internal/cli/currency_commands.go`) and CSV import (`NewTransactionImportAccounts`) keep the base **code** — import resolves it user-aware (pinned by `TestImport_NewAccount_PrefersOwnCustomCurrencyOverGlobalCode`); do NOT pre-resolve those to the global id.
- The CLI container (`internal/cli/container.go`) is untouched — `currency:add` must work before the base resolves.
- sqlc regeneration: `cd internal/infra/storage/sqlc && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate` (v1.30.0 pinned — a newer local sqlc changes EXISTS typing; comments in `.sql` files ASCII-only).
- `gofmt` before every commit; conventional-commit messages.
- Seeded USD global currency id (baseline migration): `dffc2a06-6f29-4704-8575-31709adee926`.

---

### Task 1: `model.BaseCurrency`, boot resolution in `server.Build`, drift WARN

**Files:**
- Modify: `internal/model/currency.go` (add `BaseCurrency`)
- Modify: `internal/infra/storage/sqlc/query/sqlite/currency_write.sql`, `internal/infra/storage/sqlc/query/pgsql/currency_write.sql` (add `GetLatestRateBase`)
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}`
- Modify: `internal/currency/repo/write.go` (add `LatestRateBase`)
- Modify: `internal/server/server.go` (`Build` error return + resolution + drift WARN; `BuildAPI` panics)
- Modify: `cmd/econumo/main.go:222` (handle the Build error)
- Test: `internal/server/basecurrency_test.go` (new), extend `internal/currency/repo/write_integration_test.go` if present, else a new `internal/currency/repo/latestratebase_integration_test.go`

**Interfaces:**
- Produces (used by Tasks 2–4):
  - `model.BaseCurrency{ID string; Code string}`
  - inside `Build`, a local `base := model.BaseCurrency{ID: ..., Code: cfg.CurrencyBase}` available to all later wiring
  - `Build(cfg config.Config, db *sql.DB, seams Seams) (http.Handler, http.Handler, *appcurrency.RateUpdater, error)`
  - `(*currencyrepo.WriteRepo).LatestRateBase(ctx) (baseID string, ok bool, err error)` — `ok=false` when no rates exist

- [ ] **Step 1: Write the failing boot-resolution tests**

Create `internal/server/basecurrency_test.go` (package `server_test`; copy the `cfg` literal style from `internal/server/mcp_test.go`):

```go
package server_test

// Boot-time base-currency behavior: an unresolvable ECONUMO_CURRENCY_BASE
// refuses to serve, and rates stored against a different base draw a WARN
// (nothing is rewritten -- old rows are inert by design).

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

func baseTestConfig(engine string) config.Config {
	return config.Config{
		DatabaseDriver:     engine,
		CurrencyBase:       "USD",
		AllowRegistration:  true,
		CORSAllowedOrigins: []string{"*"},
		RateLimitLogin:     5,
		RateLimitReset:     5,
		RateLimitRemind:    3,
		RateLimitRegister:  5,
		RateLimitWindow:    15 * time.Minute,
		RateLimitGlobal:    60,
	}
}

func TestBuild_UnknownBaseCurrency_Fails(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := baseTestConfig(db.Engine)
	cfg.CurrencyBase = "XXX"
	_, _, _, err := server.Build(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})
	if err == nil {
		t.Fatal("Build with an unknown base code must fail")
	}
	want := `ECONUMO_CURRENCY_BASE="XXX" does not match any global currency; add it first with currency:add`
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

// warnRecorder captures WARN-level slog output for the drift assertion.
type warnRecorder struct {
	slog.Handler
	msgs *[]string
}

func (h warnRecorder) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		*h.msgs = append(*h.msgs, r.Message)
	}
	return nil
}

func captureWarns(t *testing.T) *[]string {
	t.Helper()
	msgs := &[]string{}
	prev := slog.Default()
	slog.SetDefault(slog.New(warnRecorder{Handler: prev.Handler(), msgs: msgs}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return msgs
}

func TestBuild_RateBaseDrift_Warns(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	eur := f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	f.Rate(fixture.Rate{CurrencyID: eur, BaseCurrencyID: eur, Date: "2026-07-01"})

	msgs := captureWarns(t)
	if _, _, _, err := server.Build(baseTestConfig(db.Engine), db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(*msgs, "\n")
	if !strings.Contains(joined, "different base currency") {
		t.Fatalf("want a rate-base drift WARN, got warns: %q", joined)
	}
}

func TestBuild_NoRates_NoDriftWarn(t *testing.T) {
	db := dbtest.NewSQLite(t)
	msgs := captureWarns(t)
	if _, _, _, err := server.Build(baseTestConfig(db.Engine), db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}); err != nil {
		t.Fatal(err)
	}
	for _, m := range *msgs {
		if strings.Contains(m, "different base currency") {
			t.Fatalf("unexpected drift WARN: %q", m)
		}
	}
}
```

Adapt the two fixture calls to the REAL fixture API: check `internal/test/fixture/entities.go` for the `Currency` and rate helpers (a rate helper may be named `Rate` or similar and may take a struct or positional args; if none exists, insert the rate row with `db.Raw.Exec` using `db.Rebind` — columns `(id, currency_id, base_currency_id, published_at, rate)`). Keep the assertions exactly as written.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/server/ -run 'TestBuild_' -v 2>&1 | tail -10`
Expected: compile FAIL — `Build` has 3 return values, tests destructure 4. That is the correct red state (signature change is the feature).

- [ ] **Step 3: Add `model.BaseCurrency`**

Append to `internal/model/currency.go`:

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

- [ ] **Step 4: Add the `GetLatestRateBase` query (both engines) and regenerate**

Append to `internal/infra/storage/sqlc/query/sqlite/currency_write.sql` (after `GetLatestRateDate`):

```sql
-- name: GetLatestRateBase :one
-- Base currency of the newest stored rate, for the boot-time drift WARN when
-- ECONUMO_CURRENCY_BASE no longer matches what the rates were written against.
SELECT base_currency_id FROM currencies_rates ORDER BY published_at DESC LIMIT 1;
```

Same statement in `internal/infra/storage/sqlc/query/pgsql/currency_write.sql` (no placeholders needed). Regenerate:

Run: `cd internal/infra/storage/sqlc && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate && cd - && go build ./internal/infra/...`
Expected: build passes.

- [ ] **Step 5: Add `WriteRepo.LatestRateBase`**

In `internal/currency/repo/write.go`, mirror `LatestRateDate` exactly (interface entry + both engine querier methods + repo method):

```go
// in writeQuerier interface:
	GetLatestRateBase(ctx context.Context, db backend.DBTX) (string, error)

// repo method (next to LatestRateDate):
// LatestRateBase returns the base currency id of the newest stored rate;
// ok=false when no rates exist.
func (r *WriteRepo) LatestRateBase(ctx context.Context) (string, bool, error) {
	id, err := r.q.GetLatestRateBase(ctx, r.db(ctx))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

// engine adapters (next to the GetLatestRateDate ones):
func (sqliteWriteQuerier) GetLatestRateBase(ctx context.Context, db backend.DBTX) (string, error) {
	return sqlitegen.New(db).GetLatestRateBase(ctx)
}
func (pgsqlWriteQuerier) GetLatestRateBase(ctx context.Context, db backend.DBTX) (string, error) {
	return pgsqlgen.New(db).GetLatestRateBase(ctx)
}
```

(If the pgsql generated return type differs from `string`, convert in the pgsql adapter — whole-value cast, same as the neighboring shims.)

- [ ] **Step 6: Resolve in `Build`, WARN on drift, error return, `BuildAPI` panic, serve handling**

In `internal/server/server.go`:

1. Change the signatures:

```go
func BuildAPI(cfg config.Config, db *sql.DB, seams Seams) http.Handler {
	api, _, _, err := Build(cfg, db, seams)
	if err != nil {
		// Test-only entry: every harness uses the seeded USD base. serve calls
		// Build directly and reports the error.
		panic(err)
	}
	return api
}

func Build(cfg config.Config, db *sql.DB, seams Seams) (http.Handler, http.Handler, *appcurrency.RateUpdater, error) {
```

2. Immediately after `currencyLookup := currencyrepo.New(cfg.DatabaseDriver, txm)` add:

```go
	// The base currency is resolved exactly once: every stored rate is
	// denominated "X per 1 base unit", so consumers get the resolved row, not
	// a code to re-look-up. A bad code refuses to boot instead of failing at
	// runtime as lookup errors.
	baseID, err := currencyLookup.GetIDByCode(context.Background(), cfg.CurrencyBase)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ECONUMO_CURRENCY_BASE=%q does not match any global currency; add it first with currency:add", cfg.CurrencyBase)
	}
	base := model.BaseCurrency{ID: baseID, Code: cfg.CurrencyBase}
```

(`context` / `fmt` / `model` imports may need adding.)

3. Right after `currencyWriteRepo := currencyrepo.NewWriteRepo(cfg.DatabaseDriver, txm)` add the drift WARN (an admin MAY change the base; old rows stay untouched and inert — the WARN is the only acknowledgment, per the spec):

```go
	if latestBase, ok, lbErr := currencyWriteRepo.LatestRateBase(context.Background()); lbErr == nil && ok && latestBase != base.ID {
		slog.Warn("stored rates were written against a different base currency; conversions will use only rates written against the current base",
			"configured_base", base.Code, "stored_base_currency_id", latestBase)
	}
```

4. Every `return` in `Build` gains the trailing `nil` error; the final return returns the three values plus `nil`.

In `cmd/econumo/main.go` (line ~222):

```go
	handler, adminHandler, rateUpdater, err := server.Build(cfg, db, server.Seams{Updates: updates})
	if err != nil {
		return err
	}
```

- [ ] **Step 7: Run the new tests + the affected packages**

Run: `go build ./... && go test ./internal/server/ ./internal/currency/... -count=1 2>&1 | grep -vE "INFO|WARN" | tail -8`
Expected: all PASS, including the three `TestBuild_*` tests.

- [ ] **Step 8: Commit**

```bash
gofmt -l internal/ cmd/ | grep -v /gen/
git add -A
git commit -m "feat(currency): resolve the base currency once at boot; WARN on rate-base drift"
```

---

### Task 2: `ReadService` takes `model.BaseCurrency`

**Files:**
- Modify: `internal/currency/read.go`
- Modify: `internal/server/server.go` (call site), `internal/currency/read_scoping_test.go`, `internal/currency/api/harness_test.go`, `internal/currency/mcp/mcp_test.go`

**Interfaces:**
- Consumes: `model.BaseCurrency`, the `base` local in `Build` (Task 1).
- Produces: `currency.NewReadService(read ReadModel, base model.BaseCurrency) *ReadService`.

- [ ] **Step 1: Switch the service**

In `internal/currency/read.go`: field `baseCode string` becomes `base model.BaseCurrency`; `NewReadService(read ReadModel, base model.BaseCurrency)`. In `GetCurrencyRateList`, delete the visible-list code scan (`baseID := ""` / `if v.UserID == nil && v.Code == s.baseCode`) and use `s.base.ID` directly:

```go
	visibleSet := make(map[string]bool, len(visible))
	for _, v := range visible {
		visibleSet[v.ID] = true
	}
	...
		if r.CurrencyID == s.base.ID {
			baseHasRate = true
		}
	...
	if !baseHasRate && len(items) > 0 {
		items = append(items, model.CurrencyRateResult{
			CurrencyId:     s.base.ID,
			BaseCurrencyId: s.base.ID,
			Rate:           "1",
			UpdatedAt:      latest,
		})
	}
```

(The `baseID != ""` guard disappears — the composition root guarantees resolution. Globals are always visible, so the base needs no visibility check.)

- [ ] **Step 2: Update the call sites**

- `internal/server/server.go`: `appcurrency.NewReadService(currencyReadRepo, base)`
- `internal/currency/read_scoping_test.go`: `NewReadService(fake, model.BaseCurrency{ID: "visible-usd", Code: "USD"})` in `FiltersToVisible`; `{ID: "usd-id", Code: "USD"}` in the three synthetic-rate tests and in `GetCurrencyList_ScopeAndFlags` use `{ID: "global-usd", Code: "USD"}`.
- `internal/currency/api/harness_test.go`: `appcurrency.NewReadService(readRepo, model.BaseCurrency{ID: usdID, Code: cfg.CurrencyBase})` (the `usdID` const already exists in that package).
- `internal/currency/mcp/mcp_test.go`: `appcurrency.NewReadService(currencyrepo.NewReadRepo(db.Engine, db.TX), model.BaseCurrency{ID: "dffc2a06-6f29-4704-8575-31709adee926", Code: "USD"})`.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/currency/... ./internal/server/ -count=1 2>&1 | grep -vE "INFO|WARN" | tail -6`
Expected: PASS — every existing synthetic-rate assertion holds unchanged.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "refactor(currency): ReadService takes the resolved base currency"
```

---

### Task 3: `ManageService` takes `model.BaseCurrency`; retire `GetGlobalIDByCode`

**Files:**
- Modify: `internal/currency/manage.go`, `internal/currency/rates.go`, `internal/currency/visibility.go`
- Modify: `internal/currency/repo/manage.go` (drop `GetGlobalIDByCode` + querier entries), `internal/infra/storage/sqlc/query/{sqlite,pgsql}/currency_write.sql` (drop `GetGlobalCurrencyIDByCode`), regenerate sqlc
- Modify: `internal/server/server.go`, `internal/currency/manage_test.go`, `internal/currency/api/harness_test.go`, `internal/currency/repo/manage_integration_test.go` (if it exercises `GetGlobalIDByCode`)

**Interfaces:**
- Consumes: `model.BaseCurrency`, the `base` local in `Build`.
- Produces: `currency.NewManageService(repo ManageModel, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, profile ProfileCurrency, base model.BaseCurrency) *ManageService`; `ManageModel` loses `GetGlobalIDByCode`.

- [ ] **Step 1: Switch the service**

`internal/currency/manage.go`: struct field `baseCode string` → `base model.BaseCurrency`; ctor parameter `base model.BaseCurrency`; delete `GetGlobalIDByCode(ctx context.Context, code string) (string, error)` from the `ManageModel` interface. At `manage.go:201` (the optional initial rate in `CreateCurrency`) replace

```go
			baseID, berr := s.repo.GetGlobalIDByCode(ctx, s.baseCode)
			if berr != nil {
				return berr
			}
```

with direct use of `s.base.ID` (delete the lookup; use `BaseCurrencyID: s.base.ID` in the `model.RateRow`). Same in `internal/currency/rates.go` (`SetCurrencyRate`): delete the `GetGlobalIDByCode` call, use `s.base.ID`.

`internal/currency/visibility.go`: the base guard `if rec.Code == s.baseCode` becomes `if rec.ID == s.base.ID` (both occurrences if `ShowCurrency` has one too — grep `baseCode` in the file). Frozen messages unchanged.

- [ ] **Step 2: Retire the repo method and query**

- `internal/currency/repo/manage.go`: delete `GetGlobalIDByCode` (repo method), the `GetGlobalCurrencyIDByCode` entry in the `manageQuerier` interface, and both engine adapter methods.
- Delete the `-- name: GetGlobalCurrencyIDByCode :one` block from BOTH `query/sqlite/currency_write.sql` and `query/pgsql/currency_write.sql`.
- Regenerate: `cd internal/infra/storage/sqlc && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate && cd -`

- [ ] **Step 3: Update call sites and tests**

- `internal/server/server.go`: `appcurrency.NewManageService(currencyManageRepo, txm, opGuard, clk, NewCurrencyProfileCurrency(userRepo), base)`
- `internal/currency/manage_test.go`: add `var testBase = model.BaseCurrency{ID: "global-usd", Code: "USD"}` near the top; both `NewManageService(...)` call sites (lines ~166 and ~687) pass `testBase` instead of `"USD"`. The fakes use `"global-usd"` as the USD record id, so every existing assertion (`BaseCurrencyID == "global-usd"`, the `TestHideCurrency_BaseCurrency` guard) holds. Delete the fake repo's `GetGlobalIDByCode` method if it has one.
- `internal/currency/api/harness_test.go`: `appcurrency.NewManageService(manageRepo, tdb.TX, opGuard, clk, profile, model.BaseCurrency{ID: usdID, Code: cfg.CurrencyBase})`
- `internal/currency/repo/manage_integration_test.go`: delete any test exercising `GetGlobalIDByCode` (grep first; if one exists it tests a method that no longer exists).

- [ ] **Step 4: Run tests**

Run: `go build ./... && go test ./internal/currency/... -count=1 2>&1 | grep -vE "INFO|WARN" | tail -6`
Expected: PASS. In particular `TestSetCurrencyRate_*` and `TestHideCurrency_BaseCurrency` unchanged and green.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "refactor(currency): ManageService takes the resolved base; drop per-request GetGlobalIDByCode"
```

---

### Task 4: `RateProvider` takes the resolved base id

**Files:**
- Modify: `internal/currency/repo/convertor_provider.go`
- Modify: `internal/server/server.go`, `internal/currency/repo/convertor_provider_test.go`, `internal/budget/mcp/mcp_test.go` (constructs a RateProvider)

**Interfaces:**
- Consumes: `base.ID` from `Build`.
- Produces: `currencyrepo.NewRateProvider(driver string, tx *backend.TxManager, lookup *Lookup, baseID string) *RateProvider`.

- [ ] **Step 1: Switch the provider**

In `internal/currency/repo/convertor_provider.go`: field `baseCode string` → `baseID string`; ctor doc + parameter `baseID string` ("the boot-resolved base currency id"); `BaseCurrencyID` stops resolving:

```go
// BaseCurrencyID returns the boot-resolved base currency's id.
func (p *RateProvider) BaseCurrencyID(ctx context.Context) (vo.Id, error) {
	return vo.ParseId(p.baseID)
}
```

Grep the file for other `p.baseCode` uses (e.g. inside `AverageRates`) and replace with `p.baseID` — if any call resolves code→id first, delete the resolution and use `p.baseID` directly.

- [ ] **Step 2: Update call sites**

- `internal/server/server.go`: `currencyrepo.NewRateProvider(cfg.DatabaseDriver, txm, currencyLookup, base.ID)`
- `internal/currency/repo/convertor_provider_test.go` (2 sites): pass `usdID` (package const, `dffc2a06-...`) instead of `"USD"`.
- `internal/budget/mcp/mcp_test.go`: `currencyrepo.NewRateProvider(db.Engine, txm, currencyLookup, "dffc2a06-6f29-4704-8575-31709adee926")` (was `"USD"`).

- [ ] **Step 3: Run tests**

Run: `go test ./internal/currency/... ./internal/budget/... -count=1 2>&1 | grep -vE "INFO|WARN" | tail -6`
Expected: PASS (the provider tests assert the same averages; only the ctor argument changed).

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "refactor(currency): RateProvider takes the boot-resolved base id"
```

---

### Task 5: Full verification — goldens must be byte-identical

- [ ] **Step 1: Smoke suite + golden check**

Run: `make go-test 2>&1 | tail -2 && git status --short internal/test/apiparity/testdata/ internal/test/mcpparity/testdata/`
Expected: `SMOKE suite passed`; `git status` prints NOTHING for the testdata dirs — an identical golden set is the proof the refactor is behavior-neutral. If any golden changed, that is a BUG in the refactor: diff it and fix the code, do not regenerate.

- [ ] **Step 2: Engine comparison (PostgreSQL)**

Run: `CGO_ENABLED=0 DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@localhost:5432/econumo_test?sslmode=disable' go test -tags enginecompare ./internal/test/enginecompare/ ./internal/test/mcpparity/ 2>&1 | tail -3`
Expected: both `ok` (the new `GetLatestRateBase` pgsql adapter and the drift path run on both engines).

- [ ] **Step 3: Commit anything outstanding**

```bash
git status --short
git add -A && git commit -m "test(currency): verify base-currency boot resolution end to end" || echo "nothing to commit"
```
