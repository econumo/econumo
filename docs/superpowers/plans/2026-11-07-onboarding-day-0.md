# Onboarding Day 0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the six-step onboarding page with a two-step Day 0 flow (accounts as of today + currencies, then categories), an automatic budget, platform-aware "get ready" cards and Inbox next steps, and a "Keep current balance" option on CSV import.

**Architecture:** The flow is SPA-only and composes existing endpoints (create-account, update-currency, hide/show-currency, create-category, create-budget, complete-onboarding) with client-generated operation ids persisted in a localStorage draft, so a retry or reload never duplicates. Two backend additions: a `getting_started` user option with its write endpoint, and an account-feature snapshot/restore pair that keeps an account's balance across a chunked CSV import. One SQL migration completes onboarding for existing users who already have accounts.

**Tech Stack:** Go (stdlib HTTP, sqlc, SQLite + PostgreSQL), React 19 + Vite + TanStack Query + vitest/msw, shared `locales/*.json` (11 languages).

**Spec:** `docs/superpowers/specs/2026-11-01-onboarding-day-0-design.md`

## Global Constraints

- Follow CLAUDE.md: vertical feature packages, features never import features, engine-adapter sqlc pattern, `.sql` files ASCII-only, comments sparse.
- `make go-test` and `cd web && pnpm test && pnpm lint` pass at every commit that touches them.
- New routes get apiparity scenarios; regenerate goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and inspect the diff; never hand-edit a golden.
- Every transaction write bumps `transactionsRevision` through `internal/infra/txrevision` (guarded by `internal/test/revisionguard`).
- SQLite and PostgreSQL must give identical results; new queries exist in both `query/sqlite` and `query/pgsql`, regenerated with `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`).
- Every new user action fires a `METRICS` key via `trackEvent` (`web/src/lib/metrics.ts`); `metrics-coverage.test.ts` must stay green.
- Every new string goes into all 11 `locales/<lang>.json` with real translations; `internal/test/i18ntest` must stay green.
- No new top banner. Attention items go to the Inbox.
- No AI anywhere in this feature. All lists are static. No payees are created.
- SimpleFIN is out of scope: no card, no checkbox, no change.
- Apple Wallet import is always available in the SPA today (its flag was removed on main); show its card on iPhone only.
- Branch: `feature/onboarding-day-0`. Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. Reload, double-click or a network failure mid-flow: no duplicate accounts, categories or budgets (stable ids in the draft; server operation guards dedupe). Owner: Task 5.
2. Balance input in local formats ("1 234,50", "-80", "", "abc"): blank means 0, unparseable blocks Continue with an inline error, never a silent 0. Owner: Task 6.
3. A guessed or chosen currency the server cannot hide/show (base currency, profile currency): the flow continues; those calls are skipped, not fatal. Owner: Task 5.
4. Restore after an import that added nothing to an account, or whose opening correction the user edited meanwhile (no longer correction-shaped): no-op or new correction, never a moved real transaction. Owner: Task 3.
5. A user who reaches the budget page with accounts but no budget (skipped at step 2) or with a budget and only opening corrections: cards show; after the first real transaction they are gone. Owner: Task 9.

---

