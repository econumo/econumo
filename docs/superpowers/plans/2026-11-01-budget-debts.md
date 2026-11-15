# Budget Debts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track loans (mortgage, car loan, personal loan) as debts tied to an expense category, so that each loan shows its balance, payments left, money left and overpayment, and appears in the Budget and the Plan as a Debts block.

**Architecture:** The debt math (due dates, implied rate, per-payment split, projection) is pure code in `internal/model/debt_math.go`. A new feature package, `internal/debt`, stores debts, snapshots and per-payment splits, and keeps the splits in sync by materializing them lazily before every read. The budget gains element type `ElementDebt = 6`. It reads debts through a `DebtSource` port and attributes matched payments to debt rows through a `debts_payments` join in its spending SQL. The SPA adds a Debts block to the month view and the Plan grid, a debt form, a details sheet and a Settings list.

**Tech Stack:** Go (sqlc, sqlite + PostgreSQL), React 19 + TanStack Query, vitest.

**Spec:** `docs/superpowers/specs/2026-10-29-budget-debts-design.md`. Read it before any task. This plan argues from it.

## Global Constraints

- Follow `CLAUDE.md` throughout: features never import features, so cross-feature access goes through consumer ports with glue in `internal/server`; there are only two methods, GET and POST; handlers are thin `endpoint.Handle` wrappers with swag blocks (`@Failure 401/402/500` on POST, `401/500` on GET); `.sql` files stay ASCII-only; comments are sparse.
- Migrations: `internal/infra/storage/migrations/{sqlite,pgsql}/20261110000500.sql`, the same version on both engines (`20261101000000` is taken by the `migration:zero-deleted-accounts` command step; a new migration must sit above every released version, see CLAUDE.md "command steps"). Regenerate sqlc with `go generate ./internal/infra/storage/sqlc/...` (install `sqlc` at the version in `go.mod` if it is missing).
- Money: `vo.DecimalNumber` strings on the wire and in storage. The math runs in `float64`, and every stored or emitted amount is rounded to the debt currency's fraction digits (`RateProvider.FractionDigits`).
- The element type value `6` and alias `"debt"` are persisted, so they are frozen.
- Every new coded error is added to `errs` (constant + `AllCodes`) and has `errors.*` text in all 11 `locales/*.json`. Every new UI key is added to all 11 locales, respecting each locale's register (de Sie, nl u, fr vous, pt European).
- Analytics: `METRICS.DEBT_CREATE`, `DEBT_UPDATE`, `DEBT_DELETE`, `DEBT_CONVERT`, `DEBT_MARK_PAID_OFF`, fired at the mutation's success point.
- Go toolchain: `/usr/local/go/bin/go` with `GOTOOLCHAIN=go1.27.1`. The smoke gate is `make go-test`; `make test` also runs the PostgreSQL comparison.
- UI house rules: no bold for hierarchy; every type-to-filter picker uses `PickerSheet` + `rankByName`; amounts carry their currency symbol.
- Branch: `feature/budget-debts` (it already holds the spec commit).

## Review Focus

