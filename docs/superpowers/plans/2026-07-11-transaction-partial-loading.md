# Transaction Partial Loading & Server-Side Classification Sorting — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the unbounded boot fetch of all transactions with per-account windows (50 newest per visible account, keyset-cursor chunks on scroll), and add server-side `sort-{category,payee,tag}-list` endpoints (by name or by usage over a 1–6 month window).

**Architecture:** The backend extends `GET /api/v1/transaction/get-transaction-list` additively with a boot mode (`perAccountLimit`) and a keyset page mode (`accountId+limit[+cursor]`); legacy responses stay byte-identical. The frontend keeps the single flat `['transactions']` cache (union of loaded windows) plus a `['transactionPages']` per-account state map, with a horizon rule hiding stray transfer rows. Search fetches the account's full list on demand; the budget dialog fetches its month window. Sorting moves to three new POST endpoints that renumber positions server-side.

**Tech Stack:** Go (stdlib HTTP, hand-built SQL over database/sql — no sqlc regeneration needed), React 19 + TanStack Query v5, vitest.

**Spec:** `docs/superpowers/specs/2026-07-11-transaction-partial-loading-design.md`

**Deviation from spec (approved rationale):** the spec suggested sqlc-generated page queries. The keyset predicate needs a parameter reused across engines with different placeholder rules (`?` vs `$N`), which would make the two engines' generated param structs diverge and break the whole-struct adapter shim. Both new queries are therefore hand-built in the repo with the existing `placeholders()` helper — the same precedent as `ListByAccountIDs`.

## Global Constraints

- Work on branch `feat/transaction-partial-loading` in the current worktree (`.claude/worktrees/cached-whistling-riddle`). Run all commands from the worktree root.
- **Frozen wire contract:** legacy `get-transaction-list` responses (no new params) must stay byte-identical. Existing goldens in `internal/test/apiparity/testdata/golden/` must NOT change; only NEW golden files may appear. Regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, then inspect `git diff --stat internal/test/apiparity/testdata/` — any modified (not added) file is a bug. Never hand-edit a golden.
- New validation messages introduced here become frozen once shipped — copy them exactly as written in the tasks.
- After changing any swag `// @…` annotation, run `make swagger` and commit the regenerated docs (the `make go-test` docs-fresh check fails otherwise).
- `make go-test` must pass at the end of every backend task (build + vet + gofmt + docs-fresh + tests + coverage gate ≥72%).
- Comments: sparse, only non-obvious rationale; never reference the former PHP implementation. If you touch any `.sql` file under `internal/infra/storage/sqlc/query/`, use ASCII-only comment text (an em dash there corrupts sqlc codegen) — this plan does not require sqlc changes.
- Frontend: run `cd web && pnpm test` / `pnpm lint` using the existing `node_modules` (do NOT run `make web-install`; it is broken under pnpm 11). Known pre-existing failure on main: `ImportCsvDialog.test.tsx` — ignore it, do not try to fix it.
- Datetime strings on the wire are always `"2006-01-02 15:04:05"` (`datetime.Layout`). `periodStart`/`periodEnd` validation rejects date-only strings — the frontend must send full datetimes.
- Commit at the end of every task with the message given in the task.

---

## Part A — Backend: paged transaction list

### Task 1: Page cursor codec

**Files:**
- Create: `internal/transaction/cursor.go`
- Test: `internal/transaction/cursor_test.go`

**Interfaces:**
- Consumes: `datetime.Layout`, `errs.NewValidation`, `vo.ParseId` (all existing).
- Produces: `type PageCursor struct { SpentAt time.Time; ID vo.Id }`, `func EncodeCursor(c PageCursor) string`, `func decodeCursor(raw string) (PageCursor, error)`. `EncodeCursor` is exported because the apiparity catalogue (Task 5) builds cursors from fixture constants.

- [ ] **Step 1: Write the failing test**

`internal/transaction/cursor_test.go`:

```go
package transaction

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestCursorRoundTrip(t *testing.T) {
	in := PageCursor{
		SpentAt: time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC),
		ID:      vo.MustParseId("d0000000-0000-0000-0000-000000000001"),
	}
	raw := EncodeCursor(in)
	out, err := decodeCursor(raw)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !out.SpentAt.Equal(in.SpentAt) || !out.ID.Equal(in.ID) {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	for _, raw := range []string{
		"%%%not-base64",
		"aGVsbG8",              // decodes but has no separator
		"MjAyNi0wMy0wNXxub3Bl", // "2026-03-05|nope": bad datetime AND bad id
	} {
		_, err := decodeCursor(raw)
		if err == nil {
			t.Fatalf("decodeCursor(%q): want error", raw)
		}
		var verr *errs.ValidationError
		if !errs.As(err, &verr) {
			t.Fatalf("decodeCursor(%q): err type %T, want *errs.ValidationError", raw, err)
		}
	}
}
```

Note: check how existing tests assert a validation error type — grep `errs.ValidationError` in `internal/`; if the taxonomy uses a different assertion helper (e.g. `errors.As` with a concrete exported type), mirror that exact pattern instead of `errs.As`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transaction/ -run TestCursor -v`
Expected: FAIL — `undefined: PageCursor`, `undefined: EncodeCursor`.

- [ ] **Step 3: Write the implementation**

`internal/transaction/cursor.go`:

```go
package transaction

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// PageCursor is a position in the (spent_at DESC, id ASC) transaction order:
// the row it points at was the last one already returned.
type PageCursor struct {
	SpentAt time.Time
	ID      vo.Id
}

// EncodeCursor serializes a cursor as base64url("spent_at|id"). Exported so the
// api-parity catalogue can build deterministic cursors from fixture constants.
func EncodeCursor(c PageCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.SpentAt.Format(datetime.Layout) + "|" + c.ID.String()))
}

func invalidCursor() error {
	return errs.NewValidation("Form validation error",
		errs.FieldError{Key: "cursor", Message: "This value is not a valid cursor."})
}

func decodeCursor(raw string) (PageCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return PageCursor{}, invalidCursor()
	}
	at, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return PageCursor{}, invalidCursor()
	}
	spentAt, err := time.Parse(datetime.Layout, at)
	if err != nil {
		return PageCursor{}, invalidCursor()
	}
	parsed, err := vo.ParseId(id)
	if err != nil {
		return PageCursor{}, invalidCursor()
	}
	return PageCursor{SpentAt: spentAt, ID: parsed}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/transaction/ -run TestCursor -v`
Expected: PASS (adjust the error-type assertion per the Step 1 note if needed).

- [ ] **Step 5: Commit**

```bash
git add internal/transaction/cursor.go internal/transaction/cursor_test.go
git commit -m "feat(transaction): keyset page cursor codec"
```

### Task 2: List request/response DTO extensions

**Files:**
- Modify: `internal/model/transaction_dto.go` (TransactionListRequest, GetTransactionListResult)
- Test: `internal/model/transaction_dto_test.go` (create if absent; if a DTO test file already exists for this package, add to it)

**Interfaces:**
- Produces (request): `TransactionListRequest` gains string fields `Limit`, `Cursor`, `PerAccountLimit`; helper methods `LimitValue() int` and `PerAccountLimitValue() int` (post-Validate, errors ignored).
- Produces (response): `TransactionPageResult{NextCursor *string "json:nextCursor"; HasMore bool "json:hasMore"}`, `TransactionAccountPageResult{Id string "json:id"; NextCursor *string "json:nextCursor"; HasMore bool "json:hasMore"}`; `GetTransactionListResult` gains `Page *TransactionPageResult "json:page,omitempty"` and `Accounts []TransactionAccountPageResult "json:accounts,omitempty"`.
- Frozen validation messages (envelope message stays `"Form validation error"`):
  - `limit` / `perAccountLimit` not an integer in [1,500] → key = param name, `"This value should be an integer between 1 and 500."`
  - `perAccountLimit` combined with any other param → key `perAccountLimit`, `"perAccountLimit cannot be combined with other parameters."`
  - `limit` without `accountId` → key `limit`, `"limit requires accountId."`
  - `limit` with `periodStart`/`periodEnd` → key `limit`, `"limit cannot be combined with periodStart or periodEnd."`
  - `cursor` without `limit` → key `cursor`, `"cursor requires limit."`

- [ ] **Step 1: Write the failing test**

`internal/model/transaction_dto_test.go`:

```go
package model

import (
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

func fieldMessage(t *testing.T, err error, key string) string {
	t.Helper()
	if err == nil {
		t.Fatal("want validation error, got nil")
	}
	verr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("err type %T, want *errs.ValidationError", err)
	}
	for _, f := range verr.Fields {
		if f.Key == key {
			return f.Message
		}
	}
	t.Fatalf("no field error for %q in %v", key, verr.Fields)
	return ""
}

func TestTransactionListRequest_Validate_Paging(t *testing.T) {
	const acct = "a0000000-0000-0000-0000-000000000001"

	cases := []struct {
		name    string
		req     TransactionListRequest
		key     string
		wantMsg string
	}{
		{"limit not a number", TransactionListRequest{AccountId: acct, Limit: "abc"},
			"limit", "This value should be an integer between 1 and 500."},
		{"limit zero", TransactionListRequest{AccountId: acct, Limit: "0"},
			"limit", "This value should be an integer between 1 and 500."},
		{"limit too big", TransactionListRequest{AccountId: acct, Limit: "501"},
			"limit", "This value should be an integer between 1 and 500."},
		{"limit without accountId", TransactionListRequest{Limit: "50"},
			"limit", "limit requires accountId."},
		{"limit with period", TransactionListRequest{AccountId: acct, Limit: "50",
			PeriodStart: "2026-01-01 00:00:00", PeriodEnd: "2026-02-01 00:00:00"},
			"limit", "limit cannot be combined with periodStart or periodEnd."},
		{"cursor without limit", TransactionListRequest{AccountId: acct, Cursor: "abc"},
			"cursor", "cursor requires limit."},
		{"perAccountLimit bad", TransactionListRequest{PerAccountLimit: "-1"},
			"perAccountLimit", "This value should be an integer between 1 and 500."},
		{"perAccountLimit combined", TransactionListRequest{PerAccountLimit: "50", AccountId: acct},
			"perAccountLimit", "perAccountLimit cannot be combined with other parameters."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldMessage(t, tc.req.Validate(), tc.key); got != tc.wantMsg {
				t.Errorf("message = %q, want %q", got, tc.wantMsg)
			}
		})
	}

	valid := []TransactionListRequest{
		{},
		{AccountId: acct},
		{AccountId: acct, Limit: "50"},
		{AccountId: acct, Limit: "1", Cursor: "whatever"}, // cursor CONTENT is checked in the use case
		{PerAccountLimit: "500"},
	}
	for _, req := range valid {
		if err := req.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", req, err)
		}
	}
}
```

Note: check the actual shape of `*errs.ValidationError` (grep `type ValidationError` in `internal/shared/errs/`) — if its fields accessor differs (e.g. a method instead of a `.Fields` slice), adapt `fieldMessage` to the real API. The asserted messages/keys must stay exactly as written.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestTransactionListRequest_Validate_Paging -v`
Expected: FAIL — `unknown field Limit` (compile error).

- [ ] **Step 3: Write the implementation**

In `internal/model/transaction_dto.go`, add `strconv` to imports. Replace the `TransactionListRequest` struct and `Validate`, and the result struct, with:

