# Budget Savings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Accounts can be marked as savings; every savings account that is a member of a budget becomes a plannable budget row (`ElementSavings = 5`) whose actual is the net money moved in from the budget's everyday accounts, shown in its own section of the plan view and the monthly view.

**Architecture:** `accounts.type = 3` marks a savings account (no migration). Budget element sync creates an `ElementSavings` row per member savings account (external id = account id, currency defaults to the account's), so `set-limit`, `change-element-currency`, `move-element`, clone and #246 comment threads work on it unchanged. Two new hand-built read queries (`SavingsByMonth`, `AccountsNetByMonth`) feed the monthly and plan builders, which emit savings rows in a SEPARATE `structure.savings` array (never inside `elements`, so older clients do not sum them into expenses) through the builders' existing single `BulkConvert` pass. The SPA renders savings rows by adapting them to its existing element-row components.

**Tech Stack:** Go 1.27 (stdlib `net/http`, sqlc, modernc sqlite + pgx), React 19 + TanStack Query + Zustand + dnd-kit + vitest, react-i18next over the shared `locales/*.json`.

**Spec:** `docs/superpowers/specs/2026-09-12-budget-savings-design.md` — read it before Task 1 (Decisions table, Definitions, Revisions).

## Global Constraints

- **Go is not on `PATH`.** Every Go command needs `export PATH=/usr/local/go/bin:$PATH` first (use `GOTOOLCHAIN=go1.27.1` if the toolchain complains).
- Run every command from the worktree root (the Go module root); never `cd` out of the worktree.
- Branch: local `budget-savings-work` (the PR head with `origin/v1.6-dev` merged in), pushed to `origin feature/budget-savings` (the local name `feature/budget-savings` is held by another worktree). Commit after every task. Never force-push.
- Frozen values: `model.TypeSavings AccountType = 3`; `model.ElementSavings ElementType = 5`, alias `"savings"`. `ElementTypeFromAlias` keeps accepting ONLY envelope/category/tag.
- `AccountType.Valid()` gates WRITES only. A stored `accounts.type` outside {1,2,3} must survive a repo read unchanged and reads as "everyday" (anything not 3).
- Savings amounts go through the builders' EXISTING single `BulkConvert` call — account currency → element currency. No second conversion path.
- A savings row's wire `isArchived` is derived from the account's `is_deleted`. Nothing ever writes `is_archived`/folder on a savings element (sync forces `folder_id` NULL and keeps it live).
- New wire fields are always present: `structure.savings` is `[]` (never `null`) in both `get-budget` and `get-budget-plan`; `savingsOpeningBalances` and `savingsFlows` are `[]` when empty.
- Hand-built SQL (`internal/budget/repo/read.go`): SQLite binds datetime bounds via `sqliteDatetime(t)`; PostgreSQL uses numbered `$N` params. SQLite `SUM` scans as `*float64` formatted `strconv.FormatFloat(v, 'f', 8, 64)`; PostgreSQL scans NUMERIC as `*string`. An empty id set short-circuits to no rows (empty `IN ()` is a pgsql syntax error).
- Every new `errs` code goes in `internal/shared/errs/codes.go` **and** `AllCodes` **and** all 11 `locales/*.json` (`de en es fr it nl pl pt ru uk zh`, key `errors.<code>`); `internal/test/i18ntest` asserts a two-way match. Every new SPA `t('…')` key must exist in all 11 catalogues with real translations.
- Every new `METRICS.*` key must be fired from non-test source (`web/src/lib/metrics-coverage.test.ts`).
- Changing a request/response DTO requires `make swagger` (committed OpenAPI docs are checked by `make go-lint`).
- Never hand-edit a golden. Regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` (and `./internal/test/mcpparity/`), then READ the diff: the only expected changes are the ones the task names.
- Guard floors (`minRoutes` in `internal/test/apiparity/guard_test.go`, `min` in `catalogue_test.go`): read the CURRENT value from the file and add to it; never trust a literal quoted here.
- Comments: sparse, *why* not *what* (see CLAUDE.md "Comments — write sparingly").
- SPA test runs: `cd web && pnpm exec vitest run <path>` (NOT `pnpm test -- <pattern>`, which runs everything). The pre-existing `web/src/api/transaction.test.ts` Blob failure is not ours.
- Coverage gate: `make go-test` enforces `GO_COVER_MIN=80`.

## Review Focus

1. **A savings account in a different currency from the budget and from its element** — actual must be converted account currency → element currency once (row) and the section totals shown in budget currency; no raw account-currency figure may leak into the element-currency column. Pinned in Task 4 (monthly) and Task 5 (plan).
2. **A transfer between two savings accounts, and one between a savings account and a non-member account** — neither counts as saved; the second still shows up in the plan's boundary Transfers line. Pinned in Task 3.
3. **An account switched from savings back to everyday while a budget still holds its plans** — the next budget write deletes the savings row and its limits; until then readers must not render the stale row (they derive rows from the CURRENT account type, not from the element table). Pinned in Task 2 (sync) and Task 4 (reader ignores a stale row).
4. **A savings row dragged onto a folder / an `afterId` from the expense group** — rejected with `budget.savings_folder_not_allowed` / treated as an unknown anchor; the expense group's order is untouched. Pinned in Task 2.
5. **A deleted savings account with no plan and no activity** — disappears from both views; with a plan or activity it stays, read-only (`isArchived: 1`). Pinned in Tasks 4 and 5.

---

## File Structure

**Backend — modified**
- `internal/model/account.go` — `TypeSavings`, `Valid()`, `UpdateType`
- `internal/model/account_dto.go` — optional `Type *int` on create/update requests + validation
- `internal/account/{create.go,update.go}` — apply the type
- `internal/account/mcp/mcp.go` — `create_account` gains optional `type`
- `internal/model/budget_valueobject.go` — `ElementSavings`, alias
- `internal/model/budget_view.go` — `AccountView` gains `Name`, `Icon`, `Type`; new `SavingsMonthRow`
- `internal/model/budget_dto.go` — `SavingsElementResult`, `PlanSavingsElementResult`, `PlanSavingsFlowResult`; `StructureResult.Savings`, `PlanStructureResult.Savings`, `BudgetPlanResult.SavingsOpeningBalances`, `BudgetPlanResult.SavingsFlows`
- `internal/server/glue_budget.go` — fill the new `AccountView` fields
- `internal/budget/move.go` — sync savings rows, split ordering groups, move rules
- `internal/budget/readmodel.go` + `internal/budget/repo/read.go` — `SavingsByMonth`, `AccountsNetByMonth`
- `internal/budget/builder.go` — filters gain `savingsAccounts`, `everydayAccountIDs`
- `internal/budget/builder_structure_build.go` — monthly savings rows
- `internal/budget/builder_plan.go` — plan savings rows, `savingsOpeningBalances`, `savingsFlows`
- `internal/budget/mcp/mcp.go` — `set_limit` / `move_element` descriptions
- `internal/shared/errs/codes.go`, `locales/*.json` (11)
- `internal/test/apiparity/` — new `catalogue_budget_savings.go`, floors, goldens; `internal/test/mcpparity/` goldens
- `docs/swagger*` (regenerated)

**Backend — new tests**
- `internal/model/account_type_test.go`
- `internal/budget/savings_sync_test.go`, `internal/budget/savings_builder_test.go` (use the package's existing fakes — see `builder_structure_test.go` / `merge_test.go` for the fake wiring)
- `internal/budget/repo/read_savings_test.go`
- `internal/account/repo/` — a round-trip test in `repo_integration_test.go`
- `internal/budget/api/savings_test.go` (API-level, uses the feature harness in `harness_test.go`)

**Frontend — modified / new**
- `web/src/api/dto/{account.ts,budget.ts}`, `web/src/api/account.ts` payload types
- `web/src/features/accounts/{AccountDialog.tsx,SidebarAccountTree.tsx,AccountsSettingsPage.tsx,queries.ts}`
- new `web/src/features/accounts/savingsPlans.ts` (+ test) — cached-plans lookup for the switch-off confirmation
- `web/src/features/budgets/{planMath.ts,PlanSheet.tsx,queries.ts,BudgetPage.tsx,ExpenseWidget.tsx}`
- new `web/src/features/budgets/SavingsBlock.tsx` (+ test) — monthly savings block
- `web/src/lib/metrics.ts`
- `docs/regression-test-plan.md`, `CLAUDE.md` (one Notable-behaviours bullet)

---

### Task 1: Savings account type

**Files:**
- Modify: `internal/model/account.go`, `internal/model/account_dto.go`, `internal/account/create.go`, `internal/account/update.go`, `internal/account/mcp/mcp.go`, `internal/shared/errs/codes.go`, `locales/*.json`
- Test: `internal/model/account_type_test.go`, `internal/account/repo/repo_integration_test.go`, an API test next to the existing account api tests (`internal/account/api/*_test.go`)

**Interfaces:**
- Produces: `model.TypeSavings`, `(model.AccountType).Valid() bool` (accepts 1,2,3), `(*model.Account).UpdateType(t model.AccountType, now time.Time)`, `CreateAccountRequest.Type *int`, `UpdateAccountRequest.Type *int` (json `"type"`), `errs.CodeAccountInvalidType = "account.invalid_type"`.

- [ ] **Step 1: Failing model + DTO tests** — create `internal/model/account_type_test.go`:

```go
package model_test

import (
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestAccountTypeValid(t *testing.T) {
	for v, want := range map[int16]bool{0: false, 1: true, 2: true, 3: true, 4: false} {
		if got := model.AccountType(v).Valid(); got != want {
			t.Errorf("Valid(%d)=%v want %v", v, got, want)
		}
	}
	if model.TypeSavings != 3 {
		t.Fatalf("TypeSavings is frozen at 3")
	}
}

func TestAccountUpdateType(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	a := model.NewAccount(vo.NewId(), vo.NewId(), vo.NewId(), "Main", "wallet", now)
	later := now.Add(time.Hour)
	a.UpdateType(model.TypeCreditCard, later)
	if !a.UpdatedAt.Equal(now) {
		t.Fatalf("same type must not touch UpdatedAt")
	}
	a.UpdateType(model.TypeSavings, later)
	if a.Type != model.TypeSavings || !a.UpdatedAt.Equal(later) {
		t.Fatalf("got type %d updatedAt %v", a.Type, a.UpdatedAt)
	}
}

func TestAccountRequestTypeValidation(t *testing.T) {
	bad := 4
	create := model.CreateAccountRequest{Id: "x", Name: "Main", CurrencyId: "c", Icon: "wallet", Type: &bad}
	assertInvalidType(t, create.Validate())
	update := model.UpdateAccountRequest{Id: "x", Name: "Main", Icon: "wallet", UpdatedAt: "2026-09-01 00:00:00", Type: &bad}
	assertInvalidType(t, update.Validate())
	for _, ok := range []int{1, 2, 3} {
		v := ok
		create.Type, update.Type = &v, &v
		if err := create.Validate(); err != nil {
			t.Fatalf("create type %d: %v", ok, err)
		}
		if err := update.Validate(); err != nil {
			t.Fatalf("update type %d: %v", ok, err)
		}
	}
}

func assertInvalidType(t *testing.T, err error) {
	t.Helper()
	var v *errs.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("want validation error, got %v", err)
	}
	for _, f := range v.Fields {
		if f.Key == "type" && f.Code == errs.CodeAccountInvalidType {
			return
		}
	}
	t.Fatalf("no type/%s field error in %+v", errs.CodeAccountInvalidType, v.Fields)
}
```

Check the actual names first: `vo.NewId` (or whatever constructor the package exposes — grep `func New` in `internal/shared/vo`), and the validation error type/field names in `internal/shared/errs` (grep `type ValidationError`). Adjust the test to them; do not invent API.

- [ ] **Step 2: Run** `export PATH=/usr/local/go/bin:$PATH && go test ./internal/model/ -run 'AccountType|AccountUpdateType|AccountRequestType'` — expect compile failure.

- [ ] **Step 3: Implement.**
  - `account.go`: add `TypeSavings AccountType = 3` (comment: db/wire value 3; marks a savings account for budget savings planning), update the `AccountType` doc comment (CASH=1, CREDIT_CARD=2, SAVINGS=3; reads tolerate any stored value, `Valid` gates writes only), `Valid()` returns `t == TypeCash || t == TypeCreditCard || t == TypeSavings`, and add

    ```go
    func (a *Account) UpdateType(t AccountType, now time.Time) {
    	if a.Type != t {
    		a.Type = t
    		a.UpdatedAt = now
    	}
    }
    ```
  - `codes.go`: `CodeAccountInvalidType = "account.invalid_type"` next to the other `CodeAccount*`, and in `AllCodes`.
  - `account_dto.go`: add `Type *int \`json:"type"\`` to both requests (swag: document as optional int 1|2|3). In both `Validate()` methods, after the blank checks are collected (append to the same `fields` slice so both kinds report together):

    ```go
    if r.Type != nil && !AccountType(*r.Type).Valid() {
    	fields = append(fields, errs.FieldError{Key: "type", Message: "Account type must be 1, 2 or 3", Code: errs.CodeAccountInvalidType})
    }
    ```
    Guard the int16 conversion: values outside int16 range must also be invalid — check `*r.Type < 1 || *r.Type > 3` first, or compare as int.
  - `create.go`: after `model.NewAccount(...)`, `if req.Type != nil { acct.Type = model.AccountType(*req.Type) }` (construction time, no mutator needed; absent keeps `TypeCreditCard`).
  - `update.go`: next to `UpdateIcon`, `if req.Type != nil { acct.UpdateType(model.AccountType(*req.Type), now) }`.
  - `locales/*.json`: `errors.account.invalid_type` in all 11 (en: "Account type must be 1, 2 or 3"; translate the rest).
  - `internal/account/mcp/mcp.go`: `createAccountInput` gains `Type int \`json:"type,omitempty" jsonschema:"optional account type: 1 cash, 2 card (default), 3 savings (plannable in budgets)"\``; pass `&in.Type` into the request only when non-zero. (There is no `update_account` MCP tool; do not add one — note this in the commit message.)

- [ ] **Step 4: Repo round-trip test** (Review Focus 5 / spec "legacy values") in `internal/account/repo/repo_integration_test.go`, following the file's existing fixture style: insert an account, then raw-update its type with `db.Rebind("UPDATE accounts SET type = ? WHERE id = ?")` to `0`, load it with `GetByID`, assert `Type == model.AccountType(0)` and no error; then save it back unchanged via the repo and re-read: still `0`.

- [ ] **Step 5: API test**: create with `"type":3` → response `item.type == 3`; create without type → 2; update with `"type":1` → 1; update without type → unchanged; `"type":7` → 400 with `errors.type` and code `account.invalid_type`. Use the existing account api harness (read an existing `internal/account/api/*_test.go` for the pattern).

- [ ] **Step 6: Run** `go test ./internal/model/ ./internal/account/... ./internal/test/i18ntest/` — PASS. Then `make swagger` and `go test ./internal/test/apiparity/ ./internal/test/mcpparity/` — the MCP `tools/list` golden changes (new `type` property on `create_account`): regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`, confirm that is the ONLY diff.

- [ ] **Step 7: Commit** `feat(account): savings account type (accounts.type = 3)`.

---

### Task 2: Savings budget element — type, sync, ordering, move rules

**Files:**
- Modify: `internal/model/budget_valueobject.go`, `internal/model/budget_view.go` (`AccountView`), `internal/server/glue_budget.go`, `internal/budget/move.go`, `internal/shared/errs/codes.go`, `locales/*.json`
- Test: `internal/model/budget_side_test.go`, `internal/budget/savings_sync_test.go`, `internal/server/glue_budget_test.go`

**Interfaces:**
- Consumes: `model.TypeSavings` (Task 1)
- Produces: `model.ElementSavings ElementType = 5` (alias `"savings"`), `model.AccountView{ID, CurrencyID, OwnerID, IsDeleted, Name, Icon string, Type model.AccountType}`, `errs.CodeBudgetSavingsFolderNotAllowed = "budget.savings_folder_not_allowed"`, `func savingsFolderNotAllowedErr() error` in package budget.

- [ ] **Step 1: Failing value-object test** — extend `internal/model/budget_side_test.go`: `ElementSavings == 5`, `ElementSavings.Alias() == "savings"`, `ElementSavings.IsIncomeSide() == false` (add to the `TestIsIncomeSide` map), and `ElementTypeFromAlias("savings")` returns an error (wire input stays envelope/category/tag).

- [ ] **Step 2: Implement** in `budget_valueobject.go`: `ElementSavings ElementType = 5` in the const block (comment: one row per savings account member of the budget; external id = account id; persisted, frozen), add `ElementSavings: "savings"` to `elementAliases`. `ElementTypeFromAlias`'s `<= ElementTag` guard already excludes it. Run `go test ./internal/model/` — PASS.

- [ ] **Step 3: Extend `AccountView`** (`budget_view.go`) with `Name string`, `Icon string`, `Type AccountType`, and fill them in `BudgetAccountLookup.AccountsByIDs` (`glue_budget.go`) from the loaded `*model.Account`. Extend `glue_budget_test.go` to assert the three fields. Every other constructor of `model.AccountView` (grep the repo, including test fakes) keeps compiling because the fields are additive.

- [ ] **Step 4: Failing sync/move tests** — `internal/budget/savings_sync_test.go`. Build on the package's existing fake service wiring (read `internal/budget/merge_test.go` / `comments_test.go` / `builder_structure_test.go` to find the in-memory fakes and a helper that builds a `*Service`; if the fakes live in `internal/budget/api/harness_test.go` only, write these as API tests in `internal/budget/api/savings_test.go` against real SQLite instead — that harness exercises the real repos). Cases:
  1. Budget with member accounts A (type 2) and S (type 3, currency EUR, budget currency USD). Call `move-element` for any existing category (triggers sync). The elements table then holds exactly one `ElementSavings` row with `external_id = S`, `currency_id = EUR`, `folder_id` NULL, non-empty sort key; none for A.
  2. `set-limit` on S for the budget's start month succeeds (self-heal creates the row when absent — call it BEFORE any other write).
  3. Switch S to type 2 (account update), run any budget write → the savings row and its limit rows are gone (Review Focus 3).
  4. Remove S from the budget (`remove-budget-account`, while removable), run a write → row gone.
  5. `move-element {id: S, folderId: <existing folder>}` → 400, field `folderId`, code `budget.savings_folder_not_allowed` (Review Focus 4).
  6. Two savings accounts S1, S2: `move-element {id: S2, afterId: null}` puts S2 first among savings; the expense no-folder group's relative order (read the categories' positions from `get-budget` before and after) is unchanged.
  7. `move-element {id: S1, afterId: <an expense category id>}` → S1 placed as with an unknown anchor (appended to the end of the savings group), no error.
  8. `move-element {id: <category>, afterId: <S1>}` for a no-folder expense category → category appended at the end of its group (anchor from the other group is unknown), savings order untouched.
  9. A deleted savings member (deleted account, still a member) keeps its savings row after sync.

- [ ] **Step 5: Run** the new tests — FAIL.

- [ ] **Step 6: Implement in `move.go`.**
  - Error helper next to `folderSideMixedErr`:

    ```go
    func savingsFolderNotAllowedErr() error {
    	return errs.NewValidation("Validation failed", errs.FieldError{
    		Key: "folderId", Message: "Savings cannot be put into a folder", Code: errs.CodeBudgetSavingsFolderNotAllowed,
    	})
    }
    ```
    plus the code in `codes.go`/`AllCodes` and `errors.budget.savings_folder_not_allowed` in all 11 locales.
  - `MoveElement`: after resolving `moved`, before the `sideMixed` check: `if moved != nil && moved.Type == model.ElementSavings && folderID != nil { return nil, savingsFolderNotAllowedErr() }`. Pass the group to `groupElements`: `groupElements(b.elements, folderID, moved.ExternalID, moved.Type == model.ElementSavings)`.
  - `groupElements(elements, folderID, exclude, savings bool)`: skip `e` when `(e.Type == model.ElementSavings) != savings`. Update its doc comment: savings rows form their own no-folder group.
  - `syncElements`: after the tags block, before `assignMissingKeys`:

    ```go
    // --- savings accounts: one row per savings member, deleted members included
    // (their history still counts). The row is always live and folder-less; the
    // wire's isArchived comes from the account, never from this row.
    memberIDs := make([]vo.Id, 0, len(b.accounts))
    for _, m := range b.accounts {
    	memberIDs = append(memberIDs, m.AccountID)
    }
    views, err := s.accounts.AccountsByIDs(ctx, memberIDs)
    if err != nil {
    	return err
    }
    for i, v := range views {
    	if v.Type != model.TypeSavings {
    		continue
    	}
    	e, key := ensure(memberIDs[i], model.ElementSavings)
    	if _, isNew := created[key]; isNew && e.CurrencyID == nil {
    		cid, perr := vo.ParseId(v.CurrencyID)
    		if perr != nil {
    			return perr
    		}
    		e.UpdateCurrency(&cid, now)
    	}
    	if e.FolderID != nil {
    		e.UpdateFolder(nil, now)
    		mark(e)
    	}
    	live[key] = true
    }
    ```
    (`created` entries are saved anyway, so no `mark` is needed for the currency.) Update the `syncElements` doc bullet list to mention savings rows.
  - `assignMissingKeys`: keep one tail per no-folder group:

    ```go
    tails := map[bool]sortkey.Key{} // keyed by "is a savings row"
    for key, e := range byKey {
    	if !live[key] {
    		continue
    	}
    	if e.IsSortKeyUnset() {
    		needsKey = append(needsKey, e)
    		continue
    	}
    	g := e.Type == model.ElementSavings
    	if e.FolderID == nil && e.SortKey > tails[g] {
    		tails[g] = e.SortKey
    	}
    }
    // ... sort needsKey by ID as today, then per element:
    g := e.Type == model.ElementSavings
    tail := tails[g]
    // seed or Between(tail, "") exactly as today
    tails[g] = k
    ```
    Update its doc comment (two no-folder groups: savings and everything else).

- [ ] **Step 7: Run** `go test ./internal/model/ ./internal/budget/... ./internal/server/ ./internal/test/i18ntest/` — PASS. Run `go test ./internal/test/apiparity/ ./internal/test/mcpparity/` — must still pass with NO golden change (no scenario has a savings account yet). If a golden changed, stop and investigate.

- [ ] **Step 8: Commit** `feat(budget): savings element type, sync and ordering group`.

---

### Task 3: Read queries — `SavingsByMonth` and `AccountsNetByMonth`

**Files:**
- Modify: `internal/model/budget_view.go`, `internal/budget/readmodel.go`, `internal/budget/repo/read.go`, and every fake implementing `budget.ReadModel` (grep `ReadModel` / `TransfersByMonth(` in `_test.go` files and add stub methods)
- Test: `internal/budget/repo/read_savings_test.go`

**Interfaces:**
- Produces:

```go
// model (budget_view.go)
// SavingsMonthRow is one account's amount in one month ("YYYY-MM-01"), in the
// account's own currency.
type SavingsMonthRow struct {
	AccountID string
	Month     string
	Amount    string
}

// budget.ReadModel
// SavingsByMonth: per (savings account, month) net money moved in from the
// everyday accounts over [from, to): transfers everyday -> savings count their
// amount_recipient, savings -> everyday subtract their amount. Transfers with a
// non-member or another savings account on the other side are not rows.
SavingsByMonth(ctx context.Context, savingsIDs, everydayIDs []vo.Id, from, to time.Time) ([]model.SavingsMonthRow, error)
// AccountsNetByMonth: per (account, month) net change over [from, to) with
// balanceSQL's sign rules: +income, -expense, -transfer out (amount),
// +transfer in (amount_recipient), every counterparty.
AccountsNetByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time) ([]model.SavingsMonthRow, error)
```
Both return rows sorted by month, then account id; amounts are decimal strings.

- [ ] **Step 1: Failing repo test** — `internal/budget/repo/read_savings_test.go`, following `read_plan_test.go`'s setup (it seeds accounts and transactions on a `dbtest.New(t)` database — copy its helpers/fixtures, including how currencies and accounts are created). Seed: everyday E1 (USD), everyday E2 (USD), savings S1 (USD), savings S2 (EUR), non-member X (USD). Transactions (type 2 = transfer, `amount` in source currency, `amount_recipient` in recipient currency):
  - 2026-03-10 E1→S1 amount 500 / recipient 500 → S1 March +500
  - 2026-03-20 S1→E2 amount 120 / recipient 120 → S1 March −120
  - 2026-03-31 23:59:59 E1→S2 amount 110 / recipient 100 → S2 March +100 (EUR, uses amount_recipient)
  - 2026-04-01 00:00:00 E1→S1 200/200 → S1 April +200 (month boundary)
  - 2026-03-15 S1→S2 50/45 → ignored (savings↔savings)
  - 2026-03-16 X→S1 70/70 → ignored (non-member)
  - 2026-03-17 income type 1 on S1 amount 3 (interest) → ignored by SavingsByMonth, counted by AccountsNetByMonth
  - 2026-03-18 expense type 0 on S1 amount 1 (fee)

  Expect `SavingsByMonth([S1,S2],[E1,E2], 2026-03-01, 2026-05-01)` = `[{S1,2026-03-01,380},{S2,2026-03-01,100},{S1,2026-04-01,200}]` sorted by (month, account id) — compare with normalized decimals (`vo.NewDecimal(x).String()`), as the other read tests do, because sqlite returns `380.00000000` and pgsql `380.00`.
  Expect `AccountsNetByMonth([S1], 2026-03-01, 2026-05-01)` = S1 March: +500 −120 −50 +70 +3 −1 = 402; S1 April: 200.
  Also: empty `savingsIDs` → `nil, nil`; empty `everydayIDs` → `nil, nil`; empty `accountIDs` → `nil, nil`.

- [ ] **Step 2: Run** `go test ./internal/budget/repo/ -run Savings` — compile FAIL.

- [ ] **Step 3: Implement in `read.go`** after `transfersByMonthSQL`. Keep the engine split exactly like `TransfersByMonth`:

```go
// SavingsByMonth implements ReadModel: one grouped query per direction, merged
// per (account, month) as in - out.
func (r *ReadRepo) SavingsByMonth(ctx context.Context, savingsIDs, everydayIDs []vo.Id, from, to time.Time) ([]model.SavingsMonthRow, error) {
	if len(savingsIDs) == 0 || len(everydayIDs) == 0 {
		return nil, nil
	}
	merged := map[string]vo.DecimalNumber{}
	keys := map[string]model.SavingsMonthRow{}
	for _, out := range []bool{false, true} {
		sql, args := r.savingsByMonthSQL(out, savingsIDs, everydayIDs, from, to)
		rows, err := r.monthAccountAmounts(ctx, sql, args)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			k := row.Month + "|" + row.AccountID
			acc, ok := merged[k]
			if !ok {
				acc = vo.NewDecimal("0")
			}
			amt := vo.NewDecimal(row.Amount)
			if out {
				acc = acc.Sub(amt)
			} else {
				acc = acc.Add(amt)
			}
			merged[k] = acc
			keys[k] = model.SavingsMonthRow{AccountID: row.AccountID, Month: row.Month}
		}
	}
	return sortedSavingsRows(merged, keys), nil
}

// savingsByMonthSQL: out=false sums amount_recipient of everyday -> savings
// transfers (grouped by the recipient); out=true sums amount of savings ->
// everyday transfers (grouped by the source).
func (r *ReadRepo) savingsByMonthSQL(out bool, savingsIDs, everydayIDs []vo.Id, from, to time.Time) (string, []any) {
	amountCol, savingsCol, everydayCol := "t.amount_recipient", "t.account_recipient_id", "t.account_id"
	if out {
		amountCol, savingsCol, everydayCol = "t.amount", "t.account_id", "t.account_recipient_id"
	}
	s, e := idArgs(savingsIDs), idArgs(everydayIDs)
	args := append(append([]any{}, s...), e...)
	dStart, dEnd := "?", "?"
	if r.driver == "postgresql" {
		dStart = "$" + itoa(1+len(s)+len(e))
		dEnd = "$" + itoa(2+len(s)+len(e))
		args = append(args, from, to)
	} else {
		// See sqliteDatetime.
		args = append(args, sqliteDatetime(from), sqliteDatetime(to))
	}
	month := r.planMonthExpr("t.spent_at")
	sql := "SELECT " + month + " as month, " + savingsCol + " as account_id, SUM(" + amountCol + ") as amount FROM transactions t WHERE t.type = 2 AND " +
		savingsCol + " IN (" + r.ph(1, len(s)) + ") AND " + everydayCol + " IN (" + r.ph(1+len(s), len(e)) + ") AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd +
		" GROUP BY month, " + savingsCol
	return sql, args
}

// AccountsNetByMonth implements ReadModel. The sign rules are balanceSQL's,
// bucketed by month.
func (r *ReadRepo) AccountsNetByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time) ([]model.SavingsMonthRow, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	ids := idArgs(accountIDs)
	n := len(ids)
	month := r.planMonthExpr("t.spent_at")
	var args []any
	var d1, d2, d3, d4 string
	if r.driver == "postgresql" {
		args = append(append(append([]any{}, ids...), from, to), ids...)
		args = append(args, from, to)
		d1, d2 = "$"+itoa(n+1), "$"+itoa(n+2)
		d3, d4 = "$"+itoa(2*n+3), "$"+itoa(2*n+4)
	} else {
		f, tt := sqliteDatetime(from), sqliteDatetime(to)
		args = append(append(append([]any{}, ids...), f, tt), ids...)
		args = append(args, f, tt)
		d1, d2, d3, d4 = "?", "?", "?", "?"
	}
	sql := "SELECT month, account_id, SUM(amount) as amount FROM (" +
		"SELECT " + month + " as month, t.account_id as account_id, CASE WHEN t.type = 1 THEN t.amount ELSE 0 - t.amount END as amount FROM transactions t WHERE t.account_id IN (" + r.ph(1, n) + ") AND t.spent_at >= " + d1 + " AND t.spent_at < " + d2 +
		" UNION ALL " +
		"SELECT " + month + " as month, t.account_recipient_id as account_id, t.amount_recipient as amount FROM transactions t WHERE t.type = 2 AND t.account_recipient_id IN (" + r.ph(n+3, n) + ") AND t.spent_at >= " + d3 + " AND t.spent_at < " + d4 +
		") x GROUP BY month, account_id"
	rows, err := r.monthAccountAmounts(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	merged := map[string]vo.DecimalNumber{}
	keys := map[string]model.SavingsMonthRow{}
	for _, row := range rows {
		k := row.Month + "|" + row.AccountID
		merged[k] = vo.NewDecimal(row.Amount)
		keys[k] = model.SavingsMonthRow{AccountID: row.AccountID, Month: row.Month}
	}
	return sortedSavingsRows(merged, keys), nil
}

// monthAccountAmounts scans (month, account_id, amount) rows with the
// engine's SUM representation.
func (r *ReadRepo) monthAccountAmounts(ctx context.Context, sql string, args []any) ([]model.SavingsMonthRow, error) {
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SavingsMonthRow
	for rows.Next() {
		var row model.SavingsMonthRow
		a := "0"
		if r.driver == "postgresql" {
			var amount *string
			if err := rows.Scan(&row.Month, &row.AccountID, &amount); err != nil {
				return nil, err
			}
			if amount != nil {
				a = *amount
			}
		} else {
			var amount *float64
			if err := rows.Scan(&row.Month, &row.AccountID, &amount); err != nil {
				return nil, err
			}
			if amount != nil {
				a = strconv.FormatFloat(*amount, 'f', 8, 64)
			}
		}
		row.Amount = a
		out = append(out, row)
	}
	return out, rows.Err()
}

func sortedSavingsRows(merged map[string]vo.DecimalNumber, keys map[string]model.SavingsMonthRow) []model.SavingsMonthRow {
	out := make([]model.SavingsMonthRow, 0, len(merged))
	for k, amt := range merged {
		row := keys[k]
		row.Amount = amt.String()
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month < out[j].Month
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}
```
Placeholder numbering check (pgsql, `AccountsNetByMonth`): `$1..$n` ids, `$n+1,$n+2` dates, `$n+3..$2n+2` ids, `$2n+3,$2n+4` dates — `r.ph(n+3, n)` starts at `$n+3`. Verify `r.ph`'s `start` is honored for pgsql (it is: `"$" + itoa(start+i)`).
`0 - t.amount` works on SQLite TEXT amounts (numeric affinity) and pgsql NUMERIC.
Add both methods to `ReadModel` in `readmodel.go` with the doc comments from the Interfaces block, and stub them in every test fake of `ReadModel` (return `nil, nil` unless the fake is used by Task 4/5 tests — those get real fake data there).

- [ ] **Step 4: Run** `go test ./internal/budget/...` — PASS. Then against PostgreSQL: `make test-repo-pgsql` (or, per the memory note, a throwaway pgsql via `DBTEST_ENGINE=pgsql DATABASE_TEST_PGSQL_URL=… go test -tags enginecompare ./internal/budget/repo/ -run Savings`). Both engines must pass.

- [ ] **Step 5: Commit** `feat(budget): savings and account-net by-month read queries`.

---

### Task 4: Monthly `get-budget` — `structure.savings`

**Files:**
- Modify: `internal/model/budget_dto.go`, `internal/budget/builder.go`, `internal/budget/builder_structure_build.go`; any other place that constructs `model.StructureResult` (grep `StructureResult{`) must set `Savings: []model.SavingsElementResult{}`
- Test: `internal/budget/api/savings_test.go` (real SQLite through the feature harness)
- Goldens: `internal/test/apiparity/testdata/golden/*` and mcpparity goldens (every get-budget response gains `"savings": []`)

**Interfaces:**
- Consumes: `SavingsByMonth` (Task 3), `ElementSavings`, `AccountView.{Name,Icon,Type}` (Task 2)
- Produces:

```go
// SavingsElementResult is one savings account's row in get-budget's
// structure.savings. Kept out of elements so older clients do not sum it into
// expenses. Amounts are in CurrencyId (the element currency); there is no
// carry-over: Available = Budgeted - Spent for this month only.
type SavingsElementResult struct {
	Id          string `json:"id"`
	Type        int    `json:"type"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	CurrencyId  string `json:"currencyId"`
	OwnerUserId string `json:"ownerUserId"`
	IsArchived  int    `json:"isArchived"`
	Position    int    `json:"position"`
	Budgeted    string `json:"budgeted"`
	Spent       string `json:"spent"`
	Available   string `json:"available"`
}
// StructureResult gains: Savings []SavingsElementResult `json:"savings"`
```
- filters gains `savingsAccounts []model.AccountView` (member accounts with `Type == model.TypeSavings`, in membership order) and `everydayAccountIDs []vo.Id` (every other member). Task 5 uses both.

- [ ] **Step 1: Failing API test** `internal/budget/api/savings_test.go` (use `harness_test.go`'s helpers; read `two_currency_test.go` for multi-currency + rates setup). Budget in USD; everyday E (USD); savings S1 (USD); savings S2 (EUR) with an EUR→USD rate for the month; all members. Transactions in the viewed month: E→S1 500, S1→E 200, E→S2 110 USD / 100 EUR. `set-limit` S1 = 400 and S2 = 50 for the month. `get-budget` for the month:
  - `structure.savings` has 2 rows, ordered by position, `type == 5`, `name`/`icon` from the accounts, `ownerUserId` = the account owner, `isArchived == 0`.
  - S1: `budgeted 400`, `spent 300`, `available 100`, `currencyId` USD.
  - S2: `currencyId` EUR (element currency defaults to the account's), `spent 100` (EUR, not the 110 USD), `budgeted 50`, `available -50`.
  - `change-element-currency` S2 → USD; re-read: S2 `currencyId` USD and `spent` = 100 EUR converted with the month's rate (Review Focus 1). Assert against the value computed from the seeded rate, not a literal guess.
  - `structure.elements` contains NO element with id S1/S2, and the balances/elements are unchanged versus the same budget without the savings accounts' `type = 3` (compute by flipping S1/S2 to type 2 and re-reading: `structure.elements` and `balances` byte-identical).
  - Stale row (Review Focus 3): flip S1 to type 2 WITHOUT any budget write → `structure.savings` no longer lists S1.
  - Deleted account (Review Focus 5): delete S1's plan (`set-limit` amount null), then delete the S1 account, view a month with no S1 activity → S1 absent; view the month with the transfers → S1 present with `isArchived == 1`.
  - Budget with no savings members → `"savings":[]` in the raw JSON (assert the bytes contain `"savings":[]`, not `null`).

- [ ] **Step 2: Run** — FAIL.

- [ ] **Step 3: Implement.**
  - DTO + `Savings` field (`budget_dto.go`); swag comment.
  - `buildFilters` (`builder.go`): while walking `views`, split: `if v.Type == model.TypeSavings { savings = append(savings, v) } else { everyday = append(everyday, memberIDs[i]) }`; store in the new filter fields.
  - `builder_structure_build.go`: inside `buildStructure`, BEFORE the single `BulkConvert` call, accumulate savings convert items into the same `toConvert` map; AFTER it, emit. Put the two halves in helpers in a new file `internal/budget/builder_savings.go` so `buildStructure` only gains two calls:

```go
// savingsRow is one savings account's in-progress row, shared by the monthly
// and plan builders.
type savingsRow struct {
	account    model.AccountView
	currencyID vo.Id // element currency
	sortKey    sortkey.Key
}

// savingsRows resolves each savings member's element currency (the element
// row's, else the account's own) and sort key. Rows derive from the CURRENT
// savings accounts, not from the element table, because sync is lazy: a stale
// element row whose account is no longer savings must not render.
func savingsRows(f filters, options map[string]elementOption) ([]savingsRow, error) {
	out := make([]savingsRow, 0, len(f.savingsAccounts))
	for _, a := range f.savingsAccounts {
		opt := options[elementKey(a.ID, model.ElementSavings)]
		cur := opt.currencyID
		if cur == nil {
			cid, err := vo.ParseId(a.CurrencyID)
			if err != nil {
				return nil, err
			}
			cur = &cid
		}
		out = append(out, savingsRow{account: a, currencyID: *cur, sortKey: opt.sortKey})
	}
	// Keyless rows (not synced yet) trail, by account id.
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := out[i].sortKey, out[j].sortKey
		if (ki == "") != (kj == "") {
			return kj == ""
		}
		if ki != kj {
			return ki < kj
		}
		return out[i].account.ID < out[j].account.ID
	})
	return out, nil
}

func savingsSpentKey(accountID string) string { return "savings-spent_" + accountID }

func savingsAccountIDs(f filters) []vo.Id { /* parse f.savingsAccounts ids */ }
```
    Monthly half (called from `buildStructure` with `options`, `limits`, `toConvert`):

```go
func (s *Service) addMonthlySavings(ctx context.Context, f filters, options map[string]elementOption, toConvert map[string][]model.ConvertItem) ([]savingsRow, error) {
	rows, err := savingsRows(f, options)
	if err != nil || len(rows) == 0 {
		return rows, err
	}
	ids, err := savingsAccountIDs(f)
	if err != nil {
		return nil, err
	}
	actual, err := s.read.SavingsByMonth(ctx, ids, f.everydayAccountIDs, f.periodStart, f.periodEnd)
	if err != nil {
		return nil, err
	}
	byID := map[string]savingsRow{}
	for _, r := range rows {
		byID[r.account.ID] = r
	}
	for _, a := range actual {
		r, ok := byID[a.AccountID]
		if !ok {
			continue
		}
		from, perr := vo.ParseId(r.account.CurrencyID)
		if perr != nil {
			return nil, perr
		}
		toConvert[savingsSpentKey(a.AccountID)] = append(toConvert[savingsSpentKey(a.AccountID)], model.ConvertItem{
			PeriodStart: f.periodStart, PeriodEnd: f.periodEnd, From: from, To: r.currencyID, Amount: vo.NewDecimal(a.Amount),
		})
	}
	return rows, nil
}

