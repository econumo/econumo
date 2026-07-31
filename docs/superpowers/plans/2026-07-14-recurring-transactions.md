# Recurring Transactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Manually-posted recurring transaction templates (preset schedules, one next-payment date each) with a settings management page and virtual rows merged into the account transaction list.

**Architecture:** New vertical feature package `internal/recurring` (entity/DTOs in `internal/model`, engine-adapter repo, 6 API routes under `/api/v1/recurring/`), wired in `internal/server` with consumer-side ports. The React SPA merges one virtual row per template into the account transaction list client-side; no existing wire contract changes.

**Tech Stack:** Go (stdlib + sqlc, sqlite/pgsql), React 18 + TypeScript, TanStack Query v5, zustand, vitest + MSW.

**Spec:** `docs/superpowers/specs/2026-07-14-recurring-transactions-design.md` (read it first).

## Global Constraints

- Work in the worktree `/home/dmitry/dev/econumo/econumo/.claude/worktrees/melodic-baking-mango`, branch `feat/recurring-transactions`. First command of every session: `cd` there and verify `git branch --show-current` prints `feat/recurring-transactions`.
- Features never import features: `internal/recurring` may import `internal/model`, `internal/shared/*`, `internal/web/*`, `internal/infra/storage/backend` — never `internal/transaction`, `internal/account`, etc. Enforced by `go test ./internal/test/archtest/`.
- Frozen wire formats: datetimes `"2006-01-02 15:04:05"` (`datetime.Layout`); success envelope `{"success":true,"message":"","data":...}`; type alias strings `"expense"/"income"/"transfer"`. Do NOT touch any existing endpoint's response.
- Schedule alias strings (new, then frozen): `"weekly"`, `"biweekly"`, `"monthly"`, `"quarterly"`, `"yearly"`.
- sqlc `.sql` files: ASCII-only comments (an em dash mangles sqlc v1.30 sqlite codegen).
- Comments sparingly: only non-obvious business logic / frozen-contract rationale. No godoc restating names.
- Go tests: `go test ./internal/...` must stay green; frontend: `cd web && pnpm test` (note: `ImportCsvDialog.test.tsx` already fails on main — ignore that one failure only).
- Commit after every task with the trailer:
  ```
  Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_011R1M8dVdxozEBQ3MV7UaWH
  ```

---

### Task 1: Model — entity, schedule, NextOccurrence

**Files:**
- Create: `internal/model/recurring.go`
- Test: `internal/model/recurring_test.go`

**Interfaces:**
- Consumes: `vo.Id` (`internal/shared/vo`), `model.TransactionType` (existing, `internal/model/transaction.go`).
- Produces (later tasks rely on these exact names):
  - `type RecurringSchedule string` + consts `RecurringScheduleWeekly/Biweekly/Monthly/Quarterly/Yearly` + `ParseRecurringSchedule(s string) (RecurringSchedule, bool)`
  - `NextOccurrence(current time.Time, schedule RecurringSchedule, scheduledDay int16) time.Time`
  - `type RecurringTransaction struct` / `type RecurringNewState struct`
  - `NewRecurringTransaction(s RecurringNewState) *RecurringTransaction`, `RecurringFromState(s RecurringNewState) *RecurringTransaction`
  - `(*RecurringTransaction).Update(s RecurringNewState, now time.Time)`, `(*RecurringTransaction).Advance(now time.Time)`

- [ ] **Step 1: Write the failing tests**

```go
package model_test

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestNextOccurrence(t *testing.T) {
	cases := []struct {
		name         string
		current      string
		schedule     model.RecurringSchedule
		scheduledDay int16
		want         string
	}{
		{"weekly", "2026-07-14 00:00:00", model.RecurringScheduleWeekly, 14, "2026-07-21 00:00:00"},
		{"biweekly", "2026-07-14 00:00:00", model.RecurringScheduleBiweekly, 14, "2026-07-28 00:00:00"},
		{"monthly plain", "2026-07-14 00:00:00", model.RecurringScheduleMonthly, 14, "2026-08-14 00:00:00"},
		{"monthly clamp to feb", "2027-01-31 00:00:00", model.RecurringScheduleMonthly, 31, "2027-02-28 00:00:00"},
		{"monthly clamp leap feb", "2028-01-31 00:00:00", model.RecurringScheduleMonthly, 31, "2028-02-29 00:00:00"},
		{"monthly recovers after clamp", "2027-02-28 00:00:00", model.RecurringScheduleMonthly, 31, "2027-03-31 00:00:00"},
		{"monthly 30 skips feb", "2027-01-30 00:00:00", model.RecurringScheduleMonthly, 30, "2027-02-28 00:00:00"},
		{"monthly year rollover", "2026-12-15 00:00:00", model.RecurringScheduleMonthly, 15, "2027-01-15 00:00:00"},
		{"quarterly", "2026-11-30 00:00:00", model.RecurringScheduleQuarterly, 30, "2027-02-28 00:00:00"},
		{"yearly", "2028-02-29 00:00:00", model.RecurringScheduleYearly, 29, "2029-02-28 00:00:00"},
		{"keeps time of day", "2026-07-14 09:30:00", model.RecurringScheduleWeekly, 14, "2026-07-21 09:30:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := model.NextOccurrence(d(tc.current), tc.schedule, tc.scheduledDay)
			if want := d(tc.want); !got.Equal(want) {
				t.Fatalf("NextOccurrence(%s, %s, %d) = %s, want %s", tc.current, tc.schedule, tc.scheduledDay, got, want)
			}
		})
	}
}

func TestParseRecurringSchedule(t *testing.T) {
	for _, alias := range []string{"weekly", "biweekly", "monthly", "quarterly", "yearly"} {
		s, ok := model.ParseRecurringSchedule(alias)
		if !ok || string(s) != alias {
			t.Fatalf("ParseRecurringSchedule(%q) = %q, %v", alias, s, ok)
		}
	}
	if _, ok := model.ParseRecurringSchedule("daily"); ok {
		t.Fatal("ParseRecurringSchedule accepted an unknown alias")
	}
}

func TestNewRecurringTransaction_DerivesScheduledDay(t *testing.T) {
	now := d("2026-07-14 10:00:00")
	rt := model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "50", Schedule: model.RecurringScheduleMonthly,
		NextPaymentAt: d("2026-07-31 00:00:00"), CreatedAt: now,
	})
	if rt.ScheduledDay != 31 {
		t.Fatalf("ScheduledDay = %d, want 31", rt.ScheduledDay)
	}
	if !rt.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want %s", rt.UpdatedAt, now)
	}
}

func TestRecurringAdvance_UsesScheduledDay(t *testing.T) {
	now := d("2027-03-05 10:00:00")
	rt := model.RecurringFromState(model.RecurringNewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "50", Schedule: model.RecurringScheduleMonthly,
		NextPaymentAt: d("2027-02-28 00:00:00"), ScheduledDay: 31,
		CreatedAt: d("2027-01-01 00:00:00"), UpdatedAt: d("2027-01-01 00:00:00"),
	})
	rt.Advance(now)
	if want := d("2027-03-31 00:00:00"); !rt.NextPaymentAt.Equal(want) {
		t.Fatalf("NextPaymentAt = %s, want %s", rt.NextPaymentAt, want)
	}
	if !rt.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt not stamped")
	}
}

func TestRecurringUpdate_TransferClearsClassifiers_AndRederivesDay(t *testing.T) {
	now := d("2026-07-14 10:00:00")
	cat := vo.NewId()
	recip := vo.NewId()
	rt := model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.NewId(), UserID: vo.NewId(), Type: model.TransactionTypeExpense,
		AccountID: vo.NewId(), Amount: "50", CategoryID: &cat,
		Schedule: model.RecurringScheduleMonthly, NextPaymentAt: d("2026-07-31 00:00:00"), CreatedAt: now,
	})
	later := d("2026-07-15 10:00:00")
	rt.Update(model.RecurringNewState{
		ID: rt.ID, UserID: rt.UserID, Type: model.TransactionTypeTransfer,
		AccountID: rt.AccountID, AccountRecipID: &recip, Amount: "60",
		CategoryID: &cat, Schedule: model.RecurringScheduleWeekly,
		NextPaymentAt: d("2026-08-05 00:00:00"),
	}, later)
	if rt.CategoryID != nil {
		t.Fatal("transfer must clear CategoryID")
	}
	if rt.AccountRecipID == nil || !rt.AccountRecipID.Equal(recip) {
		t.Fatal("transfer must keep AccountRecipID")
	}
	if rt.ScheduledDay != 5 {
		t.Fatalf("ScheduledDay = %d, want 5 (re-derived)", rt.ScheduledDay)
	}
	if !rt.UpdatedAt.Equal(later) {
		t.Fatal("UpdatedAt not stamped")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/model/ -run 'TestNextOccurrence|TestParseRecurringSchedule|TestRecurring' -v`
Expected: FAIL to compile — `undefined: model.RecurringSchedule` etc.

- [ ] **Step 3: Write the implementation**

`internal/model/recurring.go`:

```go
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

type RecurringSchedule string

const (
	RecurringScheduleWeekly    RecurringSchedule = "weekly"
	RecurringScheduleBiweekly  RecurringSchedule = "biweekly"
	RecurringScheduleMonthly   RecurringSchedule = "monthly"
	RecurringScheduleQuarterly RecurringSchedule = "quarterly"
	RecurringScheduleYearly    RecurringSchedule = "yearly"
)

func ParseRecurringSchedule(s string) (RecurringSchedule, bool) {
	switch RecurringSchedule(s) {
	case RecurringScheduleWeekly, RecurringScheduleBiweekly, RecurringScheduleMonthly,
		RecurringScheduleQuarterly, RecurringScheduleYearly:
		return RecurringSchedule(s), true
	}
	return "", false
}

// NextOccurrence advances from the SCHEDULED date (posting late must not drift
// the schedule). Month-based schedules clamp to the shortest month but return
// to scheduledDay afterwards (31st -> Feb 28 -> Mar 31), which is why the day
// is carried separately instead of being re-read from the current date.
func NextOccurrence(current time.Time, schedule RecurringSchedule, scheduledDay int16) time.Time {
	switch schedule {
	case RecurringScheduleWeekly:
		return current.AddDate(0, 0, 7)
	case RecurringScheduleBiweekly:
		return current.AddDate(0, 0, 14)
	}
	months := 1
	switch schedule {
	case RecurringScheduleQuarterly:
		months = 3
	case RecurringScheduleYearly:
		months = 12
	}
	y, m, _ := current.Date()
	hh, mi, ss := current.Clock()
	first := time.Date(y, m+time.Month(months), 1, hh, mi, ss, 0, current.Location())
	day := int(scheduledDay)
	if last := daysInMonth(first.Year(), first.Month()); day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, hh, mi, ss, 0, current.Location())
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

type RecurringTransaction struct {
	ID             vo.Id
	UserID         vo.Id
	Type           TransactionType
	AccountID      vo.Id
	AccountRecipID *vo.Id
	Amount         string
	CategoryID     *vo.Id
	PayeeID        *vo.Id
	TagID          *vo.Id
	Description    string
	Schedule       RecurringSchedule
	NextPaymentAt  time.Time
	ScheduledDay   int16
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RecurringNewState struct {
	ID             vo.Id
	UserID         vo.Id
	Type           TransactionType
	AccountID      vo.Id
	AccountRecipID *vo.Id
	Amount         string
	CategoryID     *vo.Id
	PayeeID        *vo.Id
	TagID          *vo.Id
	Description    string
	Schedule       RecurringSchedule
	NextPaymentAt  time.Time
	ScheduledDay   int16
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewRecurringTransaction(s RecurringNewState) *RecurringTransaction {
	rt := recurringFrom(s)
	rt.ScheduledDay = int16(s.NextPaymentAt.Day())
	rt.UpdatedAt = s.CreatedAt
	return rt
}

func RecurringFromState(s RecurringNewState) *RecurringTransaction {
	return recurringFrom(s)
}

func recurringFrom(s RecurringNewState) *RecurringTransaction {
	rt := &RecurringTransaction{
		ID: s.ID, UserID: s.UserID, Type: s.Type, AccountID: s.AccountID,
		AccountRecipID: s.AccountRecipID, Amount: s.Amount,
		CategoryID: s.CategoryID, PayeeID: s.PayeeID, TagID: s.TagID,
		Description: s.Description, Schedule: s.Schedule,
		NextPaymentAt: s.NextPaymentAt, ScheduledDay: s.ScheduledDay,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
	rt.normalize()
	return rt
}

func (rt *RecurringTransaction) Update(s RecurringNewState, now time.Time) {
	rt.Type = s.Type
	rt.AccountID = s.AccountID
	rt.AccountRecipID = s.AccountRecipID
	rt.Amount = s.Amount
	rt.CategoryID = s.CategoryID
	rt.PayeeID = s.PayeeID
	rt.TagID = s.TagID
	rt.Description = s.Description
	rt.Schedule = s.Schedule
	rt.NextPaymentAt = s.NextPaymentAt
	rt.ScheduledDay = int16(s.NextPaymentAt.Day())
	rt.normalize()
	rt.UpdatedAt = now
}

func (rt *RecurringTransaction) Advance(now time.Time) {
	rt.NextPaymentAt = NextOccurrence(rt.NextPaymentAt, rt.Schedule, rt.ScheduledDay)
	rt.UpdatedAt = now
}

// Same invariant as Transaction.Update: transfers carry no classifiers,
// non-transfers carry no recipient.
func (rt *RecurringTransaction) normalize() {
	if rt.Type.IsTransfer() {
		rt.CategoryID, rt.PayeeID, rt.TagID = nil, nil, nil
	} else {
		rt.AccountRecipID = nil
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/ -run 'TestNextOccurrence|TestParseRecurringSchedule|TestRecurring' -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Run the whole model package + gofmt**

Run: `gofmt -l internal/model/ && go test ./internal/model/`
Expected: no gofmt output, PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/model/recurring.go internal/model/recurring_test.go
git commit -m "feat(recurring): model entity, schedule, NextOccurrence advance logic"
```

---

### Task 2: Model — DTOs and validation

**Files:**
- Create: `internal/model/recurring_dto.go`
- Test: `internal/model/recurring_dto_test.go`

**Interfaces:**
- Consumes: `errs.FieldError`/`errs.NewValidation` (`internal/shared/errs`), `vo.FlexString`, `datetime.Layout` (`internal/shared/datetime`), existing `TransactionResult`, `AccountResult`, `UserResult`.
- Produces: request/result DTOs named below; every request has `Validate() error`. Later tasks marshal these exact JSON field names.

- [ ] **Step 1: Write the failing test**

