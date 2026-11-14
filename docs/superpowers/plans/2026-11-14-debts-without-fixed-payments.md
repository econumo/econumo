# Debts Without Fixed Payments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A debt schedule "No fixed payments" (`none`): payment may be 0, no interest, balance = borrowed less payments, return-by date, plans as the only projected repayments.

**Spec:** `docs/superpowers/specs/2026-11-14-debts-without-fixed-payments-design.md`

## Global Constraints
- Schedule value `none`, debt-only (do not add it to the recurring transactions' schedule parsing).
- `payment` 0 allowed only with `none`; other schedules unchanged.
- `none`: rate 0, no dues, 0 interest splits, paymentsLeft/periodsPaid/extraPaid 0, scheduledTotal 0, no below-scheduled hint, repayable checks skipped; payoff from element plans only.
- New additive field `monthlyToFinish` (DebtResult and the element `debt` object): balance / periods from the current period through the return-by period, rounded to currency digits; `""` otherwise.
- Rate changes refused for `none` with new code `debt.no_fixed_payments` (errs.AllCodes + 11 locales).
- Go: `PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1`; goldens regenerated and inspected.

### Task 1: Backend
Files: internal/model (debt schedule parse, debt_math for `none`, DTOs), internal/debt (validation, setup, read, rate_change refusal), internal/budget (element `debt` object field), apiparity scenario (+ golden), locales (error only), docs/OpenAPI.
- [ ] Tests first: `TestDebtMath_NoFixedPayments` (balance after payments, 0 interest, no dues, payoff from plans: 3,000 borrowed, plans 1,000/month from Dec → payoff Feb; no plans → no payoff), `TestCreateDebt_NoFixedPaymentsAllowsZeroPayment` (0 refused for monthly), `TestUpdateDebt_SetUpNoFixedPayments` (borrowed + start + return by, no payments_dont_cover), `TestRead_MonthlyToFinish` (3,000 left, return by 12 months ahead → 250.00 with 12 periods incl. current), `TestAddRateChange_RefusedForNoFixedPayments`, an apiparity scenario.
- [ ] Implement; `go test ./internal/...`, `make go-lint`; commit `feat(debt): Debts without fixed payments`.

### Task 2: SPA + docs
Files: web/src/features/debts (schedule picker option, payment 0 for `none`, "Return by" label, Overview lines, Rate changes card hidden), web/src/features/budgets (row debt line "X left · return by"), api/dto types, locales (11), CLAUDE.md, docs/regression-test-plan.md.
- [ ] Tests first: picker offers "No fixed payments"; with it a 0/empty payment is accepted and sent as "0"; Settings labels "Return by"; Overview shows "3,000.00 $ left · return by Dec 2027" and "≈ 250.00 $ a month to finish on time"; no Rate changes card; the row line shows left · return by; no red scheduled hint.
- [ ] Implement; tsc, oxlint, vitest debts+budgets+lib, i18ntest; commit `feat(debts): No fixed payments in the debt screens`.