1. **Back-dated or edited payments after later payments were split.** A payment inserted before already-split payments must take its interest from the running balance at its own date and must not re-split the later ones. A payment whose amount is edited keeps its stored interest, capped at the new amount. Test: `TestSync_BackdatedPaymentKeepsLaterSplits` in Task 3.
2. **Payments in a currency other than the debt's.** They are converted with the rates the app uses for the payment's day (the month's average, like every other conversion) before splitting and before counting Paid, so no raw foreign amount leaks into the balance. Tests: `TestSync_ConvertsForeignCurrencyPayment` in Task 3 and `TestGetBudget_DebtPaidConverted` in Task 7.
3. **A category used by both a debt and a tag row in a budget.** A matched payment that carries a tag must leave the tag row too, never only the category row. Test: `TestGetBudget_DebtPaymentLeavesTagRow` in Task 7.
4. **Debt with nothing recorded yet (new loan, first payment next month).** Paid is 0, Left equals the balance, payments left counts the future schedule, and no division by zero occurs. Test: `TestProjectDebt_NoPaymentsYet` in Task 1.
5. **A shared budget whose partner owns the debt.** The guest sees the row and its figures, but `get-debt` and every debt write answer as not-found for them. Tests: the apiparity scenario `debt_guest_access` in Task 4 and `TestGetBudget_PartnerDebtRowVisible` in Task 7.

---

### Task 1: Debt math in the model

**Files:**
- Create: `internal/model/debt.go` (entities + `DebtMatches`), `internal/model/debt_math.go`
- Test: `internal/model/debt_math_test.go`, `internal/model/debt_test.go`

**Interfaces:**
- Produces:
  ```go
  type Debt struct {
      ID, OwnerID, CurrencyID, CategoryID vo.Id
      Name       string
      PayeeID    *vo.Id
      TagID      *vo.Id
      StartDate  *time.Time // date only, UTC midnight
      Current    DebtTerms  // copy of the latest snapshot's values (agreed)
      CreatedAt, UpdatedAt time.Time
  }
  type DebtTerms struct {
      AsOf            time.Time // date
      Balance         vo.DecimalNumber
      Payment         vo.DecimalNumber
      Schedule        RecurringSchedule
      LastPaymentDate time.Time // date
  }
  type DebtSnapshot struct {
      ID, DebtID      vo.Id
      AsOf            time.Time // date
      Balance         vo.DecimalNumber
      Payment         vo.DecimalNumber
      Schedule        RecurringSchedule
      LastPaymentDate time.Time // date; its Day() anchors month-based schedules
      CreatedAt       time.Time
  }
  type DebtSplit struct { DebtID, TransactionID vo.Id; Interest vo.DecimalNumber }

  // Matching key + range, shared by the debt service, budget SQL glue and projectedRead.
  type DebtView struct {
      ID, OwnerID, CurrencyID, CategoryID string
      Name, Icon        string
      PayeeID, TagID    *string
      From              *time.Time // inclusive; nil = unbounded
      Until             *time.Time // exclusive (next same-key debt's start); nil = unbounded
      WholeCategory     bool       // no payee, no tag, no start date
      StartMonth, EndMonth time.Time // planning bounds (first-of-month), see Task 3
      PaidOff           bool
  }
  func DebtMatches(d DebtView, categoryID, payeeID, tagID *string, at time.Time) bool

  // math (float64, debt currency)
  type MathSnapshot struct { AsOf time.Time; Balance, Payment float64; Schedule RecurringSchedule; Last time.Time }
  type MathPayment  struct { ID string; Date time.Time; Amount float64; Interest *float64 /* stored */ }
  var ErrPaymentsDontCover = errors.New("payments do not cover the balance")
  func DueDatesBetween(s RecurringSchedule, last time.Time, from, to time.Time) []time.Time // due dates d with from < d <= to, extended past `last` with NextOccurrence
  func DueWindow(s RecurringSchedule, last, due time.Time) (start, end time.Time)          // [start,end)
  func ImpliedRate(balance, payment float64, n int) (float64, error)
  func SplitPayments(snaps []MathSnapshot, pays []MathPayment) map[string]float64          // interest for EVERY payment (stored ones echo their stored value, capped at Amount)
  func BalanceAt(snaps []MathSnapshot, pays []MathPayment, interest map[string]float64, t time.Time) float64
  type ProjectionInput struct {
      Snaps []MathSnapshot; Pays []MathPayment; Interest map[string]float64
      Today time.Time; Months []time.Time /* first-of-month, ascending */; Plans map[string]float64 /* "YYYY-MM-01" -> planned */
  }
  type ProjectionMonth struct { ClosingBalance float64; PaymentsLeft int; ScheduledTotal, StillDue float64 }
  type Projection struct {
      CurrentBalance float64; PaymentsLeft int; MoneyLeft, OverpaymentLeft, OverpaidSoFar float64
      PayoffDate, BasePayoffDate time.Time; BaseOverpaymentLeft float64
      Months map[string]ProjectionMonth
      PaidOff bool; PayoffMonth time.Time
  }
  func ProjectDebt(in ProjectionInput) Projection
  ```

These algorithms are not determined by the signatures, so they are pinned here:

- **Due dates:** walk back from `last`. Month-based schedules use `last.Day()` clamped to the month's length (31 → Feb 28 → Mar 31). Weekly and biweekly step back 7 or 14 days. Past `last`, extend forward with `NextOccurrence(d, s, int16(last.Day()))`.
- **Window:** `start = prev + (due-prev)/2` and `end = due + (next-due)/2`, both truncated to the day.
- **Implied rate:** bisection on `r ∈ [0, 1]`, 200 iterations, solving `payment·(1-(1+r)^-n)/r = balance`. Return `0` when `|payment·n − balance| < 0.005`, and `ErrPaymentsDontCover` when `payment·n < balance`. `n = len(DueDatesBetween(s, last, asOf, last))`.
- **Split, forward segment** (payments with `snap_k.AsOf < date ≤ snap_{k+1}.AsOf`, the last snapshot open-ended): group them by due window in date order. Each window's `due = B·r`, where `B` is the running balance when the window opens. The window containing `AsOf` counts as already accrued when any payment of the debt dated `≤ AsOf` lies in it. For each payment in date order:
  - its interest is `min(stored, amount)` when stored, else `min(max(due,0), amount)`;
  - then `due -= interest` and `B -= amount − interest`.
  - Periods with no payment accrue nothing ("stay flat").
- **Split, backward segment** (payments with date ≤ `snaps[0].AsOf`): process windows in reverse with `B_end` = the balance after the window (starting from the snapshot balance) and `T` = the window's total.
  - If `T ≥ B_start·r` with `B_start = (B_end+T)/(1+r)`, then `due = B_start·r`.
  - Otherwise `due = T` and `B_start = B_end`.
  - Allocate `due` across the window's payments in date order (stored values win), then set `B_end = B_start` for the previous window.
- **BalanceAt(t):**
  - With the latest snapshot `S` where `S.AsOf ≤ t`: `S.Balance − Σ(amount−interest)` over payments in `(S.AsOf, t]`.
  - When `t` precedes every snapshot: `snaps[0].Balance + Σ(amount−interest)` over payments in `(t, snaps[0].AsOf]`.
- **Projection:**
  - Start from `CurrentBalance = BalanceAt(latest, Today)`, using `r` and `Payment` of the latest snapshot.
  - **First unpaid due:** the earliest due date at or after the due date of Today's window whose window holds no payment dated `≤ Today`.
  - **Each projected due:** `interest = B·r`; `pay = min(Payment, B+interest)`; `B = B + interest − pay`. It stops when `B ≤ 0.005`.
  - **Extra per month** `m ≥ month(Today)`: `max(0, Plans[m] − ScheduledTotal(m))`. It is applied fully to principal on the month's first projected due (or at month end when the month has none), before that due's interest.
  - **Base:** the same run with no extras gives `BasePayoffDate` and `BaseOverpaymentLeft`.
  - **Per month:**
    - `ScheduledTotal = Payment × dueDatesInMonth`.
    - `StillDue` = the projected regular payments whose due falls in the month.
    - `ClosingBalance` is `BalanceAt(monthEnd)` for months before Today's month, and the projected balance after the month's last projected step otherwise.
    - `PaymentsLeft` is the number of projected payments due after month end.
  - **Totals:** `MoneyLeft = Σ projected pays`; `OverpaymentLeft = MoneyLeft − CurrentBalance`; `OverpaidSoFar = Σ interest of payments ≤ Today`.
  - **Paid off:** `PaidOff = round(CurrentBalance,2) ≤ 0` and no payment is dated after the payment or zero snapshot that took it there (a later payment reopens it). `PayoffMonth` is the month of the payment, or of the zero snapshot, that took the balance to ≤ 0.
  - A negative `CurrentBalance` is reported as 0.
- **DebtMatches:** the category must be equal; a non-nil `PayeeID` / `TagID` must also be equal; and `From ≤ at < Until`.

- [ ] **Step 1: Write the failing tests** in `debt_math_test.go`, using the reference loan `B=300000, P=2000, monthly, last = asOf + 240 months`:
  - `TestImpliedRate_ReferenceMortgage`: `r ≈ 0.0042676` (±1e-6)
  - `TestImpliedRate_InterestFree`: `ImpliedRate(12000, 1000, 12) == 0`
  - `TestImpliedRate_DoesNotCover`: `ImpliedRate(12000, 900, 12)` returns `ErrPaymentsDontCover`
  - `TestSplit_FirstRegularPayment`: one 2000 payment in the first window → interest `1280.29` (±0.01)
  - `TestSplit_ExtraPaymentGoesToDebt`: a 5000 payment in the first window → interest `1280.29`, debt part `3719.71`
  - `TestSplit_SecondPaymentSameWindowNoInterest`: 2000 + 3000 in one window → the second has interest 0
  - `TestSplit_EarlyPaymentLandsOnItsDue`: due on the 1st, paid on the 28th of the previous month → it takes that due's interest
  - `TestSplit_StoredInterestKeptAndCapped`: stored 1280.29 on a payment edited to 1000 → interest 1000
  - `TestSplit_MissedWindowStaysFlat`: the balance is unchanged across a window with no payment
  - `TestSplit_BackwardBeforeFirstSnapshot`: two payments before the snapshot → `BalanceAt` before them is greater than the snapshot balance, and their interest is > 0
  - `TestProjectDebt_Reference`: no payments, no plans → `PaymentsLeft 240`, `MoneyLeft 480000` (±1), `OverpaymentLeft 180000` (±1)
  - `TestProjectDebt_PlannedExtraShortens`: plan 5000 in the current month → `PaymentsLeft 236`, `MoneyLeft ≈ 474754.34` (±1), `BasePayoffDate` 4 dues after `PayoffDate`
  - `TestProjectDebt_NoPaymentsYet`: first due next month → `StillDue` of the current month is 0, the next month's is 2000, and `ClosingBalance` of the current month is 300000
  - `TestProjectDebt_PaidOff`: a snapshot with balance 0 → `PaidOff`, `PayoffMonth` = the snapshot month, `PaymentsLeft 0`
  - `TestProjectDebt_PaymentAfterZeroSnapshotReopens`: a zero snapshot, then a 500 payment → `PaidOff` is false
  - `TestDueDatesBetween_MonthEndClamp`: last = Mar 31 → includes Feb 28 and Jan 31
  - In `debt_test.go`, `TestDebtMatches`: table over category, payee and tag nil/set and the `From`/`Until` edges (`From` inclusive, `Until` exclusive).
- [ ] **Step 2: Run them and confirm they fail.** Run `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/model/ -run 'Debt|Implied|Split|Project|DueDates'`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement** the types and functions above in `debt.go` and `debt_math.go`.
- [ ] **Step 4: Run the tests again.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt schedule math`).

### Task 2: Storage — migration, sqlc queries, repo

**Files:**
- Create: `internal/infra/storage/migrations/{sqlite,pgsql}/20261110000500.sql`, `internal/infra/storage/sqlc/query/{sqlite,pgsql}/debts.sql`, `internal/debt/repository.go`, `internal/debt/repo/{repo.go,sqlite.go,pgsql.go}`
- Test: `internal/debt/repo/repo_test.go`

**Interfaces:**
- Consumes: Task 1 entities.
- Produces (`internal/debt/repository.go`):
  ```go
  type CandidateTx struct { ID vo.Id; SpentAt time.Time; Amount vo.DecimalNumber; CurrencyID vo.Id; PayeeID, TagID *vo.Id }
  type Repository interface {
      NextIdentity() vo.Id
      LockOwner(ctx context.Context, ownerID vo.Id) error // SELECT ... FOR UPDATE on users row (pgsql), no-op sqlite
      GetByID(ctx context.Context, id vo.Id) (*model.Debt, error) // errs.NotFound
      ListByOwners(ctx context.Context, ownerIDs []vo.Id) ([]*model.Debt, error)
      ListByCategory(ctx context.Context, categoryID vo.Id) ([]*model.Debt, error)
      Save(ctx context.Context, d *model.Debt) error
      Delete(ctx context.Context, id vo.Id) error
      AddSnapshot(ctx context.Context, s *model.DebtSnapshot) error // the service always pairs it with Save of the same terms in one tx
      Snapshots(ctx context.Context, debtIDs []vo.Id) ([]*model.DebtSnapshot, error) // ordered debt_id, as_of, created_at
      Splits(ctx context.Context, debtIDs []vo.Id) ([]model.DebtSplit, error)
      InsertSplit(ctx context.Context, s model.DebtSplit) error // ON CONFLICT (transaction_id) DO NOTHING
      DeleteSplits(ctx context.Context, debtID vo.Id, transactionIDs []vo.Id) error
      CandidateTransactions(ctx context.Context, categoryID vo.Id) ([]CandidateTx, error) // type=0 only, joins accounts for currency_id
      UsesCategory(ctx context.Context, id vo.Id) (bool, error)
      UsesPayee(ctx context.Context, id vo.Id) (bool, error)
      UsesTag(ctx context.Context, id vo.Id) (bool, error)
      ReassignCategory(ctx context.Context, oldID, newID vo.Id) error
      ReassignPayee(ctx context.Context, oldID, newID vo.Id) error
      ReassignTag(ctx context.Context, oldID, newID vo.Id) error
  }
  ```
- Tables:
  - `debts(id, user_id → users CASCADE, name, currency_id → currencies, category_id → categories RESTRICT, payee_id → payees RESTRICT NULL, tag_id → tags RESTRICT NULL, start_date DATE NULL, as_of DATE, balance NUMERIC(19,8), payment NUMERIC(19,8), schedule TEXT, last_payment_date DATE, created_at, updated_at)`. The five term columns are a copy of the latest snapshot, written together with every new snapshot.
  - `debts_snapshots(id, debt_id → debts CASCADE, as_of DATE, balance NUMERIC(19,8), payment NUMERIC(19,8), schedule TEXT, last_payment_date DATE, created_at)`
  - `debts_payments(debt_id → debts CASCADE, transaction_id → transactions CASCADE UNIQUE, interest NUMERIC(19,8))`
  - Indexes: `debts(user_id)`, `debts(category_id)`, `debts_snapshots(debt_id, as_of)`, `debts_payments(debt_id)`.
  - DATE columns are bound as `Y-m-d` text on sqlite, the same way `currencies_rates.published_at` is.

- [ ] **Step 1: Write the failing tests** in `repo_test.go` (`dbtest.New(t)` + `fixture`):
  - `TestRepo_SaveGetDebtRoundTrip` (including `Current` terms)
  - `TestRepo_SnapshotsOrdered`
  - `TestRepo_InsertSplitIsIdempotent`: inserting twice leaves one row
  - `TestRepo_SplitDeletedWithTransaction`: FK cascade
  - `TestRepo_CandidateTransactionsExpensesOnly`: an income and a transfer in the category are excluded, and `CurrencyID` is the account's
  - `TestRepo_UsesAndReassignCategory`
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/debt/repo/`. Expected: FAIL.
- [ ] **Step 3: Write the migrations and queries, run `go generate ./internal/infra/storage/sqlc/...`, and implement the repo** following the `internal/tag/repo` adapter pattern.
- [ ] **Step 4: Run the repo tests on both engines.** Run `go test ./internal/debt/repo/` and `make test-repo-pgsql`. Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt storage`).

### Task 3: Debt service — CRUD, validation, sync, timelines

**Files:**
- Create: `internal/debt/{usecase.go,ports.go,create.go,update.go,delete.go,read.go,sync.go,timeline.go}`, `internal/model/debt_dto.go`
- Modify: `internal/shared/errs/codes.go`, `locales/*.json` (`errors.debt.*`)
- Test: `internal/debt/{create,update,delete,sync,timeline}_test.go`

**Interfaces:**
- Consumes: Task 1 math and Task 2 `Repository`.
- Ports (`ports.go`):
  ```go
  type Classifications interface { // glue over category/payee/tag repos
      CategoryOf(ctx context.Context, ownerID, id vo.Id) (name, icon string, isExpense bool, err error) // errs.NotFound when not owned
      PayeeOwned(ctx context.Context, ownerID, id vo.Id) (bool, error)
      TagOwned(ctx context.Context, ownerID, id vo.Id) (bool, error)
  }
  type Rates interface {
      ConvertOnDay(ctx context.Context, from, to vo.Id, amount vo.DecimalNumber, day time.Time) (vo.DecimalNumber, error)
      FractionDigits(ctx context.Context, currencyID vo.Id) (int, error)
  }
  type BudgetElements interface { // glue over the budget service (Task 8); nil-safe until set
      TransferData(ctx context.Context, fromID vo.Id, fromType int16, toID vo.Id, toType int16, currencyID vo.Id) error
      HasData(ctx context.Context, externalID vo.Id, typ int16) (bool, error)
  }
  ```
  `Service.SetBudgetElements(BudgetElements)` installs the last one late.
- Service methods (all `(ctx, userID vo.Id, req) (res, error)` for `endpoint.Handle`):
  - `CreateDebt(req model.CreateDebtRequest) (*model.DebtResult, error)`
  - `UpdateDebt(req model.UpdateDebtRequest)`
  - `DeleteDebt(req model.DeleteDebtRequest) (*model.DeleteDebtResult, error)`
  - `MarkDebtPaidOff(req model.MarkDebtPaidOffRequest)`
  - `GetDebtList(ctx, userID) (*model.DebtListResult, error)`
  - `GetDebt(req model.GetDebtRequest) (*model.DebtDetailsResult, error)`
  - Port-facing: `Sync(ctx, ownerIDs []vo.Id) error`, `Views(ctx, ownerIDs []vo.Id) ([]model.DebtView, error)`, `Timelines(ctx, req model.DebtTimelineRequest) (map[string]model.DebtTimeline, error)`.
- DTOs (`debt_dto.go`):
  - `CreateDebtRequest{Name, CurrencyId, CategoryId string; PayeeId, TagId, StartDate *string; Balance, Payment vo.FlexString; Schedule, LastPaymentDate string}`
  - `UpdateDebtRequest`: the same fields + `Id`. A snapshot is written only when balance, payment, schedule or last payment date differ from the latest snapshot.
  - `DeleteDebtRequest{Id string; ConfirmDataRemoval bool}`
  - `MarkDebtPaidOffRequest{Id}`, `GetDebtRequest{Id}`.
  - `DebtResult`: id, name, icon (the category's), currencyId, categoryId, payeeId, tagId, startDate (`""` when none), balance, payment, schedule, lastPaymentDate (all from the latest snapshot); currentBalance, paymentsLeft (int), moneyLeft, overpaymentLeft, overpaidSoFar, payoffDate, nextDueDate, periodRate (string, 10 decimals, for the client's form recalculation); isPaidOff (0/1).
  - `DebtDetailsResult{Item DebtResult; Snapshots []DebtSnapshotResult{AsOf, Balance, Payment, Schedule, LastPaymentDate}; Payments []DebtPaymentResult{TransactionId, Date, Amount, Interest, Principal}}`, payments newest first.
  - `DebtTimelineRequest{DebtIDs []string; Months []time.Time; Plans map[string]map[string]string /* debtID -> month -> amount */; Today time.Time}`
  - `DebtTimeline{Months map[string]DebtTimelineMonth{ClosingBalance string; PaymentsLeft int; ScheduledTotal, StillDue string}; PayoffDate, BasePayoffDate, OverpaymentLeft, BaseOverpaymentLeft string}`
- New error codes (constant names `CodeDebt…`):
  - `debt.payments_dont_cover` (field `payment`)
  - `debt.key_conflict` (field `categoryId`): another debt could match the same transaction
  - `debt.start_date_taken` (field `startDate`)
  - `debt.category_not_expense` (field `categoryId`)
  - `debt.removal_unconfirmed`
  - `debt.last_payment_in_past` (field `lastPaymentDate`)

Rules the implementer cannot infer:

- **Ownership:** a debt is visible and writable to its owner only. Any other caller gets `errs.NotFound` (the frozen not-found behaviour).
- **Key checks** (create, update and Task 5's reassign), over the owner's debts on the same category:
  - two debts conflict when their keys overlap: `(payee, tag)` equal, or either side has nil where the other has a value (`key_conflict`);
  - identical keys must have distinct start dates (`start_date_taken`).
  - `From`/`Until` come from partitioning the identical-key debts by start date.
- **Planning bounds** (`DebtView.StartMonth`/`EndMonth`):
  - start = month(StartDate), else month(first matched payment), else month(first snapshot `AsOf`);
  - end = `PayoffMonth` when paid off, else month(latest `LastPaymentDate`).
- **Sync(ownerIDs):** in one `WithTx` per category:
  1. Load the category's debts and `CandidateTransactions`.
  2. Assign each candidate to the debt `DebtMatches` selects.
  3. Delete splits whose transaction no longer matches their debt.
  4. Convert each newly matched amount with `Rates.ConvertOnDay`.
  5. Run `SplitPayments` with the stored interests as `Interest` and insert only the new ones, rounded.

  Read paths call `Sync` first.
- **Current terms:** every write that adds a snapshot (create, update with changed values, mark paid off) sets `Debt.Current` to the same values and saves both inside one `WithTx`. Reads of current terms use `Debt.Current`; the snapshot history is read only for splits, backward estimates and the details view.
- **Mark paid off:** adds a snapshot as of today with balance 0, keeping the latest payment, schedule and date (no rate solve, since r is irrelevant at 0).
- **Create from a whole-category debt:** after saving, call `BudgetElements.TransferData(categoryID, 1, debtID, 6, currencyID)`.
- **Delete:**
  - Whole-category debt: `TransferData(debtID, 6, categoryID, 1, currencyID)`, then delete.
  - Otherwise, when `HasData(debtID, 6)` is true and the request has no `ConfirmDataRemoval` → `debt.removal_unconfirmed`.

- [ ] **Step 1: Write the failing tests** (sqlite `dbtest`, a fake `Rates` with 1:1 except a set EUR rate, a fake `BudgetElements` recording calls):
  - `TestCreateDebt_ValidatesAndSnapshots`
  - `TestCreateDebt_RefusesIncomeCategory`
  - `TestCreateDebt_RefusesPaymentsDontCover`
  - `TestCreateDebt_KeyConflict` (category-only vs category+payee)
  - `TestCreateDebt_SameKeyNeedsDistinctStartDates`
  - `TestCreateDebt_WholeCategoryTransfersPlans`: the fake records `(cat,1,debt,6,cur)`
  - `TestUpdateDebt_SnapshotOnlyWhenValuesChange`
  - `TestDebtCurrentMatchesLatestSnapshot`: after create, update, mark paid off and a no-op update, `Debt.Current` equals the latest snapshot's values
  - `TestUpdateDebt_RematchKeepsStoredSplits`
  - `TestDeleteDebt_PartialNeedsConfirmation`
  - `TestDeleteDebt_WholeCategoryTransfersBack`
  - `TestMarkPaidOff_ZeroSnapshot`
  - `TestSync_SplitsNewPayments`
  - `TestSync_BackdatedPaymentKeepsLaterSplits`: split two payments, insert a third dated earlier, Sync → the first two interests are unchanged and the third > 0
  - `TestSync_ConvertsForeignCurrencyPayment`
  - `TestSync_UnmatchedPaymentLosesSplit`
  - `TestSync_PartitionByStartDate`: an old and a new car loan on one category
  - `TestGetDebt_OtherUserNotFound`
  - `TestTimelines_PlansMoveProjection`
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/debt/`. Expected: FAIL.
- [ ] **Step 3: Implement** the service, the DTO `Validate()` methods (blank checks, schedule via `ParseRecurringSchedule`, dates `YYYY-MM-DD`, positive amounts), the error codes and the `errors.debt.*` text in 11 locales.
- [ ] **Step 4: Run the tests.** Run `go test ./internal/debt/ ./internal/test/i18ntest/`. Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt service`).

### Task 4: Debt API, wiring, apiparity

**Files:**
- Create: `internal/debt/api/{handler.go,routes.go,debt.go}`, `internal/server/glue_debt.go`, `internal/test/apiparity/catalogue_debt.go`, goldens
- Modify: `internal/server/server.go`, `internal/test/apiparity/{guard_test.go,catalogue_test.go}`, the swagger docs (`make swagger`), `CLAUDE.md` (feature list: seventeen features incl. `debt`, routes)

**Interfaces:**
- Routes (all authenticated):
  - `GET /api/v1/debt/get-debt-list`
  - `GET /api/v1/debt/get-debt?id=`
  - `POST /api/v1/debt/create-debt`
  - `POST /api/v1/debt/update-debt`
  - `POST /api/v1/debt/delete-debt`
  - `POST /api/v1/debt/mark-debt-paid-off`
- `glue_debt.go`: `DebtClassifications` (over the category/payee/tag repos) and `DebtRates` (`ConvertOnDay` built like `ImportsCurrencyConverter.Convert`, with the `[day, day+1)` window).

- [ ] **Step 1: Write the apiparity scenarios** in `catalogue_debt.go`:
  - `debt_crud`: create → list → get → update → mark paid off → delete
  - `debt_validation` (`err:` calls for each coded error)
  - `debt_guest_access`: a connected guest calls `get-debt` and `update-debt` on the owner's debt → `err:` not-found
  - `debt_readonly`: a readonly user's `create-debt` → 402

  Raise `minRoutes` by 6 and the scenario `min` by 4, each with a dated comment.
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/test/apiparity/`. Expected: FAIL (routes not registered).
- [ ] **Step 3: Implement** the handlers with swag blocks, `RegisterAPI`, and the wiring in `Build` (construct after the classification repos and the convertor; add to `router.Compose`). Run `make swagger`.
- [ ] **Step 4: Generate the goldens.** Run `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect the new goldens, then run `make go-test`. Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt API`).