```go
package model_test

import (
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

func fieldKeys(err error) []string {
	v, ok := errs.AsValidation(err)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(v.Fields))
	for _, f := range v.Fields {
		keys = append(keys, f.Key)
	}
	return keys
}

func TestCreateRecurringTransactionRequest_Validate(t *testing.T) {
	err := model.CreateRecurringTransactionRequest{}.Validate()
	keys := strings.Join(fieldKeys(err), ",")
	for _, want := range []string{"id", "type", "amount", "accountId", "schedule", "nextPaymentAt"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("missing field error %q in %q", want, keys)
		}
	}

	ok := model.CreateRecurringTransactionRequest{
		Id: "0197b7e0-0000-7000-8000-000000000001", Type: "expense", Amount: "50",
		AccountId: "0197b7e0-0000-7000-8000-000000000002",
		Schedule: "monthly", NextPaymentAt: "2026-08-01 00:00:00",
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	bad := ok
	bad.NextPaymentAt = "2026-08-01"
	if err := bad.Validate(); err == nil {
		t.Fatal("date without time must be rejected")
	}
}

func TestPostRecurringTransactionRequest_Validate(t *testing.T) {
	err := model.PostRecurringTransactionRequest{}.Validate()
	keys := strings.Join(fieldKeys(err), ",")
	for _, want := range []string{"recurringId", "id", "type", "amount", "accountId", "date"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("missing field error %q in %q", want, keys)
		}
	}
}
```

Note: if `errs.AsValidation` has a different signature in `internal/shared/errs` (check the file), adapt the helper — the assertion target is the set of field keys.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestCreateRecurringTransactionRequest -v`
Expected: FAIL to compile — `undefined: model.CreateRecurringTransactionRequest`.

- [ ] **Step 3: Write the DTOs**

`internal/model/recurring_dto.go`. Amount uses `vo.FlexString` (accepts JSON number or string) exactly like `CreateTransactionRequest`:

```go
package model

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type RecurringTransactionResult struct {
	Id                 string  `json:"id"`
	OwnerUserId        string  `json:"ownerUserId"`
	Type               string  `json:"type"`
	AccountId          string  `json:"accountId"`
	AccountRecipientId *string `json:"accountRecipientId"`
	Amount             string  `json:"amount"`
	CategoryId         *string `json:"categoryId"`
	PayeeId            *string `json:"payeeId"`
	TagId              *string `json:"tagId"`
	Description        string  `json:"description"`
	Schedule           string  `json:"schedule"`
	NextPaymentAt      string  `json:"nextPaymentAt"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

type GetRecurringTransactionListResult struct {
	Items []RecurringTransactionResult `json:"items"`
}

type CreateRecurringTransactionRequest struct {
	Id                 string        `json:"id"`
	Type               string        `json:"type"`
	Amount             vo.FlexString `json:"amount"`
	AccountId          string        `json:"accountId"`
	AccountRecipientId *string       `json:"accountRecipientId"`
	CategoryId         *string       `json:"categoryId"`
	PayeeId            *string       `json:"payeeId"`
	TagId              *string       `json:"tagId"`
	Description        *string       `json:"description"`
	Schedule           string        `json:"schedule"`
	NextPaymentAt      string        `json:"nextPaymentAt"`
}

func (r CreateRecurringTransactionRequest) Validate() error {
	return validateRecurringFields(r.Id, r.Type, r.Amount.String(), r.AccountId, r.Schedule, r.NextPaymentAt)
}

type CreateRecurringTransactionResult struct {
	Item RecurringTransactionResult `json:"item"`
}

type UpdateRecurringTransactionRequest struct {
	Id                 string        `json:"id"`
	Type               string        `json:"type"`
	Amount             vo.FlexString `json:"amount"`
	AccountId          string        `json:"accountId"`
	AccountRecipientId *string       `json:"accountRecipientId"`
	CategoryId         *string       `json:"categoryId"`
	PayeeId            *string       `json:"payeeId"`
	TagId              *string       `json:"tagId"`
	Description        *string       `json:"description"`
	Schedule           string        `json:"schedule"`
	NextPaymentAt      string        `json:"nextPaymentAt"`
}

func (r UpdateRecurringTransactionRequest) Validate() error {
	return validateRecurringFields(r.Id, r.Type, r.Amount.String(), r.AccountId, r.Schedule, r.NextPaymentAt)
}

type UpdateRecurringTransactionResult struct {
	Item RecurringTransactionResult `json:"item"`
}

type DeleteRecurringTransactionRequest struct {
	Id string `json:"id"`
}

func (r DeleteRecurringTransactionRequest) Validate() error {
	return validateNotBlank([]blankCheck{{"id", r.Id}}, nil)
}

type DeleteRecurringTransactionResult struct{}

type PostRecurringTransactionRequest struct {
	RecurringId        string         `json:"recurringId"`
	Id                 string         `json:"id"`
	Type               string         `json:"type"`
	Amount             vo.FlexString  `json:"amount"`
	AmountRecipient    *vo.FlexString `json:"amountRecipient"`
	AccountId          string         `json:"accountId"`
	AccountRecipientId *string        `json:"accountRecipientId"`
	CategoryId         *string        `json:"categoryId"`
	PayeeId            *string        `json:"payeeId"`
	TagId              *string        `json:"tagId"`
	Description        *string        `json:"description"`
	Date               string         `json:"date"`
}

func (r PostRecurringTransactionRequest) Validate() error {
	return validateNotBlank([]blankCheck{
		{"recurringId", r.RecurringId}, {"id", r.Id}, {"type", r.Type},
		{"amount", r.Amount.String()}, {"accountId", r.AccountId}, {"date", r.Date},
	}, nil)
}

type PostRecurringTransactionResult struct {
	Item          TransactionResult `json:"item"`
	Accounts      []AccountResult   `json:"accounts"`
	NextPaymentAt string            `json:"nextPaymentAt"`
}

type SkipRecurringTransactionRequest struct {
	Id string `json:"id"`
}

func (r SkipRecurringTransactionRequest) Validate() error {
	return validateNotBlank([]blankCheck{{"id", r.Id}}, nil)
}

type SkipRecurringTransactionResult struct {
	Item RecurringTransactionResult `json:"item"`
}

type blankCheck struct {
	key string
	val string
}

func validateNotBlank(checks []blankCheck, extra []errs.FieldError) error {
	var fields []errs.FieldError
	for _, c := range checks {
		if strings.TrimSpace(c.val) == "" {
			fields = append(fields, errs.FieldError{Key: c.key, Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
		}
	}
	fields = append(fields, extra...)
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

func validateRecurringFields(id, typ, amount, accountID, schedule, nextPaymentAt string) error {
	var extra []errs.FieldError
	if strings.TrimSpace(schedule) != "" {
		if _, ok := ParseRecurringSchedule(schedule); !ok {
			extra = append(extra, errs.FieldError{Key: "schedule", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
		}
	}
	if strings.TrimSpace(nextPaymentAt) != "" {
		if _, err := time.Parse(datetime.Layout, nextPaymentAt); err != nil {
			extra = append(extra, errs.FieldError{Key: "nextPaymentAt", Message: "This value is not valid.", Code: "INVALID_FORMAT_ERROR"})
		}
	}
	return validateNotBlank([]blankCheck{
		{"id", id}, {"type", typ}, {"amount", amount},
		{"accountId", accountID}, {"schedule", schedule}, {"nextPaymentAt", nextPaymentAt},
	}, extra)
}
```

Before writing, open `internal/model/transaction_dto.go` and copy its exact blank-check message/code strings if they differ from the above (`"This value should not be blank."` / `IS_BLANK_ERROR` is the established pair; reuse whatever code string the transaction DTO uses for its date-format error instead of `INVALID_FORMAT_ERROR` if one exists).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/ -v -run 'RecurringTransactionRequest'`
Expected: PASS.

- [ ] **Step 5: gofmt + full package**

Run: `gofmt -l internal/model/ && go test ./internal/model/`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/model/recurring_dto.go internal/model/recurring_dto_test.go
git commit -m "feat(recurring): request/result DTOs with validation"
```

---

### Task 3: Storage — migration + sqlc queries + generated code

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260714000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260714000000.sql`
- Create: `internal/infra/storage/sqlc/query/sqlite/recurring_transactions.sql`
- Create: `internal/infra/storage/sqlc/query/pgsql/recurring_transactions.sql`
- Generated (by sqlc, commit them): new files under `internal/infra/storage/sqlc/gen/sqlite/` and `gen/pgsql/`

**Interfaces:**
- Produces: table `recurring_transactions`; generated `sqlitegen.RecurringTransaction`, `sqlitegen.UpsertRecurringTransactionParams`, querier methods `GetRecurringTransactionByID(ctx, db, id string)`, `UpsertRecurringTransaction(ctx, db, arg)`, `DeleteRecurringTransaction(ctx, db, id string)` (pgsql equivalents with per-query Row types). Task 4's repo consumes these.
- No embed/config registration needed: migrations are picked up by glob `//go:embed sqlite/*.sql`; sqlc reads the migrations dir as schema.

- [ ] **Step 1: Write the sqlite migration**

`internal/infra/storage/migrations/sqlite/20260714000000.sql`. FK semantics copy `transactions` (user/source-account CASCADE, classifiers SET NULL) with ONE deliberate deviation: `account_recipient_id` is CASCADE, not SET NULL — a transfer template without a recipient can never be posted, so deleting the recipient account deletes the template.

```sql
CREATE TABLE recurring_transactions
(
    id                   TEXT           NOT NULL
    , user_id              TEXT           NOT NULL
    , account_id           TEXT           NOT NULL
    , account_recipient_id TEXT           DEFAULT NULL
    , category_id          TEXT           DEFAULT NULL
    , payee_id             TEXT           DEFAULT NULL
    , tag_id               TEXT           DEFAULT NULL
    , type                 SMALLINT       NOT NULL
    , amount               NUMERIC(19, 8) NOT NULL
    , description          VARCHAR(255)   NOT NULL
    , schedule             VARCHAR(16)    NOT NULL
    , next_payment_at      DATETIME       NOT NULL
    , scheduled_day        SMALLINT       NOT NULL
    , created_at           DATETIME       NOT NULL
    , updated_at           DATETIME       NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (account_recipient_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE SET NULL
    , FOREIGN KEY (payee_id) REFERENCES payees (id) ON DELETE SET NULL
    , FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE SET NULL
);
CREATE INDEX account_id_idx_recurring_transactions ON recurring_transactions (account_id);
CREATE INDEX user_id_idx_recurring_transactions ON recurring_transactions (user_id);
```

- [ ] **Step 2: Write the pgsql migration**

`internal/infra/storage/migrations/pgsql/20260714000000.sql` — same statement with pgsql types (everything else identical, including index names):

```sql
CREATE TABLE recurring_transactions
(
    id                   UUID           NOT NULL
    , user_id              UUID           NOT NULL
    , account_id           UUID           NOT NULL
    , account_recipient_id UUID           DEFAULT NULL
    , category_id          UUID           DEFAULT NULL
    , payee_id             UUID           DEFAULT NULL
    , tag_id               UUID           DEFAULT NULL
    , type                 SMALLINT       NOT NULL
    , amount               NUMERIC(19, 8) NOT NULL
    , description          VARCHAR(255)   NOT NULL
    , schedule             VARCHAR(16)    NOT NULL
    , next_payment_at      TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , scheduled_day        SMALLINT       NOT NULL
    , created_at           TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at           TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (account_recipient_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE SET NULL
    , FOREIGN KEY (payee_id) REFERENCES payees (id) ON DELETE SET NULL
    , FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE SET NULL
);
CREATE INDEX account_id_idx_recurring_transactions ON recurring_transactions (account_id);
CREATE INDEX user_id_idx_recurring_transactions ON recurring_transactions (user_id);
```

- [ ] **Step 3: Write the sqlite query file**

`internal/infra/storage/sqlc/query/sqlite/recurring_transactions.sql` (ASCII-only comments; column order identical to pgsql file — this is what makes the struct-conversion shim compile):

```sql
-- name: GetRecurringTransactionByID :one
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
       type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at
FROM recurring_transactions
WHERE id = ?;

-- name: UpsertRecurringTransaction :exec
INSERT INTO recurring_transactions (id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
                                    type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    account_id = excluded.account_id
    , account_recipient_id = excluded.account_recipient_id
    , category_id = excluded.category_id
    , payee_id = excluded.payee_id
    , tag_id = excluded.tag_id
    , type = excluded.type
    , amount = excluded.amount
    , description = excluded.description
    , schedule = excluded.schedule
    , next_payment_at = excluded.next_payment_at
    , scheduled_day = excluded.scheduled_day
    , updated_at = excluded.updated_at;

-- name: DeleteRecurringTransaction :exec
DELETE FROM recurring_transactions WHERE id = ?;
```

- [ ] **Step 4: Write the pgsql query file**

`internal/infra/storage/sqlc/query/pgsql/recurring_transactions.sql` — byte-identical to Step 3 except `?` placeholders become `$1 ... $15` (in the VALUES list) and `$1` (in the two WHERE clauses).

- [ ] **Step 5: Generate and build**

Run: `sqlc version && go generate ./internal/infra/storage/sqlc/... && go build ./...`
Expected: sqlc ~v1.30; generation succeeds; build green. Inspect `internal/infra/storage/sqlc/gen/sqlite/models.go` — the new `RecurringTransaction` struct must have `Amount string`, `Type int16`, `ScheduledDay int16`, `NextPaymentAt time.Time` (the existing sqlc.yaml overrides handle this; if any field came out differently, fix the DDL type, not the config).

- [ ] **Step 6: Verify migrations apply on both engines' loaders**

Run: `go test ./internal/infra/storage/... ./internal/test/dbtest/...`
Expected: PASS (dbtest runs the sqlite migration set; a broken migration fails here).

- [ ] **Step 7: Commit**

```bash
git add internal/infra/storage/migrations internal/infra/storage/sqlc
git commit -m "feat(recurring): recurring_transactions table + sqlc queries (both engines)"
```

---

### Task 4: Repository — engine-adapter repo + integration tests

**Files:**
- Create: `internal/recurring/repository.go`
- Create: `internal/recurring/repo/repo.go`
- Create: `internal/recurring/repo/sqlite.go`
- Create: `internal/recurring/repo/pgsql.go`
- Test: `internal/recurring/repo/repo_integration_test.go`

**Interfaces:**
- Consumes: Task 3's generated code; `backend.TxManager`/`backend.DBTX` (`internal/infra/storage/backend`); `dbtest.New(t)` + `fixture.New(t, db)` for tests.
- Produces (Task 5 consumes):
  ```go
  package recurring
  type Repository interface {
  	NextIdentity() vo.Id
  	GetByID(ctx context.Context, id vo.Id) (*model.RecurringTransaction, error)
  	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id) ([]*model.RecurringTransaction, error)
  	Save(ctx context.Context, rt *model.RecurringTransaction) error
  	Delete(ctx context.Context, id vo.Id) error
  }
  ```
  and `recurringrepo.NewRepo(driver string, tx *backend.TxManager) *Repo` satisfying it.

**Reference implementations to copy structure from (read them first):** `internal/tag/repo/{repo.go,sqlite.go,pgsql.go}` (split-file engine-adapter pattern) and `internal/transaction/repo/repo.go` (the `hydrate`/`idPtr`/`parseOpt` helpers, and `ListByAccountIDs` hand-built `IN`-list dynamic SQL with a `placeholders(driver, start, n)` helper — recurring needs its own copy of that helper since features don't share internals).

- [ ] **Step 1: Write `internal/recurring/repository.go`** — exactly the `Repository` interface above (package `recurring`).

- [ ] **Step 2: Write the failing integration test**

`internal/recurring/repo/repo_integration_test.go` (package `repo_test`). Model on `internal/tag/repo/repo_integration_test.go` — same harness helpers, same `dbtest.New(t)`/`fixture.New(t, db)` construction:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	recurringrepo "github.com/econumo/econumo/internal/recurring/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	userA    = "0197c000-0000-7000-8000-00000000000a"
	accountA = "0197c000-0000-7000-8000-00000000000b"
	accountB = "0197c000-0000-7000-8000-00000000000c"
	rtA      = "0197c000-0000-7000-8000-00000000000d"
)

var fixedTime = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

func newRepo(t *testing.T) (*recurringrepo.Repo, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return recurringrepo.NewRepo(db.Engine, db.TX), fixture.New(t, db)
}

func seed(t *testing.T, f *fixture.Builder) {
	t.Helper()
	f.User(fixture.User{ID: userA})
	f.Account(fixture.Account{ID: accountA, UserID: userA})
	f.Account(fixture.Account{ID: accountB, UserID: userA})
}

func template(id string) *model.RecurringTransaction {
	return model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA),
		Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(accountA),
		Amount: "50.5", Description: "rent",
		Schedule: model.RecurringScheduleMonthly,
		NextPaymentAt: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		CreatedAt: fixedTime,
	})
}