### Task 1: Complete onboarding for existing users with accounts

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20261108000000.sql`, `internal/infra/storage/migrations/pgsql/20261108000000.sql`
- Test: `internal/infra/storage/migrations/migration_20261108_test.go` (model on `migration_20261103_test.go`)

**Interfaces:**
- Produces: after migrating, every `users_options` row `name = 'onboarding'` whose user owns at least one row in `accounts` (deleted or not) has `value = 'completed'`. Users with no accounts are untouched.

- [ ] **Step 1: Write the failing test** `TestMigration20261108CompletesOnboardingForUsersWithAccounts`: seed three users at the version before (onboarding `started` + one account; `started` + no account; `completed` + account), run the migration, assert values `completed`, `started`, `completed`.
- [ ] **Step 2: Run** `go test ./internal/infra/storage/migrations/ -run 20261108` — expect FAIL (file missing).
- [ ] **Step 3: Write the two SQL files** (one UPDATE with `user_id IN (SELECT user_id FROM accounts)`, also setting `updated_at` the way other option writes do). Keep ASCII.
- [ ] **Step 4: Run** the test and `go test ./internal/infra/storage/migrations/` — expect PASS (including `version_parity_test.go`). If a pgsql twin test exists for comparable migrations (`migration_20261017_pgsql_test.go`), add the same for this one.
- [ ] **Step 5: Commit** `feat: complete onboarding for existing users with accounts`

### Task 2: `getting_started` user option

**Files:**
- Modify: `internal/model/user.go` (constant + setter/reader), `internal/model/user_dto.go` (request/result), `internal/user/profile.go` (use case), `internal/user/api/` (new handler file `getting_started.go` + `routes.go`), `internal/test/apiparity/catalogue.go` (scenario `user_write_read`), `web/src/api/dto/user.ts`, `web/src/api/user.ts`, `web/src/features/user/queries.ts`
- Test: `internal/model/user_test.go`, `internal/user/profile_test.go` (or the file covering `UpdateAnalytics`), `web/src/features/user/queries.test.ts` (create if absent)

**Interfaces:**
- Produces (Go): `model.OptionGettingStarted = "getting_started"`; `func (u *User) SetGettingStarted(value string, id vo.Id, now time.Time)` (upsert, appends when missing, like `SetAnalytics`); `model.UpdateGettingStartedRequest{Value *string json:"value"}` with `Validate()`; result `model.UpdateGettingStartedResult{User CurrentUserResult json:"user"}`; `func (s *Service) UpdateGettingStarted(ctx, userID vo.Id, req model.UpdateGettingStartedRequest) (*model.UpdateGettingStartedResult, error)`; route `POST /api/v1/user/update-getting-started`.
- Value format: comma-separated keys, each from `plan`, `import`, `apple_wallet`, with an optional `!` prefix meaning dismissed (e.g. `"plan,!import"`); `""` is valid and means "went through Day 0, nothing done yet". Validation: nil → `common.is_blank` on `value`; unknown key or duplicate → field error `value` with the existing `common.invalid_choice` code.
- Produces (TS): `UserOptions.GETTING_STARTED = 'getting_started'`; `updateGettingStarted(value: string): Promise<CurrentUserDto>`; `useUpdateGettingStarted()` (mutate arg `value: string`, writes `queryKeys.user`); `gettingStartedOption(user): string | null` (null = option absent = not a Day 0 user).

- [ ] **Step 1: Write failing tests**: model `TestSetGettingStartedAppendsAndUpdates`; DTO `TestUpdateGettingStartedRequestValidate` cases `nil`→blank, `"plan,!import"` ok, `""` ok, `"plan,plan"` invalid, `"foo"` invalid; service `TestUpdateGettingStartedPersists` (option present in `CurrentUserResult.Options` with the value).
- [ ] **Step 2: Run** `go test ./internal/model/ ./internal/user/...` — expect FAIL.
- [ ] **Step 3: Implement** the model, DTO, service, handler (`endpoint.Handle`, swag block like `analytics.go`), route.
- [ ] **Step 4: Add apiparity call** `{Label: "update-getting-started", Method: "POST", Path: "/api/v1/user/update-getting-started", Auth: "owner", Body: map[string]any{"value": "plan,!import"}}` plus an `err:` call with `"value": "foo"`; regenerate goldens; inspect.
- [ ] **Step 5: Run** `go test ./internal/... ` for user, model, apiparity — expect PASS.
- [ ] **Step 6: Implement the TS client, hook and reader**; test `gettingStartedOption` returns null when absent and the string when present; run `cd web && pnpm test src/features/user`.
- [ ] **Step 7: Commit** `feat: add getting_started user option`

### Task 3: Balance snapshot and restore (backend)

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/account_corrections.sql` (new queries), regenerate `gen/`; `internal/account/repository.go`, `internal/account/repo/{repo.go,sqlite.go,pgsql.go}`; `internal/model/account_dto.go`; `internal/account/api/routes.go` + new `internal/account/api/keep_balance.go`; `internal/test/apiparity/catalogue_account.go`
- Create: `internal/account/keep_balance.go`
- Test: `internal/account/keep_balance_test.go`, apiparity goldens