func emitMonthlySavings(rows []savingsRow, limits map[string]budgetedAmount, get func(string) vo.DecimalNumber) []model.SavingsElementResult {
	zero := vo.NewDecimal("0")
	out := []model.SavingsElementResult{}
	for _, r := range rows {
		budgeted := orZero(limits[elementKey(r.account.ID, model.ElementSavings)].budgeted, zero)
		spent := get(savingsSpentKey(r.account.ID))
		// A deleted account stays only while it still carries a plan or activity.
		if r.account.IsDeleted && budgeted.IsZero() && spent.IsZero() {
			continue
		}
		out = append(out, model.SavingsElementResult{
			Id: r.account.ID, Type: int(model.ElementSavings.Int16()), Name: r.account.Name, Icon: r.account.Icon,
			CurrencyId: r.currencyID.String(), OwnerUserId: r.account.OwnerID, IsArchived: boolToInt(r.account.IsDeleted),
			Position: len(out), Budgeted: budgeted.String(), Spent: spent.String(), Available: budgeted.Sub(spent).String(),
		})
	}
	return out
}
```
    In `buildStructure`: `savings, err := s.addMonthlySavings(ctx, f, options, toConvert)` right before `s.convertor.BulkConvert(...)`; after `get` is defined, `Savings: emitMonthlySavings(savings, limits, get)` in the returned `StructureResult`. `orZero` handles a zero-value `DecimalNumber` (empty string) exactly as the other elements do.
    Note: `buildElementsLimits` already keys savings limits as `"<accountId>-savings"` generically — verify with the test, do not special-case it.

- [ ] **Step 4: Run** the new test + `go test ./internal/budget/...` — PASS.

- [ ] **Step 5: Goldens.** `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`, then `git diff --stat internal/test/` and read the diffs: every changed golden must differ ONLY by an added `"savings": []` inside `structure` of get-budget responses (and the MCP get_budget equivalents). Anything else → investigate before committing. `make swagger`.

- [ ] **Step 6: Commit** `feat(budget): savings rows in get-budget`.

---

### Task 5: Plan `get-budget-plan` — savings rows, opening balances, flows

**Files:**
- Modify: `internal/model/budget_dto.go`, `internal/budget/builder_plan.go`, `internal/budget/builder_savings.go`
- Test: `internal/budget/api/savings_test.go` (extend), `internal/budget/api/clone_test.go` (extend)
- Goldens: apiparity/mcpparity (get-budget-plan responses gain three fields)

**Interfaces:**
- Consumes: Task 3 queries, Task 4 `savingsRows`, `savingsAccountIDs`, filters fields
- Produces:

```go
// PlanSavingsElementResult is one savings account's plan row. Cells align with
// BudgetPlanResult.Months; Actual is money moved in from the everyday accounts,
// in CurrencyId (the element currency); Planned is "" with no limit.
type PlanSavingsElementResult struct {
	Id          string           `json:"id"`
	Type        int              `json:"type"`
	Name        string           `json:"name"`
	Icon        string           `json:"icon"`
	CurrencyId  string           `json:"currencyId"`
	OwnerUserId string           `json:"ownerUserId"`
	IsArchived  int              `json:"isArchived"`
	Position    int              `json:"position"`
	Cells       []PlanCellResult `json:"cells"`
}

