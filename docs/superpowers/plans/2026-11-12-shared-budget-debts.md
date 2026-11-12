# Debts That Belong to a Budget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move debts from a user to a budget: every plan-capable participant manages them, a debt matches payments through one or more rules (any participant's category), only on the budget's member accounts, and classification deletes and merges never refuse because of a debt.

**Architecture:** A new migration drops the unreleased debt tables and recreates them keyed by `budget_id`, with a `debts_rules` child table and `debts_payments` keyed `(debt_id, transaction_id)`. The pure matching rules (overlap, conflict, range, cleanup) live in `internal/model`. The debt service works per budget: it reads access, member accounts and participants through a new `debt.Budgets` port, takes the BUDGET row lock instead of the owner's, and syncs one budget at a time. The budget feature passes a `model.DebtScope` (budget id + member accounts) to `DebtSource`, scopes its `debts_payments` joins to its own debts, copies debts on clone, and stops treating debts as a participant's property. The SPA gets a rule list editor, an "Add to a debt…" modal, read-only review for guests and a Settings list grouped by budget.

**Tech Stack:** Go (sqlc v1.30.0, SQLite + PostgreSQL), React 19 + TanStack Query, vitest.

**Spec:** `docs/superpowers/specs/2026-11-12-shared-budget-debts-design.md` (amends `2026-10-29-budget-debts-design.md` and `2026-11-07-debt-setup-redesign-design.md`; read all three and CLAUDE.md "Budget debts" / "Debt SPA flow" / onboarding "Debt detection").

## Global Constraints

