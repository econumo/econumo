# Budget–Account Membership Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the budget account blacklist (`budgets_excluded_accounts`) with explicit, activity-locked membership (`budgets_accounts`) that keeps soft-deleted accounts in the population, so `budgetedBefore` and `spentBefore` are always drawn from the same accounts and phantom "available" can no longer accrue.

**Architecture:** A new `budgets_accounts` table is read by the budget builder without an `is_deleted` filter through a new `AccountLookup.AccountsByIDs` port. Membership can be removed only while the account has no transactions in a closed month of the budget (`started_at ≤ spent_at < first-of-current-month`). Deleting an account auto-zeroes its balance with a "Balance adjustment (account deleted)" correction; the same account-feature use case backs a new `migration:zero-deleted-accounts` CLI command, which the boot migration runner invokes as a **command step** between two SQL migrations (create table → zero deleted accounts → seed membership + drop blacklist). The zero-balance invariant is **self-healing** (spec §5a): creating a transaction against a deleted account is rejected (`transaction.account_deleted`); deleting or editing a transaction touching one re-zeroes the deleted side atomically, with the correction dated at the affected transaction's `spent_at`.

**Tech Stack:** Go 1.x (stdlib `net/http`, `database/sql`), sqlc (sqlite + pgsql), modernc sqlite / pgx, React 19 + Vite + vitest, react-i18next.

**Spec:** `docs/superpowers/specs/2026-08-16-budget-account-membership-design.md`

## Global Constraints

- Branch: `bug/budget-account-membership` (already exists; commit there).
- Two HTTP methods only: `GET` reads, `POST` writes; routes `/api/v1/budget/<action>-<subject>`.
- Frozen wire conventions: datetimes `"2006-01-02 15:04:05"`; JSON arrays are `[]` never `null`; envelope shapes unchanged.
- Features never import features; `internal/infra` and `internal/shared` never import a feature (archtest).
- Every coded error is registered in `errs.AllCodes` AND translated in all 11 catalogues (`locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`); every new `t()` key exists in all 11.
- Goldens (`internal/test/apiparity`, `internal/test/mcpparity`) are regenerated with `UPDATE_GOLDEN=1` and the diff INSPECTED — never hand-edited.
- After changing sqlc queries: `cd internal/infra/storage/sqlc && go generate ./...` (runs `sqlc generate`; sqlc reads the migration SQL files as schema).
- After changing handlers/DTOs: `make swagger` (the OpenAPI-docs-fresh check in `make go-lint` fails otherwise).
- Comments only for non-obvious "why"; no godoc that restates names; no references to the former PHP implementation.
- Commit after every task with the `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` trailer.
- Verification gates: `make go-lint` and `go test ./...` must pass at the end of every task; `make test` (pgsql engine-compare + frontend) at the end of Task 12.

---

## File Map

| Area | Files |
|---|---|
| Migration runner command steps | `internal/infra/storage/migrate/migrate.go`, `migrate_test.go`; `internal/infra/storage/backend/backend.go`; `internal/infra/storage/migrations/embed.go` (+ new `commands.go`, `commands_test.go`); every `migrate.Run` call site (listed in Task 1) |
| Account zeroing | `internal/infra/storage/sqlc/query/{sqlite,pgsql}/accounts.sql`; `internal/account/repository.go`, `repo/repo.go`, `repo/engine.go` (or wherever the account querier adapter lives); `internal/account/zero.go` (new), `delete.go`; tests in `internal/account/api/` |
| CLI | `internal/cli/container.go`, `cli.go`, new `migration_commands.go`, `data_commands.go`, `commands_test.go`; `cmd/econumo/main.go`; `CLAUDE.md` CLI list |
| Schema + repo | `internal/infra/storage/migrations/{sqlite,pgsql}/20260817000000.sql`, `…/20260817000002.sql`; `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql`; `internal/budget/repo/{engine.go,repo.go}`; `internal/budget/repository.go`; `internal/model/budget.go` (BudgetAccount); `internal/test/fixture/entities.go`; migration test `internal/infra/storage/migrations/migration_20260817_test.go` |
| Read model | `internal/budget/readmodel.go`, `internal/budget/repo/read.go` (+ test) |
| Port + glue | `internal/budget/ports.go`, `internal/server/glue_budget.go`, `glue_budget_test.go`, `internal/model/budget_view.go` |
| DTOs / errors / i18n | `internal/model/budget_dto.go`, `internal/shared/errs/codes.go`, `locales/*.json` |
| Budget service + API | `internal/budget/{usecase.go,builder.go,accounts.go,crud.go,create.go,accesssvc.go}`, `internal/budget/api/{budget_more.go,routes.go}`, tests in `internal/budget/api/` |
| MCP | `internal/budget/mcp/mcp.go`, `internal/test/mcpparity/catalogue.go` + goldens |
| Self-healing zero (§5a) | `internal/transaction/{create,update,delete}.go`, `ports.go`, `usecase.go`; `internal/account/zero.go`; `internal/server/glue_transaction_zeroer.go`; `internal/shared/errs/codes.go`; `locales/*.json`; tests in `internal/transaction/api/` |
| apiparity | `internal/test/apiparity/{fixture.go,catalogue_budget_access.go,catalogue_budget_plan.go,catalogue_budget_income.go,…}` + goldens |
| SPA | `web/src/api/budget.ts`, `web/src/api/dto/budget.ts`, `web/src/features/budgets/{BudgetAccountsField,BudgetDialog,BudgetUpdateDialog}.tsx` (+ tests), `web/src/features/accounts/queries.ts` |

---

### Task 1: Migration runner — command steps

**Files:**
- Modify: `internal/infra/storage/migrate/migrate.go`
- Modify: `internal/infra/storage/migrate/migrate_test.go`
- Modify: `internal/infra/storage/backend/backend.go:46-49`
- Modify: `internal/infra/storage/migrations/embed.go`
- Create: `internal/infra/storage/migrations/commands.go`, `internal/infra/storage/migrations/commands_test.go`
- Modify (add `migrate.WithCommandRunner(migrate.NoCommands)`): `internal/test/dbtest/db.go:80`, `internal/test/dbtest/pgsql.go:71`, `internal/cli/commands_test.go:43`, `internal/infra/storage/migrations/{email_verified_backfill_test.go,migration_20260730_test.go,migration_20260802_test.go,migration_20260803_test.go}`, `internal/{budget,category,label,payee,tag,recurring,transaction,user}/api/harness_test.go`
- Modify (copy `Command` when mapping): `cmd/econumo/main.go:321` (`toMigrateMigrations`), `internal/cli/data_commands.go:100` (`toMigratePg`), `internal/test/dbtest/{db.go,pgsql.go}` (`toMigrations*`), every harness `toMigrations`, migration tests' `toRunList`/`runUpTo`/`runAll`

**Interfaces:**
- Produces: `migrate.Migration{Version, SQL, Command string}`; `migrate.CommandRunner func(ctx context.Context, name string) error`; `migrate.WithCommandRunner(r CommandRunner) Option`; `migrate.NoCommands CommandRunner`; `migrate.Run(ctx, db, migs []Migration, opts ...Option) error`; `backend.Migration{Version, Up, Command string}`; `migrations.File{Version, SQL, Command string}`; `migrations.RegisterCommand(version, name string)` (package-level registry consumed by `load`).

- [ ] **Step 1: Write the failing runner tests**

Append to `internal/infra/storage/migrate/migrate_test.go`:

```go
func TestRun_CommandStep_RunsInOrderAndIsRecordedOnce(t *testing.T) {
	db := openMemory(t)
	var calls []string
	runner := func(ctx context.Context, name string) error {
		calls = append(calls, name)
		return nil
	}
	migs := []Migration{
		{Version: "0001", SQL: "CREATE TABLE a (id INTEGER)"},
		{Version: "0002", Command: "migration:demo"},
		{Version: "0003", SQL: "INSERT INTO a (id) VALUES (1)"},
	}
	if err := Run(context.Background(), db, migs, WithCommandRunner(runner)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Join(calls, ",") != "migration:demo" {
		t.Fatalf("calls = %v, want [migration:demo]", calls)
	}
	if got := appliedSet(t, db); !got["0002"] || !got["0003"] {
		t.Fatalf("applied = %v, want 0001..0003", got)
	}
	if err := Run(context.Background(), db, migs, WithCommandRunner(runner)); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("command re-ran on second Run: %v", calls)
	}
}

func TestRun_CommandStep_FailureRecordsNothing(t *testing.T) {
	db := openMemory(t)
	boom := func(ctx context.Context, name string) error { return errors.New("boom") }
	migs := []Migration{{Version: "0001", Command: "migration:demo"}}
	err := Run(context.Background(), db, migs, WithCommandRunner(boom))
	if err == nil || !strings.Contains(err.Error(), `apply "0001"`) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want apply-wrapped boom", err)
	}
	if got := appliedSet(t, db); got["0001"] {
		t.Fatalf("failed command step was recorded: %v", got)
	}
}

func TestRun_CommandStep_WithoutRunnerFails(t *testing.T) {
	db := openMemory(t)
	migs := []Migration{{Version: "0001", Command: "migration:demo"}}
	err := Run(context.Background(), db, migs)
	if err == nil || !strings.Contains(err.Error(), "no command runner") {
		t.Fatalf("err = %v, want no-command-runner error", err)
	}
}

func TestRun_CommandStep_NoCommandsRecordsWithoutRunning(t *testing.T) {
	db := openMemory(t)
	migs := []Migration{{Version: "0001", Command: "migration:demo"}}
	if err := Run(context.Background(), db, migs, WithCommandRunner(NoCommands)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := appliedSet(t, db); !got["0001"] {
		t.Fatalf("NoCommands must still record the version: %v", got)
	}
}

func TestMigration_SQLAndCommandAreExclusive(t *testing.T) {
	db := openMemory(t)
	migs := []Migration{{Version: "0001", SQL: "SELECT 1", Command: "migration:demo"}}
	if err := Run(context.Background(), db, migs, WithCommandRunner(NoCommands)); err == nil {
		t.Fatal("expected error for a migration with both SQL and Command")
	}
}
```

Add `"errors"` to the test file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/infra/storage/migrate/ -run 'CommandStep|Exclusive' -v`
Expected: compile errors (`Command`, `WithCommandRunner`, `NoCommands` undefined).

- [ ] **Step 3: Implement command steps in the runner**

In `internal/infra/storage/migrate/migrate.go`:

```go
// Migration is a single ordered schema change. Exactly one of SQL / Command is
// set: SQL runs statements in one transaction; Command names a CLI command
// (`migration:<slug>`) the composition root dispatches — a data step that
// needs feature code the infra layer cannot import. A command step gets no
// surrounding transaction (the command manages its own), so it must be
// idempotent; its version is recorded only after it succeeds.
type Migration struct {
	Version string
	SQL     string
	Command string
}

// CommandRunner dispatches a command step by name.
type CommandRunner func(ctx context.Context, name string) error

// NoCommands records command steps without running them — for fresh test
// databases, where a data command has nothing to act on.
var NoCommands CommandRunner = func(context.Context, string) error { return nil }

type Option func(*options)

type options struct{ commands CommandRunner }

func WithCommandRunner(r CommandRunner) Option { return func(o *options) { o.commands = r } }

func Run(ctx context.Context, db *sql.DB, migs []Migration, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	// ... existing body unchanged up to the apply loop ...
	for _, m := range ordered {
		if applied[m.Version] {
			continue
		}
		if m.SQL != "" && m.Command != "" {
			return fmt.Errorf("migrate: apply %q: migration sets both SQL and Command", m.Version)
		}
		var err error
		if m.Command != "" {
			err = applyCommand(ctx, db, m, o.commands)
		} else {
			err = applyMigration(ctx, db, m)
		}
		if err != nil {
			return fmt.Errorf("migrate: apply %q: %w", m.Version, err)
		}
	}
	return nil
}