// PlanSavingsFlowResult is one (month, account currency) net change of the
// savings accounts: every transaction on them, interest and boundary transfers
// included, so it deliberately exceeds the Savings row, which counts only what
// was moved in from the everyday accounts.
type PlanSavingsFlowResult struct {
	Month      string `json:"month"`
	CurrencyId string `json:"currencyId"`
	Amount     string `json:"amount"`
}
// PlanStructureResult gains:  Savings []PlanSavingsElementResult `json:"savings"`
// BudgetPlanResult gains:     SavingsOpeningBalances []OpeningBalanceResult `json:"savingsOpeningBalances"`
//                             SavingsFlows []PlanSavingsFlowResult `json:"savingsFlows"`
```

- [ ] **Step 1: Failing API tests** (same fixture as Task 4, window of 3 months starting the month before the transfers):
  - `structure.savings`: S1 cells — month0 `actual 0 planned ""`, month1 `actual 300 planned 400`, month2 `actual 0 planned ""`; S2 month1 `actual 100` (EUR element currency).
  - With S2's element currency changed to USD: S2 month1 actual = 100 EUR converted with month1's rate.
  - `savingsOpeningBalances`: seed S1 with 1000 USD income dated before the window → `[{USD, 1000}, {EUR, 0}]` (budget currency first, then discovery order of savings accounts' currencies; only currencies of savings accounts).
  - `savingsFlows`: add interest income 3 on S1 in month1 → `[{month1, USD, 303}, {month1, EUR, 100}]` ordered by month, budget currency first then currency id. Months with no activity have no entry. (The Savings row says 300 while flows say 303 — the intended divergence.)
  - `structure.elements` unchanged versus the same budget with S1/S2 as type 2 (the E→S transfers stay internal, so Transfers/elements do not move).
  - Deleted S1 with neither plan nor actual in the window → absent; with a plan in the window → present, `isArchived 1`.
  - No savings members → `"savings":[]`, `"savingsOpeningBalances":[]`, `"savingsFlows":[]` in the raw bytes.
  - `clone_test.go`: clone with limits → the copy's `get-budget-plan` lists S1 with the same planned cells; clone without limits → S1 listed, planned `""`.

- [ ] **Step 2: Run** — FAIL.

- [ ] **Step 3: Implement.**
  - DTOs as above.
  - In `buildPlanStructure`, before the `BulkConvert` call: build `rows, err := savingsRows(f, options)`; when non-empty call `s.read.SavingsByMonth(ctx, ids, f.everydayAccountIDs, monthsList[0], windowEnd)` and, for each result row in the window, append a `ConvertItem{PeriodStart: monthsList[i], PeriodEnd: monthsList[i].AddDate(0,1,0), From: <account currency>, To: row.currencyID, Amount}` under `planKey(i, elementKey(accountID, model.ElementSavings))`, recording `hasActual[accountID] = true`. After the conversion, emit:

```go
func emitPlanSavings(rows []savingsRow, plannedFor func(string) []string, hasActual map[string]bool, get func(string) vo.DecimalNumber, nMonths int) []model.PlanSavingsElementResult {
	out := []model.PlanSavingsElementResult{}
	for _, r := range rows {
		index := elementKey(r.account.ID, model.ElementSavings)
		planned := plannedFor(index)
		hasPlan := false
		for _, p := range planned {
			if p != "" {
				hasPlan = true
			}
		}
		if r.account.IsDeleted && !hasPlan && !hasActual[r.account.ID] {
			continue
		}
		cells := make([]model.PlanCellResult, nMonths)
		for i := range cells {
			cells[i] = model.PlanCellResult{Actual: get(planKey(i, index)).String(), Planned: planned[i]}
		}
		out = append(out, model.PlanSavingsElementResult{
			Id: r.account.ID, Type: int(model.ElementSavings.Int16()), Name: r.account.Name, Icon: r.account.Icon,
			CurrencyId: r.currencyID.String(), OwnerUserId: r.account.OwnerID, IsArchived: boolToInt(r.account.IsDeleted),
			Position: len(out), Cells: cells,
		})
	}
	return out
}
```
    `emitPlanElements` builds its own `get`; construct an equivalent one over `converted` for savings. Return `PlanStructureResult{Folders, Elements, Savings}`.
  - In `BuildBudgetPlan`: `savingsOpening, err := s.buildSavingsOpeningBalances(ctx, b.budget.CurrencyID, f, from)` and `savingsFlows, err := s.buildSavingsFlows(ctx, b.budget.CurrencyID, f, from, windowEnd)`, both in `builder_savings.go`:

```go
// buildSavingsOpeningBalances is buildOpeningBalances over the savings
// accounts only (strictly before the window), per savings-account currency,
// budget currency first then discovery order.
func (s *Service) buildSavingsOpeningBalances(ctx context.Context, budgetCurrencyID vo.Id, f filters, from time.Time) ([]model.OpeningBalanceResult, error) {
	out := []model.OpeningBalanceResult{}
	ids, err := savingsAccountIDs(f)
	if err != nil || len(ids) == 0 {
		return out, err
	}
	rows, err := s.read.AccountsBalancesBeforeDate(ctx, ids, from)
	if err != nil {
		return nil, err
	}
	for _, cid := range savingsCurrencies(f, budgetCurrencyID) {
		out = append(out, model.OpeningBalanceResult{CurrencyId: cid, Amount: sumBalances(rows, cid).String()})
	}
	return out, nil
}

