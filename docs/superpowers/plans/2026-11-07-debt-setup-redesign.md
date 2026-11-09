# Debt Setup Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Shorten debt creation to name + matching + payment, add label matching with an older-debt tiebreaker, allow debts without terms ("not set up"), and replace the details sheet with a full-screen review screen.

**Architecture:** The debt model gains an optional label and optional terms (`Debt.Current` becomes `*DebtTerms`). Matching becomes "pick the oldest debt that matches" via one model helper used by the debt sync, the budget's projected reads and its drill-downs. A not-set-up debt still syncs matches (split rows with interest 0, re-split when setup finishes) so budget attribution is unchanged. The SPA gets a short create form, a shared "payments from" sentence editor, a terms form, and a full-screen review screen.

**Tech Stack:** Go (sqlc, sqlite + PostgreSQL), React 19 + TanStack Query, vitest.

**Spec:** `docs/superpowers/specs/2026-11-07-debt-setup-redesign-design.md` (builds on `docs/superpowers/specs/2026-10-29-budget-debts-design.md`; read both).

## Global Constraints

- Everything in CLAUDE.md applies (features never import features; ports + glue in `internal/server`; GET/POST only; `endpoint.Handle` + swag `@Failure` blocks; ASCII-only `.sql`; sparse comments; regenerate apiparity/mcpparity goldens with `UPDATE_GOLDEN=1` and INSPECT them).
- The migration `internal/infra/storage/migrations/{sqlite,pgsql}/20261109000000.sql` is unreleased and is EDITED IN PLACE (no new migration): `debts.as_of`, `balance`, `last_payment_date` become nullable; add `label_id TEXT NULL` (pgsql UUID NULL) with `FOREIGN KEY (label_id) REFERENCES labels (id)` (default NO ACTION, like payee/tag) and index `debts_label_idx`. `payment` and `schedule` stay NOT NULL.
- sqlc v1.30.0 (`go generate ./internal/infra/storage/sqlc/...`).
- Go: `/usr/local/go/bin/go`, `GOTOOLCHAIN=go1.27.1`. Smoke: `make go-test`; PostgreSQL: throwaway `postgres:17` container + `make test-engines` and `make test-repo-pgsql`.
- Wire additions (additive): `labelId` (string|null) and `isSetUp` (0/1) and `periodsPaid` (int) on `DebtResult`; `labelId` on create/update requests; `isSetUp` (0/1) on the budget's `DebtElementResult` and `PlanDebtElementResult`. When `isSetUp` is 0: `balance`, `lastPaymentDate`, `currentBalance`, `moneyLeft`, `overpaymentLeft`, `overpaidSoFar`, `payoffDate`, `nextDueDate`, `periodRate` are `""`; `paymentsLeft` and `periodsPaid` are 0; budget `closingBalance` `""`, `scheduledTotal` and `upcoming` `"0"`.
- New error code: `label.used_by_debt` (AllCodes + all 11 locales).
- New metric: `METRICS.DEBT_FINISH_SETUP: 'appDebtFinishSetup'`, fired when terms are saved on a debt that was not set up.
- UI copy in all 11 locales (de Sie, nl u, fr vous, pt European); `pluralPick` pipes (ru/uk/pl three forms); no bold for hierarchy; pickers via `PickerSheet` + `rankByName`.
- Branch `feature/budget-debts`; commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; do not push.

## Review Focus

1. A transaction carrying BOTH labels of two label-separated debts: it must land on the older debt only, in the debt figures, the budget attribution AND the category drill-down exclusion. Test: `TestSync_SharedLabelPaymentGoesToOlderDebt` (Task 3) and `TestGetBudget_SharedLabelPaymentOnOlderDebtRow` (Task 4).
2. Removing a label from a transaction (or adding one) after it was matched: the split moves or disappears on the next sync, and the payment returns to the category row. Test: `TestSync_LabelRemovedFromPaymentUnmatches` (Task 3).
3. Finishing setup on a debt that already collected payments: those payments get real interest splits (backward estimate) instead of the provisional 0. Test: `TestFinishSetup_ResplitsCollectedPayments` (Task 3).
4. Converting a whole category without terms, then planning months before the first payment: those plans stay visible after setup (start = earlier of first payment and creation period). Test: `TestFinishSetup_KeepsPlanningStart` (Task 3).
5. A not-set-up debt in a shared budget: the partner sees Planned/Paid and a dash for Left, never "Finish setup". Test: `debt row shows a dash for a partner when not set up` (Task 6).