- Everything in CLAUDE.md applies: features never import features (ports in the consumer, adapters in `internal/server/glue_*.go`); GET/POST only; handlers through `endpoint.Handle*` with swag blocks; ASCII-only `.sql`; sparse comments (the why, never the what); `make swagger` after handler changes; goldens regenerated with `UPDATE_GOLDEN=1` and INSPECTED.
- Go: `/usr/local/go/bin/go` with `GOTOOLCHAIN=go1.27.1`; sqlc via `go generate ./internal/infra/storage/sqlc/...`; PostgreSQL suites via a throwaway `postgres:17` container + `make test-engines` and `make test-repo-pgsql`.
- Migration `20261110000500` is NOT edited. The new one is `internal/infra/storage/migrations/{sqlite,pgsql}/20261112000000.sql` (same version on both engines, above every migration on `main`, whose newest is `20261110000000`).
- **Tasks 1–5 are one unit.** Task 1 keeps the build but switches matching to rules, so the debt and budget suites fail until Tasks 2 and 5. After Task 2 `go build ./...` fails in `internal/budget`, `internal/server` and everything that imports them until Task 5 lands. Tasks 1–4 verify only the packages they list; Task 5 restores the full build; Task 6 ends with `make go-test` green.
- Wire (unreleased, may change): `DebtResult` drops `categoryId/payeeId/tagId/labelId`, gains `budgetId`, `budgetName`, `canEdit` (0/1), `rules[]`; `create-debt` takes `budgetId` + `rules` (1..10), `currencyId` optional; `update-debt` takes optional `rules`; new `POST /api/v1/debt/add-debt-rule` and `GET /api/v1/debt/get-debt-rule-options`; `get-debt-forecast` loses `budgetId`; `structure.debts[].ownerUserId` (get-budget and get-budget-plan) goes away.
- Error codes removed from `errs` + `AllCodes` + all 11 locales: `category.used_by_debt`, `payee.used_by_debt`, `tag.used_by_debt`, `label.used_by_debt`. No new codes: rule count errors use `common.is_blank` / `common.out_of_range`, rule overlaps `debt.key_conflict`.
- New metric `METRICS.DEBT_RULE_ADD: 'appDebtRuleAdd'`, fired when `add-debt-rule` succeeds from the "Add to a debt…" modal.
- UI copy in all 11 locales (de Sie, nl u, fr vous, pt European); pickers via `PickerSheet` + `rankByName`; no bold for hierarchy; every new write control reads `useOnline`.
- Branch `feature/budget-debts`; each task commits with a message ending `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Rulings (decisions the spec leaves open; binding for every task)

1. The migration drops and recreates `debts`, `debts_snapshots`, `debts_payments` (their data is deleted anyway, which avoids SQLite table rebuilds) and deletes the limits, comments and rows of every `budgets_elements` row with `type = 6` explicitly (the runner turns foreign keys off, so nothing cascades).
2. `debts_rules` keeps NO ACTION foreign keys to categories/payees/tags/labels: a delete that forgot to release the rules fails loudly instead of leaving dangling ids. The delete paths release the rules first. (No app path hard-deletes a user.)
3. A debt's range is truncated only by a later-starting debt of the same budget that has a rule with an IDENTICAL key to one of its rules (today's partition, applied to all its rules). Rules that overlap only through different labels keep the older-debt tiebreak.
4. Within one debt, identical or overlapping rules are refused (`debt.key_conflict`), except two rules on one category with different labels (harmless inside one debt). When a classification change makes a rule narrower than another rule of the same debt it covers, the narrower one is dropped.
5. Plans move ONTO the debt row per whole-category rule (create, add-debt-rule, a rule edited into whole-category, an earlier first planning day, a merge onto it). Plans go BACK to a category (delete, setup tail, later first planning day) only when the debt's sole rule is whole-category; otherwise they stay on the debt row (a delete then needs `confirmDataRemoval` when the row has data).
6. `update-debt` without `rules` keeps the stored rules; with `rules` it replaces them (a rule matched by `id`, else by identical key, keeps its id and `created_at`); `[]` is `common.is_blank`. Kept rules are not revalidated, so a rule of a participant who left survives an edit.
7. Rule validation and conflicts use the flat field key `rules`; `debt.start_date_taken` stays on `startDate`.
8. No read access to the debt's budget (or a pending invite) → 404 (`Debt not found` / `Budget not found`); read but not plan (guest) → 403 `Access denied`; archived budget → 403 `budget.archived` on every debt write.
9. `create-debt` without `currencyId` takes the budget's currency. A currency the budget already uses (its own or an element row's) skips `EnsureUsable`; any other goes through it for the caller.
10. The forecast always uses the debt's own budget's plans; `budgetId` leaves the request, `debtDetailsBudgetId` leaves the SPA store.
11. New `GET /api/v1/debt/get-debt-rule-options?budgetId=` (plan access) lists the participants (caller first, then by name), each with active expense categories, payees, tags and labels. The SPA has no other source for a partner's classifications.
12. Each `DebtResult.rules[]` entry also carries `payeeName`, `tagName`, `labelName`, so the read-only sentence renders for every reader.
13. Participants are the budget owner plus accepted non-guests (whose categories the budget shows); a rule's category must belong to one; its payee/tag/label to that category's owner.
14. `clone-budget` copies each debt with new ids but its original `created_at` and `created_day`, so the tiebreak and the planning start are unchanged in the copy.
15. The budget row lock is `SELECT id FROM budgets WHERE id = $1 FOR NO KEY UPDATE` (does not block the KEY SHARE of child inserts); global order: budget rows in id order, then element rows. Classification deletes and merges take them first (`Prepare`).
16. The budget's spending and drill-down SQL joins `debts_payments` only for the budget's own debts; otherwise a payment counted by debts in two budgets fans out and double-counts.
17. A debt's icon is its first rule's category icon (rules ordered `created_at, id`), `model.DefaultIcon` with no rules.
18. Row menu: Details for every reader (guests get the read-only screen); Mark as paid off, Delete, Track as debt and Add to a debt only with plan access on an unarchived budget.
19. `get-debt-list` covers every budget the caller can read, archived ones included (`canEdit` 0); Settings → Debts disables Create when the user can plan in no unarchived budget.

## Review Focus

1. **A payment on an account that is a member of two budgets, each with a debt matching it.** Each budget must count it once, on its own debt row, and nothing on the other budget's debt. Test `TestGetBudget_PaymentMatchedInTwoBudgetsCountsOnceInEach` (Task 5).
2. **An account leaving the budget** (participant revoked, `remove-account`). Its payments stop counting for the debt, and the stale splits go on the next sync. Test `TestSync_AccountLeavingTheBudgetDropsItsSplits` (Task 2).
3. **A former participant or a guest deletes their own category that a shared debt's rule names.** The delete succeeds without access to the debt; the rule goes and the debt stays. Test `TestReleaseClassification_NeedsNoAccessToTheDebt` (Task 2) and `TestDeleteCategory_ReleasesDebtRules` (Task 3).
4. **Deleting a payee that makes two debts' rules identical.** With the same start date the newer debt loses the rule; with distinct start dates both stay, partitioned. Tests `TestNormalizeBudgetRules_IdenticalAcrossDebts*` (Task 1).
5. **PostgreSQL lock order.** `add-debt-rule` or `delete-debt` racing `set-limit` on the same rows must neither deadlock nor lose the plan. Test `TestAddDebtRule_ConcurrentSetLimitIsMovedOrWaits` (Task 5, `enginecompare` tag).

---

### Task 1: Model — rules, conflicts, range and cleanup

**Files:**
- Modify: `internal/model/debt.go`
- Test: `internal/model/debt_test.go`

**Interfaces:**
- Produces (additive; old `Debt`/`DebtView` key fields are removed in Task 2):
  ```go
  type DebtRule struct {
      ID, CategoryID           vo.Id
      PayeeID, TagID, LabelID  *vo.Id
      CreatedAt                time.Time
  }
  func (r DebtRule) IsWhole() bool              // no payee, tag or label
  func (r DebtRule) SameKey(o DebtRule) bool    // same category, payee, tag, label
  func (r DebtRule) Overlaps(o DebtRule) bool   // same category, payee and tag each unset-or-equal, and not (both labels set and different)
  func (r DebtRule) Covers(o DebtRule) bool     // same category; each of r's payee/tag/label unset or equal to o's
  // Debt gains: BudgetID vo.Id; Rules []DebtRule (ordered CreatedAt, ID)
  func CheckDebtRules(d *Debt, budgetDebts []*Debt) error   // budgetDebts may contain d (skipped by ID)
  func DebtUntil(d *Debt, budgetDebts []*Debt) *time.Time
  func IsWholeCategoryRule(d *Debt, r DebtRule, budgetDebts []*Debt) bool // r.IsWhole() and no other debt has a whole rule on r.CategoryID
  func SoleWholeCategory(d *Debt, budgetDebts []*Debt) (vo.Id, bool)      // exactly one rule and it is whole-category
  func NormalizeBudgetRules(budgetDebts []*Debt) []*Debt                  // mutates in place, returns the debts whose rules changed
  type DebtRuleView struct{ CategoryID string; PayeeID, TagID, LabelID *string }
  // DebtView gains: BudgetID string; Rules []DebtRuleView
  func (v DebtView) CarriesKey(categoryID, tagID *string) bool // some rule on categoryID whose tag is unset or equal
  ```
  `DebtMatches` and `AssignDebt` keep their signatures and match when ANY rule matches (plus the range).
- `CheckDebtRules` errors: `debt.start_date_taken` on `startDate` when another debt has a rule with an identical key and the same start date (nil equals nil); `debt.key_conflict` on `rules` for any other overlap, within d (Ruling 4) or with another debt.

- [ ] **Step 1: Write the failing tests** in `internal/model/debt_test.go`:
  - `TestDebtRule_Overlaps`: X vs X+P → true; X+P1 vs X+P2 → false; X+L1 vs X+L2 → false; X+L1 vs X → true; X+P vs X+T → true; X vs Y → false.
  - `TestDebtRule_Covers`: X covers X+P; X+P does not cover X; X+P covers X+P+T.
  - `TestCheckDebtRules_WithinOneDebt`: rules {X, X} → key_conflict on `rules`; {X, X+P} → key_conflict; {X+L1, X+L2} → nil.
  - `TestCheckDebtRules_AcrossDebts`: other debt {X} start Jan, d {X} start Jan → start_date_taken on `startDate`; d start Feb → nil; d {X+P} → key_conflict; other {X+L1}, d {X+L2} → nil.
  - `TestDebtUntil_LaterIdenticalRuleEndsEveryRule`: d {X, Y} from Jan; e {Y} from Jun → Until Jun; f {Z} from Mar ignored; g {X+L1} from Apr ignored; d nil start, e Jun → Jun; nothing later → nil.
  - `TestDebtMatches_AnyRule`: view rules {X, Y+P}: category Y payee P → true; Y payee Q → false; X any → true; outside From/Until → false.
  - `TestAssignDebt_OldestWins` (adapt the existing one to rules).
  - `TestDebtView_CarriesKey`: rule {X, tag T} carries (X, T) not (X, nil); rule {X} carries (X, anything).
  - `TestSoleWholeCategory`: d {X} alone → (X, true); d {X, Y} → false; d {X+P} → false; other debt with whole X (distinct start) → false.
  - `TestNormalizeBudgetRules_DuplicateInOneDebtCollapses`: {X, X} → one rule left, the older id.
  - `TestNormalizeBudgetRules_NarrowerRuleOfTheSameDebtDropped`: {X, X+P2} → {X}.
  - `TestNormalizeBudgetRules_OverlapAcrossDebtsNewerLoses`: older a {X}, newer b {X+P, Y} → b {Y}; a unchanged; returns [b].
  - `TestNormalizeBudgetRules_IdenticalAcrossDebtsSameStartNewerLoses` and `..._DistinctStartsKept`.
  - `TestNormalizeBudgetRules_DifferentLabelsKept`: a {X+L1}, b {X+L2} → unchanged, returns nil.
- [ ] **Step 2: Run** `go test ./internal/model/ -run 'DebtRule|CheckDebtRules|DebtUntil|DebtMatches|AssignDebt|CarriesKey|SoleWhole|NormalizeBudgetRules'` — expect FAIL (undefined).
- [ ] **Step 3: Implement** the types and functions in `internal/model/debt.go`. `NormalizeBudgetRules` processes debts oldest first (`CreatedAt`, then `ID`): first drops, within each debt, a rule covered by another rule of that debt (of two identical, the newer `CreatedAt, ID`), then drops a newer debt's rule that fails `CheckDebtRules`' cross-debt test against an older debt's rule.
- [ ] **Step 4: Run** `go test ./internal/model/` — expect PASS (old debt tests in other packages are not run here).
- [ ] **Step 5: Commit** `feat(debt): Debt rules with per-rule conflicts and cleanup in the model`.

### Task 2: Storage and the debt service on budgets

**Files:**
- Create: `internal/infra/storage/migrations/{sqlite,pgsql}/20261112000000.sql`, `internal/infra/storage/migrations/migration_20261112_test.go`, `migration_20261112_pgsql_test.go` (`enginecompare` twin, same pattern as `migration_20261108_*`), `internal/debt/rules.go` (add-debt-rule, rule parsing/validation/resolution), `internal/debt/access.go` (budget access helpers), `internal/debt/clone.go`, `internal/debt/options.go` (rule options)
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/debts.sql` (+ regenerate), `internal/debt/{repository.go,ports.go,usecase.go,create.go,update.go,delete.go,read.go,sync.go,timeline.go,forecast.go,reconcile.go,classification.go,onboarding.go}`, `internal/debt/repo/{repo.go,sqlite.go,pgsql.go}`, `internal/debt/api/debt.go` (compile only: request types), `internal/model/debt.go`, `internal/model/debt_dto.go`
- Test: `internal/debt/repo/repo_test.go`, every `internal/debt/*_test.go` (rewrite `helpers_test.go` fakes), `internal/model/debt_dto_test.go`