func TestRecurringRepo_SaveGetRoundTrip(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(rtA))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Amount != "50.5" || got.Schedule != model.RecurringScheduleMonthly || got.ScheduledDay != 31 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if !got.NextPaymentAt.Equal(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextPaymentAt = %s", got.NextPaymentAt)
	}
}

func TestRecurringRepo_UpsertUpdates(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	rt := template(rtA)
	if err := repo.Save(ctx, rt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rt.Advance(fixedTime.Add(time.Hour))
	if err := repo.Save(ctx, rt); err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	got, _ := repo.GetByID(ctx, vo.MustParseId(rtA))
	if !got.NextPaymentAt.Equal(time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextPaymentAt after advance = %s, want 2026-08-31", got.NextPaymentAt)
	}
}

func TestRecurringRepo_ListByAccountIDs(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	rtB := "0197c000-0000-7000-8000-00000000000e"
	a := template(rtA)
	b := template(rtB)
	b.AccountID = vo.MustParseId(accountB)
	for _, rt := range []*model.RecurringTransaction{a, b} {
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(accountA)})
	if err != nil || len(got) != 1 || got[0].ID.String() != rtA {
		t.Fatalf("ListByAccountIDs(accountA) = %v items, err %v", len(got), err)
	}
	both, _ := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(accountA), vo.MustParseId(accountB)})
	if len(both) != 2 {
		t.Fatalf("ListByAccountIDs(both) = %d items, want 2", len(both))
	}
	none, err := repo.ListByAccountIDs(ctx, nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty id list must return empty slice, no error")
	}
}

func TestRecurringRepo_Delete_AndGetMissing(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, vo.MustParseId(rtA)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, vo.MustParseId(rtA)); err == nil {
		t.Fatal("GetByID after delete must return not-found")
	}
}
```

Check `internal/test/fixture/entities.go` for the exact `fixture.User`/`fixture.Account` field names before running; adjust the seed calls if defaults differ.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/recurring/... -v`
Expected: FAIL to compile — `recurringrepo.NewRepo` undefined.

- [ ] **Step 4: Write the repo**

`internal/recurring/repo/repo.go` — copy the tag repo skeleton, adjust to these types. Canonical types + querier + methods:

```go
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	rtRow        = sqlitegen.RecurringTransaction
	upsertParams = sqlitegen.UpsertRecurringTransactionParams
)

type querier interface {
	GetRecurringTransactionByID(ctx context.Context, db backend.DBTX, id string) (rtRow, error)
	UpsertRecurringTransaction(ctx context.Context, db backend.DBTX, arg upsertParams) error
	DeleteRecurringTransaction(ctx context.Context, db backend.DBTX, id string) error
}

type Repo struct {
	driver string
	tx     *backend.TxManager
	q      querier
}

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{driver: driver, tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{driver: driver, tx: tx, q: pgsqlQuerier{}}
	}
	panic(fmt.Sprintf("unknown database driver %q", driver))
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.RecurringTransaction, error) {
	row, err := r.q.GetRecurringTransactionByID(ctx, r.db(ctx), id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.NewNotFound("Recurring transaction not found")
	}
	if err != nil {
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) Save(ctx context.Context, rt *model.RecurringTransaction) error {
	return r.q.UpsertRecurringTransaction(ctx, r.db(ctx), upsertParams{
		ID:                 rt.ID.String(),
		UserID:             rt.UserID.String(),
		AccountID:          rt.AccountID.String(),
		AccountRecipientID: idPtr(rt.AccountRecipID),
		CategoryID:         idPtr(rt.CategoryID),
		PayeeID:            idPtr(rt.PayeeID),
		TagID:              idPtr(rt.TagID),
		Type:               rt.Type.Int16(),
		Amount:             rt.Amount,
		Description:        rt.Description,
		Schedule:           string(rt.Schedule),
		NextPaymentAt:      rt.NextPaymentAt,
		ScheduledDay:       rt.ScheduledDay,
		CreatedAt:          rt.CreatedAt,
		UpdatedAt:          rt.UpdatedAt,
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteRecurringTransaction(ctx, r.db(ctx), id.String())
}

// Variadic IN list, so this is hand-built SQL like the transaction repo's
// ListByAccountIDs; column order matches the sqlc SELECTs so hydrate is shared.
func (r *Repo) ListByAccountIDs(ctx context.Context, accountIDs []vo.Id) ([]*model.RecurringTransaction, error) {
	if len(accountIDs) == 0 {
		return []*model.RecurringTransaction{}, nil
	}
	args := make([]any, len(accountIDs))
	for i, id := range accountIDs {
		args[i] = id.String()
	}
	query := fmt.Sprintf(`SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
       type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at
FROM recurring_transactions
WHERE account_id IN (%s)
ORDER BY next_payment_at, id`, placeholders(r.driver, 1, len(accountIDs)))
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.RecurringTransaction{}
	for rows.Next() {
		var row rtRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.AccountID, &row.AccountRecipientID,
			&row.CategoryID, &row.PayeeID, &row.TagID, &row.Type, &row.Amount, &row.Description,
			&row.Schedule, &row.NextPaymentAt, &row.ScheduledDay, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		rt, err := hydrate(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func placeholders(driver string, start, n int) string {
	parts := make([]string, n)
	for i := range parts {
		if driver == "postgresql" {
			parts[i] = fmt.Sprintf("$%d", start+i)
		} else {
			parts[i] = "?"
		}
	}
	return strings.Join(parts, ", ")
}

func hydrate(row rtRow) (*model.RecurringTransaction, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	accountID, err := vo.ParseId(row.AccountID)
	if err != nil {
		return nil, err
	}
	recip, err := parseOpt(row.AccountRecipientID)
	if err != nil {
		return nil, err
	}
	cat, err := parseOpt(row.CategoryID)
	if err != nil {
		return nil, err
	}
	payee, err := parseOpt(row.PayeeID)
	if err != nil {
		return nil, err
	}
	tag, err := parseOpt(row.TagID)
	if err != nil {
		return nil, err
	}
	return model.RecurringFromState(model.RecurringNewState{
		ID: id, UserID: userID, Type: model.TransactionType(row.Type),
		AccountID: accountID, AccountRecipID: recip, Amount: row.Amount,
		CategoryID: cat, PayeeID: payee, TagID: tag, Description: row.Description,
		Schedule: model.RecurringSchedule(row.Schedule), NextPaymentAt: row.NextPaymentAt,
		ScheduledDay: row.ScheduledDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func idPtr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func parseOpt(s *string) (*vo.Id, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
```

IMPORTANT: the generated field names in `upsertParams` / `rtRow` come from sqlc — after Task 3, open `gen/sqlite/models.go` and use the exact generated names (sqlc may emit `AccountRecipientID` vs `AccountRecipID` etc. based on column names; the columns above generate the names used here). Amount timestamps map straight through (`string` / `time.Time`) — no conversion in the repo.

`internal/recurring/repo/sqlite.go` — passthrough (copy `internal/tag/repo/sqlite.go` shape):

```go
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

func (sqliteQuerier) GetRecurringTransactionByID(ctx context.Context, db backend.DBTX, id string) (rtRow, error) {
	return sqlitegen.New(db).GetRecurringTransactionByID(ctx, id)
}

func (sqliteQuerier) UpsertRecurringTransaction(ctx context.Context, db backend.DBTX, arg upsertParams) error {
	return sqlitegen.New(db).UpsertRecurringTransaction(ctx, arg)
}

func (sqliteQuerier) DeleteRecurringTransaction(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteRecurringTransaction(ctx, id)
}
```

`internal/recurring/repo/pgsql.go` — conversion shim (copy `internal/tag/repo/pgsql.go` shape; whole-struct conversions compile because field types are override-unified and SELECT/INSERT column order matches):

```go
package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

func (pgsqlQuerier) GetRecurringTransactionByID(ctx context.Context, db backend.DBTX, id string) (rtRow, error) {
	row, err := pgsqlgen.New(db).GetRecurringTransactionByID(ctx, id)
	if err != nil {
		return rtRow{}, err
	}
	return rtRow(row), nil
}

func (pgsqlQuerier) UpsertRecurringTransaction(ctx context.Context, db backend.DBTX, arg upsertParams) error {
	return pgsqlgen.New(db).UpsertRecurringTransaction(ctx, pgsqlgen.UpsertRecurringTransactionParams(arg))
}

func (pgsqlQuerier) DeleteRecurringTransaction(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteRecurringTransaction(ctx, id)
}
```

If `rtRow(row)` fails to compile because pgsql's `GetRecurringTransactionByIDRow` field ORDER differs from sqlite's model struct, the SELECT column order in the two `.sql` files has drifted — fix the query files (Task 3), regenerate, don't hand-map fields. Check the exact sqlite adapter signature style in `internal/tag/repo/sqlite.go` (whether `New(db)` is per-call or cached) and mirror it.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/recurring/... -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Run against PostgreSQL too**

Run: `make test-repo-pgsql` (or, if a local pg is configured: `DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/recurring/...`)
Expected: PASS. If no PostgreSQL is available locally, note it — CI runs this tier; do not skip silently.

- [ ] **Step 7: Verify archtest sees the new feature cleanly**

Run: `go test ./internal/test/archtest/`
Expected: PASS (`internal/recurring` is auto-detected as a feature; it imports only model/shared/infra).

- [ ] **Step 8: Commit**

```bash
git add internal/recurring
git commit -m "feat(recurring): engine-adapter repository with integration tests"
```

---

### Task 5: Service — scaffolding, list read, API skeleton

**Files:**
- Create: `internal/recurring/ports.go`
- Create: `internal/recurring/usecase.go`
- Create: `internal/recurring/read.go`
- Create: `internal/recurring/api/handler.go`
- Create: `internal/recurring/api/recurring.go`
- Create: `internal/recurring/api/routes.go`
- Test: `internal/recurring/api/harness_test.go`, `internal/recurring/api/recurring_endpoints_test.go`

**Interfaces:**
- Consumes: Task 4's `Repository`; `port.TxRunner/OperationGuard/Clock` (`internal/shared/port`); `endpoint.Handle`/`HandleNoBody` (`internal/web/endpoint`); `middleware.Auth` + `router.RegisterAPI`; `authstub.Authenticator{}` for tests.
- Produces:
  ```go
  func NewService(repo Repository, accounts AccountResolver, grants AccountGrants,
  	visible VisibleAccounts, creator TransactionCreator,
  	tx port.TxRunner, ops port.OperationGuard, clock port.Clock) *Service
  func (s *Service) GetRecurringTransactionList(ctx context.Context, userID vo.Id) (*model.GetRecurringTransactionListResult, error)
  ```
  API: `api.NewHandlers(svc *recurring.Service, dev bool) *Handlers`, `api.RegisterAPI(h, authn, dev) router.RegisterAPI`, route `GET /api/v1/recurring/get-recurring-transaction-list`.

- [ ] **Step 1: Write `ports.go`**

```go
package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Consumer-side ports; internal/server wires the account, connection and
// transaction services onto these at composition time.
type AccountResolver interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
}

type AccountGrants interface {
	HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

type VisibleAccounts interface {
	VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

type TransactionCreator interface {
	CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error)
}
```

- [ ] **Step 2: Write `usecase.go`**

```go
package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Service struct {
	repo     Repository
	accounts AccountResolver
	grants   AccountGrants
	visible  VisibleAccounts
	creator  TransactionCreator
	tx       port.TxRunner
	ops      port.OperationGuard
	clock    port.Clock
}

func NewService(repo Repository, accounts AccountResolver, grants AccountGrants, visible VisibleAccounts, creator TransactionCreator, tx port.TxRunner, ops port.OperationGuard, clock port.Clock) *Service {
	return &Service{repo: repo, accounts: accounts, grants: grants, visible: visible, creator: creator, tx: tx, ops: ops, clock: clock}
}

// Same matrix as transaction writes: owner or admin/user grant; guest denied.
func (s *Service) checkWriteAccess(ctx context.Context, userID, accountID vo.Id) error {
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		return errs.NewValidation("account.account.not_available")
	}
	if owner.Equal(userID) {
		return nil
	}
	ok, err := s.grants.HasWriteGrant(ctx, accountID, userID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return errs.NewValidation("account.account.not_available")
}

func parseType(alias string) (model.TransactionType, error) {
	switch alias {
	case "expense":
		return model.TransactionTypeExpense, nil
	case "income":
		return model.TransactionTypeIncome, nil
	case "transfer":
		return model.TransactionTypeTransfer, nil
	}
	return 0, errs.NewValidation("Validation failed", errs.FieldError{Key: "type", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
}

func toResult(rt *model.RecurringTransaction) model.RecurringTransactionResult {
	return model.RecurringTransactionResult{
		Id:                 rt.ID.String(),
		OwnerUserId:        rt.UserID.String(),
		Type:               rt.Type.Alias(),
		AccountId:          rt.AccountID.String(),
		AccountRecipientId: idStr(rt.AccountRecipID),
		Amount:             rt.Amount,
		CategoryId:         idStr(rt.CategoryID),
		PayeeId:            idStr(rt.PayeeID),
		TagId:              idStr(rt.TagID),
		Description:        rt.Description,
		Schedule:           string(rt.Schedule),
		NextPaymentAt:      rt.NextPaymentAt.Format(datetime.Layout),
		CreatedAt:          rt.CreatedAt.Format(datetime.Layout),
		UpdatedAt:          rt.UpdatedAt.Format(datetime.Layout),
	}
}

func idStr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}
```