```go
// TransactionListRequest is the get-transaction-list query (all optional).
// Modes: accountId alone (full per-account list), periodStart+periodEnd
// (window across visible accounts), accountId+limit[+cursor] (keyset page),
// perAccountLimit (newest N per visible account), or nothing (all visible).
type TransactionListRequest struct {
	AccountId       string `json:"accountId"`
	PeriodStart     string `json:"periodStart"`
	PeriodEnd       string `json:"periodEnd"`
	Limit           string `json:"limit"`
	Cursor          string `json:"cursor"`
	PerAccountLimit string `json:"perAccountLimit"`
}

// boundedInt reports whether v parses as an integer in [1, 500].
func boundedInt(v string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	return err == nil && n >= 1 && n <= 500
}

// LimitValue returns the parsed limit; call only after Validate succeeded.
func (r TransactionListRequest) LimitValue() int {
	n, _ := strconv.Atoi(strings.TrimSpace(r.Limit))
	return n
}

// PerAccountLimitValue returns the parsed perAccountLimit; call only after
// Validate succeeded.
func (r TransactionListRequest) PerAccountLimitValue() int {
	n, _ := strconv.Atoi(strings.TrimSpace(r.PerAccountLimit))
	return n
}

// Validate: every field is optional, but when present accountId must be a UUID,
// periodStart/periodEnd must match the strict "Y-m-d H:i:s" datetime format,
// limit/perAccountLimit must be integers in [1,500], and the paging params must
// form a consistent mode (see the struct doc). The exact messages and field
// grouping are wire-frozen.
func (r TransactionListRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.AccountId) != "" {
		if _, err := vo.ParseId(r.AccountId); err != nil {
			fields = append(fields, errs.FieldError{Key: "accountId", Message: "This value is not a valid UUID."})
		}
	}
	for _, f := range []struct{ key, val string }{
		{"periodStart", r.PeriodStart},
		{"periodEnd", r.PeriodEnd},
	} {
		if strings.TrimSpace(f.val) == "" {
			continue
		}
		if _, err := time.Parse(datetime.Layout, f.val); err != nil {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is not a valid datetime."})
		}
	}
	if r.PerAccountLimit != "" {
		if !boundedInt(r.PerAccountLimit) {
			fields = append(fields, errs.FieldError{Key: "perAccountLimit", Message: "This value should be an integer between 1 and 500."})
		}
		if r.AccountId != "" || r.Limit != "" || r.Cursor != "" || r.PeriodStart != "" || r.PeriodEnd != "" {
			fields = append(fields, errs.FieldError{Key: "perAccountLimit", Message: "perAccountLimit cannot be combined with other parameters."})
		}
	}
	if r.Limit != "" {
		if !boundedInt(r.Limit) {
			fields = append(fields, errs.FieldError{Key: "limit", Message: "This value should be an integer between 1 and 500."})
		}
		if r.AccountId == "" {
			fields = append(fields, errs.FieldError{Key: "limit", Message: "limit requires accountId."})
		}
		if r.PeriodStart != "" || r.PeriodEnd != "" {
			fields = append(fields, errs.FieldError{Key: "limit", Message: "limit cannot be combined with periodStart or periodEnd."})
		}
	}
	if r.Cursor != "" && r.Limit == "" {
		fields = append(fields, errs.FieldError{Key: "cursor", Message: "cursor requires limit."})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Form validation error", fields...)
	}
	return nil
}

// TransactionPageResult is the page-mode pagination block.
type TransactionPageResult struct {
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// TransactionAccountPageResult is one account's pagination state in boot mode.
type TransactionAccountPageResult struct {
	Id         string  `json:"id"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// GetTransactionListResult is the response: {items: [...]}. page appears only
// in page mode, accounts only in boot mode; both omitted otherwise so legacy
// responses stay byte-identical.
type GetTransactionListResult struct {
	Items    []TransactionResult            `json:"items"`
	Page     *TransactionPageResult         `json:"page,omitempty"`
	Accounts []TransactionAccountPageResult `json:"accounts,omitempty"`
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/model/ ./internal/transaction/... -v -run 'TestTransactionListRequest|TestGetTransactionList'`
Expected: PASS, including the pre-existing endpoint validation tests (legacy behavior untouched).

- [ ] **Step 5: Commit**

```bash
git add internal/model/transaction_dto.go internal/model/transaction_dto_test.go
git commit -m "feat(transaction): paging params + page/accounts response blocks in list DTO"
```

### Task 3: Repository — keyset page and per-account recent windows

**Files:**
- Modify: `internal/transaction/repository.go` (interface), `internal/transaction/repo/repo.go` (implementation)
- Test: `internal/transaction/repo/paging_integration_test.go` (create)

**Interfaces:**
- Produces:
  - `Repository.ListPageByAccount(ctx context.Context, accountID vo.Id, after *PageCursor, limit int) ([]*model.Transaction, error)` — rows where the account is source or recipient, strictly older than `after` (nil = from the top), `ORDER BY spent_at DESC, id`, at most `limit` rows. Callers pass `limit+1` to detect hasMore.
  - `Repository.ListRecentByAccountIDs(ctx context.Context, accountIDs []vo.Id, perAccountLimit int) (map[string][]*model.Transaction, error)` — keyed by account id string; each value is that account's newest rows (source or recipient) in `(spent_at DESC, id)` order, at most `perAccountLimit` each. A transfer between two requested accounts appears in BOTH values. Callers pass `perAccountLimit+1` to detect per-account hasMore.

- [ ] **Step 1: Write the failing test**

`internal/transaction/repo/paging_integration_test.go`:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/transaction"
	txrepo "github.com/econumo/econumo/internal/transaction/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	pUSD   = "dffc2a06-6f29-4704-8575-31709adee926"
	pUser  = "11111111-1111-1111-1111-111111111111"
	pAcctA = "a0000000-0000-0000-0000-0000000000a1"
	pAcctB = "a0000000-0000-0000-0000-0000000000b1"
)

func pagingSetup(t *testing.T) (*txrepo.Repo, *dbtest.DB) {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: pUser, Name: "u"})
	f.Account(fixture.Account{ID: pAcctA, UserID: pUser, CurrencyID: pUSD, Name: "A"})
	f.Account(fixture.Account{ID: pAcctB, UserID: pUser, CurrencyID: pUSD, Name: "B"})
	return txrepo.NewRepo(db.Engine, db.TX), db
}

func seedTx(t *testing.T, db *dbtest.DB, id, account string, recipient *string, spentAt time.Time) {
	t.Helper()
	_, err := db.Raw.Exec(db.Rebind(`INSERT INTO transactions
		(id, user_id, account_id, account_recipient_id, type, amount, amount_recipient, description, spent_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		id, pUser, account, recipient, txType(recipient), "5.00000000", recipAmount(recipient), "t", spentAt, spentAt, spentAt)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func txType(recipient *string) int {
	if recipient != nil {
		return 2 // transfer
	}
	return 0 // expense
}

func recipAmount(recipient *string) *string {
	if recipient != nil {
		a := "5.00000000"
		return &a
	}
	return nil
}

func day(d int) time.Time { return time.Date(2026, 6, d, 12, 0, 0, 0, time.UTC) }

// ids chosen so the same-timestamp tie-break (id ASC) is observable
func TestListPageByAccount_Keyset(t *testing.T) {
	repo, db := pagingSetup(t)
	ctx := context.Background()
	seedTx(t, db, "d0000000-0000-0000-0000-000000000001", pAcctA, nil, day(3)) // newest, tie
	seedTx(t, db, "d0000000-0000-0000-0000-000000000002", pAcctA, nil, day(3)) // tie, larger id -> second
	seedTx(t, db, "d0000000-0000-0000-0000-000000000003", pAcctA, nil, day(2))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000004", pAcctA, nil, day(1))
	acct := vo.MustParseId(pAcctA)

	first, err := repo.ListPageByAccount(ctx, acct, nil, 3)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	wantIDs(t, first, "…001", "…002", "…003")

	after := &domtransaction.PageCursor{SpentAt: first[1].SpentAt, ID: first[1].ID} // cursor at …002
	second, err := repo.ListPageByAccount(ctx, acct, after, 3)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	wantIDs(t, second, "…003", "…004")
}

func TestListRecentByAccountIDs_PartitionsAndTransfers(t *testing.T) {
	repo, db := pagingSetup(t)
	ctx := context.Background()
	b := pAcctB
	// transfer A->B: must appear in BOTH windows
	seedTx(t, db, "d0000000-0000-0000-0000-000000000011", pAcctA, &b, day(5))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000012", pAcctA, nil, day(4))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000013", pAcctA, nil, day(3))
	seedTx(t, db, "d0000000-0000-0000-0000-000000000014", pAcctB, nil, day(2))

	windows, err := repo.ListRecentByAccountIDs(ctx,
		[]vo.Id{vo.MustParseId(pAcctA), vo.MustParseId(pAcctB)}, 2)
	if err != nil {
		t.Fatalf("ListRecentByAccountIDs: %v", err)
	}
	wantIDs(t, windows[pAcctA], "…011", "…012") // newest 2 of A's 3
	wantIDs(t, windows[pAcctB], "…011", "…014") // transfer counts for B too
}

func TestListRecentByAccountIDs_Empty(t *testing.T) {
	repo, _ := pagingSetup(t)
	windows, err := repo.ListRecentByAccountIDs(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("empty ids: %v", err)
	}
	if len(windows) != 0 {
		t.Fatalf("windows = %v, want empty", windows)
	}
}

// wantIDs asserts the ordered id suffixes ("…0NN" means suffix "0NN") match.
func wantIDs(t *testing.T, rows []*model.Transaction, suffixes ...string) {
	t.Helper()
	if len(rows) != len(suffixes) {
		t.Fatalf("got %d rows, want %d", len(rows), len(suffixes))
	}
	for i, want := range suffixes {
		got := rows[i].ID.String()
		if got[len(got)-3:] != want[len(want)-3:] {
			t.Errorf("row %d id = %s, want suffix %s", i, got, want)
		}
	}
}
```

Add the missing import `"github.com/econumo/econumo/internal/model"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transaction/repo/ -run 'TestListPage|TestListRecent' -v`
Expected: FAIL — `repo.ListPageByAccount undefined` (compile error).

- [ ] **Step 3: Add the interface methods**

In `internal/transaction/repository.go`, append to the `Repository` interface:

```go
	// ListPageByAccount returns up to limit transactions where the account is
	// source or recipient, strictly older than after in the (spent_at DESC, id)
	// order (nil after = from the newest). Callers pass limit+1 to detect a
	// further page.
	ListPageByAccount(ctx context.Context, accountID vo.Id, after *PageCursor, limit int) ([]*model.Transaction, error)

	// ListRecentByAccountIDs returns, per account id, its newest transactions
	// (source or recipient) in (spent_at DESC, id) order, at most perAccountLimit
	// each. A transfer between two requested accounts appears in both windows.
	ListRecentByAccountIDs(ctx context.Context, accountIDs []vo.Id, perAccountLimit int) (map[string][]*model.Transaction, error)
```

- [ ] **Step 4: Implement in the repo**

Append to `internal/transaction/repo/repo.go` (uses the existing `cols` const pattern, `placeholders()`, `hydrate`, `txRow`; note `cols` is currently a local const inside `ListByAccountIDs` — lift it to a package-level `const txCols = "id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient"` and update `ListByAccountIDs` to use it):

```go
// ListPageByAccount returns one keyset page for an account. The mixed-direction
// order (spent_at DESC, id ASC) forces the expanded OR predicate instead of a
// row-value comparison.
func (r *Repo) ListPageByAccount(ctx context.Context, accountID vo.Id, after *domtransaction.PageCursor, limit int) ([]*model.Transaction, error) {
	s := accountID.String()
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(txCols)
	b.WriteString(" FROM transactions WHERE (account_id = ")
	b.WriteString(placeholders(r.driver, 1, 1))
	b.WriteString(" OR account_recipient_id = ")
	b.WriteString(placeholders(r.driver, 2, 1))
	b.WriteString(")")
	args := []any{s, s}
	if after != nil {
		b.WriteString(" AND (spent_at < ")
		b.WriteString(placeholders(r.driver, 3, 1))
		b.WriteString(" OR (spent_at = ")
		b.WriteString(placeholders(r.driver, 4, 1))
		b.WriteString(" AND id > ")
		b.WriteString(placeholders(r.driver, 5, 1))
		b.WriteString("))")
		args = append(args, after.SpentAt, after.SpentAt, after.ID.String())
	}
	b.WriteString(" ORDER BY spent_at DESC, id LIMIT ")
	b.WriteString(placeholders(r.driver, len(args)+1, 1))
	args = append(args, limit)
	return r.scanTransactions(ctx, b.String(), args)
}