### Task 5: Classification merge and delete respect debts

**Files:**
- Modify: `internal/{category,payee,tag}/{ports.go,usecase.go,merge.go,delete.go}`, `internal/server/glue_debt.go`, `internal/server/server.go`, `internal/shared/errs/codes.go`, `locales/*.json`
- Test: `internal/{category,payee,tag}/{merge,delete}_test.go`, a new apiparity scenario `debt_classification_guard`

**Interfaces:**
- In each of category, payee and tag, `ports.go` gains:
  ```go
  type DebtUsage interface {
      InUse(ctx context.Context, id vo.Id) (bool, error)
      Reassign(ctx context.Context, oldID, newID vo.Id) error // returns debt.key_conflict ValidationError when the result collides
  }
  ```
  plus `Service.SetDebtUsage(DebtUsage)`, nil-safe.
- New codes:
  - `category.used_by_debt`, `payee.used_by_debt`, `tag.used_by_debt` (message: "A debt uses this … Delete or change the debt first.")
- The debt service gains `ReassignClassification(ctx, kind string, oldID, newID vo.Id) error`. It reassigns, re-runs the key checks for the affected owner, and returns `debt.key_conflict` on a collision. The glue adapts it to the three ports.

- [ ] **Step 1: Write the failing tests:**
  - `TestDeleteCategory_RefusedWhileDebtUsesIt`, and the same for payee and tag
  - `TestMergeCategory_DebtFollowsTarget`
  - `TestMergeTag_RefusedOnDebtKeyConflict`
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/category/ ./internal/payee/ ./internal/tag/`. Expected: FAIL.
- [ ] **Step 3: Implement** the port calls inside the existing `WithTx` blocks: delete checks `InUse` before `repo.Delete`, and merge calls `Reassign` before the source is deleted.
- [ ] **Step 4: Run the tests, regenerate and inspect the goldens, then run `make go-test`.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Keep debts consistent through category, payee and tag merges`).