- [ ] **Step 3: Write `read.go`**

```go
package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) GetRecurringTransactionList(ctx context.Context, userID vo.Id) (*model.GetRecurringTransactionListResult, error) {
	accountIDs, err := s.visible.VisibleAccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListByAccountIDs(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	out := make([]model.RecurringTransactionResult, 0, len(items))
	for _, rt := range items {
		out = append(out, toResult(rt))
	}
	return &model.GetRecurringTransactionListResult{Items: out}, nil
}
```

- [ ] **Step 4: Write the API skeleton**

`internal/recurring/api/handler.go`:

```go
package api

import (
	apprecurring "github.com/econumo/econumo/internal/recurring"
	"github.com/econumo/econumo/internal/web/apidoc"
)

var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *apprecurring.Service
	dev bool
}

func NewHandlers(svc *apprecurring.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}
```

`internal/recurring/api/recurring.go` (list handler now; later tasks append the write handlers to this file):

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.GetRecurringTransactionListResult{}

// GetRecurringTransactionList handles GET /api/v1/recurring/get-recurring-transaction-list (auth).
//
// @Summary     List recurring transactions
// @Description Returns every recurring transaction template on accounts the caller can access.
// @Tags        Recurring
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetRecurringTransactionListResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/recurring/get-recurring-transaction-list [get]
func (h *Handlers) GetRecurringTransactionList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetRecurringTransactionList)
}
```

`internal/recurring/api/routes.go` (routes for later tasks are included now but commented OUT would break the guard regex — instead register only the list route now and ADD one line per later task):

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("GET /api/v1/recurring/get-recurring-transaction-list", auth(h.GetRecurringTransactionList))
	}
}
```

- [ ] **Step 5: Write the failing endpoint test + harness**

`internal/recurring/api/harness_test.go`: copy `internal/transaction/api/harness_test.go` (package `api_test`) verbatim, then adapt:
1. Keep the sqlite-in-memory + migrations + fixture seeding and the `do`/`token`/`envelope`/`mustUnmarshal` helpers unchanged.
2. Keep the construction of the real account service, `connectionrepo.NewAccountAccessResolver`, and the real transaction service exactly as the transaction harness builds them (the recurring service needs them as ports).
3. After the transaction service is built, add:
```go
recurringRepo := recurringrepo.NewRepo("sqlite", db.TX)
recurringSvc := apprecurring.NewService(recurringRepo, accountSvc, accountAccessResolver, accountSvc, transactionSvc, db.TX, opGuard, clk)
handlers := handlerrecurring.NewHandlers(recurringSvc, cfg.IsDev())
h := router.New(router.Deps{Cfg: cfg, DB: nil, RegisterAPI: handlerrecurring.RegisterAPI(handlers, authstub.Authenticator{}, cfg.IsDev())})
```
(match the harness's actual local variable names for `accountSvc`, `accountAccessResolver`, `transactionSvc`, `opGuard`, `clk`, `db`, `cfg` — keep whatever the copied file calls them). The harness must also expose a seeded second user with a shared-account grant if the transaction harness has one; if it doesn't, add a `seedGuest`-style helper later when Task 6 needs it.

`internal/recurring/api/recurring_endpoints_test.go`:

```go
package api_test

import (
	"net/http"
	"testing"
)

type recurringItem struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	AccountID     string  `json:"accountId"`
	Amount        string  `json:"amount"`
	Schedule      string  `json:"schedule"`
	NextPaymentAt string  `json:"nextPaymentAt"`
	CategoryID    *string `json:"categoryId"`
	Description   string  `json:"description"`
}

type recurringList struct {
	Items []recurringItem `json:"items"`
}

func TestGetRecurringTransactionList_EmptyByDefault(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[recurringList](t, env.Data)
	if len(res.Items) != 0 {
		t.Fatalf("expected empty list, got %d", len(res.Items))
	}
}

func TestGetRecurringTransactionList_RequiresAuth(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", status)
	}
}
```

- [ ] **Step 6: Run tests, verify fail, then pass**

Run: `go test ./internal/recurring/api/ -v`
Expected: first FAIL to compile while the harness is being adapted, then PASS.

- [ ] **Step 7: gofmt + build + archtest**

Run: `gofmt -l internal/recurring/ && go build ./... && go test ./internal/test/archtest/`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add internal/recurring
git commit -m "feat(recurring): service scaffolding, list read, API skeleton"
```

---

### Task 6: Create use case + endpoint

**Files:**
- Create: `internal/recurring/create.go`
- Modify: `internal/recurring/api/recurring.go` (append handler)
- Modify: `internal/recurring/api/routes.go` (add route)
- Test: `internal/recurring/api/recurring_endpoints_test.go` (append)

**Interfaces:**
- Produces: `(s *Service) CreateRecurringTransaction(ctx, userID vo.Id, req model.CreateRecurringTransactionRequest) (*model.CreateRecurringTransactionResult, error)`; route `POST /api/v1/recurring/create-recurring-transaction`.
- `req.Id` is the idempotency/operation id (entity gets a fresh UUIDv7), exactly like tag/transaction create.

- [ ] **Step 1: Write the failing tests** (append to `recurring_endpoints_test.go`)

```go
func createRecurringReq(opID, typ, amount string) map[string]any {
	return map[string]any{
		"id": opID, "type": typ, "amount": amount,
		"accountId": seedAccountID, // use the harness's seeded account id variable
		"schedule":  "monthly", "nextPaymentAt": "2026-08-31 00:00:00",
		"description": "rent",
	}
}

func TestCreateRecurringTransaction_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const opID = "0197c100-0000-7000-8000-000000000001"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "42.50"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data)
	if res.Item.ID == "" || res.Item.ID == opID {
		t.Fatalf("entity id must be fresh, got %q", res.Item.ID)
	}
	if res.Item.Schedule != "monthly" || res.Item.NextPaymentAt != "2026-08-31 00:00:00" || res.Item.Amount != "42.5" {
		t.Fatalf("unexpected item: %+v", res.Item)
	}

	// list now contains it
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	list := mustUnmarshal[recurringList](t, listEnv.Data)
	if len(list.Items) != 1 {
		t.Fatalf("list has %d items, want 1", len(list.Items))
	}
}

func TestCreateRecurringTransaction_IdempotencyReplayLocked(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	const opID = "0197c100-0000-7000-8000-000000000002"
	h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "10"))
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "10"))
	if status != http.StatusBadRequest {
		t.Fatalf("replay status=%d body=%s", status, env.raw)
	}
}

func TestCreateRecurringTransaction_TransferRequiresRecipient(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	req := createRecurringReq("0197c100-0000-7000-8000-000000000003", "transfer", "10")
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, req)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	if _, ok := env.errorsMap()["accountRecipientId"]; !ok {
		t.Fatalf("expected accountRecipientId field error, got %s", env.raw)
	}
}

func TestCreateRecurringTransaction_BadSchedule(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	req := createRecurringReq("0197c100-0000-7000-8000-000000000004", "expense", "10")
	req["schedule"] = "daily"
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, req)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
}
```

(`seedAccountID` = whatever the copied harness names the seeded account constant.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/recurring/api/ -run TestCreateRecurring -v`
Expected: FAIL (404 route not registered / compile error).

- [ ] **Step 3: Implement `internal/recurring/create.go`**

```go
package recurring

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) CreateRecurringTransaction(ctx context.Context, userID vo.Id, req model.CreateRecurringTransactionRequest) (*model.CreateRecurringTransactionResult, error) {
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	st, err := s.buildState(req.Type, req.AccountId, req.AccountRecipientId, req.Amount.String(),
		req.CategoryId, req.PayeeId, req.TagId, req.Description, req.Schedule, req.NextPaymentAt)
	if err != nil {
		return nil, err
	}
	st.ID = vo.NewId()
	st.UserID = userID

	var created *model.RecurringTransaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.checkWriteAccess(ctx, userID, st.AccountID); aerr != nil {
			return aerr
		}
		now := s.clock.Now()
		already, cerr := s.ops.Claim(ctx, opID, now)
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}
		st.CreatedAt = now
		created = model.NewRecurringTransaction(st)
		if serr := s.repo.Save(ctx, created); serr != nil {
			return serr
		}
		return s.ops.MarkHandled(ctx, opID, now)
	}); err != nil {
		return nil, err
	}
	return &model.CreateRecurringTransactionResult{Item: toResult(created)}, nil
}

// buildState parses and validates the shared create/update payload into a
// RecurringNewState (ID/UserID/CreatedAt left for the caller).
func (s *Service) buildState(typAlias, accountID string, accountRecipID *string, amount string,
	categoryID, payeeID, tagID *string, description *string, schedule, nextPaymentAt string) (model.RecurringNewState, error) {
	var st model.RecurringNewState

	typ, err := parseType(typAlias)
	if err != nil {
		return st, err
	}
	accID, err := vo.ParseId(accountID)
	if err != nil {
		return st, err
	}
	sched, ok := model.ParseRecurringSchedule(schedule)
	if !ok {
		return st, errs.NewValidation("Validation failed", errs.FieldError{Key: "schedule", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
	}
	nextAt, err := time.Parse(datetime.Layout, nextPaymentAt)
	if err != nil {
		return st, errs.NewValidation("Validation failed", errs.FieldError{Key: "nextPaymentAt", Message: "This value is not valid.", Code: "INVALID_FORMAT_ERROR"})
	}
	recip, err := parseOptID(accountRecipID)
	if err != nil {
		return st, err
	}
	if typ.IsTransfer() && recip == nil {
		return st, errs.NewValidation("Validation failed", errs.FieldError{Key: "accountRecipientId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	cat, err := parseOptID(categoryID)
	if err != nil {
		return st, err
	}
	payee, err := parseOptID(payeeID)
	if err != nil {
		return st, err
	}
	tag, err := parseOptID(tagID)
	if err != nil {
		return st, err
	}

	st.Type = typ
	st.AccountID = accID
	st.AccountRecipID = recip
	st.Amount = vo.NewDecimal(amount).String()
	st.CategoryID = cat
	st.PayeeID = payee
	st.TagID = tag
	if description != nil {
		st.Description = *description
	}
	st.Schedule = sched
	st.NextPaymentAt = nextAt
	return st, nil
}

func parseOptID(s *string) (*vo.Id, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
```

- [ ] **Step 4: Add the handler + route**

Append to `internal/recurring/api/recurring.go`:

```go
// CreateRecurringTransaction handles POST /api/v1/recurring/create-recurring-transaction (auth).
//
// @Summary     Create a recurring transaction
// @Description Creates a recurring transaction template. Idempotent on the request id.
// @Tags        Recurring
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateRecurringTransactionRequest true "Create recurring transaction request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateRecurringTransactionResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/recurring/create-recurring-transaction [post]
func (h *Handlers) CreateRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.CreateRecurringTransaction)
}
```

Add to `routes.go`:

```go
		mux.Handle("POST /api/v1/recurring/create-recurring-transaction", auth(h.CreateRecurringTransaction))
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/recurring/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/recurring
git commit -m "feat(recurring): create use case + endpoint"
```

---

### Task 7: Update + Delete use cases + endpoints

**Files:**
- Create: `internal/recurring/update.go`, `internal/recurring/delete.go`
- Modify: `internal/recurring/api/recurring.go`, `internal/recurring/api/routes.go`
- Test: `internal/recurring/api/recurring_endpoints_test.go` (append)

**Interfaces:**
- Produces: `UpdateRecurringTransaction(ctx, userID, req model.UpdateRecurringTransactionRequest) (*model.UpdateRecurringTransactionResult, error)`, `DeleteRecurringTransaction(ctx, userID, req model.DeleteRecurringTransactionRequest) (*model.DeleteRecurringTransactionResult, error)`; routes `POST /api/v1/recurring/update-recurring-transaction`, `POST /api/v1/recurring/delete-recurring-transaction`. Here `req.Id` is the TEMPLATE id (no op id — mirrors tag update/delete).

- [ ] **Step 1: Write the failing tests** (append; create a template via the create endpoint first, read its fresh id from the response, then update/delete it)

```go
func createTemplate(t *testing.T, h harness, tok string) recurringItem { // adjust receiver type to the harness's actual type name
	t.Helper()
	opID := "0197c1ff-" + randomTail(t) // or just use distinct hardcoded op ids per test
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/create-recurring-transaction", tok, createRecurringReq(opID, "expense", "42.50"))
	if status != http.StatusOK {
		t.Fatalf("create failed: %s", env.raw)
	}
	return mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data).Item
}

func TestUpdateRecurringTransaction_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok)
	body := map[string]any{
		"id": item.ID, "type": "expense", "amount": "99",
		"accountId": seedAccountID, "schedule": "weekly",
		"nextPaymentAt": "2026-09-05 00:00:00", "description": "updated",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/update-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data)
	if res.Item.Schedule != "weekly" || res.Item.Amount != "99" || res.Item.NextPaymentAt != "2026-09-05 00:00:00" {
		t.Fatalf("unexpected item: %+v", res.Item)
	}
}

func TestUpdateRecurringTransaction_NotFound(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	body := map[string]any{
		"id": "0197c1aa-0000-7000-8000-000000000099", "type": "expense", "amount": "1",
		"accountId": seedAccountID, "schedule": "weekly", "nextPaymentAt": "2026-09-05 00:00:00",
	}
	status, _ := h.do(t, http.MethodPost, "/api/v1/recurring/update-recurring-transaction", tok, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
}

func TestDeleteRecurringTransaction_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/delete-recurring-transaction", tok, map[string]any{"id": item.ID})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	if list := mustUnmarshal[recurringList](t, listEnv.Data); len(list.Items) != 0 {
		t.Fatalf("template still listed after delete")
	}
}
```

(If a `randomTail` helper is awkward, use fixed distinct op-id constants per test — the harness DB is fresh per test.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/recurring/api/ -run 'TestUpdateRecurring|TestDeleteRecurring' -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/recurring/update.go`:

```go
package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) UpdateRecurringTransaction(ctx context.Context, userID vo.Id, req model.UpdateRecurringTransactionRequest) (*model.UpdateRecurringTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	st, err := s.buildState(req.Type, req.AccountId, req.AccountRecipientId, req.Amount.String(),
		req.CategoryId, req.PayeeId, req.TagId, req.Description, req.Schedule, req.NextPaymentAt)
	if err != nil {
		return nil, err
	}

	var updated *model.RecurringTransaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rt, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		// moving the template to another account also needs write access there
		if !st.AccountID.Equal(rt.AccountID) {
			if aerr := s.checkWriteAccess(ctx, userID, st.AccountID); aerr != nil {
				return aerr
			}
		}
		rt.Update(st, s.clock.Now())
		updated = rt
		return s.repo.Save(ctx, rt)
	}); err != nil {
		return nil, err
	}
	return &model.UpdateRecurringTransactionResult{Item: toResult(updated)}, nil
}
```

`internal/recurring/delete.go`:

```go
package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) DeleteRecurringTransaction(ctx context.Context, userID vo.Id, req model.DeleteRecurringTransactionRequest) (*model.DeleteRecurringTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rt, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}
	return &model.DeleteRecurringTransactionResult{}, nil
}
```

Handlers (append to `api/recurring.go`, same annotation pattern as CreateRecurringTransaction with `model.UpdateRecurringTransactionRequest`/`Result` and `model.DeleteRecurringTransactionRequest`/`Result`; routers `[post] /api/v1/recurring/update-recurring-transaction` and `/api/v1/recurring/delete-recurring-transaction`):

```go
func (h *Handlers) UpdateRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.UpdateRecurringTransaction)
}