---

### Task 1: Model — optional terms, label, oldest-match assignment, periods paid

**Files:**
- Modify: `internal/model/debt.go`, `internal/model/debt_math.go`, `internal/model/debt_dto.go`
- Test: `internal/model/debt_test.go`, `internal/model/debt_math_test.go`, `internal/model/debt_dto_test.go` (create if absent)

**Interfaces:**
- Produces:
  ```go
  // Debt: Current becomes optional; LabelID added.
  type Debt struct { ...; LabelID *vo.Id; Current *DebtTerms; Payment vo.DecimalNumber; Schedule RecurringSchedule; ... }
  func (d *Debt) IsSetUp() bool // Current != nil
  ```
  `Debt.Payment`/`Debt.Schedule` hold the payment terms for every debt (required); `DebtTerms` keeps `AsOf, Balance, Payment, Schedule, LastPaymentDate` as today and, when present, its Payment/Schedule equal the debt's.
  ```go
  // DebtView gains:
  LabelID   *string
  CreatedAt time.Time
  SetUp     bool
  func DebtMatches(d DebtView, categoryID, payeeID, tagID *string, labelIDs []string, at time.Time) bool
  func AssignDebt(views []DebtView, categoryID, payeeID, tagID *string, labelIDs []string, at time.Time) (DebtView, bool) // oldest match: CreatedAt asc, then ID asc
  // Projection gains:
  PeriodsPaid int // due windows (latest snapshot's schedule) up to Today holding >= 1 payment
  ```
- DTO additions: `CreateDebtRequest.LabelId *string`; `Balance` and `LastPaymentDate` become optional there (both or neither: one without the other → field error `common.is_blank` on the missing one); `UpdateDebtRequest.LabelId *string`, same optional pair; `DebtResult.LabelId *string`, `IsSetUp int`, `PeriodsPaid int`.

- [ ] **Step 1: Write failing tests:**
  - `TestDebtMatches_Label`: label set → matches only when `labelIDs` contains it; nil label → ignores labels.
  - `TestAssignDebt_OldestWins`: two views on one category, labels "car" and "student", transaction carrying both → returns the one with earlier `CreatedAt`; equal `CreatedAt` → lower `ID`.
  - `TestAssignDebt_NoMatch`: returns `false`.
  - `TestProjectDebt_PeriodsPaid`: monthly loan, payments in Jan, Feb (two payments), Apr → `PeriodsPaid == 3`.
  - `TestCreateDebtRequest_TermsPair`: balance without lastPaymentDate → field error on `lastPaymentDate`; neither → valid; payment or schedule missing → invalid.
- [ ] **Step 2: Run `go test ./internal/model/` — expect FAIL (undefined).**
- [ ] **Step 3: Implement.** Keep every existing caller compiling (adapt call sites of `DebtMatches` and `Debt.Current` across the repo mechanically; behaviour for set-up debts unchanged).
- [ ] **Step 4: Run `go test ./internal/model/ ./internal/debt/... ./internal/budget/...` and `go build ./...` — expect PASS.**
- [ ] **Step 5: Commit** `feat: Debt model takes a label and optional terms`.

### Task 2: Storage — migration, queries, repo

**Files:**
- Modify: both `20261109000000.sql`, `internal/infra/storage/sqlc/query/{sqlite,pgsql}/debts.sql`, `internal/debt/repository.go`, `internal/debt/repo/*.go`
- Test: `internal/debt/repo/repo_test.go`

**Interfaces:**
- Consumes: Task 1 `Debt`.
- Produces: `CandidateTx.LabelIDs []vo.Id` (from `transactions_labels`, loaded in one extra query per category, not per row); `Repository.UsesLabel(ctx, id vo.Id) (bool, error)`, `ReassignLabel(ctx, oldID, newID vo.Id) error`; `ListByClassification` accepts `"label"`; `Save` persists `label_id`, nullable terms, `payment`, `schedule`.

