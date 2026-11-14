# Debts on Elements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A debt is attached to a top-level expense category or envelope of a budget; the element keeps its plans and spending and shows the debt's figures; debt rows, rules and plan moves go away.

**Architecture:** The `debts` row gains its element (external id + type), status and end; `debts_rules` is dropped. The debt service matches payments by the element's categories with the budget's own spending attribution (untagged expenses, everyday member accounts), reads plans from the element's limits, and ends a debt with mark-paid-off (and reopens it). The budget feature stops emitting debt rows and stops moving matched payments off their rows; instead every category/envelope row with a debt in force in the period carries an additive `debt` object. The SPA folds the debt UI into element rows and menus.

**Tech Stack:** Go (sqlc, SQLite + PostgreSQL), React 19 + TanStack Query, vitest.

**Spec:** `docs/superpowers/specs/2026-11-13-debts-on-elements-design.md` (approved; the authority). File map with line numbers: `docs/superpowers/plans/2026-11-13-debts-on-elements-filemap.md`.

**Starts after** `docs/superpowers/plans/2026-11-13-debt-rate-changes.md` is complete (same files). Rate changes, statements, snapshots, splits, the projection and the review screen's Overview and Balance tabs carry over unchanged.

## Global Constraints

- Migration on both engines, one version above every migration on the branch (above `20261113000100`): deletes all debts (snapshots, splits, rate changes cascade) and every `budgets_elements` row of type 6 with its limits and comments; reshapes `debts` (adds `element_external_id`, `element_type`, `status`, `ended_at`; drops `name`, `icon`; keeps `currency_id`); drops `debts_rules`.
- `element_type`: `category` | `envelope`. `status`: `active` | `paid_off`. At most one `active` debt per (budget, element).
- Eligible element: a top-level expense category (not inside an envelope) or an envelope of the budget. Refusal: `debt.element_not_eligible` (field `elementId`).
- Payments of a debt = exactly what the element's Spent counts: expenses (never transfers), `tag_id IS NULL`, on everyday (non-savings) member accounts of the budget (soft-deleted members included), in the element's categories (an envelope's current children), dated on/after the start date (any date while not set up) and on/before `ended_at` once paid off.
- A planned extra = the element's limit (plan) in a period above the scheduled payment; `ProjectionInput.Plans` is filled from the element's limits.
- The debt's currency = the element row's currency, copied at creation; `change-element-currency` moves the element's ACTIVE debt (amounts unconverted, splits dropped); a paid-off debt keeps its currency.
- A new debt on an element with history must start after the latest paid-off debt's `ended_at`: `debt.start_before_previous_end` (field `startDate`); a second active debt: `debt.element_has_debt` (field `elementId`).
- `mark-debt-paid-off {id, date?}`: date default today, not before the start, not after today (`common.out_of_range` on `date`); stores a 0 statement as of that date (replacing one that day), `status=paid_off`, `ended_at=date`. Allowed on a debt that still owes.
- `reopen-debt {id}`: removes `ended_at`, the status and the 0 statement dated `ended_at`; refused with `debt.cannot_reopen` when the element has an active debt or a debt started after it. `delete-debt-reconciliation` refuses a paid-off debt's closing statement with `debt.paid_off`; a paid-off debt is otherwise read-only (`debt.paid_off` on every write except delete and reopen).
- Moving a category carrying an ACTIVE debt into an envelope: `budget.element_has_debt`.
- Deleting a category or envelope deletes its debts (all history). A category merge moves the source's debts to the target only when the target has no debts at all, is eligible and its row has the same currency; otherwise the source's debts are deleted.
- New coded errors (registered in `errs.AllCodes`, `errors.*` in all 11 locales, each catalogue's register: de Sie, nl u, fr vous, pt European): `debt.element_not_eligible`, `debt.element_has_debt`, `budget.element_has_debt`, `debt.start_before_previous_end`, `debt.cannot_reopen`, `debt.paid_off`. Removed codes (with their locale keys): `debt.key_conflict`, `debt.start_date_taken`, `debt.category_not_expense`, `budget.debt_folder_not_allowed`, `budget.debt_period_out_of_range`, `budget.debt_currency_fixed` (keep any still used elsewhere).
- Metrics: `DEBT_REOPEN: 'appDebtReopen'` added; `DEBT_RULE_ADD` removed; `DEBT_MARK_PAID_OFF` keeps firing.
- Go: `PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1`. `.sql` ASCII only; `;` never alone on a line. Comments sparingly. apiparity/mcpparity goldens: regenerate with `UPDATE_GOLDEN=1`, INSPECT, never hand-edit.

## Rulings

- The element is stored by its external id (category or envelope id), not the `budgets_elements` row id: the row can be re-created by structure sync (a participant leaving and returning), the external id survives.
- Envelope delete is in the budget feature: it calls the debt service through a new `DebtSource` method; category delete/merge keep reaching the debt service through the category feature's `DebtUsage` port, reshaped. Payee, tag and label lose their debt hooks entirely.
- Intermediate state: after Task 1 the budget still renders debt rows (fed by the reshaped views); Task 2 removes them. Each task's commit compiles and passes its own suites.

## Review Focus

1. A tagged payment in the element's category: not a debt payment, and the element's Spent and the debt agree (Task 1 test `TestSync_TaggedExpenseIsNotAPayment`).
2. Viewing a past period after a debt was paid off: the element shows that debt's figures; a period after its end shows none (Task 2 `TestGetBudget_DebtInForceByPeriod`).
3. Reopen after a later debt exists on the element: refused (Task 1 `TestReopenDebt_RefusedWhenALaterDebtExists`).
4. A category with an active debt dragged into an envelope: refused with `budget.element_has_debt`; dragging one with only paid-off history: allowed (Task 2 `TestMoveIntoEnvelope_DebtRules`).
5. Changing the element's currency moves the active debt but not a paid-off one (Task 2 `TestChangeElementCurrency_MovesOnlyTheActiveDebt`).

---

### Task 1: Backend core — debts on elements

**Files:** see filemap §1, §2, §3, §5, §6 (migration, sqlc/repo, `internal/model/debt*.go`, `internal/shared/errs/codes.go`, `internal/debt/**`, `internal/server/glue_debt*.go`, `glue_onboarding.go`, `server.go`, `internal/{category,payee,tag,label}` debt hooks, `internal/onboarding/debts.go`, `ports.go`), locales (new/removed `errors.*` only), `internal/test/apiparity/catalogue_debt.go` (+ goldens), `internal/budget` only as needed to compile (its `DebtSource` consumers, `DebtRowPlans` -> element plans).

**Interfaces:**
- Produces:
  - `model.Debt` gains `ElementID vo.Id`, `ElementType model.DebtElementType` (`DebtElementCategory`, `DebtElementEnvelope`), `Status model.DebtStatus` (`DebtActive`, `DebtPaidOff`), `EndedAt *time.Time`; loses `Name`, `Icon`, `Rules`.
  - `model.DebtView` keeps `ID, BudgetID, CurrencyID, CreatedAt, SetUp, From` and gains `ElementID, ElementType, Until *time.Time` (from `ended_at`); name/icon come from the element (`Name, Icon` filled by the debt service from the element lookup).
  - `model.CreateDebtRequest{BudgetId, ElementId string; Payment vo.FlexString; Schedule string; StartDate *string}`; `model.MarkDebtPaidOffRequest{Id string; Date string}` (`""` = today); `model.ReopenDebtRequest{Id string}`.
  - `DebtResult`: drops `rules` and its own `icon` semantics; adds `elementId`, `elementType`, `status` (`active`|`paid_off`), `endedAt` (`""` while active); `name`, `icon`, `currencyId` are the element's.
  - Debt service port `debt.Elements` (wired in `glue_budget_debt.go` over the budget feature): `Element(ctx, budgetID, elementID vo.Id) (model.DebtElementInfo, error)` with `DebtElementInfo{ID vo.Id; Type DebtElementType; Name, Icon string; CurrencyID vo.Id; CategoryIDs []vo.Id; Eligible bool}`, and `Plans(ctx, budgetID, elementID vo.Id) (map[string]float64, error)` (period key -> planned, element currency).
  - `(*debt.Service).ReopenDebt(ctx, userID, model.ReopenDebtRequest) (*model.DebtResult, error)`; `MarkDebtPaidOff` takes the date; `DeleteElementDebts(ctx, budgetID, elementID vo.Id) error` and `MoveElementDebts(ctx, budgetID, fromID, toID vo.Id) error` (merge rule above) for the budget and category features.
  - Routes: `POST /api/v1/debt/reopen-debt`; `get-debt-rule-options` and `add-debt-rule` removed.

- [ ] **Step 1: Write failing tests** (`internal/debt/*_test.go`, rewrite the rule-era tests; keep the math/snapshot/statement/rate-change tests):
  - `TestCreateDebt_OnACategoryAndAnEnvelope`: debt on a top-level category; on an envelope; result name/icon/currency = the element's.
  - `TestCreateDebt_Refusals`: category inside an envelope / income / tag -> `debt.element_not_eligible`; second active debt -> `debt.element_has_debt`; start before the previous paid-off debt's end -> `debt.start_before_previous_end`; guest -> 403.
  - `TestSync_MatchesTheElementsSpending`: expenses in the category on everyday accounts count; a transfer, a savings-member expense and `TestSync_TaggedExpenseIsNotAPayment` do not; an envelope debt counts every child category.
  - `TestMarkDebtPaidOff_EndsTheDebt`: date 2026-02-01: status paid_off, `ended_at`, 0 statement that day, a payment after the end stops matching on the next sync.
  - `TestReopenDebt`: restores active and removes the closing statement; `TestReopenDebt_RefusedWhenALaterDebtExists`; refused when another active debt exists.
  - `TestPaidOffDebtIsReadOnly`: update/reconcile/rate change -> `debt.paid_off`; delete allowed; `delete-debt-reconciliation` of the closing statement -> `debt.paid_off`.
  - `TestProjection_ExtraFromTheElementsPlan`: element plan 3,000 against payment 2,647.82 projects an extra of 352.18 in that period.
  - `TestClassification_DeleteAndMerge`: deleting the category deletes its debts; merge into a target with no debts moves them; into a target with any debt deletes them.
  - `TestCloneDebts_KeepsElements`; onboarding `TestDetectDebts_AttachToTheCategory`.
- [ ] **Step 2: Run** `go test ./internal/debt/...` — FAIL.
- [ ] **Step 3: Implement** per filemap §1–§3, §5, §6: migration + migration test (old shape removed), sqlc regen, repo (candidates by category ids + everyday account ids + `tag_id IS NULL` + window, list by element, status/ended_at writes), model reshape (delete the rule machinery listed in filemap §2), service (create/update/mark-paid-off/reopen/delete/read/list/sync/forecast/timeline/clone/classification), glue + ports, onboarding (attach to the category, drop `CoveredCategories`/`TakeCategoryPlans`; skip a category that already has an active debt), category `DebtUsage` reshaped to delete/move, payee/tag/label hooks removed, routes + swag (`make swagger`), apiparity debt scenarios rewritten (create on category and envelope, mark paid off with date, reopen, refusals) + goldens.
- [ ] **Step 4: Run** `go test ./internal/... ` except `./internal/test/mcpparity/` and `make go-lint` — PASS (budget suites may still show debt rows; budget goldens may move only for removed rule/name/icon fields — inspect).
- [ ] **Step 5: Commit** `feat(debt): Debts attach to budget elements`.

### Task 2: Budget integration — no debt rows, the element carries the debt

**Files:** filemap §4, §7; `internal/model/budget_dto.go`, `budget_view.go`, `budget_valueobject.go`; apiparity budget goldens, mcpparity goldens.

**Interfaces:**
- Consumes: Task 1's `DebtView` (element, `From`/`Until`), `Timelines`, `DeleteElementDebts`, `MoveElementDebts`, the currency hook.
- Produces:
  - `model.ElementDebtResult{Id string; IsSetUp int; PaymentsLeft int; PayoffDate, BasePayoffDate, ClosingBalance, ScheduledTotal, Upcoming string; IsPaidOff int; OverpaymentLeft, BaseOverpaymentLeft string}` as `debt` (omitempty) on `get-budget` category/envelope element results and on `get-budget-plan` rows, with per-cell `debt` (same figures per period + `id`) on plan cells.
  - `structure.debts`, plan `debts`, `ElementDebt` handling, `debtId` on `get-transaction-list` removed.
  - `DebtSource.ElementDebtsInForce`-style lookup: for a period, element external id -> the debt in force (active or paid-off covering the period).
- [ ] **Step 1: Write failing tests** (`internal/budget/*_test.go`, rewrite the debt-row tests):
  - `TestGetBudget_PaymentsStayOnTheirElement`: a matched payment counts in its category's Spent, its envelope's and folder's totals.
  - `TestGetBudget_DebtObjectOnTheElement`: the category row carries `debt` with payments left and payoff; an envelope debt on the envelope row.
  - `TestGetBudget_DebtInForceByPeriod`: paid off 2026-02-01: January shows it, March shows none; a newer debt from April shows from April.
  - `TestGetBudgetPlan_CellsCarryTheDebtInForce`.
  - `TestMoveIntoEnvelope_DebtRules`; `TestChangeElementCurrency_MovesOnlyTheActiveDebt`; `TestDeleteEnvelope_DeletesItsDebts`; `TestCloneBudget_CopiesDebtsOnElements`.
- [ ] **Step 2: Run** `go test ./internal/budget/...` — FAIL.
- [ ] **Step 3: Implement** per filemap §4 (remove attribution, debt rows, debt-period checks, folder refusal, debt sort group, plan-move uses; add the `debt` object and the per-cell debt; envelope refusal; currency hook; envelope delete; clone), §7 (MCP descriptions); regenerate apiparity and mcpparity goldens and INSPECT: debt rows/`structure.debts` disappear, `debt` objects appear, category/envelope Spent grows by the formerly attributed payments.
- [ ] **Step 4: Run** `go test ./internal/...` and `make go-lint` — PASS.
- [ ] **Step 5: Commit** `feat(budget): Elements carry their debts; no debt rows`.

### Task 3: SPA debt feature — create, review, paid off, reopen

**Files:** filemap §8 `features/debts/*`, `api/debt.ts`, `api/dto/debt.ts`, `app/queryKeys.ts`, `app/uiStore.ts`, `lib/metrics.ts`, `features/classifications/queries.ts`, `features/iconSuggestion/inline.ts`, locales `debts.*`.

**Interfaces:**
- Consumes: Task 1/2 wire.
- Produces: `OpenDebtParams{budgetId, elementId}`; `useCreateDebt` (`{budgetId, elementId, payment, schedule}`), `useMarkDebtPaidOff({id, date})`, `useReopenDebt({id})` (fires `appDebtReopen`); `RulesEditor`, `PaymentsFromEditor`, `AddToDebtDialog`, `rules.ts`, `useDebtRuleOptions`, `useAddDebtRule` deleted.
- [ ] **Step 1: Failing tests** (`features/debts/*.test.tsx`): the create form shows the element's icon and name in its header and asks only payment and schedule, sending `{budgetId, elementId, payment, schedule}`; the review screen header is the element's icon and name; Settings has no name, icon, rules or currency; Danger zone: Mark as paid off with a date (default today) sends `{id, date}`; a paid-off debt opens read-only with a Reopen action (sends `{id}`) and Delete; `debt.cannot_reopen` shows its message.
- [ ] **Step 2: Run** `pnpm exec vitest run src/features/debts` — FAIL.
- [ ] **Step 3: Implement**; delete the rule components and their tests; locale keys added/removed in all 11 catalogues.
- [ ] **Step 4: Run** tsc, oxlint, `pnpm exec vitest run src/features/debts src/lib`, `go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat(debts): Debts are created and closed on their element`.

### Task 4: SPA budget views — the element shows its debt

**Files:** filemap §8 `features/budgets/*` (MonthFlows, PhoneMonthView, BudgetPage, PlanSheet, PlanRows, ElementSheet, phoneMonth.ts, planCell.ts, planMath.ts, budgetMath.ts, budgetTransactionsParams.ts, planHint.ts, debtRow.tsx, useBudgetLineMenus.tsx, api/dto/budget.ts, api/budget.ts), `features/pendingWrites/{limitPatches,overlay,sender}.ts`, locales `budgets.*`.

**Interfaces:**
- Consumes: the `debt` object on elements/plan rows/cells (Task 2), Task 3 hooks and `OpenDebtParams`.
- [ ] **Step 1: Failing tests:** no Debts block in Budget (desktop and phone) or Plan; a category row with `debt` shows its normal columns plus a quiet debt line (payments left · payoff month, "Finish setup" when not set up for an editor); the item sheet has a Debt section with Details; the row menu of a top-level expense category or envelope offers "Track as debt" (no debt in force), else "Debt details", "Mark as paid off", "Delete", and "Past debts (N)" listing the history (opens read-only); a category inside an envelope, income, tag or savings rows offer none; the red plan hint on a debt-carrying row is "plan below the scheduled amount"; Plan cells show the debt hint per period; the expense totals no longer add debt rows (`debtTotals` gone); offline, the debt menu items are disabled.
- [ ] **Step 2: Run** `pnpm exec vitest run src/features/budgets src/features/pendingWrites` — FAIL.
- [ ] **Step 3: Implement**; remove `structure.debts`/plan debt rows handling, `debtId` drill-down, pending-writes debt-row patches; layout within the existing row styles (no new top banner, no bold for hierarchy).
- [ ] **Step 4: Run** tsc, oxlint, `pnpm exec vitest run` (whole web suite once), `go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat(budgets): Budget and Plan rows show their debt`.

### Task 5: Docs and cleanup

**Files:** `CLAUDE.md` ("Budget debts", the `debt` feature intro, onboarding debt detection, MCP notes, error codes), `docs/regression-test-plan.md` (debt items rewritten: track on a category and an envelope, element keeps plans, debt line on the row, mark paid off with a date, reopen, past debts, envelope refusal, currency change, delete), the three earlier debt specs get a "Superseded by 2026-11-13-debts-on-elements-design.md" pointer, unused locale keys removed from all 11 catalogues.

- [ ] **Step 1:** Edit. **Step 2:** `go test ./internal/test/i18ntest/` — PASS. **Step 3:** Commit `docs: Debts on elements`.