func applyCommand(ctx context.Context, db *sql.DB, m Migration, run CommandRunner) error {
	if run == nil {
		return fmt.Errorf("command step %q: no command runner configured", m.Command)
	}
	if err := run(ctx, m.Command); err != nil {
		return fmt.Errorf("command %q: %w", m.Command, err)
	}
	return recordVersion(ctx, db, m.Version)
}
```

- [ ] **Step 4: Run the runner tests**

Run: `go test ./internal/infra/storage/migrate/ -v`
Expected: all PASS (existing + new).

- [ ] **Step 5: Thread `Command` through backend and migrations, add the registry**

`internal/infra/storage/backend/backend.go`:

```go
type Migration struct {
	Version string
	Up      string
	Command string
}
```

`internal/infra/storage/migrations/embed.go`: add `Command string` to `File`, and at the end of `load` (before the sort) append the registered command steps:

```go
	for _, c := range commandSteps() {
		out = append(out, File{Version: c.version, Command: c.name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
```

Create `internal/infra/storage/migrations/commands.go`:

```go
package migrations

import "strings"

type commandStep struct{ version, name string }

var registry []commandStep

// RegisterCommand adds a command migration step: at Version, the boot runner
// invokes the CLI command `name` (must be `migration:<slug>`). Called from
// init() next to the SQL files so the ordered list stays in one place.
func RegisterCommand(version, name string) {
	if !strings.HasPrefix(name, "migration:") {
		panic("migrations: command step " + version + " must be a migration:* command, got " + name)
	}
	for _, c := range registry {
		if c.version == version {
			panic("migrations: duplicate command step version " + version)
		}
	}
	registry = append(registry, commandStep{version: version, name: name})
}

func commandSteps() []commandStep { return registry }
```

Update `sqlite.Backend.Migrations()` and `pgsql.Backend.Migrations()` to copy `Command: f.Command`. Update `cmd/econumo/main.go` `toMigrateMigrations` to copy `Command: m.Command`; `internal/cli/data_commands.go` `toMigratePg` likewise; `internal/test/dbtest/{db.go,pgsql.go}` `toMigrations*` likewise; every `toMigrations` in the api harnesses and `toRunList`/`runUpTo`/`runAll`/`before`/`all` builders in the migrations tests likewise (they construct `migrate.Migration{Version: f.Version, SQL: f.SQL}` — add `Command: f.Command`).

Create `internal/infra/storage/migrations/commands_test.go`:

```go
package migrations

import "testing"

func TestRegisterCommand_RejectsNonMigrationPrefixAndDuplicates(t *testing.T) {
	saved := registry
	t.Cleanup(func() { registry = saved })
	registry = nil
	RegisterCommand("99990101000000", "migration:demo")
	mustPanic := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		f()
	}
	mustPanic(func() { RegisterCommand("99990101000001", "data:demo") })
	mustPanic(func() { RegisterCommand("99990101000000", "migration:other") })
}

func TestLoad_MergesCommandStepsInVersionOrder(t *testing.T) {
	saved := registry
	t.Cleanup(func() { registry = saved })
	registry = nil
	RegisterCommand("20000101000000", "migration:demo")
	files := SQLite()
	found := -1
	for i, f := range files {
		if f.Command == "migration:demo" {
			found = i
		}
	}
	if found < 0 {
		t.Fatal("command step missing from SQLite() list")
	}
	if found == 0 || files[found-1].Version > files[found].Version {
		t.Fatalf("command step not in version order at %d", found)
	}
}
```

- [ ] **Step 6: Add `WithCommandRunner(migrate.NoCommands)` to every test/dev call site**

Run: `grep -rn "migrate\.Run(" --include=*.go . | grep -v "internal/infra/storage/migrate/\|cmd/econumo/main.go\|internal/cli/data_commands.go"` and append `, migrate.WithCommandRunner(migrate.NoCommands)` to each listed call (dbtest sqlite + pgsql, cli/commands_test.go, the four migration test files, the eight api harnesses). `cmd/econumo/main.go` and `data_commands.go` are wired to the real runner in Task 3; leave them for now (no command step is registered yet, so they still pass).

- [ ] **Step 7: Build, vet, run everything**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(migrate): command steps — migrations may invoke a migration:* CLI command

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Account feature — `ZeroDeletedAccounts` use case and delete-account auto-zero

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/accounts.sql`, `internal/infra/storage/sqlc/query/pgsql/accounts.sql` (+ regenerate)
- Modify: `internal/account/repository.go:17-35` (`AccountStore`), `internal/account/repo/repo.go`, and the account querier adapter (`internal/account/repo/engine.go` or equivalent — `grep -n "ListAvailableAccounts" internal/account/repo/*.go`)
- Create: `internal/account/zero.go`
- Modify: `internal/account/delete.go`
- Test: `internal/account/api/zero_deleted_test.go` (new), `internal/account/api/harness_test.go` (expose `svc`)

**Interfaces:**
- Produces: `AccountStore.ListDeleted(ctx) ([]*model.Account, error)`; `(*account.Service).ZeroDeletedAccounts(ctx) (int, error)` (returns the number of corrections written); unexported `(*Service).zeroBalance(ctx, acct *model.Account, spentAt, now time.Time, description string) (bool, error)`; constant `correctionAccountDeleted = "Balance adjustment (account deleted)"` (alongside `correctionComment` in `usecase.go`, spec §5 — deletion-driven zeroing must be distinguishable from a manual balance edit; Task 9a adds the two §5a siblings).
- Consumes: `BalanceReader.Balance(ctx, accountID, before)`, `AccountStore.SaveCorrection`.

- [ ] **Step 1: sqlc query `ListDeletedAccounts` (both engines)**

Append to `query/sqlite/accounts.sql`:

```sql
-- name: ListDeletedAccounts :many
SELECT id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at
FROM accounts WHERE is_deleted = 1 ORDER BY id;
```

Same in `query/pgsql/accounts.sql` (pgsql `is_deleted` is BOOLEAN: `WHERE is_deleted = TRUE`). Match the exact column list `GetAccountByID` selects (open the file and copy it). Run: `cd internal/infra/storage/sqlc && go generate ./... && cd -`.

- [ ] **Step 2: Repo + interface**

Add `ListDeleted(ctx context.Context) ([]*model.Account, error)` to `AccountStore` in `internal/account/repository.go`; add `ListDeletedAccounts` to the account querier interface with sqlite/pgsql adapters (same pattern as `ListAvailableAccounts` in the account repo's engine adapter); implement in `repo.go`:

```go
func (r *Repo) ListDeleted(ctx context.Context) ([]*model.Account, error) {
	rows, err := r.q.ListDeletedAccounts(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]*model.Account, 0, len(rows))
	for _, row := range rows {
		a, herr := hydrateAccount(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, a)
	}
	return out, nil
}
```

Run: `go build ./...` — fix any stub types (e.g. fake AccountStores in tests) until it compiles.

- [ ] **Step 3: Failing tests for the use case and delete auto-zero**

In `internal/account/api/harness_test.go` add `svc *appaccount.Service` to `harness` and set it in `newHarnessWithClock`. Create `internal/account/api/zero_deleted_test.go`:

```go
package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	zeroAcctPos = "acc00000-0000-0000-0000-00000000aa01"
	zeroAcctNeg = "acc00000-0000-0000-0000-00000000aa02"
	zeroAcctNil = "acc00000-0000-0000-0000-00000000aa03"
)

type corrRow struct {
	typ         int
	amount      string
	spentAt     string
	description string
}

func corrections(t *testing.T, h *harness, accountID string) []corrRow {
	t.Helper()
	rows, err := h.db.Query(`SELECT type, amount, spent_at, description FROM transactions WHERE account_id = ? AND category_id IS NULL ORDER BY created_at`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []corrRow
	for rows.Next() {
		var r corrRow
		if err := rows.Scan(&r.typ, &r.amount, &r.spentAt, &r.description); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestZeroDeletedAccounts_WritesOneCorrectionPerNonZeroDeletedAccount(t *testing.T) {
	h := newHarness(t)
	// +12.5 (income), -3 (expense), and a zero-balance deleted account.
	h.f.Account(fixture.Account{ID: zeroAcctPos, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 1, Amount: "12.5", SpentAt: "2026-01-10 00:00:00"})
	h.f.Account(fixture.Account{ID: zeroAcctNeg, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctNeg, Type: 0, Amount: "3", SpentAt: "2026-01-10 00:00:00"})
	h.f.Account(fixture.Account{ID: zeroAcctNil, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	// a live account with a balance must be untouched
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: seedAccountID, Type: 1, Amount: "100", SpentAt: "2026-01-10 00:00:00"})

	n, err := h.svc.ZeroDeletedAccounts(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("ZeroDeletedAccounts = %d, %v; want 2, nil", n, err)
	}
	var updatedAt string
	if err := h.db.QueryRow(`SELECT updated_at FROM accounts WHERE id = ?`, zeroAcctPos).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	pos := corrections(t, h, zeroAcctPos)
	if len(pos) != 1 || pos[0].typ != 0 || pos[0].amount != "12.5" || pos[0].description != "Balance adjustment (account deleted)" || pos[0].spentAt != updatedAt {
		t.Fatalf("positive: %+v (updated_at=%s)", pos, updatedAt)
	}
	neg := corrections(t, h, zeroAcctNeg)
	if len(neg) != 1 || neg[0].typ != 1 || neg[0].amount != "3" {
		t.Fatalf("negative: %+v", neg)
	}
	if len(corrections(t, h, zeroAcctNil)) != 0 {
		t.Fatal("zero-balance account got a correction")
	}
	if len(corrections(t, h, seedAccountID)) != 0 {
		t.Fatal("live account got a correction")
	}
	// idempotent
	n, err = h.svc.ZeroDeletedAccounts(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("second run = %d, %v; want 0", n, err)
	}
}

func TestZeroDeletedAccounts_IgnoresSubScaleResidue(t *testing.T) {
	h := newHarness(t)
	h.f.Account(fixture.Account{ID: zeroAcctPos, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	// 0.1 + 0.2 - 0.3 is a float residue in SQLite's SUM; rounded to 8 dp it is 0.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 1, Amount: "0.1", SpentAt: "2026-01-10 00:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 1, Amount: "0.2", SpentAt: "2026-01-10 00:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: zeroAcctPos, Type: 0, Amount: "0.3", SpentAt: "2026-01-10 00:00:00"})
	n, err := h.svc.ZeroDeletedAccounts(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("residue produced a correction: n=%d err=%v", n, err)
	}
}

func TestDeleteAccount_AutoZeroesBalanceThenSoftDeletes(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: seedAccountID, Type: 1, Amount: "40", SpentAt: "2026-01-10 00:00:00"})
	status, env := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": seedAccountID})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, env.raw)
	}
	var deleted int
	if err := h.db.QueryRow(`SELECT is_deleted FROM accounts WHERE id = ?`, seedAccountID).Scan(&deleted); err != nil || deleted != 1 {
		t.Fatalf("is_deleted=%d err=%v", deleted, err)
	}
	c := corrections(t, h, seedAccountID)
	if len(c) != 1 || c[0].typ != 0 || c[0].amount != "40" || c[0].description != "Balance adjustment (account deleted)" {
		t.Fatalf("corrections=%+v", c)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", c[0].spentAt); err != nil {
		t.Fatalf("spent_at layout: %v", err)
	}
}

func TestDeleteAccount_ZeroBalanceWritesNoCorrection(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	if st, env := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": seedAccountID}); st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	if len(corrections(t, h, seedAccountID)) != 0 {
		t.Fatal("unexpected correction")
	}
}
```

Use the harness's existing seeded owner account constant (open `harness_test.go`; if the seeded account is created per test, seed one at the top of each test with `h.f.Account(...)` + folder membership as `first_account_folder_test.go` does, and name it `seedAccountID`). Note the harness fixture uses `f.Account` — check whether `Deleted: true` sets `updated_at` (it uses `b.now()`); the assertion compares to the stored value so either is fine.

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/account/api/ -run 'ZeroDeleted|DeleteAccount_' -v`
Expected: FAIL (`ZeroDeletedAccounts` undefined; delete writes no correction).

- [ ] **Step 5: Implement**

In `internal/account/usecase.go`, next to `correctionComment`, add:

```go
// correctionAccountDeleted marks corrections written because the account was
// deleted (delete-account auto-zero, migration:zero-deleted-accounts, and the
// §5a self-healing re-zero glue), distinguishable from a manual balance edit.
const correctionAccountDeleted = "Balance adjustment (account deleted)"
```

Create `internal/account/zero.go`:

```go
package account

import (
	"context"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// allTimeBalanceBound is the exclusive spent_at bound that includes every
// transaction, future-dated ones included: a deleted account must end at
// exactly zero, not "zero as of today".
var allTimeBalanceBound = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

// balanceScale is the transactions.amount NUMERIC(19,8) scale; a residue below
// it cannot be expressed by any transaction, so it never warrants a correction.
const balanceScale = 8

// ZeroDeletedAccounts writes a balance-correction transaction for every
// soft-deleted account whose all-time balance is non-zero, dated at the
// account's deletion time (UpdatedAt). Idempotent; returns the number written.
func (s *Service) ZeroDeletedAccounts(ctx context.Context) (int, error) {
	accounts, err := s.accounts.ListDeleted(ctx)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, acct := range accounts {
		var wrote bool
		err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
			var zerr error
			wrote, zerr = s.zeroBalance(txCtx, acct, acct.UpdatedAt, s.clock.Now(), correctionAccountDeleted)
			return zerr
		})
		if err != nil {
			return written, err
		}
		if wrote {
			written++
		}
	}
	return written, nil
}

// zeroBalance reconciles acct to a zero balance with one correction dated
// spentAt (the same sign rule as update-account: a positive balance is removed
// by an EXPENSE, a negative one filled by an INCOME). The description says
// which flow zeroed it (account deletion vs a §5a self-healing write).
// Returns whether a row was written.
func (s *Service) zeroBalance(ctx context.Context, acct *model.Account, spentAt, now time.Time, description string) (bool, error) {
	raw, err := s.balances.Balance(ctx, acct.ID, allTimeBalanceBound)
	if err != nil {
		return false, err
	}
	balance := vo.NewDecimal(raw).Round(balanceScale)
	if balance.IsZero() {
		return false, nil
	}
	corrType := int16(0) // expense
	if balance.IsNegative() {
		corrType = 1 // income
	}
	corr := model.AccountCorrection{
		ID:          s.accounts.NextIdentity(),
		UserID:      acct.UserID,
		AccountID:   acct.ID,
		Description: description,
		Type:        corrType,
		Amount:      balance.Abs().String(),
		SpentAt:     spentAt,
		CreatedAt:   now,
	}
	if err := s.accounts.SaveCorrection(ctx, corr); err != nil {
		return false, err
	}
	slog.InfoContext(ctx, "zero-account-balance", "account_id", acct.ID.String(), "amount", corr.Amount, "type", corrType)
	return true, nil
}
```

Check `vo.DecimalNumber.Round(precision int)` semantics in `internal/shared/vo/decimal.go:143` (round half-up to `precision` decimals — confirm; if it truncates, that is still fine for a residue check but note it). If `NextIdentity` lives on `AccountStore` under a different name, use that.

In `internal/account/delete.go`, owner branch:

```go
	if acct.UserID.Equal(userID) {
		if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			now := s.clock.Now()
			// A deleted account must not carry a balance forward (it stays a
			// budget member and its history keeps counting), so reconcile it
			// to zero first — dated in the caller's local time like update-account.
			if _, err := s.zeroBalance(ctx, acct, s.localNow(ctx), now, correctionAccountDeleted); err != nil {
				return err
			}
			acct.Delete(now)
			return s.accounts.Save(ctx, acct)
		}); err != nil {
			return nil, err
		}
		return &model.DeleteAccountResult{}, nil
	}