func (h *Handlers) DeleteRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.DeleteRecurringTransaction)
}
```

Routes:

```go
		mux.Handle("POST /api/v1/recurring/update-recurring-transaction", auth(h.UpdateRecurringTransaction))
		mux.Handle("POST /api/v1/recurring/delete-recurring-transaction", auth(h.DeleteRecurringTransaction))
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/recurring/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/recurring
git commit -m "feat(recurring): update + delete use cases and endpoints"
```

---

### Task 8: Post + Skip use cases + endpoints

**Files:**
- Create: `internal/recurring/post.go`, `internal/recurring/skip.go`
- Modify: `internal/recurring/api/recurring.go`, `internal/recurring/api/routes.go`
- Test: `internal/recurring/api/recurring_endpoints_test.go` (append)

**Interfaces:**
- Produces: `PostRecurringTransaction(ctx, userID, req model.PostRecurringTransactionRequest) (*model.PostRecurringTransactionResult, error)`, `SkipRecurringTransaction(ctx, userID, req model.SkipRecurringTransactionRequest) (*model.SkipRecurringTransactionResult, error)`; routes `POST /api/v1/recurring/post-recurring-transaction`, `POST /api/v1/recurring/skip-recurring-transaction`.
- Atomicity: `TxManager.WithTx` nests via SAVEPOINT when a tx is already on the context (`internal/infra/storage/backend/tx.go`), so wrapping the port call + advance in one `WithTx` makes post atomic — the transaction service's inner `WithTx` joins as a savepoint. Both services must share the same `*backend.TxManager` (they do: `txm` in `server.BuildAPI`, `db.TX` in the harness).
- Idempotency: the inner `CreateTransaction` claims `req.Id`; a replayed post fails with `"Operation is locked"` and the whole outer tx (including the advance) rolls back — no double-create, no double-advance.

- [ ] **Step 1: Write the failing tests** (append)

```go
func TestPostRecurringTransaction_CreatesAndAdvances(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok) // nextPaymentAt 2026-08-31, monthly

	const txOpID = "0197c200-0000-7000-8000-000000000001"
	body := map[string]any{
		"recurringId": item.ID, "id": txOpID, "type": "expense", "amount": "42.50",
		"accountId": seedAccountID, "date": "2026-08-31 00:00:00", "description": "rent",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item          struct{ ID, Amount, Date string } `json:"item"`
		NextPaymentAt string                            `json:"nextPaymentAt"`
	}](t, env.Data)
	if res.Item.ID == "" || res.Item.Amount != "42.5" {
		t.Fatalf("unexpected transaction: %+v", res.Item)
	}
	if res.NextPaymentAt != "2026-09-30 00:00:00" {
		t.Fatalf("nextPaymentAt = %q, want advanced one month (clamped Sep 30)", res.NextPaymentAt)
	}

	// the real transaction exists
	_, txEnv := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	_ = txEnv // assert the item id appears in txEnv.Data if the harness registers transaction routes; otherwise skip this check
}

func TestPostRecurringTransaction_ReplayIsLocked(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok)
	const txOpID = "0197c200-0000-7000-8000-000000000002"
	body := map[string]any{
		"recurringId": item.ID, "id": txOpID, "type": "expense", "amount": "10",
		"accountId": seedAccountID, "date": "2026-08-31 00:00:00",
	}
	h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/post-recurring-transaction", tok, body)
	if status != http.StatusBadRequest {
		t.Fatalf("replay status=%d body=%s", status, env.raw)
	}
	// schedule advanced exactly once
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/recurring/get-recurring-transaction-list", tok, nil)
	list := mustUnmarshal[recurringList](t, listEnv.Data)
	if list.Items[0].NextPaymentAt != "2026-09-30 00:00:00" {
		t.Fatalf("nextPaymentAt = %q after replay, want single advance", list.Items[0].NextPaymentAt)
	}
}

func TestSkipRecurringTransaction_AdvancesWithoutTransaction(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	item := createTemplate(t, h, tok)
	status, env := h.do(t, http.MethodPost, "/api/v1/recurring/skip-recurring-transaction", tok, map[string]any{"id": item.ID})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item recurringItem `json:"item"`
	}](t, env.Data)
	if res.Item.NextPaymentAt != "2026-09-30 00:00:00" {
		t.Fatalf("nextPaymentAt = %q, want 2026-09-30 00:00:00", res.Item.NextPaymentAt)
	}
}
```

If the recurring harness registers ONLY recurring routes, drop the `get-transaction-list` call in the first test and instead verify via the repo or extend the harness to also register `handlertransaction.RegisterAPI` in the same `router.Compose` — preferred, it makes the post test end-to-end.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/recurring/api/ -run 'TestPostRecurring|TestSkipRecurring' -v`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/recurring/post.go`**

```go
package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) PostRecurringTransaction(ctx context.Context, userID vo.Id, req model.PostRecurringTransactionRequest) (*model.PostRecurringTransactionResult, error) {
	rtID, err := vo.ParseId(req.RecurringId)
	if err != nil {
		return nil, err
	}
	createReq := model.CreateTransactionRequest{
		Id:                 req.Id,
		Type:               req.Type,
		Amount:             req.Amount,
		AmountRecipient:    req.AmountRecipient,
		AccountId:          req.AccountId,
		AccountRecipientId: req.AccountRecipientId,
		CategoryId:         req.CategoryId,
		PayeeId:            req.PayeeId,
		TagId:              req.TagId,
		Description:        req.Description,
		Date:               req.Date,
	}
	if verr := createReq.Validate(); verr != nil {
		return nil, verr
	}

	var created *model.CreateTransactionResult
	var rt *model.RecurringTransaction
	// One outer tx: the transaction service's inner WithTx nests as a
	// SAVEPOINT, so create + advance commit or roll back together.
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		var gerr error
		rt, gerr = s.repo.GetByID(ctx, rtID)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		created, gerr = s.creator.CreateTransaction(ctx, userID, createReq)
		if gerr != nil {
			return gerr
		}
		rt.Advance(s.clock.Now())
		return s.repo.Save(ctx, rt)
	}); err != nil {
		return nil, err
	}
	return &model.PostRecurringTransactionResult{
		Item:          created.Item,
		Accounts:      created.Accounts,
		NextPaymentAt: rt.NextPaymentAt.Format(datetime.Layout),
	}, nil
}
```

Check the exact field set of `model.CreateTransactionRequest` in `internal/model/transaction_dto.go` before writing `createReq` — copy every field it has.

`internal/recurring/skip.go`:

```go
package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) SkipRecurringTransaction(ctx context.Context, userID vo.Id, req model.SkipRecurringTransactionRequest) (*model.SkipRecurringTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	var rt *model.RecurringTransaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		var gerr error
		rt, gerr = s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		rt.Advance(s.clock.Now())
		return s.repo.Save(ctx, rt)
	}); err != nil {
		return nil, err
	}
	return &model.SkipRecurringTransactionResult{Item: toResult(rt)}, nil
}
```

Handlers (append; same annotation pattern, `model.PostRecurringTransactionRequest`/`Result`, `model.SkipRecurringTransactionRequest`/`Result`):

```go
func (h *Handlers) PostRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.PostRecurringTransaction)
}

func (h *Handlers) SkipRecurringTransaction(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.SkipRecurringTransaction)
}
```

Routes:

```go
		mux.Handle("POST /api/v1/recurring/post-recurring-transaction", auth(h.PostRecurringTransaction))
		mux.Handle("POST /api/v1/recurring/skip-recurring-transaction", auth(h.SkipRecurringTransaction))
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/recurring/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/recurring
git commit -m "feat(recurring): post + skip use cases and endpoints"
```

---

### Task 9: Server wiring + swagger

**Files:**
- Modify: `internal/server/server.go` (wire repo/service/handlers + Compose entry)
- Modify: `Makefile` (add `../../recurring` to the `SWAG_INIT -d` list)
- Modify (generated, commit them): `internal/web/apidoc/docs/{docs.go,swagger.json,swagger.yaml}`

**Interfaces:**
- Consumes: everything from Tasks 4–8. `accountSvc` satisfies `AccountResolver` + `VisibleAccounts`; `accountAccessResolver` (connection repo adapter) satisfies `AccountGrants`; `transactionSvc` satisfies `TransactionCreator` — pass them directly, no glue files needed (the ports match the providers' method sets exactly).

- [ ] **Step 1: Wire the feature in `server.BuildAPI`**

In `internal/server/server.go`, after the transaction service is built (find `transactionSvc := apptransaction.NewService(...)`), add:

```go
	recurringRepo := recurringrepo.NewRepo(cfg.DatabaseDriver, txm)
	recurringSvc := apprecurring.NewService(recurringRepo, accountSvc, accountAccessResolver, accountSvc, transactionSvc, txm, opGuard, clk)
	recurringHandlers := handlerrecurring.NewHandlers(recurringSvc, cfg.IsDev())
```

with imports:

```go
	apprecurring "github.com/econumo/econumo/internal/recurring"
	handlerrecurring "github.com/econumo/econumo/internal/recurring/api"
	recurringrepo "github.com/econumo/econumo/internal/recurring/repo"
```

and in the `router.Compose(...)` list add:

```go
		handlerrecurring.RegisterAPI(recurringHandlers, userSvc, cfg.IsDev()),