// savingsCurrencies: the savings accounts' currencies, budget currency first
// (only if a savings account holds it), then discovery order.
func savingsCurrencies(f filters, budgetCurrencyID vo.Id) []string { /* dedupe f.savingsAccounts[i].CurrencyID */ }

// buildSavingsFlows sums AccountsNetByMonth per (month, account currency).
func (s *Service) buildSavingsFlows(ctx context.Context, budgetCurrencyID vo.Id, f filters, from, to time.Time) ([]model.PlanSavingsFlowResult, error) {
	out := []model.PlanSavingsFlowResult{}
	ids, err := savingsAccountIDs(f)
	if err != nil || len(ids) == 0 {
		return out, err
	}
	rows, err := s.read.AccountsNetByMonth(ctx, ids, from, to)
	if err != nil {
		return nil, err
	}
	currencyOf := map[string]string{}
	for _, a := range f.savingsAccounts {
		currencyOf[a.ID] = a.CurrencyID
	}
	sums := map[[2]string]vo.DecimalNumber{}
	for _, r := range rows {
		k := [2]string{r.Month, currencyOf[r.AccountID]}
		acc, ok := sums[k]
		if !ok {
			acc = vo.NewDecimal("0")
		}
		sums[k] = acc.Add(vo.NewDecimal(r.Amount))
	}
	budgetCur := budgetCurrencyID.String()
	for k, v := range sums {
		out = append(out, model.PlanSavingsFlowResult{Month: k[0], CurrencyId: k[1], Amount: v.String()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month < out[j].Month
		}
		if (out[i].CurrencyId == budgetCur) != (out[j].CurrencyId == budgetCur) {
			return out[i].CurrencyId == budgetCur
		}
		return out[i].CurrencyId < out[j].CurrencyId
	})
	return out, nil
}
```
    `Month` strings come from `planMonthExpr` (`YYYY-MM-01`), matching `Months`.
  - Set the two new fields on `BudgetPlanResult`.

- [ ] **Step 4: Run** new tests + `go test ./internal/budget/...` — PASS.

- [ ] **Step 5: Goldens.** Regenerate apiparity + mcpparity; every changed get-budget-plan golden must differ ONLY by `"savings": []` in `structure`, and `"savingsOpeningBalances": []`, `"savingsFlows": []` at the top level. `make swagger`.

- [ ] **Step 6: Commit** `feat(budget): savings rows, opening balances and flows in get-budget-plan`.

---

### Task 6: MCP descriptions, parity scenario, both engines

**Files:**
- Modify: `internal/budget/mcp/mcp.go`, `internal/test/apiparity/catalogue.go` (register), `internal/test/apiparity/catalogue_test.go` (floor)
- Create: `internal/test/apiparity/catalogue_budget_savings.go`
- Goldens: the new scenario's golden files

- [ ] **Step 1: MCP descriptions.** `set_limit`: append "Savings rows (structure.savings) take limits too: element_id is the savings account id." `move_element`: append "A savings row (id = savings account id) reorders only among savings rows and cannot be put into a folder." Regenerate mcpparity goldens; the only diff is those two descriptions in `tools/list`.

- [ ] **Step 2: apiparity scenario** `budget_savings` modelled on `catalogue_budget_comments.go` / `catalogue_budget_income.go` (read one first for the scenario/step DSL). Steps: create everyday + savings accounts (`create-account` with `"type":3`, and one `update-account` flipping a second account to 3), create a budget with both as members, `set-limit` on the savings account, create an everyday→savings transfer, `get-budget`, `get-budget-plan`, `move-element` of the savings row with a `folderId` (expect the 400), `move-element` reorder with `folderId: null`, `create-account` with `"type":9` (expect the 400 `account.invalid_type`). Register it; raise `min` in `catalogue_test.go` by one from its CURRENT value (and do not touch `minRoutes` — no new route).

- [ ] **Step 3: Generate and inspect** `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`; read every new golden: savings rows present with the expected numbers, `elements` free of the savings account, error envelopes coded correctly.

- [ ] **Step 4: Both engines.** `make test-repo-pgsql` and the enginecompare suite (`make test` runs both, plus the frontend; or run the enginecompare target alone — see the Makefile). Both must pass: SQLite and PostgreSQL responses byte-identical for the new scenario.

- [ ] **Step 5: `make go-test`** — PASS including the 80% coverage gate.

- [ ] **Step 6: Commit** `test(budget): savings parity scenario; MCP descriptions`.

---

### Task 7: SPA — account type, savings switch, confirmation, markers, metric

**Files:**
- Modify: `web/src/api/dto/account.ts`, `web/src/api/account.ts` (create/update payload types), `web/src/api/dto/budget.ts`, `web/src/features/accounts/{AccountDialog.tsx,queries.ts,SidebarAccountTree.tsx,AccountsSettingsPage.tsx}`, `web/src/lib/metrics.ts`, `locales/*.json`
- Create: `web/src/features/accounts/savingsPlans.ts`, `web/src/features/accounts/savingsPlans.test.ts`
- Test: `web/src/features/accounts/AccountDialog.test.tsx`, `queries.test.tsx`

**Interfaces:**
- Produces (TS):

```ts
// dto/account.ts
export const AccountType = { CASH: 1, CREDIT_CARD: 2, SAVINGS: 3 } as const
export const isSavingsAccount = (a: { type: number }): boolean => a.type === AccountType.SAVINGS

// dto/budget.ts
export const BudgetElementType = { ENVELOPE: 0, CATEGORY: 1, TAG: 2, INCOME_CATEGORY: 3, INCOME_ENVELOPE: 4, SAVINGS: 5 } as const
export interface BudgetSavingsElementDto {
  id: Id; type: BudgetElementType; name: string; icon: string; currencyId: Id; ownerUserId: Id
  isArchived: 0 | 1; position: number; budgeted: string; spent: string; available: string
}
export interface PlanSavingsElementDto {
  id: Id; type: BudgetElementType; name: string; icon: string; currencyId: Id; ownerUserId: Id
  isArchived: 0 | 1; position: number; cells: PlanCellDto[]
}
export interface PlanSavingsFlowDto { month: string; currencyId: Id; amount: string }
// BudgetDto.structure gains  savings?: BudgetSavingsElementDto[]
// BudgetPlanDto.structure gains savings?: PlanSavingsElementDto[]
// BudgetPlanDto gains savingsOpeningBalances?: OpeningBalanceDto-shaped[]; savingsFlows?: PlanSavingsFlowDto[]
```
Mark the new fields optional (`?`) and read them with `?? []` everywhere: cached/persisted query data from an older server may lack them.

```ts
// features/accounts/savingsPlans.ts
/** How many budgets the SPA already holds in its query cache plan savings for
 *  this account: a monthly budget with a non-zero `budgeted` savings row, or a
 *  plan window with any non-empty `planned` savings cell. Counts distinct
 *  budget ids. Budgets the user cannot see are not counted — the confirmation
 *  wording ("will be removed") does not promise a precise total. */
