# Budget Savings — Membership Flag Rework Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move "this account is savings" from the account (`accounts.type = 3`) to the budget membership (`budgets_accounts.is_savings`), edited in the budget's settings behind a server-side confirmation, and make the plan's balance split follow savings balances as well as rows.

**Architecture:** Everything below the flag already exists on the branch and stays: `ElementSavings = 5`, `SavingsByMonth` / `AccountsNetByMonth`, `structure.savings` + `savingsOpeningBalances` / `savingsFlows`, the plan section and the monthly block. This plan (1) adds the column and switches the builders and `syncElements` to read it, (2) adds the write paths and the confirmation guard, (3) removes the account-type work, (4) replaces the account dialog switch with a per-account toggle in the budget settings, (5) fixes the balance-split condition, (6) closes docs and gates.

**Tech Stack:** Go 1.27 (stdlib `net/http`, sqlc, modernc sqlite + pgx), React 19 + TanStack Query + vitest, react-i18next over `locales/*.json`.

**Spec:** `docs/superpowers/specs/2026-09-12-budget-savings-design.md` — Revisions 7–9 (2026-09-25) and the rewritten sections "Savings flag on budget membership", "Element sync", "Budget settings", "Analytics", "i18n", "Testing". Read those before Task 1.

## Global Constraints

- **Go is not on `PATH`**: `export PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=go1.27.1`.
- Work in the worktree root; branch `budget-savings-work` (pushes to `origin feature/budget-savings`, fast-forward only). Commit after every task. Never force-push, never `git stash` (the stash stack is shared).
- PostgreSQL for the pgsql tiers: start a throwaway `postgres:17-alpine` on `127.0.0.1:55433` (user/password `econumo`, db `econumo_test`); `DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@127.0.0.1:55433/econumo_test?sslmode=disable'`.
- Migrations: one new version for BOTH engines, same number, `.sql` files ASCII-only (comments too). sqlc query files: ASCII-only, `;` ends the last SQL line (never alone on a line); regenerate with `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`; use the pinned sqlc from go.mod / the Makefile).
- Frozen values: column `budgets_accounts.is_savings BOOLEAN NOT NULL DEFAULT false`; request fields `savingsAccountIds` (create-budget, update-budget), `isSavings` (add-account), `confirmSavingsRemoval` (update-budget, add-account, remove-account); `filters.accounts[].isSavings` (bool); error codes `budget.savings_account_not_member` (field `savingsAccountIds`) and `budget.savings_removal_unconfirmed` (field `confirmSavingsRemoval`); MCP params `is_savings`, `confirm_savings_removal`; metric `ACCOUNT_SAVINGS_TOGGLE` removed and `BUDGET_SAVINGS_TOGGLE: 'appBudgetSavingsToggle'` added.
- The confirmation guard is server-side: a refused write writes NOTHING.
- Every new `errs` code: `codes.go` + `AllCodes` + real translations in all 11 `locales/*.json`. Removed codes leave all three places too.
- New `t()` keys in all 11 catalogues with real translations; plurals pipe-delimited via `pluralPick`.
- Every `METRICS` key must be fired from non-test source; removed keys leave no reference.
- DTO changes → `make swagger`. Never hand-edit goldens: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`, then read the diff; each task names the only allowed changes.
- Guard floors: read the current `min` / `minRoutes` from the files; raise by what you add, never lower.
- Web tests: `cd web && pnpm exec vitest run <path>` (not `pnpm test -- …`). Known pre-existing failure: `src/api/transaction.test.ts` Blob test.
- Comments sparingly (why, not what); no PHP references.

## Review Focus

1. **A refused write is a no-op** — `update-budget` that both renames the budget and turns a planned savings member off, without confirmation, must leave the name, membership, flags, element and limits untouched. (Task 2)
2. **Removing a savings member through `accountIds`** (not through the flag) is guarded exactly like turning the flag off. (Task 2)
3. **Another participant's savings account** — `savingsAccountIds` naming an account the caller doesn't own is refused and changes nothing; the owner's flag survives the other participant's `update-budget` that omits it (replace-set is over the CALLER's own accounts only). (Task 2)
4. **An unplanned, uncommented savings member** turns off without any confirmation, and its (empty) element disappears in the same request. (Task 2)
5. **A deleted savings account with a pre-window balance and no row** still produces the Balance / Savings balance split in the plan. (Task 5)

---

## File Structure

- Backend new: `internal/infra/storage/migrations/{sqlite,pgsql}/20260925000000.sql`
- Backend modified: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql` (+ regenerated `gen/`), `internal/budget/repo/{engine.go,repo.go}` (+ adapters), `internal/budget/repository.go`, `internal/model/budget.go` (`BudgetAccount.IsSavings`), `internal/model/budget_dto.go`, `internal/model/budget_view.go` (`AccountView` loses `Type`), `internal/server/glue_budget.go`, `internal/budget/{builder.go,move.go,create.go,crud.go,accounts.go,clone.go,accesssvc.go}`, new `internal/budget/savings_flag.go` (guard + flag application helpers), `internal/budget/mcp/mcp.go`, `internal/shared/errs/codes.go`, `locales/*.json`
- Account-type removal: `internal/model/{account.go,account_dto.go,account_test.go}`, delete `internal/model/account_type_test.go` and `internal/account/api/account_type_test.go`, `internal/account/{create.go,update.go}`, `internal/account/mcp/mcp.go`
- Tests: `internal/budget/api/savings_*_test.go`, `internal/budget/repo/*savings*_test.go`, `internal/test/apiparity/catalogue_budget_savings.go` + goldens, mcpparity goldens
- SPA: `web/src/api/{account.ts,dto/account.ts,dto/budget.ts,budget.ts}`, `web/src/features/accounts/*` (remove savings bits; delete `SavingsMarker.tsx`, `savingsPlans.ts(+test)`), `web/src/features/budgets/{BudgetAccountsField.tsx,BudgetUpdateDialog.tsx,BudgetDialog.tsx,queries.ts,PlanSheet.tsx,planMath.ts}`, `web/src/lib/metrics.ts`
- Docs: `docs/regression-test-plan.md`, `CLAUDE.md`, the spec's Status section, whitespace in `docs/superpowers/plans/2026-09-24-budget-savings.md`