// ListRecentByAccountIDs windows each account's newest rows via ROW_NUMBER over
// the union of source-side and recipient-side matches, so a transfer ranks in
// both accounts' partitions.
func (r *Repo) ListRecentByAccountIDs(ctx context.Context, accountIDs []vo.Id, perAccountLimit int) (map[string][]*model.Transaction, error) {
	out := make(map[string][]*model.Transaction, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	ids := make([]any, len(accountIDs))
	for i, id := range accountIDs {
		ids[i] = id.String()
	}
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(txCols)
	b.WriteString(", acct FROM (SELECT u.*, ROW_NUMBER() OVER (PARTITION BY u.acct ORDER BY u.spent_at DESC, u.id) AS rn FROM (")
	b.WriteString("SELECT ")
	b.WriteString(txCols)
	b.WriteString(", account_id AS acct FROM transactions WHERE account_id IN (")
	b.WriteString(placeholders(r.driver, 1, len(ids)))
	b.WriteString(") UNION ALL SELECT ")
	b.WriteString(txCols)
	b.WriteString(", account_recipient_id AS acct FROM transactions WHERE account_recipient_id IN (")
	b.WriteString(placeholders(r.driver, 1+len(ids), len(ids)))
	b.WriteString(")) u) w WHERE w.rn <= ")
	b.WriteString(placeholders(r.driver, 1+2*len(ids), 1))
	b.WriteString(" ORDER BY acct, rn")

	args := make([]any, 0, len(ids)*2+1)
	args = append(args, ids...)
	args = append(args, ids...)
	args = append(args, perAccountLimit)

	rows, err := r.db(ctx).QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row txRow
		var acct string
		if serr := rows.Scan(
			&row.ID, &row.UserID, &row.AccountID, &row.AccountRecipientID, &row.CategoryID,
			&row.PayeeID, &row.TagID, &row.Description, &row.CreatedAt, &row.UpdatedAt,
			&row.SpentAt, &row.Type, &row.Amount, &row.AmountRecipient, &acct,
		); serr != nil {
			return nil, serr
		}
		t, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out[acct] = append(out[acct], t)
	}
	return out, rows.Err()
}