```

- [ ] **Step 2: Build + full backend test sweep**

Run: `go build ./... && go test ./internal/...`
Expected: PASS everywhere EXCEPT `internal/test/apiparity` guards (`TestGuard_EveryRouteHasScenario` now fails: 6 registered routes have no scenario). That failure is expected and fixed in Task 10.

- [ ] **Step 3: Add the module to swag and regenerate**

In `Makefile`, find the `SWAG_INIT` definition and add `,../../recurring` to the `-d` list (after `../../budget`). Then:

Run: `make swagger && git diff --stat internal/web/apidoc/docs/`
Expected: the docs pick up 6 new `/api/v1/recurring/*` paths.

- [ ] **Step 4: Verify docs freshness check passes**

Run: `make go-lint`
Expected: PASS (build + vet + gofmt + swagger-check).

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go Makefile internal/web/apidoc/docs
git commit -m "feat(recurring): wire feature into server + OpenAPI docs"
```

---

### Task 10: apiparity scenarios + goldens + guard bumps

**Files:**
- Create: `internal/test/apiparity/catalogue_recurring.go`
- Create (generated): `internal/test/apiparity/testdata/golden/recurring_crud.golden`, `recurring_post.golden`
- Modify: `internal/test/apiparity/guard_test.go` (`minRoutes` 85 → 91)
- Modify: `internal/test/apiparity/catalogue_test.go` (`min` 33 → 35)

**Interfaces:**
- Consumes: `register(Scenario{...})`, `Call` (with `CaptureIDInto`), fixture constants `OwnerAccount`, `CatFood`, `ClockTime` from `internal/test/apiparity/fixture.go`. Verify the exact constant names in that file first.

- [ ] **Step 1: Write the scenarios**

`internal/test/apiparity/catalogue_recurring.go`:

```go
package apiparity

func init() {
	register(Scenario{Name: "recurring_crud", Calls: func() []Call {
		const opCreate = "e0000000-0000-0000-0000-0000000000a1"
		var rtID string
		return []Call{
			{Label: "create-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opCreate, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": "2026-08-31 00:00:00",
					"description": "rent",
				}, CaptureIDInto: &rtID},
			{Label: "list-after-create", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
			{Label: "update-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/update-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": &rtID, "type": "expense", "amount": "60.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "weekly", "nextPaymentAt": "2026-09-05 00:00:00",
					"description": "rent updated",
				}},
			{Label: "skip-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/skip-recurring-transaction", Auth: "owner",
				Body: map[string]any{"id": &rtID}},
			{Label: "delete-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/delete-recurring-transaction", Auth: "owner",
				Body: map[string]any{"id": &rtID}},
			{Label: "list-after-delete", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
		}
	}})

	register(Scenario{Name: "recurring_post", Calls: func() []Call {
		const opCreate = "e0000000-0000-0000-0000-0000000000b1"
		const opTx = "e0000000-0000-0000-0000-0000000000b2"
		var rtID string
		return []Call{
			{Label: "create-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opCreate, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": "2026-08-31 00:00:00",
					"description": "rent",
				}, CaptureIDInto: &rtID},
			{Label: "post-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/post-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"recurringId": &rtID, "id": opTx, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"date": "2026-08-31 00:00:00", "description": "rent",
				}},
			{Label: "recurring-list-after-post", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
			{Label: "transaction-list-after-post", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner"},
		}
	}})
}
```

Match the surrounding catalogue style: check how existing scenarios pass captured ids (`&rtID` as `*string` in body maps) in `catalogue.go` and copy exactly.

- [ ] **Step 2: Bump the guards**

- `internal/test/apiparity/guard_test.go`: `const minRoutes = 85` → `91`.
- `internal/test/apiparity/catalogue_test.go`: `const min = 33` → `35`.

- [ ] **Step 3: Generate goldens and INSPECT them**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/golden/`
Expected: exactly 2 NEW golden files, zero changes to existing goldens (any existing-golden diff means observable behavior of an existing endpoint changed — stop and investigate). Read both new goldens end to end: every non-`err:` call must be `-> 200`, `nextPaymentAt` values normalized to `<datetime>`, the skip/post advances visible in structure.

- [ ] **Step 4: Run the suite for real**

Run: `go test ./internal/test/apiparity/`
Expected: PASS including all guards.

- [ ] **Step 5: Full smoke tier**

Run: `make go-test`
Expected: PASS (build + vet + gofmt + swagger-fresh + tests + coverage gate ≥ 72%).

- [ ] **Step 6: Commit**

```bash
git add internal/test/apiparity
git commit -m "test(recurring): apiparity scenarios, goldens, guard floors"
```

---

### Task 11: Frontend — API client, DTOs, query hooks

**Files:**
- Create: `web/src/api/dto/recurring.ts`
- Create: `web/src/api/recurring.ts`
- Modify: `web/src/app/queryKeys.ts`
- Create: `web/src/features/recurring/queries.ts`
- Test: `web/src/features/recurring/queries.test.tsx`

**Interfaces:**
- Consumes: `api`/`apiUrl` (`web/src/api/client.ts`), `coerceTransaction`/`coerceAccount` (`web/src/api/account.ts` + `web/src/api/transaction.ts` — check where each lives), `CreateTransactionDto`/`TransactionDto` (`web/src/api/dto/transaction.ts`).
- Produces (later tasks import these): `RecurringDto`, `CreateRecurringDto`, `RecurringSchedule`, `PostRecurringPayload`, `PostRecurringResult`; hooks `useRecurring()`, `useCreateRecurring()`, `useUpdateRecurring()`, `useDeleteRecurring()`, `useSkipRecurring()`, `usePostRecurring()`; `queryKeys.recurring`.

- [ ] **Step 1: Write the DTOs**

`web/src/api/dto/recurring.ts`:

```ts
import type { Id } from '../types'
import type { AccountDto } from './account'
import type { CreateTransactionDto, TransactionDto, TransactionType } from './transaction'

export type RecurringSchedule = 'weekly' | 'biweekly' | 'monthly' | 'quarterly' | 'yearly'

export interface CreateRecurringDto {
  id: Id
  type: TransactionType
  accountId: Id
  accountRecipientId: Id | null
  amount: number
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  description: string
  schedule: RecurringSchedule
  nextPaymentAt: string
}

export interface RecurringDto extends CreateRecurringDto {
  ownerUserId: Id
}

export interface PostRecurringPayload extends CreateTransactionDto {
  recurringId: Id
}

export interface PostRecurringResult {
  item: TransactionDto
  accounts: AccountDto[]
  nextPaymentAt: string
}
```

- [ ] **Step 2: Write the client**

`web/src/api/recurring.ts` (envelope + coercion pattern copied from `web/src/api/transaction.ts` — amounts arrive as decimal strings):

```ts
import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CreateRecurringDto, PostRecurringPayload, PostRecurringResult, RecurringDto } from './dto/recurring'
import { coerceAccount, coerceTransaction } from './account'

interface Envelope<T> {
  data: T
}

function coerceRecurring(raw: RecurringDto): RecurringDto {
  return { ...raw, amount: Number(raw.amount) }
}

export async function getRecurringList(): Promise<RecurringDto[]> {
  const response = await api.get<Envelope<{ items: RecurringDto[] }>>(apiUrl('/api/v1/recurring/get-recurring-transaction-list'))
  return response.data.data.items.map(coerceRecurring)
}

export async function createRecurring(form: CreateRecurringDto): Promise<RecurringDto> {
  const response = await api.post<Envelope<{ item: RecurringDto }>>(apiUrl('/api/v1/recurring/create-recurring-transaction'), form)
  return coerceRecurring(response.data.data.item)
}

export async function updateRecurring(form: CreateRecurringDto): Promise<RecurringDto> {
  const response = await api.post<Envelope<{ item: RecurringDto }>>(apiUrl('/api/v1/recurring/update-recurring-transaction'), form)
  return coerceRecurring(response.data.data.item)
}

export async function deleteRecurring(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/recurring/delete-recurring-transaction'), { id })
}

export async function skipRecurring(id: Id): Promise<RecurringDto> {
  const response = await api.post<Envelope<{ item: RecurringDto }>>(apiUrl('/api/v1/recurring/skip-recurring-transaction'), { id })
  return coerceRecurring(response.data.data.item)
}

export async function postRecurring(payload: PostRecurringPayload): Promise<PostRecurringResult> {
  const response = await api.post<Envelope<PostRecurringResult>>(apiUrl('/api/v1/recurring/post-recurring-transaction'), payload)
  const { item, accounts, nextPaymentAt } = response.data.data
  return { item: coerceTransaction(item), accounts: accounts.map(coerceAccount), nextPaymentAt }
}
```

(Check whether `coerceTransaction`/`coerceAccount` are exported from `web/src/api/account.ts` — `transaction.ts` imports them from there; import from the same place.)

- [ ] **Step 3: Add the query key**

In `web/src/app/queryKeys.ts` add to the object: `recurring: ['recurring'] as const,`

- [ ] **Step 4: Write the failing hook tests**

`web/src/features/recurring/queries.test.tsx` — copy the wrapper helper from `web/src/features/transactions/queries.test.tsx` (`makeWrapper`, `beforeEach` clearing localStorage/econumoConfig):

```tsx
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { RecurringDto } from '@/api/dto/recurring'
import { usePostRecurring, useRecurring, useSkipRecurring } from './queries'

const wireRecurring = {
  id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '50.5', categoryId: 'c1', payeeId: null, tagId: null, description: 'rent',
  schedule: 'monthly', nextPaymentAt: '2026-08-31 00:00:00',
}

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('useRecurring fetches and coerces amounts', async () => {
  server.use(http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [wireRecurring] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useRecurring(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data![0].amount).toBe(50.5)
})

it('useSkipRecurring updates the cached template', async () => {
  server.use(http.post('*/api/v1/recurring/skip-recurring-transaction', () =>
    HttpResponse.json({ success: true, message: '', data: { item: { ...wireRecurring, nextPaymentAt: '2026-09-30 00:00:00' } } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, [{ ...wireRecurring, amount: 50.5 } as RecurringDto])
  const { result } = renderHook(() => useSkipRecurring(), { wrapper })
  result.current.mutate('r1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  const cached = queryClient.getQueryData<RecurringDto[]>(queryKeys.recurring)
  expect(cached![0].nextPaymentAt).toBe('2026-09-30 00:00:00')
})

it('usePostRecurring prepends the transaction, replaces accounts, advances the template', async () => {
  const wireTx = {
    id: 't1', author: { id: 'u1', name: 'U', avatar: 'face:fuchsia' }, type: 'expense',
    accountId: 'a1', accountRecipientId: null, amount: '50.5', amountRecipient: null,
    categoryId: 'c1', description: 'rent', payeeId: null, tagId: null, date: '2026-08-31 00:00:00',
  }
  server.use(http.post('*/api/v1/recurring/post-recurring-transaction', () =>
    HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [], nextPaymentAt: '2026-09-30 00:00:00' } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, [{ ...wireRecurring, amount: 50.5 } as RecurringDto])
  queryClient.setQueryData(queryKeys.transactions, [])
  const { result } = renderHook(() => usePostRecurring(), { wrapper })
  result.current.mutate({ recurringId: 'r1', id: 'op1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 50.5, amountRecipient: null, categoryId: 'c1', description: 'rent', payeeId: null, tagId: null, date: '2026-08-31 00:00:00' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect((queryClient.getQueryData(queryKeys.transactions) as unknown[]).length).toBe(1)
  expect(queryClient.getQueryData<RecurringDto[]>(queryKeys.recurring)![0].nextPaymentAt).toBe('2026-09-30 00:00:00')
})
```

- [ ] **Step 5: Run to verify failure**

Run: `cd web && pnpm vitest run src/features/recurring/queries.test.tsx`
Expected: FAIL — module `./queries` not found.

- [ ] **Step 6: Write the hooks**

`web/src/features/recurring/queries.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as recurringApi from '@/api/recurring'
import type { CreateRecurringDto, PostRecurringPayload, RecurringDto } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { TransactionDto } from '@/api/dto/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'

export function useRecurring() {
  return useQuery({
    queryKey: queryKeys.recurring,
    queryFn: recurringApi.getRecurringList,
    staleTime: TEN_MINUTES,
    select: (items) => [...items].sort((a, b) => (a.nextPaymentAt < b.nextPaymentAt ? -1 : a.nextPaymentAt > b.nextPaymentAt ? 1 : 0)),
  })
}

function useReplaceRecurring() {
  const queryClient = useQueryClient()
  return (item: RecurringDto) => {
    queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) => {
      const items = prev ?? []
      return items.some((r) => r.id === item.id) ? items.map((r) => (r.id === item.id ? item : r)) : [...items, item]
    })
  }
}

export function useCreateRecurring() {
  const replace = useReplaceRecurring()
  return useMutation({ mutationFn: recurringApi.createRecurring, onSuccess: replace })
}

export function useUpdateRecurring() {
  const replace = useReplaceRecurring()
  return useMutation({
    mutationFn: (form: CreateRecurringDto) => recurringApi.updateRecurring(form),
    onSuccess: replace,
  })
}

export function useDeleteRecurring() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => recurringApi.deleteRecurring(id),
    onSuccess: (_res, id) => {
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) => (prev ?? []).filter((r) => r.id !== id))
    },
  })
}

export function useSkipRecurring() {
  const replace = useReplaceRecurring()
  return useMutation({ mutationFn: (id: Id) => recurringApi.skipRecurring(id), onSuccess: replace })
}

export function usePostRecurring() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (payload: PostRecurringPayload) => recurringApi.postRecurring(payload),
    onSuccess: (result, payload) => {
      queryClient.setQueryData(queryKeys.accounts, result.accounts)
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) => [result.item, ...(prev ?? [])])
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) =>
        (prev ?? []).map((r) => (r.id === payload.recurringId ? { ...r, nextPaymentAt: result.nextPaymentAt } : r)),
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
    },
  })
}
```

Caveat for the test above: `usePostRecurring` replaces the accounts cache with `[]` in that fixture — if any assertion depends on accounts, seed a non-empty accounts payload instead.

- [ ] **Step 7: Run tests + lint**

Run: `cd web && pnpm vitest run src/features/recurring/ && pnpm lint`
Expected: PASS, no new lint errors.

- [ ] **Step 8: Commit**

```bash
git add web/src/api/dto/recurring.ts web/src/api/recurring.ts web/src/app/queryKeys.ts web/src/features/recurring
git commit -m "feat(web/recurring): API client, DTOs, query hooks"
```

---

### Task 12: Frontend — settings page + route + i18n

**Files:**
- Modify: `web/src/app/router-pages.ts` (add `SETTINGS_RECURRING: '/settings/recurring'`)
- Modify: `web/src/app/routes.tsx` (register the route)
- Modify: `web/src/features/settings/SettingsPage.tsx` (Finance-section `MenuRow`)
- Create: `web/src/features/recurring/RecurringSettingsPage.tsx`
- Modify: `web/src/locales/en-US.ts` (all recurring strings for Tasks 12–16, added once here)
- Test: `web/src/features/recurring/RecurringSettingsPage.test.tsx`

**Interfaces:**
- Consumes: `useRecurring` (Task 11), `useAccounts`/`useCategories`/`usePayees` (existing hooks), `MenuRow`/`MenuGroup` (local to SettingsPage), `moneyFormat` (`@/lib/money`), `EntityIcon`.
- Produces: route `/settings/recurring`; page emits `onSelect(rt: RecurringDto)` into a view dialog placeholder (wired for real in Task 14) and a create button calling `openRecurringModal({})` (store action added in Task 13 — for THIS task, render the button disabled with a `data-testid="recurring-create"`; Task 13 enables it).

- [ ] **Step 1: Add i18n strings** (single locale file `web/src/locales/en-US.ts`; brace interpolation). Add under the existing `pages.settings` object a sibling key and a new `modals.recurring` block:

```ts
// inside pages.settings:
recurring: {
  menu_item: 'Recurring transactions',
  header: 'Recurring transactions',
  empty: 'No recurring transactions yet',
  create: 'Add recurring transaction',
  delete_question: 'Delete this recurring transaction?',
},
// inside modals (new block):
recurring: {
  create_form: { header: 'Add recurring transaction' },
  update_form: { header: 'Edit recurring transaction' },
  post_form: { header: 'Post recurring transaction' },
  form: {
    schedule: { label: 'Repeats' },
    next_payment: { label: 'Next payment' },
  },
  schedule: {
    weekly: 'Weekly',
    biweekly: 'Every 2 weeks',
    monthly: 'Monthly',
    quarterly: 'Every 3 months',
    yearly: 'Yearly',
  },
  preview: {
    header: 'Recurring transaction',
    post: 'Post',
    skip: 'Skip',
    next_payment: 'Next payment',
    schedule: 'Repeats',
  },
  make_recurring: 'Make recurring',
},
```

- [ ] **Step 2: Route + settings entry**

- `web/src/app/router-pages.ts`: add `SETTINGS_RECURRING: '/settings/recurring',` to the `RouterPage` object.
- `web/src/app/routes.tsx`: import `RecurringSettingsPage` from `@/features/recurring/RecurringSettingsPage` and add `{ path: '/settings/recurring', element: <RecurringSettingsPage /> },` next to the other `/settings/*` children.
- `web/src/features/settings/SettingsPage.tsx`: in the Finance group (`pages.settings.settings.groups.service`), add:

```tsx
<MenuRow label={t('pages.settings.recurring.menu_item')} to={RouterPage.SETTINGS_RECURRING} />
```

- [ ] **Step 3: Write the failing page test**

`web/src/features/recurring/RecurringSettingsPage.test.tsx` — use the dialog-test setup pattern from `TransactionDialog.test.tsx` (QueryClientProvider + `createMemoryRouter` + `server.use(...coreHandlers())` + `mockMatchMedia`), plus a recurring-list handler:

```tsx
// setup: server.use(...coreHandlers(), http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
//   HttpResponse.json({ success: true, message: '', data: { items: [wireRecurring] } })))
it('lists templates with schedule and next payment date', async () => {
  renderPage() // helper: router at /settings/recurring rendering <RecurringSettingsPage />
  expect(await screen.findByText('rent')).toBeInTheDocument()
  expect(screen.getByText('Monthly')).toBeInTheDocument()
})

it('shows the empty state when there are no templates', async () => {
  // recurring handler returning { items: [] }
  renderPage()
  expect(await screen.findByText('No recurring transactions yet')).toBeInTheDocument()
})
```

Use a `wireRecurring` fixture whose `accountId`/`categoryId` match ids in `fixtureAccounts`/`fixtureCategories` from `@/test/fixtures` so name resolution renders.

- [ ] **Step 4: Implement the page**

`web/src/features/recurring/RecurringSettingsPage.tsx` — list page, one tappable row per template (model the row markup on the settings list pages, e.g. `CategoriesPage`; header layout copied from a sibling settings page):

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecurringDto } from '@/api/dto/recurring'
import { EntityIcon } from '@/components/EntityIcon' // match the actual import path used by TransactionRow
import { Button } from '@/components/ui/button'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees } from '@/features/classifications/queries'
import { moneyFormat } from '@/lib/money'
import { dayKey } from '@/lib/datetime'
import { useRecurring } from './queries'

export function RecurringSettingsPage() {
  const { t } = useTranslation()
  const { data: recurring } = useRecurring()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const [selected, setSelected] = useState<RecurringDto | null>(null)

  const scheduleLabel = (rt: RecurringDto) => t(`modals.recurring.schedule.${rt.schedule}`)
  const accountOf = (rt: RecurringDto) => accounts?.find((a) => a.id === rt.accountId)
  const title = (rt: RecurringDto) =>
    rt.description ||
    payees?.find((p) => p.id === rt.payeeId)?.name ||
    categories?.find((c) => c.id === rt.categoryId)?.name ||
    scheduleLabel(rt)

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      <h1 className="text-lg uppercase">{t('pages.settings.recurring.header')}</h1>
      <div className="flex-1 overflow-y-auto">
        {(recurring ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('pages.settings.recurring.empty')}</p>
        ) : (
          (recurring ?? []).map((rt) => (
            <div
              key={rt.id}
              data-testid={`recurring-${rt.id}`}
              className="flex cursor-pointer items-center gap-3 rounded-md p-2 hover:bg-accent"
              onClick={() => setSelected(rt)}
            >
              <EntityIcon
                name={rt.type === 'transfer' ? 'sync_alt' : (categories?.find((c) => c.id === rt.categoryId)?.icon ?? 'question_mark')}
                className="text-muted-foreground"
              />
              <div className="min-w-0 flex-1">
                <p className="truncate">{title(rt)}</p>
                <p className="text-sm text-muted-foreground">
                  {scheduleLabel(rt)} · {dayKey(rt.nextPaymentAt)}
                </p>
              </div>
              <p className="text-sm">
                {moneyFormat(rt.amount, accountOf(rt)?.currency, { useNativePrecision: false })}
              </p>
            </div>
          ))
        )}
      </div>
      <Button type="button" data-testid="recurring-create" disabled>
        {t('pages.settings.recurring.create')}
      </Button>
      {/* selected -> ViewRecurringDialog wired in Task 14; keep state so the row click is testable */}
      {selected ? <span data-testid="recurring-selected" className="hidden" /> : null}
    </div>
  )
}
```

Match the surrounding settings pages' header/back-navigation conventions (open `CategoriesPage.tsx` and mirror its page chrome — back button on compact layout etc.). Adjust the `EntityIcon` import to wherever `TransactionRow.tsx` imports it from.

- [ ] **Step 5: Run tests + lint**

Run: `cd web && pnpm vitest run src/features/recurring/ && pnpm lint`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/app web/src/features/settings/SettingsPage.tsx web/src/features/recurring web/src/locales/en-US.ts
git commit -m "feat(web/recurring): settings page, route, i18n strings"
```