```

(`s.localNow(ctx)` is the helper create.go uses for `spentAt`; confirm its name with `grep -n "func (s \*Service) localNow" internal/account/*.go`.)

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/account/... -v -run 'ZeroDeleted|DeleteAccount'` then `go test ./internal/account/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(account): ZeroDeletedAccounts use case; delete-account auto-zeroes the balance

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: CLI `migration:zero-deleted-accounts` + runner wiring + step registration

**Files:**
- Modify: `internal/cli/container.go`, `internal/cli/cli.go:23-30`
- Create: `internal/cli/migration_commands.go`
- Modify: `internal/cli/data_commands.go:79` (pass runner), `cmd/econumo/main.go:216` (pass runner)
- Modify: `internal/infra/storage/migrations/commands.go` (register `20260817000001`)
- Modify: `internal/cli/commands_test.go`
- Modify: `CLAUDE.md` (CLI list + a note under Database on command steps), `docs/run-without-docker.md` only if it lists commands (`grep -n "token:purge" docs README.md`)

**Interfaces:**
- Produces: `cli.MigrationCommandRunner(cfg config.Config, db *sql.DB) migrate.CommandRunner` (exported; builds the container lazily on first call, dispatches only `migration:*` names, never closes `db`); container field `account *appaccount.Service`; command `migration:zero-deleted-accounts` (prints `zeroed N account(s)`).
- Consumes: `(*account.Service).ZeroDeletedAccounts` (Task 2), `migrations.RegisterCommand` (Task 1).

- [ ] **Step 1: Failing CLI test**

`internal/cli/commands_test.go` drives commands through `cliEnv(t)` (a migrated
sqlite file behind `DATABASE_URL`) + `Run(args)`. Add, in the same file:

```go
func TestMigrationZeroDeletedAccounts_CommandAndRunner(t *testing.T) {
	cliEnv(t)
	ctx := context.Background()
	c, err := newContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer c.Close()
	// seed straight through the container's db: a deleted account holding +5
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := c.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES ('u1','u1','u1@e.test','U','','x','','2026-01-01 00:00:00','2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES ('acc-del','dffc2a06-6f29-4704-8575-31709adee926','u1','A',2,'wallet',1,'2026-01-01 00:00:00','2026-02-01 00:00:00')`)
	mustExec(`INSERT INTO transactions (id, user_id, account_id, description, type, amount, spent_at, created_at, updated_at) VALUES ('t1','u1','acc-del','',1,5,'2026-01-10 00:00:00','2026-01-10 00:00:00','2026-01-10 00:00:00')`)

	if code := Run([]string{"migration:zero-deleted-accounts"}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	var n int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE account_id = 'acc-del' AND type = 0 AND amount = 5`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("corrections=%d err=%v", n, err)
	}
	// second run is a no-op
	if code := Run([]string{"migration:zero-deleted-accounts"}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE account_id = 'acc-del'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("rows after second run=%d err=%v (want 2: original + one correction)", n, err)
	}

	// the runner dispatches by name over a caller-owned db and refuses non-migration commands
	runner := MigrationCommandRunner(c.cfg, c.db)
	if err := runner(ctx, "migration:zero-deleted-accounts"); err != nil {
		t.Fatalf("runner: %v", err)
	}
	if err := runner(ctx, "user:create"); err == nil {
		t.Fatal("runner must refuse non-migration commands")
	}
	if err := runner(ctx, "migration:nope"); err == nil {
		t.Fatal("runner must fail on an unknown command")
	}
}
```

(`accounts.id`/`users.id` here are not UUIDs — the command lists accounts through the repo, whose `hydrateAccount` calls `vo.ParseId`; if that rejects non-UUID ids, use `vo.NewId().String()` values instead. USD's id `dffc2a06-…` is seeded by the baseline migration.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cli/ -run MigrationZeroDeleted -v` → FAIL (command/runner undefined).

- [ ] **Step 3: Implement**

`internal/cli/container.go`: split `newContainer` into `newContainer(ctx)` (loads config, opens the DB, calls `newContainerFor`) and

```go
// newContainerFor wires the services over an already-open database. Used by
// newContainer and by the migration command runner, which shares serve's
// connection and must not close it.
func newContainerFor(cfg config.Config, db *sql.DB) *container
```

Move everything after `be.Open` into `newContainerFor`. Add the account service to the container (mirror `internal/server/server.go:259-270` — `accountrepo.NewRepo/NewFolderRepo/NewAccessRepo`, `server.NewAccountCurrencyLookup(currencyLookup)`, `server.NewUserOwnerLookup(userRepo)`, `connectionrepo.NewAccountAccessResolver(connectionrepo.NewRepo(...))`, `operationrepo.NewGuard(cfg.DatabaseDriver, txm)`, `clk`) as `account *appaccount.Service`. Add `owned bool` (true when the container opened the DB) and make `Close` a no-op when `!owned`.

Create `internal/cli/migration_commands.go`:

```go
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
)

// migrationCommands are data migrations the boot runner invokes as command
// steps (internal/infra/storage/migrations/commands.go). Every one must be
// idempotent — the runner records the version only after success, and an
// operator may re-run them by hand.
func migrationCommands() []command {
	return []command{
		{
			name:    "migration:zero-deleted-accounts",
			summary: "write a 'Balance adjustment (account deleted)' correction so every deleted account's balance is zero (idempotent)",
			run: func(ctx context.Context, c *container, args []string) error {
				n, err := c.account.ZeroDeletedAccounts(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("zeroed %d account(s)\n", n)
				return nil
			},
		},
	}
}

// MigrationCommandRunner adapts the CLI registry to migrate.CommandRunner for
// the composition root (serve, data:import-sqlite). The container is built on
// first use over the caller's db and is never closed here.
func MigrationCommandRunner(cfg config.Config, db *sql.DB) migrate.CommandRunner {
	var c *container
	cmds := index(migrationCommands())
	return func(ctx context.Context, name string) error {
		if !strings.HasPrefix(name, "migration:") {
			return fmt.Errorf("cli: %q is not a migration command", name)
		}
		cmd, ok := cmds[name]
		if !ok {
			return fmt.Errorf("cli: unknown migration command %q", name)
		}
		if c == nil {
			c = newContainerFor(cfg, db)
		}
		logCommandStart(name, nil)
		return cmd.run(ctx, c, nil)
	}
}
```

Add `cs = append(cs, migrationCommands()...)` to `commandList()`. In `data_commands.go` (`data:import-sqlite`) change the call to `migrate.Run(ctx, c.db, toMigratePg(migrations.Pgsql()), migrate.WithCommandRunner(MigrationCommandRunner(c.cfg, c.db)))`. In `cmd/econumo/main.go` change to `migrate.Run(ctx, db, toMigrateMigrations(be.Migrations()), migrate.WithCommandRunner(cli.MigrationCommandRunner(cfg, db)))`.

Register the step — in `internal/infra/storage/migrations/commands.go` add:

```go
func init() {
	// 20260817000000.sql creates budgets_accounts; this step zeroes every
	// deleted account; 20260817000002.sql seeds membership and drops the
	// blacklist. See docs/superpowers/specs/2026-08-16-budget-account-membership-design.md §2.
	RegisterCommand("20260817000001", "migration:zero-deleted-accounts")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/cli/ ./internal/infra/storage/... ./cmd/...` → PASS. Run `go test ./...` — every DB-backed suite still passes because tests use `NoCommands`.

- [ ] **Step 5: Docs**

`CLAUDE.md`: add `migration:zero-deleted-accounts` to the CLI command list with one line: "writes a 'Balance adjustment (account deleted)' correction so every deleted account's balance is zero; idempotent; also invoked automatically at boot as migration step `20260817000001`". Under **Database**, add: "Migrations may also be **command steps** (`migrations.RegisterCommand(version, "migration:<slug>")`): the boot runner invokes the named CLI command in version order between SQL files, records the version only on success, and gives it no surrounding transaction — every `migration:*` command must be idempotent. `serve` and `data:import-sqlite` pass `cli.MigrationCommandRunner`; test databases pass `migrate.NoCommands`." If `docs/run-without-docker.md`/README list the commands, add it there too.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(cli): migration:zero-deleted-accounts; boot runner dispatches command steps via cli.MigrationCommandRunner

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: `budgets_accounts` schema, sqlc queries, repo methods, fixture helper (additive)

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260817000000.sql`, `internal/infra/storage/migrations/pgsql/20260817000000.sql`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql` (+ regenerate)
- Modify: `internal/model/budget.go` (add `BudgetAccount`), `internal/budget/repository.go` (`BudgetStore`), `internal/budget/repo/engine.go`, `internal/budget/repo/repo.go`
- Modify: `internal/test/fixture/entities.go` (add `BudgetAccount`)
- Test: `internal/budget/repo/accounts_test.go` (new)

**Interfaces:**
- Produces: `model.BudgetAccount{AccountID vo.Id; CreatedAt time.Time}`; `BudgetStore.MemberAccounts(ctx, budgetID) ([]model.BudgetAccount, error)`, `AddAccount(ctx, budgetID, accountID vo.Id, now time.Time) error`, `RemoveAccount(ctx, budgetID, accountID vo.Id) error`, `RemoveAccountsOwnedBy(ctx, budgetID, ownerID vo.Id) error`; fixture `(*Builder).BudgetAccount(budgetID, accountID string)`.
- The old `ExcludedAccountIDs/ExcludeAccount/IncludeAccount` stay until Task 9.

- [ ] **Step 1: Migration 000 (both engines)**

`sqlite/20260817000000.sql`:

```sql
-- Budget membership becomes an explicit set (replacing the excluded-accounts
-- blacklist in 20260817000002). A soft-deleted member keeps counting, so
-- budgeted and spent totals are always drawn from the same accounts.
CREATE TABLE budgets_accounts
(
    budget_id  TEXT     NOT NULL
    , account_id TEXT     NOT NULL
    , created_at DATETIME NOT NULL
    , PRIMARY KEY (budget_id, account_id)
    , FOREIGN KEY (budget_id)  REFERENCES budgets (id)  ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);
CREATE INDEX budgets_accounts_account_id_idx ON budgets_accounts (account_id);
```

`pgsql/20260817000000.sql`: same with `TIMESTAMP` instead of `DATETIME` (copy the type the pgsql baseline uses for `budgets_access.created_at`).

- [ ] **Step 2: sqlc queries (both engines)**

Append to `query/sqlite/budgets.sql`:

```sql
-- name: ListBudgetAccounts :many
SELECT account_id, created_at FROM budgets_accounts WHERE budget_id = ? ORDER BY created_at, account_id;

-- name: AddBudgetAccount :exec
INSERT INTO budgets_accounts (budget_id, account_id, created_at) VALUES (?, ?, ?)
ON CONFLICT (budget_id, account_id) DO NOTHING;

-- name: RemoveBudgetAccount :exec
DELETE FROM budgets_accounts WHERE budget_id = ? AND account_id = ?;

-- name: RemoveBudgetAccountsOwnedBy :exec
DELETE FROM budgets_accounts
WHERE budget_id = ? AND account_id IN (SELECT id FROM accounts WHERE user_id = ?);
```

pgsql: same with `$1..$3`. Regenerate: `cd internal/infra/storage/sqlc && go generate ./... && cd -`.

- [ ] **Step 3: Failing repo test**

`internal/budget/repo/accounts_test.go`:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestBudgetAccounts_AddListRemoveAndRemoveByOwner(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u1, u2 := vo.NewId(), vo.NewId()
	f.User(fixture.User{ID: u1.String(), Email: "u1@e.test", Name: "U1", Password: "pw", Salt: "s"})
	f.User(fixture.User{ID: u2.String(), Email: "u2@e.test", Name: "U2", Password: "pw", Salt: "s"})
	b, a1, a2, a3 := vo.NewId(), vo.NewId(), vo.NewId(), vo.NewId()
	f.Account(fixture.Account{ID: a1.String(), UserID: u1.String()})
	f.Account(fixture.Account{ID: a2.String(), UserID: u1.String()})
	f.Account(fixture.Account{ID: a3.String(), UserID: u2.String()})
	f.Budget(fixture.Budget{ID: b.String(), UserID: u1.String()})

	r := budgetrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for i, a := range []vo.Id{a2, a1, a3} {
		if err := r.AddAccount(ctx, b, a, t0.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	// re-adding a member keeps its original created_at
	if err := r.AddAccount(ctx, b, a2, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := r.MemberAccounts(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].AccountID.Equal(a2) || !got[1].AccountID.Equal(a1) || !got[2].AccountID.Equal(a3) {
		t.Fatalf("order/len: %+v", got)
	}
	if !got[0].CreatedAt.Equal(t0) {
		t.Fatalf("created_at refreshed on re-add: %v", got[0].CreatedAt)
	}
	if err := r.RemoveAccountsOwnedBy(ctx, b, u2); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.MemberAccounts(ctx, b); len(got) != 2 {
		t.Fatalf("after RemoveAccountsOwnedBy: %+v", got)
	}
	if err := r.RemoveAccount(ctx, b, a1); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveAccount(ctx, b, a1); err != nil { // non-member: no-op
		t.Fatal(err)
	}
	if got, _ = r.MemberAccounts(ctx, b); len(got) != 1 || !got[0].AccountID.Equal(a2) {
		t.Fatalf("after RemoveAccount: %+v", got)
	}
}
```

(`fixture.User` requires the same fields the api harnesses pass — open `internal/test/fixture/entities.go` `User` and copy the required set. If `dbtest.New` returns `Engine`/`TX` under other names, adapt.)

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/budget/repo/ -run BudgetAccounts -v` → FAIL (methods undefined).

- [ ] **Step 5: Implement model, interface, engine adapter, repo**

`internal/model/budget.go` (near `BudgetAccess`):

```go
// BudgetAccount is a budget membership row: the account's transactions and
// balances count in the budget, from the budget's start, deleted or not.
type BudgetAccount struct {
	AccountID vo.Id
	CreatedAt time.Time
}
```

`internal/budget/repository.go` `BudgetStore` — add:

```go
	MemberAccounts(ctx context.Context, budgetID vo.Id) ([]BudgetAccount, error) // model.BudgetAccount
	AddAccount(ctx context.Context, budgetID, accountID vo.Id, now time.Time) error
	RemoveAccount(ctx context.Context, budgetID, accountID vo.Id) error
	// RemoveAccountsOwnedBy drops every membership row for accounts owned by
	// ownerID — a departing participant takes their accounts with them.
	RemoveAccountsOwnedBy(ctx context.Context, budgetID, ownerID vo.Id) error
```

`internal/budget/repo/engine.go`: add to `querier` (canonical sqlite types — `sqlitegen.ListBudgetAccountsRow`, `sqlitegen.AddBudgetAccountParams`, …) and both adapters:

```go
	ListBudgetAccounts(ctx context.Context, db backend.DBTX, budgetID string) ([]sqlitegen.ListBudgetAccountsRow, error)
	AddBudgetAccount(ctx context.Context, db backend.DBTX, p sqlitegen.AddBudgetAccountParams) error
	RemoveBudgetAccount(ctx context.Context, db backend.DBTX, budgetID, accountID string) error
	RemoveBudgetAccountsOwnedBy(ctx context.Context, db backend.DBTX, budgetID, userID string) error
```

The pgsql adapter converts `pgsqlgen.ListBudgetAccountsRow` → `sqlitegen.ListBudgetAccountsRow` field by field (`AccountID`, `CreatedAt`), and `sqlitegen.AddBudgetAccountParams` → `pgsqlgen.AddBudgetAccountParams`. Follow the neighbouring access-row conversions exactly.

`internal/budget/repo/repo.go`:

```go
func (r *Repo) MemberAccounts(ctx context.Context, budgetID vo.Id) ([]model.BudgetAccount, error) {
	rows, err := r.q.ListBudgetAccounts(ctx, r.db(ctx), budgetID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.BudgetAccount, 0, len(rows))
	for _, row := range rows {
		id, perr := vo.ParseId(row.AccountID)
		if perr != nil {
			return nil, perr
		}
		out = append(out, model.BudgetAccount{AccountID: id, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (r *Repo) AddAccount(ctx context.Context, budgetID, accountID vo.Id, now time.Time) error {
	return r.q.AddBudgetAccount(ctx, r.db(ctx), sqlitegen.AddBudgetAccountParams{BudgetID: budgetID.String(), AccountID: accountID.String(), CreatedAt: now})
}

func (r *Repo) RemoveAccount(ctx context.Context, budgetID, accountID vo.Id) error {
	return r.q.RemoveBudgetAccount(ctx, r.db(ctx), budgetID.String(), accountID.String())
}

func (r *Repo) RemoveAccountsOwnedBy(ctx context.Context, budgetID, ownerID vo.Id) error {
	return r.q.RemoveBudgetAccountsOwnedBy(ctx, r.db(ctx), budgetID.String(), ownerID.String())
}
```

If the sqlite generated `CreatedAt` is `time.Time` and pgsql's is `time.Time` too, no conversion is needed; if either is a string, parse with `datetime.Layout` the way `hydrateAccess` does.

`internal/test/fixture/entities.go`:

```go
// BudgetAccount adds an account to a budget's member set.
func (b *Builder) BudgetAccount(budgetID, accountID string) {
	b.t.Helper()
	b.insert(`INSERT INTO budgets_accounts (budget_id, account_id, created_at) VALUES (?, ?, ?)`, budgetID, accountID, b.now())
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/budget/... ./internal/test/... ` → PASS (repo test + everything still on the old path).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(budget): budgets_accounts table + repo membership methods (additive)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Read model — `AccountsWithTransactions`

**Files:**
- Modify: `internal/budget/readmodel.go`, `internal/budget/repo/read.go`
- Test: `internal/budget/repo/read_accounts_test.go` (new)

**Interfaces:**
- Produces: `ReadModel.AccountsWithTransactions(ctx, accountIDs []vo.Id, start, end time.Time) ([]vo.Id, error)` — subset of `accountIDs` that appear as `account_id` OR `account_recipient_id` on a transaction with `start <= spent_at < end`; empty input → `nil, nil`; result order = input order.

- [ ] **Step 1: Failing test**

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestAccountsWithTransactions(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId().String()
	f.User(fixture.User{ID: u, Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	src, dst, quiet, early := vo.NewId(), vo.NewId(), vo.NewId(), vo.NewId()
	for _, a := range []vo.Id{src, dst, quiet, early} {
		f.Account(fixture.Account{ID: a.String(), UserID: u})
	}
	f.Transaction(fixture.Transaction{UserID: u, AccountID: src.String(), Type: 2, Amount: "10", AccountRecipientID: dst.String(), AmountRecipient: "10", SpentAt: "2026-06-15 12:00:00"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: early.String(), Type: 0, Amount: "1", SpentAt: "2026-05-31 23:59:59"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: quiet.String(), Type: 0, Amount: "1", SpentAt: "2026-08-01 00:00:00"}) // == end, excluded

	r := budgetrepo.NewReadRepo(db.Engine, db.TX)
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	got, err := r.AccountsWithTransactions(context.Background(), []vo.Id{quiet, dst, src, early}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Equal(dst) || !got[1].Equal(src) {
		t.Fatalf("got %v want [dst src] (input order)", got)
	}
	if got, _ := r.AccountsWithTransactions(context.Background(), nil, start, end); got != nil {
		t.Fatal("empty input must return nil")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/budget/repo/ -run AccountsWithTransactions` → FAIL.

- [ ] **Step 3: Implement**

`readmodel.go` (add to the interface with a comment):

```go
	// AccountsWithTransactions returns the subset of accountIDs (input order)
	// that appear as source or transfer recipient on any transaction with
	// start <= spent_at < end. Drives the membership removal rule.
	AccountsWithTransactions(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]vo.Id, error)
```

`read.go`:

```go
func (r *ReadRepo) AccountsWithTransactions(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]vo.Id, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	ids := idArgs(accountIDs)
	var sql string
	var args []any
	if r.driver == "postgresql" {
		in1 := r.ph(3, len(ids))
		in2 := r.ph(3+len(ids), len(ids))
		sql = "SELECT account_id FROM transactions WHERE spent_at >= $1 AND spent_at < $2 AND account_id IN (" + in1 + ")" +
			" UNION SELECT account_recipient_id FROM transactions WHERE spent_at >= $1 AND spent_at < $2 AND account_recipient_id IN (" + in2 + ")"
		args = append(append([]any{start, end}, ids...), ids...)
	} else {
		in := r.ph(1, len(ids))
		sql = "SELECT account_id FROM transactions WHERE spent_at >= ? AND spent_at < ? AND account_id IN (" + in + ")" +
			" UNION SELECT account_recipient_id FROM transactions WHERE spent_at >= ? AND spent_at < ? AND account_recipient_id IN (" + in + ")"
		ds, de := sqliteDatetime(start), sqliteDatetime(end)
		args = append(append(append([]any{ds, de}, ids...), ds, de), ids...)
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	active := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		active[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []vo.Id
	for _, id := range accountIDs {
		if active[id.String()] {
			out = append(out, id)
		}
	}
	return out, nil
}
```

Any fake `ReadModel` in tests (`grep -rn "AccountsBalancesOnDate(" internal --include=*_test.go -l`) gets a stub method.

- [ ] **Step 4: Run** — `go test ./internal/budget/... ` → PASS. Also `DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/budget/repo/ -run AccountsWithTransactions` if a Postgres is available (`DATABASE_TEST_PGSQL_URL`); otherwise Task 12 covers it.

- [ ] **Step 5: Commit** — `git commit -am "feat(budget): AccountsWithTransactions read query"` (with trailer).

---

### Task 6: Port `AccountsByIDs` (replaces `AccountsForOwners`) + glue

**Files:**
- Modify: `internal/model/budget_view.go:137-141` (`AccountView.IsDeleted`), `internal/budget/ports.go:36-39`, `internal/server/glue_budget.go:35-85`, `internal/server/glue_budget_test.go`
- Modify: `internal/budget/builder.go` only enough to compile (temporary — Task 8 rewrites `buildFilters`); see Step 3.

**Interfaces:**
- Produces: `AccountLookup.AccountsByIDs(ctx, ids []vo.Id) ([]model.AccountView, error)` — input order, no `is_deleted` filter, `AccountView{ID, CurrencyID, OwnerID string; IsDeleted bool}`; an unknown id → the account repo's `*errs.NotFoundError` (fail loudly; a hard-deleted account cascades its membership row so it cannot be asked for).
- Removes: `AccountsForOwners`.

- [ ] **Step 1: Failing glue test** (`internal/server/glue_budget_test.go`, alongside the existing `AccountsForOwners` test which is deleted):

```go
func TestBudgetAccountLookup_AccountsByIDs_IncludesDeletedInInputOrder(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId().String()
	f.User(fixture.User{ID: u, Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	live, dead := vo.NewId(), vo.NewId()
	f.Account(fixture.Account{ID: live.String(), UserID: u})
	f.Account(fixture.Account{ID: dead.String(), UserID: u, Deleted: true})
	l := server.NewBudgetAccountLookup(accountrepo.NewRepo(db.Engine, db.TX))
	got, err := l.AccountsByIDs(context.Background(), []vo.Id{dead, live})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != dead.String() || !got[0].IsDeleted || got[1].ID != live.String() || got[1].IsDeleted || got[0].OwnerID != u {
		t.Fatalf("got %+v", got)
	}
	if _, err := l.AccountsByIDs(context.Background(), []vo.Id{vo.NewId()}); err == nil {
		t.Fatal("unknown id must error")
	}
}
```

- [ ] **Step 2: Run** → FAIL (method undefined).

- [ ] **Step 3: Implement**

`budget_view.go`: add `IsDeleted bool` to `AccountView`. `ports.go`:

```go
// AccountLookup resolves the budget's member accounts + ownership. AccountsByIDs
// deliberately returns soft-deleted accounts too: a deleted member keeps
// counting in the budget.
type AccountLookup interface {
	AccountsByIDs(ctx context.Context, ids []vo.Id) ([]model.AccountView, error)
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
}
```

`glue_budget.go`: `budgetAccountRepo` keeps only `GetByID`; replace `AccountsForOwners` with:

```go
func (l *BudgetAccountLookup) AccountsByIDs(ctx context.Context, ids []vo.Id) ([]model.AccountView, error) {
	out := make([]model.AccountView, 0, len(ids))
	for _, id := range ids {
		a, err := l.accounts.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, model.AccountView{
			ID: a.ID.String(), CurrencyID: a.CurrencyID.String(), OwnerID: a.UserID.String(), IsDeleted: a.IsDeleted,
		})
	}
	return out, nil
}
```

Temporary compile bridge in `builder.go` `buildFilters` (replaced in Task 8): the call `s.accounts.AccountsForOwners(ctx, userIDs)` no longer exists — replace with the membership read that Task 8 finalises:

```go
	memberIDs := make([]vo.Id, 0, len(b.accounts))
	for _, m := range b.accounts {
		memberIDs = append(memberIDs, m.AccountID)
	}
	accounts, err := s.accounts.AccountsByIDs(ctx, memberIDs)
```

which requires `budgetAggregate.accounts []model.BudgetAccount` loaded in `loadAggregate` via `s.budgets.MemberAccounts` (add it now, keep `excludedAccountIDs` alongside for the moment). Fake `AccountLookup`s in tests (`grep -rn "AccountsForOwners" internal --include=*_test.go`) are updated to `AccountsByIDs`.

- [ ] **Step 4: Run** — `go build ./... && go test ./internal/server/ ./internal/budget/...`. Budget api tests that relied on "all owned accounts are in" now fail (no membership rows) — that is expected and fixed in Task 8; run only `./internal/server/` for the green check here: PASS.

- [ ] **Step 5: Commit** — `git commit -am "refactor(budget): AccountsByIDs port (deleted accounts included) replaces AccountsForOwners"` (with trailer).

---

### Task 7: DTOs, error codes, i18n keys

**Files:**
- Modify: `internal/model/budget_dto.go` (`FiltersResult`, `CreateBudgetRequest`, `UpdateBudgetRequest`, `ExcludeAccount*/IncludeAccount*` → `AddAccount*/RemoveAccount*`)
- Modify: `internal/shared/errs/codes.go`
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`
- Test: `internal/test/i18ntest` (existing guards), `internal/model/budget_dto_test.go` if it exists