- [ ] **Step 1: Failing tests:** `TestRepo_SaveDebtWithoutTerms` (round-trip `Current == nil`, payment/schedule kept), `TestRepo_LabelRoundTripAndReassign`, `TestRepo_CandidateTransactionsCarryLabels`.
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Edit the migration in place, the queries, regenerate sqlc, implement the repo.**
- [ ] **Step 4: `go test ./internal/debt/repo/ ./internal/infra/storage/...` on sqlite and pgsql — PASS.**
- [ ] **Step 5: Commit** `feat: Debt storage for labels and debts without terms`.

### Task 3: Debt service — create/update/setup, label rules, sync

**Files:**
- Modify: `internal/debt/{create.go,update.go,usecase.go,sync.go,read.go,timeline.go,classification.go}`, `internal/label/{ports.go,usecase.go,delete.go,merge.go}`, `internal/server/glue_debt.go`, `internal/server/server.go`, `internal/shared/errs/codes.go`, `locales/*.json`, `internal/debt/api/*.go` (swag), apiparity `catalogue_debt*.go` + goldens
- Test: `internal/debt/*_test.go`, `internal/label/*_test.go`

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces (service behaviour later tasks rely on):
  - `CreateDebt` without balance/lastPaymentDate creates a not-set-up debt (no snapshot); with both, as today.
  - `UpdateDebt`: when the debt is not set up and the request carries balance + lastPaymentDate → writes the first snapshot as of today ("finish setup"); without them → updates name, matching, payment, schedule, start date only. When set up: as today (a snapshot only when terms change).
  - `Views` returns `SetUp`, `LabelID`, `CreatedAt`; not-set-up views: `FirstDay` = earlier of first matched payment and the debt's creation day (or `StartDate` when set), `LastDay` zero = open end (budget treats zero as unbounded).
  - Set-up debts with no start date: `FirstDay` = earlier of first matched payment and creation day (replaces "first snapshot").
  - `DebtResult` for a not-set-up debt: empty figures per Global Constraints, `IsSetUp 0`; set-up: `PeriodsPaid` from the projection.

Rules:
- **Key check** (`checkKeys`): two debts on one category whose (payee, tag) overlap conflict, UNLESS both have a label and the labels differ. Identical (payee, tag, label) → distinct start dates required (`debt.start_date_taken`).
- **Sync assignment:** replace "first match" with `model.AssignDebt` over the category's views; candidates carry labels. Not-set-up debts: insert split rows with interest `0` (provisional, recomputed like future-dated ones on every sync until set up); after setup, all their splits are recomputed once by the normal rules.
- **Label guard:** `internal/label` gets a `DebtUsage` port (`InUse`, `Reassign`, `LockForMerge` — mirror payee/tag); delete refused with `label.used_by_debt`; merge moves the debt (re-run key checks).

- [ ] **Step 1: Failing tests:**
  - `TestCreateDebt_WithoutTerms` (no snapshot, `IsSetUp 0`, figures empty), `TestCreateDebt_TermsPairRequired`.
  - `TestUpdateDebt_FinishSetupWritesFirstSnapshot`, `TestUpdateDebt_NotSetUpUpdatesPaymentOnly`.
  - `TestFinishSetup_ResplitsCollectedPayments` (payments collected before setup get interest > 0 after setup via the backward estimate).
  - `TestFinishSetup_KeepsPlanningStart` (no payments; created in Sep; setup in Nov → `FirstDay` stays in Sep).
  - `TestCheckKeys_LabelsTellDebtsApart` (both labelled, different → allowed; one labelled one not → `debt.key_conflict`).
  - `TestSync_SharedLabelPaymentGoesToOlderDebt`, `TestSync_LabelRemovedFromPaymentUnmatches`.
  - `TestDeleteLabel_RefusedWhileDebtUsesIt`, `TestMergeLabel_DebtFollowsTarget`.
  - apiparity: extend `debt_crud` with a create without terms → get (isSetUp 0) → finish setup → get (isSetUp 1); `debt_validation` with the terms-pair error and a label key conflict; `debt_classification_guard` with label delete/merge.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** (+ error code, locales, swag, `make swagger`).