---

### Task 13: Frontend — template dialog (create/edit) + uiStore

**Files:**
- Modify: `web/src/app/uiStore.ts` (add `recurringModal` state + `OpenRecurringParams`; extend `OpenTransactionParams` with `postRecurring`)
- Create: `web/src/features/recurring/useRecurringForm.ts`
- Create: `web/src/features/recurring/RecurringDialog.tsx`
- Modify: `web/src/app/layouts/ApplicationLayout.tsx` (mount `<RecurringDialog />` next to `<TransactionDialog />`)
- Modify: `web/src/features/recurring/RecurringSettingsPage.tsx` (enable the create button)
- Test: `web/src/features/recurring/RecurringDialog.test.tsx`, `web/src/features/recurring/useRecurringForm.test.ts`

**Interfaces:**
- Consumes: `useTransactionForm.ts` helpers — read that file first and reuse `evaluatedNumber`, the `seedAmount` pattern, `accountOptions`/`categoryOptions` (exported there); `ResponsiveDialog`; `EntitySelect`; `useCreateRecurring`/`useUpdateRecurring`.
- Produces:
  ```ts
  // uiStore.ts
  export interface OpenRecurringParams {
    recurring?: RecurringDto          // present => EDIT
    fromTransaction?: TransactionDto  // present => CREATE prefilled ("make recurring")
    accountId?: Id
  }
  // store fields: recurringModal: OpenRecurringParams | null
  //               openRecurringModal(params), closeRecurringModal()
  // OpenTransactionParams gains: postRecurring?: RecurringDto   (consumed in Task 15)
  ```
  `useRecurringForm(params: OpenRecurringParams, accounts: AccountDto[]): { form, patch, setType, account, accountRecipient }` with `RecurringFormState` = `{ id, isNew, type, accountId, accountRecipientId, amount: string, categoryId, payeeId, tagId, description, schedule: RecurringSchedule, nextPaymentAt: string }`, and `buildRecurringPayload(form): CreateRecurringDto`.

- [ ] **Step 1: Extend the uiStore**

Copy the `transactionModal` trio's exact shape (open sets state, close nulls it); add `postRecurring?: RecurringDto` to `OpenTransactionParams`.

- [ ] **Step 2: Write the failing form-hook test**

`web/src/features/recurring/useRecurringForm.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { RecurringDto } from '@/api/dto/recurring'
import type { TransactionDto } from '@/api/dto/transaction'
import { buildRecurringPayload, initialRecurringFormState } from './useRecurringForm'

const accounts = [{ id: 'a1', currency: { symbol: '$', fractionDigits: 2 } }] as never

it('create mode defaults: monthly, next payment = today, fresh id', () => {
  const s = initialRecurringFormState({}, accounts)
  expect(s.isNew).toBe(true)
  expect(s.schedule).toBe('monthly')
  expect(s.nextPaymentAt.length).toBe(19) // "YYYY-MM-DD HH:mm:ss"
  expect(s.id).toBeTruthy()
})

it('fromTransaction prefills fields but not the date', () => {
  const tx = { id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 42.5, amountRecipient: null, categoryId: 'c1', payeeId: null, tagId: null, description: 'rent', date: '2026-06-01 10:00:00' } as unknown as TransactionDto
  const s = initialRecurringFormState({ fromTransaction: tx }, accounts)
  expect(s.isNew).toBe(true)
  expect(s.amount).toBe('42.5')
  expect(s.categoryId).toBe('c1')
  expect(s.nextPaymentAt).not.toBe(tx.date)
})

it('edit mode seeds from the template and keeps its id', () => {
  const rt = { id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 50.5, categoryId: 'c1', payeeId: null, tagId: null, description: 'rent', schedule: 'weekly', nextPaymentAt: '2026-08-31 00:00:00' } as RecurringDto
  const s = initialRecurringFormState({ recurring: rt }, accounts)
  expect(s.isNew).toBe(false)
  expect(s.id).toBe('r1')
  expect(s.schedule).toBe('weekly')
  expect(s.nextPaymentAt).toBe('2026-08-31 00:00:00')
})

it('buildRecurringPayload evaluates the amount and nulls classifier ids for transfers', () => {
  const s = initialRecurringFormState({}, accounts)
  const payload = buildRecurringPayload({ ...s, type: 'transfer', accountId: 'a1', accountRecipientId: 'a2', amount: '10+5', categoryId: 'c1' })
  expect(payload.amount).toBe(15)
  expect(payload.categoryId).toBeNull()
  expect(payload.accountRecipientId).toBe('a2')
})
```

- [ ] **Step 3: Run to verify failure, then implement the hook**

Run: `cd web && pnpm vitest run src/features/recurring/useRecurringForm.test.ts` → FAIL.

`web/src/features/recurring/useRecurringForm.ts` (mirror `useTransactionForm.ts` closely — same seedAmount formatting, same evaluated-formula amounts):

```ts
import { useMemo, useState } from 'react'
import { v7 as uuidv7 } from 'uuid'
import type { AccountDto } from '@/api/dto/account'
import type { CreateRecurringDto, RecurringSchedule } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { TransactionType } from '@/api/dto/transaction'
import type { OpenRecurringParams } from '@/app/uiStore'
import { formatDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { evaluatedNumber } from '@/features/transactions/useTransactionForm'

export interface RecurringFormState {
  id: Id
  isNew: boolean
  type: TransactionType
  accountId: Id | null
  accountRecipientId: Id | null
  amount: string
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  description: string
  schedule: RecurringSchedule
  nextPaymentAt: string
}

const seedAmount = (v: number | null | undefined, account?: AccountDto): string =>
  v == null ? '' : moneyFormat(v, account?.currency, { showCurrency: false, useNativePrecision: false, useThousandSeparator: false })

export function initialRecurringFormState(params: OpenRecurringParams, accounts: AccountDto[]): RecurringFormState {
  const rt = params.recurring
  if (rt) {
    const account = accounts.find((a) => a.id === rt.accountId)
    return {
      id: rt.id, isNew: false, type: rt.type, accountId: rt.accountId,
      accountRecipientId: rt.accountRecipientId, amount: seedAmount(rt.amount, account),
      categoryId: rt.categoryId, payeeId: rt.payeeId, tagId: rt.tagId,
      description: rt.description, schedule: rt.schedule, nextPaymentAt: rt.nextPaymentAt,
    }
  }
  const tx = params.fromTransaction
  if (tx) {
    const account = accounts.find((a) => a.id === tx.accountId)
    return {
      id: uuidv7(), isNew: true, type: tx.type, accountId: tx.accountId,
      accountRecipientId: tx.accountRecipientId, amount: seedAmount(tx.amount, account),
      categoryId: tx.categoryId, payeeId: tx.payeeId, tagId: tx.tagId,
      description: tx.description, schedule: 'monthly', nextPaymentAt: formatDateTime(new Date()),
    }
  }
  return {
    id: uuidv7(), isNew: true, type: 'expense',
    accountId: params.accountId ?? accounts[0]?.id ?? null, accountRecipientId: null,
    amount: '', categoryId: null, payeeId: null, tagId: null, description: '',
    schedule: 'monthly', nextPaymentAt: formatDateTime(new Date()),
  }
}

export function buildRecurringPayload(form: RecurringFormState): CreateRecurringDto {
  const isTransfer = form.type === 'transfer'
  return {
    id: form.id,
    type: form.type,
    accountId: form.accountId as Id,
    accountRecipientId: isTransfer ? form.accountRecipientId : null,
    amount: evaluatedNumber(form.amount),
    categoryId: isTransfer ? null : form.categoryId,
    payeeId: isTransfer ? null : form.payeeId,
    tagId: isTransfer ? null : form.tagId,
    description: form.description || '',
    schedule: form.schedule,
    nextPaymentAt: form.nextPaymentAt,
  }
}

export function useRecurringForm(params: OpenRecurringParams, accounts: AccountDto[]) {
  const [form, setForm] = useState(() => initialRecurringFormState(params, accounts))
  const patch = (partial: Partial<RecurringFormState>) => setForm((prev) => ({ ...prev, ...partial }))
  const account = useMemo(() => accounts.find((a) => a.id === form.accountId), [accounts, form.accountId])
  const accountRecipient = useMemo(() => accounts.find((a) => a.id === form.accountRecipientId), [accounts, form.accountRecipientId])
  const setType = (type: TransactionType) => patch({ type, categoryId: null })
  return { form, patch, setType, account, accountRecipient }
}
```

(If `evaluatedNumber` is not exported from `useTransactionForm.ts`, export it there — one-line change — rather than duplicating the calculator wiring.)

- [ ] **Step 4: Write the dialog**

`web/src/features/recurring/RecurringDialog.tsx` — singleton driven by `useUiStore((s) => s.recurringModal)`, `null` when absent, exactly like `TransactionDialog`. Build the form body by copying `TransactionDialog.tsx`'s field sections and binding them to `useRecurringForm`'s state:
- type radiogroup (same `TYPE_ORDER`), account `EntitySelect`, recipient-account `EntitySelect` when transfer (NO recipient-amount input — computed at post time), amount input (calculator-enabled, same as TransactionDialog), category/payee/tag `EntitySelect`s for non-transfer, description input;
- two NEW fields: schedule (a select over the five values, labels from `t('modals.recurring.schedule.*')` — use the same select component TransactionDialog uses for comparable pickers, or a plain shadcn `Select`) and next-payment date (the same date-input component TransactionDialog uses for `date`, bound to `form.nextPaymentAt`);
- `ResponsiveDialog` chrome, form id `recurring-dialog-form`, title `t('modals.recurring.create_form.header')` / `update_form.header` by `form.isNew`;
- submit: validate `accountId` + non-empty amount + (transfer ⇒ recipient), then `form.isNew ? createRecurring.mutateAsync(payload) : updateRecurring.mutateAsync(payload)`, then `closeRecurringModal()`; dialog stays open on failure (try/catch, same as TransactionDialog).

Mount `<RecurringDialog />` in `ApplicationLayout.tsx` directly after `<TransactionDialog />`.

Enable the create button on `RecurringSettingsPage`: replace `disabled` with `onClick={() => openRecurringModal({})}` (import `useUiStore`).

- [ ] **Step 5: Write the dialog test**

`web/src/features/recurring/RecurringDialog.test.tsx` — copy the setup from `TransactionDialog.test.tsx` (`mockMatchMedia`, `coreHandlers`, memory router, `useUiStore.setState({ recurringModal: null, ... })` in `beforeEach`):

```tsx
it('creates a template', async () => {
  server.use(http.post('*/api/v1/recurring/create-recurring-transaction', async ({ request }) => {
    const body = (await request.json()) as Record<string, unknown>
    return HttpResponse.json({ success: true, message: '', data: { item: { ...body, ownerUserId: 'u1', amount: String(body.amount) } } })
  }))
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openRecurringModal({})
  await screen.findByRole('heading', { name: 'Add recurring transaction' })
  // fill amount + submit; assert store closes
  // (drive the same way TransactionDialog.test.tsx drives its create test)
})

it('edit mode shows the update header and prefills schedule', async () => {
  renderDialog()
  useUiStore.getState().openRecurringModal({ recurring: wireRecurringAsDto })
  await screen.findByRole('heading', { name: 'Edit recurring transaction' })
})
```

- [ ] **Step 6: Run tests + lint**

