# Getting Started Stepper Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the two-step Day 0 page into one Getting started page with a stepper (Import, Categories, Accounts, Budget & plan, Install, Apple Wallet), move every getting-started action there, and restyle each step to the approved mockup.

**Architecture:** Step state lives in the existing `getting_started` user option (keys extended) plus a per-device key for the install step; a `useOnboardingSteps()` hook derives the visible steps, their state and the current step. `OnboardingPage` becomes a stepper shell rendering one step component at a time; the existing Day 0 writes (`useDay0`) are split into per-step actions. The budget-page cards and the Inbox "Next steps" are deleted.

**Tech Stack:** Go (one DTO change), React 19 + TanStack Query + vitest/msw, `locales/*.json` (11 languages).

**Spec:** `docs/superpowers/specs/2026-11-01-onboarding-day-0-design.md`, section "Revision 2026-11-08: Getting started as one stepper page" (it overrides the earlier flow/cards/Inbox text; "Import and the typed balance" still applies).

## Global Constraints

- Everything in the previous plan's Global Constraints still applies (`docs/superpowers/plans/2026-11-07-onboarding-day-0.md`): CLAUDE.md conventions, tests green, i18n in all 11 catalogues, METRICS fired for user actions and `metrics-coverage.test.ts` green, no top banner, mobile-first, no bold for hierarchy, fuzzy pickers via `rankByName`.
- Visual direction (approved mockup): rows grouped in ONE rounded card with soft dividers (not a bordered box per row); each row = icon tile (rounded, tinted) + name + control on the right; accent colour only for progress and the primary button; ghost secondary actions ("Skip", "I don't have one"); headings `text-2xl font-normal`, muted subtext.
- Stepper: desktop = horizontal, numbered dots joined by a line, label under each (done = filled accent circle with ✓, skipped = dashed outline, current = accent outline, upcoming = grey); phone (compact) = "Getting started · N of M" plus a thin segmented progress bar. Steps are clickable to revisit.
- Step order and keys: `import`, `categories`, `accounts`, `plan`, `home_screen` (client-only, per device), `apple_wallet`. `home_screen` visible only when `installMode()` is non-null; `apple_wallet` visible only on iPhone or Mac.
- Completion: onboarding completes (`complete-onboarding`) when every visible step is done or skipped, or on "Skip setup".
- Branch `feature/onboarding-day-0` (PR #445). Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. A user who imports first: steps 0-2 checked, budget created at step 3 from ALL their accounts (import-created ones included), no duplicate categories.
2. Reload on any step resumes on the first open step with nothing duplicated.
3. Phone right after sign-up lands on Getting started (not the nav list) with no install popup/banner on top.
4. Desktop: install and Apple Wallet steps hidden where impossible (Chrome on Linux shows neither; Mac Safari shows both).
5. Plan step: empty amounts are not written; "0" is written as a real zero; income amounts saved as plans.

---

### Task 12: Server accepts the new step keys

**Files:** Modify `internal/model/user_dto.go` (allowlist at ~:254), its test, `internal/test/apiparity/catalogue.go` (`user_write_read` call body), regenerate goldens; Modify `web/src/features/onboarding/gettingStarted.ts` types.
**Interfaces — Produces:** allowlist `plan, import, apple_wallet, categories, accounts`; TS `StoredStepKey = 'import' | 'categories' | 'accounts' | 'plan' | 'apple_wallet'`.

- [ ] Write failing DTO test cases: `"import,categories,!accounts,plan"` valid; `"categories,categories"` invalid.
- [ ] Run `go test ./internal/model/` — FAIL. Implement. Run — PASS.
- [ ] Update the apiparity body to include a new key, `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect, then `go test ./internal/...` for model/user/apiparity — PASS.
- [ ] Update the TS type; `pnpm exec tsc -b` clean. Commit `feat: getting_started accepts categories and accounts steps`.

### Task 13: Step model hook; remove cards and Inbox next steps

**Files:** Create `web/src/features/onboarding/useOnboardingSteps.ts` (+ test). Modify `web/src/lib/platform.ts` (+ test). Delete `GettingStartedCards.tsx`, `useGettingStartedActions.tsx`, `web/src/features/inbox/NextStepsSection.tsx` and their tests; remove their mounts from `BudgetPage.tsx` (~:116-120, :344-345, :685-686, :995) and `InboxPage.tsx` (:64); replace `useGettingStarted.ts` (keep `markHomeScreenDone` and the home-screen store, move them into the new hook file or a small `homeScreen.ts`). Modify `web/src/lib/metrics.ts` (remove `GETTING_STARTED_OPEN`/`GETTING_STARTED_DISMISS`, add `ONBOARDING_STEP_DONE: 'appOnboardingStepDone'` `{step}` and `ONBOARDING_STEP_SKIP: 'appOnboardingStepSkip'` `{step}`), locales (delete unused `onboarding.cards.*`, `inbox` next-steps keys).
**Interfaces — Produces:**
- `isMac(): boolean` in platform.ts (Macintosh UA and not `isIOS()`).
- `type OnboardingStepKey = 'import' | 'categories' | 'accounts' | 'plan' | 'home_screen' | 'apple_wallet'`; `type OnboardingStepState = 'open' | 'done' | 'skipped'`.
- `useOnboardingSteps(): { steps: { key: OnboardingStepKey; state: OnboardingStepState }[]; current: OnboardingStepKey | null; allResolved: boolean; markDone(key): Promise<void>; skip(key): Promise<void>; ready: boolean }` — visible steps in the order above; `current` = first `open` step; `done` from the option keys (plain = done, `!` = skipped) plus: `apple_wallet` done when an Apple Wallet import source exists, `home_screen` from the per-device store / `isStandalone()`; `markDone`/`skip` write the option via the existing reconciled `writeStepState` (home_screen via the local store) and fire the step metrics.
- `markGettingStartedDone(queryClient, key)` keeps working for outside call sites (set-limit hooks, ImportCsvDialog) with the new key type.
**Steps:**
- [ ] Failing tests: visible steps on desktop Linux Chrome = 4 (no home_screen, no apple_wallet); Mac Safari = 6; iPhone Safari = 6; installed (standalone) iPhone = 5; option `"import,categories,accounts"` → current `plan`; `"!import"` → import skipped, current `categories`; all resolved → `allResolved` true; `isMac` true for Mac Safari UA, false for iPad UA.
- [ ] FAIL → implement → PASS. Delete the card/Inbox code and its tests; BudgetPage and InboxPage tests updated (no cards, no Next steps). `pnpm test src/features/onboarding src/features/budgets src/features/inbox src/lib`, `pnpm lint`, `pnpm exec tsc -b`, `go test ./internal/test/i18ntest/` — PASS.
- [ ] Commit `refactor: one getting-started step model; drop budget cards and inbox next steps`.

### Task 14: Stepper shell, phone landing, install prompts hidden

**Files:** Create `web/src/features/onboarding/Stepper.tsx` (+ test). Rewrite `OnboardingPage.tsx` (+ test). Modify `web/src/app/layouts/ApplicationLayout.tsx` (compact `/` logic ~:114-115; TopBanner install banner and `<InstallPopup/>` gating ~:188-189). Locales `onboarding.steps_nav.*` (labels: Import, Categories, Accounts, Budget & plan, Install app, Apple Wallet; "optional" tag on Import; "Getting started · {current} of {total}"; "Skip setup").
**Interfaces:** Consumes Task 13 hook. Produces `Stepper({ steps, current, onSelect })` and a page that renders a placeholder step body per key (later tasks fill them) via a `STEP_BODIES: Record<OnboardingStepKey, ComponentType<StepBodyProps>>` map with `StepBodyProps = { onDone(): Promise<void>; onSkip(): Promise<void> }`.
**Behaviour:** the page shows the selected step (default `current`); clicking a stepper item selects it; `onDone`/`onSkip` call the hook then advance to the next open step; when `allResolved` → `completeOnboarding()` (existing API) then navigate to `RouterPage.BUDGET`; "Skip setup" marks every open step skipped then completes. On compact with onboarding incomplete, `/` renders the workspace (HomePage → onboarding), not the sidebar-only view. While onboarding is incomplete, neither the install banner nor the install popup renders.
- [ ] Failing tests: stepper states render (done ✓, skipped dashed, current outlined) and selecting works; desktop vs compact variants; resolving the last step calls complete-onboarding and lands on `/budget`; Skip setup resolves all; compact `/` for an incomplete user shows the onboarding heading; install banner/popup absent while incomplete, present after completion (existing install tests adjusted).
- [ ] FAIL → implement → PASS (onboarding, app/layouts, install folders), lint, tsc. Commit `feat: getting started stepper page`.

### Task 15: Import, Install and Apple Wallet step bodies

**Files:** Create `ImportStep.tsx`, `InstallStep.tsx`, `AppleWalletStep.tsx` in `web/src/features/onboarding/` (+ tests); register them in `STEP_BODIES`. Locales `onboarding.import_step.*`, `onboarding.install_step.*`, `onboarding.wallet_step.*` (copy from the mockup: "Bring in your history" / "Have a CSV export from your bank or another app? It creates accounts, categories and transactions in one go." / "Choose a CSV file" / "I don't have one"; "Keep Econumo one tap away" / "Add it to your home screen so logging a coffee takes seconds."; "Card payments, logged for you" / "Connect Apple Wallet and every tap with your card appears in Econumo on its own.").
**Behaviour:**
- Import: primary "Choose a CSV file" opens the existing `ImportCsvDialog`; on `onComplete(result)` with `result.imported > 0` → mark `import`, `categories`, `accounts` done (in one option write) and advance; "I don't have one" = skip import.
- Install: shows the device's instructions inline for instruction modes (reuse the steps from `InstallInstructionsDialog`'s content — extract a shared `InstallSteps` component instead of duplicating), or an "Install" button for `prompt` mode calling `promptInstall()`; "Done" marks `home_screen` done; Skip skips.
- Apple Wallet: "Set up" navigates to `RouterPage.SETTINGS_APPLE_WALLET` (the step turns done when the source exists); Skip skips.
- [ ] Failing tests per body (import success marks three steps done in one request; zero imported marks nothing; install prompt mode calls promptInstall; instruction mode renders the shared steps; wallet navigates). FAIL → implement → PASS, lint, tsc. Commit `feat: import, install and Apple Wallet steps`.

### Task 16: Categories and Accounts steps in the new style

**Files:** Modify `CategoriesStep.tsx`, `AccountsStep.tsx`, `CurrenciesBlock.tsx`, `useDay0.ts`, `draft.ts` (+ tests); register in `STEP_BODIES` via thin adapters.
**Interfaces:** `useDay0()` becomes `{ ready, pending, error, saveCategories(rows: CategoryRow[]): Promise<void>, saveAccounts(rows, extraCurrencyIds, mainCurrencyId): Promise<void>, ensureBudget(): Promise<Id>, ... }` — `finish`/`skip` removed (the shell completes onboarding). Keep the incremental draft/op-id/reconcile behaviour; `ensureBudget` creates the budget once (id persisted in the draft) with `accountIds` = all of the user's own non-deleted accounts (import-created included) and `savingsAccountIds` from the draft rows, and returns the budget id (or the existing default budget id when one already exists).
**Behaviour:**
- Categories: grouped card per type, row = icon tile + inline name + checkbox on the right; the user's existing categories (import, revisit) listed first, ticked and locked; defaults whose name already exists are omitted; on a fresh account all defaults ticked; "add your own" row at the end of each group. Continue → `saveCategories` → `onDone`.
- Accounts: one grouped card, compact row = icon tile + name (savings shown as a small accent label under the name, toggled by a switch in an expandable row detail or a trailing control — keep it one line on phones) + balance + currency chip; existing server accounts listed locked (no balance prompt); "Add account" row; Currencies block restyled as a compact line ("USD (main) · + Add", tap main to change) plus the no-rate note. Continue → `saveAccounts` → `onDone`.
- [ ] Update/extend the existing step and useDay0 tests for the new layout, existing-items-locked behaviour, defaults omitted when present, and `ensureBudget` (all owned accounts, idempotent). FAIL → implement → PASS, lint, tsc. Commit `feat: categories and accounts steps in the getting started style`.

### Task 17: Budget & plan step

**Files:** Create `PlanStep.tsx` (+ test); register in `STEP_BODIES`. Locales `onboarding.plan_step.*` ("Plan this month", "Your budget is ready. Put a rough amount next to what matters — you can refine it any time.", group titles "Income · {month}" / "Expenses · {month}", "Planned {planned} of {total}", "Save plan", "Skip for now").
**Interfaces:** Consumes `useDay0().ensureBudget()`, `setLimit` (web/src/api/budget.ts:129, `{budgetId, elementId: categoryId, period: 'YYYY-MM-01', amount}`), `useBudget()`/`getBudget` for the structure, `useExchange` + accounts for the total.
**Behaviour:** on mount call `ensureBudget()`, load the budget for the current month, list income-category elements (Income group) and expense-category elements (Expenses group) with one amount input each, prefilled from existing `budgeted` values. "Planned X of Y": X = sum of expense amounts; Y = sum of income amounts when any, else the total of the user's accounts converted to the budget currency. "Save plan" writes `setLimit` only for changed, non-empty inputs (`"0"` is a real zero, blank never written), sequentially, then `onDone` (plan is also marked by the set-limit path). Amount input uses the same locale-aware parser as balances (`parseBalance`). Skip for now → `onSkip`.
- [ ] Failing tests: budget created once on mount (msw); groups and prefill; blank not written, "0" written; planned line uses income when present, accounts total otherwise; save posts set-limit per changed row with period first-of-month then advances. FAIL → implement → PASS, lint, tsc. Commit `feat: budget and plan step`.

### Task 18: Docs, full verification, screenshots

**Files:** `docs/regression-test-plan.md` §3 (rewrite for the stepper: steps per device, import path auto-checks, resume, phone landing, install prompts hidden, plan step), remove card/Inbox next-steps items; `CLAUDE.md` (frontend Day 0 paragraph: stepper, step keys, no budget cards/Inbox next steps); spec unchanged.
- [ ] Update docs. Run `make go-test`, `cd web && pnpm test && pnpm lint && pnpm build` — PASS (re-run known load-flaky files alone if they time out). Commit `docs: getting started stepper`.