### Task 6: Budget element type, rows and planning bounds

**Files:**
- Modify: `internal/model/budget_valueobject.go` (`ElementDebt ElementType = 6`, alias `"debt"`), `internal/budget/{ports.go,usecase.go,move.go,accounts.go,comments.go}`, `internal/server/glue_budget_debt.go` (new), `internal/server/server.go`, `internal/shared/errs/codes.go`, `locales/*.json`
- Test: `internal/budget/debt_elements_test.go`, `internal/model/budget_side_test.go`

**Interfaces:**
- Budget port:
  ```go
  type DebtSource interface {
      Views(ctx context.Context, ownerIDs []vo.Id) ([]model.DebtView, error)
      Sync(ctx context.Context, ownerIDs []vo.Id) error
      Timelines(ctx context.Context, req model.DebtTimelineRequest) (map[string]model.DebtTimeline, error)
  }
  func (s *Service) SetDebts(d DebtSource)
  ```
- `syncElements`: for every debt of the budget's participants, `ensure(debtID, ElementDebt)`, with the currency set to the debt's on new rows and no folder. Debt rows get their own sort group in `assignMissingKeys`, are refused by `groupElements`/`MoveElement` like savings (new code `budget.debt_folder_not_allowed`), and are deleted by the existing unseen-rows pass when the debt is gone.
- `SetLimit` and `CreateComment`: when the element type is `ElementDebt`, a period outside `[StartMonth, EndMonth]` → new code `budget.debt_period_out_of_range` (field `period`).
- `TransferData`/`HasData` (the targets of the debt `BudgetElements` port) are implemented as `Service.TransferElementData(ctx, fromID vo.Id, fromType model.ElementType, toID vo.Id, toType model.ElementType, currencyID vo.Id) error` and `Service.ElementHasData(ctx, externalID vo.Id, typ model.ElementType) (bool, error)`. Transfer runs over every budget holding a `(fromID, fromType)` element whose currency equals `currencyID`:
  1. `syncElements` that budget;
  2. move limit and comment rows to the target element, dropping limits whose period already exists on the target.

  `glue_budget_debt.go` adapts both directions.