**Interfaces:**
- Produces:
  ```go
  type BudgetAccountFilter struct { Id string `json:"id"`; Removable bool `json:"removable"` }
  type FiltersResult struct { PeriodStart string `json:"periodStart"`; PeriodEnd string `json:"periodEnd"`; Accounts []BudgetAccountFilter `json:"accounts"` }
  CreateBudgetRequest.AccountIds []string `json:"accountIds"`   // replaces ExcludedAccounts
  UpdateBudgetRequest.AccountIds []string `json:"accountIds"`   // nil when absent
  type AddAccountRequest struct { BudgetId string `json:"id"`; AccountId string `json:"accountId"` }  // Validate: id + accountId NotBlank
  type AddAccountResult struct { Item MetaResult `json:"item"` }
  type RemoveAccountRequest / RemoveAccountResult — same shapes
  errs.CodeBudgetAccountNotRemovable = "budget.account_not_removable"
  errs.CodeBudgetAccountsRequired    = "budget.accounts_required"
  ```
- Catalogue keys: `errors.budget.account_not_removable`, `errors.budget.accounts_required`, `budgets.modal.budget_form.accounts_locked_hint`, `budgets.form.budget.accounts.validation.required`.

- [ ] **Step 1: DTOs**

Edit `internal/model/budget_dto.go`: replace `FiltersResult`, rename the request/result types, replace `ExcludedAccounts` with `AccountIds` on create/update (`CreateBudgetRequest.Validate` stays id+name NotBlank — the ≥1 rule is a coded error raised in the use case, Task 8). Delete `ExcludeAccountRequest/Result`, `IncludeAccountRequest/Result`. `go build ./...` will now fail in `internal/budget` and `internal/budget/{api,mcp}` — expected until Task 8; do not fix them here.

