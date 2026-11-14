# Debt Rate Changes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An optional per-debt list of rate changes (date, yearly rate, payment) that the payment splits and the forecast follow from each change's date on.

**Architecture:** A new `debts_rate_changes` table, read with the snapshots into the debt state and attached to the math snapshots (`model.WithRates`), so every existing consumer of the debt math (splits, projection, payments left, extra paid, scheduled totals, statement checks) looks up the rate and the payment in force by date without signature churn. Two new debt write routes; a "Rate changes" card on the Balance tab.

**Tech Stack:** Go (sqlc, sqlite + pgsql), React 19 + TanStack Query, vitest.

**Spec:** `docs/superpowers/specs/2026-11-13-debt-rate-changes-design.md` (read it first; it is the authority).

## Global Constraints

- Period rate = `annual_rate / 100 / periods per year`: weekly 52, biweekly 26, monthly 12, quarterly 4, yearly 1.
- Rate in force on a day = the latest change dated on or before it, else the loan's implied rate (`withLoanRate`).
- Payment in force on a day = the latest change's payment on or before it, else the ORIGIN's payment (statements' payments are never read by this rule).
- Interest of the window ending at `due` = `balance x sum over each day d in [from, due) of rate(d) / days(prevDue, due)`, where `prevDue` is the previous due and `from` = `prevDue`, or the start date for the origin's first period. A change dated on a due applies to the period after it. With no changes this must equal today's figures exactly (existing tests stay green unchanged).
- A first change dated after the start, when no row is dated on the start date, also stores a start-date row: the implied yearly rate (8 decimals) and the origin's payment, same transaction.
- New migration on both engines: `20261113000100.sql`. Rates stored `NUMERIC(19,8)`.
- Rates on the wire: decimal strings, at most 8 decimals, trailing zeros trimmed; `""` while the debt is not set up.
- New error code `debt.conflicts_with_rate_change` (registered in `errs.AllCodes`, `errors.debt.conflicts_with_rate_change` in all 11 locales).
- New metrics `DEBT_RATE_CHANGE_ADD: 'appDebtRateChangeAdd'`, `DEBT_RATE_CHANGE_DELETE: 'appDebtRateChangeDelete'`.
- Go: `PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1`. `.sql` query files ASCII only. Comments sparingly (CLAUDE.md "Comments").

## Rulings

- The spec says the two routes answer "the debt details"; the plan follows the existing statement routes (`reconcile-debt`) and answers `DebtResult`; the SPA refetches `get-debt` as it does after a statement. Cost if wrong: one extra GET.

## Review Focus

1. A change dated exactly on a due date: its rate must start with the NEXT period (Task 1 test `TestRates_ChangeOnADueAppliesToTheNextPeriod`).
2. A change in the middle of a period: that period's interest is prorated by days (Task 1 `TestRates_MidPeriodChangeIsProratedByDays`).
3. Adding a change on a date that already has one replaces it, never two rows (Task 3 `TestAddRateChange_SameDateReplaces`).
4. Deleting the stored start row while later changes exist: the start falls back to the implied rate (Task 3 `TestDeleteRateChange_StartRowFallsBackToImplied`).
5. A 0% rate is valid and splits no interest (Task 1 `TestRates_ZeroRateSplitsNoInterest`).

---

### Task 1: Rates in the debt math

**Files:**
- Modify: `internal/model/debt_math.go`
- Test: `internal/model/debt_rates_test.go` (new)

**Interfaces:**
- Produces:
  - `type MathRate struct { From time.Time; PeriodRate float64; Payment float64 }`
  - `func WithRates(snaps []MathSnapshot, rates []MathRate) []MathSnapshot`: a copy of snaps, each carrying the rates sorted by `From` (unexported field), applied by every public entry point (`SplitPayments`, `BalanceAt`, `ProjectDebt`, `PeriodRate`, `StatementRepayable`).
  - `func AnnualToPeriodRate(annualPercent float64, s RecurringSchedule) float64` and `func PeriodToAnnualRate(periodRate float64, s RecurringSchedule) float64`.
  - `func PeriodRateOn(snaps []MathSnapshot, day time.Time) float64`: the rate in force on day.
  - `func PaymentOn(snaps []MathSnapshot, day time.Time) float64`: the payment in force on day.
  - `func DueWindowContaining(s RecurringSchedule, last, day time.Time) (start, end time.Time)`: the window of the due containing day (exported wrapper over `dueContaining` + `DueWindow`).

- [ ] **Step 1: Write the failing tests** in `debt_rates_test.go`, using the helpers of `debt_math_test.go` (`ymd`, `pay`, `near`, `mustRate`). Loan for all: `MathSnapshot{AsOf: 2026-05-01, Balance: 594078.30, Payment: 2647.82, Schedule: monthly, Last: 2056-05-01, Digits: 2, Origin: true}`.
  - `TestRates_NoChangesKeepTodaysFigures`: `SplitPayments(WithRates(snaps, nil), pays)` equals `SplitPayments(snaps, pays)` for Jun..Nov payments of 2647.82 on the 1st.
  - `TestRates_ChangeOnADueAppliesToTheNextPeriod`: change `{From: 2026-09-01, PeriodRate: AnnualToPeriodRate(3.69, monthly), Payment: 2647.82}`; the Sep 1 payment's interest equals the no-change split's; the Oct 1 interest equals `balanceAfterSep x 0.0369/12` (within 1e-6).
  - `TestRates_MidPeriodChangeIsProratedByDays`: change from 2026-09-16; the Oct 1 interest = `balanceAfterSep x (implied x 15 + 0.0369/12 x 15) / 30` (September has 30 days; days 1..15 old, 16..30 new).
  - `TestRates_PaymentChangeDrivesTheProjection`: change from 2026-12-01 with payment 3000; `ProjectDebt(...).Payments[k].Payment` is 3000 for dues from Dec 1 and `PaymentsLeft` is lower than without it.
  - `TestRates_HigherRateSamePaymentMovesThePayoffLater`: change from 2026-12-01, 3.69%, payment 2647.82; `PayoffDate` after 2056-05-01.
  - `TestRates_ZeroRateSplitsNoInterest`: change from the start date, rate 0; every split is 0.
  - `TestRates_StartDateChangeReplacesTheImpliedRate`: change on 2026-05-01 at 3.5%: `PeriodRateOn(snaps, 2026-06-01)` = 0.035/12, and a start-day payment of 1175.27 splits `min(1175.27, 594078.30 x 0.035/12)`.
  - `TestRates_ExtraPaidAndScheduledUseThePaymentInForce`: with a change to 3000 from Dec 1, a Dec 1 payment of 3000 adds 0 to `ExtraPaid`, and the December `Months["2026-12-01"].ScheduledTotal` is 3000.
  - `TestRates_PeriodRateConversions`: `AnnualToPeriodRate(5.2, weekly)` = 0.001; `PeriodToAnnualRate(0.003075, monthly)` = 3.69.

- [ ] **Step 2: Run** `go test ./internal/model/ -run TestRates` — expected FAIL (undefined `WithRates`).

- [ ] **Step 3: Implement** in `debt_math.go`: the field on `MathSnapshot`, `WithRates`, the lookups, and one helper `windowInterest(s MathSnapshot, balance float64, from, prevDue, due time.Time) float64` implementing the Global Constraints formula (summing per rate segment, not per day). Replace every use of `snapshotRate(s)` for a window/step and every `s.Payment`/`latest.Payment`/`terms.Payment` used as the regular payment on a due with these lookups: `splitForward`, `splitFromOrigin` (the start-day cap uses the rate on the start date), `splitBackward`, `runProjection`, `paymentsLeftAt`, `extraPaid`, the `covered`/`ScheduledTotal` sums in `ProjectDebt`, `StatementRepayable`. `sortedSnaps` keeps the rates; `withLoanRate` stays the fallback.

- [ ] **Step 4: Run** `go test ./internal/model/` — expected PASS, the whole package (no existing test edited).

- [ ] **Step 5: Commit** `feat(debt): The debt math follows rate changes by date`.

### Task 2: Storage

**Files:**
- Create: `internal/infra/storage/migrations/{sqlite,pgsql}/20261113000100.sql`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/debts.sql`, regenerate `gen/`; `internal/model/debt.go`; `internal/debt/repository.go`; `internal/debt/repo/{repo,sqlite,pgsql}.go`
- Test: `internal/debt/repo/repo_test.go`

**Interfaces:**
- Produces:
  - `model.DebtRateChange { ID, DebtID vo.Id; EffectiveDate time.Time; AnnualRate, Payment vo.DecimalNumber; CreatedAt time.Time }`
  - Repository: `RateChanges(ctx, debtIDs []vo.Id) ([]*model.DebtRateChange, error)` ordered by debt, effective_date; `SaveRateChange(ctx, *model.DebtRateChange) error` (one row per `(debt_id, effective_date)`: an existing row on that date is replaced); `DeleteRateChange(ctx, debtID, id vo.Id) (bool, error)` (false when no such row of that debt).

- [ ] **Step 1: Write the failing test** `TestRateChanges_SaveListReplaceDelete`: save two rows for a debt (Sep 1, Dec 1), list them in date order with exact `AnnualRate` ("3.69") and `Payment`; save another on Sep 1 → still two rows, Sep 1 now the new rate; `DeleteRateChange` with another debt's id → false; with its own → true; deleting the debt cascades its rows.
- [ ] **Step 2: Run** `go test ./internal/debt/repo/ -run RateChanges` — FAIL.
- [ ] **Step 3: Implement** the migration (table per spec "Model", index on `debt_id`, unique `(debt_id, effective_date)`, FK cascade; same version both engines), the sqlc queries (ASCII only, `;` at the end of the statement line), regenerate with `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`), and the repo methods through both adapters.
- [ ] **Step 4: Run** `go test ./internal/debt/repo/ ./internal/infra/storage/...` — PASS.
- [ ] **Step 5: Commit** `feat(debt): Store debt rate changes`.

### Task 3: Service: rate changes in every read and write

**Files:**
- Create: `internal/debt/rate_change.go`, `internal/debt/rate_change_test.go`
- Modify: `internal/debt/sync.go` (`debtState.rates`, `mathSnaps` via `WithRates`), the loaders of snapshots in `sync.go`/`update.go`/`reconcile.go`, `usecase.go` (`termsFloats` callers attach rates), `read.go` (item fields, details list, `PeriodRate` = `PeriodRateOn(today)`), `reconcile.go` (a statement stores `PaymentOn(its date)`), `update.go` (`correctTerms` conflict), `clone.go`, `internal/model/debt_dto.go`, `internal/shared/errs/codes.go`, `locales/*.json` (the error only)

**Interfaces:**
- Consumes: Task 1 lookups, Task 2 repository.
- Produces:
  - `model.AddDebtRateChangeRequest {Id, Date string; AnnualRate, Payment vo.FlexString}` with `Validate()`; `model.DeleteDebtRateChangeRequest {Id, RateChangeId string}`.
  - `(*Service).AddDebtRateChange(ctx, userID vo.Id, req model.AddDebtRateChangeRequest) (*model.DebtResult, error)`, `(*Service).DeleteDebtRateChange(ctx, userID vo.Id, req model.DeleteDebtRateChangeRequest) (*model.DebtResult, error)`, both through `inOneTx` and `withSetUpDebt`.
  - `DebtResult.AnnualRate`, `DebtResult.ImpliedAnnualRate` (`json:"annualRate"`, `json:"impliedAnnualRate"`); `DebtDetailsResult.RateChanges []DebtRateChangeResult` (`json:"rateChanges"`, `{id, date, annualRate, payment}`, oldest first, `[]` when none).
  - `errs.CodeDebtConflictsWithRateChange = "debt.conflicts_with_rate_change"`.

- [ ] **Step 1: Write the failing tests** in `rate_change_test.go` (use the `newEnv`/`createOrigin` helpers; origin: 12000 from 2025-03-01, 600 monthly to 2027-03-01, today 2026-03-10):
  - `TestAddRateChange_StoresTheStartRowFirst`: add 2025-09-01 at 5% with 600 → details list two rows: 2025-03-01 with the implied rate (`== model.PeriodToAnnualRate(implied, monthly)` to 8 decimals) and payment 600, then 2025-09-01 at 5.
  - `TestAddRateChange_SameDateReplaces`: add 2025-09-01 at 5, then at 6 → two rows, the second at 6.
  - `TestAddRateChange_ResplitsFromTheWindowHoldingTheDate`: with payments on the 1st of each month, add a change on 2025-09-16; the stored split of the 2025-09-01 payment is unchanged and of 2025-10-01 recomputed (differs from before).
  - `TestAddRateChange_Refusals`: debt not set up → `debt.not_set_up`; date before start → field `date` `common.out_of_range`; rate -1 or 101 → field `annualRate` `common.out_of_range`; payment 0 → field `payment` (same code the create form uses for a non-positive payment); payment 1 at 50% → field `payment` `debt.payments_dont_cover`; a guest → 403; an archived budget → `budget.archived`.
  - `TestDeleteRateChange_StartRowFallsBackToImplied`: add 2025-09-01, delete the start row → one row left, `get-debt` periodRate before September equals the implied rate.
  - `TestDeleteRateChange_OtherDebtsRowIsNotFound`.
  - `TestUpdateDebt_StartAfterARateChangeIsRefused`: add 2025-09-01; a correction with start 2025-10-01 → field `startDate` `debt.conflicts_with_rate_change`.
  - `TestRead_AnnualRates`: no changes → `ImpliedAnnualRate == AnnualRate`; after a change dated before today at 5 → `AnnualRate == "5"`; not set up → both `""`.
  - `TestCloneDebts_CopiesRateChanges`.
- [ ] **Step 2: Run** `go test ./internal/debt/ -run 'RateChange|AnnualRates|StartAfterARate|CopiesRate'` — FAIL.
- [ ] **Step 3: Implement.** Splits are dropped from `model.DueWindowContaining(schedule, last, date).start` on, through a `dropSplitsFrom(ctx, d, day)` beside `dropSplitsAfter`. The cover check runs after the write inside the transaction: build the state (`stateNow`, splitting the dropped payments in memory if it does not already), project, refuse with `fieldError("payment", ..., errs.CodeDebtPaymentsDontCover)` when the balance is never repaid within the horizon. Register the error code in `AllCodes` and add `errors.debt.conflicts_with_rate_change` ("This start date comes after a rate change.") to all 11 locales in their register (`locale-register` memory: de Sie, nl u, fr vous, pt European).
- [ ] **Step 4: Run** `go test ./internal/debt/... ./internal/model/ ./internal/budget/... ./internal/onboarding/ ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat(debt): Rate changes in the debt service`.

### Task 4: API routes and parity

**Files:**
- Modify: `internal/debt/api/{debt.go,routes.go}`, `internal/test/apiparity/` (scenario file holding the debt scenarios + goldens), OpenAPI docs (`make swagger`)

**Interfaces:**
- Consumes: Task 3 service methods.
- Produces: `POST /api/v1/debt/add-debt-rate-change` and `POST /api/v1/debt/delete-debt-rate-change`, handlers `AddDebtRateChange`/`DeleteDebtRateChange` via `endpoint.Handle`, swag blocks like `ReconcileDebt`'s.

- [ ] **Step 1: Add apiparity scenarios**: add a change (owner), the same date again, delete it, a guest's add (403), a debt not set up (400 `debt.not_set_up`), then `get-debt` showing `rateChanges` and the item's two rate fields.
- [ ] **Step 2: Run** `go test ./internal/test/apiparity/` — FAIL (unregistered routes / missing goldens).
- [ ] **Step 3: Implement** the handlers and routes, run `make swagger`, then `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and INSPECT the diff: only the new scenarios and the additive `annualRate`/`impliedAnnualRate`/`rateChanges` fields change existing goldens.
- [ ] **Step 4: Run** `go test ./internal/test/apiparity/ ./internal/test/mcpparity/ ./internal/test/revisionguard/ ./internal/test/archtest/` and `make go-lint` — PASS.
- [ ] **Step 5: Commit** `feat(debt): Rate change routes`.

### Task 5: Balance tab "Rate changes" card

**Files:**
- Modify: `web/src/api/dto/debt.ts`, `web/src/api/debt.ts`, `web/src/features/debts/queries.ts`, `web/src/features/debts/DebtBalanceTab.tsx`, `web/src/lib/metrics.ts`, `locales/*.json`
- Test: `web/src/features/debts/DebtBalanceTab.test.tsx` (new or the existing Balance tests in `DebtReviewScreen.test.tsx`), `web/src/features/debts/testDebt.ts` fixtures

**Interfaces:**
- Consumes: the Task 4 routes and fields.
- Produces: `useAddDebtRateChange()`, `useDeleteDebtRateChange()` built on `useReconciliationWrite` with the two new metrics.

- [ ] **Step 1: Write the failing tests:**
  - with no stored rows, the card lists one row "From the start", the implied rate formatted with two decimals and "%", the payment, the caption "from your loan's terms", and no delete button;
  - the form's Rate and Payment are prefilled with the item's `annualRate` (two decimals) and the latest payment in force; Date defaults to today; Add sends `{id, date, annualRate, payment}` typed plain ("3,69" is read as 3.69 via `plainAmount`) and fires `appDebtRateChangeAdd`;
  - stored rows each have a delete button behind a confirm; confirming sends `{id, rateChangeId}` and fires `appDebtRateChangeDelete`;
  - a refusal on `annualRate`/`date`/`payment` shows under that field;
  - `readOnly` shows the table without form or delete buttons; offline, Add and delete are disabled.
- [ ] **Step 2: Run** `pnpm exec vitest run src/features/debts` — FAIL.
- [ ] **Step 3: Implement** the card under the statements history, matching the history table's styling, the form fields as the reconcile form's (`AmountField`, the date picker). Locale keys under `debts.rates.*` in all 11 catalogues, in each one's register.
- [ ] **Step 4: Run** `pnpm exec tsc --noEmit -p tsconfig.app.json`, `pnpm exec oxlint src/features/debts`, `pnpm exec vitest run src/features/debts src/lib` — PASS (the metrics coverage test included).
- [ ] **Step 5: Commit** `feat(debts): Record rate changes on the Balance tab`.

### Task 6: Docs

**Files:** `CLAUDE.md` ("Budget debts": the routes, the rate rules, the Balance tab card), `docs/regression-test-plan.md` (a "Rate changes" item: the derived first row, adding a change stores the start row, same-date replace, delete, a higher rate moving Last payment later, guest read-only).

- [ ] **Step 1:** Edit both. **Step 2:** Commit `docs: Debt rate changes`.