export function countBudgetsPlanningSavings(queryClient: QueryClient, accountId: Id): number
```
- `METRICS.ACCOUNT_SAVINGS_TOGGLE = 'appAccountSavingsToggle'`.

- [ ] **Step 1: Failing tests.**
  - `savingsPlans.test.ts`: seed a `QueryClient` with a budget detail (use the real query keys from `features/budgets/queries.ts` — read how `useBudget` and `useBudgetPlan` key their data) containing savings rows for account A with `budgeted "100"` and for B with `budgeted "0"`, and a plan cache for another budget with an A cell `planned "50"`; expect `countBudgetsPlanningSavings(qc, A) === 2`, `B → 0`, unknown → 0.
  - `AccountDialog.test.tsx`: (a) create with the "Savings account" switch on sends `type: 3`, off sends `type: 2`; (b) edit a type-1 account, switch on → `type: 3`; edit a savings account, switch off → previous type unknown so `2`… except an account whose type is 1 or 2 keeps it — test: type-1 account round trip (on then off before saving) sends `1`; (c) switching a savings account off when `countBudgetsPlanningSavings > 0` opens a confirmation with the n-budgets text; cancelling sends nothing, confirming sends `type: 2`; (d) with no budget planning it, no confirmation appears and the update is sent directly.
  - `queries.test.tsx`: the metric fires once on creating a savings account, once on an update that flips the savings state, and not on an update that keeps it (mock `trackEvent` the way existing account metric tests do).

- [ ] **Step 2: Run** `cd web && pnpm exec vitest run src/features/accounts` — FAIL.

- [ ] **Step 3: Implement.**
  - DTO changes above; create/update payload types gain optional `type?: AccountType`.
  - `AccountDialog.tsx`: a labelled `Switch` (use the shadcn `Switch` already in `components/ui` — grep for an existing usage) "Savings account" with hint text; state initialised from `isSavingsAccount(account)`. Payload: create → `type: savings ? 3 : 2`; update → `savings ? 3 : (account.type === 1 || account.type === 2 ? account.type : 2)`. On submit of an update that turns savings off, compute `n = countBudgetsPlanningSavings(queryClient, account.id)`; when `n > 0` show a confirmation (reuse the app's existing confirm dialog component — grep `ConfirmDialog` / `AlertDialog` in `web/src/components`) with `pluralPick(t('accounts.dialog.savings.confirm_off'), n, lang)` ("Planned savings for this account will be removed from {n} budget | budgets. Saved amounts and transactions are not affected.") and only then send.
  - `queries.ts` (accounts): in the create mutation's `onSuccess`, fire `trackEvent(METRICS.ACCOUNT_SAVINGS_TOGGLE)` when `data.item.type === 3`. In the update mutation, capture the account's previous type from the accounts cache in `onMutate` (return it in the context) and fire in `onSuccess` when `(prev === 3) !== (data.item.type === 3)`.
  - Marker: a small muted "Savings" label (`t('accounts.savings.marker')`) next to savings accounts in `SidebarAccountTree.tsx` and `AccountsSettingsPage.tsx`.
  - i18n (all 11 catalogues, real translations): `accounts.dialog.savings.label`, `accounts.dialog.savings.hint` ("Plan how much goes into this account in your budgets"), `accounts.dialog.savings.confirm_title`, `accounts.dialog.savings.confirm_off` (pipe-plural form per the language's plural rules — see `web/src/lib/plural.ts`), `accounts.dialog.savings.confirm_action`, `accounts.savings.marker`. Match the existing key naming in `locales/en.json` under the accounts namespace (read it first and adapt the key paths to its actual structure).
  - `metrics.ts`: `ACCOUNT_SAVINGS_TOGGLE: 'appAccountSavingsToggle'`.

- [ ] **Step 4: Run** the account tests, `pnpm exec vitest run src/lib/metrics-coverage.test.ts`, `pnpm exec tsc -b`, `pnpm lint`, and `export PATH=/usr/local/go/bin:$PATH && go test ./internal/test/i18ntest/` — PASS.

- [ ] **Step 5: Commit** `feat(web): savings account switch with plan-removal confirmation`.

---

### Task 8: SPA — plan math (Savings, Net, balance split)

**Files:**
- Modify: `web/src/features/budgets/planMath.ts`
- Test: `web/src/features/budgets/planMath.test.ts`

**Interfaces:**
- Consumes: `PlanSavingsElementDto`, `PlanSavingsFlowDto` (Task 7)
- Produces:

```ts
// PlanMonthTotals gains:
savingsActual: string    // Σ savings actual, budget currency
savingsPlanned: string   // Σ non-archived savings planned
effectiveSavings: string // past: actual; current/future: per-row max(actual, planned) (archived rows: actual)
// netActual  = incomeActual − expenseActual + transfersNet − savingsActual
// netPlanned = incomePlanned − expensePlanned − savingsPlanned
// effectiveNet is UNCHANGED: it is the COMBINED balance's per-month contribution,
// and an everyday→savings transfer moves nothing between the combined total.