- [ ] **Step 2: Error codes**

`internal/shared/errs/codes.go`: add both constants next to the budget codes and both to `AllCodes`.

- [ ] **Step 3: Catalogues (all 11)**

Add, in every `locales/<lang>.json`, under `errors.budget`, `budgets.modal.budget_form` and `budgets.form.budget` (create the `accounts.validation` object under `budgets.form.budget`):

| lang | `errors.budget.account_not_removable` | `errors.budget.accounts_required` | `budgets.modal.budget_form.accounts_locked_hint` | `budgets.form.budget.accounts.validation.required` |
|---|---|---|---|---|
| en | This account has transactions in past months and can no longer be removed from the budget | Select at least one account | Accounts with transactions in past months can't be removed | Select at least one account |
| de | Dieses Konto hat Buchungen in vergangenen Monaten und kann nicht mehr aus dem Budget entfernt werden | Wähle mindestens ein Konto | Konten mit Buchungen in vergangenen Monaten können nicht entfernt werden | Wähle mindestens ein Konto |
| es | Esta cuenta tiene transacciones en meses anteriores y ya no se puede quitar del presupuesto | Selecciona al menos una cuenta | Las cuentas con transacciones en meses anteriores no se pueden quitar | Selecciona al menos una cuenta |
| fr | Ce compte a des transactions dans des mois passés et ne peut plus être retiré du budget | Sélectionnez au moins un compte | Les comptes ayant des transactions dans des mois passés ne peuvent pas être retirés | Sélectionnez au moins un compte |
| it | Questo conto ha transazioni nei mesi passati e non può più essere rimosso dal budget | Seleziona almeno un conto | I conti con transazioni nei mesi passati non possono essere rimossi | Seleziona almeno un conto |
| nl | Deze rekening heeft transacties in eerdere maanden en kan niet meer uit het budget worden verwijderd | Selecteer minstens één rekening | Rekeningen met transacties in eerdere maanden kunnen niet worden verwijderd | Selecteer minstens één rekening |
| pl | To konto ma transakcje w poprzednich miesiącach i nie można go już usunąć z budżetu | Wybierz co najmniej jedno konto | Kont z transakcjami w poprzednich miesiącach nie można usunąć | Wybierz co najmniej jedno konto |
| pt | Esta conta tem transações em meses anteriores e já não pode ser removida do orçamento | Selecione pelo menos uma conta | Contas com transações em meses anteriores não podem ser removidas | Selecione pelo menos uma conta |
| ru | У этого счёта есть операции в прошлых месяцах, и его больше нельзя убрать из бюджета | Выберите хотя бы один счёт | Счета с операциями в прошлых месяцах нельзя убрать | Выберите хотя бы один счёт |
| uk | На цьому рахунку є операції в минулих місяцях, і його більше не можна прибрати з бюджету | Виберіть принаймні один рахунок | Рахунки з операціями в минулих місяцях не можна прибрати | Виберіть принаймні один рахунок |
| zh | 此账户在过去的月份中有交易，无法再从预算中移除 | 请至少选择一个账户 | 在过去月份中有交易的账户无法移除 | 请至少选择一个账户 |

Keep JSON key order consistent with the surrounding entries (alphabetical is not required, but every catalogue must carry every key). Frontend keys will be consumed in Task 11 — the frontend-source-coverage guard only fails on keys used but missing, not on unused keys; verify with `go test ./internal/test/i18ntest/`.

- [ ] **Step 4: Run** — `go test ./internal/test/i18ntest/ ./internal/shared/... ./internal/model/...` → PASS (`internal/budget` doesn't build yet; that's fine — do not run `./...`).

- [ ] **Step 5: Commit** — `git commit -am "feat(budget): membership DTOs, error codes, catalogue keys"` (with trailer).

---

### Task 8: Budget service — membership resolution, rules, endpoints, tests

**Files:**
- Modify: `internal/budget/usecase.go` (aggregate), `internal/budget/builder.go` (`filters`, `buildFilters`, `BuildBudget`), `internal/budget/accounts.go` (`AddAccount`/`RemoveAccount`, drop `toggleAccount`), `internal/budget/crud.go` (`UpdateBudget`), `internal/budget/create.go`, `internal/budget/accesssvc.go` (`removeMemberRecords`)
- Modify: `internal/budget/api/budget_more.go` (handlers), `internal/budget/api/routes.go:38-39`
- Modify: every test creating a budget in `internal/budget/api/*_test.go` and `internal/test/enginecompare/budget_tag_visibility_test.go` (add `accountIds`), `internal/budget/api/update_budget_excluded_scope_test.go` (rewrite as membership scope), `internal/budget/api/shared_account_test.go`
- Create: `internal/budget/api/membership_test.go`
- Run: `make swagger`

**Interfaces:**
- Produces: `(*Service).AddAccount(ctx, userID, model.AddAccountRequest) (*model.AddAccountResult, error)`; `RemoveAccount(...)`; unexported `removableAccounts(ctx, b *budgetAggregate, ids []vo.Id, now time.Time) (map[string]bool, error)`; routes `POST /api/v1/budget/add-account`, `POST /api/v1/budget/remove-account`; `budgetAggregate.accounts []model.BudgetAccount` (the `excludedAccountIDs` field is removed).
- Consumes: Tasks 4–7.

- [ ] **Step 1: Write the failing membership tests** — `internal/budget/api/membership_test.go`. Reuse the harness helpers (`newHarnessWithClock`, `h.do`, `h.doH` with `X-Timezone`, `createBudgetReq` — extend `createBudgetReq` to include `"accountIds": []string{accountID}`; the harness seeds `accountID` for `seedUserID`). Add a second seeded account constant `accountID2` in the harness (`h.f.Account(...)` + folder + option) so tests can add/remove it.

```go
package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/fixture"
)

// now = 2026-08-17 10:00 UTC → current month starts 2026-08-01. tzClock is the
// fixed clock create_timezone_test.go declares (struct{ t time.Time } with Now()).
func fixedAugust() tzClock { return tzClock{t: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)} }

type filtersOut struct {
	Item struct {
		Filters struct {
			Accounts []struct {
				Id        string `json:"id"`
				Removable bool   `json:"removable"`
			} `json:"accounts"`
		} `json:"filters"`
	} `json:"item"`
}

func TestCreateBudget_RequiresAtLeastOneOwnedAccount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "B", "currencyId": usdID, "accountIds": []string{}})
	if st != http.StatusBadRequest || !containsField(env, "accountIds", "Select at least one account") {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	// deleted account cannot be a member
	h.f.Account(fixture.Account{ID: deadAccountID, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "B", "currencyId": usdID, "accountIds": []string{deadAccountID}})
	if st != http.StatusBadRequest {
		t.Fatalf("deleted account accepted: %s", env.raw)
	}
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "B"))
	if st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	res := mustUnmarshal[filtersOut](t, env.Data)
	if len(res.Item.Filters.Accounts) != 1 || res.Item.Filters.Accounts[0].Id != accountID || !res.Item.Filters.Accounts[0].Removable {
		t.Fatalf("filters=%+v", res.Item.Filters)
	}
}

func TestMembership_DeletedMemberKeepsCounting(t *testing.T) {
	// The core regression: spend on a since-deleted member stays in spentBefore,
	// so a category with a 100 limit and 100 spend in July shows available 0 in August.
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "100", SpentAt: "2026-07-10 12:00:00"})
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "B", "currencyId": usdID, "startDate": "2026-07-01", "accountIds": []string{accountID}}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-07-01", "amount": "100"}))
	// soft-delete the account directly (the account feature isn't wired in this harness)
	if _, err := h.db.Exec(`UPDATE accounts SET is_deleted = 1 WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	st, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-15", tok, nil)
	if st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	res := mustUnmarshal[elementView](t, env.Data) // elementView is declared in carryover_test.go
	found := false
	for _, el := range res.Item.Structure.Elements {
		if el.Id == catID {
			found = true
			if el.Available != "0" {
				t.Fatalf("available=%q want 0 (spentBefore must include the deleted member)", el.Available)
			}
		}
	}
	if !found {
		t.Fatal("Food element missing")
	}
}

func TestMembership_RemovalRule(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.Account(fixture.Account{ID: accountID2, UserID: seedUserID, CurrencyID: usdID, Name: "Savings"})
	// accountID: transaction in July (closed month) → locked. accountID2: only August → removable.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2026-07-20 12:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID2, Type: 0, Amount: "1", SpentAt: "2026-08-02 12:00:00"})
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "B", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID, accountID2}}))

	st, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	res := mustUnmarshal[filtersOut](t, env.Data)
	removable := map[string]bool{}
	for _, a := range res.Item.Filters.Accounts {
		removable[a.Id] = a.Removable
	}
	if removable[accountID] || !removable[accountID2] {
		t.Fatalf("removable=%v", removable)
	}

	st, env = h.do(t, http.MethodPost, "/api/v1/budget/remove-account", tok, map[string]any{"id": budgetID1, "accountId": accountID})
	if st != http.StatusBadRequest || !containsField(env, "accountId", "can no longer be removed") {
		t.Fatalf("locked removal: status=%d body=%s", st, env.raw)
	}
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/remove-account", tok, map[string]any{"id": budgetID1, "accountId": accountID2}))
	// update-budget replace-set: omitting the locked member is refused
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "B", "currencyId": usdID, "accountIds": []string{}})
	if st != http.StatusBadRequest {
		t.Fatalf("update omitting locked member: %s", env.raw)
	}
	// update-budget without accountIds leaves membership alone
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "B2", "currencyId": usdID}))
	// re-add + transaction before started_at doesn't lock
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/add-account", tok, map[string]any{"id": budgetID1, "accountId": accountID2}))
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID2, Type: 0, Amount: "1", SpentAt: "2026-05-20 12:00:00"})
	_, env = h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	res = mustUnmarshal[filtersOut](t, env.Data)
	for _, a := range res.Item.Filters.Accounts {
		if a.Id == accountID2 && !a.Removable {
			t.Fatal("pre-start transaction must not lock")
		}
	}
}

func TestMembership_BoundaryFollowsCallerTimezone(t *testing.T) {
	// 2026-08-01 02:00 UTC is still July 31 in America/Los_Angeles: a July 31 transaction
	// is in the CURRENT month for that caller, so the account stays removable there.
	h := newHarnessWithClock(t, tzClock{t: time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)})
	tok := h.token(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2026-07-31 12:00:00"})
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "B", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID}}))
	_, env := h.doH(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil, map[string]string{"X-Timezone": "America/Los_Angeles"})
	if res := mustUnmarshal[filtersOut](t, env.Data); !res.Item.Filters.Accounts[0].Removable {
		t.Fatal("LA caller: still removable")
	}
	_, env = h.doH(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil, map[string]string{"X-Timezone": "UTC"})
	if res := mustUnmarshal[filtersOut](t, env.Data); res.Item.Filters.Accounts[0].Removable {
		t.Fatal("UTC caller: locked")
	}
}