**Interfaces:**
- Consumes: Task 1.
- Schema (`20261112000000.sql`, both engines; pgsql uses `UUID` / `TIMESTAMP(0) WITHOUT TIME ZONE` as `20261110000500` does):
  - `DELETE` the `budgets_elements_limits` and `budgets_elements_comments` of `budgets_elements` rows with `type = 6`, then those rows; `DROP TABLE debts_payments, debts_snapshots, debts`.
  - `debts`: `20261110000500`'s columns minus `user_id, category_id, payee_id, tag_id, label_id`, plus `budget_id NOT NULL REFERENCES budgets (id) ON DELETE CASCADE`; indexes `debts_budget_idx`, `debts_currency_idx`.
  - `debts_rules (id PK, debt_id NOT NULL FK debts CASCADE, category_id NOT NULL FK categories, payee_id FK payees, tag_id FK tags, label_id FK labels, created_at NOT NULL)`; indexes `debts_rules_debt_idx`, `_category_idx`, `_payee_idx`, `_tag_idx`, `_label_idx`.
  - `debts_snapshots` as in `20261110000500`; `debts_payments` as before with `PRIMARY KEY (debt_id, transaction_id)` and index `debts_payments_transaction_idx (transaction_id)`.
- Model (`internal/model/debt.go`): `Debt` loses `OwnerID, CategoryID, PayeeID, TagID, LabelID`; `DebtView` loses `OwnerID, CategoryID, PayeeID, TagID, LabelID`. `ElementDataTransfer` gains `BudgetID vo.Id` (the only budget whose rows move). `DebtTimelineRequest` gains `Scope DebtScope`. New:
  ```go
  type DebtScope struct{ BudgetID vo.Id; AccountIDs []vo.Id }          // member accounts, soft-deleted included
  type DebtBudget struct {
      ID, CurrencyID vo.Id; Name string; PeriodStartDay int
      CurrencyIDs    []vo.Id // the budget's and its element rows' own currencies
      IsArchived, CanPlan bool // CanPlan: owner, admin or user
      ParticipantIDs []vo.Id // owner + accepted non-guests
      AccountIDs     []vo.Id
  }
  type DebtClassification struct{ OwnerID vo.Id; Name, Icon string; IsExpense bool }
  ```
- DTOs (`internal/model/debt_dto.go`):
  ```go
  type DebtRuleRequest struct { Id *string `json:"id"`; CategoryId string `json:"categoryId"`; PayeeId, TagId, LabelId *string } // json tags payeeId, tagId, labelId
  // CreateDebtRequest: + BudgetId string `json:"budgetId"`, Rules []DebtRuleRequest `json:"rules"`; - CategoryId/PayeeId/TagId/LabelId; CurrencyId optional
  // UpdateDebtRequest: + Rules []DebtRuleRequest `json:"rules"` (nil = keep); - CategoryId/PayeeId/TagId/LabelId
  type AddDebtRuleRequest struct { Id string `json:"id"`; Rule DebtRuleRequest `json:"rule"` }
  type GetDebtRuleOptionsRequest struct { BudgetId string `json:"budgetId"` }
  // GetDebtForecastRequest: - BudgetId
  type DebtRuleResult struct { Id, CategoryId, CategoryName, CategoryIcon, OwnerUserId, OwnerName string; PayeeId *string; PayeeName string; TagId *string; TagName string; LabelId *string; LabelName string } // camelCase json
  // DebtResult: - CategoryId/PayeeId/TagId/LabelId; + BudgetId, BudgetName string; CanEdit int; Rules []DebtRuleResult
  type DebtRuleOption struct { Id, Name, Icon string }
  type DebtRuleParticipant struct { UserId, Name string; Categories, Payees, Tags, Labels []DebtRuleOption }
  type DebtRuleOptionsResult struct { Participants []DebtRuleParticipant `json:"participants"` }
  ```
  `Validate()`: create needs `budgetId` (uuid) and 1..10 rules; update's non-nil `rules` 1..10; each rule's ids are uuids, `id` optional, tag+label together → `common.invalid_choice`; all on key `rules` (`common.is_blank` for none, `common.out_of_range` for >10).
- Ports (`internal/debt/ports.go`): `Classifications` becomes `Classification(ctx, kind string, id vo.Id) (model.DebtClassification, error)` (errs.NotFound when missing; kinds `KindCategory/KindPayee/KindTag/KindLabel`); `Owners` becomes `Users { GetTimezone(ctx, id) (string, error); GetName(ctx, id) (string, error) }`; `BudgetPlans` is removed; new:
  ```go
  type Budgets interface {
      Access(ctx context.Context, userID, budgetID vo.Id) (model.DebtBudget, error) // errs.NotFound unless userID has accepted access
      Readable(ctx context.Context, userID vo.Id) ([]model.DebtBudget, error)        // budget-list order
      Scope(ctx context.Context, budgetID vo.Id) (model.DebtScope, error)             // no access check
      RuleOptions(ctx context.Context, viewerID, budgetID vo.Id) (*model.DebtRuleOptionsResult, error)
      DebtPlans(ctx context.Context, budgetID, debtID vo.Id) (plans model.DebtBudgetPlans, ok bool, err error)
  }
  ```
  `NewService(repo, tx, clock, class Classifications, rates Rates, currencies Currencies, users Users)`; `SetBudgets(Budgets)`, `SetBudgetElements(BudgetElements)` (both installed late; a nil `Budgets` makes every use case fail with an internal error, tests always wire a fake).