**Interfaces:**
- Produces: `GET /api/v1/account/get-balance-snapshot?accountIds=a,b` → `model.BalanceSnapshotResult{TakenAt string json:"takenAt"; Items []BalanceSnapshotItem json:"items"}` with `BalanceSnapshotItem{AccountId string json:"accountId"; Balance string json:"balance"; OpeningCorrectionId string json:"openingCorrectionId"; Fresh bool json:"fresh"}`. `takenAt` uses `datetime.Layout` in UTC (server clock). Ids not writable by the caller are omitted, never an error. At most 200 ids.
- Produces: `POST /api/v1/account/restore-balance` body `model.RestoreBalanceRequest{AccountId string json:"accountId"; Balance string json:"balance"; OpeningCorrectionId string json:"openingCorrectionId"; Since string json:"since"}` → `model.RestoreBalanceResult{Item AccountResult json:"item"; Restored bool json:"restored"}`.
- Service: `func (s *Service) GetBalanceSnapshot(ctx, userID vo.Id, accountIDs []string) (*model.BalanceSnapshotResult, error)`; `func (s *Service) RestoreBalance(ctx, userID vo.Id, req model.RestoreBalanceRequest) (*model.RestoreBalanceResult, error)`.
- Definitions:
  - **Opening correction**: the account's only transaction (counting rows where it is `account_id` or `account_recipient_id`), with NULL `category_id`, `payee_id`, `tag_id`, `account_recipient_id` and `description = ''`. `Fresh` = the account has zero transactions or exactly that one.
  - **Balance**: the same figure `get-account-list` shows (`s.balances.Balance(ctx, id, s.balanceBefore(ctx))`).
  - **Restore**, in one DB transaction holding the account row lock used by update-account: `delta = req.Balance - current`. If `delta == 0` → `Restored=false`. `earliest` = MIN(`spent_at`) of the account's transactions with `created_at >= since`, excluding `req.OpeningCorrectionId`; none → `Restored=false`, nothing written. `at = earliest - 1s`. If `req.OpeningCorrectionId` names a row that still satisfies the opening-correction shape on this account (ignoring the "only transaction" rule, since imports now exist): new signed amount = old signed + delta; zero → delete the row; otherwise update `type` (1 income for positive, 0 expense), `amount` (absolute) and `spent_at = at`. Otherwise insert a new correction via `SaveCorrection` with description `"Balance adjustment"` (the existing constant in `internal/account/usecase.go`) at `at`. Every write bumps the revision. `Restored=true`.
  - Access: write access to the account, same check as update-account; otherwise the existing not-found error.
- New repo methods (names fixed): `AccountTransactionShape(ctx, accountID) (count int, onlyID string, onlyIsCorrection bool, err error)`, `EarliestSpentSince(ctx, accountID, since time.Time, excludeID string) (time.Time, bool, error)`, `GetCorrection(ctx, id, accountID) (*model.AccountCorrection, bool, error)` (false when missing or not correction-shaped), `UpdateCorrection(ctx, c model.AccountCorrection) error` (bumps), `DeleteCorrection(ctx, id, accountID string) error` (bumps).

- [ ] **Step 1: Write failing service tests** (sqlite via `dbtest.New`):
  - `TestBalanceSnapshotFreshStates`: new account with balance 100 → `Fresh=true`, `OpeningCorrectionId` = its correction, `Balance="100"`; account with no balance → `Fresh=true`, id `""`; account with one categorized transaction → `Fresh=false`, id `""`; inaccessible id omitted.
  - `TestRestoreMovesOpeningCorrection`: balance 100 (correction dated today), snapshot, import-like insert of an expense 30 dated 10 days ago with `created_at` after `takenAt`, restore → balance back to 100, the correction now has amount 130, type income, `spent_at` = 10 days ago − 1s; revision moved.
  - `TestRestoreDeletesZeroedCorrection`: balance 30, insert income 30 in the past, restore → correction deleted, balance 30.
  - `TestRestoreAddsCorrectionWhenNotFresh`: account with existing categorized transactions, restore with `OpeningCorrectionId=""` → new "Balance adjustment" row just before the earliest imported row; no existing row changed.
  - `TestRestoreEditedOpeningCorrectionIsNotMoved`: the opening correction got a category before restore → it is untouched and a new correction is inserted.
  - `TestRestoreNoImportedRowsIsNoop`: delta ≠ 0 but nothing created since → `Restored=false`, no writes.
  - `TestRestoreRequiresWriteAccess`.