Run: `cd web && pnpm vitest run src/features/recurring/ && pnpm lint`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/app web/src/features/recurring
git commit -m "feat(web/recurring): template create/edit dialog + uiStore wiring"
```

---

### Task 14: Frontend — ViewRecurringDialog + settings-page integration

**Files:**
- Create: `web/src/features/recurring/ViewRecurringDialog.tsx`
- Modify: `web/src/features/recurring/RecurringSettingsPage.tsx` (wire the dialog + delete confirm)
- Test: `web/src/features/recurring/ViewRecurringDialog.test.tsx`

**Interfaces:**
- Consumes: `ResponsiveDialog`, `ConfirmDialog`, `CardField` + the hero layout — copy the structure of `web/src/features/transactions/ViewTransactionDialog.tsx` (read it fully first); `useSkipRecurring`/`useDeleteRecurring`; `openRecurringModal` for edit.
- Produces:
  ```tsx
  interface ViewRecurringDialogProps {
    recurring: RecurringDto
    onClose: () => void
    onPost?: () => void      // rendered as the primary action only when provided (account-list context, Task 15)
    onSkip: () => void
    onEdit: () => void
    onDelete: () => void
    canChange: boolean
    dismissible?: boolean
  }
  export function ViewRecurringDialog(props: ViewRecurringDialogProps)
  ```

- [ ] **Step 1: Write the failing tests**

```tsx
it('settings context: skip/edit/delete, no Post button', async () => {
  renderView({ onPost: undefined })
  expect(await screen.findByText('Recurring transaction')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Skip' })).toBeInTheDocument()
})

it('account context: Post is the primary action', async () => {
  const onPost = vi.fn()
  renderView({ onPost })
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Post' }))
  expect(onPost).toHaveBeenCalled()
})

it('disables mutating actions when canChange is false', async () => {
  renderView({ canChange: false })
  expect(await screen.findByRole('button', { name: 'Skip' })).toBeDisabled()
})
```

(`renderView` = QueryClient wrapper + `coreHandlers()` + the dialog with a `wireRecurring`-derived `RecurringDto` and vi.fn() callbacks.)

- [ ] **Step 2: Implement the dialog**

Copy `ViewTransactionDialog.tsx`'s skeleton: hero (category/transfer icon via `EntityIcon`, big `moneyFormat(recurring.amount, account?.currency, { useNativePrecision: false })`, subtitle `scheduleLabel · dayKey(nextPaymentAt)`), `CardField` rows (account, recipient for transfers, category, payee, tag, description, `t('modals.recurring.preview.schedule')`, `t('modals.recurring.preview.next_payment')`), resolving names via `useAccounts()`/`useCategories()`/`usePayees()`/`useTags()` internally. Footer:

```tsx
footer={
  <div className="flex gap-3 [&_button]:h-11">
    <Button variant="secondary" size="icon" className="size-11" onClick={onClose}>
      <ChevronDown className="size-4" />
    </Button>
    {onPost ? (
      <Button className="flex-1" disabled={!canChange} onClick={onPost}>
        {t('modals.recurring.preview.post')}
      </Button>
    ) : null}
    <Button variant="secondary" className={onPost ? '' : 'flex-1'} disabled={!canChange} onClick={onSkip}>
      {t('modals.recurring.preview.skip')}
    </Button>
    <Button variant="secondary" size="icon" className="size-11" disabled={!canChange} onClick={onEdit}>
      <Pencil className="size-4" />
    </Button>
    <Button variant="destructive" size="icon" className="size-11" disabled={!canChange} onClick={onDelete}>
      <Trash2 className="size-4" />
    </Button>
  </div>
}
```

- [ ] **Step 3: Wire it into `RecurringSettingsPage`**

Replace the Task-12 placeholder: when `selected`, render `ViewRecurringDialog` with `onPost` OMITTED, `onSkip={() => skipRecurring.mutate(selected.id)}`, `onEdit={() => { setSelected(null); openRecurringModal({ recurring: selected }) }}`, `onDelete` opening a `ConfirmDialog` (question `t('pages.settings.recurring.delete_question')`) that calls `deleteRecurring.mutate(selected.id)`. `canChange`: owner or admin/user role on the template's account — compute from `accounts` exactly like `AccountPage.tsx:88-91` does (`account.owner.id === user?.id || role === 'admin' || role === 'user'`, with `useUser()` for the current user).

- [ ] **Step 4: Run tests + lint**

Run: `cd web && pnpm vitest run src/features/recurring/ && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/recurring
git commit -m "feat(web/recurring): view dialog + settings page actions"
```

---

### Task 15: Frontend — virtual rows in the account list + posting flow

**Files:**
- Modify: `web/src/features/transactions/useAccountTransactions.ts` (merge virtual rows)
- Modify: `web/src/features/transactions/TransactionRow.tsx` (recurring badge/dimming)
- Modify: `web/src/features/accounts/AccountPage.tsx` (recurring preview + actions)
- Modify: `web/src/features/transactions/useTransactionForm.ts` + `TransactionDialog.tsx` (postRecurring mode)
- Test: `web/src/features/transactions/useAccountTransactions.test.tsx` (append cases), `web/src/features/transactions/TransactionDialog.test.tsx` (append a post case)

**Interfaces:**
- Consumes: `useRecurring`, `usePostRecurring`, `openTransactionModal({ postRecurring })` (param added in Task 13), `ViewRecurringDialog`.
- Produces: `ViewTransaction` gains `recurring?: RecurringDto` and its `author` becomes optional (`extends Omit<TransactionDto, 'author'> { author?: UserDto; ... }` — `TransactionRow`/`haystack` already use `tx.author?.name`; grep for other non-optional `author` usages of `ViewTransaction` and add `?.`).

- [ ] **Step 1: Write the failing merge tests** (append to the existing `useAccountTransactions` test file, following its established render/wrapper pattern; add an MSW handler for the recurring list)

```tsx
it('merges one virtual row per template at its next payment date', async () => {
  // recurring handler: template on account a1, nextPaymentAt in the future
  // assert: entries contain a transaction entry whose transaction.recurring is set,
  // positioned under the day separator for dayKey(nextPaymentAt), sorted date-desc with real rows
})

it('virtual transfer rows appear only on the source account', async () => {
  // template type transfer accountId a1, accountRecipientId a2:
  // hook for a2 must NOT contain the virtual row; hook for a1 must
})

it('overdue templates surface at their past date', async () => {
  // nextPaymentAt yesterday -> virtual row appears under yesterday's separator
})
```

- [ ] **Step 2: Implement the merge**

In `useAccountTransactions.ts`:
1. Add `const { data: recurring } = useRecurring()` (import from `@/features/recurring/queries`).
2. Extend `ViewTransaction` as described in Interfaces.
3. After building `enriched`, build the virtual rows and merge BEFORE the search filter (so search covers them too):

```ts
const virtual: ViewTransaction[] = (recurring ?? [])
  .filter((rt) => rt.accountId === accountId) // transfers: source account only
  .map((rt) => ({
    id: rt.id,
    type: rt.type,
    accountId: rt.accountId,
    accountRecipientId: rt.accountRecipientId,
    amount: rt.amount,
    amountRecipient: null,
    categoryId: rt.categoryId,
    payeeId: rt.payeeId,
    tagId: rt.tagId,
    description: rt.description,
    date: rt.nextPaymentAt,
    account: accounts?.find((a) => a.id === rt.accountId),
    accountRecipient: rt.accountRecipientId ? accounts?.find((a) => a.id === rt.accountRecipientId) : undefined,
    category: rt.categoryId ? categories?.find((c) => c.id === rt.categoryId) : undefined,
    payee: rt.payeeId ? payees?.find((p) => p.id === rt.payeeId) : undefined,
    tag: rt.tagId ? tags?.find((tg) => tg.id === rt.tagId) : undefined,
    isInFuture: isFuture(rt.nextPaymentAt),
    recurring: rt,
  }))
const merged = [...enriched, ...virtual].sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : 0))
```

and run the existing search-filter + day-grouping over `merged` instead of `enriched`. Add `recurring` to the `useMemo` dependency array.

- [ ] **Step 3: Row styling**

In `TransactionRow.tsx`: dim virtual rows like future ones (extend the existing `isInFuture` opacity condition with `|| !!tx.recurring`) and render a small repeat glyph next to the title when `tx.recurring` is set (`<Repeat className="size-3 inline text-muted-foreground" />` from `lucide-react`, matching how the row lays out its title line).

- [ ] **Step 4: AccountPage wiring**

- Add `const [recurringPreview, setRecurringPreview] = useState<ViewTransaction | null>(null)`.
- Row click: `onClick={() => (entry.transaction.recurring ? setRecurringPreview(entry.transaction) : setPreview(entry.transaction))}`.
- Desktop kebab: for virtual rows, show Edit (opens `openRecurringModal({ recurring: tx.recurring })`) and Delete (recurring delete confirm) instead of the transaction actions.
- Render, when `recurringPreview?.recurring`:

```tsx
<ViewRecurringDialog
  recurring={recurringPreview.recurring}
  onClose={() => setRecurringPreview(null)}
  onPost={() => {
    openTransactionModal({ postRecurring: recurringPreview.recurring })
    setRecurringPreview(null)
  }}
  onSkip={() => skipRecurring.mutate(recurringPreview.recurring!.id, { onSettled: () => setRecurringPreview(null) })}
  onEdit={() => {
    setRecurringPreview(null)
    openRecurringModal({ recurring: recurringPreview.recurring })
  }}
  onDelete={() => {
    setRecurringDeleteTarget(recurringPreview.recurring)
    setRecurringPreview(null)
  }}
  canChange={canChangeTransaction}
/>
```

plus a second `ConfirmDialog` for recurring deletion mirroring the existing transaction one.

- [ ] **Step 5: Posting mode in the transaction dialog**

`useTransactionForm.ts` — in `initialFormState`, FIRST branch (before the edit branch):

```ts
const rt = params.postRecurring
if (rt) {
  const account = accounts.find((a) => a.id === rt.accountId)
  return {
    id: uuidv7(), isNew: true, type: rt.type, accountId: rt.accountId,
    accountRecipientId: rt.accountRecipientId,
    amount: seedAmount(rt.amount, account), amountRecipient: '',
    categoryId: rt.categoryId, payeeId: rt.payeeId, tagId: rt.tagId,
    description: rt.description, date: rt.nextPaymentAt,
  }
}
```

`TransactionDialog.tsx`:
- title: when `params.postRecurring`, use `t('modals.recurring.post_form.header')`;
- cross-currency prefill: on mount, if `params.postRecurring` and the transfer is cross-currency and `form.amountRecipient === ''`, `patch({ amountRecipient: recomputeRecipientAmount(form.amount, account, accountRecipient, exchange) })` (one-shot `useEffect` with empty deps);
- submit: when `params.postRecurring`, call `postRecurring.mutateAsync({ ...buildPayload(form), recurringId: params.postRecurring.id })` instead of create/update, then `onDone()`.

- [ ] **Step 6: Append a dialog test**

In `TransactionDialog.test.tsx`, add: open via `useUiStore.getState().openTransactionModal({ postRecurring: wireRecurringDto })`, assert the heading is `Post recurring transaction` and the date input holds the template's `nextPaymentAt`; mock `*/api/v1/recurring/post-recurring-transaction` and assert submit hits it (not create-transaction).

- [ ] **Step 7: Run the full web suite + lint**

Run: `cd web && pnpm test && pnpm lint`
Expected: PASS (except the pre-existing `ImportCsvDialog.test.tsx` failure).

- [ ] **Step 8: Commit**

```bash
git add web/src/features/transactions web/src/features/accounts web/src/features/recurring
git commit -m "feat(web/recurring): virtual rows in account list + posting flow"
```

---

### Task 16: Frontend — "make recurring" from an existing transaction

**Files:**
- Modify: `web/src/features/transactions/ViewTransactionDialog.tsx` (optional `onMakeRecurring` prop + footer button)
- Modify: `web/src/features/accounts/AccountPage.tsx` (pass the callback)
- Test: append to `web/src/features/accounts/AccountPage.test.tsx` if it exists, else to `ViewTransactionDialog`'s test file (create one following the Task 14 dialog-test pattern if absent)

**Interfaces:**
- Produces: `ViewTransactionDialogProps` gains `onMakeRecurring?: () => void`; when set, the footer renders a `Repeat` icon button (secondary, `size-11`, between Edit and Delete, `aria-label={t('modals.recurring.make_recurring')}`, disabled with `!canChange`).

- [ ] **Step 1: Failing test** — render `ViewTransactionDialog` with `onMakeRecurring={vi.fn()}`, click the button by aria-label `Make recurring`, assert the callback fired; render without the prop and assert the button is absent.

- [ ] **Step 2: Implement** — add the prop + button to the footer flex row in `ViewTransactionDialog.tsx`:

```tsx
{onMakeRecurring ? (
  <Button variant="secondary" size="icon" className="size-11" disabled={!canChange}
    aria-label={t('modals.recurring.make_recurring')} title={t('modals.recurring.make_recurring')}
    onClick={onMakeRecurring}>
    <Repeat className="size-4" />
  </Button>
) : null}
```

In `AccountPage.tsx`, pass:

```tsx
onMakeRecurring={() => {
  setPreview(null)
  openRecurringModal({ fromTransaction: preview })
}}
```

- [ ] **Step 3: Run tests + lint**

Run: `cd web && pnpm test && pnpm lint`
Expected: PASS (modulo the known pre-existing failure).

- [ ] **Step 4: Commit**

```bash
git add web/src/features/transactions web/src/features/accounts
git commit -m "feat(web/recurring): make-recurring action on transaction preview"
```

---

### Task 17: Full verification sweep

**Files:** none new — verification and fixes only.

- [ ] **Step 1: Backend full tier**

Run: `make go-test`
Expected: PASS (build, vet, gofmt, swagger-fresh, sqlite tests, apiparity + goldens, coverage ≥ 72%).

- [ ] **Step 2: Engine comparison + pgsql repo tier**

Run: `make test` (auto-provisions PostgreSQL via compose, runs `enginecompare` + `test-repo-pgsql` + web suite)
Expected: PASS — the recurring goldens must be byte-identical on both engines. If pgsql cannot be provisioned in this environment, run `make go-test` + `cd web && pnpm test` and flag the gap explicitly in the final report.

- [ ] **Step 3: Frontend suite + lint + build**

Run: `cd web && pnpm test && pnpm lint && pnpm build`
Expected: PASS (modulo pre-existing `ImportCsvDialog.test.tsx`), lint clean, production build succeeds.

- [ ] **Step 4: End-to-end sanity (manual, via the running app)**

Run: `make go-run` with the built SPA (or `make web-run` proxying to `:8181`), then walk: Settings → Recurring transactions → create a monthly template on an account → see the virtual row in that account's list at the next-payment date → tap → preview → Post → confirm the real transaction appears and the virtual row moves one month forward → Skip once → date advances again → edit schedule to weekly → delete the template. Use the `verify` skill if available in the executing session.

- [ ] **Step 5: Commit any fixes; final review**

```bash
git status   # must be clean or only intentional fixes
```

Then invoke superpowers:requesting-code-review / finishing-a-development-branch as the session dictates (the working PR is #91 on branch feat/recurring-transactions).

---

## Self-Review Notes (already applied)

- Spec coverage: schedule presets (T1), one-virtual-row projection + overdue (T15), posting advances from scheduled date atomically (T8), skip (T8/T14/T15), both creation entry points (T13/T16), transfers incl. compute-at-post-time recipient amount (T8/T15 prefill effect), shared-account matrix (T5–T8 `checkWriteAccess`, T14 `canChange`), delete-only lifecycle (T7), settings Finance entry + preview-first UX (T12/T14), no wire-contract changes (T10 asserts existing goldens untouched).
- Deliberate deviation from spec's "FK semantics copy transactions exactly": `account_recipient_id` cascades instead of SET NULL (a recipient-less transfer template is unpostable garbage). Documented in Task 3.
- Type-consistency: `RecurringDto`/`CreateRecurringDto` names, `useRecurring`/`usePostRecurring`, `ViewRecurringDialog` props, `OpenRecurringParams`, and the Go `Service` method names are used identically across tasks — when implementing, treat the Interfaces blocks as the source of truth.
- Known look-up-before-coding points (called out inline): sqlc-generated field names (T4), `errs.AsValidation` signature (T2), fixture builder field names (T4), harness local variable names (T5), `evaluatedNumber` export (T13), `EntityIcon` import path (T12).