- [ ] **Step 1: Write the failing tests:**
  - `TestElementTypeDebt`: frozen value 6, alias `"debt"`
  - `TestSyncElements_CreatesDebtRows`
  - `TestSyncElements_DropsDebtRowWhenDebtDeleted`
  - `TestMoveElement_DebtRowRefusesFolder`
  - `TestSetLimit_DebtOutsideBoundsRefused`
  - `TestSetLimit_DebtInsideBounds`
  - `TestTransferElementData_MovesLimitsAndComments`
  - `TestTransferElementData_SkipsOtherCurrency`
  - `TestTransferElementData_KeepsTargetOnPeriodClash`
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/budget/ ./internal/model/`. Expected: FAIL.
- [ ] **Step 3: Implement** it, wire `budgetSvc.SetDebts(...)` and `debtSvc.SetBudgetElements(...)` in `server.go`, and add the codes and their text in the locales.
- [ ] **Step 4: Run the tests, then `make go-test`.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt rows in budgets`).

### Task 7: Budget attribution — payments leave category and tag rows

**Files:**
- Modify: `internal/model/budget_view.go` (`DebtID *string` on `SpendingRow` and `MonthlySpendingRow`), `internal/budget/repo/read.go` (`CountSpending`, `SpendingByMonth`: `LEFT JOIN debts_payments dp ON dp.transaction_id = t.id`, select and group by `dp.debt_id`), `internal/budget/{dated_read.go,projected_read.go,builder_structure.go,builder_structure_build.go,builder_plan.go,read.go,plan.go}`
- Test: `internal/budget/debt_attribution_test.go`