// scanTransactions runs a hand-built select over txCols and hydrates the rows.
func (r *Repo) scanTransactions(ctx context.Context, query string, args []any) ([]*model.Transaction, error) {
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Transaction
	for rows.Next() {
		var row txRow
		if serr := rows.Scan(
			&row.ID, &row.UserID, &row.AccountID, &row.AccountRecipientID, &row.CategoryID,
			&row.PayeeID, &row.TagID, &row.Description, &row.CreatedAt, &row.UpdatedAt,
			&row.SpentAt, &row.Type, &row.Amount, &row.AmountRecipient,
		); serr != nil {
			return nil, serr
		}
		t, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
```

Optionally refactor the body of `ListByAccountIDs` to use `scanTransactions` (identical scan loop) — do it, it's a pure extraction.

- [ ] **Step 5: Run tests (both engines)**

Run: `go test ./internal/transaction/repo/ -v`
Expected: PASS.
Then, if a local PostgreSQL is available (`DATABASE_TEST_PGSQL_URL` set): `make test-repo-pgsql` — otherwise note that CI runs it.

- [ ] **Step 6: Commit**

```bash
git add internal/transaction/repository.go internal/transaction/repo/repo.go internal/transaction/repo/paging_integration_test.go
git commit -m "feat(transaction): keyset page + per-account recent-window repo queries"
```

### Task 4: Use case branches + handler wiring

**Files:**
- Modify: `internal/transaction/read.go`, `internal/transaction/api/transactionlist.go`
- Test: `internal/transaction/api/get_transaction_list_test.go` (add tests)

**Interfaces:**
- Consumes: Task 1 (`decodeCursor`, `EncodeCursor`, `PageCursor`), Task 2 (request fields/methods, result structs), Task 3 (repo methods).
- Produces: `GetTransactionList` handling four modes; a package-level helper `s.buildItems(ctx, txs)` shared by all modes.

- [ ] **Step 1: Write the failing endpoint tests**

Add to `internal/transaction/api/get_transaction_list_test.go` (the harness `newHarness`, `h.token`, `h.do`, and constants `accountID`, `usdID` already exist in this package — reuse them; inspect `harness_test.go` for the fixture seeding to learn which transactions exist on `accountID`, and seed extra ones with `fixture.New` as the `ForbiddenAccount` test does):

```go
// TestGetTransactionList_PageMode: limit+cursor walks the account newest-first
// with a stable envelope: items plus a "page" block.
func TestGetTransactionList_PageMode(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	// three rows with distinct dates so page boundaries are unambiguous
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f1", UserID: userID, AccountID: accountID, Type: 0, Amount: "1.00000000", SpentAt: "2026-06-03 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f2", UserID: userID, AccountID: accountID, Type: 0, Amount: "2.00000000", SpentAt: "2026-06-02 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f3", UserID: userID, AccountID: accountID, Type: 0, Amount: "3.00000000", SpentAt: "2026-06-01 12:00:00"})

	status, env := h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?accountId="+accountID+"&limit=2", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	data := env.dataMap()
	page, ok := data["page"].(map[string]any)
	if !ok {
		t.Fatalf("no page block in %v", data)
	}
	if page["hasMore"] != true || page["nextCursor"] == nil {
		t.Fatalf("page = %v, want hasMore=true with a cursor", page)
	}

	status, env = h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?accountId="+accountID+"&limit=2&cursor="+url.QueryEscape(page["nextCursor"].(string)), tok, nil)
	if status != http.StatusOK {
		t.Fatalf("page 2 status = %d; body: %s", status, env.raw)
	}
	page2 := env.dataMap()["page"].(map[string]any)
	if page2["hasMore"] != false || page2["nextCursor"] != nil {
		t.Fatalf("page 2 = %v, want hasMore=false, nil cursor", page2)
	}
}

// TestGetTransactionList_BootMode: perAccountLimit returns deduped items plus
// per-account cursors, and NO page block.
func TestGetTransactionList_BootMode(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f1", UserID: userID, AccountID: accountID, Type: 0, Amount: "1.00000000", SpentAt: "2026-06-03 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0000000-0000-0000-0000-0000000000f2", UserID: userID, AccountID: accountID, Type: 0, Amount: "2.00000000", SpentAt: "2026-06-02 12:00:00"})

	status, env := h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?perAccountLimit=1", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	data := env.dataMap()
	accounts, ok := data["accounts"].([]any)
	if !ok || len(accounts) == 0 {
		t.Fatalf("no accounts block in %v", data)
	}
	var entry map[string]any
	for _, a := range accounts {
		if m := a.(map[string]any); m["id"] == accountID {
			entry = m
		}
	}
	if entry == nil {
		t.Fatalf("no entry for %s in %v", accountID, accounts)
	}
	if entry["hasMore"] != true || entry["nextCursor"] == nil {
		t.Fatalf("entry = %v, want hasMore=true with a cursor", entry)
	}
	if _, hasPage := data["page"]; hasPage {
		t.Fatalf("boot mode must not include a page block: %v", data)
	}
}

// TestGetTransactionList_LegacyShapeUnchanged: a no-param call has ONLY items.
func TestGetTransactionList_LegacyShapeUnchanged(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/transaction/get-transaction-list", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	data := env.dataMap()
	for _, forbidden := range []string{"page", "accounts"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("legacy response leaked %q: %v", forbidden, data)
		}
	}
}

// TestGetTransactionList_BadCursor: a malformed cursor is a 400 validation error.
func TestGetTransactionList_BadCursor(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodGet,
		"/api/v1/transaction/get-transaction-list?accountId="+accountID+"&limit=2&cursor=not-a-cursor", tok, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	msgs := env.errorsMap()["cursor"]
	if len(msgs) == 0 || msgs[0] != "This value is not a valid cursor." {
		t.Fatalf("errors[cursor] = %v", msgs)
	}
}
```

Adjust to the real harness helpers: check `harness_test.go` for the exact names of `env.dataMap()` / `env.errorsMap()` / `userID`; if `dataMap` does not exist, add a tiny helper next to `errorsMap` following its pattern. `fixture.Transaction.SpentAt` accepts a `"Y-m-d H:i:s"` string (see `internal/test/fixture/entities.go:377`).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/transaction/api/ -run TestGetTransactionList -v`
Expected: new tests FAIL (no page/accounts blocks yet); pre-existing ones PASS.

- [ ] **Step 3: Implement the use case branches**

Rewrite `internal/transaction/read.go` — keep `parseFlexible` as is, restructure `GetTransactionList`, extract the author loop:

```go
// GetTransactionList returns transactions in one of four modes: a keyset page
// for one account (accountId+limit[+cursor]), the newest perAccountLimit rows
// per visible account (perAccountLimit), a single account's full list
// (accountId), or a [periodStart, periodEnd) window across visible accounts /
// everything visible (legacy).
func (s *Service) GetTransactionList(ctx context.Context, userID vo.Id, req model.TransactionListRequest) (*model.GetTransactionListResult, error) {
	switch {
	case req.PerAccountLimit != "":
		return s.listBootWindows(ctx, userID, req.PerAccountLimitValue())
	case req.Limit != "":
		return s.listPage(ctx, userID, req)
	}

	var txs []*model.Transaction
	switch {
	case req.AccountId != "":
		accountID, err := vo.ParseId(req.AccountId)
		if err != nil {
			return nil, err
		}
		if aerr := s.checkViewAccess(ctx, userID, accountID); aerr != nil {
			return nil, aerr
		}
		list, err := s.repo.ListByAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		txs = list
	default:
		ids, err := s.visible.VisibleAccountIDs(ctx, userID)
		if err != nil {
			return nil, err
		}
		var start, end time.Time
		if req.PeriodStart != "" && req.PeriodEnd != "" {
			if start, err = parseFlexible(req.PeriodStart); err != nil {
				return nil, err
			}
			if end, err = parseFlexible(req.PeriodEnd); err != nil {
				return nil, err
			}
		}
		list, err := s.repo.ListByAccountIDs(ctx, ids, start, end)
		if err != nil {
			return nil, err
		}
		txs = list
	}

	items, err := s.buildItems(ctx, txs)
	if err != nil {
		return nil, err
	}
	return &model.GetTransactionListResult{Items: items}, nil
}

func (s *Service) listPage(ctx context.Context, userID vo.Id, req model.TransactionListRequest) (*model.GetTransactionListResult, error) {
	accountID, err := vo.ParseId(req.AccountId)
	if err != nil {
		return nil, err
	}
	if aerr := s.checkViewAccess(ctx, userID, accountID); aerr != nil {
		return nil, aerr
	}
	var after *PageCursor
	if req.Cursor != "" {
		c, cerr := decodeCursor(req.Cursor)
		if cerr != nil {
			return nil, cerr
		}
		after = &c
	}
	limit := req.LimitValue()
	rows, err := s.repo.ListPageByAccount(ctx, accountID, after, limit+1)
	if err != nil {
		return nil, err
	}
	page := &model.TransactionPageResult{HasMore: len(rows) > limit}
	if page.HasMore {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := EncodeCursor(PageCursor{SpentAt: last.SpentAt, ID: last.ID})
		page.NextCursor = &c
	}
	items, err := s.buildItems(ctx, rows)
	if err != nil {
		return nil, err
	}
	return &model.GetTransactionListResult{Items: items, Page: page}, nil
}

func (s *Service) listBootWindows(ctx context.Context, userID vo.Id, limit int) (*model.GetTransactionListResult, error) {
	ids, err := s.visible.VisibleAccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	windows, err := s.repo.ListRecentByAccountIDs(ctx, ids, limit+1)
	if err != nil {
		return nil, err
	}
	accounts := make([]model.TransactionAccountPageResult, 0, len(ids))
	seen := make(map[string]bool)
	var txs []*model.Transaction
	for _, id := range ids { // VisibleAccountIDs order keeps the accounts block deterministic
		rows := windows[id.String()]
		info := model.TransactionAccountPageResult{Id: id.String(), HasMore: len(rows) > limit}
		if info.HasMore {
			rows = rows[:limit]
			last := rows[len(rows)-1]
			c := EncodeCursor(PageCursor{SpentAt: last.SpentAt, ID: last.ID})
			info.NextCursor = &c
		}
		accounts = append(accounts, info)
		for _, t := range rows {
			if !seen[t.ID.String()] {
				seen[t.ID.String()] = true
				txs = append(txs, t)
			}
		}
	}
	sort.Slice(txs, func(i, j int) bool {
		if !txs[i].SpentAt.Equal(txs[j].SpentAt) {
			return txs[i].SpentAt.After(txs[j].SpentAt)
		}
		return txs[i].ID.String() < txs[j].ID.String()
	})
	items, err := s.buildItems(ctx, txs)
	if err != nil {
		return nil, err
	}
	return &model.GetTransactionListResult{Items: items, Accounts: accounts}, nil
}

// buildItems resolves each transaction's author embed through a per-request
// cache. A list can contain thousands of rows that nearly all share the same
// author (the owner, plus a few connected users on shared accounts), and each
// GetOwner is a DB round-trip; without the cache that is an N+1 that dominates
// the endpoint's latency.
func (s *Service) buildItems(ctx context.Context, txs []*model.Transaction) ([]model.TransactionResult, error) {
	authors := make(map[string]model.UserResult)
	items := make([]model.TransactionResult, 0, len(txs))
	for _, t := range txs {
		uid := t.UserID.String()
		author, ok := authors[uid]
		if !ok {
			av, err := s.users.GetOwner(ctx, uid)
			if err != nil {
				return nil, err
			}
			author = model.UserResult{Id: av.ID, Avatar: av.Avatar, Name: av.Name}
			authors[uid] = author
		}
		items = append(items, s.buildResult(t, author))
	}
	return items, nil
}
```

Add `"sort"` to the imports.

- [ ] **Step 4: Wire the handler params + swagger**

In `internal/transaction/api/transactionlist.go`, extend the request build and the annotations:

```go
	req := model.TransactionListRequest{
		AccountId:       q.Get("accountId"),
		PeriodStart:     q.Get("periodStart"),
		PeriodEnd:       q.Get("periodEnd"),
		Limit:           q.Get("limit"),
		Cursor:          q.Get("cursor"),
		PerAccountLimit: q.Get("perAccountLimit"),
	}
```

Add to the annotation block (keep the existing lines):

```go
// @Param       limit           query    string false "Page size (1-500); requires accountId. Keyset page mode."
// @Param       cursor          query    string false "Opaque page cursor from a previous response; requires limit"
// @Param       perAccountLimit query    string false "Newest N transactions per visible account (1-500); exclusive with all other params"
```

Then run: `make swagger` and stage the regenerated docs.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/transaction/... -v` then `make go-test`
Expected: PASS. If `make go-test` fails on apiparity guards (route/scenario counts), that is Task 5's job ONLY if you added routes — this task adds none, so any apiparity failure here means legacy behavior changed: STOP and fix before proceeding.

- [ ] **Step 6: Commit**

```bash
git add internal/transaction/read.go internal/transaction/api/transactionlist.go internal/transaction/api/get_transaction_list_test.go docs/
git commit -m "feat(transaction): boot-window and keyset-page modes on get-transaction-list"
```

### Task 5: apiparity scenarios + goldens for paging

**Files:**
- Create: `internal/test/apiparity/catalogue_transaction_paging.go`
- Create (generated): `internal/test/apiparity/testdata/golden/transaction_paging/*.golden.json`

**Interfaces:**
- Consumes: `transaction.EncodeCursor` / `transaction.PageCursor` (Task 1), fixture constants `OwnerAccount`, `Txn1`, `ClockTime` (existing in `fixture.go`).

- [ ] **Step 1: Write the scenario**

`internal/test/apiparity/catalogue_transaction_paging.go`:

```go
package apiparity

// Paging scenarios: boot mode (perAccountLimit), keyset page mode
// (accountId+limit[+cursor]), and the paging validation errors. The cursor is
// built from fixture constants so the golden stays deterministic.

import (
	"net/url"

	"github.com/econumo/econumo/internal/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/transaction"
)

func init() {
	register(Scenario{Name: "transaction_paging", Calls: func() []Call {
		cursor := domtransaction.EncodeCursor(domtransaction.PageCursor{
			SpentAt: ClockTime, ID: vo.MustParseId(Txn1),
		})
		return []Call{
			{Label: "boot-window", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?perAccountLimit=1", Auth: "owner"},
			{Label: "page-first", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount + "&limit=1", Auth: "owner"},
			{Label: "page-after", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount + "&limit=1&cursor=" + url.QueryEscape(cursor), Auth: "owner"},
			{Label: "err:paging-conflict", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?perAccountLimit=1&accountId=" + OwnerAccount, Auth: "owner"},
			{Label: "err:limit-without-account", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?limit=5", Auth: "owner"},
			{Label: "err:bad-cursor", Method: "GET",
				Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount + "&limit=1&cursor=@@@", Auth: "owner"},
		}
	}})
}
```

Check `fixture.go` first: confirm `ClockTime` is a `time.Time` and that the seeded transactions' `spent_at` equals `ClockTime` (fixture default). If a seeded transaction uses an explicit different time, point the cursor at the actual newest row of `OwnerAccount` per the `(spent_at DESC, id ASC)` order.

- [ ] **Step 2: Generate and inspect goldens**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`
Then: `git status --short internal/test/apiparity/testdata/`
Expected: ONLY added files (new `transaction_paging` goldens). Any MODIFIED existing golden = frozen-contract breakage: STOP, find what changed in legacy behavior, fix, regenerate.
Read the new goldens and verify: boot-window has `accounts` with `hasMore`/`nextCursor`, page-after returns the second transaction, error responses carry the exact frozen messages.

- [ ] **Step 3: Run the full smoke + parity suite**

Run: `make go-test`
Expected: PASS (guards: scenario count grew, no orphaned goldens).

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity/
git commit -m "test(apiparity): paging scenarios + goldens for get-transaction-list"
```

---

## Part B — Backend: sort endpoints

### Task 6: sort-category-list

**Files:**
- Modify: `internal/model/category_dto.go` (new DTOs), `internal/category/repository.go` (UsageCounts), `internal/category/repo/repo.go` (implementation), `internal/category/api/categorylist.go` (handler), `internal/category/api/routes.go` (route)
- Create: `internal/category/sort.go`
- Test: `internal/category/api/category_endpoints_test.go` (add), `internal/category/repo/repo_integration_test.go` (add)

**Interfaces:**
- Produces:
  - `model.SortCategoryListRequest{By string "json:by"; Direction string "json:direction"; PeriodMonths int "json:periodMonths"}` with `Validate()`.
  - `model.SortCategoryListResult{Items []CategoryResult "json:items"}`.
  - `category.Repository.UsageCounts(ctx context.Context, userID vo.Id, since time.Time) (map[string]int, error)` — per owned category id, COUNT of transactions referencing it with `spent_at >= since`.
  - `category.Service.SortCategoryList(ctx, userID, req)` — sorts the OWNER-ONLY set (same set order-category-list can move; the categories settings page lists own categories only), builds `{id, position}` changes 0..N, delegates to the existing `OrderCategoryList`, returns its items.
- Frozen validation (envelope message `"Validation failed"`, matching the POST-body convention):
  - `by` blank → key `by`, `"This value should not be blank."`, code `IS_BLANK_ERROR`
  - `by` not in {name, usage} → key `by`, `"The value you selected is not a valid choice."`, code `INVALID_CHOICE_ERROR`
  - `direction` blank/invalid → key `direction`, same two messages/codes as `by`
  - `by=usage` and `periodMonths` outside [1,6] (including absent/0) → key `periodMonths`, `"This value should be an integer between 1 and 6."`
  - `by=name` and `periodMonths != 0` → key `periodMonths`, `"periodMonths is only valid when by is usage."`
- Sort semantics (deterministic, computed in Go):
  - `name`: case-insensitive (`strings.ToLower`) per `direction`; ties by id ascending.
  - `usage`: count per `direction`; ties by lowercased name ascending, then id ascending (regardless of direction). `since = clock.Now().AddDate(0, -periodMonths, 0)` (UTC clock), bound as a plain SQL parameter.

- [ ] **Step 1: Write the failing DTO + endpoint tests**

Add to `internal/category/api/category_endpoints_test.go` (mirror the existing `TestOrderCategoryList_Reorders` harness usage — same `h.do`, token, and fixture ids; read that test first and reuse its seeded categories):

```go
func TestSortCategoryList_ByName(t *testing.T) {
	h := newHarness(t)
	token := h.seedUserWithCategories(t) // reuse/adapt the seeding helper TestOrderCategoryList uses

	status, env := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"by": "name", "direction": "asc"})
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	names := itemNames(t, env) // helper: extract data.items[].name in order
	if !sort.StringsAreSorted(namesLower(names)) {
		t.Fatalf("items not name-asc: %v", names)
	}
}

func TestSortCategoryList_ByUsage(t *testing.T) {
	h := newHarness(t)
	token := h.seedUserWithCategories(t)
	// Seed an account + transactions so the LAST alphabetical category is the
	// most used — usage-desc must put it first, proving counts (not names) won.
	// Reuse the ids/user the harness seeded; adapt these constants to the
	// file's real fixture ids after reading its setup.
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	const acct = "a0000000-0000-0000-0000-0000000000e1"
	f.Account(fixture.Account{ID: acct, UserID: userID, CurrencyID: usdID, Name: "UsageAcct"})
	for i, spent := range []string{"2026-07-01 10:00:00", "2026-07-02 10:00:00", "2026-07-03 10:00:00"} {
		f.Transaction(fixture.Transaction{
			ID:         fmt.Sprintf("d0000000-0000-0000-0000-0000000000e%d", i+1),
			UserID:     userID, AccountID: acct, CategoryID: lastAlphabeticalCategoryID,
			Type: 0, Amount: "1.00000000", SpentAt: spent,
		})
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"by": "usage", "direction": "desc", "periodMonths": 6})
	if status != http.StatusOK {
		t.Fatalf("status = %d; body: %s", status, env.raw)
	}
	names := itemNames(t, env)
	if len(names) == 0 || names[0] != lastAlphabeticalCategoryName {
		t.Fatalf("usage-desc order = %v, want the most-used category first", names)
	}
}

func TestSortCategoryList_Validation(t *testing.T) {
	h := newHarness(t)
	token := h.seedUserWithCategories(t)
	cases := []struct {
		body    map[string]any
		key     string
		wantMsg string
	}{
		{map[string]any{"direction": "asc"}, "by", "This value should not be blank."},
		{map[string]any{"by": "color", "direction": "asc"}, "by", "The value you selected is not a valid choice."},
		{map[string]any{"by": "name"}, "direction", "This value should not be blank."},
		{map[string]any{"by": "usage", "direction": "asc"}, "periodMonths", "This value should be an integer between 1 and 6."},
		{map[string]any{"by": "usage", "direction": "asc", "periodMonths": 7}, "periodMonths", "This value should be an integer between 1 and 6."},
		{map[string]any{"by": "name", "direction": "asc", "periodMonths": 3}, "periodMonths", "periodMonths is only valid when by is usage."},
	}
	for _, tc := range cases {
		status, env := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("body %v: status = %d, want 400; body: %s", tc.body, status, env.raw)
		}
		msgs := env.errorsMap()[tc.key]
		if len(msgs) == 0 || msgs[0] != tc.wantMsg {
			t.Fatalf("body %v: errors[%s] = %v, want %q", tc.body, tc.key, msgs, tc.wantMsg)
		}
	}
}
```

Write the small helpers (`itemNames`, `namesLower`) in the test file; adapt seeding to the file's real harness (do NOT invent helpers — read the existing test first and copy its setup verbatim, adding transactions only for the usage test).

And a repo test in `internal/category/repo/repo_integration_test.go` (reuse the file's existing setup pattern):

```go
func TestUsageCounts(t *testing.T) {
	db := dbtest.New(t)
	repo := catrepo.NewRepo(db.Engine, db.TX)
	f := fixture.New(t, db)
	const (
		ucUSD  = "dffc2a06-6f29-4704-8575-31709adee926"
		ucUser = "11111111-1111-1111-1111-111111111111"
		ucAcct = "a0000000-0000-0000-0000-0000000000c1"
		catX   = "c0000000-0000-0000-0000-0000000000c1"
		catY   = "c0000000-0000-0000-0000-0000000000c2"
		catZ   = "c0000000-0000-0000-0000-0000000000c3" // never used
	)
	f.User(fixture.User{ID: ucUser, Name: "u"})
	f.Account(fixture.Account{ID: ucAcct, UserID: ucUser, CurrencyID: ucUSD, Name: "A"})
	f.Category(fixture.Category{ID: catX, UserID: ucUser, Name: "X", Position: 0, Type: 0})
	f.Category(fixture.Category{ID: catY, UserID: ucUser, Name: "Y", Position: 1, Type: 0})
	f.Category(fixture.Category{ID: catZ, UserID: ucUser, Name: "Z", Position: 2, Type: 0})
	seed := func(id, cat, spent string) {
		f.Transaction(fixture.Transaction{ID: id, UserID: ucUser, AccountID: ucAcct,
			CategoryID: cat, Type: 0, Amount: "1.00000000", SpentAt: spent})
	}
	seed("d0000000-0000-0000-0000-0000000000c1", catX, "2026-06-01 10:00:00")
	seed("d0000000-0000-0000-0000-0000000000c2", catX, "2026-06-02 10:00:00")
	seed("d0000000-0000-0000-0000-0000000000c3", catX, "2026-06-03 10:00:00")
	seed("d0000000-0000-0000-0000-0000000000c4", catY, "2026-06-01 10:00:00")
	seed("d0000000-0000-0000-0000-0000000000c5", catY, "2025-01-01 10:00:00") // outside the window

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	counts, err := repo.UsageCounts(context.Background(), vo.MustParseId(ucUser), since)
	if err != nil {
		t.Fatalf("UsageCounts: %v", err)
	}
	if counts[catX] != 3 || counts[catY] != 1 {
		t.Errorf("counts = %v, want X=3 Y=1", counts)
	}
	if _, ok := counts[catZ]; ok {
		t.Errorf("unused category must be absent from the map, got %v", counts)
	}
}
```

(imports: `context`, `time`, `catrepo "github.com/econumo/econumo/internal/category/repo"`, `vo`, `dbtest`, `fixture`; if the existing file already aliases the repo package differently, follow its alias. Check `fixture.Category`'s real field set against `internal/test/fixture/entities.go` before running.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/category/... -run 'TestSortCategory|TestUsageCounts' -v`
Expected: FAIL (404 route / compile errors).

- [ ] **Step 3: Implement DTOs**

Append to `internal/model/category_dto.go`:

```go
// SortCategoryListRequest is the sort-category-list body: server-side sorting
// of the user's categories by name or by usage over a sliding window.
type SortCategoryListRequest struct {
	By           string `json:"by"`
	Direction    string `json:"direction"`
	PeriodMonths int    `json:"periodMonths"`
}

func (r SortCategoryListRequest) Validate() error {
	return validateSortRequest(r.By, r.Direction, r.PeriodMonths)
}

// validateSortRequest is shared by the category/payee/tag sort DTOs (identical
// frozen messages).
func validateSortRequest(by, direction string, periodMonths int) error {
	var fields []errs.FieldError
	switch by {
	case "name":
		if periodMonths != 0 {
			fields = append(fields, errs.FieldError{Key: "periodMonths", Message: "periodMonths is only valid when by is usage."})
		}
	case "usage":
		if periodMonths < 1 || periodMonths > 6 {
			fields = append(fields, errs.FieldError{Key: "periodMonths", Message: "This value should be an integer between 1 and 6."})
		}
	case "":
		fields = append(fields, errs.FieldError{Key: "by", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	default:
		fields = append(fields, errs.FieldError{Key: "by", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
	}
	switch direction {
	case "asc", "desc":
	case "":
		fields = append(fields, errs.FieldError{Key: "direction", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	default:
		fields = append(fields, errs.FieldError{Key: "direction", Message: "The value you selected is not a valid choice.", Code: "INVALID_CHOICE_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

// SortCategoryListResult is the sort-category-list response: {items: [...]}.
type SortCategoryListResult struct {
	Items []CategoryResult `json:"items"`
}
```

- [ ] **Step 4: Implement UsageCounts**

Append to `internal/category/repository.go`'s interface:

```go
	// UsageCounts returns, for each of the owner's categories that has at least
	// one transaction with spent_at >= since, the count of such transactions.
	UsageCounts(ctx context.Context, userID vo.Id, since time.Time) (map[string]int, error)
```

(add `"time"` to imports). Append to `internal/category/repo/repo.go` — the repo has no driver field, so add the query to the `querier` interface with per-engine SQL, following the file's adapter pattern:

```go
// in the querier interface:
	UsageCounts(ctx context.Context, db backend.DBTX, userID string, since time.Time) (map[string]int, error)

// method on Repo:
func (r *Repo) UsageCounts(ctx context.Context, userID vo.Id, since time.Time) (map[string]int, error) {
	return r.q.UsageCounts(ctx, r.db(ctx), userID.String(), since)
}

// shared scan helper:
func scanUsageCounts(rows *sql.Rows) (map[string]int, error) {
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// on sqliteQuerier:
func (sqliteQuerier) UsageCounts(ctx context.Context, db backend.DBTX, userID string, since time.Time) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT c.id, COUNT(t.id) FROM categories c
		 JOIN transactions t ON t.category_id = c.id AND t.spent_at >= ?
		 WHERE c.user_id = ? GROUP BY c.id`, since, userID)
	if err != nil {
		return nil, err
	}
	return scanUsageCounts(rows)
}

// on pgsqlQuerier:
func (pgsqlQuerier) UsageCounts(ctx context.Context, db backend.DBTX, userID string, since time.Time) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT c.id, COUNT(t.id) FROM categories c
		 JOIN transactions t ON t.category_id = c.id AND t.spent_at >= $1
		 WHERE c.user_id = $2 GROUP BY c.id`, since, userID)
	if err != nil {
		return nil, err
	}
	return scanUsageCounts(rows)
}
```

Add missing imports (`database/sql`, `time`). If the querier interface lives in a different file (e.g. `read.go` vs `repo.go`), put the method where the WRITE querier interface is declared.

- [ ] **Step 5: Implement the use case + handler + route**

Create `internal/category/sort.go`:

```go
// Sort use case: server-side ordering of the user's categories by name or by
// transaction usage over a sliding window, delegating the position writes to
// the order use case so own/shared semantics stay identical.
package category

import (
	"context"
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) SortCategoryList(ctx context.Context, userID vo.Id, req model.SortCategoryListRequest) (*model.SortCategoryListResult, error) {
	// Owner-only: the same set order-category-list can move (shared categories
	// keep their positions), and the same set the settings page displays.
	cats, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	var counts map[string]int
	if req.By == "usage" {
		since := s.clock.Now().AddDate(0, -req.PeriodMonths, 0)
		counts, err = s.repo.UsageCounts(ctx, userID, since)
		if err != nil {
			return nil, err
		}
	}
	asc := req.Direction == "asc"
	sort.SliceStable(cats, func(i, j int) bool {
		a, b := cats[i], cats[j]
		if req.By == "usage" {
			ca, cb := counts[a.ID.String()], counts[b.ID.String()]
			if ca != cb {
				if asc {
					return ca < cb
				}
				return ca > cb
			}
			// usage ties break by name asc then id asc, regardless of direction
			na, nb := strings.ToLower(a.Name), strings.ToLower(b.Name)
			if na != nb {
				return na < nb
			}
			return a.ID.String() < b.ID.String()
		}
		na, nb := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if na != nb {
			if asc {
				return na < nb
			}
			return na > nb
		}
		return a.ID.String() < b.ID.String()
	})
	changes := make([]model.PositionChange, len(cats))
	for i, c := range cats {
		changes[i] = model.PositionChange{Id: c.ID.String(), Position: i}
	}
	ordered, err := s.OrderCategoryList(ctx, userID, model.OrderCategoryListRequest{Changes: changes})
	if err != nil {
		return nil, err
	}
	return &model.SortCategoryListResult{Items: ordered.Items}, nil
}
```

Add the handler to `internal/category/api/categorylist.go`:

```go
// SortCategoryList handles POST /api/v1/category/sort-category-list (auth).
//
// @Summary     Sort the category list
// @Description Reorders the user's categories server-side by name or by usage over a sliding window, and returns the full ordered list.
// @Tags        Category
// @Accept      json
// @Produce     json
// @Param       request body     model.SortCategoryListRequest true "Sort category list request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.SortCategoryListResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/category/sort-category-list [post]
func (h *Handlers) SortCategoryList(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, h.svc.SortCategoryList)
}
```

Register in `internal/category/api/routes.go` after the order route:

```go
		mux.Handle("POST /api/v1/category/sort-category-list", auth(h.SortCategoryList))
```

Run `make swagger`.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/category/... -v`
Expected: PASS. (`make go-test` will fail on the apiparity route guard until Task 8 adds scenarios — that is expected mid-part; run `go test` per package until then.)

- [ ] **Step 7: Commit**

```bash
git add internal/model/category_dto.go internal/category/ docs/
git commit -m "feat(category): sort-category-list endpoint (by name / usage)"
```

### Task 7: sort-payee-list and sort-tag-list

**Files:**
- Modify: `internal/model/payee_dto.go`, `internal/model/tag_dto.go`, `internal/payee/repository.go`, `internal/payee/repo/repo.go`, `internal/tag/repository.go`, `internal/tag/repo/repo.go`, `internal/payee/api/*.go`, `internal/tag/api/*.go` (handler + routes files — find them via `grep -rn "order-payee-list\|order-tag-list" internal/{payee,tag}/api/`)
- Create: `internal/payee/sort.go`, `internal/tag/sort.go`
- Test: add cases to `internal/payee/api/*_endpoints_test.go` and `internal/tag/api/*_endpoints_test.go`

**Interfaces:**
- Produces: `model.SortPayeeListRequest` / `model.SortTagListRequest` (identical fields to `SortCategoryListRequest`, each's `Validate()` calls the shared `validateSortRequest` from Task 6 — it lives in `internal/model`, reachable from all three DTO files), `model.SortPayeeListResult{Items []PayeeResult}`, `model.SortTagListResult{Items []TagResult}`; `payee.Repository.UsageCounts` / `tag.Repository.UsageCounts` (same signature as category's, joining `t.payee_id` / `t.tag_id` and `payees`/`tags` tables); `Service.SortPayeeList` / `Service.SortTagList`.
- Semantics identical to Task 6, substituting: `ListByOwner` (both modules have it), `OrderPayeeList` / `OrderTagList` delegation, `POST /api/v1/payee/sort-payee-list`, `POST /api/v1/tag/sort-tag-list`.

- [ ] **Step 1: Write failing endpoint tests** — copy the three category tests from Task 6 Step 1 into the payee and tag endpoint-test files, substituting routes, seeded entity names, and the usage seeding (`fixture.Transaction{..., PayeeID: ...}` / `{..., TagID: ...}`).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/payee/... ./internal/tag/... -run TestSort -v`
Expected: FAIL.

- [ ] **Step 3: Implement** — mirror Task 6 exactly per module:
  - DTOs in `payee_dto.go` / `tag_dto.go` (reuse `validateSortRequest`).
  - `UsageCounts` on each repo's querier interface + both engine adapters; SQL is Task 6's with `categories c`→`payees p`/`tags g`, `t.category_id`→`t.payee_id`/`t.tag_id`, `c.user_id`→`p.user_id`/`g.user_id`.
  - `internal/payee/sort.go` / `internal/tag/sort.go`: Task 6's `sort.go` with types substituted (`model.Payee`/`model.Tag`, `OrderPayeeList`/`OrderTagList`, `model.OrderPayeeListRequest`/`model.OrderTagListRequest`). The comparator body is identical — copy it whole; do not try to generify across modules (the aggregates are distinct types and the modules must stay independent).
  - Handlers + routes: mirror the category handler annotation block with Payee/Tag tags and paths; register `POST /api/v1/payee/sort-payee-list` and `POST /api/v1/tag/sort-tag-list`.
  - Run `make swagger`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/payee/... ./internal/tag/... ./internal/model/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/ internal/payee/ internal/tag/ docs/
git commit -m "feat(payee,tag): sort-payee-list and sort-tag-list endpoints"
```

### Task 8: apiparity scenarios + goldens for sorting

**Files:**
- Create: `internal/test/apiparity/catalogue_sortlists.go`
- Create (generated): `internal/test/apiparity/testdata/golden/sort_lists/*.golden.json`

- [ ] **Step 1: Write the scenario**

```go
package apiparity

// sort_lists exercises the three sort-*-list routes: a name sort per module
// with a follow-up read, a usage sort (the seeded transactions fall inside the
// 6-month window relative to the frozen clock), and the validation errors that
// freeze the messages.
func init() {
	register(Scenario{Name: "sort_lists", Calls: func() []Call {
		return []Call{
			{Label: "sort-category-name-asc", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "name", "direction": "asc"}},
			{Label: "get-category-list-after-sort", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "sort-category-usage-desc", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "usage", "direction": "desc", "periodMonths": 6}},
			{Label: "sort-payee-name-asc", Method: "POST", Path: "/api/v1/payee/sort-payee-list", Auth: "owner",
				Body: map[string]any{"by": "name", "direction": "asc"}},
			{Label: "sort-tag-name-desc", Method: "POST", Path: "/api/v1/tag/sort-tag-list", Auth: "owner",
				Body: map[string]any{"by": "name", "direction": "desc"}},
			{Label: "err:sort-bad-by", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "color", "direction": "asc"}},
			{Label: "err:sort-usage-no-period", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "usage", "direction": "asc"}},
		}
	}})
}
```

- [ ] **Step 2: Generate goldens, inspect, run guards**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` then `git status --short internal/test/apiparity/testdata/`
Expected: only ADDED files. Inspect the new goldens: name-asc must show alphabetical items; usage-desc must put the fixture's most-transacted category first.
Then: `make go-test` — Expected: PASS (route guard now satisfied for the three new routes).

- [ ] **Step 3: Engine comparison (if PostgreSQL available)**

Run: `make test` (or note CI covers it).
Expected: PASS — byte-identical on both engines.

- [ ] **Step 4: Commit**

```bash
git add internal/test/apiparity/
git commit -m "test(apiparity): sort-list scenarios + goldens"
```

---

## Part C — Frontend

### Task 9: API client — paging params and sort functions

**Files:**
- Modify: `web/src/api/transaction.ts`, `web/src/api/category.ts`, `web/src/api/payee.ts`, `web/src/api/tag.ts`, `web/src/api/dto/transaction.ts`

**Interfaces (produced, used by every later task):**

```ts
// api/dto/transaction.ts additions
export interface TransactionPageDto { nextCursor: string | null; hasMore: boolean }
export interface TransactionAccountPageDto { id: string; nextCursor: string | null; hasMore: boolean }

// api/transaction.ts
export interface TransactionListParams {
  perAccountLimit?: number
  accountId?: Id
  limit?: number
  cursor?: string
  periodStart?: string // strict "Y-m-d H:i:s"
  periodEnd?: string
}
export interface TransactionListResponse {
  items: TransactionDto[]
  page?: TransactionPageDto
  accounts?: TransactionAccountPageDto[]
}
export async function getTransactionList(params: TransactionListParams = {}): Promise<TransactionListResponse>

// api/category.ts (payee/tag identical with their DTO types)
export interface SortListForm { by: 'name' | 'usage'; direction: 'asc' | 'desc'; periodMonths?: number }
export async function sortCategoryList(form: SortListForm): Promise<CategoryDto[]>
```

- [ ] **Step 1: Implement**

In `web/src/api/transaction.ts` replace `getTransactionList`:

```ts
export async function getTransactionList(params: TransactionListParams = {}): Promise<TransactionListResponse> {
  const response = await api.get<Envelope<TransactionListResponse>>(
    apiUrl('/api/v1/transaction/get-transaction-list'),
    { params },
  )
  const data = response.data.data
  return { ...data, items: data.items.map(coerceTransaction) }
}
```

(with the two interfaces above; import the new DTO types). In `web/src/api/category.ts` add (define `SortListForm` here and re-export/import it in payee.ts/tag.ts to avoid three copies):

```ts
export interface SortListForm {
  by: 'name' | 'usage'
  direction: 'asc' | 'desc'
  periodMonths?: number
}

export async function sortCategoryList(form: SortListForm): Promise<CategoryDto[]> {
  const response = await api.post<Envelope<{ items: CategoryDto[] }>>(apiUrl('/api/v1/category/sort-category-list'), form)
  return response.data.data.items
}
```

`web/src/api/payee.ts` / `web/src/api/tag.ts`: same function with `sort-payee-list`/`sort-tag-list` paths and `PayeeDto`/`TagDto`, importing `SortListForm` from `./category`.

- [ ] **Step 2: Fix the one existing caller and typecheck**

`web/src/features/transactions/queries.ts` still calls `getTransactionList` expecting an array — temporarily change its queryFn to `() => transactionApi.getTransactionList().then((r) => r.items)` (Task 10 replaces this properly).

Run: `cd web && pnpm lint && npx tsc --noEmit`
Expected: clean (or only pre-existing warnings).

- [ ] **Step 3: Commit**

```bash
git add web/src/api/
git commit -m "feat(web/api): paged transaction list params + sort-list clients"
```

### Task 10: Window bookkeeping helpers (pure) + tests

**Files:**
- Create: `web/src/features/transactions/window.ts`
- Test: `web/src/features/transactions/window.test.ts`

**Interfaces (produced):**

```ts
export const PER_ACCOUNT_LIMIT = 50
export const PAGE_LIMIT = 50
export interface TxKey { date: string; id: string }
export interface AccountPageState { nextCursor: string | null; hasMore: boolean; oldestLoaded: TxKey | null }
export type TransactionPagesMap = Record<string, AccountPageState>
export function byNewestFirst(a: TransactionDto, b: TransactionDto): number
export function isOlderThan(tx: TxKey, boundary: TxKey): boolean
export function buildPagesFromBoot(items: TransactionDto[], accounts: TransactionAccountPageDto[], perAccountLimit?: number): TransactionPagesMap
export function mergeTransactions(prev: TransactionDto[], fetched: TransactionDto[]): TransactionDto[]
export function advancePage(prev: AccountPageState, page: TransactionPageDto | undefined, fetched: TransactionDto[]): AccountPageState
```

- [ ] **Step 1: Write the failing tests**

`web/src/features/transactions/window.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { TransactionDto } from '@/api/dto/transaction'
import { advancePage, buildPagesFromBoot, isOlderThan, mergeTransactions } from './window'

const author = { id: 'u1', name: 'U', avatar: 'face:fuchsia' }
function tx(id: string, date: string, accountId: string, accountRecipientId: string | null = null): TransactionDto {
  return {
    id, date, accountId, accountRecipientId,
    type: accountRecipientId ? 'transfer' : 'expense',
    amount: 1, amountRecipient: accountRecipientId ? 1 : null,
    categoryId: null, description: '', payeeId: null, tagId: null,
    author,
  } as unknown as TransactionDto
}

describe('isOlderThan', () => {
  it('orders by date desc then id asc', () => {
    expect(isOlderThan({ date: '2026-06-01 10:00:00', id: 'b' }, { date: '2026-06-02 10:00:00', id: 'a' })).toBe(true)
    expect(isOlderThan({ date: '2026-06-02 10:00:00', id: 'b' }, { date: '2026-06-02 10:00:00', id: 'a' })).toBe(true) // same date, larger id = older position
    expect(isOlderThan({ date: '2026-06-02 10:00:00', id: 'a' }, { date: '2026-06-02 10:00:00', id: 'a' })).toBe(false)
  })
})

describe('buildPagesFromBoot', () => {
  it('sets the horizon at the Nth-newest row touching the account, ignoring stray transfers', () => {
    // Account B window (perAccountLimit=2): tx3 (Jun 5), tx4 (Jun 4).
    // tx1 is a transfer A->B from Jun 1 that arrived via A's window: it must
    // NOT widen B's horizon.
    const items = [
      tx('tx3', '2026-06-05 10:00:00', 'B'),
      tx('tx4', '2026-06-04 10:00:00', 'B'),
      tx('tx1', '2026-06-01 10:00:00', 'A', 'B'),
    ]
    const pages = buildPagesFromBoot(items, [
      { id: 'A', nextCursor: null, hasMore: false },
      { id: 'B', nextCursor: 'cursorB', hasMore: true },
    ], 2)
    expect(pages['A']).toEqual({ nextCursor: null, hasMore: false, oldestLoaded: null })
    expect(pages['B'].oldestLoaded).toEqual({ date: '2026-06-04 10:00:00', id: 'tx4' })
    expect(pages['B'].nextCursor).toBe('cursorB')
  })
})

describe('mergeTransactions', () => {
  it('dedupes by id, preferring the freshly fetched row', () => {
    const merged = mergeTransactions(
      [tx('a', '2026-06-05 10:00:00', 'A'), tx('b', '2026-06-04 10:00:00', 'A')],
      [tx('b', '2026-06-04 10:00:00', 'A'), tx('c', '2026-06-03 10:00:00', 'A')],
    )
    expect(merged.map((t) => t.id).sort()).toEqual(['a', 'b', 'c'])
  })
})

describe('advancePage', () => {
  const prev = { nextCursor: 'c1', hasMore: true, oldestLoaded: { date: '2026-06-04 10:00:00', id: 'b' } }
  it('advances the horizon to the last fetched row', () => {
    const next = advancePage(prev, { nextCursor: 'c2', hasMore: true }, [tx('c', '2026-06-03 10:00:00', 'A')])
    expect(next).toEqual({ nextCursor: 'c2', hasMore: true, oldestLoaded: { date: '2026-06-03 10:00:00', id: 'c' } })
  })
  it('keeps the previous horizon when the page is empty', () => {
    const next = advancePage(prev, { nextCursor: null, hasMore: false }, [])
    expect(next).toEqual({ nextCursor: null, hasMore: false, oldestLoaded: prev.oldestLoaded })
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm test -- run src/features/transactions/window.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

`web/src/features/transactions/window.ts`:

```ts
import type { TransactionAccountPageDto, TransactionPageDto } from '@/api/dto/transaction'
import type { TransactionDto } from '@/api/dto/transaction'

export const PER_ACCOUNT_LIMIT = 50
export const PAGE_LIMIT = 50

export interface TxKey {
  date: string
  id: string
}

// Per-account pagination state. oldestLoaded is the account's loaded horizon:
// rows older than it (loaded via ANOTHER account's window, e.g. a transfer)
// are hidden until scrolling reaches them, so the list never shows a false gap.
export interface AccountPageState {
  nextCursor: string | null
  hasMore: boolean
  oldestLoaded: TxKey | null
}

export type TransactionPagesMap = Record<string, AccountPageState>

// the backend list order: spent_at DESC, id ASC
export function byNewestFirst(a: TxKey, b: TxKey): number {
  if (a.date !== b.date) {
    return a.date < b.date ? 1 : -1
  }
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

export function isOlderThan(tx: TxKey, boundary: TxKey): boolean {
  return tx.date < boundary.date || (tx.date === boundary.date && tx.id > boundary.id)
}

export function buildPagesFromBoot(
  items: TransactionDto[],
  accounts: TransactionAccountPageDto[],
  perAccountLimit: number = PER_ACCOUNT_LIMIT,
): TransactionPagesMap {
  const map: TransactionPagesMap = {}
  for (const acc of accounts) {
    let oldestLoaded: TxKey | null = null
    if (acc.hasMore) {
      const touching = items
        .filter((t) => t.accountId === acc.id || t.accountRecipientId === acc.id)
        .sort(byNewestFirst)
      const boundary = touching[Math.min(perAccountLimit, touching.length) - 1]
      oldestLoaded = boundary ? { date: boundary.date, id: boundary.id } : null
    }
    map[acc.id] = { nextCursor: acc.nextCursor ?? null, hasMore: acc.hasMore, oldestLoaded }
  }
  return map
}

export function mergeTransactions(prev: TransactionDto[], fetched: TransactionDto[]): TransactionDto[] {
  const ids = new Set(fetched.map((t) => t.id))
  return [...prev.filter((t) => !ids.has(t.id)), ...fetched]
}

export function advancePage(
  prev: AccountPageState,
  page: TransactionPageDto | undefined,
  fetched: TransactionDto[],
): AccountPageState {
  const last = fetched[fetched.length - 1]
  return {
    nextCursor: page?.nextCursor ?? null,
    hasMore: page?.hasMore ?? false,
    oldestLoaded: last ? { date: last.date, id: last.id } : prev.oldestLoaded,
  }
}
```

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm test -- run src/features/transactions/window.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/transactions/window.ts web/src/features/transactions/window.test.ts
git commit -m "feat(web): transaction window bookkeeping helpers"
```

### Task 11: Boot query, pages map, pager hook, persistence exclusions

**Files:**
- Modify: `web/src/app/queryKeys.ts`, `web/src/features/transactions/queries.ts`, `web/src/lib/queryPersist.ts`

**Interfaces (produced):**

```ts
// queryKeys additions
transactionPages: ['transactionPages'] as const,
transactionSearch: (accountId: string) => ['transactionSearch', accountId] as const,
transactionPeriod: (periodStart: string) => ['transactionPeriod', periodStart] as const,

// queries.ts
export function useTransactionPages(): { data: TransactionPagesMap | undefined }
export function useAccountTransactionPager(accountId: Id | undefined): {
  hasMore: boolean
  isFetching: boolean
  fetchNext: () => void
}
```

- [ ] **Step 1: queryKeys**

Add the three keys above to `web/src/app/queryKeys.ts`.

- [ ] **Step 2: Rewrite the boot query + add the pager**

In `web/src/features/transactions/queries.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import * as transactionApi from '@/api/transaction'
import type { TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import {
  advancePage,
  buildPagesFromBoot,
  mergeTransactions,
  PAGE_LIMIT,
  PER_ACCOUNT_LIMIT,
  type TransactionPagesMap,
} from './window'

export function useTransactions() {
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: queryKeys.transactions,
    // Boot loads a window per visible account. A refetch (10-min stale, focus,
    // restore) REPLACES the flat array and resets every window: scrolled-in
    // history is dropped so server-side deletions cannot linger as ghosts.
    queryFn: async () => {
      const res = await transactionApi.getTransactionList({ perAccountLimit: PER_ACCOUNT_LIMIT })
      queryClient.setQueryData<TransactionPagesMap>(
        queryKeys.transactionPages,
        buildPagesFromBoot(res.items, res.accounts ?? []),
      )
      return res.items
    },
    staleTime: TEN_MINUTES,
    // the backend orders per mode; date desc with id tie-break is re-applied here
    select: (items) => [...items].sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : a.id < b.id ? -1 : a.id > b.id ? 1 : 0)),
  })
}

// Reactive view of the per-account pagination map. The map is written
// imperatively (boot queryFn, pager) — this query never fetches.
export function useTransactionPages() {
  return useQuery<TransactionPagesMap>({
    queryKey: queryKeys.transactionPages,
    queryFn: () => ({}),
    enabled: false,
    staleTime: Infinity,
  })
}

export function useAccountTransactionPager(accountId: Id | undefined) {
  const queryClient = useQueryClient()
  const { data: pages } = useTransactionPages()
  const state = accountId ? pages?.[accountId] : undefined
  const [isFetching, setIsFetching] = useState(false)
  const inFlight = useRef(false)

  const fetchPage = useCallback(
    async (cursor: string | undefined) => {
      if (!accountId || inFlight.current) {
        return
      }
      inFlight.current = true
      setIsFetching(true)
      try {
        const res = await transactionApi.getTransactionList({ accountId, limit: PAGE_LIMIT, cursor })
        queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
          mergeTransactions(prev ?? [], res.items),
        )
        queryClient.setQueryData<TransactionPagesMap>(queryKeys.transactionPages, (prev) => {
          const current = prev?.[accountId] ?? { nextCursor: null, hasMore: false, oldestLoaded: null }
          return { ...(prev ?? {}), [accountId]: advancePage(current, res.page, res.items) }
        })
      } finally {
        inFlight.current = false
        setIsFetching(false)
      }
    },
    [accountId, queryClient],
  )

  const fetchNext = useCallback(() => {
    if (state?.hasMore && state.nextCursor) {
      void fetchPage(state.nextCursor)
    }
  }, [state, fetchPage])

  // Ensure-window: an account absent from the map (hidden-folder accounts are
  // excluded from boot) gets its first page on demand.
  useEffect(() => {
    if (accountId && pages && !pages[accountId]) {
      void fetchPage(undefined)
    }
  }, [accountId, pages, fetchPage])

  return { hasMore: state?.hasMore ?? false, isFetching, fetchNext }
}
```

Keep `useApplyTransactionItem`, `useCreateTransaction`, `useUpdateTransaction`, `useDeleteTransaction` unchanged.

- [ ] **Step 3: Account-delete cascade drops the page entry**

In `web/src/features/accounts/queries.ts`, find `useDeleteAccount`'s `onSuccess` — it already filters the deleted account's transactions out of `queryKeys.transactions`. Add, right next to that filter:

```ts
      queryClient.setQueryData<TransactionPagesMap>(queryKeys.transactionPages, (prev) => {
        if (!prev) {
          return prev
        }
        const { [accountId]: _removed, ...rest } = prev
        return rest
      })
```

(import `type { TransactionPagesMap } from '@/features/transactions/window'`; use the variable name the surrounding code uses for the deleted account's id).

- [ ] **Step 4: Persistence exclusions**

In `web/src/lib/queryPersist.ts`, extend `createPersistOptions`:

```ts
// query families that must NOT hit localStorage: on-demand full-list fetches
// (search) and month windows (budget dialog) would re-inflate storage
const EPHEMERAL_QUERIES = new Set(['transactionSearch', 'transactionPeriod'])

export function createPersistOptions() {
  return {
    persister: createSyncStoragePersister({ storage: window.localStorage, key: QUERY_CACHE_KEY }),
    maxAge: CACHE_MAX_AGE_MS,
    // a release may change response shapes; never restore across versions
    buster: getVersion(),
    dehydrateOptions: {
      shouldDehydrateQuery: (query: { queryKey: readonly unknown[]; state: { status: string } }) =>
        query.state.status === 'success' && !EPHEMERAL_QUERIES.has(query.queryKey[0] as string),
    },
  }
}
```

- [ ] **Step 5: Verify**

Run: `cd web && npx tsc --noEmit && pnpm lint && pnpm test -- run src/features/transactions/`
Expected: clean; window tests still pass. (App-level behavior is exercised in Task 12's tests and the final verification.)

- [ ] **Step 6: Commit**

```bash
git add web/src/app/queryKeys.ts web/src/features/transactions/queries.ts web/src/features/accounts/queries.ts web/src/lib/queryPersist.ts
git commit -m "feat(web): boot windows, per-account pager, persistence exclusions"
```

### Task 12: Horizon + search in useAccountTransactions, sentinel wiring in AccountPage

**Files:**
- Modify: `web/src/features/transactions/useAccountTransactions.ts`, `web/src/features/accounts/AccountPage.tsx`
- Test: `web/src/features/transactions/useAccountTransactions.test.tsx` (create)

**Interfaces:**
- Consumes: `useTransactionPages` (Task 11), `isOlderThan`/`byNewestFirst` (Task 10), `queryKeys.transactionSearch`, `getTransactionList({ accountId })`.
- Produces: `useAccountTransactions(accountId, search)` — same signature and return type (`DailyListEntry[]`) as today. `WindowedEntries` gains an optional prop `onExhausted?: () => void`.

- [ ] **Step 1: Extend useAccountTransactions**

In `web/src/features/transactions/useAccountTransactions.ts`, add imports and change the hook body:

```ts
import { useQuery } from '@tanstack/react-query'
import * as transactionApi from '@/api/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { useTransactionPages, useTransactions } from './queries'
import { byNewestFirst, isOlderThan } from './window'
```

```ts
export function useAccountTransactions(accountId: Id | undefined, search: string): DailyListEntry[] {
  const { data: transactions } = useTransactions()
  const { data: pages } = useTransactionPages()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()

  const searching = search.trim() !== ''
  // Search runs over the account's FULL list, fetched on demand (the window
  // in memory is partial). Not persisted (see queryPersist).
  const { data: searchItems } = useQuery({
    queryKey: queryKeys.transactionSearch(accountId ?? ''),
    queryFn: () => transactionApi.getTransactionList({ accountId }).then((r) => r.items),
    enabled: searching && !!accountId,
    staleTime: TEN_MINUTES,
    gcTime: TEN_MINUTES,
  })

  return useMemo(() => {
    if (!accountId) {
      return []
    }
    const state = pages?.[accountId]
    const horizon = !searching && state?.hasMore ? state.oldestLoaded : null
    const source = searching ? searchItems : transactions
    if (!source) {
      return []
    }
    const enriched: ViewTransaction[] = [...source]
      .sort(byNewestFirst)
      .filter((tx) => tx.accountId === accountId || tx.accountRecipientId === accountId)
      .filter((tx) => !horizon || !isOlderThan(tx, horizon))
      .map((tx) => ({
        ...tx,
        account: accounts?.find((a) => a.id === tx.accountId),
        accountRecipient: tx.accountRecipientId ? accounts?.find((a) => a.id === tx.accountRecipientId) : undefined,
        category: tx.categoryId ? categories?.find((c) => c.id === tx.categoryId) : undefined,
        payee: tx.payeeId ? payees?.find((p) => p.id === tx.payeeId) : undefined,
        tag: tx.tagId ? tags?.find((tg) => tg.id === tx.tagId) : undefined,
        isInFuture: isFuture(tx.date),
      }))
    // ... keep the existing terms filter and day-grouping code UNCHANGED below
```

(the `terms` filter and grouping loop stay exactly as they are; update the `useMemo` dependency array to `[transactions, searchItems, searching, pages, accounts, categories, payees, tags, accountId, search]`).

- [ ] **Step 2: Wire the sentinel**

In `web/src/features/accounts/AccountPage.tsx`:

1. Import and instantiate the pager:

```ts
import { useAccountTransactionPager, useDeleteTransaction } from '@/features/transactions/queries'
// inside AccountPage():
const pager = useAccountTransactionPager(id)
```

2. Extend `WindowedEntries` with the `onExhausted` prop:

```tsx
function WindowedEntries({ entries, onExhausted, children }: { entries: DailyListEntry[]; onExhausted?: () => void; children: (entry: DailyListEntry) => ReactNode }) {
  const [visibleCount, setVisibleCount] = useState(LIST_CHUNK)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  const hasMore = visibleCount < entries.length

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel) {
      return
    }
    // The parent is the scroll container; it must be the observer root —
    // rootMargin on the default (viewport) root does not expand the clip
    // rect of a scrollable ancestor, so prefetch would never trigger.
    const observer = new IntersectionObserver(
      (hits) => {
        if (hits.some((hit) => hit.isIntersecting)) {
          if (hasMore) {
            setVisibleCount((count) => count + LIST_CHUNK)
          } else {
            onExhausted?.()
          }
        }
      },
      { root: sentinel.parentElement, rootMargin: '600px' },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
    // re-observe after each growth: the sentinel may still be in range and the
    // observer only fires on intersection *changes*
  }, [hasMore, visibleCount, onExhausted])

  return (
    <>
      {entries.slice(0, visibleCount).map(children)}
      {hasMore || onExhausted ? <div ref={sentinelRef} aria-hidden="true" className="h-px" /> : null}
    </>
  )
}
```

3. Pass the prop at the call site (search mode has the full dataset — no paging):

```tsx
<WindowedEntries
  key={account.id}
  entries={entries}
  onExhausted={pager.hasMore && search.trim() === '' ? pager.fetchNext : undefined}
>
```

- [ ] **Step 3: Write the hook test**

`web/src/features/transactions/useAccountTransactions.test.tsx` — follow the rendering pattern of an existing hook/component test in the repo (e.g. `PromptDialog.test.tsx` for setup; wrap in a `QueryClientProvider` with a fresh `QueryClient`, pre-seed the cache instead of mocking HTTP):

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { queryKeys } from '@/app/queryKeys'
import { useAccountTransactions } from './useAccountTransactions'

function setup(transactions: unknown[], pages: unknown) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(queryKeys.transactions, transactions)
  client.setQueryData(queryKeys.transactionPages, pages)
  client.setQueryData(queryKeys.accounts, [])
  client.setQueryData(queryKeys.categories, [])
  client.setQueryData(queryKeys.payees, [])
  client.setQueryData(queryKeys.tags, [])
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return renderHook(() => useAccountTransactions('B', ''), { wrapper })
}

const author = { id: 'u1', name: 'U', avatar: 'face:fuchsia' }
const base = { type: 'expense', amount: 1, amountRecipient: null, categoryId: null, description: '', payeeId: null, tagId: null, author, accountRecipientId: null }

describe('useAccountTransactions horizon', () => {
  it('hides rows older than the loaded horizon while hasMore', () => {
    const { result } = setup(
      [
        { ...base, id: 'new', date: '2026-06-05 10:00:00', accountId: 'B' },
        { ...base, id: 'stray', date: '2026-01-01 10:00:00', accountId: 'A', accountRecipientId: 'B', type: 'transfer', amountRecipient: 1 },
      ],
      { B: { nextCursor: 'c', hasMore: true, oldestLoaded: { date: '2026-06-05 10:00:00', id: 'new' } } },
    )
    const txIds = result.current.filter((e) => e.kind === 'transaction').map((e) => e.transaction.id)
    expect(txIds).toEqual(['new'])
  })

  it('shows everything once the account is exhausted', () => {
    const { result } = setup(
      [
        { ...base, id: 'new', date: '2026-06-05 10:00:00', accountId: 'B' },
        { ...base, id: 'old', date: '2026-01-01 10:00:00', accountId: 'B' },
      ],
      { B: { nextCursor: null, hasMore: false, oldestLoaded: null } },
    )
    const txIds = result.current.filter((e) => e.kind === 'transaction').map((e) => e.transaction.id)
    expect(txIds).toEqual(['new', 'old'])
  })
})
```

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm test -- run src/features/transactions/ && npx tsc --noEmit && pnpm lint`
Expected: PASS (ignore the pre-existing ImportCsvDialog failure if the whole suite runs).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/transactions/ web/src/features/accounts/AccountPage.tsx
git commit -m "feat(web): horizon-filtered windows, on-demand search, infinite scroll fetch"
```

### Task 13: Budget dialog month window

**Files:**
- Modify: `web/src/features/budgets/BudgetTransactionsDialog.tsx`

**Interfaces:**
- Consumes: `getTransactionList({ periodStart, periodEnd })` (strict `"Y-m-d H:i:s"` bounds), `queryKeys.transactionPeriod`.

- [ ] **Step 1: Implement**

In `BudgetTransactionsDialog.tsx`, add a month-window query and extend the lookup. `selectedDate` is the budget period start (`YYYY-MM-DD`); derive strict datetime bounds:

```ts
import { useQuery } from '@tanstack/react-query'
import * as transactionApi from '@/api/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'

// [monthStart, nextMonthStart) in the strict wire datetime format
function monthBounds(periodStart: string): { periodStart: string; periodEnd: string } {
  const [y, m] = periodStart.split('-').map(Number)
  const nextY = m === 12 ? y + 1 : y
  const nextM = m === 12 ? 1 : m + 1
  const pad = (n: number) => String(n).padStart(2, '0')
  return {
    periodStart: `${y}-${pad(m)}-01 00:00:00`,
    periodEnd: `${nextY}-${pad(nextM)}-01 00:00:00`,
  }
}
```

Inside the component (after the `useBudgetTransactions` call):

```ts
  // The flat cache holds only windows now; older own rows in this budget month
  // may be outside them. Fetch the month once so editability detection keeps
  // working (not persisted — see queryPersist).
  const { data: monthTransactions } = useQuery({
    queryKey: queryKeys.transactionPeriod(selectedDate),
    queryFn: () => transactionApi.getTransactionList(monthBounds(selectedDate)).then((r) => r.items),
    enabled: element !== null,
    staleTime: TEN_MINUTES,
    gcTime: TEN_MINUTES,
  })
```

And in `toViewTransaction`, change the lookup line:

```ts
    const tx = allTransactions?.find((item) => item.id === wireTx.id) ?? monthTransactions?.find((item) => item.id === wireTx.id)
```

- [ ] **Step 2: Verify**

Run: `cd web && npx tsc --noEmit && pnpm lint`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/budgets/BudgetTransactionsDialog.tsx
git commit -m "fix(web/budget): month-window fetch keeps older own rows editable"
```

### Task 14: SortDialog usage options + server-side sort wiring

**Files:**
- Modify: `web/src/components/SortDialog.tsx`, `web/src/features/classifications/ClassificationList.tsx`, `web/src/features/classifications/queries.ts`, `web/src/features/classifications/CategoriesPage.tsx`, `web/src/features/classifications/PayeesPage.tsx`, `web/src/features/classifications/TagsPage.tsx`, `web/src/locales/en-US.ts`
- Test: `web/src/components/SortDialog.test.tsx` (create)

**Interfaces:**
- Consumes: `SortListForm`, `sortCategoryList`/`sortPayeeList`/`sortTagList` (Task 9).
- Produces: `SortDialog` prop `onPick: (form: SortListForm) => void`; `ClassificationList` prop `onSort: (form: SortListForm) => void` (replaces the dialog's local-sort path; the drag path `onOrder` is unchanged); hooks `useSortCategories()`, `useSortPayees()`, `useSortTags()`.

- [ ] **Step 1: Write the failing SortDialog test**

`web/src/components/SortDialog.test.tsx` (follow `PromptDialog.test.tsx`'s render/i18n setup):

```tsx
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import '@/app/i18n'
import { SortDialog } from './SortDialog'

describe('SortDialog', () => {
  it('emits a name sort', () => {
    const onPick = vi.fn()
    render(<SortDialog open onClose={() => {}} onPick={onPick} />)
    fireEvent.click(screen.getByText('Alphabetically (A-Z)'))
    expect(onPick).toHaveBeenCalledWith({ by: 'name', direction: 'asc' })
  })

  it('emits a usage sort with the selected period', () => {
    const onPick = vi.fn()
    render(<SortDialog open onClose={() => {}} onPick={onPick} />)
    fireEvent.click(screen.getByRole('button', { name: '3' }))
    fireEvent.click(screen.getByText('Most used first'))
    expect(onPick).toHaveBeenCalledWith({ by: 'usage', direction: 'desc', periodMonths: 3 })
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && pnpm test -- run src/components/SortDialog.test.tsx`
Expected: FAIL (labels/props missing).

- [ ] **Step 3: Implement SortDialog**

Replace `web/src/components/SortDialog.tsx`:

```tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { SortListForm } from '@/api/category'

interface SortDialogProps {
  open: boolean
  onClose: () => void
  onPick: (form: SortListForm) => void
}

const USAGE_PERIOD_KEY = 'econumo.sort.usagePeriodMonths'
const PERIODS = [1, 2, 3, 4, 5, 6]

function storedPeriod(): number {
  const raw = Number(localStorage.getItem(USAGE_PERIOD_KEY))
  return PERIODS.includes(raw) ? raw : 3
}

export function SortDialog({ open, onClose, onPick }: SortDialogProps) {
  const { t } = useTranslation()
  const [period, setPeriod] = useState(storedPeriod)

  const pickUsage = (direction: 'asc' | 'desc') => {
    localStorage.setItem(USAGE_PERIOD_KEY, String(period))
    onPick({ by: 'usage', direction, periodMonths: period })
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modals.sort.header')}>
      <div className="flex flex-col gap-2 [&>button]:h-11">
        <Button type="button" variant="secondary" onClick={() => onPick({ by: 'name', direction: 'asc' })}>
          {t('modals.sort.mode.alphabet.asc')}
        </Button>
        <Button type="button" variant="secondary" onClick={() => onPick({ by: 'name', direction: 'desc' })}>
          {t('modals.sort.mode.alphabet.desc')}
        </Button>
        <Button type="button" variant="secondary" onClick={() => pickUsage('desc')}>
          {t('modals.sort.mode.usage.desc')}
        </Button>
        <Button type="button" variant="secondary" onClick={() => pickUsage('asc')}>
          {t('modals.sort.mode.usage.asc')}
        </Button>
        <div className="flex items-center justify-between gap-2 px-1 text-sm text-muted-foreground">
          <span>{t('modals.sort.period')}</span>
          <span className="flex gap-1">
            {PERIODS.map((p) => (
              <Button
                key={p}
                type="button"
                size="sm"
                variant={p === period ? 'default' : 'ghost'}
                onClick={() => setPeriod(p)}
              >
                {p}
              </Button>
            ))}
          </span>
        </div>
        <Button type="button" variant="ghost" onClick={onClose}>
          {t('elements.button.cancel.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
```

Add locale keys in `web/src/locales/en-US.ts` under the existing `'sort'` object (keep the file's quoting style):

```ts
    'sort': {
      'header': 'Sort',
      'period': 'Usage period (months)',
      'mode': {
        'alphabet': {
          'asc': 'Alphabetically (A-Z)',
          'desc': 'Alphabetically (Z-A)'
        },
        'usage': {
          'asc': 'Least used first',
          'desc': 'Most used first'
        }
      }
    },
```

- [ ] **Step 4: Wire ClassificationList + hooks + pages**

`web/src/features/classifications/queries.ts` — add three mutations (next to their order-counterparts, reusing the same metrics):

```ts
export function useSortCategories() {
  const ops = useEntityCacheOps('categories', false)
  return useMutation({
    mutationFn: categoryApi.sortCategoryList,
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.CATEGORY_ORDER_LIST)
    },
  })
}

export function useSortPayees() {
  const ops = useEntityCacheOps('payees', false)
  return useMutation({
    mutationFn: payeeApi.sortPayeeList,
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.PAYEE_ORDER_LIST)
    },
  })
}

export function useSortTags() {
  const ops = useEntityCacheOps('tags', false)
  return useMutation({
    mutationFn: tagApi.sortTagList,
    onSuccess: (items) => {
      ops.replaceAll(items)
      trackEvent(METRICS.TAG_ORDER_LIST)
    },
  })
}
```

(match the tag hook's `useEntityCacheOps('tags', …)` second argument to whatever `useOrderTags` currently passes).

`ClassificationList.tsx` — add the prop and rewire the dialog (drag-and-drop `onOrder` stays untouched):

```ts
// props interface: add
onSort: (form: SortListForm) => void
// import type { SortListForm } from '@/api/category'
```

```tsx
      <SortDialog
        open={sortOpen}
        onClose={() => setSortOpen(false)}
        onPick={(form) => {
          onSort(form)
          setSortOpen(false)
        }}
      />
```

Delete the now-unused local `ordered`/`localeCompare` sorting from the old `onPick`.

Each page adds its hook and prop, e.g. `CategoriesPage.tsx`:

```ts
const sortCategories = useSortCategories()
// ...
onSort={(form) => sortCategories.mutate(form)}
```

(same for `PayeesPage.tsx` with `useSortPayees`, `TagsPage.tsx` with `useSortTags`).

- [ ] **Step 5: Run tests**

Run: `cd web && pnpm test -- run src/components/SortDialog.test.tsx && npx tsc --noEmit && pnpm lint`
Expected: PASS / clean.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/ web/src/features/classifications/ web/src/locales/en-US.ts
git commit -m "feat(web): server-side sort dialog with usage options"
```

### Task 15: Full verification sweep

**Files:** none (verification only)

- [ ] **Step 1: Backend full gate**

Run: `make go-test`
Expected: PASS — build, vet, gofmt, docs-fresh, all tests, coverage ≥ 72%, apiparity guards green, `git status` shows no unexpected golden modifications.

- [ ] **Step 2: Engine comparison (needs Docker or DATABASE_TEST_PGSQL_URL)**

Run: `make test`
Expected: PASS — enginecompare byte-identical on both engines, pgsql repo rerun green, frontend suite green except the pre-existing `ImportCsvDialog.test.tsx` failure (verify it fails identically on `main` before dismissing it).

- [ ] **Step 3: Frontend**

Run: `cd web && pnpm test -- run && pnpm lint && pnpm build`
Expected: tests pass (modulo the known failure), lint clean, production build succeeds.

- [ ] **Step 4: Manual smoke (recommended)**

Run the server (`make go-run`) + SPA dev server (`make web-run`), then verify by hand:
1. Boot: network tab shows ONE `get-transaction-list?perAccountLimit=50` call; localStorage `econumo.query-cache` stays small.
2. Open an account with >50 transactions: scrolling past the window fires `?accountId=…&limit=50&cursor=…` calls; day separators render without gaps.
3. Type in the account search box: one `?accountId=…` full fetch; results match pre-change behavior.
4. Budget → element → transactions dialog: an older own transaction is still editable.
5. Settings → Categories → Sort: usage sort reorders by recent-transaction counts; alphabetical still works; drag-and-drop still works.

- [ ] **Step 5: Final commit / cleanup**

```bash
git status          # nothing unstaged left behind
git log --oneline main..HEAD
```

Then use superpowers:finishing-a-development-branch to merge/PR (this branch already has PR #77 open for the spec — push the implementation commits to the same branch and update the PR description).