func TestMembership_LeavingParticipantTakesTheirAccounts(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	// second user with an account that has a closed-month transaction (locked)
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: "s"})
	h.f.Account(fixture.Account{ID: otherAccountID, UserID: otherUserID, CurrencyID: usdID})
	h.f.Transaction(fixture.Transaction{UserID: otherUserID, AccountID: otherAccountID, Type: 0, Amount: "7", SpentAt: "2026-07-05 12:00:00"})
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "B", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID}}))
	// grant-access requires a users_connections link
	h.f.Connect(seedUserID, otherUserID)
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1}))
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/add-account", otherUserID, map[string]any{"id": budgetID1, "accountId": otherAccountID}))
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, budgetID1).Scan(&n)
	if n != 2 {
		t.Fatalf("members=%d want 2", n)
	}
	mustOK(t, h.do(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID}))
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, budgetID1, otherAccountID).Scan(&n)
	if n != 0 {
		t.Fatal("departing member's locked account must be dropped")
	}
}
```

Helpers to add in this file (grep the package first — if `mustOK`-like helpers already exist under another name, reuse them):

```go
func mustOK(t *testing.T, st int, env envelope) {
	t.Helper()
	if st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
}

// containsField reports whether the envelope's errors[field] carries a message
// containing substr.
func containsField(env envelope, field, substr string) bool {
	var errs map[string][]string
	if json.Unmarshal(env.Errors, &errs) != nil {
		return false
	}
	for _, m := range errs[field] {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}
```

Also give the harness a `f *fixture.Builder` field (set from the builder `newHarnessWithClock` already creates) so tests can seed rows, plus constants `accountID2`, `deadAccountID`, `otherAccountID`, `otherUserID` (UUID strings) in `harness_test.go`.

- [ ] **Step 2: Run** — `go test ./internal/budget/api/ -run 'Membership|RequiresAtLeastOne'` → compile FAIL.

- [ ] **Step 3: Implement the service**

`usecase.go`: `budgetAggregate{... accounts []model.BudgetAccount ...}` (delete `excludedAccountIDs`); `loadAggregate` calls `s.budgets.MemberAccounts`.

`builder.go`: `filters` gets `accountFilters []model.BudgetAccountFilter` in place of `excludedAccountIDs`; `BuildBudget` emits `Filters: model.FiltersResult{PeriodStart, PeriodEnd, Accounts: f.accountFilters}`. New `buildFilters` body for the account part:

```go
	memberIDs := make([]vo.Id, 0, len(b.accounts))
	for _, m := range b.accounts {
		memberIDs = append(memberIDs, m.AccountID)
	}
	views, err := s.accounts.AccountsByIDs(ctx, memberIDs)
	if err != nil {
		return filters{}, err
	}
	included := memberIDs
	currencySet := map[string]vo.Id{}
	var currencyIDs []vo.Id
	var ownIDs []vo.Id
	for i, v := range views {
		s.accountOwners[v.ID] = v.OwnerID
		if v.OwnerID == userID.String() {
			ownIDs = append(ownIDs, memberIDs[i])
		}
		if _, seen := currencySet[v.CurrencyID]; !seen {
			cid, cerr := vo.ParseId(v.CurrencyID)
			if cerr != nil {
				return filters{}, cerr
			}
			currencySet[v.CurrencyID] = cid
			currencyIDs = append(currencyIDs, cid)
		}
	}
	removable, err := s.removableAccounts(ctx, b, ownIDs, s.clock.Now())
	if err != nil {
		return filters{}, err
	}
	accountFilters := make([]model.BudgetAccountFilter, 0, len(ownIDs))
	for _, id := range ownIDs {
		accountFilters = append(accountFilters, model.BudgetAccountFilter{Id: id.String(), Removable: removable[id.String()]})
	}
```

`accounts.go` — replace `ExcludeAccount/IncludeAccount/toggleAccount` with:

```go
// removableAccounts reports, per account, whether it may still leave the
// budget: only while it has no transactions in a CLOSED month of the budget
// (started_at <= spent_at < first of the caller's current month). Removing such
// an account changes only the month being viewed; anything older would shrink
// spentBefore under limits that stay — the drift this design exists to end.
func (s *Service) removableAccounts(ctx context.Context, b *budgetAggregate, ids []vo.Id, now time.Time) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id.String()] = true
	}
	if len(ids) == 0 {
		return out, nil
	}
	end := localMonth(now, reqctx.Location(ctx))
	active, err := s.read.AccountsWithTransactions(ctx, ids, model.FirstOfMonth(b.budget.StartedAt), end)
	if err != nil {
		return nil, err
	}
	for _, id := range active {
		out[id.String()] = false
	}
	return out, nil
}

func accountNotRemovable() error {
	return errs.NewValidation("Validation failed", errs.FieldError{
		Key: "accountId", Message: "This account has transactions in past months and can no longer be removed from the budget",
		Code: errs.CodeBudgetAccountNotRemovable,
	})
}

func (s *Service) AddAccount(ctx context.Context, userID vo.Id, req model.AddAccountRequest) (*model.AddAccountResult, error) {
	budgetID, accountID, b, err := s.membershipPrelude(ctx, userID, req.BudgetId, req.AccountId)
	if err != nil {
		return nil, err
	}
	views, err := s.accounts.AccountsByIDs(ctx, []vo.Id{accountID})
	if err != nil {
		return nil, err
	}
	if views[0].IsDeleted {
		return nil, model.ValidateBlank(map[string]string{"accountId": ""})
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		return s.budgets.AddAccount(txCtx, budgetID, accountID, s.clock.Now())
	}); err != nil {
		return nil, err
	}
	meta, err := s.reloadMeta(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	return &model.AddAccountResult{Item: meta}, nil
}

func (s *Service) RemoveAccount(ctx context.Context, userID vo.Id, req model.RemoveAccountRequest) (*model.RemoveAccountResult, error) {
	budgetID, accountID, b, err := s.membershipPrelude(ctx, userID, req.BudgetId, req.AccountId)
	if err != nil {
		return nil, err
	}
	isMember := false
	for _, m := range b.accounts {
		if m.AccountID.Equal(accountID) {
			isMember = true
		}
	}
	if isMember {
		removable, err := s.removableAccounts(ctx, b, []vo.Id{accountID}, s.clock.Now())
		if err != nil {
			return nil, err
		}
		if !removable[accountID.String()] {
			return nil, accountNotRemovable()
		}
		if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
			return s.budgets.RemoveAccount(txCtx, budgetID, accountID)
		}); err != nil {
			return nil, err
		}
	}
	meta, err := s.reloadMeta(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	return &model.RemoveAccountResult{Item: meta}, nil
}

// membershipPrelude parses ids and enforces the two authorization rules every
// membership write shares: the caller owns the account and can update the budget.
func (s *Service) membershipPrelude(ctx context.Context, userID vo.Id, rawBudget, rawAccount string) (vo.Id, vo.Id, *budgetAggregate, error) {
	budgetID, err := vo.ParseId(rawBudget)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	accountID, err := vo.ParseId(rawAccount)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, model.ValidateBlank(map[string]string{"accountId": ""})
	}
	owner, err := s.accounts.AccountOwner(ctx, accountID)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, err
	}
	if !owner.Equal(userID) {
		return vo.Id{}, vo.Id{}, nil, accessDenied()
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return vo.Id{}, vo.Id{}, nil, err
	}
	if !s.canUpdate(b, userID) {
		return vo.Id{}, vo.Id{}, nil, accessDenied()
	}
	return budgetID, accountID, b, nil
}

func (s *Service) reloadMeta(ctx context.Context, budgetID vo.Id) (model.MetaResult, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return model.MetaResult{}, err
	}
	return s.buildMeta(ctx, b)
}
```

(`reqctx`, `errs`, `time` imports as needed. Keep the existing `ownsAccount` helper — `UpdateBudget` uses it.)

`crud.go` `UpdateBudget` — replace the excluded block inside the tx with:

```go
		// accountIds absent → membership untouched (older clients, MCP). Present →
		// replace-set over the caller's OWN accounts: add missing, remove absent
		// ones — but a member with closed-month history is permanent, so naming
		// a set that drops one fails the whole update.
		if req.AccountIds != nil {
			want := map[string]bool{}
			for _, raw := range req.AccountIds {
				aid, perr := vo.ParseId(raw)
				if perr != nil {
					return model.ValidateBlank(map[string]string{"accountIds": ""})
				}
				owned, oerr := s.ownsAccount(txCtx, userID, aid)
				if oerr != nil {
					return oerr
				}
				if !owned {
					continue
				}
				want[aid.String()] = true
			}
			var ownMembers []vo.Id
			for _, m := range b.accounts {
				owned, oerr := s.ownsAccount(txCtx, userID, m.AccountID)
				if oerr != nil {
					return oerr
				}
				if owned {
					ownMembers = append(ownMembers, m.AccountID)
				}
			}
			removable, rerr := s.removableAccounts(txCtx, b, ownMembers, now)
			if rerr != nil {
				return rerr
			}
			for _, m := range ownMembers {
				if want[m.String()] {
					continue
				}
				if !removable[m.String()] {
					return accountNotRemovable()
				}
				if serr := s.budgets.RemoveAccount(txCtx, budgetID, m); serr != nil {
					return serr
				}
			}
			for idStr := range want {
				aid, _ := vo.ParseId(idStr)
				views, verr := s.accounts.AccountsByIDs(txCtx, []vo.Id{aid})
				if verr != nil {
					return verr
				}
				if views[0].IsDeleted {
					return model.ValidateBlank(map[string]string{"accountIds": ""})
				}
				if serr := s.budgets.AddAccount(txCtx, budgetID, aid, now); serr != nil {
					return serr
				}
			}
		}
```

Update the doc comment on `UpdateBudget` ("name/currency/member-accounts").

`create.go` — replace the excluded loop with:

```go
		if len(req.AccountIds) == 0 {
			return errs.NewValidation("Validation failed", errs.FieldError{
				Key: "accountIds", Message: "Select at least one account", Code: errs.CodeBudgetAccountsRequired,
			})
		}
		for _, raw := range req.AccountIds {
			aid, perr := vo.ParseId(raw)
			if perr != nil {
				return model.ValidateBlank(map[string]string{"accountIds": ""})
			}
			owned, oerr := s.ownsAccount(txCtx, userID, aid)
			if oerr != nil {
				return oerr
			}
			views, verr := s.accounts.AccountsByIDs(txCtx, []vo.Id{aid})
			if verr != nil {
				return verr
			}
			if !owned || views[0].IsDeleted {
				return model.ValidateBlank(map[string]string{"accountIds": ""})
			}
			if serr := s.budgets.AddAccount(txCtx, budgetID, aid, now); serr != nil {
				return serr
			}
		}
```

Note: the "at least one" check must run BEFORE `s.budgets.Save` (move it above the tx or first inside it) so a refused create leaves no budget row.

`accesssvc.go` `removeMemberRecords` — after the element loop, before envelopes:

```go
	if err := s.budgets.RemoveAccountsOwnedBy(ctx, b.budget.ID, memberID); err != nil {
		return err
	}