**Interfaces:**
- Consumes: Task 6 `DebtSource`.
- At the top of `GetBudget` and `GetBudgetPlan`, call `debts.Sync(participants)`, then load `debts.Views(participants)` into the builder's `filters` as `debtViews []model.DebtView`.
- **Attribution switches** (`buildElementsSpending` and `buildPlanStructure`): a row with `DebtID != nil` whose debt is in `debtViews` goes to `elementKey(*DebtID, ElementDebt)` before the tag, category and uncategorized branches. Envelopes are covered automatically, because they sum child-category keys.
- **`projectedRead.spending`:** drop occurrences where `model.DebtMatches(view, categoryID, payeeID, tagID, dueAt)` is true for any view (the schedule replaces templates).
- `windowRead`/`unionRead` pass `DebtID` through unchanged.

- [ ] **Step 1: Write the failing tests:**
  - `TestGetBudget_DebtPaymentLeavesCategoryRow`
  - `TestGetBudget_DebtPaymentLeavesTagRow`
  - `TestGetBudget_DebtPaymentLeavesEnvelopeSpent`
  - `TestGetBudget_DebtPaidConverted`: a EUR account, a USD debt → Paid is in USD
  - `TestGetBudgetPlan_DebtActualPerMonth`
  - `TestUpcoming_MatchingTemplateDropped`
  - `TestGetBudget_ExpenseTotalsUnchanged`: the sum of expense spent plus debt Paid equals the pre-change category total for the same data
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/budget/ -run Debt`. Expected: FAIL.
- [ ] **Step 3: Implement** it on both engines (the hand-built SQL branches per engine).
- [ ] **Step 4: Run the tests, then `make go-test` and `make test`** (enginecompare + pgsql repo). Expected: PASS, with goldens unchanged except where a scenario holds a debt.
- [ ] **Step 5: Commit** (`feat: Attribute debt payments to debt rows`).

### Task 8: Debt rows on the wire — get-budget and get-budget-plan

**Files:**
- Modify: `internal/model/budget_dto.go`, `internal/budget/{builder_structure_build.go,builder_plan.go,upcoming_merge.go}`, Create: `internal/budget/builder_debts.go`
- Test: `internal/budget/builder_debts_test.go`, apiparity `catalogue_budget_debts.go` + goldens, swagger

**Interfaces:**
- **Additive wire types:**
  - `StructureResult.Debts []DebtElementResult` (json `debts`).
  - `DebtElementResult{Id, Type(6), Name, Icon, CurrencyId, OwnerUserId, IsArchived(0), Position, Budgeted, IsBudgeted, Spent /*Paid*/, Available, ClosingBalance, PaymentsLeft int, ScheduledTotal, Upcoming /*StillDue*/, PayoffDate, BasePayoffDate, OverpaymentLeft, BaseOverpaymentLeft, IsPaidOff int, IsPlannable int}`.
  - `PlanStructureResult.Debts []PlanDebtElementResult{Id, Type, Name, Icon, CurrencyId, OwnerUserId, Position, PayoffDate, BasePayoffDate, OverpaymentLeft, BaseOverpaymentLeft, Cells []PlanDebtCellResult{Actual, Planned, ClosingBalance, PaymentsLeft int, ScheduledTotal, Upcoming, IsPlannable int}}`.
  - Debt rows stay out of `structure.elements`.
- **Builder:** `builder_debts.go` builds the rows from the `ElementDebt` elements, `limits`, attributed Paid, and one `debts.Timelines` call per request. The months requested are the period's month (get-budget) or the plan window (get-budget-plan). Plans are passed only for in-bounds months.
- **Visibility:** a row is emitted in a month when it is within `[StartMonth, EndMonth]` or has Paid in it. Out-of-bounds cells carry `IsPlannable 0`, and their stored plans are emitted as `""` and ignored.
- **Upcoming:** `mergeBudgetUpcoming`/`mergePlanUpcoming` leave debt rows untouched (their `Upcoming` comes from the timeline).

- [ ] **Step 1: Write the failing tests:**
  - `TestGetBudget_DebtRowFigures`: Planned, Paid, ClosingBalance, PaymentsLeft and Upcoming for the reference loan
  - `TestGetBudget_DebtRowHiddenAfterPayoff`
  - `TestGetBudget_PartnerDebtRowVisible`
  - `TestGetBudgetPlan_PlannedExtraMovesPayoff`
  - `TestGetBudgetPlan_OutOfBoundsCellsNotPlannable`

  Also the apiparity scenario `budget_debts` (get-budget + get-budget-plan with one debt and a partner).
- [ ] **Step 2: Run them and confirm they fail.** Run `go test ./internal/budget/ -run Debt && go test ./internal/test/apiparity/`. Expected: FAIL.
- [ ] **Step 3: Implement** it, then run `make swagger`.
- [ ] **Step 4: Run `UPDATE_GOLDEN=1` for apiparity, inspect the goldens (only additive `debts` keys may change), then run `make test`.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt rows in get-budget and get-budget-plan`).