---

### Task 1: The flag column, and readers + sync read it

**Files:** migrations (both engines), sqlc queries + regenerated code, budget repo + `repository.go`, `model.BudgetAccount`, `builder.go` (`buildFilters`), `move.go` (`syncElements`), `model.AccountView` + `glue_budget.go`, savings tests.

**Interfaces:**
- Produces: `model.BudgetAccount{AccountID vo.Id; IsSavings bool; CreatedAt time.Time}`; repo `AddAccount(ctx, budgetID, accountID vo.Id, isSavings bool, now time.Time) error` (every existing caller passes `false` for now — clone passes the source member's flag); new repo `SetAccountSavings(ctx, budgetID, accountID vo.Id, isSavings bool) error`; `filters.savingsAccounts` / `filters.everydayAccountIDs` now split by `BudgetAccount.IsSavings`; `model.AccountView` loses `Type`.

- [ ] **Step 1: Failing tests.** Convert every existing savings test that makes an account savings by `accounts.type = 3` (raw SQL flips in `internal/budget/api/savings_test.go`, `savings_budget_test.go`, `savings_plan_test.go`; fixtures with `Type: 3` in `internal/budget/repo/read_savings_test.go`, which only need the type dropped) to flag the MEMBERSHIP instead, through a test helper `flagSavings(t, db, budgetID, accountID string, on bool)` doing `db.Rebind("UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?")`. Add: (a) an account flagged in budget A and not in budget B is a savings row in A and absent from B's `structure.savings` (and present in B's expense view as an everyday account); (b) `clone-budget` copies the flag (the copy's `structure.savings` lists it); (c) a repo round trip: `AddAccount(..., true, now)` then `MemberAccounts` returns `IsSavings: true`, `SetAccountSavings(..., false)` flips it — both engines.
- [ ] **Step 2: Run** — FAIL (column missing).
- [ ] **Step 3: Implement.**
  - Migration `20260925000000.sql` both engines: `ALTER TABLE budgets_accounts ADD COLUMN is_savings BOOLEAN NOT NULL DEFAULT false;` (SQLite: follow the repo's BOOLEAN convention, `DEFAULT '0'` as in `is_deleted`; PostgreSQL: `DEFAULT false`). Comment: why the flag lives on the membership (per-budget role; spec Revision 7).
  - sqlc (both engines): the member-accounts SELECT returns `is_savings`; the INSERT takes it; new `UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?`. Regenerate, extend both adapters + the querier interface (engine-adapter pattern, CLAUDE.md).
  - `buildFilters`: savings/everyday split by `b.accounts[i].IsSavings` (membership order unchanged); `syncElements`: savings members = flagged `b.accounts`, currency/name still from `AccountsByIDs`.
  - Remove `AccountView.Type` and its glue assignment + glue test assertion.
  - Every `AddAccount` caller (create, update-budget replace-set, add-account, accept-access seeding) passes `false`; clone passes the source member's `IsSavings`.
- [ ] **Step 4: Run** `go test ./internal/budget/... ./internal/server/ ./internal/model/`, the pgsql tier for `./internal/budget/...`, `go test ./internal/test/apiparity/ ./internal/test/mcpparity/`. Golden changes allowed: NONE except the `budget_savings` scenario, whose accounts are made savings through `create-account`'s `type: 3` — that scenario now shows no savings rows; accept that golden change for now (Task 3 rewrites the scenario) and say so in the report. `make go-lint`.
- [ ] **Step 5: Commit** `feat(budget): savings flag on budget membership`.

### Task 2: Write paths and the confirmation guard

**Files:** `internal/model/budget_dto.go`, new `internal/budget/savings_flag.go`, `create.go`, `crud.go` (`UpdateBudget`), `accounts.go` (`AddAccount`, `RemoveAccount`), repo (+ sqlc) for the guard query, `internal/budget/mcp/mcp.go`, `codes.go`, 11 locales, swagger, goldens.

**Interfaces:**
- Produces (wire): `CreateBudgetRequest.SavingsAccountIds []string \`json:"savingsAccountIds"\``; `UpdateBudgetRequest.SavingsAccountIds []string` (nil = absent) and `ConfirmSavingsRemoval bool \`json:"confirmSavingsRemoval"\``; `AddAccountRequest.IsSavings *bool \`json:"isSavings"\`` and `ConfirmSavingsRemoval bool`; `RemoveAccountRequest.ConfirmSavingsRemoval bool`; `BudgetAccountFilter.IsSavings bool \`json:"isSavings"\``.
- Produces (repo): `SavingsElementHasData(ctx, budgetID, accountID vo.Id) (bool, error)` — true when the budget's `ElementSavings` element with that external id has at least one row in `budgets_elements_limits` or `budgets_elements_comments`.
- Produces (errors): `errs.CodeBudgetSavingsAccountNotMember = "budget.savings_account_not_member"` (field `savingsAccountIds`, en "Savings accounts must be your own accounts in this budget"); `errs.CodeBudgetSavingsRemovalUnconfirmed = "budget.savings_removal_unconfirmed"` (field `confirmSavingsRemoval`, en "Turning savings off deletes this budget's planned amounts and comments for that account; confirm to continue").

- [ ] **Step 1: Failing API tests** (`internal/budget/api/savings_flag_test.go`, real SQLite harness; multi-user fixtures with explicit emails):
  1. `create-budget` with `savingsAccountIds ⊆ accountIds` → those members flagged (`filters.accounts[].isSavings`), savings rows present; an id outside `accountIds` → 400 `savingsAccountIds` and no budget created.
  2. `update-budget` with `savingsAccountIds` flags/unflags the caller's own members; absent leaves flags untouched; an id that is not the caller's member after `accountIds` is applied (someone else's account, or a non-member) → 400 `savingsAccountIds`, nothing changed.
  3. Review Focus 3: participant B's `update-budget` (with or without `savingsAccountIds`) never changes owner A's flags.
  4. Guard (Review Focus 1, 2, 4): member S flagged with a limit → `update-budget` turning it off (or dropping it from `accountIds`) and renaming the budget, without `confirmSavingsRemoval` → 400 field `confirmSavingsRemoval`; name, membership, flag, element and limit all unchanged. Same request with `confirmSavingsRemoval: true` → 200, the savings element, its limits and its comments are gone (query the tables). An element with only a comment is guarded too. A flagged member with no limit and no comment turns off without confirmation and its element is gone in the same request.
  5. `add-account` `isSavings: true` on a new member → flagged + row; on an existing member toggles; absent on an existing member leaves it; `isSavings: false` on a planned savings member → guarded like 4. `remove-account` of a planned savings member → guarded; confirmed → removed with its element.
  6. Archived budget: any flag write → 403 `budget.archived` (the existing guard).
- [ ] **Step 2: Run** — FAIL.
- [ ] **Step 3: Implement.**
  - `savings_flag.go`: one helper computing, for the caller, the target flag per own member from the request (create/update/add/remove), the set of members LEAVING savings (flag off or membership removed), the guard (`SavingsElementHasData` per leaving member → refuse unless confirmed), then applying membership + flags and running `syncElements` — all inside the use case's existing transaction so a refusal or failure writes nothing. Keep the three use cases thin callers of it.
  - `filters.accounts` entries set `IsSavings`.
  - MCP: `add_budget_account` gains `is_savings` (optional bool) and `confirm_savings_removal`; `remove_budget_account` gains `confirm_savings_removal`; descriptions say what the flag does and that it is visible to all budget participants.
  - Codes + 11 locales; `make swagger`.
- [ ] **Step 4: Run** the new tests + `go test ./internal/budget/... ./internal/test/...`; goldens: allowed changes are `"isSavings": false` added to every `filters.accounts` entry (REST + MCP get-budget), the MCP tools/list schema/description changes for the three request fields and two tools, and the `budget_savings` scenario (still pending Task 3). `make go-lint`, pgsql tier for `./internal/budget/...`.
- [ ] **Step 5: Commit** `feat(budget): savings flag write paths with server-side removal confirmation`.

### Task 3: Remove the account type work; rewrite the parity scenario

**Files:** `internal/model/{account.go,account_dto.go,account_test.go}`, delete `internal/model/account_type_test.go` and `internal/account/api/account_type_test.go`, `internal/account/{create.go,update.go,mcp/mcp.go}`, `codes.go` (`CodeAccountInvalidType` out of the const block and `AllCodes`), `errors.account.invalid_type` out of 11 locales, `internal/test/apiparity/catalogue_budget_savings.go` + golden, mcpparity golden, swagger.

- [ ] **Step 1:** Restore the account model to the pre-feature shape: no `TypeSavings`, `Valid()` accepts 1 and 2, no `UpdateType`, no `Type` on create/update requests, no `type` on `create_account` MCP. Keep the repo round-trip test for a stored type outside the known set (`internal/account/repo/repo_integration_test.go`) — reads still tolerate any stored value — but update its comments to not mention savings. Restore `internal/model/account_test.go`'s original assertion (3 is not a valid type). Compare with `git show c9013ee:<path>` to restore exactly; keep nothing of the account-type feature.
- [ ] **Step 2:** Rewrite the `budget_savings` apiparity scenario: create accounts WITHOUT `type`; create the budget with `accountIds`; flag savings with `update-budget` `savingsAccountIds`; set-limit; transfers (keep the second-currency account + change-element-currency); get-budget; get-budget-plan; move-element folder refusal; reorder; `update-budget` turning a planned savings account off WITHOUT confirmation (the 400) then WITH it (200, row gone from a following get-budget); drop the old `type: 9` step. Keep the scenario count unchanged (it's the same scenario).
- [ ] **Step 3: Run** `make swagger`; `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`; read the diff: `budget_savings` rewritten as described, `create_account` loses `type` in tools/list, nothing else. `make go-test` (coverage ≥ 80%), `make go-lint`, `make test-engines` and `make test-repo-pgsql` with the PostgreSQL URL.
- [ ] **Step 4: Commit** `refactor(account): drop the savings account type`.

### Task 4: SPA — savings toggle in budget settings

**Files:** remove account-level savings: `web/src/api/{account.ts,dto/account.ts}` (no `SAVINGS`, no `type` on payloads, no `isSavingsAccount`), `web/src/features/accounts/{AccountDialog.tsx,AccountDialog.test.tsx,SidebarAccountTree.tsx,SidebarAccountTree.test.tsx,AccountsSettingsPage.tsx,AccountsSettingsPage.test.tsx,queries.ts,queries.test.tsx}`, delete `SavingsMarker.tsx`, `savingsPlans.ts`, `savingsPlans.test.ts` (restore to `c9013ee` shape where a file only carried savings changes; KEEP the budgetPlan invalidation in `applyItem` added by commit dc78288 — it is an independent fix); add: `web/src/api/dto/budget.ts` (`BudgetAccountFilterDto.isSavings`; create/update payload `savingsAccountIds?`, `confirmSavingsRemoval?`), `web/src/api/budget.ts`, `web/src/features/budgets/{BudgetAccountsField.tsx,BudgetUpdateDialog.tsx,BudgetDialog.tsx,queries.ts}` (+ tests), `web/src/lib/metrics.ts`, 11 locales.

**Interfaces:**
- `BudgetAccountsField` gains `savings: Set<Id>` and `onToggleSavings(id: Id, on: boolean)`; a "Savings" switch per SELECTED own account (hidden for unselected rows), enabled even for locked members; the visibility note under the list.
- `METRICS.BUDGET_SAVINGS_TOGGLE = 'appBudgetSavingsToggle'` replaces `ACCOUNT_SAVINGS_TOGGLE`.

- [ ] **Step 1: Failing tests** (`BudgetAccountsField.test.tsx`, `BudgetUpdateDialog.test.tsx`, a create-dialog test, `queries.test.tsx` for the metric):
  - The update dialog initialises toggles from `filters.accounts[].isSavings`, sends `accountIds` and the full own `savingsAccountIds`; deselecting an account drops it from `savingsAccountIds`; the create dialog sends `savingsAccountIds ⊆ accountIds`.
  - The visibility note text is shown whenever the list is.
  - Removal round trip: MSW answers the first update with a 400 whose `errors` has `confirmSavingsRemoval`; a confirmation dialog opens (spec text); Cancel sends nothing more and keeps the dialog open with the user's edits; Confirm resends the same payload plus `confirmSavingsRemoval: true` and closes on success.
  - A different 400 (e.g. `savingsAccountIds`) surfaces as the dialog's error message, not the confirmation.
  - The metric fires once on a successful create/update whose savings set differs from the previous one, never otherwise; the account dialog no longer shows any savings switch and account payloads carry no `type`.
- [ ] **Step 2: Run** — FAIL.
- [ ] **Step 3: Implement** as specified; use the existing `ConfirmDialog`; detect the refusal with `apiFieldErrors(err, 'confirmSavingsRemoval')`. i18n keys in all 11 catalogues (remove the account-level savings keys added by the first implementation: `accounts.form.savings.*`, `accounts.account.savings_marker`). The update/create mutations invalidate the budget and plan caches on success (verify they already do; add `budgetPlan` if missing).
- [ ] **Step 4: Run** `pnpm exec vitest run src/features/accounts src/features/budgets src/lib/metrics-coverage.test.ts`, `pnpm exec tsc -b`, `pnpm lint`, the Go i18n guard (`go test ./internal/test/i18ntest/`).
- [ ] **Step 5: Commit** `feat(web): savings toggle in budget settings`.

### Task 5: SPA — balance split follows savings balances

**Files:** `web/src/features/budgets/planMath.ts` (a `planHasSavingsData(plan): boolean` helper), `PlanSheet.tsx` (`hasSavings` → the helper), tests (`planMath.test.ts`, `PlanSheet.savings.test.tsx`).

- [ ] **Step 1: Failing tests:** `planHasSavingsData` is true for any savings row, or any non-zero `savingsOpeningBalances[].amount`, or any non-zero `savingsFlows[].amount`; false for all-zero/absent. PlanSheet with NO savings rows but a savings opening balance of 1000 renders the Balance (everyday = combined − 1000) and Savings balance rows (Review Focus 5), and no Savings section/totals line (there are no rows to total).
- [ ] **Step 2–4:** implement (the Savings section and Savings totals line stay tied to rows; the balance split uses the helper); run the budgets folder, `tsc -b`, `pnpm lint`.
- [ ] **Step 5: Commit** `fix(web): the balance split follows savings balances, not only rows`.

### Task 6: Docs, gates, push

- [ ] Regression plan: replace the account-dialog savings items (§4) with budget-settings items — toggle per own member at any time, the visibility note, the removal confirmation (cancel keeps everything; confirm deletes that budget's plans and comments for the account), same account savings in one budget and everyday in another, another participant cannot change your flag; add the P2 item (deleted savings account with a balance and no activity still splits the balance). Keep plan-view and monthly items. 📱 markers accurate.
- [ ] CLAUDE.md "Budget savings" bullet: rewrite for the membership flag (`budgets_accounts.is_savings`, settings toggle, server-side confirmation via `confirmSavingsRemoval`, visible to all participants).
- [ ] Spec Status section: implemented per Revision 7–9; deviation list keeps only the `effectiveNet` note.
- [ ] Fix `git diff --check` whitespace in `docs/superpowers/plans/2026-09-24-budget-savings.md` (tabs after spaces inside indented code blocks). Do not touch goldens' trailing newline (harness output).
- [ ] Gates: `make go-test`, `make test-engines`, `make test-repo-pgsql`, `cd web && pnpm exec tsc -b && pnpm lint && pnpm test`. Record results.
- [ ] Commit `docs: budget savings membership flag`.
