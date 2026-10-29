# Budget Period Start Day Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a budget start its months on any day 1–28, shifting every budget figure to that period while period keys stay `YYYY-MM-01`.

**Architecture:** One pure period definition per stack (`model.PeriodWindow` / `model.PeriodOf` in Go, `period.ts` in the SPA). The budget service splits today's single `periodStart` into a *key* (limits, comments, response months) and a *window* (every transaction read). By-month SQL groups by day when the day is not 1 and folds days into keys in Go; a day-1 budget keeps today's SQL and bytes.

**Tech Stack:** Go (sqlc, SQLite + PostgreSQL), React 19 + TanStack Query + Zustand, vitest.

**Spec:** `docs/superpowers/specs/2026-10-28-budget-period-start-day-design.md`

## Global Constraints

- Start day range 1–28, default 1, stored per budget as `budgets.period_start_day`.
- Naming: `d = 1` key `M` = `[M-01, (M+1)-01)`; `2 <= d <= 15` key `M` = `[M-d, (M+1)-d)`; `16 <= d <= 28` key `M` = `[(M-1)-d, M-d)`. The rule lives in exactly one helper per stack.
- Keys never move: `started_at`, `ended_at`, limit and comment `period`, request dates, Plan `months`, MCP `month` stay `YYYY-MM-01`; request dates snap to the first of their calendar month (day ignored).
- `get-budget` `filters.periodStart`/`periodEnd` carry the real window; `currencyRates[]` unchanged.
- A day-1 budget must produce byte-identical responses; the only allowed golden change is the new `periodStartDay: 1` in budget meta (Task 2).
- New wire field `periodStartDay` (int) on `MetaResult`; requests: `create-budget.periodStartDay` (absent/0 → 1), `update-budget.periodStartDay` (*int, absent → untouched); clone copies.
- Error code `budget.invalid_period_start_day`, field `periodStartDay`, en text `Choose a day from 1 to 28`; translated in all 11 catalogues (respect `locale-register`: de Sie, nl u, fr vous, pt European).
- Reports, recurring schedules, account pages: untouched.
- Every new user-facing action fires a `METRICS` event; `.sql` files ASCII-only; comments sparse (CLAUDE.md rules).
- Go toolchain: `/usr/local/go/bin/go` with `GOTOOLCHAIN=go1.27.1`.

## Review Focus

1. A transaction at exactly the window start (`d = 28`, `2026-10-28 00:00:00`) belongs to "November", one at `2026-10-27 23:59:59` to "October" — Task 1 and Task 3 tests.
2. Changing the day on a budget with plans keeps every plan on its key: the same `budgeted` comes back for that month after the change — Task 2 test.
3. A partner viewing a shared budget gets the budget's periods, not calendar months — Task 3 test.
4. An ended budget with `d >= 16`: requested months clamp to the end key, the window ends at `PeriodWindow(endKey).end`, the removal window never passes it — Task 3 test.
5. Caller timezone at the boundary: UTC `2026-10-27 22:00` with `X-Timezone: Asia/Tokyo` (local Oct 28) opens "November" for `d = 28` — Task 3 test.

---

### Task 1: Go period helpers

**Files:**
- Modify: `internal/model/budget.go` (next to `FirstOfMonth`)
- Test: `internal/model/budget_period_test.go`