```

`api/budget_more.go`: rename the two handlers to `AddAccount`/`RemoveAccount` with swag blocks (`@Summary Add an account to a budget` / `Remove an account from a budget`, `@Router /api/v1/budget/add-account [post]` / `remove-account`, request/result types renamed). `routes.go`: `mux.Handle("POST /api/v1/budget/add-account", auth(h.AddAccount))`, `mux.Handle("POST /api/v1/budget/remove-account", auth(h.RemoveAccount))`.

Run `make swagger`.

- [ ] **Step 4: Fix the existing test suite**

- Harness: seed `accountID` (already), add constants; make `createBudgetReq` return `"accountIds": []string{accountID}`.
- Every inline create-budget body in `internal/budget/api/*_test.go` and `internal/test/enginecompare/budget_tag_visibility_test.go`: add `"accountIds": []string{<the owner's seeded account id>}` (list with `grep -rn '"create-budget"' internal --include=*_test.go`). Tests that used `excludedAccounts` (`update_budget_excluded_scope_test.go`, `shared_account_test.go`, others: `grep -rln excludedAccounts internal --include=*_test.go`) are rewritten in membership terms: `update_budget_excluded_scope_test.go` → `update_budget_membership_scope_test.go` asserting a partner's members survive the owner's `accountIds` replace-set (same intent, inverted model).
- Any test reading `filters.excludedAccountsIds` reads `filters.accounts`.

Run: `go test ./internal/budget/... ./internal/server/... ./internal/model/...` until PASS.

- [ ] **Step 5: MCP compile fix (temporary minimal)** — Task 10 rewrites the tools; for now make `internal/budget/mcp` compile: `update_budget` no longer round-trips (drop the `GetBudget` call and the `ExcludedAccounts` field); `set_budget_account_included` calls `AddAccount`/`RemoveAccount`; `create_budget` passes `AccountIds: in.AccountIDs` (add `AccountIDs []string \`json:"account_ids"\`` to its input). `go build ./...` must pass. mcpparity goldens will fail until Task 10 — acceptable within this task only if you go straight on; otherwise run Task 10 next.

- [ ] **Step 6: Run everything except goldens** — `go build ./... && go vet ./... && go test $(go list ./... | grep -v '/test/apiparity\|/test/mcpparity')` → PASS.

- [ ] **Step 7: Commit** — `git commit -am "feat(budget): explicit account membership — resolution over budgets_accounts (deleted included), activity-locked removal, add/remove-account, accountIds on create/update"` (with trailer).

---

### Task 9: Migration 002 (seed + drop), remove the blacklist code path, migration test

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260817000002.sql`, `internal/infra/storage/migrations/pgsql/20260817000002.sql`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/budgets.sql` (delete the three `*Excluded*` queries; regenerate), `internal/budget/repo/engine.go`, `repo/repo.go`, `internal/budget/repository.go` (delete `ExcludedAccountIDs/ExcludeAccount/IncludeAccount`)
- Create: `internal/infra/storage/migrations/migration_20260817_test.go`
- Test: `internal/infra/storage/migrate/labels_schema_test.go`-style existence check optional

> **Escape hatch (spec §2d):** first preference is plain SQL. If the seed
> `INSERT … SELECT` proves too complex to keep byte-identical across the two
> dialects, switch `20260817000002` to a command step
> `migration:seed-budget-accounts` (register like `20260817000001`) and move
> the `DROP TABLE` to a new `20260817000003.sql`. The command must use
> hand-written SQL with `db.Rebind` (sqlc cannot see
> `budgets_excluded_accounts` — the final schema drops it) and must **no-op
> when the blacklist table is missing**, which is what keeps a by-hand re-run
> safe after the migration era. The migration test in this task stays
> identical either way.

- [ ] **Step 1: Failing migration test**

```go
package migrations_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

const membershipSchemaVersion = "20260817000000"

func TestMigration20260817_SeedsMembershipAndDropsBlacklist(t *testing.T) {
	db := runUpTo(t, "membership_seed", membershipSchemaVersion)
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const ts = "'2026-01-01 00:00:00'"
	for _, u := range []string{"owner", "member", "guest"} {
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?, ?, ?, 'U', '', 'x', '', ` + ts + `, ` + ts + `)`, u, u, u+"@e.test")
	}
	mustExec(`INSERT INTO currencies (id, code, name, symbol, fraction_digits, created_at) VALUES ('cur1', 'EUR', 'Euro', '€', 2, ` + ts + `)`)
	// budget started 2026-03, created 2026-04-01
	mustExec(`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at) VALUES ('b1', 'cur1', 'owner', 'B', '2026-03-01 00:00:00', '2026-04-01 00:00:00', '2026-04-01 00:00:00')`)
	mustExec(`INSERT INTO budgets_access (budget_id, user_id, role, is_accepted, created_at, updated_at) VALUES ('b1', 'member', 1, 1, ` + ts + `, ` + ts + `)`)
	mustExec(`INSERT INTO budgets_access (budget_id, user_id, role, is_accepted, created_at, updated_at) VALUES ('b1', 'guest', 2, 1, ` + ts + `, ` + ts + `)`)
	acct := func(id, user string, deleted int, updated string) {
		mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, 'cur1', ?, 'A', 2, 'wallet', ?, ` + ts + `, ?)`, id, user, deleted, updated)
	}
	acct("a-live", "owner", 0, "2026-01-01 00:00:00")
	acct("a-excl", "owner", 0, "2026-01-01 00:00:00")
	acct("a-del-after", "owner", 1, "2026-05-10 00:00:00")   // deleted after start AND after creation → seeded
	acct("a-del-between", "owner", 1, "2026-03-15 00:00:00") // after start, before creation → not seeded
	acct("a-del-before", "owner", 1, "2026-02-01 00:00:00")  // before start → not seeded
	acct("a-member", "member", 0, "2026-01-01 00:00:00")
	acct("a-guest", "guest", 0, "2026-01-01 00:00:00")
	mustExec(`INSERT INTO budgets_excluded_accounts (budget_id, account_id) VALUES ('b1', 'a-excl')`)

	var calls []string
	all := make([]migrate.Migration, 0)
	for _, f := range migrations.SQLite() {
		all = append(all, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(context.Background(), db, all, migrate.WithCommandRunner(func(ctx context.Context, name string) error {
		calls = append(calls, name)
		return nil
	})); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(calls) != 1 || calls[0] != "migration:zero-deleted-accounts" {
		t.Fatalf("command steps = %v", calls)
	}
	got := idsInKeyOrder(t, db, `SELECT account_id FROM budgets_accounts WHERE budget_id = 'b1' ORDER BY account_id`)
	assertSame(t, got, []string{"a-del-after", "a-live", "a-member"}, "seeded members")
	if _, err := db.Exec(`SELECT 1 FROM budgets_excluded_accounts`); err == nil {
		t.Fatal("budgets_excluded_accounts still exists")
	}
}
```

(`runUpTo`, `idsInKeyOrder`, `assertSame` exist in `migration_20260803_test.go`; `runUpTo` builds `migrate.Migration` from files — after Task 1 it copies `Command` and passes `NoCommands`.)

- [ ] **Step 2: Run** → FAIL (no 002; table still there).

- [ ] **Step 3: Write migration 002**

`sqlite/20260817000002.sql`:

```sql
-- Seed explicit membership from the current implicit population: every account
-- owned by the owner or an accepted non-guest participant, minus the
-- blacklist. A deleted account is seeded only if it was deleted after the
-- budget both started and existed — its history was part of the budget.
INSERT INTO budgets_accounts (budget_id, account_id, created_at)
SELECT DISTINCT b.id, a.id, CURRENT_TIMESTAMP
FROM budgets b
JOIN accounts a
  ON a.user_id = b.user_id
  OR a.user_id IN (SELECT ba.user_id FROM budgets_access ba WHERE ba.budget_id = b.id AND ba.is_accepted = 1 AND ba.role <> 2)
WHERE (a.is_deleted = 0 OR (a.updated_at >= b.started_at AND a.updated_at > b.created_at))
  AND NOT EXISTS (SELECT 1 FROM budgets_excluded_accounts e WHERE e.budget_id = b.id AND e.account_id = a.id);

DROP TABLE budgets_excluded_accounts;
```

pgsql: `a.is_deleted = FALSE`, `ba.is_accepted = TRUE`, otherwise identical. Check the sqlite `CURRENT_TIMESTAMP` renders as `YYYY-MM-DD HH:MM:SS` (it does) and that pgsql's `TIMESTAMP` column accepts `CURRENT_TIMESTAMP` (it does).

- [ ] **Step 4: Remove the old code path**

Delete the three `*Excluded*` queries from both `budgets.sql`, regenerate sqlc, delete the querier/adapter methods in `engine.go`, the repo methods in `repo.go`, and the three interface methods in `repository.go`. Update the `BudgetStore` doc comment ("…and the member-accounts join table").

- [ ] **Step 5: Run** — `go build ./... && go test ./internal/infra/storage/... ./internal/budget/...` → PASS. The seeded-data test above runs on sqlite; the pgsql 000/002 SQL is applied on every fresh pgsql test database (`dbtest`, `enginecompare`) and the seed query's syntax is thereby verified on both engines in Task 13's `make test`.

- [ ] **Step 6: Commit** — `git commit -am "feat(budget): migration 20260817000002 seeds budgets_accounts and drops budgets_excluded_accounts"` (with trailer).

---

### Task 9a: Self-healing zero — transaction writes touching deleted accounts (spec §5a)

**Files:**
- Modify: `internal/account/zero.go` (public `ZeroIfDeleted`)
- Modify: `internal/transaction/ports.go`, `internal/transaction/usecase.go` (new port field + constants), `create.go`, `update.go`, `delete.go`
- Create: `internal/server/glue_transaction_zeroer.go`; wire in `internal/server` where the transaction service is built
- Modify: `internal/shared/errs/codes.go`, all 11 `locales/<lang>.json` (`errors.transaction.account_deleted`)
- Test: `internal/transaction/api/deleted_account_test.go` (new); one apiparity scenario + goldens

**Interfaces:**
- Produces (account): `(*account.Service).ZeroIfDeleted(ctx, accountID vo.Id, spentAt time.Time, description string) (bool, error)` — `GetByID`; **no-op `(false, nil)` on a live account**; otherwise `zeroBalance(ctx, acct, spentAt, s.clock.Now(), description)`. Deliberately NO `WithTx` of its own: it must join the caller's transaction (both features' repos read the tx from the shared `TxRunner` ctx).
- Produces (transaction): port `AccountZeroer { ZeroDeleted(ctx context.Context, accountID vo.Id, spentAt time.Time, description string) error }`; `AccountResolver` gains `AccountDeleted(ctx, accountID vo.Id) (bool, error)` (glue: a small account-repo lookup, same shape as `AccountOwner`); constants `correctionDeletedTx = "Balance adjustment (deleted transaction)"` and `correctionEditedTx = "Balance adjustment (edited transaction)"` in `usecase.go`.
- Produces (errs): `CodeTransactionAccountDeleted = "transaction.account_deleted"` in `AllCodes`; `en` text `"This account has been deleted"` (translate in all 11 catalogues — i18ntest enforces parity both ways).

Rules recap (spec §5a): create rejects a deleted `accountId`/`accountRecipientId`; update rejects a deleted account **newly introduced** to the account set but allows edits keeping/removing one; delete is always allowed; after a delete or edit, every deleted account in old ∪ new sets is re-zeroed **inside the same `WithTx`** — `spent_at` = old `spent_at` when the transaction was deleted or the account was swapped out, new `spent_at` when it remains a party. Bulk update is exempt (classification-only, transfers rejected). Import is exempt (its `Importer.AvailableAccounts`/`AccountByID` resolution returns live accounts only — asserted by test, not re-guarded).

- [ ] **Step 1: Failing tests**

Create `internal/transaction/api/deleted_account_test.go` in the transaction api harness style (`grep -n "newHarness" internal/transaction/api/*_test.go`; reuse its `corrections`-style raw-SQL helper pattern from Task 2 — copy locally, `db.Rebind` for pgsql). Seed: live account B, account A with a transfer A→B `50` at `"2026-02-10 00:00:00"`, then delete A over the API (auto-zero from Task 2 runs; A's correction set is now `{zeroing at delete}`). Tests:

1. `TestDeleteTransaction_ReZeroesDeletedSide` — delete the transfer from B's side → 200; A gains exactly one NEW correction: `spent_at = "2026-02-10 00:00:00"`, `description = "Balance adjustment (deleted transaction)"`, amount/type reconciling A to zero; B gains none; A's all-time balance is `0`.
2. `TestUpdateTransaction_AmountEdit_ReZeroesDeletedSide` — change the transfer amount `50→30` → new correction on A at the transaction's `spent_at`, `description = "Balance adjustment (edited transaction)"`; A ends at zero.
3. `TestUpdateTransaction_SwapOutDeletedSide` — repoint `accountRecipientId` to a second live account → correction on A at the **old** `spent_at` (edit description); A ends at zero.
4. `TestUpdateTransaction_DateOnlyEdit_NoCorrection` — move `date` only → no new correction on A.
5. `TestUpdateTransaction_CannotIntroduceDeletedAccount` — an expense on B edited to `accountId = A` (and separately a transfer edited to `accountRecipientId = A`) → 400, envelope code `transaction.account_deleted`, field matches the offending side; nothing written.
6. `TestCreateTransaction_RejectsDeletedAccount` — create expense on A → 400 same code; transfer B→A → 400 on `accountRecipientId`; `CreateTransactionFromRecurring` path via its public entry point → same.
7. `TestDeleteTransaction_DeletingCorrectionRestoresIt` — delete A's zeroing correction over the API → 200, and a fresh correction exists (A back at zero).
8. `TestDeleteTransaction_BothSidesDeleted` — transfer A↔C with both deleted → deleting it re-zeroes both.
9. `TestImport_CannotTargetDeletedAccount` — an import row naming A's account name creates a NEW live account (find-or-create resolves available accounts only) rather than writing to A; assert A has no new transactions.

- [ ] **Step 2: Run to verify failure** — `go test ./internal/transaction/api/ -run 'DeletedAccount|ReZero|DeletedSide|Introduce' -v`. Expected: FAIL (guards and corrections absent).

- [ ] **Step 3: Implement**

1. `internal/account/zero.go`: add `ZeroIfDeleted` (signature above).
2. `internal/transaction/usecase.go`: add the two description constants; add `zeroer AccountZeroer` to the service struct + constructor.
3. `internal/transaction/ports.go`: `AccountZeroer`; extend `AccountResolver` with `AccountDeleted`.
4. `create.go` (`createTransaction`), after the write-access checks (access first — a caller with no access must still get the access error, and must not learn a foreign account's deleted state): for `accountID` and, when a transfer, the recipient id — `AccountDeleted` → `errs.ValidationError{Msg: "This account has been deleted", MsgCode: errs.CodeTransactionAccountDeleted}` on the offending field (follow the codebase's field-error construction; check how `checkReferences` attaches field names).
5. `update.go` (`updateTransaction`), inside `WithTx`: capture `oldParties := partySet(t)` and `oldSpentAt := t.SpentAt` before `t.Update(st, now)`; reject any deleted account in the NEW set that is not in `oldParties` (same error as create); after `Save`/`ReplaceLabels`, for each deleted account in `oldParties ∪ newParties`: `spentAt := t.SpentAt` (new) if it is in the new set, else `oldSpentAt`; call `s.zeroer.ZeroDeleted(ctx, id, spentAt, correctionEditedTx)`. Add a tiny `partySet(t) []vo.Id` helper (source + recipient when present).
6. `delete.go`, inside `WithTx` after `s.repo.Delete`: for each deleted account among the transaction's parties → `ZeroDeleted(ctx, id, t.SpentAt, correctionDeletedTx)`.
7. `internal/server/glue_transaction_zeroer.go`: adapter over `account.Service.ZeroIfDeleted`; extend the existing transaction `AccountResolver` glue with `AccountDeleted`; wire.
8. `errs.CodeTransactionAccountDeleted` in `codes.go` + `AllCodes`; add `errors.transaction.account_deleted` to all 11 catalogues.
9. Fix compile fallout in transaction test fakes (new port + resolver method).

- [ ] **Step 4: apiparity scenario** — add one create-transaction-against-deleted-account scenario (fixture gains a deleted account or delete one in the scenario setup); `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and INSPECT: exactly the one new golden, count grows.
- [ ] **Step 5: Run** — `go test ./internal/transaction/... ./internal/account/... ./internal/test/i18ntest/ ./internal/test/apiparity/`. Expected: PASS.
- [ ] **Step 6: Commit** — `git commit -am "feat(transaction): self-healing zero for writes touching deleted accounts"` (with trailer).

---

### Task 10: MCP tools + mcpparity goldens

**Files:**
- Modify: `internal/budget/mcp/mcp.go`, `internal/test/mcpparity/catalogue.go`, goldens under `internal/test/mcpparity/testdata/`

- [ ] **Step 1: Rewrite the tools**

- `create_budget` input gains `AccountIDs []string \`json:"account_ids" jsonschema:"account ids (UUIDs) to track in the budget, at least one; from list_accounts"\``.
- `update_budget`: description "Rename a budget or change its currency. Member accounts are left untouched (use add_budget_account / remove_budget_account)." — passes no `AccountIds`.
- Replace `set_budget_account_included` with two tools:

```go
type budgetAccountInput struct {
	BudgetID  string `json:"budget_id" jsonschema:"budget id (UUID), from list_budgets"`
	AccountID string `json:"account_id" jsonschema:"account id (UUID) owned by you, from list_accounts"`
}
type budgetAccountResult struct {
	BudgetID  string `json:"budget_id"`
	AccountID string `json:"account_id"`
	Member    bool   `json:"member"`
}
sdk.AddTool(s, &sdk.Tool{Name: "add_budget_account", Description: "Add one of your accounts to a budget so its transactions and balance count. Use list_accounts for account_id."}, …AddAccount… → budgetAccountResult{…, Member: true})
sdk.AddTool(s, &sdk.Tool{Name: "remove_budget_account", Description: "Remove one of your accounts from a budget. Only possible while the account has no transactions in past months of the budget."}, …RemoveAccount… → Member: false)
```

- [ ] **Step 2: Catalogue + goldens**

In `internal/test/mcpparity/catalogue.go`: `create_budget` args gain `"account_ids":["<apiparity.OwnerAccount>"]`; the `set_budget_account_included` call becomes `remove_budget_account` (owner's account has an April-2024 transaction and the fixture clock… — check whether the call is expected to succeed: the catalogue's budget is created with `start_date 2024-05-01`, so an April transaction is before start → removable → 200) plus an `add_budget_account` call after it. Then:

```bash
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/ && git diff --stat internal/test/mcpparity/testdata
```

INSPECT the diff: expected changes are `tools/list` (renamed tools, `account_ids` schema), the create/get budget outputs (`filters.accounts` instead of `excludedAccountsIds`), and the two membership calls. Anything else (e.g. numbers moving) is a bug — investigate before committing.

- [ ] **Step 3: Run** — `go test ./internal/test/mcpparity/` → PASS.

- [ ] **Step 4: Commit** — `git commit -am "feat(mcp): add_budget_account/remove_budget_account, create_budget.account_ids; goldens"` (with trailer).

---

### Task 11: apiparity fixture, catalogue, goldens

**Files:**
- Modify: `internal/test/apiparity/fixture.go`, `catalogue_budget_access.go`, `catalogue_budget_plan.go`, `catalogue_budget_income.go`, any other catalogue file with `create-budget` or `exclude-account` (`grep -rn "create-budget\|exclude-account\|include-account" internal/test/apiparity/*.go`), goldens under `internal/test/apiparity/testdata/golden/`
- Modify: `internal/test/apiparity/guard_test.go` only if a route rename requires it (it scans registration files; the count stays ≥ 112).

- [ ] **Step 1: Fixture** — after the budgets are seeded (`fixture.go:153-159`), add `f.BudgetAccount(Budget, OwnerAccount)` and `f.BudgetAccount(Budget2, OwnerAccount)` (every owner-owned account the fixture creates — check for a second owner account; `SharedAccount` is guest-owned and shared, NOT a member, matching today's owner-only rule).

- [ ] **Step 2: Catalogue** — every `create-budget` body gains `"accountIds": []string{OwnerAccount}`; `budget_account_writes`: `exclude-account` → `remove-account`, `include-account` → `add-account` (same bodies; keep the "id" quirk comment). Add one call asserting the locked case if the fixture makes it natural: `Budget` started at fixture time (April 2024) with `Txn1` in April 2024, and the real wall clock is 2026 → OwnerAccount is LOCKED in `Budget`, so `remove-account` there returns 400 — good: keep `remove-account` first (expect 400 in the golden), then `add-account` (200, idempotent). Update the scenario comment accordingly.

- [ ] **Step 3: Regenerate + inspect**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff --stat internal/test/apiparity/testdata/golden
```

Expected diff: `filters` shape in every budget response (`accounts: [{id, removable}]`), the two renamed calls in `budget_account_writes`, `create-budget` responses' filters. Numbers (`available`, balances) must NOT change for existing scenarios — the fixture memberships mirror the previous population exactly. If any amount changes, stop and investigate.

- [ ] **Step 4: Run** — `go test ./internal/test/apiparity/` → PASS; `make go-lint` → PASS.

- [ ] **Step 5: Commit** — `git commit -am "test(apiparity): membership fixture rows, add/remove-account scenarios, goldens"` (with trailer).

---

### Task 12: SPA — DTOs, account field, dialogs, delete invalidation

**Files:**
- Modify: `web/src/api/budget.ts`, `web/src/api/dto/budget.ts`
- Modify: `web/src/features/budgets/BudgetAccountsField.tsx`, `BudgetDialog.tsx`, `BudgetUpdateDialog.tsx`, `BudgetsPage.tsx:151`, `BudgetPage.tsx:387`
- Modify: `web/src/features/accounts/queries.ts` (`useDeleteAccount`)
- Tests: `web/src/features/budgets/BudgetsPage.test.tsx`, `queries.test.tsx`, new `BudgetAccountsField.test.tsx`, `BudgetUpdateDialog.test.tsx` (create if absent)

- [ ] **Step 1: Failing tests**

Update `queries.test.tsx:92-110` expectations from `excludedAccounts` to `accountIds` (create sends `accountIds: ['a1']`; update sends `accountIds`). `BudgetsPage.test.tsx:104`: creating a budget with the dialog now requires toggling ≥1 account ON (all default OFF) and the body carries `accountIds: ['a2']`; add a case: submit with none selected shows `Select at least one account` and sends nothing. New `BudgetAccountsField.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BudgetAccountsField } from './BudgetAccountsField'
// wrap with the test providers used by sibling tests (i18n + query client)

describe('BudgetAccountsField', () => {
  const accounts = [
    { id: 'a1', name: 'Cash', icon: 'wallet', folderId: null } as any,
    { id: 'a2', name: 'Savings', icon: 'savings', folderId: null } as any,
  ]
  it('renders selected state and disables locked rows with a hint', () => {
    render(<BudgetAccountsField accounts={accounts} selected={new Set(['a1', 'a2'])} locked={new Set(['a1'])} onToggle={vi.fn()} />)
    const cash = screen.getByRole('switch', { name: 'include Cash' })
    expect(cash).toBeChecked()
    expect(cash).toBeDisabled()
    expect(screen.getByRole('switch', { name: 'include Savings' })).not.toBeDisabled()
    expect(screen.getByText("Accounts with transactions in past months can't be removed")).toBeInTheDocument()
    expect(screen.getByText('2 of 2 included')).toBeInTheDocument()
  })
  it('shows no hint when nothing is locked', () => {
    render(<BudgetAccountsField accounts={accounts} selected={new Set()} locked={new Set()} onToggle={vi.fn()} />)
    expect(screen.queryByText(/can't be removed/)).toBeNull()
  })
})
```

`BudgetUpdateDialog` test: given `budget.filters.accounts = [{id:'a1', removable:false}, {id:'a2', removable:true}]`, submitting sends `accountIds: ['a1','a2']`; toggling `a2` off sends `['a1']`; `a1`'s switch is disabled.

- [ ] **Step 2: Run** — `cd web && pnpm test -- budgets` → FAIL.

- [ ] **Step 3: Implement**

`api/dto/budget.ts`: `filters: { periodStart: string; periodEnd: string; accounts: { id: Id; removable: boolean }[] }`.
`api/budget.ts`: `CreateBudgetForm.accountIds: Id[]`, `UpdateBudgetForm.accountIds?: Id[]`; replace `excludeAccount/includeAccount` with `addAccount/removeAccount` hitting `/api/v1/budget/add-account` / `remove-account` (same body).
`BudgetAccountsField.tsx`: props `{ accounts, selected: Set<Id>, locked: Set<Id>, onToggle }`; `checked={selected.has(id)}`, `disabled={locked.has(id)}`; counter uses `selected.size`; render `<p className="text-[11px] text-muted-foreground">{t('budgets.modal.budget_form.accounts_locked_hint')}</p>` when `locked.size > 0`.
`BudgetDialog.tsx`: state `selected` (starts empty), `toggleAccount` adds/removes; validation `if (selected.size === 0) next.accounts = t('budgets.form.budget.accounts.validation.required')` rendered under the field (a `CardField`-style error line, mirror the name error); `onSubmit({ name, currencyId, accountIds: [...selected] })`. Update the `onSubmit` prop type and both call sites (`BudgetsPage.tsx:151`, `BudgetPage.tsx:387`) to pass `accountIds: form.accountIds`.
`BudgetUpdateDialog.tsx`: `selected = new Set(budget.filters.accounts.map(a => a.id))`, `locked = new Set(budget.filters.accounts.filter(a => !a.removable).map(a => a.id))`; mutation body `accountIds: [...selected]`.
No `METRICS` change: the SPA never called exclude/include-account (no keys exist), and membership edits ride `BUDGET_CREATE`/`BUDGET_UPDATE`, which already fire in `useCreateBudget`/`useUpdateBudgetDetail`.
`accounts/queries.ts` `useDeleteAccount.onSuccess`: add `queryClient.invalidateQueries({ queryKey: queryKeys.budget })` (use the budget query key(s) the budgets feature exports — `grep -n "queryKeys" web/src/features/budgets/queries.ts`; invalidate the get-budget/plan keys, and transactions since a correction may have been written: `queryClient.invalidateQueries({ queryKey: queryKeys.transactions })`).

- [ ] **Step 4: Run** — `cd web && pnpm test && pnpm lint && pnpm build` → PASS. Also `go test ./internal/test/i18ntest/` (frontend key coverage) → PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(web): budget membership — accountIds on create/update, locked rows, delete-account budget invalidation"` (with trailer).

---

### Task 13: Docs, full verification

**Files:**
- Modify: `CLAUDE.md` (Notable behaviours + API conventions bullets), `README.md` if it documents excluded accounts (`grep -rn "exclud" README.md docs/`)

- [ ] **Step 1: CLAUDE.md** — under **Notable behaviours** add:
  "**Budget account membership** is explicit (`budgets_accounts`): a budget counts exactly its member accounts, soft-deleted members included (their history keeps counting so `budgetedBefore`/`spentBefore` share one population). An account can be removed only while it has no transactions between the budget start and the first of the caller's current month (`budget.account_not_removable`); a departing participant's accounts leave with them. `create-budget` requires ≥1 owned `accountIds`; `update-budget.accountIds` is optional (absent = untouched, present = replace-set over the caller's own accounts). Deleting an account auto-writes a 'Balance adjustment (account deleted)' correction so deleted accounts always have zero balance, and the invariant is **self-healing**: creating a transaction against a deleted account is rejected (`transaction.account_deleted`), and deleting/editing a transaction that still touches one (e.g. an old transfer to a live account) re-zeroes the deleted side in the same DB transaction with a correction dated at the affected transaction's `spent_at` ('Balance adjustment (deleted transaction)' / '(edited transaction)')."
  Replace the two `exclude-account`/`include-account` mentions with `add-account`/`remove-account` (search `CLAUDE.md`).
- [ ] **Step 2: Full suite** — `make go-test` (coverage gate ≥ 80%) and `make test` (needs Postgres: compose auto-provisions, or set `DATABASE_TEST_PGSQL_URL`). Expected: PASS, including enginecompare byte parity of every golden and the pgsql rerun of the repo/unit suite (exercises the pgsql migration 000/002 and the new queries).
- [ ] **Step 3: Commit** — `git commit -am "docs: budget account membership"` (with trailer).
- [ ] **Step 4: Handoff** — push the branch and open a PR against `main` titled "fix: budget account membership — explicit, activity-locked; deleted accounts keep counting (#<issue if any>)" using the commit-commands:commit-push-pr skill or `gh pr create`; body summarises the spec §"Design summary" and lists the golden diff areas.