export function savingsBalanceRow(plan: BudgetPlanDto, totals: PlanMonthTotals[], ex: MonthExchange, now?: Date): string[]
// running = Σ savingsOpeningBalances (month-0 rate), then per month i:
//   month <  cur: + flows(i)
//   month == cur: + flows(i) + max(0, savingsPlanned_i − savingsActual_i)
//   month >  cur: + savingsPlanned_i
// flows(i) = Σ ex(f.currencyId, f.amount, i) over savingsFlows with f.month === plan.months[i]

export function everydayBalanceRow(combined: string[], savings: string[]): string[] // combined[i] − savings[i]
```

- [ ] **Step 1: Failing tests** in `planMath.test.ts`, reusing the file's plan-fixture builder (read the top of the test file). A 3-month plan (past, current, future via the `now` parameter), budget USD, one expense element, one savings row in EUR with a EUR rate per month:
  - `planTotals`: savingsActual converts EUR→USD per month; savingsPlanned skips an archived savings row; effectiveSavings past = actual even when planned is higher, current = max, future = planned when actual 0.
  - `netActual` / `netPlanned` subtract savings; `effectiveNet` equals the value computed without any savings rows (assert by running `planTotals` on the same plan with `structure.savings` emptied).
  - `savingsBalanceRow`: opening 1000 USD + flows 303 in the past month + (flows 0 + max(0, 400−300)) current + 400 future.
  - `everydayBalanceRow(balanceRow(...), savingsBalanceRow(...))` sums back to `balanceRow` exactly per month.
  - A plan without `savings`/`savingsFlows`/`savingsOpeningBalances` fields (older server) → savings totals `'0'`, savings balance all `'0'`.

- [ ] **Step 2: Run** `pnpm exec vitest run src/features/budgets/planMath.test.ts` — FAIL.

- [ ] **Step 3: Implement** in `planTotals` (a second loop over `plan.structure.savings ?? []`, same `ex`/`effective` rules as the expense loop), the two new exported functions, and keep `balanceRow` unchanged.

- [ ] **Step 4: Run** — PASS; `pnpm exec tsc -b`.

- [ ] **Step 5: Commit** `feat(web): plan math for savings, net and the balance split`.

---

### Task 9: SPA — plan view Savings section

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`, `web/src/features/budgets/queries.ts` (`usePlanSetLimit`, `useFillPlannedCells` optimistic patches), `locales/*.json`
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (or a new `PlanSheet.savings.test.tsx` using the same render helpers)