- Repository (`internal/debt/repository.go`): `LockOwner`, `ListByCategory`, `ListByOwners`, `Uses*`, `Reassign*` go; `CandidateTx` gains `CategoryID vo.Id`; new/changed:
  ```go
  LockBudget(ctx context.Context, budgetID vo.Id) error // FOR NO KEY UPDATE on pgsql (no-rows = success), no-op on sqlite
  GetByID(ctx, id) (*model.Debt, error)                 // rules loaded
  ListByBudget(ctx, budgetID vo.Id) ([]*model.Debt, error)            // created_at, id
  ListByBudgets(ctx, budgetIDs []vo.Id) ([]*model.Debt, error)
  ListByClassification(ctx, kind string, id vo.Id) ([]*model.Debt, error) // debts with a rule naming id
  Save(ctx, d *model.Debt) error // upsert the row, then delete + insert its rules
  InsertSplit(ctx, s model.DebtSplit) error // upsert on (debt_id, transaction_id)
  CandidateTransactions(ctx, categoryIDs, accountIDs []vo.Id) ([]CandidateTx, error) // expenses (type 0), hand-built IN lists
  ```
- Service (produced for Tasks 3–6):
  ```go
  func (s *Service) CreateDebt(ctx, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error)
  func (s *Service) UpdateDebt(ctx, userID vo.Id, req model.UpdateDebtRequest) (*model.DebtResult, error)
  func (s *Service) AddDebtRule(ctx, userID vo.Id, req model.AddDebtRuleRequest) (*model.DebtResult, error)
  func (s *Service) GetDebtRuleOptions(ctx, userID vo.Id, req model.GetDebtRuleOptionsRequest) (*model.DebtRuleOptionsResult, error)
  // DeleteDebt, MarkDebtPaidOff, ReconcileDebt, DeleteDebtReconciliation, GetDebt, GetDebtList, GetDebtForecast: same signatures
  func (s *Service) Views(ctx, scope model.DebtScope) ([]model.DebtView, error)  // no lock, no write
  func (s *Service) Sync(ctx, scope model.DebtScope) error                       // own tx, budget lock
  func (s *Service) Timelines(ctx, req model.DebtTimelineRequest) (map[string]model.DebtTimeline, error)
  func (s *Service) CloneDebts(ctx, fromBudgetID, toBudgetID vo.Id) (map[vo.Id]vo.Id, error) // joins the caller's tx
  func (s *Service) LockForClassification(ctx, kind string, ids ...vo.Id) error  // budgets of the debts naming any id, id order
  func (s *Service) ReleaseClassification(ctx, kind string, id vo.Id) error
  func (s *Service) ReassignClassification(ctx, kind string, oldID, newID vo.Id) error // never refuses
  func (s *Service) CoveredCategories(ctx, budgetID vo.Id) ([]vo.Id, error)
  func (s *Service) TakeCategoryPlans(ctx, budgetID, categoryID vo.Id) (moved bool, err error)
  ```
- Behaviour the tests pin (Rulings 3–17 apply):
  - Every use case resolves the budget with `Budgets.Access` (404 masks), writes need `CanPlan` (403) and not `IsArchived` (403 `budget.archived`).
  - Sync per budget: one transaction, `LockBudget`, `ListByBudget`, candidates in the union of rule categories on `scope.AccountIDs`, `AssignDebt` over the budget's views (ranges from `model.DebtUntil`), stale splits deleted before new ones are inserted. Splits of other budgets' debts are never touched.
  - Rule checks: a new or changed rule's category must be an expense category (`debt.category_not_expense` on `rules`) owned by a participant (else `Category not found`), its payee/tag/label owned by the category's owner (else not found); then `model.CheckDebtRules` against `ListByBudget`.
  - `CreatedDay` uses the creating user's stored timezone (`Users.GetTimezone`).
  - Plan moves call `BudgetElements.TransferData` with `BudgetID` set to the debt's budget (Ruling 5).
  - `ReleaseClassification`: category → its rules removed; payee/tag/label → that field cleared; then `model.NormalizeBudgetRules` per affected budget and `Save` the changed debts; no access check, plans untouched. `ReassignClassification`: replace the id, normalize, then for a category move plans onto every whole-category rule's debt on the target (as today's merge), then sync each affected budget inside the caller's transaction.
  - `CloneDebts`: rules (new ids, same keys and `CreatedAt`), snapshots (new ids), every debt column, new `BudgetID`; no splits.

- [ ] **Step 1: Write the migration test** `TestMigration20261112_DropsDebtsAndDebtRows`: run up to `20261110000500`, seed a user, budget, category, debt (old shape), snapshot, split, a type-6 element with a limit and a comment, and a category element with a limit; `runAll`; assert the debt tables are empty, the type-6 element, limit and comment are gone, the category element and its limit remain, `debts` has `budget_id` and no `user_id`, and two `debts_payments` rows for one transaction with different debts insert. PostgreSQL twin shares the check function.
- [ ] **Step 2: Write the repo tests** in `internal/debt/repo/repo_test.go`: `TestRepo_SaveReplacesRules` (save 2 rules, save 1 → 1 left, id kept), `TestRepo_ListByBudgetLoadsRulesInOrder`, `TestRepo_ListByClassification` (each kind), `TestRepo_CandidateTransactionsOnTheGivenAccounts` (an expense in a rule category on a non-listed account is absent; incomes absent; labels loaded), `TestRepo_SplitsOfOneTransactionForTwoDebts`, `TestRepo_BudgetDeleteCascades` (rules, snapshots, splits gone), `TestRepo_LockBudget` (missing budget succeeds; runs on pgsql under `make test-repo-pgsql`).
- [ ] **Step 3: Write the service tests** (rewrite `helpers_test.go`: `fakeBudgets` keyed by budget id with role per user, archived flag, participants, accounts, currencies; `fakeElements` recording `TransferData` calls including `BudgetID`; `fakeClass` keyed by kind+id with owner/name/icon/expense; `fakeUsers` with timezone and name). Keep every existing test that still describes valid behaviour, re-expressed with a budget and one rule; add:
  - Create: `TestCreateDebt_RulesAcrossParticipants` (his + her category; result `rules` ordered, `ownerName` set, `budgetId`, `budgetName`, `canEdit` 1); `TestCreateDebt_CategoryOfANonParticipantNotFound`; `TestCreateDebt_PayeeMustBeTheCategoryOwners`; `TestCreateDebt_GuestForbidden` (403); `TestCreateDebt_NoAccessNotFound`; `TestCreateDebt_ArchivedBudget` (`budget.archived`); `TestCreateDebt_CurrencyDefaultsToTheBudgets`; `TestCreateDebt_BudgetCurrencySkipsEnsureUsable`; `TestCreateDebt_CreatedDayInTheCreatorsTimezone`; `TestCreateDebt_MovesPlansOfEachWholeCategoryRule` (two whole rules → two transfers, both with the budget id).
  - Sync: `TestSync_PaymentsFromEveryRule`; `TestSync_OnlyTheBudgetsAccounts`; `TestSync_SoftDeletedMemberStillCounts`; `TestSync_AccountLeavingTheBudgetDropsItsSplits` (Review Focus 2: shrink the fake scope, sync, split row gone); `TestSync_SameTransactionCountsInTwoBudgets` (two splits, one per debt); `TestSync_LaterDebtEndsEveryRule`.
  - Update / add rule: `TestUpdateDebt_RulesReplaceSetKeepsIds`; `TestUpdateDebt_RulesLeftOutAreKept`; `TestUpdateDebt_EmptyRulesRefused`; `TestUpdateDebt_KeptRuleOfAFormerParticipantNotRevalidated`; `TestUpdateDebt_RuleEditedIntoWholeMovesThatCategory`; `TestUpdateDebt_RemovedRuleLeavesThePlans` (no transfer back); `TestFinishSetup_TailGoesBackOnlyForASoleWholeRule`; `TestAddDebtRule_MovesTheCategoryPlansAddingUp` (`SumOnClash` true); `TestAddDebtRule_CategoryAlreadyCovered` (`debt.key_conflict`); `TestAddDebtRule_ConflictWithAnotherDebt`; `TestAddDebtRule_TenRulesAtMost` (`common.out_of_range` on `rules`).
  - Delete: `TestDeleteDebt_SoleWholeRuleHandsBack`; `TestDeleteDebt_SeveralRulesNeedConfirmation`; `TestDeleteDebt_NoRulesNeedsConfirmationOnlyWithData`.
  - Read: `TestGetDebtList_EveryReadableBudget` (two budgets, archived included with `canEdit` 0, pending invite absent); `TestGetDebt_GuestReadsWithCanEditZero`; `TestGetDebtForecast_UsesTheDebtsBudget`; `TestTimelines_ScopeLimitsPayments`; `TestGetDebtRuleOptions_NeedsPlanAccess`.
  - Classifications: `TestReleaseClassification_CategoryRemovesItsRules` (debt with that sole rule keeps existing, matches nothing); `TestReleaseClassification_PayeeWidensTheRule`; `TestReleaseClassification_OverlapOlderDebtKeeps`; `TestReleaseClassification_NeedsNoAccessToTheDebt` (Review Focus 3); `TestReassignClassification_NeverRefuses`; `TestReassignClassification_MergeIntoWholeRuleMovesThePlans`.
  - Clone / onboarding-facing: `TestCloneDebts_CopiesRulesSnapshotsAndTerms` (new ids, same `CreatedAt`/`CreatedDay`, no splits); `TestCoveredCategories_PerBudget`; `TestTakeCategoryPlans_PerBudget`.
