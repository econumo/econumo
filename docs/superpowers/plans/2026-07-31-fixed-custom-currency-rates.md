# Fixed Custom-Currency Rates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace dated custom-currency rate history with one fixed, mandatory, retroactive rate stored on the currency row — per the approved spec `docs/superpowers/specs/2026-07-31-fixed-custom-currency-rates-design.md`.

**Architecture:** `currencies.rate` column (NULL for globals); migration backfills latest custom rate and purges custom rows from `currencies_rates`. `RateProvider.AverageRates` overlays fixed rates period-independently. `get-currency-rate-list` synthesizes custom rows from the column. `create/update-currency` require `rate`; `set-currency-rate` (route, use case, DTOs, RateDialog, "Set rate" action) is deleted.

**Tech Stack:** Go + sqlc v1.30.0, React 19, vitest/msw.

## Global Constraints

- Rate direction unchanged: "X units per 1 base unit" against the boot-resolved base.
- Frozen strings: `"Rate must be a positive number"` (field `rate`) stays; blank rate → `"This value should not be blank."` (code `common.is_blank`). `"The base currency cannot be modified"` no longer has a set-rate path but keeps its hide-currency path.
- Legacy customs with no rate migrate to NULL and serve no rate row (no data invented).
- Guard-floor adjustments are one-time and commented: apiparity `minRoutes` 102 → 101 (deliberate pre-release route removal). The scenario-count floor in `catalogue_test.go` is unchanged (no scenario is removed, only calls within one).
- sqlc regen: `cd internal/infra/storage/sqlc && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate`; swagger via `make swagger`; gofmt before each commit.
- Goldens: regenerate with `UPDATE_GOLDEN=1` and INSPECT — expected diffs only in currency scenarios (create/update carrying rate, rate-list gaining synthesized custom rows, set-rate steps gone).
- errs: `CodeCurrencyDateInvalid` retired from `internal/shared/errs/codes.go` + `AllCodes` + `errors.currency.date_invalid` in BOTH `locales/{en,ru}.json` (two-way guard).

---