### Task 9: SPA data layer and debt math

**Files:**
- Create: `web/src/api/debt.ts`, `web/src/api/dto/debt.ts`, `web/src/features/debts/{queries.ts,debtMath.ts}`
- Modify: `web/src/api/dto/budget.ts` (`BudgetElementType.DEBT: 6`, `BudgetDebtElementDto`, `PlanDebtElementDto`, `structure.debts?`), `web/src/api/budget.ts` (map `structure.debts` through `withEmptyPlan`), `web/src/app/queryKeys.ts` (`debts`), `web/src/lib/metrics.ts`
- Test: `web/src/features/debts/{debtMath,queries}.test.ts`

**Interfaces:**
- API functions: `getDebtList()`, `getDebt(id)`, `createDebt(form)`, `updateDebt(form)`, `deleteDebt({id, confirmDataRemoval})`, `markDebtPaidOff(id)`.
- Hooks: `useDebts()`, `useDebt(id)`, `useCreateDebt({fromBudget?: boolean})`, `useUpdateDebt()`, `useDeleteDebt()`, `useMarkDebtPaidOff()`. Every mutation invalidates `debts`, `budget` and `budgetPlan`. Create fires `DEBT_CONVERT` when `fromBudget` and `DEBT_CREATE` otherwise; the other hooks fire `DEBT_UPDATE`, `DEBT_DELETE` and `DEBT_MARK_PAID_OFF`.
- `debtMath.ts`: `projectLastPaymentDate(balance: number, payment: number, periodRate: number, schedule: RecurringSchedule, nextDue: string): string | null` returns `null` when `payment ≤ balance·periodRate` (it never pays off). The step is `interest = B·r; B -= payment − interest` until `B ≤ 0.005`, stepping dates like the server (month-end clamp).

- [ ] **Step 1: Write the failing tests:**
  - `projectLastPaymentDate` on the reference loan (r = 0.0042676 → 240 months after nextDue − 1 month)
  - interest-free
  - never-pays-off → null
  - the hooks fire the right METRICS and invalidate the three keys (MSW)