- [ ] **Step 4: Run** `go test ./internal/model/ ./internal/debt/... ./internal/infra/storage/...` — expect FAIL (compile and new behaviour).
- [ ] **Step 5: Implement**: migration files; queries in `debts.sql` (drop the owner/category/uses/reassign queries; add `ListDebtsByBudget`, `DeleteDebtRules`, `InsertDebtRule`, pgsql `LockDebtBudget`; `InsertDebtSplit` conflicts on `(debt_id, transaction_id)`), `go generate ./internal/infra/storage/sqlc/...`; repo (hand-built `ListByBudgets`, rules read, `ListByClassification`, `CandidateTransactions` with the same placeholder helpers as today); the service per the Interfaces above, replacing `syncCategory`/`ownedDebt`/`LockOwner` with `syncBudget`/`readBudget`/`editableDebt`/`LockBudget`; `internal/debt/api/debt.go` only as far as compiling (new handlers come in Task 6).
- [ ] **Step 6: Run** `go test ./internal/model/ ./internal/debt/... ./internal/infra/storage/...` and `go vet ./internal/debt/...` — expect PASS (`internal/server` and `internal/budget` do not build yet).
- [ ] **Step 7: Commit** `feat(debt): Debts belong to a budget and match through rules`.

### Task 3: Classification deletes and merges release debt rules

**Files:**
- Modify: `internal/{category,payee,tag,label}/ports.go`, `.../delete.go`, `.../merge.go`; `internal/server/glue_debt.go` (`DebtUsage` adapter, `DebtClassifications` → `Classification`, compiled in Task 5); `internal/shared/errs/codes.go`; `locales/*.json` (11)
- Test: `internal/{category,payee,tag,label}/*_test.go` (delete/merge tests with the fake `DebtUsage`)

**Interfaces:**
- Consumes: Task 2's `LockForClassification`, `ReleaseClassification`, `ReassignClassification`.
- Produces, in each of the four packages:
  ```go
  type DebtUsage interface {
      // Prepare locks the budgets of the debts whose rules name any of ids; first in the transaction.
      Prepare(ctx context.Context, ids ...vo.Id) error
      // Release takes id out of every debt rule; called before the row is deleted.
      Release(ctx context.Context, id vo.Id) error
      Reassign(ctx context.Context, oldID, newID vo.Id) error
  }
  ```
  Delete order inside the existing transaction: ownership check, `Prepare(id)`, `Release(id)`, `repo.Delete`. Merge: `Prepare(sourceID, targetID)` where `PrepareMerge` was, `Reassign` where it was; a merge never fails because of a debt.

- [ ] **Step 1: Write the failing tests**: `TestDeleteCategory_ReleasesDebtRules` (fake records `Prepare` then `Release` with the id before the repo delete; no error even when the fake reports rules), same for payee/tag/label (`TestDeletePayee_ReleasesDebtRules`, ...); `TestMergeCategory_PreparesBothSidesFirst`; remove the `*_UsedByDebt` refusal tests.
- [ ] **Step 2: Run** `go test ./internal/category/... ./internal/payee/... ./internal/tag/... ./internal/label/...` — expect FAIL.
- [ ] **Step 3: Implement** the port, the delete/merge call order, the glue adapter (`Prepare` → `LockForClassification(kind, ids...)`, `Release` → `ReleaseClassification`, `Reassign` → `ReassignClassification`); delete the four codes from `codes.go`/`AllCodes` and the `errors.{category,payee,tag,label}.used_by_debt` keys from all 11 catalogues.
- [ ] **Step 4: Run** the four package suites and `go test ./internal/test/i18ntest/ ./internal/shared/...` — expect PASS.
- [ ] **Step 5: Commit** `feat(debt): Deleting or merging a classification no longer waits on debts`.

### Task 4: Onboarding debt detection per budget

**Files:**
- Modify: `internal/onboarding/ports.go`, `internal/onboarding/debts.go`, `internal/server/glue_onboarding.go` (compiled in Task 5)
- Test: `internal/onboarding/*debt*_test.go` (fakes)

**Interfaces:**
- Produces (`onboarding.DebtSteps`):
  ```go
  CreateDebt(ctx context.Context, userID, budgetID vo.Id, name string, categoryID, currencyID vo.Id, payment vo.DecimalNumber, schedule model.RecurringSchedule) (created bool, err error)
  CoveredCategories(ctx context.Context, budgetID vo.Id) ([]vo.Id, error)
  TakeCategoryPlans(ctx context.Context, budgetID, categoryID vo.Id) (moved bool, err error)
  ```
  `startPlanDebts` passes save-plan's budget; `detectDebts` its `budgetID` (the import review's default budget). The glue builds `CreateDebtRequest{BudgetId, Name, CurrencyId, Rules: [{CategoryId}], Payment, Schedule}` and keeps treating validation, not-found and access-denied refusals as "skipped".