- [ ] **Step 2: Run** `go test ./internal/account/ -run 'Snapshot|Restore'` — expect FAIL.
- [ ] **Step 3: Add the sqlc queries** to both engines, `sqlc generate`, extend the querier interface and both adapters.
- [ ] **Step 4: Implement** DTOs (with `Validate()`: accountId/since/balance not blank, balance a decimal, since in `datetime.Layout`), service methods, handlers (`endpoint.HandleNoBody`-style query-param read is hand-written like other query-param reads; restore uses `endpoint.Handle`), routes.
- [ ] **Step 5: Run** the account tests — expect PASS.
- [ ] **Step 6: Add apiparity scenario** `account_keep_balance`: create account with balance, get snapshot, restore with a delta, plus `err:` restore on a foreign account; regenerate goldens; inspect. Run `go test ./internal/test/apiparity/ ./internal/test/revisionguard/` — expect PASS.
- [ ] **Step 7: Commit** `feat: add account balance snapshot and restore`

### Task 4: Static defaults and guessing helpers (frontend)

**Files:**
- Create: `web/src/features/onboarding/defaults.ts`, `web/src/lib/regionCurrency.ts`
- Modify: `locales/*.json` (new `onboarding.categories.*`, `onboarding.words.*`)
- Test: `web/src/features/onboarding/defaults.test.ts`, `web/src/lib/regionCurrency.test.ts`

**Interfaces:**
- Produces: `DEFAULT_EXPENSE_CATEGORIES` and `DEFAULT_INCOME_CATEGORIES: readonly { key: string; icon: string }[]` with keys `groceries, housing, utilities, transport, eating_out, health, shopping, entertainment, kids, subscriptions` and `salary, other_income` (label = `t('onboarding.categories.<key>')`); `guessAccountIcon(name: string, words: { cash: string; savings: string }): string`; `looksLikeSavings(name: string, savingsWords: string): boolean`; `currencyForRegion(region: string): string | null` (ISO 3166-1 alpha-2 → ISO 4217, full table); `guessCurrencyCode(languages: readonly string[]): string | null` (first locale carrying a region, via `Intl.Locale`).
- Words come from the catalogue as pipe-delimited strings: `onboarding.words.cash` (en: `"cash | wallet"`), `onboarding.words.savings` (en: `"savings | saving | reserve | deposit | emergency"`). Matching is case-insensitive substring per word, trimmed.
- Icons: cash → `payments`, savings → `savings`, anything else → `credit_card`. Category icons are fixed per key; choose Material ligatures that exist in `availableIcons`.

- [ ] **Step 1: Write failing tests**: every default icon and the three account icons are in `availableIcons`; `looksLikeSavings('My Savings', 'savings | reserve')` true, `'Chase'` false; `guessAccountIcon('Cash', …)` → `payments`; `currencyForRegion('CH')` → `CHF`, `'DE'` → `EUR`, `'ZZ'` → null; `guessCurrencyCode(['en', 'de-CH'])` → `CHF`, `['en']` → null.
- [ ] **Step 2: Run** `cd web && pnpm test src/features/onboarding/defaults src/lib/regionCurrency` — FAIL.
- [ ] **Step 3: Implement** both modules and add the catalogue keys to all 11 languages (translate the category names and word lists properly per language).
- [ ] **Step 4: Run** the tests and `go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat: add onboarding defaults and currency guess`

### Task 5: Day 0 draft and writes