- [ ] **Step 2: Run them and confirm they fail.** Run `cd web && pnpm test src/features/debts`. Expected: FAIL.
- [ ] **Step 3: Implement** it.
- [ ] **Step 4: Run `pnpm test` (including `metrics-coverage.test.ts`) and `pnpm lint`.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt API client and form math`).

### Task 10: Debt form, details sheet and Settings list

**Files:**
- Create: `web/src/features/debts/{DebtDialog.tsx,useDebtForm.ts,DebtDetailsSheet.tsx,DebtsSettingsPage.tsx}`
- Modify: `web/src/app/router-pages.ts` (`SETTINGS_DEBTS: '/settings/debts'`), `web/src/app/routes.tsx`, `web/src/features/settings/SettingsPage.tsx` (row after Categories in the Classification group), `web/src/app/uiStore.ts` (`openDebtModal({debt?, categoryId?, currencyId?, fromBudget?})`), `web/src/app/layouts/ApplicationLayout.tsx` (mount `DebtDialog`), `locales/*.json` (`debts.*`)
- Test: `web/src/features/debts/{DebtDialog,DebtsSettingsPage,DebtDetailsSheet}.test.tsx`

**Behaviour:**
- **Form:** modelled on `RecurringDialog`.
  - Fields: name; currency (`CurrencyPickerDialog`); category (`EntitySelect`, expense categories only); payee and tag (optional); start date (optional); balance, payment and schedule (`OptionSelect` over the recurring schedules); last payment date.
  - **Editing:** changing balance or payment re-fills the last payment date with `projectLastPaymentDate` using the debt's `periodRate`. Editing the date changes nothing else. A `null` projection shows `debts.form.never_pays_off` under the payment.
  - Server field errors are shown inline through `useFormErrors`.
  - **Delete:** a `debt.removal_unconfirmed` answer opens a confirm naming what is dropped, then resends with `confirmDataRemoval`.
- **Details sheet** (`ResponsiveDialog`):
  - figures: current balance, payments left, money left, overpayment left, overpaid so far, payoff date;
  - snapshot history;
  - payments list with Interest / Principal columns;
  - actions: Edit, Mark as paid off, Delete.

  It is opened from Settings and from the budget row (owner only).
- **Settings page:** a `SettingsShell` list of debts (name, current balance, "N payments left", paid-off badge) with a create button.

- [ ] **Step 1: Write the failing tests:**
  - "editing the balance recomputes the last payment date"
  - "editing the last payment date leaves the other fields"
  - "a removal_unconfirmed answer asks and resends with confirmDataRemoval"
  - "Settings lists debts with payments left"
  - "the details sheet shows interest and principal per payment"
- [ ] **Step 2: Run them and confirm they fail.** Expected: FAIL.
- [ ] **Step 3: Implement** it, with the copy in all 11 locales.
- [ ] **Step 4: Run `pnpm test`, `pnpm lint` and `go test ./internal/test/i18ntest/`.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debt form, details and Settings list`).

### Task 11: Debts block in the month view, and "Track as debt"

**Files:**
- Modify: `web/src/features/budgets/{MonthFlows.tsx,PhoneMonthView.tsx,BudgetPage.tsx,useBudgetLineMenus.tsx,phoneMonth.ts,budgetMath.ts,planHint.ts,BudgetAccountsField.tsx}`, `locales/*.json` (`budgets.page.debts.*`)
- Test: `web/src/features/budgets/{MonthFlows.debts,PhoneMonthView.debts,useBudgetLineMenus.debts}.test.tsx`, `planHint.test.ts`

**Behaviour:**
- **Desktop section:** `<section data-testid="month-debts">` placed after Savings, using `MonthSectionHeader foldKey="debts"` with headings Planned / Paid / Left.
  - The third cell is `closingBalance` plus `pluralPick(t('budgets.page.debts.payments_left'), paymentsLeft)`.
  - `SheetTarget` gains `{kind:'debt'; row}`.
- **Phone:** a card with Planned · Paid; Left shows from `sm` up, like savings.
- **Red rule:** `planHint` gains kind `'debt'`, which flags `isBudgeted && planned < scheduledTotal` only, from the current month onward.
- **Extra note:** when `basePayoffDate` differs from `payoffDate`, the row's editor shows `budgets.page.debts.extra_effect` ("Paid off {months} months earlier, {amount} less overpayment").
- **Totals:** `budgetMath` adds debt `spent`/`budgeted` into the expense sums and Available, so totals match pre-feature numbers.
- **Menus:**
  - `expenseRowMenu` and `envelopeChildMenu` gain "Track as debt" for an owned, non-archived expense category. It calls `openDebtModal({categoryId, currencyId: element.currencyId, fromBudget: true})`.
  - The new `debtRowMenu` offers Details (owner: the details sheet), Edit (owner) and nothing for others.
  - A non-plannable row's planned cell is read-only.
- **Visibility note:** the budget settings dialogs gain a note next to the savings one (`budgets.modal.budget_form.debts_note`): "Debts are shown by name, with their payments and balances, to everyone with access to this budget."

- [ ] **Step 1: Write the failing tests:**
  - "renders the Debts section after Savings with Planned/Paid/Left"
  - "shows payments left"
  - "a plan below the scheduled amount is red"
  - "Track as debt opens the form prefilled"
  - "a guest sees no Edit in the debt row menu"
  - "expense totals include debt rows"
- [ ] **Step 2: Run them and confirm they fail.** Expected: FAIL.
- [ ] **Step 3: Implement** it, with the copy in 11 locales.
- [ ] **Step 4: Run `pnpm test` and `pnpm lint`.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debts block in the budget month view`).

### Task 12: Debts section in the Plan grid

**Files:**
- Modify: `web/src/features/budgets/{PlanSheet.tsx,PlanRows.tsx,planCell.ts,planMath.ts,queries.ts}` (`patchPlanCells` patches `structure.debts`), `locales/*.json`
- Test: `web/src/features/budgets/PlanSheet.debts.test.tsx`, `planCell.test.ts`

**Behaviour:**
- **Section:** `<section data-testid="plan-section-debts" className="plan-band">` after Savings.
- **Cells:** each cell shows planned, with `closingBalance` + payments left as its balance line (the `planCellView` balance branch for `DEBT`).
- **Bounds:** `isEditableCell` is false when `isPlannable === 0`.
- **Totals:** group sums are included in the expense totals.
- **Payoff summary:** the row header shows "Paid off {date}", plus `extra_effect` when it differs from the base.
- **Panel:** `wantsUpcomingPanel` is not used for debt rows.

- [ ] **Step 1: Write the failing tests:**
  - "renders the Debts section after Savings"
  - "editing a debt cell sends set-limit with the debt id"
  - "cells outside the debt's months are read-only"
  - "a planned extra shows the earlier payoff"
- [ ] **Step 2: Run them and confirm they fail.** Expected: FAIL.
- [ ] **Step 3: Implement** it.
- [ ] **Step 4: Run `pnpm test` and `pnpm lint`.** Expected: PASS.
- [ ] **Step 5: Commit** (`feat: Debts section in the Plan grid`).

### Task 13: Docs and full verification

**Files:**
- Modify: `docs/regression-test-plan.md` (section 9: new **Debts** bullets with 📱 markers for the phone card; section on Settings: the Debts list), `CLAUDE.md` (a "Notable behaviours → Budget debts" paragraph summarising matching, snapshots, splits, bounds and attribution; the transaction-import and budget sections only if touched)

- [ ] **Step 1: Write the regression items** as verifiable outcomes:
  - convert from the budget (plans move);
  - partial debt (payee) leaves the category row;
  - extra plan shortens payoff;
  - out-of-range months are read-only;
  - category delete refused;
  - guest sees the row only;
  - foreign-currency payment;
  - mark paid off;
  - the phone card.
- [ ] **Step 2: Run the full verification:** `make test` and `cd web && pnpm test && pnpm lint`. Expected: all PASS.
- [ ] **Step 3: Commit** (`docs: Debts in the regression plan and CLAUDE.md`).