- [ ] **Step 4: `make go-test`, inspect goldens; pgsql `make test-repo-pgsql` — PASS.**
- [ ] **Step 5: Commit** `feat: Debts without terms, label matching and finish setup`.

### Task 4: Budget — not-set-up rows and label-aware matching

**Files:**
- Modify: `internal/budget/{builder.go,builder_debts.go,projected_read.go,txlist.go}`, `internal/model/budget_dto.go`, apiparity `budget_debts` scenario + goldens, mcpparity goldens, `internal/web/mcp/prompts.go` (one glossary clause)
- Test: `internal/budget/builder_debts_test.go`, `internal/budget/debt_attribution_test.go`

**Interfaces:**
- Consumes: Task 3 views (`SetUp`, `LabelID`, `CreatedAt`, open `LastDay`).
- Produces: `IsSetUp int json:"isSetUp"` on `DebtElementResult` and `PlanDebtElementResult`.

Rules:
- Not-set-up debt rows: Planned and Paid as for any row; `closingBalance ""`, `paymentsLeft 0`, `scheduledTotal "0"`, `upcoming "0"`, payoff fields `""`; plannable from `FirstDay`'s period with no end; Timelines are not requested for them.
- `projectedRead` (templates) and `txlist`'s `debtOf` / drill-down exclusion use `model.AssignDebt` with the transaction's/template's labels, so a shared-label payment is excluded from the category once and lands on the older debt's drill-down.

- [ ] **Step 1: Failing tests:** `TestGetBudget_NotSetUpDebtRow` (Paid counted, Left empty, scheduled 0, plannable months from first payment onward, isSetUp 0), `TestGetBudget_SharedLabelPaymentOnOlderDebtRow`, `TestDrilldown_LabelledDebtExcludesOnlyItsPayments`, `TestUpcoming_TemplateWithDebtLabelDropped`.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement; regenerate goldens (only additive `isSetUp` keys + new scenario rows may change — inspect).**
- [ ] **Step 4: `make go-test`; pgsql `make test-engines` — PASS.**
- [ ] **Step 5: Commit** `feat: Budget rows for debts not set up, label-aware debt matching`.

### Task 5: SPA — data layer, sentence editor, short create form

**Files:**
- Modify: `web/src/api/dto/debt.ts`, `web/src/api/dto/budget.ts`, `web/src/api/debt.ts`, `web/src/features/debts/{queries.ts,DebtDialog.tsx,useDebtForm.ts}`, `web/src/lib/metrics.ts`, `locales/*.json`
- Create: `web/src/features/debts/PaymentsFromEditor.tsx` (the sentence of pickers, reused by Task 6), `web/src/features/debts/DebtTermsForm.tsx` (balance, payment, repeats, last payment date, start date — reused by Task 6)
- Test: `web/src/features/debts/{DebtDialog,PaymentsFromEditor,DebtTermsForm}.test.tsx`