**Interfaces:**
- Produces:
  - `const MinPeriodStartDay, MaxPeriodStartDay = 1, 28`
  - `func ValidPeriodStartDay(d int) bool`
  - `func PeriodWindow(key time.Time, startDay int) (start, end time.Time)` — `key` is snapped with `FirstOfMonth` first; `end` exclusive; location of `key` kept.
  - `func PeriodOf(t time.Time, startDay int) time.Time` — the key (first of month, 00:00, `t`'s location).
  - `func PeriodKeyOfDay(day string, startDay int) (string, error)` — `day` is `"YYYY-MM-DD"` or `"YYYY-MM-01"`; returns the key as `"YYYY-MM-01"` (used by the repo fold in Task 4).

- [ ] **Step 1: Write the failing tests** — table test `TestPeriodWindow`:
  - `(2026-10-01, 1)` → `2026-10-01`, `2026-11-01`
  - `(2026-10-01, 5)` → `2026-10-05`, `2026-11-05`
  - `(2026-10-01, 15)` → `2026-10-15`, `2026-11-15`
  - `(2026-10-01, 16)` → `2026-09-16`, `2026-10-16`
  - `(2026-11-01, 28)` → `2026-10-28`, `2026-11-28`
  - `(2027-01-01, 28)` → `2026-12-28`, `2027-01-28`
  - `(2028-03-01, 28)` → `2028-02-28`, `2028-03-28`

  `TestPeriodOf`: `(2026-10-30 12:00, 28)` → `2026-11-01`; `(2026-10-27 23:59:59, 28)` → `2026-10-01`; `(2026-10-28 00:00:00, 28)` → `2026-11-01`; `(2026-10-04, 5)` → `2026-09-01`; `(2026-12-28, 28)` → `2027-01-01`; `(2028-02-29, 28)` → `2028-03-01`; `(2026-10-31, 1)` → `2026-10-01`.

  `TestPeriodRoundTrip`: for every `d` in 1..28 and every day 2026-01-01..2028-12-31, `k := PeriodOf(day, d)`, `s, e := PeriodWindow(k, d)`: `!day.Before(s) && day.Before(e)`, and `PeriodOf(s, d) == k`, `PeriodOf(e.Add(-time.Second), d) == k`.

  `TestPeriodKeyOfDay`: `("2026-10-28", 28)` → `"2026-11-01"`; `("2026-10-01", 1)` → `"2026-10-01"`; `("bad", 5)` → error. `TestValidPeriodStartDay`: 0, 29, -1 false; 1, 28 true.

- [ ] **Step 2: Run** `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/model/ -run 'Period' -v` — expect FAIL (undefined).
- [ ] **Step 3: Implement** the four functions. `PeriodOf`: subtract `startDay-1` days, `FirstOfMonth`, add one month when `startDay >= 16`. `PeriodWindow`: start = `key + (startDay-1)` days, minus one month when `startDay >= 16`; end = start + 1 month (safe because `startDay <= 28`). Treat `startDay` outside 1–28 as 1.
- [ ] **Step 4: Run** the same command — expect PASS.
- [ ] **Step 5: Commit** `feat: Add budget period window helpers`.

---

### Task 2: Store and edit the start day

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261028000000.sql`, `internal/infra/storage/migrations/pgsql/20261028000000.sql` (`ALTER TABLE budgets ADD COLUMN period_start_day INTEGER NOT NULL DEFAULT 1;` / `SMALLINT` on pgsql per house style)
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql` (`GetBudgetByID`, `ListBudgetsForUser`, `UpsertBudget` incl. `ON CONFLICT` set), regenerate `gen/` with `sqlc generate`
- Modify: `internal/model/budget.go` (`Budget.PeriodStartDay int`; `NewBudget` sets 1; mutator), `internal/budget/repo/repo.go` (Save + row mapping), `internal/budget/repo/adapters.go` if the pgsql shim maps fields
- Modify: `internal/model/budget_dto.go` (`MetaResult.PeriodStartDay int json:"periodStartDay"`, `CreateBudgetRequest.PeriodStartDay int json:"periodStartDay"`, `UpdateBudgetRequest.PeriodStartDay *int json:"periodStartDay"`)
- Modify: `internal/budget/builder.go` (`buildMeta`), `internal/budget/create.go`, `internal/budget/crud.go` (update), `internal/budget/clone.go`
- Modify: `internal/shared/errs/codes.go` (`CodeBudgetInvalidPeriodStartDay = "budget.invalid_period_start_day"`, add to `AllCodes`), `locales/*.json` (`errors.budget.invalid_period_start_day`, 11 files)
- Modify: `internal/budget/mcp/mcp.go` (`create_budget`/`update_budget` input `PeriodStartDay` `json:"period_start_day,omitempty"` with jsonschema `"day of the month (1-28) a budget month starts on; default 1"`, `*int` on update; `get_budget`/`get_budget_plan` descriptions note shifted months; `month`/`start_date`/`end_date` schemas say the value names the budget month and the day is ignored)
- Modify: budget api swag annotations if request structs are documented there; run `make swagger`
- No change: `data:import-sqlite` (`internal/infra/storage/sqliteimport`) copies the target's columns by introspection, so the new column travels; its existing test must stay green
- Test: `internal/model/budget_test.go`, `internal/budget/*_test.go` (create/update/clone), `internal/test/apiparity` (new scenarios + regenerated goldens), `internal/test/mcpparity` goldens

**Interfaces:**
- Consumes: `model.ValidPeriodStartDay` (Task 1).
- Produces: `model.Budget.PeriodStartDay int`; `func (b *Budget) SetPeriodStartDay(d int, now time.Time) error` (returns `errs.NewValidation("Validation failed", errs.FieldError{Key: "periodStartDay", Message: "Choose a day from 1 to 28", Code: errs.CodeBudgetInvalidPeriodStartDay})` when invalid; touches `UpdatedAt` only on change); wire `periodStartDay` on every budget meta.

- [ ] **Step 1: Write failing tests:**
  - `TestSetPeriodStartDay`: 0, 29 → field error with code `budget.invalid_period_start_day`; 28 → `PeriodStartDay == 28`.
  - Service `TestCreateBudgetPeriodStartDay`: absent → meta `PeriodStartDay == 1`; `28` → 28; `29` → field error on `periodStartDay` and NO budget row written.
  - Service `TestUpdateBudgetPeriodStartDay`: nil leaves 28 as 28; `&5` sets 5; archived budget → `budget.archived` (existing rule).
  - Service `TestUpdateBudgetPeriodStartDayKeepsPlans` (Review Focus 2): set-limit 100 on key 2026-10-01 at `d = 1`, update to 28, `get-budget` date 2026-10-01 → that element's `budgeted == "100"`.
  - Service `TestCloneBudgetCopiesPeriodStartDay`: source 28 → copy 28.
  - Repo round trip on both engines (existing repo test style): save with 16, reload → 16.
- [ ] **Step 2: Run** `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/model/ ./internal/budget/... -run 'PeriodStartDay' -v` — expect FAIL.
- [ ] **Step 3: Implement** migration, sqlc queries + `sqlc generate`, model field + mutator, repo mapping, DTO fields, create (`req.PeriodStartDay == 0` → 1, validate before any write), update (apply when non-nil, inside the existing tx), clone (copy), `buildMeta`, MCP inputs/descriptions, error code + 11 catalogue entries, swagger.
- [ ] **Step 4: Run** the step-2 command — expect PASS. Then `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/ ./internal/test/archtest/` — PASS.
- [ ] **Step 5: Add apiparity scenarios** in the budget catalogue: create with `periodStartDay: 28`, update to `5`, update with `29` (400), clone of a day-28 budget. Regenerate: `UPDATE_GOLDEN=1 GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/apiparity/ ./internal/test/mcpparity/`. Inspect `git diff --stat internal/test/*/testdata` and `git diff` samples: existing goldens may only gain `"periodStartDay": 1`; anything else is a bug.
- [ ] **Step 6: Run** `make go-test` — PASS (coverage gate ≥ 80).
- [ ] **Step 7: Commit** `feat: Store a period start day on budgets`.

---

### Task 3: Shifted periods in get-budget, transaction lists and current-period rules

**Files:**
- Modify: `internal/budget/builder.go` (`filters`, `buildFilters`, `BuildBudget`, `Filters` block), `internal/budget/read.go` (`GetBudget`, `parsePeriodDate` call order), `internal/budget/usecase.go` (`localMonth` → `currentPeriod`), `internal/budget/txlist.go`, `internal/budget/builder_structure.go` (limit lookups by key, carry-over loop by windows), `internal/budget/builder_savings.go` (monthly savings + `addSavingsPending`), `internal/budget/upcoming_merge.go` (`upcomingShown`), `internal/budget/dated_read.go` (`upcomingRead`), `internal/budget/projected_read.go` (occurrence keys + conversion windows), `internal/budget/accounts.go` (`removableAccounts`), `internal/budget/create.go`, `internal/budget/clone.go`, `internal/budget/comments.go` (`parseCommentFrom` fallback), `internal/budget/plan.go` (current-period fallback only; window work is Task 4)
- Test: `internal/budget/period_read_test.go` (service level, sqlite), `internal/budget/repo/read_period_test.go` if repo assertions are needed

**Interfaces:**
- Consumes: `model.PeriodWindow`, `model.PeriodOf` (Task 1); `Budget.PeriodStartDay` (Task 2).
- Produces:
  - `filters` gains `periodKey, keyEnd time.Time` (keys, `[periodKey, keyEnd)`) and `startDay int`; `periodStart, periodEnd` become the window `[PeriodWindow(periodKey).start, PeriodWindow(keyEnd minus one month).end)`.
  - `func (s *Service) buildFilters(ctx context.Context, userID vo.Id, b *budgetAggregate, key time.Time, months int) (filters, error)` (replaces the `(periodStart, periodEnd)` form; all callers updated, `BuildBudgetPlan` passes `months`).
  - `func currentPeriod(now time.Time, loc *time.Location, startDay int) time.Time` = `model.PeriodOf(localToday(now, loc), startDay)`; `localMonth` removed or left only for non-budget callers.
  - `func upcomingShown(ctx context.Context, periodEnd, now time.Time, startDay int) bool` — true when `periodEnd` is after the start of the caller's current period window.

Rules for the split (apply at every site): limits/comments/`StartedAt`/`EndedAt`/`clampPeriod`/`LimitsByMonth`/`SummarizedLimits`/`ListLimitsForPeriod` take **keys**; every transaction, balance, holdings and rate read takes **window** bounds; `ConvertItem.PeriodStart/End` are windows. `GetBudget` and `GetBudgetPlan` load the budget before resolving the fallback period (fallback = `currentPeriod(..., b.budget.PeriodStartDay)`). `CreateBudget` default start = current period of the requested day. `removableAccounts`: from `PeriodWindow(StartedAt).start` to the start of the current period window, capped at `PeriodWindow(endKey).end`. Projected occurrences file under `PeriodOf(o.FiledAt, d)`; `projectedMonth` takes `startDay`.

- [ ] **Step 1: Write failing tests** (budget `d = 28`, started key 2026-10-01, one expense category, everyday account; clock 2026-12-15 unless stated):
  - `TestGetBudgetShiftedWindow`: expenses 100 @ `2026-10-27 23:59:59`, 50 @ `2026-10-28 00:00:00`, 20 @ `2026-11-27 10:00`, 10 @ `2026-11-28 00:00`. `date=2026-11-01` → `filters.periodStart == "2026-10-28 00:00:00"`, `periodEnd == "2026-11-28 00:00:00"`, category spent `70`, spentBefore `100`. `date=2026-10-01` → spent `100`, `filters.periodStart == "2026-09-28 00:00:00"`.
  - `TestGetBudgetShiftedDefaultPeriod`: clock 2026-10-30, no date → `filters.periodStart == "2026-10-28 00:00:00"`.
  - `TestGetBudgetShiftedTimezone` (Review Focus 5): clock `2026-10-27 22:00 UTC`, `reqctx` location `Asia/Tokyo` → `filters.periodStart == "2026-10-28 00:00:00"`.
  - `TestGetBudgetShiftedPartner` (Review Focus 3): accepted admin partner calls `get-budget` `date=2026-11-01` → same spent `70` and window.
  - `TestBudgetTransactionListShifted`: category list for `periodStart=2026-11-01` returns exactly the 50 and 20 rows.
  - `TestGetBudgetShiftedEnded` (Review Focus 4): `d = 20`, end key 2026-11-01; `date=2027-01-01` clamps to key 2026-11-01, `filters.periodEnd == "2026-11-20 00:00:00"`; an account with a txn on `2026-11-25` stays removable (clock 2027-01-10).
  - `TestRemovableAccountsShifted`: clock 2026-10-30; txn `2026-10-28` → removable; txn `2026-10-27` → not removable.
  - `TestUpcomingShiftedOccurrences`: clock 2026-10-30, monthly template next payment `2026-11-29` → upcoming on key 2026-12-01 at `d = 28` and on key 2026-11-01 at `d = 1`; an overdue occurrence dated `2026-10-20` counts once in key 2026-11-01 at `d = 28`.
  - `TestSavingsClosingShifted`: savings member, transfer in 30 @ `2026-10-29`; `date=2026-11-01` → savings saved `30`, `closingBalance` includes it; `date=2026-10-01` → saved `0`.
- [ ] **Step 2: Run** `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/budget/ -run 'Shifted' -v` — expect FAIL.
- [ ] **Step 3: Implement** the split per the rules above, site by site in the files listed.
- [ ] **Step 4: Run** the step-2 command — PASS. Then `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/budget/... ./internal/test/apiparity/ ./internal/test/mcpparity/ ./internal/test/revisionguard/` — PASS with **no** golden changes (`git status` clean under `testdata`).
- [ ] **Step 5: Commit** `feat: Read budgets over their shifted periods`.

---

### Task 4: Shifted periods in the Plan grid and by-month reads

**Files:**
- Modify: `internal/budget/readmodel.go` (add `startDay int` as the last parameter of `SpendingByMonth`, `IncomeByMonth`, `TransfersByMonth`, `AccountsNetByMonth`, `AccountsIncomeExpenseByMonth`; `LimitsByMonth` unchanged, called with key bounds)
- Modify: `internal/budget/repo/read.go` (the five methods + `transfersByMonthSQL`), `internal/budget/dated_read.go` (`windowRead`, `unionRead` pass-throughs), `internal/budget/projected_read.go` (`projectedRead` by-month methods use `PeriodOf(…, startDay)`), any test fakes implementing `ReadModel`
- Modify: `internal/budget/builder_plan.go` (`monthsList` = keys; per-month rates, opening balances, `BulkConvert` top-level and items on windows), `internal/budget/builder_savings.go` (`addPlanSavings`, `addPlanSavingsClosings`, `buildSavingsFlows`), `internal/budget/plan.go`
- Test: `internal/budget/repo/read_plan_test.go` (both engines), `internal/budget/period_plan_test.go`

**Interfaces:**
- Consumes: `model.PeriodKeyOfDay`, `model.PeriodWindow` (Task 1); `filters.periodKey/keyEnd/startDay`, `buildFilters(…, key, months)` (Task 3).
- Produces: the five by-month `ReadModel` methods with signature `(ctx, …existing args…, from, to time.Time, startDay int)`; rows keep `Month` as the `"YYYY-MM-01"` key.

Repo rule: `startDay == 1` keeps `planMonthExpr` and today's scan unchanged (byte identity). Otherwise group by `planDayExpr(col)` (`substr(col, 1, 10)` on SQLite, `to_char(col, 'YYYY-MM-DD')` on PostgreSQL), map each row's day with `model.PeriodKeyOfDay`, and merge rows sharing a key (and the method's other grouping columns) by `vo.Decimal` addition, emitting `.String()`; keep each method's existing output order.

- [ ] **Step 1: Write failing tests:**
  - Repo `TestSpendingByMonthStartDay` (both engines via `dbtest.New`): same four expenses as Task 3; `from=2026-09-28, to=2026-11-28, startDay=28` → rows `{2026-10-01: 100}`, `{2026-11-01: 70}`; `startDay=1` over `2026-10-01..2026-12-01` → `{2026-10: 150}`, `{2026-11: 30}`.
  - Repo: one equivalent case each for `IncomeByMonth`, `TransfersByMonth` (in and out legs), `AccountsNetByMonth`, `AccountsIncomeExpenseByMonth`.
  - Service `TestGetBudgetPlanShifted`: `d = 28`, `from=2026-10-01`, `months=2` → `months == ["2026-10-01","2026-11-01"]`, the category's actual cells `100` and `70`, opening balance = balance before `2026-09-28`, a limit set on key 2026-11-01 shows as planned in column 2.
  - Service `TestGetBudgetPlanShiftedCurrency`: a foreign-currency account expense on `2026-10-29` is converted with the rates of window `[2026-10-28, 2026-11-28)` (assert the `ConvertItem` window via the convertor fake, or the converted amount with two distinct seeded rates).
- [ ] **Step 2: Run** `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/budget/... -run 'StartDay|PlanShifted' -v` — expect FAIL.
- [ ] **Step 3: Implement** the repo grouping/fold, the pass-through signatures, the projected read bucketing, and the plan builder's key/window split.
- [ ] **Step 4: Run** step-2 command — PASS. Then `make go-test` and `make test-repo-pgsql` — PASS, no golden changes.
- [ ] **Step 5: Add apiparity scenarios** `get-budget` and `get-budget-plan` on a day-28 budget with transactions on both sides of a boundary; `UPDATE_GOLDEN=1 … ./internal/test/apiparity/` and inspect: only the new scenarios' goldens are added. Run `make test` (includes enginecompare) — PASS.
- [ ] **Step 6: Commit** `feat: Bucket the plan grid by shifted periods`.

---

### Task 5: SPA period model

**Files:**
- Create: `web/src/features/budgets/period.ts`, `web/src/features/budgets/period.test.ts`, `web/src/features/budgets/PeriodDates.tsx` (+ test)
- Modify: `web/src/api/dto/budget.ts` (`BudgetMetaDto.periodStartDay?: number`, absent → 1)
- Modify every `currentMonth()` call that means the budget's current month: `BudgetPage.tsx`, `PhoneMonthView.tsx`, `ElementSheet.tsx`, `PlanSheet.tsx`, `planHint.ts` (`wantsUpcomingPanel` gains `startDay`), `queries.ts` (lines ~146, ~206), `planMath.ts` (~126, ~271, ~486), `CompleteBudgetDialog.tsx`, `DuplicateBudgetDialog.tsx`; `PeriodStrip.tsx` / `budgetMath.ts` `periodRange` current marker
- Test: the touched components' existing tests + new shifted cases

**Interfaces:**
- Consumes: wire `meta.periodStartDay` (Task 2), `filters.periodStart/periodEnd` (Task 3).
- Produces (`period.ts`):
  - `export function periodStartDayOf(meta: Pick<BudgetMetaDto, 'periodStartDay'>): number` (absent/invalid → 1)
  - `export function periodWindow(key: string, startDay: number): { start: string; end: string }` — `'YYYY-MM-DD'`, end exclusive
  - `export function periodOf(day: string, startDay: number): string` — `day` `'YYYY-MM-DD'`, returns `'YYYY-MM-01'`
  - `export function currentPeriod(startDay: number, now: Date = new Date()): string` (local date of `now`)
  - `PeriodDates` props `{ periodKey: string; startDay: number }`: renders nothing for `startDay === 1`, else a muted `"28 Oct – 27 Nov"` (inclusive last day, `Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' })`).

- [ ] **Step 1: Write failing tests** `period.test.ts`, mirroring Task 1 values: `periodWindow('2026-11-01', 28)` → `{start:'2026-10-28', end:'2026-11-28'}`; `('2026-10-01', 16)` → `{start:'2026-09-16', end:'2026-10-16'}`; `periodOf('2026-10-28', 28)` → `'2026-11-01'`; `periodOf('2026-10-27', 28)` → `'2026-10-01'`; `currentPeriod(28, new Date(2026, 9, 30))` → `'2026-11-01'`; `currentPeriod(1, new Date(2026, 9, 30))` → `'2026-10-01'`; round-trip loop for all `d` over 2026–2028. `PeriodDates.test.tsx`: `startDay 1` renders nothing; `('2026-11-01', 28)` in `en` shows `Oct 28` and `Nov 27`. `planHint` test: `wantsUpcomingPanel('2026-10-01', …, startDay 28)` with now 2026-10-30 is false (October is past).
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/budgets/period src/features/budgets/PeriodDates src/features/budgets/planHint` — expect FAIL.
- [ ] **Step 3: Implement** `period.ts`, `PeriodDates`, the DTO field, and replace each listed `currentMonth()` with `currentPeriod(periodStartDayOf(meta))`. Render `PeriodDates` next to the period strip on desktop and in the phone month header (placement is provisional; keep it one element easy to move).
- [ ] **Step 4: Run** `make web-test && make web-lint && cd web && pnpm tsc --noEmit` — PASS.
- [ ] **Step 5: Commit** `feat: Show budgets in their shifted periods`.

---

### Task 6: SPA setting, warning, analytics

**Files:**
- Modify: `web/src/features/budgets/BudgetDialog.tsx`, `BudgetUpdateDialog.tsx`, `queries.ts` (create/update mutations), `web/src/api/budget.ts` (request types), `web/src/lib/metrics.ts`, `locales/*.json` (11)
- Test: `BudgetDialog.test.tsx`, `BudgetUpdateDialog.test.tsx`, `web/src/lib/metrics-coverage.test.ts` (must stay green)

**Interfaces:**
- Consumes: `periodStartDayOf` (Task 5); wire request fields (Task 2).
- Produces: `METRICS.BUDGET_PERIOD_START_DAY_CHANGE = 'appBudgetPeriodStartDayChange'`, fired in the update mutation's `onSuccess` when the submitted day differs from the budget's stored one, data `{ periodStartDay }`; `BUDGET_CREATE` gains data `{ periodStartDay }`.
- Catalogue keys: `budgets.form.budget.period_start_day.label` (en "Month starts on day"), `budgets.form.budget.period_start_day.warning` (en "Existing plans stay on their months; the dates each month covers will change.").

- [ ] **Step 1: Write failing tests:** create dialog submits `periodStartDay: 1` by default and `28` after picking 28; update dialog pre-fills the stored day, shows the warning only once the value differs, omits `periodStartDay` from the payload when unchanged; the change metric fires once on success with `{ periodStartDay: 5 }`.
- [ ] **Step 2: Run** `cd web && pnpm vitest run src/features/budgets/BudgetDialog src/features/budgets/BudgetUpdateDialog` — expect FAIL.
- [ ] **Step 3: Implement** a day picker (1–28) in both dialogs with the existing card picker components (fuzzy search per `rankByName` if the picker filters), the warning, request wiring, metrics, and the two keys in all 11 catalogues.
- [ ] **Step 4: Run** `make web-test && make web-lint && GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat: Choose the day a budget month starts on`.

---

### Task 7: Docs, full verification, PR

**Files:**
- Modify: `docs/regression-test-plan.md` (budget section: set day on create; change day with existing plans → warning, plans stay; shifted budget figures, lists and Plan columns split at the boundary; header dates; shared budget seen by both users with the same periods; 📱 phone header dates), `CLAUDE.md` ("Notable behaviours": a "Budget period start day" bullet stating the key/window rule, the 2–15 / 16–28 naming, `filters` carrying the window, request dates naming keys, and that changing the naming rule needs a key migration)

- [ ] **Step 1:** Write both doc changes.
- [ ] **Step 2: Run** `make test` — PASS (Go smoke + pgsql repo suite + enginecompare + web).
- [ ] **Step 3:** Commit `docs: Describe budget period start days`; push `git push -u origin feature/budget-period-start-day`; open a draft PR against `main` (`Closes #279`), body ending with the Claude Code line.
- [ ] **Step 4:** Run the `codex-review` skill and address its findings.