- [ ] **Step 1: Write the failing tests**: `TestDetectDebts_CreatesInTheGivenBudget` (fake records `budgetID`), `TestStartPlanDebts_TakesPlansInThePlannedBudget`, `TestDetectDebts_CoveredIsPerBudget` (a category covered in another budget is still asked about).
- [ ] **Step 2: Run** `go test ./internal/onboarding/` — expect FAIL.
- [ ] **Step 3: Implement** the port change and call sites; adapt `OnboardingDebts` in the glue.
- [ ] **Step 4: Run** `go test ./internal/onboarding/` — expect PASS.
- [ ] **Step 5: Commit** `feat(onboarding): Detected debts go into the budget the step uses`.

### Task 5: Budget integration and server wiring

**Files:**
- Create: `internal/budget/debt_access.go` (the methods behind `debt.Budgets`)
- Modify: `internal/budget/{ports.go,usecase.go,builder.go,builder_debts.go,debt_elements.go,move.go,accesssvc.go,clone.go,txlist.go,projected_read.go,dated_read.go,readmodel.go}`, `internal/budget/repo/read.go`, `internal/model/budget*_dto.go` (`OwnerUserId` off `DebtElementResult` and `PlanDebtElementResult`), `internal/server/{glue_budget_debt.go,glue_debt.go,server.go}`
- Test: `internal/budget/{debt_attribution_test.go,builder_debts_test.go,debt_elements_test.go,debt_txlist_test.go,debt_period_test.go}`, `internal/budget/api/transfer_lock_pgsql_test.go`, new `internal/budget/debt_access_test.go`

**Interfaces:**
- Consumes: Task 2 service and model; Tasks 3–4 glue.
- Produces:
  ```go
  // internal/budget/ports.go
  type DebtSource interface {
      Views(ctx context.Context, scope model.DebtScope) ([]model.DebtView, error)
      Sync(ctx context.Context, scope model.DebtScope) error
      Timelines(ctx context.Context, req model.DebtTimelineRequest) (map[string]model.DebtTimeline, error)
      CloneDebts(ctx context.Context, fromBudgetID, toBudgetID vo.Id) (map[vo.Id]vo.Id, error)
  }
  func (a *budgetAggregate) debtScope() model.DebtScope // budget id + every member account id
  // internal/budget/debt_access.go (wrapped by server.DebtBudgets into debt.Budgets)
  func (s *Service) DebtBudget(ctx, userID, budgetID vo.Id) (model.DebtBudget, error)
  func (s *Service) DebtBudgets(ctx, userID vo.Id) ([]model.DebtBudget, error)
  func (s *Service) DebtScope(ctx, budgetID vo.Id) (model.DebtScope, error)
  func (s *Service) DebtRuleOptions(ctx, viewerID, budgetID vo.Id) (*model.DebtRuleOptionsResult, error) // MetadataLookup + LabelsForUsers + UserLookup.GetOwner
  func (s *Service) DebtRowPlans(ctx, budgetID, debtID vo.Id) (model.DebtBudgetPlans, bool, error)   // userID dropped
  ```
  `ReadModel`: `CountSpending`, `SpendingByMonth`, `BudgetTransactionsByCategories`, `BudgetTransactionsByTag` gain `budgetID vo.Id` right after `ctx`, and their join becomes `LEFT JOIN debts_payments dp ON dp.transaction_id = t.id AND dp.debt_id IN (SELECT d.id FROM debts d WHERE d.budget_id = <ph>)` (Ruling 16; mind the placeholder order on both engines). `BudgetTransactionsByDebt` filters `t.category_id IN (<rule categories>)` and the service keeps rows whose `(category, tag)` the view `CarriesKey`. `filters.debtOf` uses `CarriesKey`.
- Behaviour: `filtersWithDebts`, `syncElements`, `checkDebtPeriod` and `debtTimelines` use `b.debtScope()`; `TransferElementData` touches only `t.BudgetID`'s rows (zero `BudgetID` is a programming error → returns an error); revoke-access no longer deletes the leaving member's debt rows (debts stay, their rules match nothing on accounts that left); `CloneBudget` calls `CloneDebts` inside its transaction before copying elements and maps `ElementDebt` external ids through it; budget delete relies on the FK cascade. Server: `debtSvc.SetBudgets(NewDebtBudgets(budgetSvc))`, `NewDebtUsers(userRepo)`, `DebtClassifications.Classification`, `BudgetPlans` glue removed.