**Interfaces:**
- Produces:
  - `PaymentsFromEditor({ value: {categoryId, payeeId, tagId, labelId}, onChange, categoryLocked?: boolean, errors })` rendering "Payments in [category] · from [any payee] · tagged [any tag] · labelled [any label]" with the explainer line.
  - `DebtTermsForm({ debt, mode: 'setup' | 'edit', onSaved })` — the existing balance/payment recalculation of the last payment date kept (anchorDay, schedule-change rule); in `setup` mode saving with balance + last payment date fires `DEBT_FINISH_SETUP` (via the update hook's `onSuccess` when the debt was `isSetUp 0`).
  - `DebtDialog` (create only): name, `PaymentsFromEditor`, payment + repeats side by side, compact currency line; a `debt.start_date_taken`/`debt.key_conflict` answer reveals a start date field; on success calls `useUiStore().openDebtDetails(id)`.
  - DTO types per Global Constraints.

- [ ] **Step 1: Failing tests:** "create form asks only name, payments from, payment and repeats"; "converting from a category locks the category and prefills the name"; "a key conflict reveals the start date field and resends with it"; "saving opens the debt's review screen"; "PaymentsFromEditor offers label and shows 'any label' when empty"; "DebtTermsForm in setup mode fires DEBT_FINISH_SETUP once".
- [ ] **Step 2: Run `cd web && pnpm exec vitest run src/features/debts` — FAIL.**
- [ ] **Step 3: Implement (+ copy in 11 locales).**
- [ ] **Step 4: `pnpm exec vitest run src/features/debts src/lib`, `pnpm lint`, `pnpm exec tsc --noEmit -p tsconfig.app.json`, `go test ./internal/test/i18ntest/` — PASS.**
- [ ] **Step 5: Commit** `feat: Short debt creation with a payments-from sentence`.

### Task 6: SPA — full-screen review screen, Settings list, budget row

**Files:**
- Create: `web/src/features/debts/DebtReviewScreen.tsx` (replaces `DebtDetailsSheet.tsx`; delete the old file and its test, port the still-valid tests)
- Modify: `web/src/app/layouts/ApplicationLayout.tsx` (mount), `web/src/features/debts/DebtsSettingsPage.tsx`, `web/src/features/budgets/{debtRow.tsx,MonthFlows.tsx,PhoneMonthView.tsx,PlanRows.tsx}`, `locales/*.json`
- Test: `web/src/features/debts/{DebtReviewScreen,DebtsSettingsPage}.test.tsx`, `web/src/features/budgets/MonthFlows.debts.test.tsx`

**Interfaces:**
- Consumes: Task 5 components; `useUiStore().debtDetailsId` / `openDebtDetails` (existing).

Layout (full-screen dialog on every breakpoint, scrollable, house rules):
- Header: name, close, menu (Mark as paid off — set up and not paid off only; Delete — existing confirm flow).
- Top, set up: `pluralPick(payments_left)` large; progress bar `periodsPaid` of `periodsPaid + paymentsLeft` ("77 of 360 paid"); "Owed now X · Paid off by Mon YYYY" (or "Paid off").
- Top, not set up: paid so far (sum of payments), **Finish setup** button opening `DebtTermsForm mode="setup"`, line "Add what you owe and when the loan ends to see payments left".
- Loan card (set up): owed now, payment + repeats, next payment, **Loan ends**, start date (or "From the first payment"), money left to pay, overpayment so far, overpayment still to come, currency; **Edit** → `DebtTermsForm mode="edit"`.
- Which payments count: `PaymentsFromEditor` editable in place; Save button appears when changed → `useUpdateDebt`.
- Payments: date, amount, interest, principal (no split columns when not set up).
- History: snapshots, collapsed by default.
- Settings list rows: payments left + owed now, or "Finish setup".
- Budget/Plan debt rows: when `isSetUp 0`, Left cell shows "Finish setup" (owner, opens the review screen) or "—" (others); Plan row header shows no payoff line.

- [ ] **Step 1: Failing tests:** "set-up debt shows payments left first with an X of Y progress"; "not-set-up debt shows Finish setup and no balance"; "Finish setup saves terms and the screen switches to payments left"; "editing payments-from saves the label"; "menu hides Mark as paid off while not set up"; "Settings list shows Finish setup for a debt without terms"; "debt row shows a dash for a partner when not set up"; "the review screen is full screen on phone and desktop" (asserts the dialog's full-screen class at both viewport widths used elsewhere in tests).
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement (+ copy in 11 locales).**
- [ ] **Step 4: `pnpm test`, `pnpm lint`, tsc, i18ntest — PASS.**
- [ ] **Step 5: Commit** `feat: Full-screen debt review with Finish setup`.

### Task 7: Docs and full verification

**Files:** `docs/regression-test-plan.md` (Debts section: short create, sentence matching incl. label, shared-label payment → older debt, not-set-up state + Finish setup, review screen top/progress/Loan card, Settings "Finish setup", partner dash), `CLAUDE.md` ("Budget debts" bullet: optional terms, label + tiebreaker, creation-day planning start, `label.used_by_debt`).

- [ ] **Step 1: Update both documents to the behaviour as built.**
- [ ] **Step 2: Full verification:** `make go-test`; throwaway pg17 `make test-engines`, `make test-repo-pgsql`, `make test-upgrade`; `cd web && pnpm test && pnpm lint && pnpm exec tsc --noEmit -p tsconfig.app.json`. All PASS.
- [ ] **Step 3: Commit** `docs: Debt setup redesign in the regression plan and CLAUDE.md`.