**Files:**
- Create: `web/src/features/onboarding/draft.ts`, `web/src/features/onboarding/useDay0.ts`
- Test: `web/src/features/onboarding/useDay0.test.tsx`

**Interfaces:**
- Consumes: Task 2 `updateGettingStarted`; existing `createAccount`, `updateCurrency`, `hideAllCurrencies`, `showCurrency`, `createCategory`, `createBudget`, `completeOnboarding` from `web/src/api/*` (call the API functions directly, not the dedupe hooks, so ids stay stable).
- Produces:
  - `type AccountRow = { key: string; name: string; balance: string; currencyId: Id; savings: boolean; icon: string }`
  - `type CategoryRow = { key: string; name: string; type: 'expense' | 'income'; icon: string; checked: boolean }`
  - `type Day0Draft = { step: 'accounts' | 'categories'; ops: Record<string, Id>; accountIds: Id[]; savingsAccountIds: Id[]; mainCurrencyId: Id | null }` persisted under localStorage key `econumo-day0:<userId>`; `loadDraft(userId)`, `saveDraft(userId, d)`, `clearDraft(userId)`.
  - `useDay0()` returning `{ draft, saveAccounts(rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id): Promise<void>, finish(categories: CategoryRow[]): Promise<void>, skip(): Promise<void>, pending: boolean, error: unknown }`.
- Behaviour:
  - Operation ids are `uuidv7()` minted once per row/step and stored in `draft.ops` before the request (keys `account:<rowKey>`, `category:<rowKey>`, `budget`), so a retry re-sends the same id and the server's operation guard returns the original.
  - `saveAccounts`: create each account (`folderId: null`, balance `""`→`"0"`), then `updateCurrency(mainCode)`, then `hideAllCurrencies()`, then `showCurrency` for every selected currency id; a 4xx from hide/show is ignored (base/profile currency rules). Then `step = 'categories'`.
  - `finish`: create checked categories (expense and income), then `createBudget({ id, name: t('onboarding.budget_name'), startDate: '', currencyId: mainCurrencyId, accountIds, savingsAccountIds })` (the server makes it the default), then `updateGettingStarted('')`, then `completeOnboarding()`, then `clearDraft`, invalidate all queries, track `METRICS.USER_COMPLETE_ONBOARDING` (already fired by the existing hook path; fire once).
  - `skip`: `updateGettingStarted('')`, `completeOnboarding()`, `clearDraft`, track `METRICS.ONBOARDING_SKIP` with `{ step }`.
  - Resume: if the draft is missing but the user already owns accounts and onboarding is not completed, start at `categories` with those accounts as `accountIds`.

- [ ] **Step 1: Write failing tests** with msw: `saveAccounts` posts create-account per row with ids from the draft, and a second call after a simulated failure on the 2nd account re-sends the same id for row 1 and 2; hide-currency 400 does not reject; `finish` posts categories before create-budget, budget body contains `accountIds` and `savingsAccountIds`, then update-getting-started `""` and complete-onboarding; `skip` posts update-getting-started then complete-onboarding; resume-from-accounts case.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS (add `ONBOARDING_SKIP: 'appOnboardingSkip'` to `METRICS`).
- [ ] **Step 5: Commit** `feat: add Day 0 setup writes`

### Task 6: Accounts step UI

**Files:**
- Create: `web/src/features/onboarding/AccountsStep.tsx`, `web/src/features/onboarding/CurrenciesBlock.tsx`
- Modify: `locales/*.json` (`onboarding.accounts.*`, `onboarding.currencies.*`)
- Test: `web/src/features/onboarding/AccountsStep.test.tsx`