- [ ] **Step 1: Write the failing tests** (adapt `newAttrHarness` to create debts with `budgetId` + rules):
  - `TestGetBudget_DebtRowOnlyInItsOwnBudget` (the same participants share a second budget: no row there).
  - `TestGetBudget_PartnersCategoryPaymentsOnTheSharedDebt` (his + her category rules; both payments on the row, neither on its category row).
  - `TestGetBudget_PaymentMatchedInTwoBudgetsCountsOnceInEach` (Review Focus 1: one account in budgets A and B, a debt in each matching the payment; each budget's totals count it once, on its own debt row).
  - `TestTxList_DebtDrillDownListsEveryRule`; `TestTxList_CategoryDrillDownLeavesOutOnlyThisBudgetsDebtPayments`.
  - `TestTransferElementData_OnlyTheGivenBudget`.
  - `TestRevokeAccess_KeepsTheBudgetsDebts` (row stays, the leaver's payments stop counting).
  - `TestCloneBudget_CopiesDebtsAndTheirRows` (new debt ids, row external ids remapped, plans copied from the start month).
  - `TestDeleteBudget_DeletesItsDebts`.
  - `TestDebtBudget_RolesAndParticipants` (guest: `CanPlan` false and not a participant; pending invite → NotFound; archived flag; `CurrencyIDs` include element currencies) and `TestDebtRuleOptions_CallerFirstActiveExpenseOnly`.
  - In `transfer_lock_pgsql_test.go`: `TestAddDebtRule_ConcurrentSetLimitIsMovedOrWaits` (Review Focus 5) next to the existing transfer case, adapted to the budget lock.
  - Update the existing `OwnerUserId` assertions in `builder_debts_test.go` (field gone).
- [ ] **Step 2: Run** `go build ./... && go test ./internal/budget/...` — expect FAIL.
- [ ] **Step 3: Implement** the port, the read SQL, the builders, clone, revoke, access methods and the server wiring; check every caller of the changed `ReadModel` methods (`projected_read.go`, `dated_read.go`, `upcoming_merge.go`).
- [ ] **Step 4: Run** `go build ./... && go vet ./... && go test ./internal/... ` (smoke engine) and `make test-engines` + `make test-repo-pgsql` against a throwaway PostgreSQL — expect PASS except the parity goldens, which Task 6 regenerates.
- [ ] **Step 5: Commit** `feat(budget): A budget shows and plans only its own debts`.

### Task 6: API edge, swagger and parity suites

**Files:**
- Modify: `internal/debt/api/{debt.go,routes.go}`, `internal/web/apidoc/docs/*` (via `make swagger`), `internal/test/apiparity/{catalogue_debt.go,catalogue_budget_debts.go,catalogue_test.go,guard_test.go}`, goldens under `internal/test/apiparity/testdata/golden/` and `internal/test/mcpparity/testdata/golden/`
- Test: the parity suites, `internal/test/revisionguard/`

**Interfaces:**
- Produces routes (11): the nine existing plus `POST /api/v1/debt/add-debt-rule` (`endpoint.Handle`, body `AddDebtRuleRequest`, returns `DebtResult`) and `GET /api/v1/debt/get-debt-rule-options?budgetId=` (hand-written query read like `get-debt`, returns `DebtRuleOptionsResult`). `get-debt-forecast` reads `id` only.

- [ ] **Step 1: Rewrite the scenarios**: `debtBody` gains `"budgetId": Budget, "rules": [{"categoryId": CatFood}]`; `debt_crud`, `debt_currency_change`, `debt_validation` (+ `err:no-rules`, `err:eleven-rules`, `err:tag-and-label-in-a-rule`), `debt_forecast`, `debt_reconcile`, `debt_readonly`, `budget_debts` (partner's debt now in the shared budget). Replace `debt_guest_access` with `debt_budget_access`: guest before accepting → `err:` 404 on get; accepts → get (`canEdit` 1) and update succeed; owner re-grants role guest (2) (revoke + grant + accept if grant-access does not change a role) → get `canEdit` 0, `err:` 403 on update. Replace `debt_classification_guard` with `debt_classification_cleanup`: delete a category a rule names (succeeds, rule gone), delete a payee that narrows a rule (rule widened), payee delete making two debts' rules overlap (newer loses), merges that once conflicted now succeed. Add `debt_rules` (two-rule create with the partner's category, `add-debt-rule` success, `err:` add a covered category, `err:` add a conflicting one, update replace-set, `err:` `rules: []`) and `debt_rule_options` (owner; guest `err:` 403; stranger `err:` 404).
- [ ] **Step 2: Bump floors**: `TestCatalogueSize` min 98 → 100 (+`debt_rules`, `debt_rule_options`; two renamed), `minRoutes` 149 → 151, each with a dated comment line.
- [ ] **Step 3: Run** `go test ./internal/test/apiparity/ ./internal/test/mcpparity/ ./internal/test/revisionguard/` — expect FAIL (missing routes/goldens).
- [ ] **Step 4: Implement** the handlers and routes; `make swagger`.
- [ ] **Step 5: Regenerate** `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ ./internal/test/mcpparity/`, then INSPECT: debt bodies carry `budgetId`, `budgetName`, `canEdit`, `rules`; no `ownerUserId` on budget debt rows; no other golden changes except removed `used_by_debt` refusals.
- [ ] **Step 6: Run** `make go-test` and `make test-engines` — expect PASS.
- [ ] **Step 7: Commit** `feat(debt): Debt routes take a budget and rules; add-debt-rule and rule options`.

### Task 7: SPA — data layer, rule editor, create form, review screen, Settings list

**Files:**
- Create: `web/src/features/debts/rules.ts` (+ `rules.test.ts`), `web/src/features/debts/RulesEditor.tsx` (+ `RulesEditor.test.tsx`)
- Modify: `web/src/api/dto/debt.ts`, `web/src/api/debt.ts`, `web/src/app/queryKeys.ts`, `web/src/app/uiStore.ts`, `web/src/features/debts/{queries.ts,useDebtForm.ts,PaymentsFromEditor.tsx,DebtDialog.tsx,DebtSettingsForm.tsx,DebtReviewScreen.tsx,DebtBalanceTab.tsx,DebtsSettingsPage.tsx,testDebt.ts}` and their tests, `locales/*.json`

**Interfaces:**
- DTOs mirror Task 2: `DebtDto` (`budgetId`, `budgetName`, `canEdit: 0 | 1`, `rules: DebtRuleDto[]`, category/payee/tag/label ids gone); `DebtRuleFormDto { id?: Id; categoryId: Id; payeeId?: Id | null; tagId?: Id | null; labelId?: Id | null }`; `CreateDebtDto` + `budgetId`, `rules`, `currencyId?`; `UpdateDebtDto` + `rules?`; `AddDebtRuleDto { id: Id; rule: DebtRuleFormDto }`; `DebtRuleOptionsDto`.
- API: `addDebtRule(form: AddDebtRuleDto): Promise<DebtDto>`, `getDebtRuleOptions(budgetId: Id): Promise<DebtRuleOptionsDto>`, `getDebtForecast(id: Id)`.
- Queries: `queryKeys.debtRuleOptions(budgetId)`, `queryKeys.debtForecast(id)`; `useDebtRuleOptions(budgetId: Id | null)`; `useAddDebtRule()` (same invalidations as create; fires `METRICS.DEBT_RULE_ADD` in `onSuccess`); `openDebtDetails(id, tab?)` (no budget id).
- `rules.ts`:
  ```ts
  export type PaymentsFrom = { categoryId: Id | null; payeeId: Id | null; tagId: Id | null; labelId: Id | null } // moved from useDebtForm
  export function addRuleBlocker(debts: DebtDto[], debtId: Id, categoryId: Id): 'covered' | 'conflict' | null // client mirror of model.CheckDebtRules for a whole-category rule, debts of one budget
  export function ruleOwnerLabel(rule: DebtRuleDto, viewerId: Id | undefined, t: TFunction): string // "{owner}'s {category}" unless the viewer owns it
  export function rulesChanged(before: DebtRuleDto[], after: PaymentsFrom[]): boolean
  ```
- `RulesEditor { value: PaymentsFrom[]; onChange; options: DebtRuleOptionsDto | undefined; lockedCategoryId?: Id; error?: string; readOnly?: boolean }`: one `PaymentsFromEditor` sentence per rule, "+ Add a category" (hidden at 10), a remove button on every line while there are two or more; the category picker groups by participant (caller first) with `rankByName`; payee/tag/label pickers list the chosen category owner's options. Read-only renders the sentences with `ruleOwnerLabel`.
- Create form (`DebtDialog`): from a budget row the budget is `OpenDebtParams.budgetId` and the first rule's category is locked; from Settings a budget picker (`PickerSheet`) lists budgets the caller can plan in and that are not archived, default the `budget` user option when it is one of them, else the first. `currencyId` is sent only when known from the row. `buildCreatePayload` sends `budgetId` and `rules`.
- Settings form: `buildTermsPayload` sends `rules` only when `rulesChanged`; the field error key `rules` shows under the editor.
- Review screen: `canEdit` 0 → tabs Overview and Balance only; Balance shows the history without the reconcile form or delete buttons; Overview's "Which payments count" lists every rule read-only. Settings → Debts groups rows under a heading per `budgetName` (server order), open debts first within a group; Create disabled with `debts.settings.no_budget` when no plan-capable unarchived budget exists.
- New locale keys (all 11): `debts.rules.add_category`, `debts.rules.remove`, `debts.rules.owned_category` (`"{owner}'s {category}"`), `debts.rules.heading`, `debts.form.budget`, `debts.settings.no_budget`.

- [ ] **Step 1: Write the failing tests**:
  - `rules.test.ts`: `addRuleBlocker` → `'covered'` when the picked debt has any rule on the category; `'conflict'` when another debt has a non-label rule on it with the same start date or a narrowed rule; `null` when another debt's whole rule starts on a different date; `ruleOwnerLabel` names the owner only for another user's rule.
  - `RulesEditor.test.tsx`: add a second rule, remove the first (the remove button disappears at one rule), category picker shows the partner's group after the caller's, the payee picker lists the partner's payees once the partner's category is chosen, no "+ Add" at 10.
  - `DebtDialog.test.tsx`: from Settings the budget picker defaults to the `budget` option and the payload carries `budgetId` and both rules; from a budget row the budget picker is absent and the first category locked.
  - `DebtReviewScreen.test.tsx`: `canEdit: 0` shows no Settings or Danger zone tab and no reconcile form; rules read-only with "Anna's Mortgage".
  - `DebtSettingsForm.test.tsx`: a terms-only save sends no `rules`; editing a rule sends all rules.
  - `DebtsSettingsPage.test.tsx`: two budgets → two headed groups; no plan-capable budget → Create disabled.
- [ ] **Step 2: Run** `cd web && pnpm test src/features/debts` — expect FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `cd web && pnpm test && pnpm lint && pnpm exec tsc --noEmit` — expect PASS (`metrics-coverage`, `offline-coverage` and i18n key coverage included).
- [ ] **Step 5: Commit** `feat(debt): Rule lists, budget-grouped debts and a read-only review for guests`.

### Task 8: SPA — budget row menus and "Add to a debt…"

**Files:**
- Create: `web/src/features/debts/AddToDebtDialog.tsx` (+ `AddToDebtDialog.test.tsx`)
- Modify: `web/src/features/budgets/useBudgetLineMenus.tsx`, `web/src/features/budgets/debtRow.tsx`, `web/src/api/dto/budget.ts` (`ownerUserId` off the debt row DTOs), `web/src/lib/metrics.ts`, `web/src/features/budgets/useBudgetLineMenus.debts.test.tsx`, `MonthFlows.debts.test.tsx`, `PhoneMonthView.debts.test.tsx`, `PlanSheet.debts.test.tsx`, `locales/*.json`

**Interfaces:**
- Consumes: Task 7's `useAddDebtRule`, `addRuleBlocker`, `useDebts`.
- `AddToDebtDialog { budgetId: Id; categoryId: Id; open: boolean; onOpenChange(open: boolean): void }`: `ResponsiveDialog` on desktop, `PickerSheet` on a phone; lists `useDebts()` filtered to `budgetId` with icon, name and "Payments in …" (the rule categories joined by ", ", via `ruleOwnerLabel`); a search field with `rankByName` when more than 6 debts; entries with `addRuleBlocker` non-null are disabled with `debts.add_to.covered` / `debts.add_to.conflict`; a pick sends `add-debt-rule {id, rule: {categoryId}}`, closes on success, shows the refusal inline (`apiErrorMessage`); reads `useOnline` (disabled offline).
- Menu rules (Ruling 18), with `canPlan = editDetails` (`canEditBudget` on an unarchived budget): "Track as debt" on any live expense category row (envelope children included) when `canPlan`, regardless of `ownerUserId`; "Add to a debt…" (`budgets.page.debts.add_to`) next to it when `canPlan` and the budget has at least one debt; `debtRowMenu`: Details for everyone, Mark as paid off / Delete only when `canPlan`; `DebtLeft` shows Finish setup when `canPlan`, else a dash.
- `METRICS.DEBT_RULE_ADD: 'appDebtRuleAdd'`. New locale keys: `budgets.page.debts.add_to`, `debts.add_to.title`, `debts.add_to.covered`, `debts.add_to.conflict`, `debts.add_to.payments_in` (`"Payments in {categories}"`), `debts.add_to.search`.

- [ ] **Step 1: Write the failing tests**:
  - `AddToDebtDialog.test.tsx`: lists only the budget's debts with "Payments in his Mortgage, Anna's Mortgage"; a covered debt is disabled with the reason; picking sends `{ id, rule: { categoryId } }`, fires `appDebtRuleAdd` and closes; a 400 shows the server message and stays open; offline every entry is disabled.
  - `useBudgetLineMenus.debts.test.tsx`: a partner's category offers "Track as debt" and "Add to a debt…" to a `user`-role participant; a guest gets neither and gets only Details on a debt row; an archived budget offers neither; "Add to a debt…" is absent when the budget has no debt.
  - `MonthFlows.debts.test.tsx` / `PhoneMonthView.debts.test.tsx`: Finish setup for a plan-capable participant who did not create the debt; dash for a guest.
- [ ] **Step 2: Run** `cd web && pnpm test src/features/budgets src/features/debts` — expect FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `cd web && pnpm test && pnpm lint && pnpm exec tsc --noEmit` — expect PASS.
- [ ] **Step 5: Commit** `feat(debt): Track any participant's category as a debt, or add it to one`.

### Task 9: Documentation and the branch review

**Files:**
- Modify: `CLAUDE.md` ("Budget debts", "Debt SPA flow", onboarding "Debt detection", the `debt` feature paragraph's route count and list, the debt glue sentence, the `used_by_debt` mentions under Classifications), `docs/regression-test-plan.md` (§ Debts and the onboarding debt items near line 473), `docs/superpowers/specs/2026-10-29-budget-debts-design.md` and `2026-11-07-debt-setup-redesign-design.md` (a one-line "Amended by `2026-11-12-shared-budget-debts-design.md`" pointer under each header)

- [ ] **Step 1: Update CLAUDE.md**: a debt belongs to a budget (no owner); access by budget role; rules (1..10, any participant's category, per-rule conflicts, one range per debt, oldest-debt tiebreak, cleanup on classification delete/merge, no `*.used_by_debt`); only member accounts count; budget row lock order; `debts_payments` keyed `(debt_id, transaction_id)` with budget-scoped joins; plan moves per whole-category rule and give-back only for a sole whole-category rule; clone copies debts; 11 routes incl. `add-debt-rule` and `get-debt-rule-options`; forecast follows the debt's budget; SPA rule editor, "Add to a debt…", guest read-only review, Settings grouped by budget; onboarding creates one-rule debts in the step's budget.
- [ ] **Step 2: Update the regression plan**: Settings → Debts grouped by budget and the budget picker; a partner sees and edits the shared debt; a guest sees a read-only review; "Track as debt" on a partner's category; "Add to a debt…" (disabled entries, plans add up); deleting a category/payee used by a debt succeeds and what happens to the rule; payments on an account outside the budget do not count; clone copies debts; keep the 📱 markers.
- [ ] **Step 3: Add the two spec pointers.**
- [ ] **Step 4: Run** `make test` (full: smoke + engines + upgrade + frontend) — expect PASS.
- [ ] **Step 5: Commit** `docs(debt): Debts that belong to a budget`.
- [ ] **Step 6: Push the branch and run the `agent-review` skill** (the user's global instruction for every verified change); fix what the reviewers find, each fix with its own test.