### Task 1: Migration + fixture

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260801000000.sql`, `internal/infra/storage/migrations/pgsql/20260801000000.sql`
- Create: `internal/infra/storage/migrations/migration_20260801_test.go`
- Modify: `internal/test/fixture/entities.go` (`Currency` gains `Rate string` — empty = NULL)

- [ ] **Step 1: Failing migration test**

`migration_20260801_test.go` (package `migrations_test`, reuse `openFK`/`toRunList` from `migration_20260730_test.go` — same package, no redeclare):

```go
func TestMigration20260801_FixedRateBackfill(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before []migrate.Migration
	for _, m := range all {
		if m.Version < "20260801000000" {
			before = append(before, m)
		}
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	seed := []string{
		`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		 VALUES ('u1', 'i1', 'a@b.c', 'A', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`,
		`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p1', 'PTS', 'pts', '2026-01-01 00:00:00', 'u1')`,
		`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('n1', 'NOR', 'n', '2026-01-01 00:00:00', 'u1')`,
		// custom rate history: the LATEST row must win the backfill
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r1', 'p1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-10', '5.00000000')`,
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r2', 'p1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-02-10', '10.00000000')`,
		// a global (EUR-style) rate row must survive untouched
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r3', 'dffc2a06-6f29-4704-8575-31709adee926', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-02-10', '1.00000000')`,
	}
	for _, s := range seed {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}
	var rate sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT rate FROM currencies WHERE id = 'p1'").Scan(&rate); err != nil {
		t.Fatal(err)
	}
	if !rate.Valid || rate.String != "10.00000000" {
		t.Fatalf("backfilled rate = %+v, want the LATEST row 10.00000000", rate)
	}
	if err := db.QueryRowContext(ctx, "SELECT rate FROM currencies WHERE id = 'n1'").Scan(&rate); err != nil {
		t.Fatal(err)
	}
	if rate.Valid {
		t.Fatalf("no-history custom must stay NULL, got %q", rate.String)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies_rates WHERE currency_id = 'p1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("custom rate rows purged = %d rows left, err %v", n, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies_rates").Scan(&n); err != nil || n != 1 {
		t.Fatalf("global rate rows must survive; total = %d, err %v", n, err)
	}
}
```

- [ ] **Step 2: Verify RED** — `go test -run TestMigration20260801 ./internal/infra/storage/migrations/ -v` → FAIL (migration missing → `rate` column unknown).

- [ ] **Step 3: Write the migrations**

sqlite `20260801000000.sql`:

```sql
-- Custom currencies get ONE fixed rate (no history): currencies.rate holds
-- "X units per 1 base unit" for customs (NULL for globals). Backfill each
-- custom's LATEST dated rate, then purge custom rows from currencies_rates,
-- which returns to globals-only OXR history. Customs created without a rate
-- stay NULL (no data invented); the first edit forces one in.
ALTER TABLE currencies ADD COLUMN rate NUMERIC(19, 8) DEFAULT NULL;
UPDATE currencies SET rate = (
    SELECT cr.rate FROM currencies_rates cr
    WHERE cr.currency_id = currencies.id
    ORDER BY cr.published_at DESC LIMIT 1
) WHERE user_id IS NOT NULL;
DELETE FROM currencies_rates WHERE currency_id IN (SELECT id FROM currencies WHERE user_id IS NOT NULL);
```

pgsql: identical statements (correlated subquery + `LIMIT 1` are valid pgsql; keep the same comment).

- [ ] **Step 4: Fixture** — `fixture.Currency` gains `Rate string`; the INSERT adds the column (`nullable(c.Rate)`).

- [ ] **Step 5: GREEN + commit** — migration test passes; `go test ./internal/infra/... ./internal/test/... -count=1` (sqlc not yet regenerated, so currency repos untouched). Commit `feat(currency): migration to fixed custom rates + backfill`.

---

### Task 2: sqlc + repos + provider overlay

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/currency_write.sql` (InsertUserCurrency + UpdateCurrencyDetails + GetCurrencyRecord gain `rate`; NEW `GetFixedCurrencyRates`), `currency_read.sql` (GetUserCurrencyListView gains `c.rate, c.created_at`), regenerate
- Modify: `internal/model/currency.go` (`CurrencyRecord.Rate *string`), `internal/model/currency_view.go` (`CurrencyViewRow` gains `Rate *string; CreatedAt time.Time`)
- Modify: `internal/currency/repo/manage.go` (param/row mapping; DELETE `UpsertRate`), `internal/currency/repo/read.go` (row mapping), `internal/currency/repo/convertor_provider.go` (overlay)
- Tests: extend `internal/currency/repo/manage_integration_test.go`, `internal/currency/repo/convertor_provider_test.go`

**Interfaces (produced):**
- `GetFixedCurrencyRates :many` → `SELECT id, rate FROM currencies WHERE rate IS NOT NULL;` (both engines)
- `providerQuerier` gains `GetFixedRates(ctx, db) ([]fixedRateRow, error)` (`fixedRateRow{CurrencyID, Rate string}`)
- `AverageRates` = AVG result + one `model.FullRate` per fixed-rate currency (customs are absent from AVG after Task 1's purge)
- `ManageModel` loses `UpsertRate`; `InsertUserCurrency`/`UpdateCurrencyDetails` carry the rate via `CurrencyRecord.Rate` / a new `rate *string` arg: change `UpdateCurrencyDetails(ctx, id, name, symbol string, fractionDigits int, rate string)` (rate REQUIRED at the service layer, so plain string)

- [ ] **Step 1: Failing provider test** — in `convertor_provider_test.go`: seed a custom currency with fixed rate `10` (fixture `Rate: "10.00000000"`) and NO rate rows; request a HISTORICAL period (e.g. April 2024, while a global EUR rate row exists in Jan 2026); assert the result includes the custom at `10` (this is exactly the case that fails today) and EUR at its average.
- [ ] **Step 2: RED**, then queries + regenerate + mappings + overlay (append `model.FullRate{CurrencyID, Rate: vo.NewDecimal(r.Rate)}` after the AVG loop). Fix pgsql shims per compile errors.
- [ ] **Step 3: Manage repo test updates** — Insert/Get roundtrip asserts `Rate`; delete the `UpsertRate` test block.
- [ ] **Step 4: GREEN + commit** — `go test ./internal/currency/... -count=1`. Commit `feat(currency): fixed-rate column through repos + period-independent provider overlay`.

---

### Task 3: Manage service — mandatory rate; delete SetCurrencyRate

**Files:**
- Modify: `internal/model/currency_dto.go` (Create: `Rate *string` REQUIRED via Validate blank-check; Update gains `Rate string` + blank-check; DELETE `SetCurrencyRateRequest/Result`)
- Modify: `internal/currency/manage.go` (create stores `rec.Rate`, drops the UpsertRate call; update passes rate through; delete `todayIn` if now unused)
- Delete: `internal/currency/rates.go`
- Modify: `internal/shared/errs/codes.go` (retire `CodeCurrencyDateInvalid`), `locales/{en,ru}.json` (drop `errors.currency.date_invalid`)
- Tests: `internal/currency/manage_test.go` (create/update require rate; delete SetCurrencyRate tests + `fakeManageRepo.UpsertRate`)

- [ ] **Step 1: Failing service tests** — create without rate → validation `rate`/`"This value should not be blank."`; create with bad shape → `"Rate must be a positive number"`; create stores `Rate` on the record (no `repo.rates` entries); update without rate → blank error; update with rate persists it.
- [ ] **Step 2: RED → implement → GREEN.** Run `go test ./internal/currency/... ./internal/test/i18ntest/ -count=1`.
- [ ] **Step 3: Commit** `feat(currency): mandatory fixed rate on create/update; retire set-currency-rate use case`.

---

### Task 4: Read synthesis of custom rate rows

**Files:**
- Modify: `internal/currency/read.go` (`GetCurrencyRateList` appends synthesized rows), `internal/currency/read_scoping_test.go`

- [ ] **Step 1: Failing test** — fake rows include an own custom `{Rate: strPtr("10"), CreatedAt: <fixed time>}` and a foreign-shared custom with a rate; no stored rate rows at all EXCEPT one global. Expect: global row, both customs' synthesized rows `{id, base.ID, "10", created_at as datetime.Layout}`, synthetic base row. A NULL-rate custom yields no row.
- [ ] **Step 2: Implement** — after the real-row loop (and before the synthetic-base append), iterate `visible`: `if v.Rate != nil` append `model.CurrencyRateResult{CurrencyId: v.ID, BaseCurrencyId: s.base.ID, Rate: vo.NewDecimal(*v.Rate).String(), UpdatedAt: v.CreatedAt.Format(datetime.Layout)}` and treat a custom row for the base as impossible (customs are never the base). `baseHasRate`/`latest` keep considering ONLY real stored rows.
- [ ] **Step 3: GREEN + commit** `feat(currency): serve fixed custom rates through get-currency-rate-list`.

---

### Task 5: HTTP edge + parity

**Files:**
- Modify: `internal/currency/api/currency.go` (delete SetCurrencyRate handler), `routes.go` (drop the route), `internal/currency/api/currency_endpoints_test.go` (rework set-rate tests: update-currency now carries rate; create requires it)
- Modify: `internal/test/apiparity/catalogue.go` (drop both set-rate calls; add `"rate"` to create/duplicate/update/foreign bodies; add an `err:create-missing-rate` call), `guard_test.go` (`minRoutes` 101 + comment)
- Regenerate: `make swagger`, goldens

- [ ] **Step 1: Apply edge deletions + catalogue rework.** Catalogue body updates: `create-currency` keeps `"rate": "100"`; `err:create-duplicate-code` gains `"rate": "3"` (so it still exercises the duplicate branch, not the new blank-rate error); new call `{Label: "err:create-missing-rate", ... Body: map[string]any{"id": <fresh op uuid>, "code": "PTX", "name": "No rate"}}`; `update-currency` gains `"rate": "120.5"`; `err:update-foreign` gains `"rate": "1"`.
- [ ] **Step 2: `make swagger`; `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`; inspect diffs** — expected: currency goldens only (rate in update body responses is not echoed; rate-list rows change; set-rate steps gone; new missing-rate error).
- [ ] **Step 3: `make go-test` + enginecompare (`DATABASE_TEST_PGSQL_URL=... go test -tags enginecompare ./internal/test/...`).**
- [ ] **Step 4: Commit** `feat(currency)!: delete set-currency-rate; rate rides create/update`.

---

### Task 6: Frontend

**Files:**
- Modify: `web/src/api/dto/currency.ts` (Update gains `rate: string`; delete SetCurrencyRate types), `web/src/api/currency.ts` (delete `setCurrencyRate`; `updateCurrency` gains rate), `web/src/features/currencies/queries.ts` (delete `useSetCurrencyRate`)
- Modify: `web/src/features/currencies/CurrencyDialog.tsx` (rate field in edit mode, required both modes, prefilled via a new `currentRate?: string` prop)
- Modify: `web/src/features/currencies/CurrenciesPage.tsx` (drop RateDialog + "Set rate" action + rateError; pass `currentRate={rateFor(currency.id)?.rate}`; update mutation carries rate)
- Delete: `web/src/features/currencies/RateDialog.tsx`
- Modify: `web/src/features/currencies/CurrenciesPage.test.tsx` (set-rate cases → edit-with-rate cases; kebab/sheet lose "Set exchange rate"), check `web/src/test/fixtures.ts` mutation handlers for set-rate remnants
- Locales: remove now-unused `classifications.currencies.modals.rate.*` keys from BOTH catalogues IF no `t()` call remains (grep first — the dialog may reuse `forms` keys for the rate field label)

- [ ] **Step 1: Rework tests first (RED)** — kebab shows Edit/Delete only; edit dialog shows a required, prefilled rate; update posts `{id, name, symbol, fractionDigits, rate}`; create still posts rate; no set-rate request path remains.
- [ ] **Step 2: Implement → GREEN**; `pnpm test`, `pnpm lint`, then `go test ./internal/test/i18ntest/` after locale removals.
- [ ] **Step 3: Commit** `feat(web): fixed rate edited in the currency dialog; RateDialog retired`.

---

### Task 7: Full verification

- [ ] **Step 1:** `make test` (Go smoke + engines + web). Expected green; coverage gate ≥ 80.
- [ ] **Step 2:** Browser check on the demo DB (rebuild SPA + binary, restart :8181): edit PTS → change rate 10 → 20; the row caption updates; open a PAST month's budget with PTS spending and confirm it converts at 20 (retroactive re-valuation visible). Revoke the session after.
- [ ] **Step 3:** Update the memory note `currency-exchange-base-rate-bug` if wording refers to dated custom rates; commit anything outstanding.