**Interfaces:**
- Consumes: Task 7 DTOs, Task 8 math
- Produces: `savingsAsPlanElement(s: PlanSavingsElementDto): PlanElementDto` (in `planMath.ts` or PlanSheet): `{ ...s, folderId: null, children: [], ownerUserId: s.ownerUserId }` — the adapter that lets `ElementRow`, the limit editor, fill, keyboard navigation and #246's comment marker work on savings rows unchanged.

- [ ] **Step 1: Failing tests** (reuse the existing PlanSheet test harness/mocks):
  1. A plan with two savings rows renders `data-testid="plan-section-savings"` AFTER `plan-section-expense` and BEFORE `plan-section-archived`, with both rows in `position` order.
  2. Folding the section (header toggle) hides the rows and persists `planFolds.savings`; keyboard ArrowDown from the last expense row lands on the first savings row, and from the last savings row on the first archived row.
  3. Editing a savings planned cell calls set-limit with `elementId = <account id>` and the month; the optimistic update patches `structure.savings` (the new value shows before the response).
  4. The row menu of a savings row has no "Move to folder…" item.
  5. Edit mode: the savings rows are sortable within their own band; dropping S2 above S1 calls move-element with `{id: S2, folderId: null, afterId: null}`; there is no droppable folder in the savings band.
  6. Totals: a "Savings" totals row below Transfers shows `effectiveSavings`; the sticky balance area shows "Balance" = everyday balance and "Savings balance" = savings balance row, the latter with the tooltip text "Includes interest and other activity on savings accounts, which is not counted as saved".
  7. A savings cell with comments shows the corner marker, and Shift+Enter / the popover footer opens the thread for `commentCellKey(<account id>, month)` (#246 seam).
  8. A deleted-account savings row (`isArchived: 1`) renders read-only (no editable cell), not draggable.
  9. No savings rows → no Savings section, no Savings totals row, no Savings balance row; Balance equals today's `balanceRow`.

- [ ] **Step 2: Run** — FAIL.

- [ ] **Step 3: Implement.**
  - Rows: `const savingsRows = useMemo(() => [...(plan.structure.savings ?? [])].sort((a, b) => a.position - b.position).map((s) => ({ element: savingsAsPlanElement(s), hidden: false })), [plan])`.
  - `buildFlatRows`: take `savingsRows` and push them between the expense band and archived, skipped when `folded('savings')`.
  - Section JSX after the expense `<section>`: `<section role="rowgroup" data-testid="plan-section-savings" className="plan-band-savings mt-6 flex flex-col px-1 py-1">` with `SectionHeader label={t('budgets.page.plan.section.savings')} foldKey="savings" …`; body wrapped, in edit mode, in its own `DndContext` + `SortableContext` of the live rows' ids (look at how `LooseRowsContainer` / `handleBandDragEnd` compute `afterId` for the loose expense rows and reuse that helper from `elementMove.ts`; call `useMoveElement` with `folderId: null`). Deleted rows render with `ElementRow` outside the sortable list.
  - `RowMenu`: hide "Move to folder…" when `el.type === BudgetElementType.SAVINGS`. Envelope actions are already gated by `isEnvelopeType`.
  - `queries.ts`: `usePlanSetLimit` / `useFillPlannedCells` optimistic updaters also map `structure.savings` cells the same way they map `structure.elements` (write a small shared helper so the two arrays use one patch function).
  - Totals: add `{ key: 'savings', labelKey: 'budgets.page.plan.totals.savings', value: (t) => t.effectiveSavings }` to the rendered rows only when savings rows exist (pass a flag to `PlanTotals`). Balance: compute `savingsBalance = savingsBalanceRow(...)` and `everyday = everydayBalanceRow(balance, savingsBalance)`; when savings rows exist render the Balance row with `everyday` and a second sticky row "Savings balance" (`budgets.page.plan.totals.savings_balance`) with an info icon/`title` tooltip `budgets.page.plan.totals.savings_balance_tooltip`; otherwise render exactly as today.
  - i18n (11 catalogues): `budgets.page.plan.section.savings`, `budgets.page.plan.totals.savings`, `budgets.page.plan.totals.savings_balance`, `budgets.page.plan.totals.savings_balance_tooltip`.

- [ ] **Step 4: Run** the PlanSheet tests, the whole `src/features/budgets` folder, `tsc -b`, `pnpm lint`, the i18n Go guard — PASS.

- [ ] **Step 5: Commit** `feat(web): savings section in the plan view`.

---

### Task 10: SPA — monthly Savings block and widget line

**Files:**
- Create: `web/src/features/budgets/SavingsBlock.tsx`, `web/src/features/budgets/SavingsBlock.test.tsx`
- Modify: `web/src/features/budgets/BudgetPage.tsx`, `web/src/features/budgets/ExpenseWidget.tsx`, `web/src/features/budgets/queries.ts` (`useSetLimit` optimistic patch), `locales/*.json`

**Interfaces:**
- Consumes: `BudgetSavingsElementDto` (Task 7), #246's `CommentsDialog`, `commentCellKey`, `commentsReadOnly`, `SetLimitDialog`
- Produces:

```tsx
export function SavingsBlock(props: {
  budget: BudgetDto
  currencies: CurrencyDto[]
  selectedDate: string                 // "YYYY-MM-01"
  canEdit: boolean                     // same rule the budget table uses for limits
  editMode: boolean                    // drag handles shown only here
  commentsByCell: Map<string, BudgetCommentDto[]>
  onEditPlanned: (row: BudgetSavingsElementDto) => void   // opens the page's SetLimitDialog
  onOpenComments: (row: BudgetSavingsElementDto) => void  // opens the page's CommentsDialog
  onMove: (id: Id, afterId: Id | null) => void            // useMoveElement with folderId null
}): JSX.Element | null   // null when structure.savings is empty
```

- [ ] **Step 1: Failing tests** `SavingsBlock.test.tsx`:
  - Renders nothing for an empty/absent `savings`.
  - Renders a collapsible block (`data-testid="budget-savings-block"`) with one row per savings account: Planned / Saved / Remaining formatted in the row's currency; Remaining < 0 carries the same over-plan class the budget table uses for a negative available (read `BudgetTable.tsx` for it).
  - Clicking Planned (when `canEdit`) calls `onEditPlanned(row)`; not clickable when `!canEdit` or `isArchived`.
  - A row with comments shows `data-testid="comment-marker"` and clicking it calls `onOpenComments(row)`.
  - Edit mode: drag S2 above S1 → `onMove(S2, null)`; the block's `DndContext` is separate from the table's (no folder droppables inside).
  - `ExpenseWidget`: with savings rows (USD budget, one EUR row) shows "Saved {saved} of {planned} planned" in budget currency using `makeBudgetExchange`; without savings rows the line is absent.
  - `BudgetPage` integration (extend `BudgetPage.test.tsx`): the block renders below the table; editing Planned sends set-limit with `elementId = <account id>` and the optimistic update patches `structure.savings` (`budgeted`, `available`).

- [ ] **Step 2: Run** — FAIL.

- [ ] **Step 3: Implement** `SavingsBlock.tsx` (fold state persisted via `useBudgetPeriodStore`'s `planFolds['monthly-savings']` + `togglePlanFold`), mount it in `BudgetPage.tsx` below `BudgetTable` wiring `onEditPlanned` to the page's existing `SetLimitDialog` target state (`{ id, name, value: budgeted }`) and `onOpenComments` to the existing `CommentsDialog` target state; extend `useSetLimit`'s optimistic updater to patch `structure.savings` rows (`budgeted = amount`, `available = amount − spent`); add the widget line. i18n (11): `budgets.page.savings.title`, `…planned`, `…saved`, `…remaining`, `budgets.modal.expense_widget.saved_of_planned` ("Saved {saved} of {planned} planned").

- [ ] **Step 4: Run** the budgets folder tests, `tsc -b`, lint, metrics coverage, i18n guard — PASS.

- [ ] **Step 5: Commit** `feat(web): monthly savings block and widget line`.

---

### Task 11: Docs, regression plan, full gates, PR

**Files:**
- Modify: `docs/regression-test-plan.md`, `CLAUDE.md`, the spec's "Status and next steps" section

- [ ] **Step 1: Regression plan** — add a "Budget savings" group with verifiable items (📱 where the flow exists on mobile/tablet): mark an account as savings (create + edit) and see the Savings marker; switching it off when a budget plans it asks for confirmation naming the budgets, cancel keeps it, confirm removes the plans on the next budget change; plan view Savings section (fold, reorder by drag, no folder drop, cell edit, fill); Savings totals row; Balance + Savings balance rows sum to the former balance; a Savings balance rising by more than Saved (interest) is expected and the tooltip says so; monthly Savings block Planned/Saved/Remaining with over-plan style; widget "Saved X of Y planned"; an everyday→savings transfer shows as saved, savings↔savings and non-member transfers do not; clone keeps savings rows and (with limits) plans; comment thread on a savings cell in both views; deleted savings account stays only while it has a plan or activity.
- [ ] **Step 2: CLAUDE.md** — one "Notable behaviours" bullet: savings accounts (`accounts.type = 3`) get `ElementSavings` rows, actual = net everyday↔savings transfers, emitted in `structure.savings` (never `elements`), plan adds `savingsOpeningBalances`/`savingsFlows`, flows deliberately exceed the Savings row.
- [ ] **Step 3: Spec status** — replace "SPEC ONLY — nothing is implemented" with the implemented status and the one deliberate deviation (no `update_account` MCP tool exists, so only `create_account` gained `type`).
- [ ] **Step 4: Full gates** — `make go-test`, `make test-repo-pgsql`, enginecompare, `cd web && pnpm exec tsc -b && pnpm lint && pnpm test` (only the known Blob failure allowed). Record the numbers.
- [ ] **Step 5: Commit, push** `git push -u origin budget-savings-work:feature/budget-savings` (a fast-forward: the branch merged v1.6-dev rather than rebasing), update PR #245's title/body (base `v1.6-dev`) from spec-only to the implementation, mark ready only when the user says so.