**Interfaces:**
- Consumes: Task 4 helpers, `useCurrencies()`, `useCurrencyRates()`, `useUserData()` (`userCurrencyId`).
- Produces: `AccountsStep({ onContinue, onSkip, pending }: { onContinue(rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id): void; onSkip(): void; pending: boolean })`.
- Behaviour (copy per spec): heading "Where's your money right now?", subtext "Today's balances are enough, no history needed." One empty row to start; typing in the last row appends a new empty row; rows have a remove button. Row currency defaults to the guessed currency (`guessCurrencyCode(navigator.languages)` if that code is in the currency list, else the user's profile currency); tapping opens a searchable currency picker. Icon follows `guessAccountIcon` until the user edits the name (recomputed on name change only). Savings switch defaults from `looksLikeSavings` and stops following the name once toggled. Currencies block: chips of row currencies (not removable while used), "+ Add currency" search for extras, "Main: XXX · based on your accounts" (most-used row currency, ties → first row; tap to pick any selected currency), note "You can add more currencies later in Settings.", and a note when a selected currency has no rate in `useCurrencyRates()` (empty rate list or missing `currencyId`, ignoring the base). Balance accepts `-`, spaces, `,` or `.` as decimal separator; blank = 0; anything else shows an inline error and disables Continue. Continue needs at least one row with a non-blank name. "Skip setup" link calls `onSkip`.

- [ ] **Step 1: Write failing tests**: adding a row appears after typing; "Cash" row gets the cash icon and "Savings" gets the savings switch on; balance "1 234,50" submits `"1234.50"`, "abc" disables Continue; main currency follows the majority; removing the only EUR row drops EUR from the block; no-rate note shows for a currency without a rate; Skip calls `onSkip`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement** (shadcn components, mobile-first, no bold for hierarchy). **Step 4: Run** — PASS, plus `pnpm lint`.
- [ ] **Step 5: Commit** `feat: add onboarding accounts step`

### Task 7: Categories step UI

**Files:**
- Create: `web/src/features/onboarding/CategoriesStep.tsx`
- Modify: `locales/*.json` (`onboarding.categories_step.*`)
- Test: `web/src/features/onboarding/CategoriesStep.test.tsx`

**Interfaces:**
- Produces: `CategoriesStep({ onContinue, onSkip, pending }: { onContinue(rows: CategoryRow[]): void; onSkip(): void; pending: boolean })`.
- Behaviour: heading "What do you spend on?", subtext "Untick what you don't need. You can change these later." Groups "Expenses" and "Income" built from Task 4 defaults, all checked. Each row: checkbox, icon button opening `IconPicker`, inline-editable name. Each group ends with an "add your own" row that becomes a checked row on Enter/blur when non-empty. Continue disabled while no row is checked. Names trimmed; duplicates within a type are merged (case-insensitive).

- [ ] **Step 1: Write failing tests**: all 12 defaults checked; unticking Kids excludes it from `onContinue`; renaming and icon change are passed through; adding "Pets" under Expenses appears checked with type expense; adding under Income gives type income; duplicate names collapse.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: add onboarding categories step`

### Task 8: Wire the new onboarding page, remove the old one

**Files:**
- Modify: `web/src/features/onboarding/OnboardingPage.tsx` (rewrite), `web/src/features/onboarding/OnboardingPage.test.tsx` (rewrite), `web/src/lib/metrics.ts`, `locales/*.json` (add `onboarding.budget_name` = "My budget"; delete old `onboarding.*` keys no longer used, keep `common.nav.onboarding`)

**Interfaces:**
- Consumes: Tasks 5–7.
- Behaviour: page shows `AccountsStep` or `CategoriesStep` per `draft.step`; a small step indicator "Step 1 of 2"; after `finish` navigate to `RouterPage.BUDGET`; after `skip` navigate to `RouterPage.BUDGET`. Errors render via `apiErrorMessage` above the Continue button. `HomePage` gating and the nav links stay as they are (they already disappear once completed). Fire `ONBOARDING_ACCOUNTS_COMPLETE: 'appOnboardingAccountsComplete'` (with `{ accounts: n, currencies: m }`) after `saveAccounts`.

- [ ] **Step 1: Rewrite the page test** (msw, template = old test's render helper): fresh user sees step 1; filling one account and continuing posts create-account and shows step 2; continuing posts categories, create-budget, update-getting-started, complete-onboarding and lands on `/budget`; Skip on step 1 posts update-getting-started + complete-onboarding and lands on `/budget`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement**; delete unused old keys from all catalogues. **Step 4: Run** `pnpm test`, `pnpm lint`, `go test ./internal/test/i18ntest/` — PASS.
- [ ] **Step 5: Commit** `feat: replace onboarding page with the Day 0 flow`

### Task 9: Get-ready cards and Inbox next steps

**Files:**
- Create: `web/src/features/onboarding/useGettingStarted.ts`, `web/src/features/onboarding/GettingStartedCards.tsx`, `web/src/features/inbox/NextStepsSection.tsx`
- Modify: `web/src/features/budgets/BudgetPage.tsx` (empty-budget and has-budget branches), `web/src/features/inbox/InboxPage.tsx`, `web/src/features/budgets/queries.ts` (`useSetLimit`, `usePlanSetLimit` onSuccess), `web/src/features/transactions/ImportCsvDialog.tsx` (on success), `web/src/lib/metrics.ts`, `locales/*.json` (`onboarding.cards.*`)
- Test: `web/src/features/onboarding/useGettingStarted.test.tsx`, `web/src/features/onboarding/GettingStartedCards.test.tsx`, extend `BudgetPage` and `InboxPage` tests

**Interfaces:**
- Consumes: Task 2 `gettingStartedOption`, `useUpdateGettingStarted`; `useIsCompact()`, `isIOS()`, `isStandalone()`, `useInstallApp()`, `useImportSources()`, `useTransactions()`, `useAccounts()`.
- Produces: `type StepKey = 'home_screen' | 'apple_wallet' | 'plan' | 'import'`; `useGettingStarted(): { active: boolean; steps: { key: StepKey; done: boolean; dismissed: boolean }[]; inboxSteps: { key: StepKey; done: boolean; dismissed: boolean }[]; hasData: boolean; markDone(key): void; dismiss(key): void }`.
- Rules:
  - `active` = the `getting_started` option exists.
  - Card steps by device: compact (phone/tablet) → `home_screen`, then `apple_wallet` if `isIOS()`; otherwise → `plan`, then `import`. Inbox steps: the card steps plus `plan` and `import` on every device (so a phone user still sees them), never `home_screen` on desktop, never `apple_wallet` off iOS. `steps` returns the card steps; `inboxSteps` returns the Inbox list (add it to the hook's return type).
  - Done: `plan`/`import`/`apple_wallet` from the option keys (plain key = done, `!key` = dismissed); `apple_wallet` also done when an Apple Wallet import source exists; `home_screen` done when `isStandalone()` or localStorage `econumo-getting-started-home-screen` is `done`/`dismissed` (per device).
  - `markDone('plan')` is called from `useSetLimit` and `usePlanSetLimit` onSuccess, `markDone('import')` from `ImportCsvDialog` success; both no-ops when not active or already done (no request).
  - `hasData` = some accessible account has a transaction that is not an opening correction (same shape as Task 3: an account whose only transaction has no category/payee/tag/recipient and empty description).
  - Cards render on the budget page when `active && !hasData` and at least one device step is neither done nor dismissed: in the `budget-empty` state (below the existing create-budget/create-account content) and in place of the empty budget table when a budget exists. Card actions: `home_screen` → `useInstallApp().start('settings')` (extend `InstallSource` with `'getting_started'`), `apple_wallet` → navigate `RouterPage.SETTINGS_APPLE_WALLET`, `plan` → navigate `RouterPage.PLAN`, `import` → open `ImportCsvDialog`. Each card has "Skip" → `dismiss`.
  - Inbox `NextStepsSection` (title "Next steps") renders when `active` and any listed step is neither done nor dismissed; items are the same actions plus a dismiss button; NOT counted in `useInbox().count`; rendered after the existing sections.
  - Metrics: `GETTING_STARTED_OPEN: 'appGettingStartedOpen'` (`{ step, surface: 'budget' | 'inbox' }`), `GETTING_STARTED_DISMISS: 'appGettingStartedDismiss'` (`{ step }`).

- [ ] **Step 1: Write failing tests**: hook — desktop gives `plan, import`, compact iOS gives `home_screen, apple_wallet`, compact Android gives `home_screen`; option `"plan,!import"` → plan done, import dismissed; absent option → `active=false`; `hasData` false with only an opening correction, true after a categorized transaction. Cards — visible on the budget page with a budget and only opening corrections, gone once a real transaction exists, gone when all steps done/dismissed; Skip posts update-getting-started with `!key`. Inbox — Next steps section shows, badge count unchanged.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** `pnpm test`, `pnpm lint` — PASS.
- [ ] **Step 5: Commit** `feat: add getting started cards and inbox next steps`

### Task 10: "Keep current balance" in CSV import

**Files:**
- Modify: `web/src/api/account.ts` (`getBalanceSnapshot(accountIds: Id[]): Promise<BalanceSnapshotDto>`, `restoreBalance(req: RestoreBalanceRequestDto): Promise<RestoreBalanceResultDto>`), `web/src/api/dto/account.ts`, `web/src/features/transactions/importCsv.ts`, `web/src/features/transactions/ImportCsvDialog.tsx`, the import result dialog, `web/src/lib/metrics.ts`, `locales/*.json` (`transactions.import.keep_balance.*`)
- Test: `web/src/features/transactions/importCsv.test.ts`, `web/src/features/transactions/ImportCsvDialog.test.tsx`

**Interfaces:**
- Consumes: Task 3 routes.
- Produces: `targetAccountIds(analysis, selection, accounts: AccountDto[]): Id[]` (fixed account → `[accountId]`; CSV column → distinct column values matched case-insensitively against writable account names; unmatched names ignored); `runImportKeepingBalance(analysis, selection, post, accounts, onProgress?): Promise<AggregatedImportResult & { kept: { accountId: Id; balance: string }[] }>`.
- Behaviour: when targets are known, fetch a snapshot; the checkbox "Keep current balance" (hint: "Today's balance stays as it is; older transactions are balanced by an adjustment.") is shown when there is at least one target, ticked by default iff every target's `fresh` is true. On submit with the box ticked: take a fresh snapshot, run the normal chunked import, then call `restoreBalance` for every snapshotted account with its `balance`, `openingCorrectionId` and `since = takenAt` (also after a partial import); collect `restored === true` into `kept`. The result dialog adds one line per kept account: "Kept today's balance of {amount}" (formatted with the account currency). Unticked → exactly today's behaviour. Fire `IMPORT_KEEP_BALANCE: 'appImportKeepBalance'` with `{ accounts: kept.length }` when ticked.

- [ ] **Step 1: Write failing tests**: `targetAccountIds` both modes; dialog shows the box ticked for a fresh account and unticked for a used one; ticked submit calls snapshot → import → restore with the snapshot values; partial import (one chunk 500) still restores; unticked never calls snapshot/restore; result shows the kept line.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat: keep current balance on CSV import`

### Task 11: Docs and full verification

**Files:**
- Modify: `docs/regression-test-plan.md` (rewrite `## 3. Onboarding (fresh user)`; update :657 nav note and the item at :2727 if affected; add import "Keep current balance" items in the CSV import section; mark 📱 accurately), `CLAUDE.md` (frontend section: Day 0 flow, `getting_started` option and Next steps not counted in the badge; Notable behaviours: import keep-balance and the two account routes), `docs/superpowers/specs/2026-11-01-onboarding-day-0-design.md` (Apple Wallet flag no longer exists: card shows on iPhone)

- [ ] **Step 1: Update the docs** as listed; regression items phrased as verifiable outcomes, covering: fresh sign-up through both steps on phone and desktop, reload mid-flow without duplicates, skip on each step, cards per device and their disappearance after the first transaction, Inbox Next steps without badge, CSV import with the box ticked/unticked, existing users not seeing onboarding.
- [ ] **Step 2: Run** `make go-test` — PASS (coverage gate ≥ 80). Run `cd web && pnpm test && pnpm lint && pnpm build` — PASS.
- [ ] **Step 3: Commit** `docs: onboarding Day 0 regression plan and CLAUDE.md`
